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

// alpha is the per-edge attenuation of transmitted excess. It is an internal
// constant, not configuration: the methodology's guarantees (the BM25 floor,
// hub silence) hold for any 0 < α < 1, and the two-edge seed→anchor→fact path
// applies it squared. α per *edge* rather than per path is deliberate — a
// single un-squared α ran the graph channel twice as hot as the text channel
// and collapsed the scorer (RRF_FINDINGS Round 8).
const alpha = 0.5

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

	adjacency, predecessors := g.AdjacencyMap(), g.PredecessorMap()
	neighbours := func(key K) []K {
		out := make([]K, 0, len(adjacency[key])+len(predecessors[key]))
		for neighbour := range adjacency[key] {
			out = append(out, neighbour)
		}
		for neighbour := range predecessors[key] {
			out = append(out, neighbour)
		}
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

// ExcessScorer folds a candidate's observations under the excess-transmission
// methodology: relevance is the candidate's own seed mass plus the
// above-background surplus its anchors transmitted, attenuated α² for the
// two-edge path. Each graph observation carries its anchor's full observed
// mass; the fold subtracts the candidate's own mass (self-exclusion — a fact
// never funds its own boost) and the anchor's size-proportional share of the
// background, and keeps only what remains above zero. An anchor at or below
// its fair share therefore contributes nothing — hubs are heard exactly when
// they are surprising, and silent when they are merely large.
//
// Scores stay in raw seed units end to end: relevance is homogeneous of
// degree 1 in the mass scale, so normalizing anywhere is a provable ordering
// no-op that only breaks the commensurability of the channels.
type ExcessScorer[K comparable, P float32 | float64] struct {
	// background is the query's bound null rate. It is set only by
	// WithBackground returning a fresh value — never mutated in place — so
	// the graph's shared instance stays pure and the zero value is the
	// unbound scorer seed fusion runs.
	background P
}

// NewExcessScorer returns the excess-transmission fold, unbound (background
// zero) until WithBackground binds a query's rate.
func NewExcessScorer[K comparable, P float32 | float64]() *ExcessScorer[K, P] {
	return &ExcessScorer[K, P]{}
}

// WithBackground returns a scorer bound to one query's background rate.
func (s *ExcessScorer[K, P]) WithBackground(background P) Scorer[K, P] {
	return &ExcessScorer[K, P]{background: background}
}

// Score folds contributions at the bound background rate. Seed mass first —
// the text and vector observations sum directly — then the hinge over each
// graph observation, in list order, so identical inputs fold to
// byte-identical scores.
func (s *ExcessScorer[K, P]) Score(contributions []Contribution[K, P]) P {
	var mass P
	for _, c := range contributions {
		if c.Src == SrcText || c.Src == SrcVector {
			mass += c.Score
		}
	}

	var excess P
	for _, c := range contributions {
		if c.Src != SrcGraph {
			continue
		}
		if surplus := c.Score - mass - P(c.Degree)*s.background; surplus > 0 {
			excess += surplus
		}
	}
	return mass + alpha*alpha*excess
}
