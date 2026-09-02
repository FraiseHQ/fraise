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

// TestExcessScorerFloor is Property 5.2 at the fold: a graph-free list folds
// to exactly its seed mass, and any graph observation can only add — the
// hinge clips negative surplus to zero instead of subtracting.
func TestExcessScorerFloor(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()

	if got := scorer.WithBackground(3).Score(excessContributions(8)); got != 8 {
		t.Errorf("graph-free fold = %v, want exactly the seed mass 8", got)
	}
	// An anchor far below background must not drag the score under m.
	starved := excessContributions(8, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 1, Via: 1, Degree: 50, Count: 1})
	if got := scorer.WithBackground(3).Score(starved); got != 8 {
		t.Errorf("fold with a deeply-below-background anchor = %v, want the floor 8", got)
	}
}

// TestExcessScorerHubSilenceAndPreemption pins the hinge arithmetic exactly
// (Properties 5.3 and 5.4). With background 2, a degree-6 anchor observing
// mass 14 owes 12 to the null and 2 to the candidate's own mass (self-
// exclusion): it transmits nothing — fair share exactly. Shrink its degree to
// 3 and the same observation carries surplus 6 spread over its 3 edges, of
// which α² = 1/4 arrives per edge.
func TestExcessScorerHubSilenceAndPreemption(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()

	fairShare := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 6, Count: 2})
	if got := scorer.WithBackground(2).Score(fairShare); got != 2 {
		t.Errorf("fair-share anchor fold = %v, want the seed mass 2 alone (hub silence)", got)
	}

	surplus := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})
	if want := 2 + 0.25*(14-2-3*2.0)/3; scorer.WithBackground(2).Score(surplus) != want {
		t.Errorf("surplus fold = %v, want m + α²·(M − m − d·ρ₀)/d = %v", scorer.WithBackground(2).Score(surplus), want)
	}

	// Earned preemption: a pure non-seed funded by a rich anchor outranks a
	// weak seed of mass 1. Transmission is per-edge, so the anchor must clear
	// α²·(M − d·ρ₀)/d > 1: at degree 3 and background 2, mass 20 funds 7/6.
	nonSeed := []scoring.Contribution[uint64, float64]{{Src: scoring.SrcGraph, Score: 20, Via: 1, Degree: 3, Count: 2}}
	if got := scorer.WithBackground(2).Score(nonSeed); got <= 1 {
		t.Errorf("funded non-seed fold = %v, want it to preempt a weak seed's mass 1", got)
	}
}

// TestExcessScorerScaleInvariance is Property 5.5 at the fold: scaling the
// masses and the background by the same power of two scales the score exactly
// — the methodology has no absolute scale to destabilize.
func TestExcessScorerScaleInvariance(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()
	list := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})

	const c = 4.0
	scaled := make([]scoring.Contribution[uint64, float64], len(list))
	copy(scaled, list)
	for i := range scaled {
		scaled[i].Score *= c
	}
	if got, want := scorer.WithBackground(2*c).Score(scaled), c*scorer.WithBackground(2).Score(list); got != want {
		t.Errorf("scaled fold = %v, want exactly c·original = %v", got, want)
	}
}

// TestExcessScorerPurity pins the Scorer contract's purity clause: folding
// the same list twice yields the identical result and leaves the list
// unmutated, and binding a background never mutates the shared instance —
// Search shares it across concurrent queries.
func TestExcessScorerPurity(t *testing.T) {
	scorer := scoring.NewExcessScorer[uint64, float64]()
	list := excessContributions(2, scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})
	snapshot := make([]scoring.Contribution[uint64, float64], len(list))
	copy(snapshot, list)

	first := scorer.WithBackground(2).Score(list)
	second := scorer.WithBackground(2).Score(list)
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

// TestExcessScorerAnchorSightingsAreSeedMass pins the anchor-seeded fold: anchor
// sightings sum into the seed mass exactly as text and vector ones do — one
// unit per named anchor a fact is filed under, so two sightings fold to 2 —
// and that mass is self-excluded by the hinge like any other, so a graph
// observation on top of it transmits only what exceeds it.
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
	// 6 − 2 − 1·0 = 4, of which α² = 1/4 arrives.
	funded := append(append([]scoring.Contribution[uint64, float64]{}, two...), scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 6, Via: 3, Degree: 1, Count: 1})
	if got, want := scorer.Score(funded), 2+0.25*(6-2-1*0.0)/1; got != want {
		t.Errorf("funded fold = %v, want m + α²·(M − m − d·ρ₀)/d = %v", got, want)
	}
}
