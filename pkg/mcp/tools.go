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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// call posts in as the /api/v1/q request body and decodes the response into
// Out. Every failure comes back as an error for the typed handler to return,
// which the SDK packs into an in-band tool error — the model reads the
// daemon's own message ("top:0 out of range (1-1000)") and can correct its
// query, exactly the self-correction contract the HTTP API's error bodies
// exist for. Nothing here is a protocol error: even an unreachable daemon is
// something the model should relay, not something that should kill the call.
func call[Out any](ctx context.Context, s *MCPServer, in any) (Out, error) {
	var out Out

	body, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/v1/q", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return out, fmt.Errorf("could not reach fraise at %s — is the daemon running? (brew services start fraise, or systemctl --user start fraise): %w", s.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// The error body's "error" field is the message an agent can act on;
		// fall back to the status line when the body is not the API's JSON
		// (a proxy page, a truncated response).
		var e struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&e); err != nil || e.Error == "" {
			return out, fmt.Errorf("fraise answered %s", resp.Status)
		}
		return out, errors.New(e.Error)
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("undecodable response from fraise at %s: %w", s.baseURL, err)
	}
	return out, nil
}

// recall forwards the tool input as a query and returns the response twice
// over: as structured content for programmatic clients, and rendered to text
// for the model.
func (s *MCPServer) recall(ctx context.Context, _ *mcp.CallToolRequest, in RecallInput) (*mcp.CallToolResult, RecallOutput, error) {
	out, err := call[RecallOutput](ctx, s, in)
	if err != nil {
		return nil, RecallOutput{}, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: renderRecall(out)}},
	}, out, nil
}

// remember forwards the tool input as a query and confirms the write, with
// any parse warnings the server attached rendered beside the confirmation.
func (s *MCPServer) remember(ctx context.Context, _ *mcp.CallToolRequest, in RememberInput) (*mcp.CallToolResult, RememberOutput, error) {
	out, err := call[RememberOutput](ctx, s, in)
	if err != nil {
		return nil, RememberOutput{}, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: renderRemember(out)}},
	}, out, nil
}
