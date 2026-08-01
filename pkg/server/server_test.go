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

// newTestServer builds a fully wired server with a started database and engine,
// ready to serve requests through its router. It registers cleanup so the
// engine's workers are stopped when the test ends.
func newTestServer(t *testing.T) *Server[uint64, float32] {
	t.Helper()
	cfg := config.New()
	s, err := New[uint64, float32](cfg, hash.NewHasher[uint64](cfg))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := s.DB.Start(); err != nil {
		t.Fatalf("db.Start returned error: %v", err)
	}
	s.Engine.Start()
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
