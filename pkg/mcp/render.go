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
// structured payload, in the voice the Python integrations established: one
// line per hit, best first, relevance to three decimals. An empty result
// says so in words — an empty string reads as a failure to a model — and
// warnings follow one per line, because the query ran and what the parser
// flagged is exactly what the model needs to see to fix its next one.

// renderRecall renders a recall response for the model. graphEmpty separates
// the two ways a recall comes back with nothing, which the daemon distinguishes
// by status: a graph that holds nothing at all, and a populated graph none of
// whose facts matched. The model needs them apart — the first is answered by
// remembering something, the second by asking differently — and a single "no
// match" line sends it re-phrasing a query against a graph that was never
// written to.
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
// write with a status token rather than a payload, so the confirmation is
// fixed text; only warnings vary.
func renderRemember(out RememberOutput) string {
	lines := []string{"Remembered."}
	for _, warning := range out.Warnings {
		lines = append(lines, "warning: "+warning)
	}
	return strings.Join(lines, "\n")
}
