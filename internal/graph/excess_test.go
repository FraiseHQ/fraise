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
	"slices"
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/google/go-cmp/cmp"
)

// excessTraversalGraph builds the smallest graph that exercises every step
// rule. A source fact sits on two anchors, one shared with a sibling and one
// exclusive:
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

// runExcess walks g from sources with a fresh ExcessTraversal.
func runExcess(t *testing.T, g *graph.InMemoryGraph[uint64, float64], sources ...uint64) graph.TraversalResult[uint64] {
	t.Helper()
	tr := graph.NewExcessTraversal[uint64, float64]()
	tr.SetSources(sources)
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

// ascending returns keys sorted, the order ExcessTraversal lists what it
// reaches in.
func ascending(keys ...uint64) []uint64 {
	slices.Sort(keys)
	return keys
}

// TestExcessTraversalStepsAcrossTheBipartition pins the one step. A fact
// steps to its anchors, along its outgoing IsAbout and Mentions edges, and an
// anchor steps to its members, along its incoming ones. Everything reached
// sits at depth 1, listed in ascending key order. Parents carries the full
// incidence, every source a vertex was reached from, in the order the sources
// were given: an anchor two seeds reach is funded twice, and a member of two
// anchors is observed twice. A seed is a member of its own anchors. A source
// the graph does not hold is skipped.
func TestExcessTraversalStepsAcrossTheBipartition(t *testing.T) {
	g, source, sibling, left, right := excessTraversalGraph(t)
	type walk struct {
		Order   []uint64
		Depth   map[uint64]int
		Parents map[uint64][]uint64
	}
	cases := map[string]struct {
		sources []uint64
		want    walk
	}{
		"a fact steps to its anchors": {
			sources: []uint64{source},
			want: walk{
				Order:   ascending(left, right),
				Depth:   map[uint64]int{left: 1, right: 1},
				Parents: map[uint64][]uint64{left: {source}, right: {source}},
			},
		},
		"an anchor steps to its members, the seed among them": {
			sources: []uint64{left},
			want: walk{
				Order:   ascending(source, sibling),
				Depth:   map[uint64]int{source: 1, sibling: 1},
				Parents: map[uint64][]uint64{source: {left}, sibling: {left}},
			},
		},
		"an anchor two seeds reach is funded by both, in source order": {
			sources: []uint64{sibling, source},
			want: walk{
				Order:   ascending(left, right),
				Depth:   map[uint64]int{left: 1, right: 1},
				Parents: map[uint64][]uint64{left: {sibling, source}, right: {source}},
			},
		},
		"a member of two anchors is observed through both": {
			sources: []uint64{right, left},
			want: walk{
				Order:   ascending(source, sibling),
				Depth:   map[uint64]int{source: 1, sibling: 1},
				Parents: map[uint64][]uint64{source: {right, left}, sibling: {left}},
			},
		},
		"a source the graph does not hold is skipped": {
			sources: []uint64{0xdeadbeef, source},
			want: walk{
				Order:   ascending(left, right),
				Depth:   map[uint64]int{left: 1, right: 1},
				Parents: map[uint64][]uint64{left: {source}, right: {source}},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := runExcess(t, g, tc.sources...)
			got := walk{Order: r.Order, Depth: r.Depth, Parents: r.Parents}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("walk from %v mismatch (-want +got):\n%s", tc.sources, diff)
			}
			for _, v := range r.Order {
				if parents := r.Parents[v]; r.Parent[v] != parents[0] {
					t.Errorf("Parent[%d] = %d, want the first of Parents %v for tree-shaped consumers", v, r.Parent[v], parents)
				}
			}
		})
	}
}

// TestExcessTraversalCloneIsIndependent pins Clone's contract: a clone walks
// from its own sources without disturbing the original's.
func TestExcessTraversalCloneIsIndependent(t *testing.T) {
	g, source, sibling, left, right := excessTraversalGraph(t)

	original := graph.NewExcessTraversal[uint64, float64]()
	original.SetSources([]uint64{source})
	clone := original.Clone()
	clone.SetSources([]uint64{sibling})

	fromClone, err := clone.Run(g)
	if err != nil {
		t.Fatalf("clone Run = %v, want nil", err)
	}
	if diff := cmp.Diff([]uint64{left}, fromClone.(graph.TraversalResult[uint64]).Order); diff != "" {
		t.Errorf("clone's walk mismatch (-want +got), want the sibling's own anchor:\n%s", diff)
	}
	fromOriginal, err := original.Run(g)
	if err != nil {
		t.Fatalf("original Run = %v, want nil", err)
	}
	if diff := cmp.Diff(ascending(left, right), fromOriginal.(graph.TraversalResult[uint64]).Order); diff != "" {
		t.Errorf("original's walk mismatch (-want +got) after Clone:\n%s", diff)
	}
}

// TestExcessTraversalMissingSource mirrors the BFS contract: a walk from no
// source the graph holds is an error, not an empty walk.
func TestExcessTraversalMissingSource(t *testing.T) {
	g, _, _, _, _ := excessTraversalGraph(t)
	tr := graph.NewExcessTraversal[uint64, float64]()
	tr.SetSources([]uint64{0xdeadbeef})
	if _, err := tr.Run(g); err == nil {
		t.Fatal("Run from a missing source = nil error, want ErrSourceNotFound")
	}
}
