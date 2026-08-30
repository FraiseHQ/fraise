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

// Package mcpbridge exposes a running Fraise daemon as a stateless MCP server.
package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/FraiseHQ/fraise/pkg/version"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// DefaultURL is the local Fraise daemon endpoint used by the CLI bridge.
	DefaultURL = "http://localhost:9876"
	// DefaultGraph is the memory graph selected when the user supplies no flag.
	DefaultGraph = 0

	defaultTop   = 5
	defaultDepth = 0
	maxBodyBytes = 1 << 20
)

var reservedWords = map[string]struct{}{
	"recall": {}, "remember": {}, "forget": {}, "update": {},
	"topic": {}, "entity": {}, "since": {}, "until": {},
	"top": {}, "depth": {}, "vec": {},
}

// Config binds one MCP server process to one Fraise daemon and memory graph.
type Config struct {
	URL        string
	Graph      uint8
	HTTPClient *http.Client
}

// Bridge forwards MCP memory tools to a Fraise daemon without holding state.
type Bridge struct {
	baseURL string
	graph   uint8
	client  *http.Client
}

type rememberInput struct {
	Fact     string   `json:"fact" jsonschema:"a single, self-contained statement to remember"`
	Topics   []string `json:"topics,omitempty" jsonschema:"optional subject tags grouping related facts"`
	Entities []string `json:"entities,omitempty" jsonschema:"optional named things the fact is about"`
}

type recallInput struct {
	Keywords []string `json:"keywords" jsonschema:"salient names, topics, or nouns to search for"`
	Top      *int     `json:"top,omitempty" jsonschema:"maximum number of facts to return, most relevant first"`
	Depth    *int     `json:"depth,omitempty" jsonschema:"retrieval lane: 0 is text only, 1 is precision, 2 is maximum recall"`
}

type queryRequest struct {
	Query string `json:"query"`
}

type queryResponse struct {
	Results struct {
		Count int `json:"count"`
		Hits  []struct {
			Value string  `json:"value"`
			Score float64 `json:"score"`
		} `json:"hits"`
	} `json:"results"`
	Warnings []string `json:"warnings,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// New validates the daemon endpoint and builds a stateless bridge.
func New(config Config) (*Bridge, error) {
	rawURL := strings.TrimSpace(config.URL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid fraise URL %q", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid fraise URL scheme %q: expected http or https", parsed.Scheme)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid fraise URL %q: query strings and fragments are not supported", rawURL)
	}

	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Bridge{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		graph:   config.Graph,
		client:  client,
	}, nil
}

// Check verifies the daemon is reachable before MCP starts reading stdin.
func (b *Bridge) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+"/", nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return b.startupError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return b.startupError(fmt.Errorf("health check returned %s", resp.Status))
	}
	return nil
}

// Server returns the MCP server used by the stdio command and in-memory tests.
func (b *Bridge) Server() *sdkmcp.Server {
	server := sdkmcp.NewServer(
		&sdkmcp.Implementation{
			Name:        "fraise",
			Title:       "Fraise memory",
			Description: "Long-term memory backed by a local Fraise daemon.",
			Version:     version.FullVersion(),
			WebsiteURL:  "https://github.com/FraiseHQ/fraise",
		},
		&sdkmcp.ServerOptions{Capabilities: &sdkmcp.ServerCapabilities{}},
	)

	closedWorld := false
	nondestructive := false
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "recall_memory",
		Description: "Search the assistant's long-term memory for facts related to some " +
			"keywords and return them ranked by relevance. Call this before answering " +
			"when the user refers to something they told you earlier.",
		Annotations: &sdkmcp.ToolAnnotations{
			Title:         "Recall memory",
			ReadOnlyHint:  true,
			OpenWorldHint: &closedWorld,
		},
	}, b.recall)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "remember_fact",
		Description: "Store a single self-contained fact in the assistant's long-term memory " +
			"for later recall. Use it when the user shares something durable worth " +
			"remembering - a preference, a name, or a decision.",
		Annotations: &sdkmcp.ToolAnnotations{
			Title:           "Remember fact",
			DestructiveHint: &nondestructive,
			OpenWorldHint:   &closedWorld,
		},
	}, b.remember)

	return server
}

// Run checks the daemon and serves MCP over stdin/stdout until the client exits.
func (b *Bridge) Run(ctx context.Context) error {
	if err := b.Check(ctx); err != nil {
		return err
	}
	return b.Server().Run(ctx, &sdkmcp.StdioTransport{})
}

func (b *Bridge) recall(
	ctx context.Context,
	_ *sdkmcp.CallToolRequest,
	input recallInput,
) (*sdkmcp.CallToolResult, any, error) {
	top := defaultTop
	if input.Top != nil {
		top = *input.Top
	}
	depth := defaultDepth
	if input.Depth != nil {
		depth = *input.Depth
	}
	q, err := buildRecall(b.graph, input.Keywords, top, depth)
	if err != nil {
		return nil, nil, err
	}
	response, err := b.query(ctx, q)
	if err != nil {
		return nil, nil, fmt.Errorf("memory lookup failed: %w", err)
	}
	if len(response.Results.Hits) == 0 {
		return textResult("No stored facts matched those keywords."), nil, nil
	}

	lines := make([]string, len(response.Results.Hits))
	for i, hit := range response.Results.Hits {
		lines[i] = fmt.Sprintf("- %s (relevance %.3f)", hit.Value, hit.Score)
	}
	return textResult(strings.Join(lines, "\n")), nil, nil
}

func (b *Bridge) remember(
	ctx context.Context,
	_ *sdkmcp.CallToolRequest,
	input rememberInput,
) (*sdkmcp.CallToolResult, any, error) {
	q, err := buildRemember(b.graph, input.Fact, input.Topics, input.Entities)
	if err != nil {
		return nil, nil, err
	}
	if _, err := b.query(ctx, q); err != nil {
		return nil, nil, fmt.Errorf("could not store the fact: %w", err)
	}
	return textResult("Stored: " + input.Fact), nil, nil
}

func (b *Bridge) query(ctx context.Context, q string) (*queryResponse, error) {
	payload, err := json.Marshal(queryRequest{Query: q})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, b.baseURL+"/api/v1/q", bytes.NewReader(payload),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach fraise at %s: %w", b.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var failure errorResponse
		_ = json.Unmarshal(body, &failure)
		message := strings.TrimSpace(failure.Error)
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("fraise query failed: %s", message)
	}

	var result queryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode fraise response: %w", err)
	}
	return &result, nil
}

func (b *Bridge) startupError(cause error) error {
	return fmt.Errorf(
		"could not reach fraise daemon at %s: %w; start it with "+
			"`brew services start fraise` or `systemctl --user start fraise`",
		b.baseURL, cause,
	)
}

func textResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}}}
}

func buildRemember(graph uint8, fact string, topics, entities []string) (string, error) {
	quoted, err := quoteValue(fact)
	if err != nil {
		return "", err
	}
	parts := []string{"remember@" + strconv.Itoa(int(graph)), quoted}
	for _, topic := range topics {
		value, err := token("topic", topic)
		if err != nil {
			return "", err
		}
		parts = append(parts, "topic:"+value)
	}
	for _, entity := range entities {
		value, err := token("entity", entity)
		if err != nil {
			return "", err
		}
		parts = append(parts, "entity:"+value)
	}
	return strings.Join(parts, " "), nil
}

func buildRecall(graph uint8, keywords []string, top, depth int) (string, error) {
	if len(keywords) == 0 {
		return "", fmt.Errorf("recall needs at least one keyword")
	}
	if top <= 0 {
		return "", fmt.Errorf("top must be positive, got %d", top)
	}
	if depth < 0 || depth > 2 {
		return "", fmt.Errorf("depth must be one of 0, 1, or 2, got %d", depth)
	}

	parts := []string{"recall@" + strconv.Itoa(int(graph))}
	for i, keyword := range keywords {
		value, err := token("keyword", keyword)
		if err != nil {
			return "", err
		}
		if i > 0 {
			if _, reserved := reservedWords[strings.ToLower(value)]; reserved {
				value, err = quoteValue(value)
				if err != nil {
					return "", err
				}
			}
		}
		parts = append(parts, value)
	}
	parts = append(parts, "top:"+strconv.Itoa(top), "depth:"+strconv.Itoa(depth))
	return strings.Join(parts, " "), nil
}

func token(kind, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", kind)
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf("%s must not contain whitespace: %q", kind, value)
	}
	return value, nil
}

func quoteValue(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("a quoted value must not be empty")
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}
