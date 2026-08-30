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
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/pkg/version"
)

// defaultTimeout bounds one query round trip, mirroring the Python SDK's
// default: long enough for a large recall, short enough that a wedged daemon
// turns into a readable tool error instead of a hung agent.
const defaultTimeout = 30 * time.Second

// MCPServer is the stdio MCP bridge to a running fraise daemon. It is a thin
// adapter over the HTTP query API — not a second engine — so it reads the
// same config the daemon does and derives the daemon's address from it:
// `fraise mcp -config x` finds whatever `fraise -config x` serves.
type MCPServer struct {
	Config *config.ConfigSet
	Server *mcp.Server

	client  *http.Client
	baseURL string
}

// New builds the bridge and registers its two tools. Registration goes
// through the SDK's typed AddTool: it unmarshals and validates arguments
// against the input schema before a handler runs, marshals the typed output
// into structured content and validates it against the output schema, and
// turns a handler error into an in-band tool error the model can read.
func New(c *config.ConfigSet) *MCPServer {

	server := mcp.NewServer(&mcp.Implementation{Name: "fraise", Version: version.Version}, nil)

	s := &MCPServer{
		Config:  c,
		Server:  server,
		client:  &http.Client{Timeout: defaultTimeout},
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", c.Server.Port),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         "recall",
		Description:  "Search the agent's long-term memory: run an FQL recall against the fraise daemon and return the matching facts ranked by relevance and recency.",
		InputSchema:  recallInputSchema,
		OutputSchema: recallOutputSchema,
	}, s.recall)

	mcp.AddTool(server, &mcp.Tool{
		Name:         "remember",
		Description:  "Store a self-contained fact in the agent's long-term memory: run an FQL remember against the fraise daemon.",
		InputSchema:  rememberInputSchema,
		OutputSchema: rememberOutputSchema,
	}, s.remember)

	return s
}

// Start serves MCP over stdio until ctx is cancelled. Stdout belongs to the
// protocol from here on — nothing on this path may print to it.
func (s *MCPServer) Start(ctx context.Context) error {
	return s.Server.Run(ctx, &mcp.StdioTransport{})
}
