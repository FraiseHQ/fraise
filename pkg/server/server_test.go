// MIT License

// Copyright (c) 2026 René-Jean Corneille

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// This is a white-box test. It lives in package server so it can drive the
// unexported router directly, without binding a real TCP port via Start.
package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/pkg/logger"
	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// newTestServer builds a fully wired server on the default config, with a
// started database and engine. Tests send requests through its router.
func newTestServer(t *testing.T) *Server[uint64, float32] {
	t.Helper()
	return newTestServerCfg(t, config.New())
}

// newTestServerCfg is newTestServer with a caller-supplied config, so a test
// can exercise a specific limit: body size, top or depth ceiling, or vector
// dimension. It registers cleanup that stops the engine's workers when the test
// ends.
func newTestServerCfg(t *testing.T, cfg *config.ConfigSet) *Server[uint64, float32] {
	t.Helper()
	s, err := New[uint64, float32](cfg, hash.NewHasher[uint64](cfg))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := s.DB.Start(); err != nil {
		t.Fatalf("db.Start returned error: %v", err)
	}
	if err := s.Engine.Start(); err != nil {
		t.Fatalf("engine.Start returned error: %v", err)
	}
	t.Cleanup(func() { s.Engine.Stop() })
	return s
}

// do sends an HTTP request through the server's router and returns the recorder.
func (s *Server[K, P]) do(method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

// TestHealthCheck checks that the root endpoint reports the server is alive.
func TestHealthCheck(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodGet, "/", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %q, want it to report status ok", w.Body.String())
	}
}

// TestFailuresOutsideTheHandlersKeepTheErrorShape pins the shape of the two
// failures gin answers itself. One is an unknown route, or a known path with
// the wrong method. The other is a recovered panic. Both use the {"error": ...}
// shape a client decodes every other failure with. A panic's detail stays in
// the log, never in the body.
func TestFailuresOutsideTheHandlersKeepTheErrorShape(t *testing.T) {
	s := newTestServer(t)
	s.router.GET("/panics", func(*gin.Context) { panic("boom") })

	cases := []struct {
		method, target string
		status         int
		want           string
	}{
		{http.MethodGet, "/api/v1/nope", http.StatusNotFound, `{"error":"no endpoint GET /api/v1/nope"}`},
		{http.MethodGet, "/api/v1/q", http.StatusNotFound, `{"error":"no endpoint GET /api/v1/q"}`},
		{http.MethodGet, "/panics", http.StatusInternalServerError, `{"error":"internal server error"}`},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			w := s.do(tc.method, tc.target, "")

			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tc.status, w.Body.String())
			}
			if got := w.Body.String(); got != tc.want {
				t.Errorf("body = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestQueryMalformedBody checks that a request with an invalid JSON body is
// rejected with 400.
func TestQueryMalformedBody(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodPost, "/api/v1/q", "{not json")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestQueryTextThatIsNotUTF8IsRejected pins that a query whose text decoding
// altered is rejected with 400 instead of stored. encoding/json replaces
// invalid UTF-8, and a \u escape for half a surrogate pair, with U+FFFD
// without an error, and the parser rejects U+FFFD. A fact written with either
// would be stored as text its client never sent.
func TestQueryTextThatIsNotUTF8IsRejected(t *testing.T) {
	bodies := map[string]string{
		"invalid bytes":                  "{\"query\":\"recall \xff\xfe\"}",
		"lone high surrogate":            `{"query":"remember 'a \ud800' topic:t"}`,
		"lone low surrogate":             `{"query":"remember 'a \udc00' topic:t"}`,
		"high surrogate before a letter": `{"query":"remember 'a \ud800\u0041' topic:t"}`,
		"pair in the wrong order":        `{"query":"remember 'a \ude00\ud83d' topic:t"}`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)

			w := s.do(http.MethodPost, "/api/v1/q", body)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
			}
			if want := "not UTF-8 text"; !strings.Contains(w.Body.String(), want) {
				t.Errorf("body = %q, want it to contain %q", w.Body.String(), want)
			}
		})
	}
}

// TestQueryEscapedTextRoundTrips is the other half of the pair: an escape that
// does spell UTF-8 text is stored as that text and returned unchanged. A
// surrogate pair is one character, and an escaped backslash before "u" starts
// no escape.
func TestQueryEscapedTextRoundTrips(t *testing.T) {
	cases := map[string]struct {
		phrase string
		want   string
	}{
		"surrogate pair":    {`smile \ud83d\ude00`, "smile \U0001F600"},
		"escaped backslash": {`path \\ud800`, `path \ud800`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)
			if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 '`+tc.phrase+`' topic:t"}`); w.Code != http.StatusOK {
				t.Fatalf("write: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
			}

			w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@0 topic:t"}`)
			if w.Code != http.StatusOK {
				t.Fatalf("recall: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
			}
			var resp struct {
				Results struct {
					Hits []struct {
						Value string `json:"value"`
					} `json:"hits"`
				} `json:"results"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decoding %s: %v", w.Body.String(), err)
			}
			if len(resp.Results.Hits) != 1 {
				t.Fatalf("hits = %d, want 1 (body: %s)", len(resp.Results.Hits), w.Body.String())
			}
			if got := resp.Results.Hits[0].Value; got != tc.want {
				t.Errorf("stored fact = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestQueryUnparsable checks that a syntactically invalid query string is
// rejected with 400 before it reaches the engine.
func TestQueryUnparsable(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"forget anna"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestQueryOutOfRangeGraph checks that a selector past the allocated graph range
// is rejected with 400 rather than failing deep in the scheduler.
func TestQueryOutOfRangeGraph(t *testing.T) {
	s := newTestServer(t)

	// DefaultNumGraph is 8, so @8 is one past the last valid selector.
	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@8 anna"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "does not exist") {
		t.Errorf("body = %q, want an out-of-range message", w.Body.String())
	}
}

// TestQueryWrappingGraphSelectorRejected checks that a selector past the uint8
// range is rejected with 400 before execution. Wrapped into a valid-looking
// graph (@256 -> 0, @300 -> 44), it would silently run against the wrong
// graph, breaking tenant isolation.
func TestQueryWrappingGraphSelectorRejected(t *testing.T) {
	s := newTestServer(t)

	for _, body := range []string{
		`{"query":"remember@256 'secret plan' topic:x"}`, // would wrap to graph 0
		`{"query":"remember@300 'secret plan' topic:x"}`, // would wrap to graph 44
		`{"query":"recall@256 secret"}`,
	} {
		w := s.do(http.MethodPost, "/api/v1/q", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want %d (%s)",
				body, w.Code, http.StatusBadRequest, w.Body.String())
		}
	}
}

// TestQueryBodyTooLarge checks that a request body over the configured cap is
// rejected with 400 before it is buffered and bound.
func TestQueryBodyTooLarge(t *testing.T) {
	cfg := config.New()
	cfg.Server.MaxBodyBytes = 32 // tiny cap so a normal-looking body overflows
	s := newTestServerCfg(t, cfg)

	body := `{"query":"recall@0 ` + strings.Repeat("a", 256) + `"}`
	w := s.do(http.MethodPost, "/api/v1/q", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// TestQueryTopOverCeiling checks that a recall asking for more results than the
// top ceiling is rejected with 400 rather than clamped.
func TestQueryTopOverCeiling(t *testing.T) {
	cfg := config.New()
	cfg.DB.MaxTop = 10
	s := newTestServerCfg(t, cfg)

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@0 anna top:99"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "out of range") {
		t.Errorf("body = %q, want a limit message", w.Body.String())
	}
}

// TestQueryDepthOverCeiling checks that a recall walking deeper than the depth
// ceiling is rejected with 400.
func TestQueryDepthOverCeiling(t *testing.T) {
	cfg := config.New()
	cfg.DB.MaxDepth = 2
	s := newTestServerCfg(t, cfg)

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@0 anna depth:9"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "out of range") {
		t.Errorf("body = %q, want a limit message", w.Body.String())
	}
}

// TestQueryVectorTooLarge checks that a bound vector longer than the configured
// dimension ceiling is rejected with 400 before it reaches the index.
func TestQueryVectorTooLarge(t *testing.T) {
	cfg := config.New()
	cfg.DB.MaxVectorDimension = 3
	s := newTestServerCfg(t, cfg)

	w := s.do(http.MethodPost, "/api/v1/q",
		`{"query":"remember@0 'a fact' vec:$v","parameters":{"v":[1,2,3,4]}}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "dimensions") {
		t.Errorf("body = %q, want a dimension-limit message", w.Body.String())
	}
}

// TestQueryVectorDimensionMismatch checks that a write whose vector does not
// match the graph's established dimension is a client error. It answers 400
// with the expected and supplied dimensions, not an opaque 500. The first
// vector fixes the dimension, so the mismatch is only detectable at commit
// time. The test pins the commit error surviving the scheduler intact.
func TestQueryVectorDimensionMismatch(t *testing.T) {
	s := newTestServer(t)

	// First write fixes graph 0's vector dimension at 3.
	w := s.do(http.MethodPost, "/api/v1/q",
		`{"query":"remember@0 'vec one' vec:$v","parameters":{"v":[0.1,0.2,0.3]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("first write status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	w = s.do(http.MethodPost, "/api/v1/q",
		`{"query":"remember@0 'vec two' vec:$v","parameters":{"v":[0.1,0.2,0.3,0.4]}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "expects 3, got 4") {
		t.Errorf("body = %q, want the expected vs supplied dimensions", w.Body.String())
	}
}

// TestRecallVectorDimensionMismatch is the read half of the dimension rule. A
// recall carrying a vector of the wrong width is rejected with 400 naming both
// dimensions, as a write is. A 200 from the text index alone would drop the
// semantic half of the question without a word.
func TestRecallVectorDimensionMismatch(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodPost, "/api/v1/q",
		`{"query":"remember@0 'vec one' vec:$v","parameters":{"v":[0.1,0.2,0.3]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("seeding write status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	w = s.do(http.MethodPost, "/api/v1/q",
		`{"query":"recall@0 'vec' vec:$v","parameters":{"v":[0.1,0.2]}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "expects 3, got 2") {
		t.Errorf("body = %q, want the expected vs supplied dimensions", w.Body.String())
	}
}

// TestQuerySuccess checks that a well-formed recall query is planned, executed,
// and returns 200 with a results payload. The graph is seeded first. A recall
// of a graph that holds nothing is answered 204, so an unseeded graph would
// exercise that path instead of this one.
func TestQuerySuccess(t *testing.T) {
	s := newTestServer(t)

	if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 'anna sent the invoice'"}`); w.Code != http.StatusOK {
		t.Fatalf("seeding write: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@0 anna"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "results") {
		t.Errorf("body = %q, want a results payload", w.Body.String())
	}
}

// TestQueryWriteIsAcknowledged pins the write response apart from a read's.
// With a recall's empty envelope, {"count":0,"hits":[]} would mean both
// "stored" and "matched nothing". The status token is what tells them apart.
// The MCP bridge returns it as its remember tool's output, so if this test has
// to change, the bridge changes with it.
func TestQueryWriteIsAcknowledged(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 'anna sent the invoice' topic:billing"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if got, want := strings.TrimSpace(w.Body.String()), `{"status":"ok"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestQueryEmptyGraphIsNoContent checks the third answer a query can give. A
// recall of a graph holding nothing comes back 204 with no body at all. It is
// neither an error nor a miss. The caller has yet to write anything here, so no
// rephrasing of the query would have helped. That is what separates it from
// TestQueryNoMatchOnPopulatedGraphIsEmptyResults below.
func TestQueryEmptyGraphIsNoContent(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@0 anna"}`)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusNoContent, w.Body.String())
	}
	if body := w.Body.String(); body != "" {
		t.Errorf("body = %q, want it empty: 204 must not carry one", body)
	}
}

// TestQueryNoMatchOnPopulatedGraphIsEmptyResults is the other half of the pair.
// The graph holds facts and none of them matched, so the answer stays 200 with
// an empty result set. A caller that cannot tell this from the 204 debugs its
// query when it should be checking whether it ever wrote.
func TestQueryNoMatchOnPopulatedGraphIsEmptyResults(t *testing.T) {
	s := newTestServer(t)

	if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 'anna sent the invoice'"}`); w.Code != http.StatusOK {
		t.Fatalf("seeding write: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@0 zzznomatchzzz"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"count":0`) {
		t.Errorf("body = %q, want an empty result set", w.Body.String())
	}
}

// TestQueryEmptyGraphIsPerGraph checks that emptiness is read off the graph the
// query selected, not the store as a whole. Writing to graph 0 must not stop
// graph 1 from answering 204. The selector is the unit of isolation everywhere
// else in the API. A store-wide check would silently make the 204 mean "nothing
// has ever been written anywhere".
func TestQueryEmptyGraphIsPerGraph(t *testing.T) {
	s := newTestServer(t)
	if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 'anna sent the invoice'"}`); w.Code != http.StatusOK {
		t.Fatalf("seeding write: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@1 anna"}`)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// readBody is a read's 200 response as a client decodes it: the result, with
// the time a when answers with and the times each fact was given, and the
// warnings beside it.
type readBody struct {
	Results  readResults `json:"results"`
	Warnings []string    `json:"warnings"`
}

type readResults struct {
	Count int        `json:"count"`
	Start string     `json:"start"`
	End   string     `json:"end"`
	Hits  []readHit  `json:"hits"`
	When  *readWhens `json:"when"`
}

type readWhens struct {
	Since *readResults `json:"since"`
	Until *readResults `json:"until"`
}

type readHit struct {
	Value    string     `json:"value"`
	Said     string     `json:"said"`
	Occurred []readSpan `json:"occurred"`
	Decay    float64    `json:"decay"`
}

type readSpan struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// read posts query to target and decodes the 200 it must answer with.
func (s *Server[K, P]) read(t *testing.T, target, query string) readBody {
	t.Helper()
	request, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("encoding %q: %v", query, err)
	}
	w := s.do(http.MethodPost, target, string(request))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want %d (body: %s)", query, w.Code, http.StatusOK, w.Body.String())
	}
	var body readBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: decoding %s: %v", query, w.Body.String(), err)
	}
	return body
}

// newTrainingServer returns a server whose graph 0 holds three dated facts
// about anna: shoes bought on 1 May 2023 and told on 2 May, a marathon run on
// 7 May and told on 8 May, and a rest taken from 9 May that is not over and
// was told at no given time.
func newTrainingServer(t *testing.T) *Server[uint64, float32] {
	t.Helper()
	s := newTestServer(t)
	for _, statement := range []string{
		"remember@0 'anna bought shoes' said:'2023-05-02' occurred:'2023-05-01'",
		"remember@0 'anna ran the marathon' said:'2023-05-08' occurred:'2023-05-07'",
		"remember@0 'anna rested' occurred:'2023-05-09'..",
	} {
		request, err := json.Marshal(map[string]string{"query": statement})
		if err != nil {
			t.Fatalf("encoding %q: %v", statement, err)
		}
		if w := s.do(http.MethodPost, "/api/v1/q", string(request)); w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want %d (body: %s)", statement, w.Code, http.StatusOK, w.Body.String())
		}
	}
	return s
}

// TestQueryWhenAnswersWithATime pins the response of a when statement. It is a
// read's envelope whose result also carries the time the when answers with:
// start and end, and no end for an answer that is not over. Each hit carries
// the times its fact was given, said and occurred, and neither key for a time
// the fact was not given.
func TestQueryWhenAnswersWithATime(t *testing.T) {
	s := newTrainingServer(t)
	tests := map[string]readResults{
		"when marathon": {
			Count: 1,
			Start: "2023-05-07T00:00:00Z",
			End:   "2023-05-08T00:00:00Z",
			Hits: []readHit{{
				Value:    "anna ran the marathon",
				Said:     "2023-05-08T00:00:00Z",
				Occurred: []readSpan{{Start: "2023-05-07T00:00:00Z", End: "2023-05-08T00:00:00Z"}},
			}},
		},
		"when:said marathon": {
			Count: 1,
			Start: "2023-05-08T00:00:00Z",
			End:   "2023-05-08T00:00:00Z",
			Hits: []readHit{{
				Value:    "anna ran the marathon",
				Said:     "2023-05-08T00:00:00Z",
				Occurred: []readSpan{{Start: "2023-05-07T00:00:00Z", End: "2023-05-08T00:00:00Z"}},
			}},
		},
		"when rested": {
			Count: 1,
			Start: "2023-05-09T00:00:00Z",
			Hits: []readHit{{
				Value:    "anna rested",
				Occurred: []readSpan{{Start: "2023-05-09T00:00:00Z"}},
			}},
		},
	}
	for query, want := range tests {
		t.Run(query, func(t *testing.T) {
			got := s.read(t, "/api/v1/q", query)

			if diff := cmp.Diff(want, got.Results); diff != "" {
				t.Errorf("%s results mismatch (-want +got):\n%s", query, diff)
			}
			if len(got.Warnings) != 0 {
				t.Errorf("%s warnings = %q, want none", query, got.Warnings)
			}
		})
	}
}

// TestQueryRecallReadsItsBoundFromAWhen pins a nested when end to end: parsed,
// planned, scheduled and committed as part of the recall that holds it. until
// keeps the facts that happened before the answer, and since the ones that
// happened after it. A when that names a clock moves the bound onto it:
// since:(when:said marathon) keeps the facts told from 8 May on, and the rest
// is one of them, since a fact told at no given time was told when it was
// written.
func TestQueryRecallReadsItsBoundFromAWhen(t *testing.T) {
	s := newTrainingServer(t)
	tests := map[string][]string{
		"recall@0 anna until:(when marathon)":      {"anna bought shoes"},
		"recall@0 anna since:(when marathon)":      {"anna rested"},
		"recall@0 anna since:(when:said marathon)": {"anna ran the marathon", "anna rested"},
	}
	for query, want := range tests {
		t.Run(query, func(t *testing.T) {
			body := s.read(t, "/api/v1/q", query)

			got := make([]string, len(body.Results.Hits))
			for i, hit := range body.Results.Hits {
				got[i] = hit.Value
			}
			if diff := cmp.Diff(want, got, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("%s hits mismatch (-want +got):\n%s", query, diff)
			}
			if len(body.Warnings) != 0 {
				t.Errorf("%s warnings = %q, want none", query, body.Warnings)
			}
		})
	}
}

// TestQueryWarnsWhenAWhenMatchedNoFact pins the whole response for a when that
// has no time to give. The query ran, so the answer is a 200 with an empty
// result, and the warning says why it is empty: a recall whose bound went
// unanswered returns no hits rather than search without the bound. The
// parser's warnings come first, then the ones raised while the query ran.
func TestQueryWarnsWhenAWhenMatchedNoFact(t *testing.T) {
	const unanswered = "the when matched no fact, so it has no time to answer with"
	s := newTrainingServer(t)
	tests := map[string][]string{
		"when zebras": {unanswered},
		"recall@0 anna until:(when zebras)": {
			"until: " + unanswered + "; the recall has no bound to read and returns no hits",
		},
		"recall@0 the anna since:(when zebras)": {
			`parse warning at column 12: term "the" is a stop word: stored facts never contain it, so it cannot match`,
			"since: " + unanswered + "; the recall has no bound to read and returns no hits",
		},
	}
	for query, warnings := range tests {
		t.Run(query, func(t *testing.T) {
			request, err := json.Marshal(map[string]string{"query": query})
			if err != nil {
				t.Fatalf("encoding %q: %v", query, err)
			}

			w := s.do(http.MethodPost, "/api/v1/q", string(request))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
			}
			rendered, err := json.Marshal(warnings)
			if err != nil {
				t.Fatalf("encoding the warnings: %v", err)
			}
			want := `{"results":{"count":0,"hits":[]},"warnings":` + string(rendered) + `}`
			if diff := cmp.Diff(want, strings.TrimSpace(w.Body.String())); diff != "" {
				t.Errorf("body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestQueryWhenOnAnEmptyGraphIsNoContent pins that a when is a read where the
// empty graph is concerned. As a statement or in a bound, a when over a graph
// that holds nothing is answered 204 with no body, as a recall is: the caller
// has yet to write, and no warning about a missing match would say so.
func TestQueryWhenOnAnEmptyGraphIsNoContent(t *testing.T) {
	s := newTestServer(t)
	queries := []string{
		"when marathon",
		"recall@0 anna until:(when marathon)",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			w := s.do(http.MethodPost, "/api/v1/q", `{"query":"`+query+`"}`)

			if w.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusNoContent, w.Body.String())
			}
			if body := w.Body.String(); body != "" {
				t.Errorf("body = %q, want it empty: 204 must not carry one", body)
			}
		})
	}
}

// TestExplainCarriesTheWhenItsBoundRead pins what explain adds for the
// temporal clauses. Each hit carries decay, the recency factor its relevance
// was scaled by. A recall whose bound holds a when carries that when's result
// under when, keyed by the bound: the fact it matched and the time it gave.
// The plain query route carries neither, so an ordinary response does not
// grow.
func TestExplainCarriesTheWhenItsBoundRead(t *testing.T) {
	const query = "recall@0 anna until:(when rested)"
	marathon := readHit{
		Value:    "anna ran the marathon",
		Said:     "2023-05-08T00:00:00Z",
		Occurred: []readSpan{{Start: "2023-05-07T00:00:00Z", End: "2023-05-08T00:00:00Z"}},
	}
	shoes := readHit{
		Value:    "anna bought shoes",
		Said:     "2023-05-02T00:00:00Z",
		Occurred: []readSpan{{Start: "2023-05-01T00:00:00Z", End: "2023-05-02T00:00:00Z"}},
	}
	rested := readHit{
		Value:    "anna rested",
		Occurred: []readSpan{{Start: "2023-05-09T00:00:00Z"}},
	}
	byValue := cmpopts.SortSlices(func(a, b readHit) bool { return a.Value < b.Value })
	withoutDecay := cmpopts.IgnoreFields(readHit{}, "Decay")
	s := newTrainingServer(t)

	t.Run("explain", func(t *testing.T) {
		got := s.read(t, "/api/v1/explain", query).Results

		want := readResults{
			Count: 2,
			Hits:  []readHit{shoes, marathon},
			When: &readWhens{Until: &readResults{
				Count: 1,
				Start: "2023-05-09T00:00:00Z",
				Hits:  []readHit{rested},
			}},
		}
		if diff := cmp.Diff(want, got, byValue, withoutDecay); diff != "" {
			t.Errorf("explain results mismatch (-want +got):\n%s", diff)
		}
		for _, hit := range got.Hits {
			if hit.Decay <= 0 || hit.Decay > 1 {
				t.Errorf("hit %q decay = %v, want a recency factor in (0, 1]", hit.Value, hit.Decay)
			}
		}
	})

	t.Run("query", func(t *testing.T) {
		got := s.read(t, "/api/v1/q", query).Results

		want := readResults{Count: 2, Hits: []readHit{shoes, marathon}}
		if diff := cmp.Diff(want, got, byValue); diff != "" {
			t.Errorf("query results mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestExplainAcceptsAWhen pins that explain serves every read. A when ranks
// as the recall it runs, so it has a ranking to explain, and its answer comes
// back with it. A remember stays refused: probing a ranking never writes.
func TestExplainAcceptsAWhen(t *testing.T) {
	s := newTrainingServer(t)

	got := s.read(t, "/api/v1/explain", "when marathon").Results

	if got.Start != "2023-05-07T00:00:00Z" || got.End != "2023-05-08T00:00:00Z" {
		t.Errorf("explained when answered %q to %q, want 7 May to 8 May 2023", got.Start, got.End)
	}
	if len(got.Hits) != 1 || got.Hits[0].Decay <= 0 {
		t.Errorf("explained when hits = %+v, want its one fact with a recency factor", got.Hits)
	}
	if w := s.do(http.MethodPost, "/api/v1/explain", `{"query":"remember@0 'anna slept' said:'2023-05-10'"}`); w.Code != http.StatusBadRequest {
		t.Errorf("explaining a remember: status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// TestStatsEndpoint checks that GET /api/v1/stats returns one snapshot per
// graph, naming the relevance model each graph ranks with. A committed write
// with a vector shows in the counts. That includes the forest_entries gauge the
// end-to-end forest-bloat test reads.
func TestStatsEndpoint(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodGet, "/api/v1/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	var stats struct {
		Graphs []struct {
			ID            int    `json:"id"`
			Nodes         int    `json:"nodes"`
			Vectors       int    `json:"vectors"`
			ForestEntries int    `json:"forest_entries"`
			Relevance     string `json:"relevance"`
		} `json:"graphs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("stats body is not valid JSON: %v (body: %s)", err, w.Body.String())
	}
	if got, want := len(stats.Graphs), s.DB.NumGraphs(); got != want {
		t.Fatalf("len(graphs) = %d, want %d", got, want)
	}
	for _, g := range stats.Graphs {
		if g.Relevance != s.Config.DB.RelevanceModel.Name {
			t.Errorf("graphs[%d].relevance = %q, want %q", g.ID, g.Relevance, s.Config.DB.RelevanceModel.Name)
		}
	}

	w = s.do(http.MethodPost, "/api/v1/q",
		`{"query":"remember@0 'stats probe fact' vec:$v topic:probe","parameters":{"v":[1,2,3]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("remember status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	w = s.do(http.MethodGet, "/api/v1/stats", "")
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("stats body is not valid JSON: %v", err)
	}
	g0 := stats.Graphs[0]
	if g0.Vectors != 1 {
		t.Errorf("graphs[0].vectors = %d, want 1", g0.Vectors)
	}
	if g0.ForestEntries < g0.Vectors || g0.ForestEntries > 2*g0.Vectors {
		t.Errorf("graphs[0].forest_entries = %d, want within [vectors, 2*vectors] = [%d, %d]",
			g0.ForestEntries, g0.Vectors, 2*g0.Vectors)
	}
}

// captureStdout swaps os.Stdout for a pipe while fn runs and returns what was
// written to it. The log handler captures os.Stdout when it is built, so fn
// must build and install the logger too, not just log through one.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured output: %v", err)
	}
	return string(out)
}

// TestQueryLogsCarryShapeNotContent pins that a remembered fact never reaches
// the log at the default level. A query is what a caller remembers, and stdout
// lands in every log aggregator the server runs under, so the text there is
// personal data copied out of the store. At INFO the log still says that the
// query ran, with its graph, hit count and duration. The text appears only at
// DEBUG, where an operator has opted in. That covers the errors too: a parse
// error that quotes the query, and a write that fails at commit, which the
// scheduler and the handler log at Error. The DEBUG row keeps the marker visible, so the INFO row's absence
// comes from the level and not from a query that never ran.
func TestQueryLogsCarryShapeNotContent(t *testing.T) {
	const private = "zzprivate"

	cases := map[string]struct {
		level       string
		wantContent bool
	}{
		"info hides the text":  {level: config.LogLevelInfo, wantContent: false},
		"debug shows the text": {level: config.LogLevelDebug, wantContent: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := config.New()
			cfg.Log.Level = tc.level
			t.Cleanup(func() { logger.SetDefault(nil) })

			got := captureStdout(t, func() {
				logger.SetDefault(logger.NewLogger(cfg))
				s := newTestServerCfg(t, cfg)

				if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 '`+private+` anna sent the invoice'"}`); w.Code != http.StatusOK {
					t.Fatalf("remember: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
				}
				if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@0 `+private+`"}`); w.Code != http.StatusOK {
					t.Fatalf("recall: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
				}
				// The second quoted phrase is unexpected, and the parse error
				// quotes it back.
				if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 'anna' '`+private+` second fact'"}`); w.Code != http.StatusBadRequest {
					t.Fatalf("unparsable: status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
				}
				// The first vector fixes the graph's dimension at 3, so the
				// second fails at commit, past the parser, and its error is
				// logged at Error by the scheduler and the handler.
				if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 'vec one' vec:$v","parameters":{"v":[0.1,0.2,0.3]}}`); w.Code != http.StatusOK {
					t.Fatalf("first vector: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
				}
				if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"remember@0 '`+private+` vector fact' vec:$v","parameters":{"v":[0.1,0.2,0.3,0.4]}}`); w.Code != http.StatusBadRequest {
					t.Fatalf("mismatched vector: status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
				}
			})

			if gotContent := strings.Contains(got, private); gotContent != tc.wantContent {
				t.Errorf("log contains %q = %t, want %t; log:\n%s", private, gotContent, tc.wantContent, got)
			}
			for _, want := range []string{`msg="Query executed"`, "graph=0", "hits=1", "duration=", `msg="Rejecting unparsable query"`} {
				if !strings.Contains(got, want) {
					t.Errorf("log is missing %q; log:\n%s", want, got)
				}
			}
		})
	}
}
