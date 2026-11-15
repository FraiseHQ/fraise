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
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FraiseHQ/fraise/internal/config"
)

// parsed builds a config the way `fraise mcp` does, through Parse with no
// config file. The bridge then sees the address Parse derives, not a zero
// value.
func parsed(t *testing.T, args ...string) *config.ConfigSet {
	t.Helper()
	c := config.New()
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")
	if err := c.Parse(append([]string{"-config", missing}, args...)); errors.Is(err, config.ErrInvalidValue) || errors.Is(err, config.ErrInvalidFlag) {
		t.Fatalf("Parse(%v) = %v, want the flags accepted", args, err)
	}
	return c
}

// TestNewWiresTheBridgeFromConfig pins New's wiring. With no -addr, the daemon
// address derives from the config's port, so `fraise mcp -config x` finds
// whatever `fraise -config x` serves. Construction is also the schema smoke
// test. The SDK's AddTool panics on a tool schema that fails to resolve. So a
// schema edit that breaks registration fails here, at unit speed, rather than
// at the first live handshake.
func TestNewWiresTheBridgeFromConfig(t *testing.T) {
	s := New(parsed(t, "-port", "4242"))

	if want := "http://127.0.0.1:4242"; s.baseURL != want {
		t.Errorf("baseURL = %q, want %q (derived from the config's port)", s.baseURL, want)
	}
	if s.client == nil || s.client.Timeout != defaultTimeout {
		t.Errorf("client timeout = %v, want %v", s.client.Timeout, defaultTimeout)
	}
	if s.Server == nil {
		t.Error("Server = nil, want the MCP server with both tools registered")
	}
}

// TestNewForwardsToAddr pins -addr. The bridge forwards to the daemon it
// names, remote or on another port, rather than to the local one the config
// describes. A trailing slash is dropped, since the query path is appended to
// it and a doubled slash is a 404.
func TestNewForwardsToAddr(t *testing.T) {
	s := New(parsed(t, "-addr", "http://10.0.0.5:9876/"))

	if want := "http://10.0.0.5:9876"; s.baseURL != want {
		t.Errorf("baseURL = %q, want %q", s.baseURL, want)
	}
}

// TestRecallToolTeachesDescribe pins that the recall tool tells a model about
// describe recall in both places a model reads it: the tool's description and
// its query's. Each says that this tool runs a describe, so a model looking for
// one reaches for the tool it has rather than one it lacks. A model writes only
// the statements its tools show it, and a describe is the one to run before it
// knows how memory is filed. The tools are listed through an MCP client, so
// what is pinned is what a model is sent, not a string in this package.
func TestRecallToolTeachesDescribe(t *testing.T) {
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := New(parsed(t)).Server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connecting the bridge = %v, want nil", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connecting a client = %v, want nil", err)
	}
	defer func() { _ = session.Close() }()

	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("listing the tools = %v, want nil", err)
	}
	var recall *mcp.Tool
	for _, tool := range listed.Tools {
		if tool.Name == "recall" {
			recall = tool
		}
	}
	if recall == nil {
		t.Fatalf("tools = %+v, want recall among them", listed.Tools)
	}
	schema, err := json.Marshal(recall.InputSchema)
	if err != nil {
		t.Fatalf("encoding the input schema = %v, want nil", err)
	}
	var input struct {
		Properties struct {
			Query struct {
				Description string `json:"description"`
			} `json:"query"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &input); err != nil {
		t.Fatalf("decoding the input schema %s = %v, want nil", schema, err)
	}

	descriptions := map[string]struct {
		description string
		runs        string
	}{
		"tool":  {recall.Description, "This tool also runs describe"},
		"query": {input.Properties.Query.Description, "which this tool runs as it runs a recall"},
	}
	for name, tc := range descriptions {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{"describe recall", tc.runs} {
				if !strings.Contains(tc.description, want) {
					t.Errorf("%s description = %q, want it to contain %q", name, tc.description, want)
				}
			}
		})
	}
}
