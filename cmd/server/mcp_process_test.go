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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPCommandSpeaksStdio(t *testing.T) {
	queries := make(chan string, 1)
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
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		queries <- request.Query
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"count":0,"hits":[]}}`))
	}))
	defer daemon.Close()

	command := exec.Command(
		os.Args[0], "-test.run=TestMCPHelperProcess", "--",
		"--url", daemon.URL, "--graph", "9",
	)
	command.Env = append(os.Environ(), "GO_WANT_MCP_HELPER_PROCESS=1")
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "stdio-test", Version: "1.0.0"}, nil,
	)
	ctx := context.Background()
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect to fraise mcp subprocess: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "recall_memory",
		Arguments: map[string]any{"keywords": []string{"parrot"}},
	})
	if err != nil {
		t.Fatalf("call recall_memory: %v", err)
	}
	if result.IsError {
		t.Fatalf("recall_memory returned tool error: %#v", result.Content)
	}
	if got := <-queries; got != "recall@9 parrot top:5 depth:0" {
		t.Errorf("daemon query = %q", got)
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER_PROCESS") != "1" {
		return
	}
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		fmt.Fprintln(os.Stderr, "missing helper argument separator")
		os.Exit(2)
	}
	if err := runMCP(os.Args[separator+1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestMCPCommandReportsDaemonRecovery(t *testing.T) {
	command := exec.Command(
		os.Args[0], "-test.run=TestMCPHelperProcess", "--",
		"--url", "http://127.0.0.1:1",
	)
	command.Env = append(os.Environ(), "GO_WANT_MCP_HELPER_PROCESS=1")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("unreachable daemon command exited successfully")
	}
	message := string(output)
	for _, recovery := range []string{"brew services start fraise", "systemctl --user start fraise"} {
		if !strings.Contains(message, recovery) {
			t.Errorf("output %q does not name %q", message, recovery)
		}
	}
}
