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

package algorithms

import "github.com/RonsenbergVI/fraise/internal/graph"

// BFS is a breadth-first traversal: it explores the graph frontier by frontier
// (nearest vertices first) using a FIFO queue, starting from a configured
// source vertex. It implements Traversal.
type BFS[K comparable, P float32 | float64] struct {
	dir    Direction // which edges to follow
	source K         // vertex the traversal starts from
}

// NewBFS returns a breadth-first traversal that starts from source and follows
// edges in the given direction.
func NewBFS[K comparable, P float32 | float64](source K, dir Direction) *BFS[K, P] {
	return &BFS[K, P]{dir: dir, source: source}
}

// Run traverses g from the configured source and returns the result as an
// AlgorithmResult (a TraversalResult).
func (b *BFS[K, P]) Run(g graph.Graph[K, P]) AlgorithmResult {
	panic("not implemented")
}

// traverse walks g breadth-first from the configured source.
func (b *BFS[K, P]) traverse(g graph.Graph[K, P], source K) (TraversalResult[K], error) {
	panic("not implemented")
}
