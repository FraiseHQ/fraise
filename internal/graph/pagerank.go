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

// PageRank scores every vertex by the stationary distribution of a random walk
// that follows an outgoing edge with probability damping and teleports to a
// uniformly random vertex with probability 1-damping. It iterates until the
// scores change by less than tol or maxIter iterations are reached. It
// implements Ranking.
type PageRank[K comparable, P float32 | float64] struct {
	damping P   // probability of following an edge (typically 0.85)
	maxIter int // iteration cap
	tol     P   // convergence threshold on the score delta
}

// NewPageRank returns a PageRank ranking with the given damping factor,
// iteration cap and convergence tolerance.
func NewPageRank[K comparable, P float32 | float64](damping P, maxIter int, tol P) *PageRank[K, P] {
	return &PageRank[K, P]{damping: damping, maxIter: maxIter, tol: tol}
}

// Name returns the algorithm identifier.
func (pr *PageRank[K, P]) Name() string { return "pagerank" }

// Rank computes the PageRank score of every vertex in g. Vertices are taken
// from the union of both edge views, so isolated nodes (no edges at all) are
// not ranked. Dangling vertices (no outgoing edges) redistribute their mass
// uniformly, keeping the scores a probability distribution.
func (pr *PageRank[K, P]) rank(g Graph[K, P]) (map[K]P, error) {
	adjacency := g.AdjacencyMap()
	predecessors := g.PredecessorMap()

	vertices := make(map[K]bool)
	for u, targets := range adjacency {
		vertices[u] = true
		for v := range targets {
			vertices[v] = true
		}
	}
	for v := range predecessors {
		vertices[v] = true
	}
	n := len(vertices)
	if n == 0 {
		return nil, ErrEmptyGraph
	}

	scores := make(map[K]P, n)
	for v := range vertices {
		scores[v] = P(1) / P(n)
	}

	for iter := 0; iter < pr.maxIter; iter++ {
		var danglingMass P
		for v := range vertices {
			if len(adjacency[v]) == 0 {
				danglingMass += scores[v]
			}
		}

		next := make(map[K]P, n)
		base := (P(1)-pr.damping)/P(n) + pr.damping*danglingMass/P(n)
		var delta P
		for v := range vertices {
			score := base
			for u := range predecessors[v] {
				score += pr.damping * scores[u] / P(len(adjacency[u]))
			}
			next[v] = score

			diff := score - scores[v]
			if diff < 0 {
				diff = -diff
			}
			delta += diff
		}

		scores = next
		if delta < pr.tol {
			break
		}
	}
	return scores, nil
}

// Run ranks every vertex of g and returns the result as an AlgorithmResult
// (a RankingResult).
func (pr *PageRank[K, P]) Run(g Graph[K, P]) (AlgorithmResult, error) {
	scores, err := pr.rank(g)
	if err != nil {
		return nil, fmt.Errorf("page rank failed: %w", err)
	}
	return RankingResult[K, P]{Scores: scores}, nil
}
