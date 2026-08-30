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

package mcpbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestQueryBuildersMatchTheSDKContract(t *testing.T) {
	tests := []struct {
		name  string
		build func() (string, error)
		want  string
	}{
		{
			name: "remember",
			build: func() (string, error) {
				return buildRemember(3, "it's orange", []string{"color"}, []string{"anne"})
			},
			want: "remember@3 'it''s orange' topic:color entity:anne",
		},
		{
			name: "recall",
			build: func() (string, error) {
				return buildRecall(2, []string{"ferry", "top"}, 5, 0)
			},
			want: "recall@2 ferry 'top' top:5 depth:0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.build()
			if err != nil {
				t.Fatalf("build query: %v", err)
			}
			if got != test.want {
				t.Errorf("query = %q, want %q", got, test.want)
			}
		})
	}
}

func TestQueryBuildersRejectInvalidArguments(t *testing.T) {
	if _, err := buildRemember(0, "", nil, nil); err == nil {
		t.Error("empty fact accepted")
	}
	if _, err := buildRecall(0, nil, 5, 0); err == nil {
		t.Error("empty keywords accepted")
	}
	if _, err := buildRecall(0, []string{"two words"}, 5, 0); err == nil {
		t.Error("whitespace keyword accepted")
	}
	if _, err := buildRecall(0, []string{"two\u00a0words"}, 5, 0); err == nil {
		t.Error("unicode-whitespace keyword accepted")
	}
	if _, err := buildRecall(0, []string{"x"}, 5, 3); err == nil {
		t.Error("unsupported depth accepted")
	}
}

func TestCheckNamesBothServiceRecoveryCommands(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer daemon.Close()

	bridge, err := New(Config{URL: daemon.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = bridge.Check(context.Background())
	if err == nil {
		t.Fatal("Check = nil, want daemon startup error")
	}
	for _, command := range []string{"brew services start fraise", "systemctl --user start fraise"} {
		if !strings.Contains(err.Error(), command) {
			t.Errorf("error %q does not name %q", err, command)
		}
	}
}

func TestMCPToolsRoundTripThroughTheDaemon(t *testing.T) {
	var mutex sync.Mutex
	var queries []string
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","version":"0.1.0"}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/q" {
			http.NotFound(w, r)
			return
		}
		var request queryRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mutex.Lock()
		queries = append(queries, request.Query)
		mutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(request.Query, "recall") {
			_, _ = w.Write([]byte(`{"results":{"count":1,"hits":[{"value":"anne likes orange","score":0.875}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":{"count":1,"hits":[]}}`))
	}))
	defer daemon.Close()

	bridge, err := New(Config{URL: daemon.URL, Graph: 4})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := bridge.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- bridge.Server().Run(ctx, serverTransport) }()

	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil,
	)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		cancel()
		<-serverDone
	})

	var names []string
	tools := make(map[string]*sdkmcp.Tool)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		names = append(names, tool.Name)
		tools[tool.Name] = tool
	}
	sort.Strings(names)
	if got, want := strings.Join(names, ","), "recall_memory,remember_fact"; got != want {
		t.Fatalf("tools = %q, want %q", got, want)
	}
	assertRequiredFields(t, tools["recall_memory"], "keywords")
	assertRequiredFields(t, tools["remember_fact"], "fact")
	if !tools["recall_memory"].Annotations.ReadOnlyHint {
		t.Error("recall_memory is not marked read-only")
	}
	if tools["remember_fact"].Annotations.ReadOnlyHint {
		t.Error("remember_fact is marked read-only")
	}

	remembered, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "remember_fact",
		Arguments: map[string]any{
			"fact":     "anne likes orange",
			"topics":   []string{"color"},
			"entities": []string{"anne"},
		},
	})
	if err != nil {
		t.Fatalf("remember_fact: %v", err)
	}
	if got := textOf(t, remembered); got != "Stored: anne likes orange" {
		t.Errorf("remember result = %q", got)
	}

	recalled, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "recall_memory",
		Arguments: map[string]any{"keywords": []string{"anne", "color"}},
	})
	if err != nil {
		t.Fatalf("recall_memory: %v", err)
	}
	if got := textOf(t, recalled); got != "- anne likes orange (relevance 0.875)" {
		t.Errorf("recall result = %q", got)
	}

	mutex.Lock()
	defer mutex.Unlock()
	wantQueries := []string{
		"remember@4 'anne likes orange' topic:color entity:anne",
		"recall@4 anne color top:5 depth:0",
	}
	if strings.Join(queries, "\n") != strings.Join(wantQueries, "\n") {
		t.Errorf("queries = %#v, want %#v", queries, wantQueries)
	}
}

func textOf(t *testing.T, result *sdkmcp.CallToolResult) string {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content length = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}

func assertRequiredFields(t *testing.T, tool *sdkmcp.Tool, want ...string) {
	t.Helper()
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("%s input schema type = %T", tool.Name, tool.InputSchema)
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("%s required schema type = %T", tool.Name, schema["required"])
	}
	got := make([]string, len(required))
	for i, field := range required {
		got[i], ok = field.(string)
		if !ok {
			t.Fatalf("%s required field type = %T", tool.Name, field)
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s required = %v, want %v", tool.Name, got, want)
	}
}
