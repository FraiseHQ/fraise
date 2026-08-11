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

package graph_test

import (
	"testing"

	"github.com/RonsenbergVI/fraise/internal/graph"
)

// compatibilityScoresExactly pins the CompatibilityScorer formula per source at
// precision P. Every expected value is exactly representable in float32, so the
// pins hold bit-for-bit at both precisions — ranks are chosen to keep the
// reciprocals dyadic (no rank 2). These numbers are the "nothing moves"
// contract of the collection-layer refactor: if one shifts, recall ranking
// shifts with it.
func compatibilityScoresExactly[P float32 | float64](t *testing.T) {
	t.Helper()

	cases := []struct {
		name          string
		attenuation   float64
		contributions []graph.Contribution[P]
		want          P
	}{
		{
			// The match count (Score 3) is deliberately ignored: the
			// pre-contribution pipeline ranked text seeds by position only.
			"text scores by reciprocal rank, not match count",
			0.5,
			[]graph.Contribution[P]{{Src: graph.SrcText, Score: 3, Rank: 0}},
			1,
		},
		{
			"text rank discounts",
			0.5,
			[]graph.Contribution[P]{{Src: graph.SrcText, Score: 1, Rank: 3}},
			0.25,
		},
		{
			// The one deliberate change from the old aggregation: similarity
			// scales the reciprocal rank instead of being discarded.
			"vector similarity weighs the seed",
			0.5,
			[]graph.Contribution[P]{{Src: graph.SrcVector, Score: 0.5, Rank: 0}},
			0.5,
		},
		{
			"vector rank discounts on top of similarity",
			0.5,
			[]graph.Contribution[P]{{Src: graph.SrcVector, Score: 0.5, Rank: 1}},
			0.25,
		},
		{
			// Rank 5 changes nothing for a graph contribution: the old
			// pipeline attenuated by hop alone, and this scorer preserves it.
			"graph attenuates the seed score per hop, ignoring rank",
			0.5,
			[]graph.Contribution[P]{{Src: graph.SrcGraph, Score: 1, Rank: 5, Hop: 2}},
			0.25,
		},
		{
			"graph hop zero is unattenuated",
			0.5,
			[]graph.Contribution[P]{{Src: graph.SrcGraph, Score: 2, Hop: 0}},
			2,
		},
		{
			"attenuation is the scorer's parameter",
			1.0,
			[]graph.Contribution[P]{{Src: graph.SrcGraph, Score: 2, Hop: 3}},
			2,
		},
		{
			"contributions sum in list order",
			0.5,
			[]graph.Contribution[P]{
				{Src: graph.SrcText, Score: 1, Rank: 0},
				{Src: graph.SrcVector, Score: 0.5, Rank: 0},
				{Src: graph.SrcGraph, Score: 2, Hop: 1},
			},
			2.5,
		},
		{
			"no contributions is score zero",
			0.5,
			nil,
			0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scorer := graph.NewCompatibilityScorer[uint64, P](tc.attenuation)
			if got := scorer.Score(tc.contributions); got != tc.want {
				t.Errorf("Score(%+v) = %v, want %v", tc.contributions, got, tc.want)
			}
		})
	}
}

func TestCompatibilityScorer_float64(t *testing.T) { compatibilityScoresExactly[float64](t) }
func TestCompatibilityScorer_float32(t *testing.T) { compatibilityScoresExactly[float32](t) }
