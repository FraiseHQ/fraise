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

// TraversalResult captures the outcome of a traversal from a set of sources.
type TraversalResult[K comparable] struct {
	AlgorithmResult
	// Order lists the visited vertices in the order the traversal defines:
	// discovery order for BFS, ascending keys for ExcessTraversal.
	Order []K

	// Parent maps each visited vertex to the vertex it was discovered from,
	// forming the traversal tree, a forest when there are several sources. A
	// source visited at depth 0 maps to its own key.
	Parent map[K]K

	// Parents maps each visited vertex to every vertex it was reached through
	// at the previous depth. Parent keeps one canonical entry, the first of
	// them, for tree-shaped consumers. Parents carries the full incidence,
	// because a member reached through two anchors is two observations, not
	// one. Tree-shaped traversals may leave it nil.
	Parents map[K][]K

	// Depth maps each visited vertex to its hop distance from the nearest
	// source.
	Depth map[K]int
}

// reachedFrom returns the vertices v was reached through: its Parents when
// the traversal records the full incidence, and otherwise its Parent alone,
// as a tree-shaped traversal observes it.
func (r TraversalResult[K]) reachedFrom(v K) []K {
	if parents := r.Parents[v]; len(parents) > 0 {
		return parents
	}
	if parent, ok := r.Parent[v]; ok {
		return []K{parent}
	}
	return nil
}

// Traversal explores a graph from a set of source vertices, in an order the
// implementation defines. BFS visits every vertex reachable from the sources,
// breadth-first. ExcessTraversal steps once from each source across the
// bipartition between facts and anchors. K is the vertex key type and P the
// graph's score precision.
type Traversal[K comparable, P float32 | float64] interface {
	Algorithm[K, P]

	// traverse walks g from sources and returns the visit order, traversal
	// tree and per-vertex depth. A source the graph does not hold is skipped,
	// and the walk fails with ErrSourceNotFound when it holds none of them.
	traverse(g Graph[K, P], sources []K) (TraversalResult[K], error)

	// SetSources sets the vertices the next Run starts from. A walk from
	// several sources is one walk, not one per source: a vertex that several
	// sources reach is visited once, so a hub costs the same whether one seed
	// reaches it or every seed does.
	SetSources(sources []K)

	// Clone returns a fresh traversal with the same configuration but no
	// per-run state. Each walk can then set its own sources and run on its
	// own instance, instead of mutating a shared one.
	Clone() Traversal[K, P]
}
