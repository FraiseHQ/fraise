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
	"github.com/RonsenbergVI/fraise/internal/hash"
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

// factLink is a test-only fact->fact relationship. Production edges only run
// fact->tag (Mentions, IsAbout), which gives facts no in-links and therefore
// near-uniform PageRank; linking facts directly lets the test build a star
// whose hub is itself a fact — the only node kind Search returns as a hit.
type factLink struct {
	src, dst *graph.Fact[uint64]
	graph.NodeAttributes
	h hash.Hasher[uint64, string]
}

func (l factLink) Key() uint64                          { return l.Hash(l.h) }
func (l factLink) GetValue() string                     { return l.Value }
func (l factLink) GetTimestamp() time.Time              { return l.Timestamp }
func (l factLink) GetAttributes() *graph.NodeAttributes { return &l.NodeAttributes }
func (l factLink) Hash(h hash.Hasher[uint64, string]) uint64 {
	return h.Hash("link:" + l.src.Value + "\x00" + l.dst.Value)
}
func (l factLink) Source() *graph.Entity[uint64] { var e graph.Entity[uint64] = l.src; return &e }
func (l factLink) Target() *graph.Entity[uint64] { var e graph.Entity[uint64] = l.dst; return &e }

// TestSearchWithPageRankRanking pins the boost stage: the installed ranking
// multiplies relevance by graph centrality. Two facts match the query with
// bit-identical text mass (same term frequency, same length, decay off), so
// without a ranking only the key tiebreak orders them; with PageRank
// installed, the fact the star concentrates its mass on comes out first
// regardless of its key.
func TestSearchWithPageRankRanking(t *testing.T) {
	now := time.Now()

	build := func() *graph.InMemoryGraph[uint64, float64] {
		cfg := testConfig()
		cfg.Engine.Halflife = 0
		g := graph.NewGraph[uint64, float64](cfg)
		h := g.GetHasher()

		plain := graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: "alpha plain note", Timestamp: now}, Hasher: h}
		if err := g.Set(plain); err != nil {
			t.Fatalf("Set(plain) = %v, want nil", err)
		}
		central := graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: "alpha central note", Timestamp: now}, Hasher: h}
		if err := g.Set(central); err != nil {
			t.Fatalf("Set(central) = %v, want nil", err)
		}

		// A star of spokes all linking to the central fact concentrates the
		// graph's PageRank mass on it.
		for _, v := range []string{"spoke fact two", "spoke fact three", "spoke fact four", "spoke fact five", "spoke fact six"} {
			fact := graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: v, Timestamp: now}, Hasher: h}
			if err := g.Set(fact); err != nil {
				t.Fatalf("Set(%q) = %v, want nil", v, err)
			}
			if err := g.Set(factLink{src: &fact, dst: &central, NodeAttributes: graph.NodeAttributes{Timestamp: now}, h: h}); err != nil {
				t.Fatalf("Set(link) = %v, want nil", err)
			}
		}
		return g
	}

	// Without a ranking the two matches tie to the bit: same term frequency,
	// same document length, no decay — nothing separates them.
	g := build()
	nodes, scores, _, _ := g.Search([]string{"alpha"}, containers.Vector[uint64, float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 || scores[0] != scores[1] {
		t.Fatalf("Search without ranking = %d nodes, scores %v, want the two matches tied", len(nodes), scores)
	}

	// The central fact concentrates the star's PageRank mass, so its boost
	// lifts it above the equally-relevant plain fact.
	g = build()
	g.SetRanking(graph.NewPageRank[uint64, float64](0.85, 100, 1e-9))
	nodes, _, _, _ = g.Search([]string{"alpha"}, containers.Vector[uint64, float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 || (*nodes[0]).GetValue() != "alpha central note" {
		t.Errorf("Search with PageRank ranking did not put the central fact first")
	}
}
