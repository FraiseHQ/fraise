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
	"errors"
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
)

func newFact(id int, value string, ts time.Time) *graph.Fact[int] {
	return &graph.Fact[int]{ID: id, NodeAttributes: graph.NodeAttributes{Value: value, Timestamp: ts}}
}

func mustSet(t *testing.T, g *graph.InMemoryGraph[int, float64], n graph.Node[int]) {
	t.Helper()
	if err := g.Set(&n); err != nil {
		t.Fatalf("Set(%v) = %v, want nil", n.GetID(), err)
	}
}

func keysOf(nodes []*graph.Node[int]) []int {
	keys := make([]int, len(nodes))
	for i, n := range nodes {
		keys[i] = (*n).GetID()
	}
	return keys
}

func TestInMemoryGraphSetGetPutDelete(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()

	fact := newFact(1, "alice works at acme", now)
	mustSet(t, g, fact)

	if err := g.Set(func() *graph.Node[int] { var n graph.Node[int] = fact; return &n }()); !errors.Is(err, graph.ErrNodeAlreadyExists) {
		t.Errorf("Set on existing key = %v, want ErrNodeAlreadyExists", err)
	}
	if err := g.Set(nil); !errors.Is(err, graph.ErrNilNode) {
		t.Errorf("Set(nil) = %v, want ErrNilNode", err)
	}

	got := g.Get(1)
	if got == nil || (*got).GetID() != 1 {
		t.Fatalf("Get(1) = %v, want the stored node", got)
	}
	if g.Get(99) != nil {
		t.Errorf("Get(99) != nil, want nil for missing key")
	}

	var replacement graph.Node[int] = newFact(1, "alice moved to initech", now)
	if err := g.Put(1, &replacement); err != nil {
		t.Fatalf("Put = %v, want nil", err)
	}
	if got := g.Get(1); (*got).GetAttributes().Value != "alice moved to initech" {
		t.Errorf("Get(1) after Put = %q, want replaced value", (*got).GetAttributes().Value)
	}

	var node graph.Node[int] = newFact(1, "", now)
	if err := g.Delete(&node); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if g.Get(1) != nil {
		t.Errorf("Get(1) after Delete != nil, want nil")
	}
	if err := g.Delete(&node); !errors.Is(err, graph.ErrNodeNotFound) {
		t.Errorf("Delete on missing node = %v, want ErrNodeNotFound", err)
	}
}

func TestInMemoryGraphRelationships(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()

	fact := newFact(1, "alice works at acme", now)
	entity := &graph.NamedEntity[int]{ID: 10, NodeAttributes: graph.NodeAttributes{Value: "alice", Timestamp: now}}
	mustSet(t, g, fact)
	mustSet(t, g, entity)

	rel := &graph.Mentions[int]{Fact: fact, NamedEntity: entity}
	if err := g.AddRelationship(1, 10, rel); err != nil {
		t.Fatalf("AddRelationship = %v, want nil", err)
	}
	if err := g.AddRelationship(1, 99, rel); !errors.Is(err, graph.ErrNodeNotFound) {
		t.Errorf("AddRelationship to missing node = %v, want ErrNodeNotFound", err)
	}

	adj := g.AdjacencyMap()
	if adj[1][10] == nil {
		t.Errorf("AdjacencyMap()[1][10] = nil, want the relationship")
	}
	pred := g.PredecessorMap()
	if pred[10][1] == nil {
		t.Errorf("PredecessorMap()[10][1] = nil, want the relationship")
	}

	if got, want := g.Size(), 1; got != want {
		t.Errorf("Size() = %d, want %d", got, want)
	}
	if got, want := len(g.Relationships()), 1; got != want {
		t.Errorf("len(Relationships()) = %d, want %d", got, want)
	}
	if got, want := g.Order(), 2; got != want {
		t.Errorf("Order() = %d, want %d", got, want)
	}
	stats := g.Stats()
	if stats.Nodes != 2 || stats.Size != 1 || stats.Order != 2 {
		t.Errorf("Stats() = %+v, want {Order:2 Size:1 Nodes:2}", stats)
	}

	// Deleting an endpoint removes the incident edge from both maps.
	var node graph.Node[int] = entity
	if err := g.Delete(&node); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if got := g.Size(); got != 0 {
		t.Errorf("Size() after deleting endpoint = %d, want 0", got)
	}
	if adj := g.AdjacencyMap(); len(adj[1]) != 0 {
		t.Errorf("AdjacencyMap()[1] = %v, want empty after endpoint delete", adj[1])
	}
}

func TestInMemoryGraphIndexes(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()
	mustSet(t, g, newFact(1, "alice works at acme", now))

	// Set must index the node's value in the text index.
	keys, err := g.GetTextIndex().Search("acme")
	if err != nil || len(keys) != 1 || keys[0] != 1 {
		t.Errorf("text Search(acme) = (%v, %v), want ([1], nil)", keys, err)
	}

	// The vector index adopts the dimension of the first inserted embedding.
	if err := g.GetVectorIndex().Insert(1, containers.NewVector([]float64{1, 0, 0})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}
	got, err := g.GetVectorIndex().Search(containers.NewVector([]float64{1, 0, 0}), 1)
	if err != nil || len(got) != 1 || got[0] != 1 {
		t.Errorf("vector Search = (%v, %v), want ([1], nil)", got, err)
	}

	// Deleting the node clears both indexes.
	var node graph.Node[int] = newFact(1, "", now)
	if err := g.Delete(&node); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if keys, err := g.GetTextIndex().Search("acme"); err == nil && len(keys) != 0 {
		t.Errorf("text Search(acme) after delete = %v, want no hits", keys)
	}
	if _, err := g.GetVectorIndex().Retrieve(1); err == nil {
		t.Errorf("vector Retrieve(1) after delete succeeded, want error")
	}
}

func TestInMemoryGraphSearchByKeywords(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()

	fact1 := newFact(1, "alice works at acme", now)
	fact2 := newFact(2, "bob plays tennis", now)
	entity := &graph.NamedEntity[int]{ID: 10, NodeAttributes: graph.NodeAttributes{Value: "alice", Timestamp: now}}
	mustSet(t, g, fact1)
	mustSet(t, g, fact2)
	mustSet(t, g, entity)
	if err := g.AddRelationship(1, 10, &graph.Mentions[int]{Fact: fact1, NamedEntity: entity}); err != nil {
		t.Fatalf("AddRelationship = %v, want nil", err)
	}

	nodes, scores := g.Search([]string{"acme"}, containers.Vector[float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetID() != 1 {
		t.Fatalf("Search(acme) = %v, want [1]", keysOf(nodes))
	}
	if len(scores) != 1 || scores[0] <= 0 {
		t.Errorf("Search(acme) scores = %v, want one positive score", scores)
	}

	// With depth 1 the mentioned entity is pulled in, at a lower score.
	nodes, scores = g.Search([]string{"acme"}, containers.Vector[float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 {
		t.Fatalf("Search(acme, depth=1) = %v, want 2 nodes", keysOf(nodes))
	}
	if (*nodes[0]).GetID() != 1 {
		t.Errorf("Search(acme, depth=1) best hit = %d, want the direct hit 1", (*nodes[0]).GetID())
	}
	if scores[1] >= scores[0] {
		t.Errorf("neighbour score %v not attenuated below seed score %v", scores[1], scores[0])
	}
}

func TestInMemoryGraphSearchByVector(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()
	mustSet(t, g, newFact(1, "alpha", now))
	mustSet(t, g, newFact(2, "beta", now))

	if err := g.GetVectorIndex().Insert(1, containers.NewVector([]float64{1, 0})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}
	if err := g.GetVectorIndex().Insert(2, containers.NewVector([]float64{0, 1})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}

	nodes, _ := g.Search(nil, containers.NewVector([]float64{0.9, 0.1}), nil, nil, 0, 1, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetID() != 1 {
		t.Errorf("Search(vector near 1) = %v, want [1]", keysOf(nodes))
	}
}

func TestInMemoryGraphSearchTopicFilter(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()

	fact1 := newFact(1, "alice works at acme", now)
	fact2 := newFact(2, "alice plays tennis", now)
	topic := &graph.Topic[int]{ID: 20, NodeAttributes: graph.NodeAttributes{Value: "work", Timestamp: now}}
	mustSet(t, g, fact1)
	mustSet(t, g, fact2)
	mustSet(t, g, topic)
	if err := g.AddRelationship(1, 20, &graph.IsAbout[int]{Fact: fact1, Topic: topic}); err != nil {
		t.Fatalf("AddRelationship = %v, want nil", err)
	}

	// Both facts match "alice", but only fact1 is tagged with topic "work".
	nodes, _ := g.Search([]string{"alice"}, containers.Vector[float64]{}, []string{"work"}, nil, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetID() != 1 {
		t.Errorf("Search(alice, topic=work) = %v, want [1]", keysOf(nodes))
	}
}

func TestInMemoryGraphSearchTimeFilter(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mustSet(t, g, newFact(1, "alice ancient fact", old))
	mustSet(t, g, newFact(2, "alice recent fact", recent))

	nodes, _ := g.Search([]string{"alice"}, containers.Vector[float64]{}, nil, nil, 0, 10, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetID() != 2 {
		t.Errorf("Search(alice, since=2025) = %v, want [2]", keysOf(nodes))
	}

	nodes, _ = g.Search([]string{"alice"}, containers.Vector[float64]{}, nil, nil, 0, 10, time.Time{}, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(nodes) != 1 || (*nodes[0]).GetID() != 1 {
		t.Errorf("Search(alice, until=2025) = %v, want [1]", keysOf(nodes))
	}
}

func TestInMemoryGraphSearchTopTruncation(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()
	mustSet(t, g, newFact(1, "shared term one", now))
	mustSet(t, g, newFact(2, "shared term two", now))
	mustSet(t, g, newFact(3, "shared term three", now))

	nodes, scores := g.Search([]string{"shared"}, containers.Vector[float64]{}, nil, nil, 0, 2, time.Time{}, time.Time{})
	if len(nodes) != 2 || len(scores) != 2 {
		t.Errorf("Search(top=2) returned %d nodes and %d scores, want 2 and 2", len(nodes), len(scores))
	}
	if scores[0] < scores[1] {
		t.Errorf("scores not descending: %v", scores)
	}
}

func TestInMemoryGraphCopyIsIndependent(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()
	fact := newFact(1, "alice works at acme", now)
	entity := &graph.NamedEntity[int]{ID: 10, NodeAttributes: graph.NodeAttributes{Value: "alice", Timestamp: now}}
	mustSet(t, g, fact)
	mustSet(t, g, entity)
	if err := g.AddRelationship(1, 10, &graph.Mentions[int]{Fact: fact, NamedEntity: entity}); err != nil {
		t.Fatalf("AddRelationship = %v, want nil", err)
	}
	if err := g.GetVectorIndex().Insert(1, containers.NewVector([]float64{1, 2})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}

	clone := g.Copy()
	if clone.Stats() != g.Stats() {
		t.Fatalf("Copy() stats = %+v, want %+v", clone.Stats(), g.Stats())
	}

	// Mutating the copy must not touch the original.
	var node graph.Node[int] = fact
	if err := clone.Delete(&node); err != nil {
		t.Fatalf("Delete on copy = %v, want nil", err)
	}
	if g.Get(1) == nil {
		t.Errorf("deleting from the copy removed node 1 from the original")
	}
	if got := g.Size(); got != 1 {
		t.Errorf("original Size() = %d after mutating copy, want 1", got)
	}
	if _, err := g.GetVectorIndex().Retrieve(1); err != nil {
		t.Errorf("original vector entry lost after mutating copy: %v", err)
	}
}

func TestInMemoryGraphMergeFrom(t *testing.T) {
	now := time.Now()

	a := graph.NewGraph[int, float64]()
	mustSet(t, a, newFact(1, "alice works at acme", now))

	b := graph.NewGraph[int, float64]()
	fact2 := newFact(2, "bob plays tennis", now)
	entity := &graph.NamedEntity[int]{ID: 10, NodeAttributes: graph.NodeAttributes{Value: "bob", Timestamp: now}}
	mustSet(t, b, fact2)
	mustSet(t, b, entity)
	if err := b.AddRelationship(2, 10, &graph.Mentions[int]{Fact: fact2, NamedEntity: entity}); err != nil {
		t.Fatalf("AddRelationship = %v, want nil", err)
	}
	if err := b.GetVectorIndex().Insert(2, containers.NewVector([]float64{3, 4})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}

	a.MergeFrom(b)

	if got := a.Stats(); got.Nodes != 3 || got.Size != 1 {
		t.Errorf("Stats() after merge = %+v, want {Nodes:3 Size:1}", got)
	}
	if a.Get(2) == nil || a.Get(10) == nil {
		t.Errorf("merged nodes missing: Get(2)=%v Get(10)=%v", a.Get(2), a.Get(10))
	}
	if keys, err := a.GetTextIndex().Search("tennis"); err != nil || len(keys) != 1 || keys[0] != 2 {
		t.Errorf("text Search(tennis) after merge = (%v, %v), want ([2], nil)", keys, err)
	}
	if _, err := a.GetVectorIndex().Retrieve(2); err != nil {
		t.Errorf("vector Retrieve(2) after merge = %v, want nil", err)
	}
}
