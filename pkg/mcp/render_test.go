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

import "testing"

// TestRenderRecall pins the model-facing text exactly: one line per hit in
// response order, relevance to three decimals, an empty result said in words
// (an empty string reads as failure to a model), warnings one per line after
// the hits. The wording matches the Python integrations' voice on purpose —
// an agent switching transports should not meet a second dialect.
func TestRenderRecall(t *testing.T) {
	cases := []struct {
		name string
		out  RecallOutput
		want string
	}{
		{
			name: "hits render one line each, best first",
			out: RecallOutput{Results: Result{Count: 2, Hits: []Hit{
				{Value: "the barometer falls before the storm", Score: 1.25},
				{Value: "storm clouds gather at sea", Score: 0.412},
			}}},
			want: "- the barometer falls before the storm (relevance 1.250)\n" +
				"- storm clouds gather at sea (relevance 0.412)",
		},
		{
			name: "an empty result says so in words",
			out:  RecallOutput{Results: Result{Count: 0, Hits: []Hit{}}},
			want: "No stored facts matched.",
		},
		{
			name: "warnings follow the hits, one per line",
			out: RecallOutput{
				Results:  Result{Count: 1, Hits: []Hit{{Value: "since the storm", Score: 1}}},
				Warnings: []string{`term "since" is also a keyword`},
			},
			want: "- since the storm (relevance 1.000)\n" +
				`warning: term "since" is also a keyword`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderRecall(tc.out); got != tc.want {
				t.Errorf("renderRecall = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRenderRemember pins the write confirmation: fixed text, since a write's
// result carries no payload, with warnings appended when the parse had
// something to say.
func TestRenderRemember(t *testing.T) {
	cases := []struct {
		name string
		out  RememberOutput
		want string
	}{
		{
			name: "a clean write is a bare confirmation",
			out:  RememberOutput{Results: Result{Count: 0, Hits: []Hit{}}},
			want: "Remembered.",
		},
		{
			name: "warnings ride under the confirmation",
			out: RememberOutput{
				Results:  Result{Count: 0, Hits: []Hit{}},
				Warnings: []string{"parse warning at column 15"},
			},
			want: "Remembered.\nwarning: parse warning at column 15",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderRemember(tc.out); got != tc.want {
				t.Errorf("renderRemember = %q, want %q", got, tc.want)
			}
		})
	}
}
