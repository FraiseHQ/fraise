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

// DefaultRRFK is the dampening constant the configuration wires when the RRF
// scorer is selected. 60 is the empirical standard from Cormack, Clarke &
// Büttcher (SIGIR 2009), where it beat every individual ranker and Condorcet
// fusion across TREC collections. It is a constant, not configuration: RRF
// exists for comparison runs against the shipped excess methodology, and a
// comparison baseline you can tune is not a baseline. NewRRFScorer still
// takes k, because k is a genuine parameter of the algorithm family — the
// contract tests exercise it — but the wiring always passes this value.
const DefaultRRFK = 60

// RRFScorer fuses a candidate's contributions by reciprocal rank,
// Σ 1/(k+Rank). Rank is the only input on purpose: the sources score on
// incomparable scales, and RRF sidesteps calibrating them by trusting only
// the position each source assigned. Contribution.Score goes deliberately
// unused — magnitudes carry no rank information — and so does the query's
// background rate: rank fusion has no null model, which is precisely the
// property that let mega-hubs manufacture consensus from size alone
// (RRF_FINDINGS Rounds 1–8). It remains available as an alternative fold for
// comparison runs; the shipped default is the ExcessScorer.
//
// The consensus property this buys: a candidate two sources place mid-list
// outranks one a single source places first (2/(k+3) > 1/(k+1) for k = 60),
// so agreement between text and vector beats either alone.
type RRFScorer[K comparable, P float32 | float64] struct {
	// k dampens the reciprocal so rank differences near the top do not
	// dominate: at k=60 ranks 0 and 1 differ by ~1.6%, not the 50% a bare
	// reciprocal rank would give. The wiring always passes DefaultRRFK.
	k int
}

// NewRRFScorer returns an RRFScorer fusing contributions as Σ 1/(k+Rank).
func NewRRFScorer[K comparable, P float32 | float64](k int) *RRFScorer[K, P] {
	return &RRFScorer[K, P]{k: k}
}

// WithBackground returns the scorer itself: rank fusion carries no null
// model (see the type comment), so there is nothing to bind.
func (s *RRFScorer[K, P]) WithBackground(P) Scorer[K, P] {
	return s
}

// Score sums 1/(k+Rank) over the contributions, in list order.
func (s *RRFScorer[K, P]) Score(contributions []Contribution[K, P]) P {
	var total P
	for _, c := range contributions {
		total += P(1) / (P(s.k) + P(c.Rank))
	}
	return total
}
