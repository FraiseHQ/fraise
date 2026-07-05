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

package graph

import (
	"sort"
	"sync"
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/index"
)

type InMemoryGraph[K comparable, P float32 | float64] struct {
	idToNodes     map[K]Node[K]
	nodeToSources map[K]map[K]Relationship[K]
	nodeToTargets map[K]map[K]Relationship[K]

	mu sync.RWMutex
}

func NewGraph[K comparable, P float32 | float64]() *InMemoryGraph[K, P] {
	g := &InMemoryGraph[K, P]{
		idToNodes:     make(map[K]Node[K]),
		nodeToSources: make(map[K]map[K]Relationship[K]),
		nodeToTargets: make(map[K]map[K]Relationship[K]),
	}
	return g
}

// write lock
func (g *InMemoryGraph[K, P]) Lock() {
	g.mu.Lock()
}

// read lock
func (g *InMemoryGraph[K, P]) RLock() {
	g.mu.RLock()
}

// write unlock
func (g *InMemoryGraph[K, P]) Unlock() {
	g.mu.Unlock()
}

// read unlock
func (g *InMemoryGraph[K, P]) RUnlock() {
	g.mu.RUnlock()
}

func (g *InMemoryGraph[K, P]) Get(key K) *Node[K] {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Set(node *Node[K]) error {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Put(key K, node *Node[K]) error {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Delete(node *Node[K]) error {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) AdjacencyMap() map[K]map[K]*Relationship[K] {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) PredecessorMap() map[K]map[K]*Relationship[K] {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Copy() Graph[K, P] {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Order() int {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Size() int {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Stats() GraphStats {
	return GraphStats{}
}

func (g *InMemoryGraph[K, P]) Entities() []*Entity[K] {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Relationships() []*Relationship[K] {
	panic("not implemented")
}

// Returns the graph vector index
func (g *InMemoryGraph[K, P]) GetVectorIndex() index.VectorIndex[K, P] {
	panic("not implemented")
}

// Returns the graph full text search index
func (g *InMemoryGraph[K, P]) GetTextIndex() index.TextIndex[K] {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*Node[K], []P) {
	// A. Search starts with gathering seeds for the graph search.
	// Seeds are found from
	// 1. Vector search (top K - default = 10)
	// 2. Matching keywords
	seeds, scores := g.gatherSeeds(keywords, vector)

	// B. Walking the graph from all searchs and uinioning the found facts
	neighbors, scores := g.findNeighbours(seeds, topics, entities, depth)

	// C. Time filtered (since or until)

	filtered, scores := g.timeFilter(neighbors, since, until)

	// D. Truncate search results
	//
	// filtered and scores are parallel slices; sort an index permutation
	// by score so both stay aligned, then truncate to top.

	order := make([]int, len(filtered))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		return scores[order[i]] > scores[order[j]]
	})

	limit := top
	if limit > len(order) {
		limit = len(order)
	}

	nodes := make([]*Node[K], limit)
	ranked := make([]P, limit)
	for i := 0; i < limit; i++ {
		nodes[i] = filtered[order[i]]
		ranked[i] = scores[order[i]]
	}

	return nodes, ranked
}

func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[P]) ([]*Node[K], []P) {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) findNeighbours(seeds []*Node[K], topics []string, entities []string, depth int) ([]*Node[K], []P) {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) timeFilter(nodes []*Node[K], since time.Time, until time.Time) ([]*Node[K], []P) {
	panic("not implemented")
}

func (g *InMemoryGraph[K, P]) MergeFrom(in Graph[K, P]) {
	panic("not implemented")
}
