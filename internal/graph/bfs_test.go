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
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
)

// chainGraph builds 1 -> 2 -> 3 plus the isolated node 4.
func chainGraph(t *testing.T) *graph.InMemoryGraph[int, float64] {
	t.Helper()
	g := graph.NewGraph[int, float64]()
	now := time.Now()

	facts := make(map[int]*graph.Fact[int])
	for id := 1; id <= 4; id++ {
		facts[id] = &graph.Fact[int]{ID: id, NodeAttributes: graph.NodeAttributes{Value: "fact", Timestamp: now}}
		var n graph.Node[int] = facts[id]
		if err := g.Set(&n); err != nil {
			t.Fatalf("Set(%d) = %v, want nil", id, err)
		}
	}
	for _, edge := range [][2]int{{1, 2}, {2, 3}} {
		rel := &graph.Mentions[int]{Fact: facts[edge[0]]}
		if err := g.AddRelationship(edge[0], edge[1], rel); err != nil {
			t.Fatalf("AddRelationship(%v) = %v, want nil", edge, err)
		}
	}
	return g
}

func TestBFSOutgoing(t *testing.T) {
	g := chainGraph(t)

	result, _ := graph.NewBFS[int, float64](1, graph.Outgoing).Run(g)

	r, ok := result.(graph.TraversalResult[int])

	if !ok {
		t.Fatalf("Run did not return a TraversalResult")
	}

	wantDepth := map[int]int{1: 0, 2: 1, 3: 2}
	if len(r.Depth) != len(wantDepth) {
		t.Fatalf("Depth = %v, want %v", r.Depth, wantDepth)
	}
	for key, depth := range wantDepth {
		if r.Depth[key] != depth {
			t.Errorf("Depth[%d] = %d, want %d", key, r.Depth[key], depth)
		}
	}
	if len(r.Order) != 3 || r.Order[0] != 1 {
		t.Errorf("Order = %v, want a 3-vertex order starting at the source", r.Order)
	}
	if r.Parent[1] != 1 || r.Parent[2] != 1 || r.Parent[3] != 2 {
		t.Errorf("Parent = %v, want {1:1, 2:1, 3:2}", r.Parent)
	}
}

func TestBFSIncoming(t *testing.T) {
	g := chainGraph(t)

	result, _ := graph.NewBFS[int, float64](3, graph.Incoming).Run(g)

	r, ok := result.(graph.TraversalResult[int])

	if !ok {
		t.Fatalf("Run did not return a TraversalResult")
	}
	wantDepth := map[int]int{3: 0, 2: 1, 1: 2}
	for key, depth := range wantDepth {
		if r.Depth[key] != depth {
			t.Errorf("Depth[%d] = %d, want %d", key, r.Depth[key], depth)
		}
	}
	if len(r.Depth) != len(wantDepth) {
		t.Errorf("Depth = %v, want exactly %v", r.Depth, wantDepth)
	}
}

func TestBFSDirectionLimitsReach(t *testing.T) {
	g := chainGraph(t)

	// Following only outgoing edges from the chain's tail reaches nothing.
	result, _ := graph.NewBFS[int, float64](3, graph.Outgoing).Run(g)

	r, _ := result.(graph.TraversalResult[int])

	if len(r.Order) != 1 {
		t.Errorf("Outgoing from tail visited %v, want only the source", r.Order)
	}

	// Both directions from the middle reach the whole chain.
	result, _ = graph.NewBFS[int, float64](2, graph.Both).Run(g)

	r, _ = result.(graph.TraversalResult[int])

	if len(r.Order) != 3 {
		t.Errorf("Both from middle visited %v, want the whole chain", r.Order)
	}
}

func TestBFSMissingSource(t *testing.T) {
	g := chainGraph(t)
	result, err := graph.NewBFS[int, float64](99, graph.Both).Run(g)

	r, _ := result.(graph.TraversalResult[int])

	if err != nil {
		t.Fatalf("Run did not return a TraversalResult")
	}
	if len(r.Order) != 0 {
		t.Errorf("Run from missing source visited %v, want nothing", r.Order)
	}
}

func TestBFSTraverse(t *testing.T) {
	g := chainGraph(t)
	traversal := graph.NewBFSTraversal[int, float64](graph.Outgoing)

	traversal.SetSource(1)
	result, err := traversal.Run(g)

	r, _ := result.(graph.TraversalResult[int])

	if err != nil {
		t.Fatalf("Traverse = %v, want nil", err)
	}
	want := map[int]int{1: 0, 2: 1, 3: 2}
	if len(r.Depth) != len(want) {
		t.Fatalf("Traverse depths = %v, want %v", r.Depth, want)
	}
	for key, depth := range want {
		if r.Depth[key] != depth {
			t.Errorf("Depth[%d] = %d, want %d", key, r.Depth[key], depth)
		}
	}

	traversal.SetSource(99)
	if _, err := traversal.Run(g); err == nil {
		t.Errorf("Traverse from missing source = nil error, want ErrSourceNotFound")
	}
}

// TestSearchWithConfiguredTraversal shows the traversal algorithm changing
// what Search returns: with an outgoing-only BFS a seed with no outgoing
// edges expands to nothing, while the default (both directions) pulls in the
// fact pointing at it.
func TestSearchWithConfiguredTraversal(t *testing.T) {
	build := func() *graph.InMemoryGraph[int, float64] {
		g := graph.NewGraph[int, float64]()
		now := time.Now()
		fact := &graph.Fact[int]{ID: 1, NodeAttributes: graph.NodeAttributes{Value: "acme makes things", Timestamp: now}}
		entity := &graph.NamedEntity[int]{ID: 10, NodeAttributes: graph.NodeAttributes{Value: "gizmo", Timestamp: now}}
		for _, n := range []graph.Node[int]{fact, entity} {
			n := n
			if err := g.Set(&n); err != nil {
				t.Fatalf("Set = %v, want nil", err)
			}
		}
		if err := g.AddRelationship(1, 10, &graph.Mentions[int]{Fact: fact, NamedEntity: entity}); err != nil {
			t.Fatalf("AddRelationship = %v, want nil", err)
		}
		return g
	}

	// The default traversal follows both directions: the fact is one hop away.
	g := build()
	nodes, _ := g.Search([]string{"gizmo"}, containers.Vector[float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 {
		t.Errorf("Search with default traversal returned %d nodes, want 2", len(nodes))
	}

	// Outgoing-only BFS: the entity has no outgoing edges, nothing joins.
	g = build()
	g.SetTraversal(graph.NewBFSTraversal[int, float64](graph.Outgoing))
	nodes, _ = g.Search([]string{"gizmo"}, containers.Vector[float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetID() != 10 {
		t.Errorf("Search with outgoing BFS returned %v nodes, want just the seed entity", len(nodes))
	}
}
