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

import "testing"

func TestParseMCPCommandDefaults(t *testing.T) {
	config, err := parseMCPCommand(nil)
	if err != nil {
		t.Fatalf("parseMCPCommand: %v", err)
	}
	if config.url != "http://localhost:9876" {
		t.Errorf("url = %q", config.url)
	}
	if config.graph != 0 {
		t.Errorf("graph = %d", config.graph)
	}
}

func TestParseMCPCommandFlags(t *testing.T) {
	config, err := parseMCPCommand([]string{"--url", "http://127.0.0.1:9999", "--graph", "7"})
	if err != nil {
		t.Fatalf("parseMCPCommand: %v", err)
	}
	if config.url != "http://127.0.0.1:9999" || config.graph != 7 {
		t.Errorf("config = %#v", config)
	}
}

func TestParseMCPCommandRejectsInvalidGraph(t *testing.T) {
	for _, graph := range []string{"-1", "256"} {
		t.Run(graph, func(t *testing.T) {
			if _, err := parseMCPCommand([]string{"--graph", graph}); err == nil {
				t.Fatalf("graph %s accepted", graph)
			}
		})
	}
}

func TestParseMCPCommandRejectsTrailingArguments(t *testing.T) {
	if _, err := parseMCPCommand([]string{"extra"}); err == nil {
		t.Fatal("trailing argument accepted")
	}
}
