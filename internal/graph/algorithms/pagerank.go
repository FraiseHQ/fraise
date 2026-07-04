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

// Rank computes the PageRank score of every vertex in g.
func (pr *PageRank[K, P]) rank(g graph.Graph[K, P]) (map[K]P, error) {
	panic("not implemented")
}

func (b *PageRank[K, P]) Run(g graph.Graph[K, P]) AlgorithmResult {
	panic("not implemented")
}
