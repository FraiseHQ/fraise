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

import "math"

// CompatibilityScorer reproduces the aggregation Search performed inline
// before contributions existed, so landing the collection layer moves no
// text- or graph-ranked result: text contributions score by reciprocal rank,
// 1/(1+Rank), and graph contributions by the seed's fused score attenuated
// per hop, Score·attenuation^Hop. The one deliberate departure is vector
// contributions, Score/(1+Rank): the old path discarded the similarity and
// ranked by position alone, so a distant nearest-neighbour seeded as strongly
// as an exact match. An exact match (similarity 1) still scores 1/(1+Rank),
// so vector ranking moves only where similarity actually separates the seeds.
type CompatibilityScorer[K comparable, P float32 | float64] struct {
	// attenuation is the per-hop decay applied to graph contributions
	// (config db.hop-attenuation). Held as float64 because the old inline
	// math raised the config value directly; narrowing to a float32 P first
	// would perturb the scores this scorer exists to preserve.
	attenuation float64
}

// NewCompatibilityScorer returns a CompatibilityScorer decaying graph
// contributions by attenuation^hop.
func NewCompatibilityScorer[K comparable, P float32 | float64](attenuation float64) *CompatibilityScorer[K, P] {
	return &CompatibilityScorer[K, P]{attenuation: attenuation}
}

// Score folds contributions by source: text 1/(1+Rank), vector Score/(1+Rank),
// graph Score·attenuation^Hop, summed in list order. The text match count is
// deliberately unused — the pre-contribution pipeline ranked text seeds by
// position only, and this scorer's contract is not to move them.
func (s *CompatibilityScorer[K, P]) Score(contributions []Contribution[P]) P {
	var total P
	for _, c := range contributions {
		switch c.Src {
		case SrcText:
			total += P(1) / (P(1) + P(c.Rank))
		case SrcVector:
			total += c.Score / (P(1) + P(c.Rank))
		case SrcGraph:
			total += c.Score * P(math.Pow(s.attenuation, float64(c.Hop)))
		}
	}
	return total
}
