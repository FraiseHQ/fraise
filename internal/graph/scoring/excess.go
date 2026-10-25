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

// ExcessScorer folds a candidate's observations under the excess-transmission
// methodology. Relevance is the candidate's own seed mass m plus, for each
// graph observation, its anchor's per-edge surplus over the fair share:
//
//	m + Σ max(0, (M_A − m − d_A·ρ₀) / d_A)
//
// M_A is the anchor's full observed mass, d_A its degree and ρ₀ the background
// rate. Subtracting m is self-exclusion: a fact never funds its own boost. An
// anchor at or below its fair share contributes nothing (hub silence). So a hub
// is heard when it is surprising and silent when it is merely large.
//
// An anchor the query named has no fair share to clear: the query asserts it
// bears on the question, so chance is not the null to test it against. Its
// members gain max(0, (M_A − m) / d_A). A match filed under it is lifted by
// the mass its siblings there found, and a match filed elsewhere keeps its own
// mass. Self-exclusion still holds, so a named anchor holding one match lifts
// nothing.
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
	return mass + graph
}

// fold reads a candidate's contributions the way the methodology does and
// returns its parts, which ExcessScorer adds and InformationScorer weighs
// apart: the text mass, the vector similarity as recorded, the seed mass m
// (text, weighted vector and anchor sightings) and the graph term, the sum of
// the hinged per-edge surpluses. Seed mass first, then the hinge over each
// graph observation in list order, so identical inputs fold to
// byte-identical parts.
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
		if c.Src != SrcGraph {
			continue
		}
		deg := P(c.Degree)
		if deg < 1 {
			deg = 1
		}
		fair := deg * s.background
		if c.Named {
			fair = 0
		}
		if surplus := (c.Score - mass - fair) / deg; surplus > 0 {
			graph += surplus
		}
	}
	return text, vector, mass, graph
}
