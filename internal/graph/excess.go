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
	"cmp"
	"sort"
)

// ExcessTraversal is the excess methodology's traversal: from a source fact
// it visits the source's anchors — Topic and NamedEntity neighbours, both
// edge directions — at depth 1, then every member of those anchors at depth
// 2. Order lists the anchors then the members, ascending key within each
// band, so the collection layer's float folds run in a fixed order. Parents
// carries the full member→anchors incidence (a member reached through two of
// the source's anchors is two observations, not one), and the source itself
// appears among its anchors' members: observed anchor mass then aggregates
// identically for every observer, and self-exclusion happens once, in the
// scorer, where the member's own mass is at hand.
// K is ordered, not merely comparable: the band ordering is what lets the
// collection layer fold floats without run-to-run drift, and an unordered key
// type would have no order to band by.
type ExcessTraversal[K cmp.Ordered, P float32 | float64] struct {
	source K
}

// NewExcessTraversal returns the excess-transmission traversal.
func NewExcessTraversal[K cmp.Ordered, P float32 | float64]() *ExcessTraversal[K, P] {
	return &ExcessTraversal[K, P]{}
}

// Run traverses from the configured source, returning an AlgorithmResult
// (a TraversalResult).
func (t *ExcessTraversal[K, P]) Run(g Graph[K, P]) (AlgorithmResult, error) {
	return t.traverse(g, t.source)
}

// SetSource configures the vertex the next Run starts from.
func (t *ExcessTraversal[K, P]) SetSource(source K) {
	t.source = source
}

// Clone returns a fresh traversal with no per-run state, so each walk can set
// its own source instead of mutating a shared instance.
func (t *ExcessTraversal[K, P]) Clone() Traversal[K, P] {
	return &ExcessTraversal[K, P]{}
}

// traverse visits the source's anchors (depth 1) and their members (depth 2).
func (t *ExcessTraversal[K, P]) traverse(g Graph[K, P], source K) (TraversalResult[K], error) {
	if g.Get(source) == nil {
		return TraversalResult[K]{}, ErrSourceNotFound
	}

	// Single-node neighbour access via the non-copying Neighbours accessor.
	// The prior code built adjacency/predecessor views with AdjacencyMap()/
	// PredecessorMap(), each of which deep-copies the ENTIRE edge set on every
	// call — and traverse runs once per seed, so a query cloned the whole graph
	// ~2x per seed (exportEdges: 61% of CPU, 213MB/query on a 60k fat hub).
	neighbours := func(key K) []K {
		out := g.Neighbours(key)
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}

	anchors := make([]K, 0)
	seenAnchor := make(map[K]struct{})
	for _, neighbour := range neighbours(source) {
		if _, dup := seenAnchor[neighbour]; dup {
			continue
		}
		if !isAnchor(g, neighbour) {
			continue
		}
		seenAnchor[neighbour] = struct{}{}
		anchors = append(anchors, neighbour)
	}

	result := TraversalResult[K]{
		Order:   make([]K, 0, len(anchors)),
		Parent:  make(map[K]K, len(anchors)),
		Parents: make(map[K][]K, len(anchors)),
		Depth:   make(map[K]int, len(anchors)),
	}
	for _, anchor := range anchors {
		result.Order = append(result.Order, anchor)
		result.Parent[anchor] = source
		result.Parents[anchor] = []K{source}
		result.Depth[anchor] = 1
	}

	members := make([]K, 0)
	for _, anchor := range anchors {
		for _, member := range neighbours(anchor) {
			if _, isAnchorVertex := seenAnchor[member]; isAnchorVertex {
				continue
			}
			if _, seen := result.Depth[member]; !seen {
				result.Parent[member] = anchor
				result.Depth[member] = 2
				members = append(members, member)
			}
			result.Parents[member] = append(result.Parents[member], anchor)
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })
	result.Order = append(result.Order, members...)

	return result, nil
}

// isAnchor reports whether the node under key is an anchor — a Topic or
// NamedEntity vertex. Anchors mediate transmission between facts; they carry
// no seed mass of their own and are never returned as hits.
func isAnchor[K comparable, P float32 | float64](g Graph[K, P], key K) bool {
	switch g.Get(key).(type) {
	case *Topic[K], *NamedEntity[K]:
		return true
	}
	return false
}
