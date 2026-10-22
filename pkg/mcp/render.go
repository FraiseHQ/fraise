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
	"fmt"
	"strings"
)

// The renderers produce the model-facing text that rides beside the
// structured payload. They use the voice of the Python integrations: one line
// per hit, best first, relevance to three decimals. An empty result says so
// in words, since an empty string reads as a failure to a model. Warnings
// follow, one per line. The query ran, and the model needs them to fix its
// next one.

// renderRecall renders a recall response for the model. A recall can come
// back with nothing in two ways, and the daemon tells them apart by status.
// Either the graph holds nothing at all, or a populated graph has no matching
// fact. graphEmpty marks the first case. The model must tell them apart. The
// first calls for remembering something, the second for asking differently.
// A single "no match" line would send the model rephrasing a query against a
// graph that was never written to.
func renderRecall(out RecallOutput, graphEmpty bool) string {
	var lines []string
	if len(out.Results.Hits) == 0 {
		if graphEmpty {
			lines = append(lines, "No stored facts matched: this graph is empty, nothing has been remembered in it yet.")
		} else {
			lines = append(lines, "No stored facts matched.")
		}
	}
	for _, hit := range out.Results.Hits {
		lines = append(lines, fmt.Sprintf("- %s (relevance %.3f)", hit.Value, hit.Score))
	}
	for _, warning := range out.Warnings {
		lines = append(lines, "warning: "+warning)
	}
	return strings.Join(lines, "\n")
}

// renderRemember confirms a write for the model. The daemon acknowledges a
// write with a status token rather than a payload. So the confirmation is
// fixed text, and only warnings vary.
func renderRemember(out RememberOutput) string {
	lines := []string{"Remembered."}
	for _, warning := range out.Warnings {
		lines = append(lines, "warning: "+warning)
	}
	return strings.Join(lines, "\n")
}
