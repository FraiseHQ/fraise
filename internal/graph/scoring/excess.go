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
// constant, not configuration, because the BM25 floor and hub silence hold for
// any 0 < α < 1. The seed → anchor → fact path has two edges, so the fold
// applies α². Applying α once per path would double the graph channel's weight
// against the text channel.
const alpha = 0.5

// ExcessScorer folds a candidate's observations under the excess-transmission
// methodology. Relevance is the candidate's own seed mass m plus, for each
// graph observation, its anchor's per-edge surplus over the fair share, and
// for each named observation, the named anchor's per-edge mass, both
// attenuated α² for the two-edge path:
//
//	m + α² · [ Σ_graph max(0, (M_A − m − d_A·ρ₀) / d_A) + Σ_named max(0, (M_N − m) / d_N) ]
//
// where M_A is the anchor's full observed mass, d_A its degree and ρ₀ the
// background rate. Subtracting m is self-exclusion: a fact never funds its own
// boost. An anchor at or below its fair share contributes nothing (hub
// silence), so a hub is heard when it is surprising and silent when it is
// merely large. A named anchor is not held to its fair share: the query asked
// for it, so its size is not noise to silence, and dividing its mass by its
// degree already makes an anchor spread over most of the graph add next to
// nothing while one whose few facts the query matched adds a lot.
//
// The seed mass is the text mass, the vector similarity weighted by the
// text's weakness, w = I_q / I_hit (see Observation.TextMissed), and the
// anchor sightings: the embedding is credited with the bits the text missed
// per bit it found, so a query whose words the best text hit holds outright
// weighs it at 1 and the text leads, and one the text half answers doubles
// it.
//
// Scores stay in raw seed units. Relevance is homogeneous of degree 1 in the
// mass scale, so rescaling every mass leaves the ordering unchanged (scale
// invariance). Normalising a single channel would only break the channels'
// commensurability.
type ExcessScorer[K comparable, P float32 | float64] struct {
	// background is the query's bound null rate and missed its text's
	// weakness. They are set only by Observe returning a fresh value — never
	// mutated in place — so the graph's shared instance stays pure and the
	// zero value is the unbound scorer seed fusion runs.
	background P
	missed     P
}

// NewExcessScorer returns the excess-transmission fold, unbound (background
// zero, the vector at weight 1) until Observe binds a query.
func NewExcessScorer[K comparable, P float32 | float64]() *ExcessScorer[K, P] {
	return &ExcessScorer[K, P]{}
}

// Observe returns a scorer bound to one query's background rate and text
// weakness. The candidate pool plays no part: each candidate folds on its
// own observations.
func (s *ExcessScorer[K, P]) Observe(o Observation[K, P]) Scorer[K, P] {
	return &ExcessScorer[K, P]{background: o.Background, missed: o.TextMissed}
}

// Score folds contributions under the bound observation.
func (s *ExcessScorer[K, P]) Score(contributions []Contribution[K, P]) P {
	_, _, mass, graph := s.fold(contributions)
	return mass + alpha*alpha*graph
}

// fold reads a candidate's contributions the way the methodology does and
// returns its parts, which ExcessScorer adds and InformationScorer weighs
// apart: the text mass, the vector similarity as recorded, the seed mass m
// (text, weighted vector and anchor sightings) and the graph term, the sum of
// the hinged per-edge surpluses before attenuation. Seed mass first, then the
// hinge over each graph and named observation in list order, so identical
// inputs fold to byte-identical parts.
func (s *ExcessScorer[K, P]) fold(contributions []Contribution[K, P]) (text, vector, mass, graph P) {
	var anchor P
	for _, c := range contributions {
		switch c.Src {
		case SrcText:
			text += c.Score
		case SrcVector:
			vector += c.Score
		case SrcAnchor:
			anchor += c.Score
		}
	}
	mass = text + (1+s.missed)*vector + anchor

	for _, c := range contributions {
		if c.Src != SrcGraph && c.Src != SrcNamed {
			continue
		}
		deg := P(c.Degree)
		if deg < 1 {
			deg = 1
		}
		share := deg * s.background
		if c.Src == SrcNamed {
			share = 0
		}
		if surplus := (c.Score - mass - share) / deg; surplus > 0 {
			graph += surplus
		}
	}
	return text, vector, mass, graph
}
