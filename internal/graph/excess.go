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
	"slices"
)

// ExcessTraversal is the excess methodology's traversal. It steps once from
// each source across the bipartition between facts and anchors. A fact steps
// to its anchors, its Topic and NamedEntity neighbours in either edge
// direction. An anchor steps to its members, the facts filed under it. Search
// walks it twice: from the seeds to the anchors they are filed under, then
// from the anchors that transmit to their members. So a hub the seeds reach
// is listed only if it transmits, and then once, however many seeds reach it.
//
// Every vertex it reaches sits at depth 1. Order lists them in ascending key
// order, so the collection layer folds floats in a fixed order. That sort is
// why K is cmp.Ordered rather than comparable. Parents carries the full
// incidence, every source a vertex was reached from, in source order. An
// anchor that two seeds reach is funded twice, and a member of two
// transmitting anchors is observed twice. A seed is among the members of its
// own anchors, so an anchor's members are the same whichever seeds reach it.
// Self-exclusion then happens once, in the scorer, where the member's own mass
// is at hand.
type ExcessTraversal[K cmp.Ordered, P float32 | float64] struct {
	sources []K
}

// NewExcessTraversal returns the excess-transmission traversal.
func NewExcessTraversal[K cmp.Ordered, P float32 | float64]() *ExcessTraversal[K, P] {
	return &ExcessTraversal[K, P]{}
}

// Run traverses from the configured sources, returning an AlgorithmResult
// (a TraversalResult).
func (t *ExcessTraversal[K, P]) Run(g Graph[K, P]) (AlgorithmResult, error) {
	return t.traverse(g, t.sources)
}

// SetSources sets the vertices the next Run starts from.
func (t *ExcessTraversal[K, P]) SetSources(sources []K) {
	t.sources = sources
}

// Clone returns a fresh traversal with no per-run state, so each walk can set
// its own sources instead of mutating a shared instance.
func (t *ExcessTraversal[K, P]) Clone() Traversal[K, P] {
	return &ExcessTraversal[K, P]{}
}

// traverse steps once from every source g holds to its neighbours of the
// other kind: a fact's anchors, an anchor's members.
func (t *ExcessTraversal[K, P]) traverse(g Graph[K, P], sources []K) (TraversalResult[K], error) {
	result := TraversalResult[K]{
		Parent:  make(map[K]K),
		Parents: make(map[K][]K),
		Depth:   make(map[K]int),
	}
	walked := make(map[K]struct{}, len(sources))
	for _, source := range sources {
		if _, dup := walked[source]; dup || g.Get(source) == nil {
			continue
		}
		walked[source] = struct{}{}
		fromAnchor := isAnchor(g, source)
		for _, neighbour := range g.Neighbours(source) {
			if isAnchor(g, neighbour) == fromAnchor {
				continue
			}
			// Neighbours lists a pair joined by an edge each way twice, but
			// it is one incidence. While a source is walked, its entry is the
			// last one appended to a neighbour's Parents.
			parents := result.Parents[neighbour]
			if n := len(parents); n > 0 && parents[n-1] == source {
				continue
			}
			if _, seen := result.Depth[neighbour]; !seen {
				result.Order = append(result.Order, neighbour)
				result.Parent[neighbour] = source
				result.Depth[neighbour] = 1
			}
			result.Parents[neighbour] = append(parents, source)
		}
	}
	if len(walked) == 0 {
		return TraversalResult[K]{}, ErrSourceNotFound
	}
	slices.Sort(result.Order)
	return result, nil
}

// isAnchor reports whether the node under key is an anchor: a Topic or
// NamedEntity vertex. Anchors mediate transmission between facts. They carry
// no seed mass of their own and are never returned as hits.
func isAnchor[K comparable, P float32 | float64](g Graph[K, P], key K) bool {
	switch g.Get(key).(type) {
	case *Topic[K], *NamedEntity[K]:
		return true
	}
	return false
}
