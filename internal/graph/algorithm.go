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

// Direction selects which edges a traversal follows in a directed graph.
type Direction int

const (
	// Outgoing follows edges from a vertex to its successors (AdjacencyMap).
	Outgoing Direction = iota

	// Incoming follows edges from a vertex to its predecessors (PredecessorMap).
	Incoming

	// Both treats edges as undirected, following successors and predecessors.
	Both
)

// Algorithm is the common root of the graph algorithms. Run executes the
// algorithm on g.
type Algorithm[K comparable, P float32 | float64] interface {
	Run(g Graph[K, P]) (AlgorithmResult, error)
}

// AlgorithmResult is what [Algorithm.Run] returns, a [TraversalResult] for a
// [Traversal]. It has no methods, so the caller type-asserts it to the result
// its algorithm produces.
type AlgorithmResult interface {
}

// TraversalResult captures the outcome of a source-based traversal.
type TraversalResult[K comparable] struct {
	AlgorithmResult
	// Order lists the vertices in the order they were first visited.
	Order []K

	// Parent maps each visited vertex to the vertex it was discovered from,
	// forming the traversal tree. A source visited at depth 0 maps to its own
	// key.
	Parent map[K]K

	// Parents maps each visited vertex to every vertex it was reached through
	// at the previous depth. Parent keeps one canonical entry, the first in
	// Order, for tree-shaped consumers. Parents carries the full incidence,
	// because a member reached through two of the source's anchors is two
	// observations, not one. Tree-shaped traversals may leave it nil.
	Parents map[K][]K

	// Depth maps each visited vertex to its hop distance from the source.
	Depth map[K]int
}

// Traversal explores a graph from a source vertex, in an order the
// implementation defines. BFS visits every reachable vertex breadth-first.
// ExcessTraversal visits only the source's anchors and their members. K is the
// vertex key type and P the graph's score precision.
type Traversal[K comparable, P float32 | float64] interface {
	Algorithm[K, P]

	// traverse walks g from source and returns the visit order, traversal
	// tree and per-vertex depth.
	traverse(g Graph[K, P], source K) (TraversalResult[K], error)

	// SetSource sets the vertex the next Run starts from.
	SetSource(source K)

	// Clone returns a fresh traversal with the same configuration but no
	// per-run state. Each walk can then set its own source and run on its own
	// instance, instead of mutating a shared one.
	Clone() Traversal[K, P]
}
