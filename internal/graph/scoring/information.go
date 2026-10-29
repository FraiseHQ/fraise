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

import "math"

// InformationScorer folds a candidate's evidence channels (text, vector and
// graph) by how far each places the candidate above the rest of the recall's
// pool, and lets the channels that are independent witnesses amplify each
// other.
//
// A channel's evidence on fact f is its z-score over the pool of n candidates,
// z = (c_f − mean) / sd, with the facts the channel did not see counted at
// zero: having been seen at all is part of what a channel says. Only the part
// above the pool's mean counts, z⁺ = max(0, z), so a channel never counts
// against a fact. The pool holds the facts the graph funds as well as the
// matches, so the anchor round moves every channel's mean and spread, and a
// match's z can fall when the graph speaks though its own mass is unchanged.
// The score is
//
//	(1 + z_text⁺) · (1 + z_vector⁺)^e + z_graph⁺
//
// A fact every channel leaves at or below the pool's mean scores 1, not 0:
// recency decay scales the score, and a zero would leave every such match
// tied whatever its age, in key order, where the newer of two equal matches
// must come first.
//
// The text and the vector multiply because they are independent readings of
// the question, one of its words and one of its meaning: a fact both single
// out is corroborated, and gains more than either alone. The graph is added
// instead, since it is not independent of them. It is the seeds' own mass
// passed on through their anchors, so multiplying it would count the seeds
// twice and raise their neighbours on the strength of the seeds themselves.
//
// e is how much the vector is trusted on f. It is 1, unless the vector is
// confident about f and the words are not: when z_vector clears the bar a
// chance maximum over n candidates would reach, √(2 ln n), e = 2 − h, where h
// is the share of the query's information f's own words hold (see
// Contribution.Held). A fact the embedding singles out beyond chance and the
// words miss entirely counts its vector squared; one whose words hold the
// whole question counts it once, as the text already corroborates it. A
// vector short of the bar is not trusted further, so a boost never amplifies
// an embedding that only rates everything alike.
//
// The channels: text is the text mass, vector the similarity as measured, and
// graph the ExcessScorer's graph term, the hinged per-edge surpluses of the
// anchors that reach f. A z-score is free of its channel's units, so the
// channels are commensurable without being calibrated against each other, and
// the fold carries no constant of its own. The graph is still seeded with the
// excess methodology's seed mass, which the scorer folds before the pool
// exists (see Observe).
//
// A pool no channel spreads has nothing to measure a fact against, so each
// fact scores its seed mass: a recall that found a single fact scores that
// fact's own match, and one with nothing to match, named anchors alone,
// scores each fact one unit per named anchor it is filed under, as under the
// ExcessScorer.
type InformationScorer[K comparable, P float32 | float64] struct {
	// excess carries the query's background and text weakness and reads each
	// candidate's channels.
	excess ExcessScorer[K, P]

	// pooled is set once a candidate pool is bound. Before that a fold is
	// seed fusion, and returns the seed mass.
	pooled bool

	// mean and sd are each channel's statistics over the pool, in the order
	// text, vector, graph, and bar the z the vector must clear to be trusted
	// further.
	mean [3]P
	sd   [3]P
	bar  P
}

// NewInformationScorer returns the information fold, unbound: until Observe
// binds a candidate pool it folds the seed mass the graph is seeded with.
func NewInformationScorer[K comparable, P float32 | float64]() *InformationScorer[K, P] {
	return &InformationScorer[K, P]{}
}

// Observe returns a scorer bound to one query. Bound without a candidate pool,
// it folds each seed's own seed mass, which is what the traversal aggregates
// before the pool exists. Bound with the pool, it reads every candidate's
// channels in o.Keys order, for each channel's mean and spread.
func (s *InformationScorer[K, P]) Observe(o Observation[K, P]) Scorer[K, P] {
	bound := &InformationScorer[K, P]{excess: ExcessScorer[K, P]{background: o.Background, missed: o.TextMissed}}
	if o.Candidates == nil || len(o.Keys) == 0 {
		return bound
	}
	bound.pooled = true
	// Two passes, the mean first and then the spread about it, so a channel
	// that holds one value over the whole pool has a spread of exactly zero
	// rather than a rounding error's worth.
	n := P(len(o.Keys))
	values := make([][3]P, len(o.Keys))
	for i, key := range o.Keys {
		text, vector, _, graph := bound.excess.fold(o.Candidates[key])
		values[i] = [3]P{text, vector, graph}
		for c, v := range values[i] {
			bound.mean[c] += v / n
		}
	}
	var spread [3]P
	for _, value := range values {
		for c, v := range value {
			spread[c] += (v - bound.mean[c]) * (v - bound.mean[c]) / n
		}
	}
	for c, variance := range spread {
		if variance > 0 {
			bound.sd[c] = P(math.Sqrt(float64(variance)))
		}
	}
	bound.bar = P(math.Sqrt(2 * math.Log(float64(n))))
	return bound
}

// Score folds contributions under the bound query: the seed mass before a
// pool is bound or when no channel spreads the pool, the amplified channels
// otherwise.
func (s *InformationScorer[K, P]) Score(contributions []Contribution[K, P]) P {
	text, vector, mass, graph := s.excess.fold(contributions)
	if !s.pooled || s.sd == [3]P{} {
		return mass
	}
	zText, zVector, zGraph := s.above(0, text), s.above(1, vector), s.above(2, graph)

	trust := P(1)
	if zVector > s.bar {
		var held P
		for _, c := range contributions {
			if c.Src == SrcText && c.Held > held {
				held = c.Held
			}
		}
		trust = 2 - held
	}
	return (1+zText)*P(math.Pow(float64(1+zVector), float64(trust))) + zGraph
}

// above is channel c's z-score for value v over the pool, floored at zero:
// how far above the pool's mean the channel places a fact it saw.
func (s *InformationScorer[K, P]) above(c int, v P) P {
	if v <= 0 || s.sd[c] <= 0 {
		return 0
	}
	return max(0, (v-s.mean[c])/s.sd[c])
}
