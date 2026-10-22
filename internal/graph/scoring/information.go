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

// InformationScorer folds a candidate's evidence channels, text, vector and
// graph, by how much each says about it beyond chance, and weighs each channel
// by how much it says about the query's candidates as a whole.
//
// A channel c's evidence on fact f is measured against the recall's own pool
// of n candidates: p = c_f / Σc is the channel's share on f and r = n·p how
// many times an even share that is, so log₂(1 + r) is the channel's bits on f:
// 0 for a fact it did not see, 1 at an even share, more the more it singles f
// out. A channel's weight is its information over the pool, I_c = Σ_f p ·
// log₂(n·p) = log₂ n − H(p): zero for a channel that cannot tell the
// candidates apart, log₂ n for one that names a single fact. The vector's
// weight is multiplied by the text's weakness I_q / I_hit (see
// Observation.TextMissed), so the embedding takes over the ranking where the
// words fall short, as the text decides rather than the vector's own scores.
// The score is
//
//	Σ_c w_c · log₂(1 + n · c_f / Σc),  w ∝ (I_text, (I_q / I_hit) · I_vector, I_graph)
//
// The channels: text is the text mass; graph is the ExcessScorer's graph term,
// the hinged per-edge surpluses of the anchors that reach f, without
// attenuation; vector is the similarity above the floor of the vector's own
// reading, the similarity of the last hit it returned. The floor is what lets
// the vector channel discriminate: an embedding places most texts at nearly
// the same distance from any question, so raw similarities differ by a few
// hundredths across a reading and every hit would carry the same bits.
// Measured above the floor, the hits the vector ranks first stand apart from
// the rest, as the text's and the graph's do.
//
// Every per-query constant cancels in r, so the channels' units, the
// attenuation α and the vector weight inside a channel all drop out: the
// channels are commensurable without being calibrated against each other, and
// the fold has no constant of its own to tune. The graph is still seeded with
// the excess methodology's seed mass, which the scorer folds before the pool
// exists (see Observe).
//
// A query with nothing to match, named anchors alone, has no channel to weigh,
// so each fact scores its anchor sightings, one unit per named anchor it is
// filed under, as under the ExcessScorer.
type InformationScorer[K comparable, P float32 | float64] struct {
	// excess carries the query's background and text weakness and reads each
	// candidate's channels.
	excess ExcessScorer[K, P]

	// pooled is set once a candidate pool is bound. Before that a fold is
	// seed fusion, and returns the seed mass.
	pooled bool

	// n is the pool's size and floor the vector reading's floor. totals and
	// weights are each channel's sum over the pool and its share of the
	// weight, in the order text, vector, graph.
	n       P
	floor   P
	totals  [3]P
	weights [3]P
}

// NewInformationScorer returns the information fold, unbound: until Observe
// binds a candidate pool it folds the seed mass the graph is seeded with.
func NewInformationScorer[K comparable, P float32 | float64]() *InformationScorer[K, P] {
	return &InformationScorer[K, P]{}
}

// Observe returns a scorer bound to one query. Bound without a candidate pool,
// it folds each seed's own seed mass, which is what the traversal aggregates
// before the pool exists. Bound with the pool, it reads every candidate's
// channels once, in o.Keys order, for each channel's total, the vector
// reading's floor and each channel's information, which set the weights.
func (s *InformationScorer[K, P]) Observe(o Observation[K, P]) Scorer[K, P] {
	bound := &InformationScorer[K, P]{excess: ExcessScorer[K, P]{background: o.Background, missed: o.TextMissed}}
	if o.Candidates == nil {
		return bound
	}
	bound.pooled = true
	bound.n = P(len(o.Keys))

	values := make([][3]P, len(o.Keys))
	floor := P(-1)
	for i, key := range o.Keys {
		text, vector, _, graph := bound.excess.fold(o.Candidates[key])
		values[i] = [3]P{text, vector, graph}
		if vector > 0 && (floor < 0 || vector < floor) {
			floor = vector
		}
	}
	if floor > 0 {
		bound.floor = floor
	}
	for i := range values {
		values[i][1] = bound.aboveFloor(values[i][1])
		for c, v := range values[i] {
			bound.totals[c] += v
		}
	}

	var information [3]P
	for _, value := range values {
		for c, v := range value {
			if v > 0 {
				p := v / bound.totals[c]
				information[c] += p * P(math.Log2(float64(bound.n*p)))
			}
		}
	}
	information[1] *= 1 + o.TextMissed
	sum := information[0] + information[1] + information[2]
	for c := range bound.weights {
		bound.weights[c] = 1
		if sum > 0 {
			bound.weights[c] = information[c] / sum
		}
	}
	return bound
}

// Score folds contributions under the bound query: the seed mass before a
// pool is bound or when no channel has anything to weigh, the weighted bits of
// its channels otherwise.
func (s *InformationScorer[K, P]) Score(contributions []Contribution[K, P]) P {
	text, vector, mass, graph := s.excess.fold(contributions)
	if !s.pooled || s.totals == [3]P{} {
		return mass
	}
	var score P
	for c, v := range [3]P{text, s.aboveFloor(vector), graph} {
		if s.totals[c] > 0 {
			score += s.weights[c] * P(math.Log2(float64(1+s.n*v/s.totals[c])))
		}
	}
	return score
}

// aboveFloor is a vector similarity measured above the reading's floor; a
// fact the vector did not return has none.
func (s *InformationScorer[K, P]) aboveFloor(vector P) P {
	if vector <= s.floor {
		return 0
	}
	return vector - s.floor
}
