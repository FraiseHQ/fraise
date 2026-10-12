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

package containers

import (
	"sort"

	"github.com/FraiseHQ/fraise/internal/comparator"
)

// topKEntry pairs a key with its score so the two travel together through
// heap swaps and the final sort, the same shape as Heap's Item.
type topKEntry[K comparable, P float32 | float64] struct {
	key   K
	score P
}

// TopK retains the k best (key, score) pairs under the total order (score
// descending, then compare(key) ascending) and drains them in that order.
// Offer order never affects the retained set or the drain order — the total
// order is the contract, exactly as SearchIndex promises. k <= 0 retains
// every offer. It is not safe for concurrent use.
type TopK[K comparable, P float32 | float64] struct {
	k       int
	compare comparator.Comparator[K]
	entries []topKEntry[K, P]
}

// NewTopK returns a TopK retaining the k best offers, breaking ties among
// equal scores by compare(key) ascending. k <= 0 retains every offer.
func NewTopK[K comparable, P float32 | float64](k int, compare comparator.Comparator[K]) *TopK[K, P] {
	return &TopK[K, P]{k: k, compare: compare}
}

// Offer considers (key, score) for retention. When k <= 0, or while still
// under capacity, the offer is always kept; once at capacity, it replaces
// the current worst kept pair iff it ranks strictly better under the total
// order, so the k best survive regardless of offer order.
func (t *TopK[K, P]) Offer(key K, score P) {
	if t.k <= 0 || len(t.entries) < t.k {
		t.entries = append(t.entries, topKEntry[K, P]{key: key, score: score})
		if t.k > 0 {
			t.percolateUp(len(t.entries) - 1)
		}
		return
	}

	root := t.entries[0]
	better := score > root.score || (score == root.score && t.compare(key, root.key) < 0)
	if !better {
		return
	}
	t.entries[0] = topKEntry[K, P]{key: key, score: score}
	t.percolateDown(0)
}

// Drain empties the TopK and returns its retained keys and scores as
// parallel slices ordered best first: score descending, then compare(key)
// ascending.
func (t *TopK[K, P]) Drain() ([]K, []P) {
	sort.Slice(t.entries, func(i, j int) bool {
		a, b := t.entries[i], t.entries[j]
		if a.score != b.score {
			return a.score > b.score
		}
		return t.compare(a.key, b.key) < 0
	})

	keys := make([]K, len(t.entries))
	scores := make([]P, len(t.entries))
	for i, e := range t.entries {
		keys[i] = e.key
		scores[i] = e.score
	}
	t.entries = nil
	return keys, scores
}

// worseThan reports whether entry i ranks worse than entry j under the total
// order: a lower score is worse, and among equal scores the larger key is
// worse (ascending key is the better tiebreak). The bounded heap keeps its
// worst entry at the root under this ordering, so Offer can evict it in
// O(log k).
func (t *TopK[K, P]) worseThan(i, j int) bool {
	a, b := t.entries[i], t.entries[j]
	if a.score != b.score {
		return a.score < b.score
	}
	return t.compare(a.key, b.key) > 0
}

func (t *TopK[K, P]) percolateUp(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !t.worseThan(index, parent) {
			break
		}
		t.entries[index], t.entries[parent] = t.entries[parent], t.entries[index]
		index = parent
	}
}

func (t *TopK[K, P]) percolateDown(index int) {
	n := len(t.entries)
	for {
		left, right := 2*index+1, 2*index+2
		worst := index
		if left < n && t.worseThan(left, worst) {
			worst = left
		}
		if right < n && t.worseThan(right, worst) {
			worst = right
		}
		if worst == index {
			break
		}
		t.entries[index], t.entries[worst] = t.entries[worst], t.entries[index]
		index = worst
	}
}
