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

import "fmt"

// BFS is a breadth-first traversal. It explores the graph frontier by
// frontier, nearest vertices first, using a FIFO queue that starts with every
// source. It implements Traversal, and Run starts from the sources set by
// NewBFS or SetSources. Several sources grow one forest: each vertex is
// discovered once, along a shortest path from the nearest source.
type BFS[K comparable, P float32 | float64] struct {
	dir     Direction // which edges to follow
	sources []K       // vertices Run starts the traversal from
}

// NewBFS returns a breadth-first traversal that starts from source and follows
// edges in the given direction.
func NewBFS[K comparable, P float32 | float64](source K, dir Direction) *BFS[K, P] {
	return &BFS[K, P]{dir: dir, sources: []K{source}}
}

// NewBFSTraversal returns a breadth-first Traversal that follows edges in the
// given direction, with no source set. It serves as a graph's search
// traversal. The graph clones it for each walk and sets the walk's sources.
func NewBFSTraversal[K comparable, P float32 | float64](dir Direction) *BFS[K, P] {
	return &BFS[K, P]{dir: dir}
}

// Run traverses g from the configured sources and returns the result as an
// AlgorithmResult (a TraversalResult).
func (b *BFS[K, P]) Run(g Graph[K, P]) (AlgorithmResult, error) {
	result, err := b.traverse(g, b.sources)
	if err != nil {
		return nil, fmt.Errorf("BFS failed: %w", err)
	}
	return result, nil
}

// SetSources sets the vertices the next Run starts from.
func (b *BFS[K, P]) SetSources(sources []K) {
	b.sources = sources
}

// Clone implements [Traversal]. The clone keeps the direction and has no
// source set.
func (b *BFS[K, P]) Clone() Traversal[K, P] {
	return &BFS[K, P]{dir: b.dir}
}

// traverse walks g breadth-first from every source g holds, following edges
// in the configured direction. The sources sit at depth 0, in the order
// given.
func (b *BFS[K, P]) traverse(g Graph[K, P], sources []K) (TraversalResult[K], error) {
	result := TraversalResult[K]{
		Parent: make(map[K]K, len(sources)),
		Depth:  make(map[K]int, len(sources)),
	}
	queue := make([]K, 0, len(sources))
	for _, source := range sources {
		if _, dup := result.Depth[source]; dup || g.Get(source) == nil {
			continue
		}
		result.Order = append(result.Order, source)
		result.Parent[source] = source
		result.Depth[source] = 0
		queue = append(queue, source)
	}
	if len(queue) == 0 {
		return TraversalResult[K]{}, ErrSourceNotFound
	}

	adjacency := g.AdjacencyMap()
	predecessors := g.PredecessorMap()

	neighbours := func(u K) []K {
		var out []K
		if b.dir == Outgoing || b.dir == Both {
			for v := range adjacency[u] {
				out = append(out, v)
			}
		}
		if b.dir == Incoming || b.dir == Both {
			for v := range predecessors[u] {
				out = append(out, v)
			}
		}
		return out
	}

	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]

		for _, v := range neighbours(u) {
			if _, visited := result.Depth[v]; visited {
				continue
			}
			result.Order = append(result.Order, v)
			result.Parent[v] = u
			result.Depth[v] = result.Depth[u] + 1
			queue = append(queue, v)
		}
	}
	return result, nil
}
