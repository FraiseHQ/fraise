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

// This is a white-box test: it lives in package server so it can drive the
// unexported router directly, without binding a real TCP port via Start.
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// newTestServer builds a fully wired server on the default config with a started
// database and engine, ready to serve requests through its router.
func newTestServer(t *testing.T) *Server[uint64, float32] {
	t.Helper()
	return newTestServerCfg(t, config.New())
}

// newTestServerCfg is newTestServer with a caller-supplied config, so a test can
// exercise a specific limit (body size, top/depth ceiling, vector dimension). It
// registers cleanup so the engine's workers are stopped when the test ends.
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

// TestQueryMalformedBody checks that a request with an invalid JSON body is
// rejected with 400.
func TestQueryMalformedBody(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodPost, "/api/v1/q", "{not json")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
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

// TestQueryWrappingGraphSelectorRejected guards the tenant-isolation fix: a
// selector past the uint8 range used to wrap into a valid-looking graph
// (@256 -> 0, @300 -> 44), silently executing against the wrong graph instead
// of being rejected. Each must now return 400 before execution.
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
// match the graph's established dimension is a client error: 400 with the
// expected and supplied dimensions, not an opaque 500. The mismatch is only
// detectable at commit time (the first vector fixes the dimension), so this
// pins the commit error surviving the scheduler intact.
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

// TestQuerySuccess checks that a well-formed recall query is planned, executed,
// and returns 200 with a results payload.
func TestQuerySuccess(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodPost, "/api/v1/q", `{"query":"recall@0 anna"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "results") {
		t.Errorf("body = %q, want a results payload", w.Body.String())
	}
}

// TestStatsEndpoint checks that GET /api/v1/stats returns one snapshot per
// graph and that a committed write with a vector is reflected in the counts —
// including the forest_entries gauge the bloat regression tests key off.
func TestStatsEndpoint(t *testing.T) {
	s := newTestServer(t)

	w := s.do(http.MethodGet, "/api/v1/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	var stats struct {
		Graphs []struct {
			ID            int `json:"id"`
			Nodes         int `json:"nodes"`
			Vectors       int `json:"vectors"`
			ForestEntries int `json:"forest_entries"`
		} `json:"graphs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("stats body is not valid JSON: %v (body: %s)", err, w.Body.String())
	}
	if got, want := len(stats.Graphs), s.DB.NumGraphs(); got != want {
		t.Fatalf("len(graphs) = %d, want %d", got, want)
	}

	// Commit a fact with a vector, then confirm the write shows up in stats.
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
