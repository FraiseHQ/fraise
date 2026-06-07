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
	return nil
}

func (g *InMemoryGraph[K, P]) Set(node *Node[K]) error {
	// err := d.currentSet(key, value)
	// if err != nil {
	// 	return err,
	// }
	// return errors
	return nil
}

func (g *InMemoryGraph[K, P]) Put(key K, node *Node[K]) error {

	return nil
}

func (g *InMemoryGraph[K, P]) Delete(node *Node[K]) error {
	return nil
}

func (g *InMemoryGraph[K, P]) AdjacencyMap() map[K]map[K]*Relationship[K] {

}

func (g *InMemoryGraph[K, P]) PredecessorMap() map[K]map[K]*Relationship[K] {

}

func (g *InMemoryGraph[K, P]) Copy() Graph[K, P] {

}

func (g *InMemoryGraph[K, P]) Order() int {

}

func (g *InMemoryGraph[K, P]) Size() int {

}

func (g *InMemoryGraph[K, P]) Stats() GraphStats {

}

func (g *InMemoryGraph[K, P]) Entities() []*Entity[K] {

}

func (g *InMemoryGraph[K, P]) Relationships() []*Relationship[K] {

}

// Returns the graph vector index
func (g *InMemoryGraph[K, P]) GetVectorIndex() *index.Index[K, containers.Vector[P], P] {

}

// Returns the graph full text search index
func (g *InMemoryGraph[K, P]) GetTextIndex() *index.Index[K, string, P] {

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

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})
	limit := top
	if limit > len(filtered) {
		limit = len(filtered)
	}

	return filtered[:limit]
}

func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[P]) ([]*Node[K], []P) {
	return nil, []P{}
}

func (g *InMemoryGraph[K, P]) findNeighbours(seeds []*Node[K], topics []string, entities []string, depth int) ([]*Node[K], []P) {
	return nil, []P{}
}

func (g *InMemoryGraph[K, P]) timeFilter(nodes []*Node[K], since time.Time, until time.Time) ([]*Node[K], []P) {
	return nil, []P{}
}

func (g *InMemoryGraph[K, P]) MergeFrom(in Graph[K, P]) {

}
