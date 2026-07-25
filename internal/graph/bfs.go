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

// BFS is a breadth-first traversal: it explores the graph frontier by frontier
// (nearest vertices first) using a FIFO queue. It implements Traversal;
// Run starts from the source configured at construction, while Traverse takes
// the source as a parameter so one BFS value can serve many seeds.
type BFS[K comparable, P float32 | float64] struct {
	dir    Direction // which edges to follow
	source K         // vertex Run starts the traversal from
}

// NewBFS returns a breadth-first traversal that starts from source and follows
// edges in the given direction.
func NewBFS[K comparable, P float32 | float64](source K, dir Direction) *BFS[K, P] {
	return &BFS[K, P]{dir: dir, source: source}
}

// NewBFSTraversal returns a breadth-first Traversal following edges in
// the given direction, for use as a graph's configured search traversal
// (where the source is supplied per Traverse call).
func NewBFSTraversal[K comparable, P float32 | float64](dir Direction) *BFS[K, P] {
	return &BFS[K, P]{dir: dir}
}

// Run traverses g from the configured source and returns the result as an
// AlgorithmResult (a TraversalResult).
func (b *BFS[K, P]) Run(g Graph[K, P]) (AlgorithmResult, error) {
	result, err := b.traverse(g, b.source)
	if err != nil {
		return nil, fmt.Errorf("BFS failed: %w", err)
	}
	return result, nil
}

// sets source. Useful to change source and run multiple traversals with the same object
func (b *BFS[K, P]) SetSource(source K) {
	b.source = source
}

// Traverse walks g breadth-first from source, following edges in the
// configured direction, and records the visit order, the traversal tree and
// each vertex's hop distance from source.
func (b *BFS[K, P]) traverse(g Graph[K, P], source K) (TraversalResult[K], error) {
	if g.Get(source) == nil {
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

	result := TraversalResult[K]{
		Order:  []K{source},
		Parent: map[K]K{source: source},
		Depth:  map[K]int{source: 0},
	}

	queue := []K{source}
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
