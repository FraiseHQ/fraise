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
	"math"
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
)

// starGraph builds spokes 2..6 all pointing at hub 1.
func starGraph() *fakeGraph {
	return newFakeGraph([][2]uint64{{2, 1}, {3, 1}, {4, 1}, {5, 1}, {6, 1}})
}

func TestPageRankStar(t *testing.T) {
	g := starGraph()

	result, err := graph.NewPageRank[uint64, float64](0.85, 100, 1e-9).Run(g)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}

	r, ok := result.(graph.RankingResult[uint64, float64])
	if !ok {
		t.Fatalf("Run did not return a RankingResult")
	}
	scores := r.Scores
	if len(scores) != 6 {
		t.Fatalf("ranked %d vertices, want 6", len(scores))
	}

	var sum float64
	for _, s := range scores {
		sum += s
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Errorf("scores sum to %v, want 1", sum)
	}

	for id := uint64(2); id <= 6; id++ {
		if scores[1] <= scores[id] {
			t.Errorf("hub score %v not above spoke %d score %v", scores[1], id, scores[id])
		}
	}
}

func TestPageRankCycleIsUniform(t *testing.T) {
	g := newFakeGraph([][2]uint64{{1, 2}, {2, 3}, {3, 1}})

	res, err := graph.NewPageRank[uint64, float64](0.85, 100, 1e-9).Run(g)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	r, _ := res.(graph.RankingResult[uint64, float64])
	if len(r.Scores) != 3 {
		t.Fatalf("ranked %d vertices, want 3", len(r.Scores))
	}
	for id := uint64(1); id <= 3; id++ {
		if math.Abs(r.Scores[id]-1.0/3.0) > 1e-6 {
			t.Errorf("scores[%d] = %v, want 1/3 on a symmetric cycle", id, r.Scores[id])
		}
	}
}

func TestPageRankEmptyGraph(t *testing.T) {
	g := newFakeGraph(nil)
	if _, err := graph.NewPageRank[uint64, float64](0.85, 100, 1e-9).Run(g); err == nil {
		t.Errorf("Run on empty graph = nil error, want ErrEmptyGraph")
	}
}

// TestSearchWithPageRankRanking shows the ranking boost re-ordering results:
// the hub of a star out-ranks the direct text hit once PageRank is installed.
func TestSearchWithPageRankRanking(t *testing.T) {
	now := time.Now()

	build := func() *graph.InMemoryGraph[uint64, float64] {
		g := graph.NewGraph[uint64, float64](testConfig())
		h := g.GetHasher()

		// A star of five facts all mentioning the same hub entity.
		hub := &graph.NamedEntity[uint64]{NodeAttributes: graph.NodeAttributes{Value: "hub", Timestamp: now}, Hasher: h}
		if err := g.Set(hub); err != nil {
			t.Fatalf("Set(hub) = %v, want nil", err)
		}
		for _, v := range []string{"spoke fact two", "spoke fact three", "spoke fact four", "spoke fact five", "spoke fact six"} {
			fact := graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: v, Timestamp: now}, Hasher: h}
			if err := g.Set(fact); err != nil {
				t.Fatalf("Set(%q) = %v, want nil", v, err)
			}
			mentions := graph.Mentions[uint64]{Fact: &fact, NamedEntity: hub, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: h}
			if err := g.Set(mentions); err != nil {
				t.Fatalf("Set(mentions) = %v, want nil", err)
			}
		}

		// The seed fact is the direct text hit; it too mentions the hub.
		seed := graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: "alpha query", Timestamp: now}, Hasher: h}
		if err := g.Set(seed); err != nil {
			t.Fatalf("Set(seed) = %v, want nil", err)
		}
		mentions := graph.Mentions[uint64]{Fact: &seed, NamedEntity: hub, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: h}
		if err := g.Set(mentions); err != nil {
			t.Fatalf("Set(seed mentions) = %v, want nil", err)
		}
		return g
	}

	// Without a ranking the direct hit wins over the hub reached at hop 1.
	g := build()
	nodes, _ := g.Search([]string{"alpha"}, containers.Vector[float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 || (*nodes[0]).GetValue() != "alpha query" {
		t.Fatalf("Search without ranking = %d nodes, want the direct hit first", len(nodes))
	}

	// The hub concentrates the star's PageRank mass, so its boost lifts it
	// above the direct hit.
	g = build()
	g.SetRanking(graph.NewPageRank[uint64, float64](0.85, 100, 1e-9))
	nodes, _ = g.Search([]string{"alpha"}, containers.Vector[float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 || (*nodes[0]).GetValue() != "hub" {
		t.Errorf("Search with PageRank ranking did not put the hub first")
	}
}
