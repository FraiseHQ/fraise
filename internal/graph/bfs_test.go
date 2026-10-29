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

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
	"github.com/google/go-cmp/cmp"
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

	traversal.SetSources([]uint64{1})
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

	traversal.SetSources([]uint64{99})
	if _, err := traversal.Run(g); err == nil {
		t.Errorf("Run from missing source = nil error, want ErrSourceNotFound")
	}
}

// TestBFSFromSeveralSources pins a walk from a source set. The sources share
// one frontier, so a vertex is discovered once, from the first source in the
// queue that reaches it, and its depth is its distance to the nearest
// source. A source the graph does not hold is skipped rather than failing a
// walk that has others to start from.
func TestBFSFromSeveralSources(t *testing.T) {
	cases := map[string]struct {
		sources    []uint64
		dir        graph.Direction
		wantDepth  map[uint64]int
		wantParent map[uint64]uint64
	}{
		"both ends of the chain meet in the middle": {
			sources:    []uint64{1, 3},
			dir:        graph.Both,
			wantDepth:  map[uint64]int{1: 0, 3: 0, 2: 1},
			wantParent: map[uint64]uint64{1: 1, 3: 3, 2: 1},
		},
		"the queue order decides who discovers a shared vertex": {
			sources:    []uint64{3, 1},
			dir:        graph.Both,
			wantDepth:  map[uint64]int{3: 0, 1: 0, 2: 1},
			wantParent: map[uint64]uint64{3: 3, 1: 1, 2: 3},
		},
		"a missing source is skipped": {
			sources:    []uint64{99, 1},
			dir:        graph.Outgoing,
			wantDepth:  map[uint64]int{1: 0, 2: 1, 3: 2},
			wantParent: map[uint64]uint64{1: 1, 2: 1, 3: 2},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			traversal := graph.NewBFSTraversal[uint64, float64](tc.dir)
			traversal.SetSources(tc.sources)
			result, err := traversal.Run(chainGraph())
			if err != nil {
				t.Fatalf("Run = %v, want nil", err)
			}
			got := result.(graph.TraversalResult[uint64])
			if diff := cmp.Diff(tc.wantDepth, got.Depth); diff != "" {
				t.Errorf("Depth mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantParent, got.Parent); diff != "" {
				t.Errorf("Parent mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSearchWithConfiguredTraversal shows the installed traversal changing
// what Search returns. BFS is tree-shaped: it observes the same anchors as
// ExcessTraversal, but reaches each member through a single parent. The
// collection layer accepts both shapes, so with BFS installed the storm
// fixture's silent cluster member is funded. With no traversal the graph
// channel is off and only text matches surface.
func TestSearchWithConfiguredTraversal(t *testing.T) {
	find := func(values []string, want string) bool {
		for _, v := range values {
			if v == want {
				return true
			}
		}
		return false
	}

	// No traversal. NewGraph installs none (db.Start installs one from
	// configuration), so on a bare graph the graph channel is off and the
	// silent member cannot surface. Both searches name the fixture's topics,
	// because the traversal runs only through a named anchor. Only the
	// traversal differs between them.
	cfg := testConfig()
	cfg.Engine.Halflife = 0
	bare := graph.NewGraph[uint64, float64](cfg)
	calm, _ := stormGraph(t, bare)
	nodes, _, _, _, _ := search(bare, []string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"weather", "archive"}, nil, 2, 20, time.Time{}, time.Time{})
	if find(values(nodes), calm) {
		t.Errorf("Search with no traversal surfaced %q — the graph channel should be off", calm)
	}

	// BFS installed. Tree-shaped incidence still observes the weather
	// cluster and funds its silent member.
	g := noDecayGraph()
	g.SetTraversal(graph.NewBFSTraversal[uint64, float64](graph.Both))
	calm, _ = stormGraph(t, g)
	nodes, _, _, _, _ = search(g, []string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"weather", "archive"}, nil, 2, 20, time.Time{}, time.Time{})
	if !find(values(nodes), calm) {
		t.Errorf("Search with BFS installed did not fund %q — a tree traversal still observes anchors", calm)
	}
}

// TestSearchFundsEveryMemberOfATransmittingAnchor pins that the traversal's
// shape does not decide which members an anchor reaches. Every fact filed
// under a transmitting anchor gets one SrcGraph contribution through it,
// seeds included. A walk that set out from the seeds would put the seed that
// discovered the anchor at depth 0 rather than among its members, and under
// BFS "storm clouds gather at sea" would go unfunded while its cluster
// siblings were funded. Expanding from the transmitting anchors puts every
// member at depth 1, whichever traversal is installed.
func TestSearchFundsEveryMemberOfATransmittingAnchor(t *testing.T) {
	cases := map[string]graph.Traversal[uint64, float64]{
		"excess": graph.NewExcessTraversal[uint64, float64](),
		"bfs":    graph.NewBFSTraversal[uint64, float64](graph.Both),
	}
	for name, traversal := range cases {
		t.Run(name, func(t *testing.T) {
			g := noDecayGraph()
			g.SetTraversal(traversal)
			calm, _ := stormGraph(t, g)
			weather := mkTopic(g, "weather", time.Now()).Key()

			nodes, _, contributions, _, _ := search(g, []string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"weather", "archive"}, nil, 2, 20, time.Time{}, time.Time{})
			got := make(map[string]int)
			for i, node := range nodes {
				for _, c := range contributions[i] {
					if c.Src == scoring.SrcGraph && c.Via == weather {
						got[node.GetValue()]++
					}
				}
			}
			want := map[string]int{
				"the barometer falls before the storm": 1,
				"storm clouds gather at sea":           1,
				calm:                                   1,
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("weather contributions per member mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
