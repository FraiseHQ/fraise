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
func chainGraph() *fakeGraph {
	return newFakeGraph([][2]uint64{{1, 2}, {2, 3}}, 4)
}

func TestBFSOutgoing(t *testing.T) {
	g := chainGraph()

	result, err := graph.NewBFS[uint64, float64](1, graph.Outgoing).Run(g)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}

	r, ok := result.(graph.TraversalResult[uint64])
	if !ok {
		t.Fatalf("Run did not return a TraversalResult")
	}

	wantDepth := map[uint64]int{1: 0, 2: 1, 3: 2}
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
	g := chainGraph()

	result, err := graph.NewBFS[uint64, float64](3, graph.Incoming).Run(g)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}

	r, ok := result.(graph.TraversalResult[uint64])
	if !ok {
		t.Fatalf("Run did not return a TraversalResult")
	}
	wantDepth := map[uint64]int{3: 0, 2: 1, 1: 2}
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
	g := chainGraph()

	// Following only outgoing edges from the chain's tail reaches nothing.
	result, err := graph.NewBFS[uint64, float64](3, graph.Outgoing).Run(g)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	r, _ := result.(graph.TraversalResult[uint64])
	if len(r.Order) != 1 {
		t.Errorf("Outgoing from tail visited %v, want only the source", r.Order)
	}

	// Both directions from the middle reach the whole chain.
	result, err = graph.NewBFS[uint64, float64](2, graph.Both).Run(g)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	r, _ = result.(graph.TraversalResult[uint64])
	if len(r.Order) != 3 {
		t.Errorf("Both from middle visited %v, want the whole chain", r.Order)
	}
}

func TestBFSMissingSource(t *testing.T) {
	g := chainGraph()
	if _, err := graph.NewBFS[uint64, float64](99, graph.Both).Run(g); err == nil {
		t.Errorf("Run from missing source = nil error, want ErrSourceNotFound")
	}
}

func TestBFSTraverse(t *testing.T) {
	g := chainGraph()
	traversal := graph.NewBFSTraversal[uint64, float64](graph.Outgoing)

	traversal.SetSource(1)
	result, err := traversal.Run(g)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}

	r, _ := result.(graph.TraversalResult[uint64])
	want := map[uint64]int{1: 0, 2: 1, 3: 2}
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
		t.Errorf("Run from missing source = nil error, want ErrSourceNotFound")
	}
}

// TestSearchWithConfiguredTraversal shows the traversal seam changing what
// Search returns. BFS remains available as a tree-shaped strategy: it
// observes the same anchors as the excess traversal but discovers each member
// through a single parent, and the collection layer accepts both shapes. The
// storm fixture's silent cluster member is funded under either installed
// strategy; with no traversal at all the graph channel is off and only text
// matches surface.
func TestSearchWithConfiguredTraversal(t *testing.T) {
	find := func(values []string, want string) bool {
		for _, v := range values {
			if v == want {
				return true
			}
		}
		return false
	}

	// No traversal: text-only, the silent member cannot surface. NewGraph
	// installs none — the traversal arrives from configuration at db.Start —
	// so a bare graph is exactly the channel-off case.
	cfg := testConfig()
	cfg.Engine.Halflife = 0
	bare := graph.NewGraph[uint64, float64](cfg)
	calm, _ := stormGraph(t, bare)
	nodes, _, _, _ := bare.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, nil, nil, 1, 20, time.Time{}, time.Time{})
	if find(values(nodes), calm) {
		t.Errorf("Search with no traversal surfaced %q — the graph channel should be off", calm)
	}

	// BFS installed: tree-shaped incidence still observes the weather
	// cluster and funds its silent member.
	g := noDecayGraph()
	g.SetTraversal(graph.NewBFSTraversal[uint64, float64](graph.Both))
	calm, _ = stormGraph(t, g)
	nodes, _, _, _ = g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, nil, nil, 1, 20, time.Time{}, time.Time{})
	if !find(values(nodes), calm) {
		t.Errorf("Search with BFS installed did not fund %q — a tree traversal still observes anchors", calm)
	}
}
