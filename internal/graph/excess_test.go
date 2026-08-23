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

package graph_test

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/graph"
)

// excessTraversalGraph builds the smallest graph exercising every band rule:
// a source fact on two anchors — one shared with a sibling, one exclusive —
// plus a fact reachable only beyond the two-band horizon.
//
//	source  — topic "left" (with sibling), entity "right" (alone)
//	sibling — topic "left" only
func excessTraversalGraph(t *testing.T) (g *graph.InMemoryGraph[uint64, float64], source, sibling, left, right uint64) {
	t.Helper()
	g = graph.NewGraph[uint64, float64](testConfig())
	now := time.Now()

	sourceFact := mkFact(g, "the source fact", now)
	siblingFact := mkFact(g, "the sibling fact", now)
	mustSet(t, g, sourceFact)
	mustSet(t, g, siblingFact)

	leftTopic := mkTopic(g, "left", now)
	mustSet(t, g, leftTopic)
	mustSet(t, g, graph.IsAbout[uint64]{Fact: &sourceFact, Topic: leftTopic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	mustSet(t, g, graph.IsAbout[uint64]{Fact: &siblingFact, Topic: leftTopic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})

	rightEntity := mkEntity(g, "right", now)
	mustSet(t, g, rightEntity)
	mustSet(t, g, graph.Mentions[uint64]{Fact: &sourceFact, NamedEntity: rightEntity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})

	return g, sourceFact.Key(), siblingFact.Key(), leftTopic.Key(), rightEntity.Key()
}

// runExcess traverses g from source with a fresh ExcessTraversal.
func runExcess(t *testing.T, g *graph.InMemoryGraph[uint64, float64], source uint64) graph.TraversalResult[uint64] {
	t.Helper()
	tr := graph.NewExcessTraversal[uint64, float64]()
	tr.SetSource(source)
	result, err := tr.Run(g)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	r, ok := result.(graph.TraversalResult[uint64])
	if !ok {
		t.Fatalf("Run returned %T, want a TraversalResult", result)
	}
	return r
}

// TestExcessTraversalBands pins the two-band shape: the source's anchors at
// depth 1 (both edge directions — the topic is reached through an outgoing
// IsAbout, the entity through the same directionality Mentions uses), their
// members at depth 2, ascending key within each band, and the source itself
// among the members of its own anchors.
func TestExcessTraversalBands(t *testing.T) {
	g, source, sibling, left, right := excessTraversalGraph(t)
	r := runExcess(t, g, source)

	anchors := []uint64{left, right}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i] < anchors[j] })
	members := []uint64{source, sibling}
	sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })
	if want := append(append([]uint64{}, anchors...), members...); !reflect.DeepEqual(r.Order, want) {
		t.Errorf("Order = %v, want anchors then members, ascending within bands: %v", r.Order, want)
	}
	for _, anchor := range anchors {
		if r.Depth[anchor] != 1 {
			t.Errorf("Depth[anchor %d] = %d, want 1", anchor, r.Depth[anchor])
		}
	}
	if r.Depth[source] != 2 || r.Depth[sibling] != 2 {
		t.Errorf("member depths = %d, %d, want 2, 2 — the source is a member of its own anchors", r.Depth[source], r.Depth[sibling])
	}
}

// TestExcessTraversalParentsCarryFullIncidence pins the field the band shape
// exists for: a member on two of the source's anchors is two observations,
// not one, so Parents carries both while Parent keeps a canonical entry.
func TestExcessTraversalParentsCarryFullIncidence(t *testing.T) {
	g, source, sibling, left, right := excessTraversalGraph(t)
	r := runExcess(t, g, source)

	sourceParents := append([]uint64{}, r.Parents[source]...)
	sort.Slice(sourceParents, func(i, j int) bool { return sourceParents[i] < sourceParents[j] })
	wantParents := []uint64{left, right}
	sort.Slice(wantParents, func(i, j int) bool { return wantParents[i] < wantParents[j] })
	if !reflect.DeepEqual(sourceParents, wantParents) {
		t.Errorf("Parents[source] = %v, want both anchors %v", r.Parents[source], wantParents)
	}
	if !reflect.DeepEqual(r.Parents[sibling], []uint64{left}) {
		t.Errorf("Parents[sibling] = %v, want the left topic alone", r.Parents[sibling])
	}
	if _, ok := r.Parent[source]; !ok {
		t.Error("Parent[source] missing: tree-shaped consumers need the canonical entry")
	}
}

// TestExcessTraversalCloneIsIndependent pins Clone's contract: a clone walks
// from its own source without disturbing the original's.
func TestExcessTraversalCloneIsIndependent(t *testing.T) {
	g, source, sibling, left, _ := excessTraversalGraph(t)

	original := graph.NewExcessTraversal[uint64, float64]()
	original.SetSource(source)
	clone := original.Clone()
	clone.SetSource(sibling)

	fromClone, err := clone.Run(g)
	if err != nil {
		t.Fatalf("clone Run = %v, want nil", err)
	}
	if r := fromClone.(graph.TraversalResult[uint64]); !reflect.DeepEqual(r.Parents[sibling], []uint64{left}) {
		t.Errorf("clone's walk = %v, want the sibling's own neighbourhood", r.Order)
	}
	fromOriginal, err := original.Run(g)
	if err != nil {
		t.Fatalf("original Run = %v, want nil", err)
	}
	if r := fromOriginal.(graph.TraversalResult[uint64]); r.Depth[source] != 2 {
		t.Errorf("original's source membership lost after Clone: %v", r.Order)
	}
}

// TestExcessTraversalMissingSource mirrors the BFS contract: an unknown
// source is an error, not an empty walk.
func TestExcessTraversalMissingSource(t *testing.T) {
	g, _, _, _, _ := excessTraversalGraph(t)
	tr := graph.NewExcessTraversal[uint64, float64]()
	tr.SetSource(0xdeadbeef)
	if _, err := tr.Run(g); err == nil {
		t.Fatal("Run from a missing source = nil error, want ErrSourceNotFound")
	}
}

// excessContributions builds a candidate list with mass m split over text and
// vector plus the given graph observations.
func excessContributions(m float64, graphs ...graph.Contribution[uint64, float64]) []graph.Contribution[uint64, float64] {
	out := []graph.Contribution[uint64, float64]{
		{Src: graph.SrcText, Score: m / 2, Rank: 0, Count: 1},
		{Src: graph.SrcVector, Score: m / 2, Rank: 0, Count: 1},
	}
	return append(out, graphs...)
}

// TestExcessScorerFloor is Property 5.2 at the fold: a graph-free list folds
// to exactly its seed mass, and any graph observation can only add — the
// hinge clips negative surplus to zero instead of subtracting.
func TestExcessScorerFloor(t *testing.T) {
	scorer := graph.NewExcessScorer[uint64, float64]()

	if got := scorer.WithBackground(3).Score(excessContributions(8)); got != 8 {
		t.Errorf("graph-free fold = %v, want exactly the seed mass 8", got)
	}
	// An anchor far below background must not drag the score under m.
	starved := excessContributions(8, graph.Contribution[uint64, float64]{Src: graph.SrcGraph, Score: 1, Via: 1, Degree: 50, Count: 1})
	if got := scorer.WithBackground(3).Score(starved); got != 8 {
		t.Errorf("fold with a deeply-below-background anchor = %v, want the floor 8", got)
	}
}

// TestExcessScorerHubSilenceAndPreemption pins the hinge arithmetic exactly
// (Properties 5.3 and 5.4). With background 2, a degree-6 anchor observing
// mass 14 owes 12 to the null and 2 to the candidate's own mass (self-
// exclusion): it transmits nothing — fair share exactly. Shrink its degree to
// 3 and the same observation carries surplus 6, of which α² = 1/4 arrives.
func TestExcessScorerHubSilenceAndPreemption(t *testing.T) {
	scorer := graph.NewExcessScorer[uint64, float64]()

	fairShare := excessContributions(2, graph.Contribution[uint64, float64]{Src: graph.SrcGraph, Score: 14, Via: 1, Degree: 6, Count: 2})
	if got := scorer.WithBackground(2).Score(fairShare); got != 2 {
		t.Errorf("fair-share anchor fold = %v, want the seed mass 2 alone (hub silence)", got)
	}

	surplus := excessContributions(2, graph.Contribution[uint64, float64]{Src: graph.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})
	if want := 2 + 0.25*(14-2-3*2.0); scorer.WithBackground(2).Score(surplus) != want {
		t.Errorf("surplus fold = %v, want m + α²·(M − m − d·ρ₀) = %v", scorer.WithBackground(2).Score(surplus), want)
	}

	// Earned preemption: a pure non-seed funded by the same surplus outranks
	// a weak seed of mass 1.
	nonSeed := []graph.Contribution[uint64, float64]{{Src: graph.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2}}
	if got := scorer.WithBackground(2).Score(nonSeed); got <= 1 {
		t.Errorf("funded non-seed fold = %v, want it to preempt a weak seed's mass 1", got)
	}
}

// TestExcessScorerScaleInvariance is Property 5.5 at the fold: scaling the
// masses and the background by the same power of two scales the score exactly
// — the methodology has no absolute scale to destabilize.
func TestExcessScorerScaleInvariance(t *testing.T) {
	scorer := graph.NewExcessScorer[uint64, float64]()
	list := excessContributions(2, graph.Contribution[uint64, float64]{Src: graph.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})

	const c = 4.0
	scaled := make([]graph.Contribution[uint64, float64], len(list))
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
	scorer := graph.NewExcessScorer[uint64, float64]()
	list := excessContributions(2, graph.Contribution[uint64, float64]{Src: graph.SrcGraph, Score: 14, Via: 1, Degree: 3, Count: 2})
	snapshot := make([]graph.Contribution[uint64, float64], len(list))
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
