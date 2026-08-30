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
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/FraiseHQ/fraise/internal/mcpbridge"
)

type mcpCommandConfig struct {
	url   string
	graph uint8
}

func parseMCPCommand(args []string) (mcpCommandConfig, error) {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baseURL := flags.String("url", mcpbridge.DefaultURL, "Fraise daemon URL")
	graph := flags.Int("graph", mcpbridge.DefaultGraph, "Fraise memory graph (0-255)")
	if err := flags.Parse(args); err != nil {
		return mcpCommandConfig{}, err
	}
	if flags.NArg() != 0 {
		return mcpCommandConfig{}, fmt.Errorf("unexpected mcp arguments: %v", flags.Args())
	}
	if *graph < 0 || *graph > 255 {
		return mcpCommandConfig{}, fmt.Errorf("graph must be between 0 and 255, got %d", *graph)
	}
	return mcpCommandConfig{url: *baseURL, graph: uint8(*graph)}, nil
}

func runMCP(args []string) error {
	config, err := parseMCPCommand(args)
	if err != nil {
		return err
	}
	bridge, err := mcpbridge.New(mcpbridge.Config{URL: config.url, Graph: config.graph})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return bridge.Run(ctx)
}
