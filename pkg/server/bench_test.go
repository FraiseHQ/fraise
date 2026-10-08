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

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/pkg/server"
)

// benchConfig is the configuration every server benchmark runs on. It lives
// with the rest of the benchmark harness, so the server measured is set in one
// place for every suite.
const benchConfig = "../../tests/perf/fraise.config.toml"

// recallResponse is the part of a read's answer the benchmarks score: the
// stored values in rank order, which is all a client gets to judge.
type recallResponse struct {
	Results struct {
		Hits []struct {
			Value string `json:"value"`
		} `json:"hits"`
	} `json:"results"`
}

// startServer starts a fresh server on benchConfig and a free port, the way
// the binary does, and returns its base URL once it answers its health check.
// Each benchmark gets its own, so no fact from an earlier run or -count is in
// its graphs. The server is shut down as on SIGTERM when the benchmark ends.
func startServer(b *testing.B) string {
	b.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("reserve a port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		b.Fatalf("release port %d: %v", port, err)
	}

	cfg := config.New()
	if err := cfg.Parse([]string{"-config", benchConfig, "-port", strconv.Itoa(port)}); err != nil {
		b.Fatalf("Parse(%s) = %v, want nil", benchConfig, err)
	}
	switch cfg.DB.Precision {
	case config.PrecisionFloat32:
		serve[float32](b, cfg)
	default:
		serve[float64](b, cfg)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		resp, err := http.Get(url + "/")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return url
			}
		}
	}
	b.Fatalf("server on port %d did not answer its health check within 10s", port)
	return ""
}

// serve runs a server of precision P until the benchmark ends.
func serve[P float32 | float64](b *testing.B, cfg *config.ConfigSet) {
	b.Helper()
	s, err := server.New[uint64, P](cfg, hash.NewHasher[uint64](cfg))
	if err != nil {
		b.Fatalf("New = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()
	b.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			b.Errorf("Start = %v, want nil after shutdown", err)
		}
	})
}

// requestBody encodes one FQL query and the vector bound to its $v
// placeholder, in the request shape the query endpoint decodes.
func requestBody(b *testing.B, fql string, vec []float64) []byte {
	b.Helper()
	req := server.HandleQueryRequest[float64]{Query: fql}
	if vec != nil {
		req.Parameters = map[string][]float64{"v": vec}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		b.Fatalf("encode %q: %v", fql, err)
	}
	return raw
}

// query sends one FQL query to the server at url and returns the values of
// the hits, in rank order. A write and a read of an empty graph both answer
// with no hits; any status other than 200 and 204 fails the benchmark, since
// a number measured over rejected queries describes the error path.
func query(b *testing.B, url, fql string, vec []float64) []string {
	b.Helper()
	resp, err := http.Post(url+"/api/v1/q", "application/json", bytes.NewReader(requestBody(b, fql, vec)))
	if err != nil {
		b.Fatalf("POST %q: %v", fql, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusOK:
	default:
		var msg bytes.Buffer
		_, _ = msg.ReadFrom(resp.Body)
		b.Fatalf("POST %q = %d %s", fql, resp.StatusCode, msg.String())
	}

	var out recallResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		b.Fatalf("decode answer to %q: %v", fql, err)
	}
	values := make([]string, len(out.Results.Hits))
	for i, h := range out.Results.Hits {
		values[i] = h.Value
	}
	return values
}
