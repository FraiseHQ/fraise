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

package graph

// RRFScorer fuses a candidate's contributions by reciprocal rank,
// Σ 1/(k+Rank). Rank is the only input on purpose: the sources score on
// incomparable scales (match count, similarity, a fused seed score), and RRF
// sidesteps calibrating them by trusting only the position each source
// assigned. Contribution.Score and Hop go deliberately unused — magnitudes
// carry no rank information, and hop distance already shapes a walk
// contribution's Rank through the walk's nearest-first ordering.
//
// The consensus property this buys: a candidate two sources place mid-list
// outranks one a single source places first (2/(k+3) > 1/(k+1) for k = 60),
// so agreement between text and vector beats either alone.
type RRFScorer[K comparable, P float32 | float64] struct {
	// k dampens the reciprocal so rank differences near the top do not
	// dominate: at k=60 ranks 0 and 1 differ by ~1.6%, not the 50% a bare
	// reciprocal rank would give. It comes from the `rrf-k` config setting
	// (config.DefaultRRFK documents why 60 is the default).
	k int
}

// NewRRFScorer returns an RRFScorer fusing contributions as Σ 1/(k+Rank).
func NewRRFScorer[K comparable, P float32 | float64](k int) *RRFScorer[K, P] {
	return &RRFScorer[K, P]{k: k}
}

// Score sums 1/(k+Rank) over the contributions, in list order.
func (s *RRFScorer[K, P]) Score(contributions []Contribution[P]) P {
	var total P
	for _, c := range contributions {
		total += P(1) / (P(s.k) + P(c.Rank))
	}
	return total
}
