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

	"github.com/google/go-cmp/cmp"

	"github.com/FraiseHQ/fraise/internal/graph/scoring"
)

// TestSourceStringNamesEveryStage pins the names the explain payload
// serialises. A client keys its breakdown on these strings and never sees the
// Go constants. So a renamed or unnamed source is a wire change. A stage added
// without a name would reach the client as "unknown".
func TestSourceStringNamesEveryStage(t *testing.T) {
	cases := map[string]scoring.Source{
		"text":    scoring.SrcText,
		"vector":  scoring.SrcVector,
		"graph":   scoring.SrcGraph,
		"anchor":  scoring.SrcAnchor,
		"unknown": scoring.Source(255),
	}
	for want, src := range cases {
		t.Run(want, func(t *testing.T) {
			if got := src.String(); got != want {
				t.Errorf("Source(%d).String() = %q, want %q", src, got, want)
			}
		})
	}
}

// TestCandidatesSightKeepsEachCandidatesSightings pins the shared array a
// reading's sightings live in. a and b are sighted side by side by the text
// reading, then a again by the vector reading. Appending a's vector sighting
// must copy a's list out of the array. Writing in place would overwrite b's
// text sighting with it.
func TestCandidatesSightKeepsEachCandidatesSightings(t *testing.T) {
	text := func(rank int) scoring.Contribution[string, float64] {
		return scoring.Contribution[string, float64]{Src: scoring.SrcText, Score: float64(10 - rank), Rank: uint16(rank), Count: 1}
	}
	vector := func(rank int) scoring.Contribution[string, float64] {
		return scoring.Contribution[string, float64]{Src: scoring.SrcVector, Score: 0.5, Rank: uint16(rank), Count: 1}
	}
	got := make(scoring.Candidates[string, float64])
	got.Sight([]string{"a", "b"}, text)
	got.Sight([]string{"a"}, vector)

	want := scoring.Candidates[string, float64]{
		"a": {text(0), vector(0)},
		"b": {text(1)},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Candidates after two readings mismatch (-want +got):\n%s", diff)
	}
}
