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
	"errors"
	"path/filepath"
	"testing"

	"github.com/FraiseHQ/fraise/internal/config"
)

// parsed builds a config the way `fraise mcp` does — through Parse, with no
// config file — so the bridge sees the address Parse derives, not a zero value.
func parsed(t *testing.T, args ...string) *config.ConfigSet {
	t.Helper()
	c := config.New()
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")
	if err := c.Parse(append([]string{"-config", missing}, args...)); errors.Is(err, config.ErrInvalidValue) || errors.Is(err, config.ErrInvalidFlag) {
		t.Fatalf("Parse(%v) = %v, want the flags accepted", args, err)
	}
	return c
}

// TestNewWiresTheBridgeFromConfig pins New's wiring: with no -addr the daemon
// address derives from the same config the daemon reads — `fraise mcp -config
// x` must find whatever `fraise -config x` serves — and construction doubles
// as the schema smoke test, because the SDK's AddTool panics on a tool schema
// that fails to compile. A schema edit that breaks registration fails here,
// at unit speed, not at the first live handshake.
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

// TestNewForwardsToAddr pins -addr: the bridge forwards to the daemon it
// names, remote or on another port, rather than to the local one the config
// describes. A trailing slash is dropped, since the query path is appended to
// it and a doubled slash is a 404.
func TestNewForwardsToAddr(t *testing.T) {
	s := New(parsed(t, "-addr", "http://10.0.0.5:9876/"))

	if want := "http://10.0.0.5:9876"; s.baseURL != want {
		t.Errorf("baseURL = %q, want %q", s.baseURL, want)
	}
}
