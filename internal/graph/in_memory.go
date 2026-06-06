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
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
)

type InMemoryGraph[K comparable, P float32 | float64] struct {
	idToNodes     map[K]Node[K]
	nodeToSources map[K]map[K]Relationship[K]
	nodeToTargets map[K]map[K]Relationship[K]
}

func (g *InMemoryGraph[K, P]) Get(key K) *Node[K] {
	return nil
}

func (d *InMemoryGraph[K, P]) Set(node *Node[K]) error {
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

func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[P], topics []string, entities []string, depth int, since time.Time, until time.Time, top int) ([]*Node[K], []P) {
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

func (d *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[P]) ([]*Node[K], []P) {
	return nil, []P{}
}

func (d *InMemoryGraph[K, P]) findNeighbours(seeds []*Node[K], topics []string, entities []string, depth int) ([]*Node[K], []P) {
	return nil, []P{}
}

func (d *InMemoryGraph[K, P]) timeFilter(nodes []*Node[K], since time.Time, until time.Time) ([]*Node[K], []P) {
	return nil, []P{}
}
