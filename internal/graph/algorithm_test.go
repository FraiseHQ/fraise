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
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/RonsenbergVI/fraise/internal/index"
)

// testConfig returns a ConfigSet carrying the flag defaults (applied by
// config.New) plus the vector-search parameters, which are otherwise only set
// during a full Parse. NewGraph reads those to size its RPTree index.
func testConfig() *config.ConfigSet {
	cfg := config.New()
	cfg.DB.VectorSearch.ProjectionDimention = 8
	cfg.DB.VectorSearch.NumberTrees = 4
	cfg.DB.VectorSearch.Seed = 4
	return cfg
}

// fakeGraph is a minimal Graph implementation used to unit-test the graph
// algorithms (BFS, PageRank) against arbitrary topologies that the typed
// Fact->Entity relationships cannot express (cycles, chains). Only the methods
// the algorithms touch — Get, AdjacencyMap and PredecessorMap — carry real
// behaviour; the rest are inert stubs.
type fakeGraph struct {
	vertices map[uint64]bool
	adj      map[uint64]map[uint64]uint64
	pred     map[uint64]map[uint64]uint64
}

// newFakeGraph builds a graph from directed edges (each [from, to]); every
// endpoint becomes a vertex, and extra isolated vertices can be added too.
func newFakeGraph(edges [][2]uint64, isolated ...uint64) *fakeGraph {
	g := &fakeGraph{
		vertices: make(map[uint64]bool),
		adj:      make(map[uint64]map[uint64]uint64),
		pred:     make(map[uint64]map[uint64]uint64),
	}
	edgeKey := uint64(1 << 32) // keep synthetic edge keys clear of vertex keys
	for _, e := range edges {
		from, to := e[0], e[1]
		g.vertices[from] = true
		g.vertices[to] = true
		if g.adj[from] == nil {
			g.adj[from] = make(map[uint64]uint64)
		}
		g.adj[from][to] = edgeKey
		if g.pred[to] == nil {
			g.pred[to] = make(map[uint64]uint64)
		}
		g.pred[to][from] = edgeKey
		edgeKey++
	}
	for _, v := range isolated {
		g.vertices[v] = true
	}
	return g
}

func (g *fakeGraph) Get(key uint64) graph.Node[uint64] {
	if g.vertices[key] {
		// A non-nil placeholder: the algorithms only test Get(source) != nil.
		return &graph.Fact[uint64]{}
	}
	return nil
}

func (g *fakeGraph) AdjacencyMap() map[uint64]map[uint64]uint64   { return g.adj }
func (g *fakeGraph) PredecessorMap() map[uint64]map[uint64]uint64 { return g.pred }

func (g *fakeGraph) Order() int { return len(g.vertices) }

func (g *fakeGraph) Size() int {
	n := 0
	for _, targets := range g.adj {
		n += len(targets)
	}
	return n
}

func (g *fakeGraph) Stats() graph.GraphStats {
	return graph.GraphStats{Order: g.Order(), Size: g.Size(), Nodes: len(g.vertices)}
}

// Remaining Graph methods are unused by the algorithms under test.
func (g *fakeGraph) GetHasher() hash.Hasher[uint64, string]             { return nil }
func (g *fakeGraph) Set(graph.Node[uint64]) error                       { return nil }
func (g *fakeGraph) Put(uint64, graph.Node[uint64]) error               { return nil }
func (g *fakeGraph) Delete(graph.Node[uint64]) error                    { return nil }
func (g *fakeGraph) GetVectorIndex() index.VectorIndex[uint64, float64] { return nil }
func (g *fakeGraph) GetTextIndex() index.TextIndex[uint64]              { return nil }
func (g *fakeGraph) MergeFrom(graph.Graph[uint64, float64])             {}
func (g *fakeGraph) Copy() graph.Graph[uint64, float64]                 { return g }
func (g *fakeGraph) Nodes() map[uint64]graph.Node[uint64]               { return nil }
func (g *fakeGraph) Search([]string, containers.Vector[float64], []string, []string, int, int, time.Time, time.Time) ([]*graph.Node[uint64], []float64) {
	return nil, nil
}
func (g *fakeGraph) RLock()   {}
func (g *fakeGraph) Lock()    {}
func (g *fakeGraph) RUnlock() {}
func (g *fakeGraph) Unlock()  {}
