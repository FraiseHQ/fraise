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
func starGraph(t *testing.T) *graph.InMemoryGraph[int, float64] {
	t.Helper()
	g := graph.NewGraph[int, float64]()
	now := time.Now()

	facts := make(map[int]*graph.Fact[int])
	for id := 1; id <= 6; id++ {
		facts[id] = &graph.Fact[int]{ID: id, NodeAttributes: graph.NodeAttributes{Value: "fact", Timestamp: now}}
		var n graph.Node[int] = facts[id]
		if err := g.Set(&n); err != nil {
			t.Fatalf("Set(%d) = %v, want nil", id, err)
		}
	}
	for id := 2; id <= 6; id++ {
		rel := &graph.Mentions[int]{Fact: facts[id]}
		if err := g.AddRelationship(id, 1, rel); err != nil {
			t.Fatalf("AddRelationship(%d->1) = %v, want nil", id, err)
		}
	}
	return g
}

func TestPageRankStar(t *testing.T) {
	g := starGraph(t)

	result, _ := graph.NewPageRank[int, float64](0.85, 100, 1e-9).Run(g)

	r, ok := result.(graph.RankingResult[int, float64])

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

	for id := 2; id <= 6; id++ {
		if scores[1] <= scores[id] {
			t.Errorf("hub score %v not above spoke %d score %v", scores[1], id, scores[id])
		}
	}
}

func TestPageRankCycleIsUniform(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	now := time.Now()
	facts := make(map[int]*graph.Fact[int])
	for id := 1; id <= 3; id++ {
		facts[id] = &graph.Fact[int]{ID: id, NodeAttributes: graph.NodeAttributes{Value: "fact", Timestamp: now}}
		var n graph.Node[int] = facts[id]
		if err := g.Set(&n); err != nil {
			t.Fatalf("Set(%d) = %v, want nil", id, err)
		}
	}
	for _, edge := range [][2]int{{1, 2}, {2, 3}, {3, 1}} {
		if err := g.AddRelationship(edge[0], edge[1], &graph.Mentions[int]{Fact: facts[edge[0]]}); err != nil {
			t.Fatalf("AddRelationship(%v) = %v, want nil", edge, err)
		}
	}

	res, err := graph.NewPageRank[int, float64](0.85, 100, 1e-9).Run(g)
	r, _ := res.(graph.RankingResult[int, float64])

	if err != nil {
		t.Fatalf("Rank = %v, want nil", err)
	}
	if len(r.Scores) != 3 {
		t.Fatalf("ranked %d vertices, want 3", len(r.Scores))
	}
	for id := 1; id <= 3; id++ {
		if math.Abs(r.Scores[id]-1.0/3.0) > 1e-6 {
			t.Errorf("scores[%d] = %v, want 1/3 on a symmetric cycle", id, r.Scores[id])
		}
	}
}

func TestPageRankEmptyGraph(t *testing.T) {
	g := graph.NewGraph[int, float64]()
	if _, err := graph.NewPageRank[int, float64](0.85, 100, 1e-9).Run(g); err == nil {
		t.Errorf("Rank on empty graph = nil error, want ErrEmptyGraph")
	}
}

// TestSearchWithPageRankRanking shows the ranking boost re-ordering results:
// the hub of a star out-ranks the direct text hit once PageRank is installed.
func TestSearchWithPageRankRanking(t *testing.T) {
	g := starGraph(t)
	now := time.Now()
	seed := &graph.Fact[int]{ID: 7, NodeAttributes: graph.NodeAttributes{Value: "alpha query", Timestamp: now}}
	var n graph.Node[int] = seed
	if err := g.Set(&n); err != nil {
		t.Fatalf("Set = %v, want nil", err)
	}
	if err := g.AddRelationship(7, 1, &graph.Mentions[int]{Fact: seed}); err != nil {
		t.Fatalf("AddRelationship = %v, want nil", err)
	}

	// Without a ranking the direct hit wins over the hub reached at hop 1.
	nodes, _ := g.Search([]string{"alpha"}, containers.Vector[float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 || (*nodes[0]).GetID() != 7 {
		t.Fatalf("Search without ranking = %v, want the direct hit 7 first", (*nodes[0]).GetID())
	}

	// The hub concentrates the star's PageRank mass, so its boost lifts it
	// above the direct hit.
	g.SetRanking(graph.NewPageRank[int, float64](0.85, 100, 1e-9))
	nodes, _ = g.Search([]string{"alpha"}, containers.Vector[float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 || (*nodes[0]).GetID() != 1 {
		t.Errorf("Search with PageRank ranking put %d first, want the hub 1", (*nodes[0]).GetID())
	}
}
