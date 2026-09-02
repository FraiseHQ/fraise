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

package scoring_test

import (
	"testing"

	"github.com/FraiseHQ/fraise/internal/graph/scoring"
)

// TestSourceStringNamesEveryStage pins the names the explain payload
// serializes: a client keys its breakdown on these strings and never sees the
// Go constants, so a renamed or unnamed source is a wire change, and a stage
// added without a name would reach the client as "unknown".
func TestSourceStringNamesEveryStage(t *testing.T) {
	cases := []struct {
		src  scoring.Source
		want string
	}{
		{scoring.SrcText, "text"},
		{scoring.SrcVector, "vector"},
		{scoring.SrcGraph, "graph"},
		{scoring.SrcAnchor, "anchor"},
		{scoring.Source(255), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.src.String(); got != tc.want {
			t.Errorf("Source(%d).String() = %q, want %q", tc.src, got, tc.want)
		}
	}
}
