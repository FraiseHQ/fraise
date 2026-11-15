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
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/internal/query"
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
// warnings beside it. Of an explained recall's blocks it reads what the
// temporal clauses add: each hit's recency factor and the answer of each when
// a bound read.
type readBody struct {
	Results  readResults `json:"results"`
	Warnings []string    `json:"warnings"`
}

type readResults struct {
	Count   int          `json:"count"`
	Start   string       `json:"start"`
	End     string       `json:"end"`
	Hits    []readHit    `json:"hits"`
	Explain *readExplain `json:"explain"`
}

type readExplain struct {
	When *readWhens `json:"when"`
}

type readWhens struct {
	Since *readResults `json:"since"`
	Until *readResults `json:"until"`
}

type readHit struct {
	Value    string          `json:"value"`
	Said     string          `json:"said"`
	Occurred []readSpan      `json:"occurred"`
	Explain  *readHitExplain `json:"explain"`
}

type readHitExplain struct {
	Decay float64 `json:"decay"`
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

// TestQueryRememberRestatedInAnotherOffsetStoresIt pins that a fact stated
// again at the same instant, written in another offset, is stored in the
// offset the second remember gives. A hit prints the day in its stored offset,
// so keeping the first offset would print the 9th where the caller said the
// 8th. Both remembers pass through the plan cache, which must not answer the
// second with the first's plan: what a remember writes cannot depend on what
// the cache holds.
func TestQueryRememberRestatedInAnotherOffsetStoresIt(t *testing.T) {
	s := newTestServer(t)
	for _, statement := range []string{
		"remember@2 'rosa rows' said:'2023-05-09T00:30:00+09:00'",
		"remember@2 'rosa rows' said:'2023-05-08T15:30:00Z'",
	} {
		if w := s.do(http.MethodPost, "/api/v1/q", `{"query":"`+statement+`"}`); w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want %d (body: %s)", statement, w.Code, http.StatusOK, w.Body.String())
		}
	}

	hits := s.read(t, "/api/v1/q", "recall@2 rosa").Results.Hits
	got := make([]string, len(hits))
	for i, hit := range hits {
		got[i] = hit.Said
	}
	if diff := cmp.Diff([]string{"2023-05-08T15:30:00Z"}, got); diff != "" {
		t.Errorf("said mismatch (-want +got):\n%s", diff)
	}
}

// TestQueryWhenOnAnEmptyGraphIsNoContent pins that a when is a read where the
// empty graph is concerned. As a statement or in a bound, a when over a graph
// that holds nothing is answered 204 with no body, as a recall is: the caller
// has yet to write, and no warning about a missing match would say so.
func TestQueryWhenOnAnEmptyGraphIsNoContent(t *testing.T) {
	s := newTestServer(t)
	queries := map[string]struct{}{
		"when marathon":                       {},
		"recall@0 anna until:(when marathon)": {},
	}
	for query := range queries {
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

// TestExplainCarriesTheWhenItsBoundRead pins what an explained recall adds for
// the temporal clauses. Each hit's block carries decay, the recency factor its
// relevance was scaled by. A recall whose bound holds a when carries that
// when's result under when in its own block, keyed by the bound: the fact it
// matched and the time it gave. The nested when explains itself with the
// recall, so its result carries its own blocks. The prefix asks for it on
// /api/v1/q, and the deprecated alias answers the plain recall the same way.
// The plain recall on /api/v1/q carries neither block, so an ordinary
// response does not grow.
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
	withoutBlock := cmpopts.IgnoreFields(readHit{}, "Explain")
	decayed := func(t *testing.T, hits []readHit) {
		t.Helper()
		for _, hit := range hits {
			if hit.Explain == nil || hit.Explain.Decay <= 0 || hit.Explain.Decay > 1 {
				t.Errorf("hit %q explain = %+v, want a block whose decay is a recency factor in (0, 1]", hit.Value, hit.Explain)
			}
		}
	}
	s := newTrainingServer(t)

	explained := map[string]struct{ target, query string }{
		"the prefix": {"/api/v1/q", "explain " + query},
		"the alias":  {"/api/v1/explain", query},
	}
	for name, tc := range explained {
		t.Run(name, func(t *testing.T) {
			got := s.read(t, tc.target, tc.query).Results

			want := readResults{
				Count: 2,
				Hits:  []readHit{shoes, marathon},
				Explain: &readExplain{When: &readWhens{Until: &readResults{
					Count:   1,
					Start:   "2023-05-09T00:00:00Z",
					Hits:    []readHit{rested},
					Explain: &readExplain{},
				}}},
			}
			if diff := cmp.Diff(want, got, byValue, withoutBlock); diff != "" {
				t.Errorf("explained results mismatch (-want +got):\n%s", diff)
			}
			decayed(t, got.Hits)
			if got.Explain != nil && got.Explain.When != nil && got.Explain.When.Until != nil {
				decayed(t, got.Explain.When.Until.Hits)
			}
		})
	}

	t.Run("plain", func(t *testing.T) {
		got := s.read(t, "/api/v1/q", query).Results

		want := readResults{Count: 2, Hits: []readHit{shoes, marathon}}
		if diff := cmp.Diff(want, got, byValue); diff != "" {
			t.Errorf("query results mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestExplainAcceptsAWhen pins that the deprecated alias serves every read. A
// when ranks as the recall it runs, so the alias explains it as that recall,
// and its answer comes back with it. A remember stays refused: probing a
// ranking never writes.
func TestExplainAcceptsAWhen(t *testing.T) {
	s := newTrainingServer(t)

	got := s.read(t, "/api/v1/explain", "when marathon").Results

	if got.Start != "2023-05-07T00:00:00Z" || got.End != "2023-05-08T00:00:00Z" {
		t.Errorf("explained when answered %q to %q, want 7 May to 8 May 2023", got.Start, got.End)
	}
	if len(got.Hits) != 1 || got.Hits[0].Explain == nil || got.Hits[0].Explain.Decay <= 0 {
		t.Errorf("explained when hits = %+v, want its one fact with a recency factor", got.Hits)
	}
	if w := s.do(http.MethodPost, "/api/v1/explain", `{"query":"remember@0 'anna slept' said:'2023-05-10'"}`); w.Code != http.StatusBadRequest {
		t.Errorf("explaining a remember: status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// explainFacts are the facts the explain tests recall from: two filed under
// anna, one of them holding the word invoice. explainedRecall seeds from that
// match and runs the anchor round through anna, so its envelope carries every
// key an explained recall can.
var explainFacts = []string{
	`remember@0 'anna sent the invoice' entity:anna`,
	`remember@0 'anna paid the rent' entity:anna`,
}

const explainedRecall = "recall@0 invoice entity:anna depth:2"

// seedExplainFacts writes explainFacts to graph 0 of s.
func seedExplainFacts(t *testing.T, s *Server[uint64, float32]) {
	t.Helper()
	for _, q := range explainFacts {
		body, _ := json.Marshal(HandleQueryRequest[float32]{Query: q})
		if w := s.do(http.MethodPost, "/api/v1/q", string(body)); w.Code != http.StatusOK {
			t.Fatalf("seeding %q: status = %d, want %d (body: %s)", q, w.Code, http.StatusOK, w.Body.String())
		}
	}
}

// postQuery sends q to target on s and returns the recorder.
func postQuery(t *testing.T, s *Server[uint64, float32], target, q string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(HandleQueryRequest[float32]{Query: q})
	if err != nil {
		t.Fatalf("encoding %q: %v", q, err)
	}
	return s.do(http.MethodPost, target, string(body))
}

// explainKeys reads which explain keys a recall response carries: the query
// block's, nil without one, and each hit's block's, nil for a hit without
// one. The values move with the data and the clock. The keys are the
// envelope's contract.
func explainKeys(t *testing.T, body []byte) ([]string, [][]string) {
	t.Helper()
	var resp struct {
		Results struct {
			Hits []struct {
				Explain map[string]json.RawMessage `json:"explain"`
			} `json:"hits"`
			Explain map[string]json.RawMessage `json:"explain"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("recall body is not valid JSON: %v (body: %s)", err, body)
	}
	keys := func(m map[string]json.RawMessage) []string {
		if m == nil {
			return nil
		}
		return slices.Sorted(maps.Keys(m))
	}
	hits := make([][]string, len(resp.Results.Hits))
	for i, hit := range resp.Results.Hits {
		hits[i] = keys(hit.Explain)
	}
	return keys(resp.Results.Explain), hits
}

// TestQueryExplainCarriesTheBlocks pins which recalls answer in the explained
// envelope. On /api/v1/q a statement explains by its own prefix: each hit
// carries its block, seed, decay and contributions, and the results the
// query's, whose admission bars are there only when the anchor round ran. A
// plain recall carries no explain key at either level, so it costs no
// response tokens. The deprecated alias answers a recall as the prefixed
// statement, with the prefix written or not.
func TestQueryExplainCarriesTheBlocks(t *testing.T) {
	hit := []string{"contributions", "decay", "seed"}
	cases := map[string]struct {
		target string
		query  string
		recall []string
		hit    []string
	}{
		"the prefix explains on /q": {
			target: "/api/v1/q", query: "explain " + explainedRecall,
			recall: []string{"admission", "background", "seeds", "terms", "timings"}, hit: hit,
		},
		"a plain recall on /q carries no block": {
			target: "/api/v1/q", query: explainedRecall,
		},
		"a recall naming no anchor runs no round and has no bars": {
			target: "/api/v1/q", query: "explain recall@0 invoice",
			recall: []string{"background", "seeds", "terms", "timings"}, hit: hit,
		},
		"the alias explains a plain recall": {
			target: "/api/v1/explain", query: explainedRecall,
			recall: []string{"admission", "background", "seeds", "terms", "timings"}, hit: hit,
		},
		"the alias explains a prefixed recall": {
			target: "/api/v1/explain", query: "explain " + explainedRecall,
			recall: []string{"admission", "background", "seeds", "terms", "timings"}, hit: hit,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)
			seedExplainFacts(t, s)

			w := postQuery(t, s, tc.target, tc.query)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
			}

			recall, hits := explainKeys(t, w.Body.Bytes())
			if len(hits) == 0 {
				t.Fatalf("no hits, want the invoice match at least (body: %s)", w.Body.String())
			}
			if diff := cmp.Diff(tc.recall, recall); diff != "" {
				t.Errorf("results.explain keys mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(slices.Repeat([][]string{tc.hit}, len(hits)), hits); diff != "" {
				t.Errorf("hits[].explain keys mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// explainedHit is a hit as a client decodes it from an explained recall.
type explainedHit struct {
	Value   string                     `json:"value"`
	Score   float32                    `json:"score"`
	Explain *query.HitExplain[float32] `json:"explain"`
}

// explainedBody is an explained recall's response as a client decodes it,
// into the wire types the server encodes from.
type explainedBody struct {
	Results struct {
		Count   int                                   `json:"count"`
		Hits    []explainedHit                        `json:"hits"`
		Explain *query.RecallExplain[uint64, float32] `json:"explain"`
	} `json:"results"`
}

// TestExplainAliasAnswersAsThePrefix pins the deprecated alias to the
// statement that replaces it. A recall sent to /api/v1/explain, with the
// prefix or without, answers exactly as the prefixed recall does on
// /api/v1/q: the same hits, seeds, contributions, rate, bars and terms. So a
// caller moving off the alias reads the same envelope. Only the scores and the
// recency factors, which decay moves between requests, and the timings, which
// are wall time, are left out.
func TestExplainAliasAnswersAsThePrefix(t *testing.T) {
	decode := func(t *testing.T, w *httptest.ResponseRecorder) explainedBody {
		t.Helper()
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
		}
		var body explainedBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("recall body is not valid JSON: %v (body: %s)", err, w.Body.String())
		}
		return body
	}
	cases := map[string]struct{}{
		explainedRecall:              {},
		"explain " + explainedRecall: {},
	}

	for q := range cases {
		t.Run(q, func(t *testing.T) {
			s := newTestServer(t)
			seedExplainFacts(t, s)

			want := decode(t, postQuery(t, s, "/api/v1/q", "explain "+explainedRecall))
			got := decode(t, postQuery(t, s, "/api/v1/explain", q))

			if diff := cmp.Diff(want, got,
				cmpopts.IgnoreFields(explainedHit{}, "Score"),
				cmpopts.IgnoreFields(query.HitExplain[float32]{}, "Decay"),
				cmpopts.IgnoreFields(query.RecallExplain[uint64, float32]{}, "Timings"),
			); diff != "" {
				t.Errorf("alias answer mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestExplainAnswersWithoutAnEnvelope pins the explained recalls and the
// misplaced prefixes that answer with no envelope at all. A prefix before a
// remember or a when is a parse error on either route, blamed at the verb's
// column, since only a recall has a ranking to explain: a remember stores a
// fact and a when answers with a time. The alias rejects a write it
// is sent plainly, so probing a ranking there never mutates a graph. An
// explained recall of a graph that holds nothing is a 204 like any recall,
// on either route: there is nothing to explain.
func TestExplainAnswersWithoutAnEnvelope(t *testing.T) {
	const (
		prefixedWrite = `{"error":"parse error at column 16: \"remember\" has no ranking to explain: the explain prefix stands only before recall — drop the prefix"}`
		prefixedWhen  = `{"error":"parse error at column 12: \"when\" answers with a time and has no ranking to explain: the explain prefix stands only before recall — drop the prefix"}`
	)
	cases := map[string]struct {
		target string
		query  string
		status int
		body   string
	}{
		"a prefixed remember on /q": {
			target: "/api/v1/q", query: "explain remember@0 'anna sent the invoice'",
			status: http.StatusBadRequest, body: prefixedWrite,
		},
		"a prefixed remember on the alias": {
			target: "/api/v1/explain", query: "explain remember@0 'anna sent the invoice'",
			status: http.StatusBadRequest, body: prefixedWrite,
		},
		"a prefixed when on /q": {
			target: "/api/v1/q", query: "explain when invoice",
			status: http.StatusBadRequest, body: prefixedWhen,
		},
		"a prefixed when on the alias": {
			target: "/api/v1/explain", query: "explain when invoice",
			status: http.StatusBadRequest, body: prefixedWhen,
		},
		"a remember on the alias": {
			target: "/api/v1/explain", query: "remember@0 'anna sent the invoice'",
			status: http.StatusBadRequest, body: `{"error":"explain applies to recall queries only"}`,
		},
		"an explained recall of an empty graph on /q": {
			target: "/api/v1/q", query: "explain " + explainedRecall,
			status: http.StatusNoContent,
		},
		"a recall of an empty graph on the alias": {
			target: "/api/v1/explain", query: explainedRecall,
			status: http.StatusNoContent,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)

			w := postQuery(t, s, tc.target, tc.query)

			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tc.status, w.Body.String())
			}
			if diff := cmp.Diff(tc.body, strings.TrimSpace(w.Body.String())); diff != "" {
				t.Errorf("body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestExplainStaysWithItsRequest pins that the explain flag never leaks
// between requests. The plan cache hands one query object to every equal
// request, and the alias raises the flag on a request's stream after
// planning. So a recall answers in the envelope its own statement and route
// ask for, whatever the requests before it asked: explained on the alias or
// with the prefix, plain otherwise, in any order.
func TestExplainStaysWithItsRequest(t *testing.T) {
	s := newTestServer(t)
	seedExplainFacts(t, s)
	requests := []struct{ target, query string }{
		{"/api/v1/q", "explain " + explainedRecall},
		{"/api/v1/q", explainedRecall},
		{"/api/v1/explain", explainedRecall},
		{"/api/v1/q", explainedRecall},
		{"/api/v1/q", "explain " + explainedRecall},
		{"/api/v1/explain", "explain " + explainedRecall},
		{"/api/v1/q", explainedRecall},
	}
	want := []bool{true, false, true, false, true, true, false}

	got := make([]bool, len(requests))
	for i, r := range requests {
		w := postQuery(t, s, r.target, r.query)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %q: status = %d, want %d (body: %s)", r.target, r.query, w.Code, http.StatusOK, w.Body.String())
		}
		recall, _ := explainKeys(t, w.Body.Bytes())
		got[i] = recall != nil
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("explained per request mismatch (-want +got):\n%s", diff)
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
// comes from the level and not from a query that never ran. The deprecated
// explain alias warns once per request, at any level, and the warning names
// the replacement, never the query.
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
				if w := s.do(http.MethodPost, "/api/v1/explain", `{"query":"recall@0 `+private+`"}`); w.Code != http.StatusOK {
					t.Fatalf("alias recall: status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
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
			deprecation := `level=WARN msg="Deprecated explain endpoint called: send explain recall to /api/v1/q instead" removal=0.5.0`
			if n := strings.Count(got, deprecation); n != 1 {
				t.Errorf("log carries the alias deprecation %d times, want once for the one alias request; log:\n%s", n, got)
			}
		})
	}
}
