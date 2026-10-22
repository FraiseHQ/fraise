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
	"reflect"
	"testing"

	"github.com/FraiseHQ/fraise/internal/graph/scoring"
)

// excessContributions builds a candidate list with mass m split over text and
// vector plus the given graph observations.
func excessContributions(m float64, graphs ...scoring.Contribution[uint64, float64]) []scoring.Contribution[uint64, float64] {
	out := []scoring.Contribution[uint64, float64]{
		{Src: scoring.SrcText, Score: m / 2, Rank: 0, Count: 1},
		{Src: scoring.SrcVector, Score: m / 2, Rank: 0, Count: 1},
	}
	return append(out, graphs...)
}

// TestExcessScorerFloor pins the BM25 floor at the fold. A graph-free list
// folds to exactly its seed mass. A graph observation can only add, since the
// hinge clips a negative surplus to zero instead of subtracting it.
func TestExcessScorerFloor(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()

	if got := scorer.Observe(scoring.Observation[uint64, float64]{Background: 3}).Score(excessContributions(8)); got != 8 {
		t.Errorf("graph-free fold = %v, want exactly the seed mass 8", got)
	}
	// An anchor far below background must not drag the score under m.
	starved := excessContributions(8, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 1, Via: 1, Degree: 50, Count: 1})
	if got := scorer.Observe(scoring.Observation[uint64, float64]{Background: 3}).Score(starved); got != 8 {
		t.Errorf("fold with a deeply-below-background anchor = %v, want the floor 8", got)
	}
}

// TestExcessScorerHubSilenceAndPreemption pins the hinge arithmetic behind hub
// silence and earned preemption. With background 2, a degree-6 anchor observing
// mass 14 owes 12 to the background. It owes 2 to the candidate's own mass
// (self-exclusion), so it transmits nothing. At degree 3, the same observation
// holds surplus 6, or 2 per edge, and α² = 1/4 of that arrives.
func TestExcessScorerHubSilenceAndPreemption(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()

	fairShare := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 6, Count: 2})
	if got := scorer.Observe(scoring.Observation[uint64, float64]{Background: 2}).Score(fairShare); got != 2 {
		t.Errorf("fair-share anchor fold = %v, want the seed mass 2 alone (hub silence)", got)
	}

	surplus := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})
	if want := 2 + 0.25*(14-2-3*2.0)/3; scorer.Observe(scoring.Observation[uint64, float64]{Background: 2}).Score(surplus) != want {
		t.Errorf("surplus fold = %v, want m + α²·(M − m − d·ρ₀)/d = %v", scorer.Observe(scoring.Observation[uint64, float64]{Background: 2}).Score(surplus), want)
	}

	// Earned preemption: a pure non-seed funded by a rich anchor outranks a
	// weak seed of mass 1. Transmission is per edge, so the anchor must clear
	// α²·(M − d·ρ₀)/d > 1. At degree 3 and background 2, mass 20 funds 7/6.
	nonSeed := []scoring.Contribution[uint64, float64]{{Src: scoring.SrcGraph, Score: 20, Via: 1, Degree: 3, Count: 2}}
	if got := scorer.Observe(scoring.Observation[uint64, float64]{Background: 2}).Score(nonSeed); got <= 1 {
		t.Errorf("funded non-seed fold = %v, want it to preempt a weak seed's mass 1", got)
	}
}

// TestExcessScorerScaleInvariance pins scale invariance at the fold. Scaling
// the masses and the background by the same power of two scales the score
// exactly. Nothing in the fold depends on an absolute scale.
func TestExcessScorerScaleInvariance(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()
	list := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})

	const c = 4.0
	scaled := make([]scoring.Contribution[uint64, float64], len(list))
	copy(scaled, list)
	for i := range scaled {
		scaled[i].Score *= c
	}
	if got, want := scorer.Observe(scoring.Observation[uint64, float64]{Background: 2 * c}).Score(scaled), c*scorer.Observe(scoring.Observation[uint64, float64]{Background: 2}).Score(list); got != want {
		t.Errorf("scaled fold = %v, want exactly c·original = %v", got, want)
	}
}

// TestExcessScorerPurity pins the purity clause of the Scorer contract. Folding
// the same list twice gives the same result and leaves the list unchanged.
// Binding a background never mutates the shared instance, which Search shares
// across concurrent queries.
func TestExcessScorerPurity(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()
	list := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})
	snapshot := make([]scoring.Contribution[uint64, float64], len(list))
	copy(snapshot, list)

	first := scorer.Observe(scoring.Observation[uint64, float64]{Background: 2}).Score(list)
	second := scorer.Observe(scoring.Observation[uint64, float64]{Background: 2}).Score(list)
	if first != second {
		t.Errorf("two folds of one list = %v then %v, want identical", first, second)
	}
	if unbound := scorer.Score(list); unbound == first {
		t.Errorf("binding leaked into the shared instance: unbound fold = bound fold = %v", unbound)
	}
	if !reflect.DeepEqual(list, snapshot) {
		t.Errorf("fold mutated its input: %+v, want %+v", list, snapshot)
	}
}

// TestExcessScorerWeighsTheVectorByTheTextsWeakness pins the vector weight at
// the fold: the vector contribution records the similarity as measured, and
// the scorer counts it 1 + TextMissed = I_q / I_hit times. A text hit holding
// half the question's information misses one bit per bit found, so the
// similarity counts double beside the same text mass, and the zero
// observation leaves it at its plain value.
func TestExcessScorerWeighsTheVectorByTheTextsWeakness(t *testing.T) {
	list := []scoring.Contribution[uint64, float64]{
		{Src: scoring.SrcText, Score: 2, Rank: 0, Count: 1},
		{Src: scoring.SrcVector, Score: 0.5, Rank: 0, Count: 1},
	}
	scorer := scoring.NewExcessScorer[uint64, float64]()

	if got := scorer.Observe(scoring.Observation[uint64, float64]{}).Score(list); got != 2.5 {
		t.Errorf("fold with nothing missed = %v, want text 2 + similarity 0.5", got)
	}
	if got := scorer.Observe(scoring.Observation[uint64, float64]{TextMissed: 1}).Score(list); got != 3 {
		t.Errorf("fold with one bit missed per bit found = %v, want text 2 + 2·0.5", got)
	}
}

// TestExcessScorerNamedObservationOwesNoFairShare pins the named term: a named
// anchor's observation is folded like a graph observation but without the
// fair share, since the query asked for that anchor. At background 2 an anchor
// of degree 6 observing 14 owes 12 to the background and transmits nothing to
// a candidate of mass 2; named, the same observation gives (14 − 2) / 6 = 2,
// of which α² = 1/4 arrives.
func TestExcessScorerNamedObservationOwesNoFairShare(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{Background: 2})

	unnamed := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 6, Count: 2})
	if got := scorer.Score(unnamed); got != 2 {
		t.Errorf("unnamed fair-share anchor fold = %v, want the seed mass 2 alone (hub silence)", got)
	}
	named := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcNamed, Score: 14, Via: 1, Degree: 6, Count: 2})
	if got, want := scorer.Score(named), 2+0.25*(14-2.0)/6; got != want {
		t.Errorf("named anchor fold = %v, want m + α²·(M − m)/d = %v", got, want)
	}
}

// TestExcessScorerAnchorSightingsAreSeedMass pins the anchor-seeded fold:
// anchor sightings sum into the seed mass as text and vector ones do, one unit
// per named anchor a fact is filed under, so two sightings fold to 2. The
// hinge self-excludes that mass like any other, so a graph observation on top
// of it transmits only what exceeds it.
func TestExcessScorerAnchorSightingsAreSeedMass(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()

	one := []scoring.Contribution[uint64, float64]{{Src: scoring.SrcAnchor, Score: 1, Via: 1, Degree: 3, Count: 1}}
	if got := scorer.Score(one); got != 1 {
		t.Errorf("one anchor sighting folds to %v, want its unit mass 1", got)
	}

	two := []scoring.Contribution[uint64, float64]{
		{Src: scoring.SrcAnchor, Score: 1, Via: 1, Degree: 3, Count: 1},
		{Src: scoring.SrcAnchor, Score: 1, Via: 2, Degree: 2, Count: 1},
	}
	if got := scorer.Score(two); got != 2 {
		t.Errorf("two anchor sightings fold to %v, want 2", got)
	}

	// Mass 2, background 0: an anchor observing 6 over one edge holds surplus
	// 6 − 2 − 1·0 = 4. α² = 1/4 of it arrives.
	funded := append(append([]scoring.Contribution[uint64, float64]{}, two...), scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 6, Via: 3, Degree: 1, Count: 1})
	if got, want := scorer.Score(funded), 2+0.25*(6-2-1*0.0)/1; got != want {
		t.Errorf("funded fold = %v, want m + α²·(M − m − d·ρ₀)/d = %v", got, want)
	}
}
