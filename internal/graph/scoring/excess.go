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

package scoring

// alpha is the per-edge attenuation of transmitted excess. It is an internal
// constant, not configuration: the BM25 floor and hub silence hold for any
// 0 < α < 1. The seed → anchor → fact path has two edges, so the fold applies
// α²; applying α once per path would double the graph channel's weight
// against the text channel.
const alpha = 0.5

// ExcessScorer folds a candidate's observations under the excess-transmission
// methodology. Relevance is the candidate's own seed mass m plus, for each
// graph observation, its anchor's per-edge surplus over the fair share,
// attenuated α² for the two-edge path:
//
//	m + α² · Σ max(0, (M_A − m − d_A·ρ₀) / d_A)
//
// where M_A is the anchor's full observed mass, d_A its degree and ρ₀ the
// background rate. Subtracting m is self-exclusion: a fact never funds its own
// boost. An anchor at or below its fair share contributes nothing (hub
// silence), so a hub is heard when it is surprising and silent when it is
// merely large.
//
// Scores stay in raw seed units: relevance is homogeneous of degree 1 in the
// mass scale, so rescaling every mass leaves the ordering unchanged, and
// normalizing a single channel would only break the channels'
// commensurability (scale invariance).
type ExcessScorer[K comparable, P float32 | float64] struct {
	// background is the query's bound null rate. It is set only by
	// WithBackground returning a fresh value — never mutated in place — so
	// the graph's shared instance stays pure and the zero value is the
	// unbound scorer seed fusion runs.
	background P
}

// NewExcessScorer returns the excess-transmission fold, unbound (background
// zero) until WithBackground binds a query's rate.
func NewExcessScorer[K comparable, P float32 | float64]() *ExcessScorer[K, P] {
	return &ExcessScorer[K, P]{}
}

// WithBackground returns a scorer bound to one query's background rate.
func (s *ExcessScorer[K, P]) WithBackground(background P) Scorer[K, P] {
	return &ExcessScorer[K, P]{background: background}
}

// Score folds contributions at the bound background rate. Seed mass first —
// the text, vector and anchor observations sum directly — then the hinge
// over each graph observation, in list order, so identical inputs fold to
// byte-identical scores.
func (s *ExcessScorer[K, P]) Score(contributions []Contribution[K, P]) P {
	var mass P
	for _, c := range contributions {
		if c.Src == SrcText || c.Src == SrcVector || c.Src == SrcAnchor {
			mass += c.Score
		}
	}

	var excess P

	for _, c := range contributions {
		if c.Src != SrcGraph {
			continue
		}
		deg := P(c.Degree)
		if deg < 1 {
			deg = 1
		}
		if surplus := (c.Score - mass - deg*s.background) / deg; surplus > 0 {
			excess += surplus
		}
	}
	return mass + alpha*alpha*excess
}
