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

package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bridge builds an MCPServer aimed at a test daemon. Tests construct the
// struct directly rather than going through New: New derives the address
// from config, and what these tests pin is the transport behaviour, not the
// wiring.
func bridge(ts *httptest.Server) *MCPServer {
	return &MCPServer{client: ts.Client(), baseURL: ts.URL}
}

// TestRecallForwardsTheQueryAndRendersTheResponse pins the round trip: the
// tool input marshals into the /api/v1/q request body verbatim, and the
// response body comes back as the structured output with its text rendering
// beside it.
func TestRecallForwardsTheQueryAndRendersTheResponse(t *testing.T) {
	var posted string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		posted = r.URL.Path + " " + string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"count":1,"hits":[{"value":"the barometer falls before the storm","timestamp":"2026-01-02T03:04:05Z","score":1.25}]}}`))
	}))
	defer ts.Close()

	res, out, err := bridge(ts).recall(context.Background(), nil, RecallInput{Query: "recall@2 barometer"})
	if err != nil {
		t.Fatalf("recall = %v, want nil", err)
	}
	if want := `/api/v1/q {"query":"recall@2 barometer"}`; posted != want {
		t.Errorf("daemon received %q, want %q (the input is the request body, verbatim)", posted, want)
	}
	if out.Results.Count != 1 || out.Results.Hits[0].Value != "the barometer falls before the storm" {
		t.Errorf("structured output = %+v, want the response body decoded as sent", out)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if want := "- the barometer falls before the storm (relevance 1.250)"; text != want {
		t.Errorf("rendered text = %q, want %q", text, want)
	}
}

// TestRecallForwardsVectorParameters pins that out-of-band vector bindings
// survive the bridge: parameters marshal beside the query, name and values
// intact, so vec:$name placeholders resolve at the daemon.
func TestRecallForwardsVectorParameters(t *testing.T) {
	var posted string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		posted = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"count":0,"hits":[]}}`))
	}))
	defer ts.Close()

	in := RecallInput{Query: "recall@0 aurora vec:$v", Parameters: map[string][]float64{"v": {0.5, 0.25}}}
	if _, _, err := bridge(ts).recall(context.Background(), nil, in); err != nil {
		t.Fatalf("recall = %v, want nil", err)
	}
	if want := `{"query":"recall@0 aurora vec:$v","parameters":{"v":[0.5,0.25]}}`; posted != want {
		t.Errorf("daemon received %q, want %q", posted, want)
	}
}

// TestDaemonErrorsSurfaceInBand pins the self-correction contract: a non-200
// from the daemon becomes an error carrying the body's own message — the
// text an agent needs to fix its query — and a body that is not the API's
// JSON falls back to the status line rather than surfacing garbage.
func TestDaemonErrorsSurfaceInBand(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"the error field surfaces verbatim", 400, `{"error":"top:0 out of range (1-1000)"}`, "top:0 out of range (1-1000)"},
		{"a non-json body falls back to the status", 502, "<html>bad gateway</html>", "fraise answered 502"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ts.Close()

			_, _, err := bridge(ts).recall(context.Background(), nil, RecallInput{Query: "recall x"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("recall err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestUnreachableDaemonNamesTheAddressAndTheFix pins the error a model
// relays when nothing is listening: it names the address it tried and the
// service commands that start the daemon, because "connection refused" alone
// sends an agent guessing.
func TestUnreachableDaemonNamesTheAddressAndTheFix(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := ts.URL
	ts.Close() // nothing listens here any more

	s := &MCPServer{client: http.DefaultClient, baseURL: url}
	_, _, err := s.recall(context.Background(), nil, RecallInput{Query: "recall x"})
	if err == nil || !strings.Contains(err.Error(), url) || !strings.Contains(err.Error(), "is the daemon running") {
		t.Errorf("recall err = %v, want it to name %s and the fix", err, url)
	}
}

// TestRememberConfirmsTheWrite pins the write path: the input forwards
// verbatim, the empty result decodes, and the model sees the confirmation
// with the server's parse warnings beside it.
func TestRememberConfirmsTheWrite(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"count":0,"hits":[]},"warnings":["parse warning at column 15"]}`))
	}))
	defer ts.Close()

	res, out, err := bridge(ts).remember(context.Background(), nil, RememberInput{Query: "remember 'a fact' topic:x"})
	if err != nil {
		t.Fatalf("remember = %v, want nil", err)
	}
	if out.Results.Count != 0 || len(out.Results.Hits) != 0 {
		t.Errorf("structured output = %+v, want the empty write result", out)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if want := "Remembered.\nwarning: parse warning at column 15"; text != want {
		t.Errorf("rendered text = %q, want %q", text, want)
	}
}
