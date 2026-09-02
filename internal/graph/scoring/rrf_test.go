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

// TestRRFConsensusBeatsASingleFavourite is the fusion property the scorer
// exists for: a candidate two sources place at rank 3 outranks one a single
// source places at rank 1 (2/63 > 1/61). k = 60 is what buys this — a bare
// reciprocal rank would hand the single rank-1 sighting the win.
func TestRRFConsensusBeatsASingleFavourite(t *testing.T) {
	scorer := scoring.NewRRFScorer[uint64, float64](60)

	twoAtThree := scorer.Score([]scoring.Contribution[uint64, float64]{
		{Src: scoring.SrcText, Rank: 3},
		{Src: scoring.SrcVector, Rank: 3},
	})
	oneAtOne := scorer.Score([]scoring.Contribution[uint64, float64]{
		{Src: scoring.SrcText, Rank: 1},
	})

	if twoAtThree <= oneAtOne {
		t.Errorf("two sources at rank 3 = %v, one source at rank 1 = %v; want consensus to win", twoAtThree, oneAtOne)
	}
}

// rrfScoresExactly pins the RRF formula Σ 1/(k+Rank) at precision P. The
// expected values are computed with the same P-typed expression the scorer
// uses, so the pins hold bit-for-bit at both precisions.
func rrfScoresExactly[P float32 | float64](t *testing.T) {
	t.Helper()

	cases := []struct {
		name          string
		k             int
		contributions []scoring.Contribution[uint64, P]
		want          P
	}{
		{
			"a rank-0 sighting is 1/k",
			60,
			[]scoring.Contribution[uint64, P]{{Src: scoring.SrcText, Rank: 0}},
			P(1) / P(60),
		},
		{
			// Score 999 and Hop 200 change nothing: rank is the only input.
			// Magnitudes carry no rank information, and hop already shaped
			// the walk rank; weighing either would re-introduce the scale
			// calibration RRF exists to avoid.
			"score and hop are deliberately ignored",
			60,
			[]scoring.Contribution[uint64, P]{{Src: scoring.SrcGraph, Score: 999, Rank: 0}},
			P(1) / P(60),
		},
		{
			"sightings sum across sources",
			60,
			[]scoring.Contribution[uint64, P]{
				{Src: scoring.SrcText, Rank: 0},
				{Src: scoring.SrcVector, Rank: 1},
				{Src: scoring.SrcGraph, Rank: 2},
			},
			P(1)/P(60) + P(1)/P(61) + P(1)/P(62),
		},
		{
			// An anchor sighting has no list, so it ranks 0 like any
			// seed: the same 1/k, whatever mass it carries.
			"an anchor sighting is a rank-0 sighting",
			60,
			[]scoring.Contribution[uint64, P]{{Src: scoring.SrcAnchor, Score: 1, Rank: 0}},
			P(1) / P(60),
		},
		{
			"k is the scorer's parameter",
			1,
			[]scoring.Contribution[uint64, P]{{Src: scoring.SrcText, Rank: 1}},
			P(1) / P(2),
		},
		{
			"no sightings is score zero",
			60,
			nil,
			0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scorer := scoring.NewRRFScorer[uint64, P](tc.k)
			if got := scorer.Score(tc.contributions); got != tc.want {
				t.Errorf("Score(%+v) = %v, want %v", tc.contributions, got, tc.want)
			}
		})
	}
}

func TestRRFScorer_float64(t *testing.T) { rrfScoresExactly[float64](t) }
func TestRRFScorer_float32(t *testing.T) { rrfScoresExactly[float32](t) }
