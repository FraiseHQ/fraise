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

// Black-box tests (package containers_test) for the bounded top-k ranker.
// TopK is a drop-in replacement for sort-everything-then-truncate at the two
// call sites that ranked search results (BTreeIndex.Search and the graph's
// Search tail), so the central contract is equivalence with that baseline:
// for any k, Drain must match sorting every offer by (score descending,
// compare(key) ascending) and truncating to k, regardless of offer order.
package containers_test

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/comparator"
	"github.com/RonsenbergVI/fraise/internal/containers"
)

// sortThenTruncate is the reference model TopK replaces: sort every (key,
// score) pair by the total order and keep the first k (all of them if k <= 0).
func sortThenTruncate(keys []string, scores []float64, k int) ([]string, []float64) {
	type pair struct {
		key   string
		score float64
	}
	pairs := make([]pair, len(keys))
	for i := range keys {
		pairs[i] = pair{keys[i], scores[i]}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		return pairs[i].key < pairs[j].key
	})
	if k > 0 && len(pairs) > k {
		pairs = pairs[:k]
	}
	outKeys := make([]string, len(pairs))
	outScores := make([]float64, len(pairs))
	for i, p := range pairs {
		outKeys[i] = p.key
		outScores[i] = p.score
	}
	return outKeys, outScores
}

func newTopK(k int) *containers.TopK[string, float64] {
	return containers.NewTopK[string, float64](k, comparator.OrderedComparator[string])
}

// ---- behaviour tests --------------------------------------------------------

// TestOffer_DrainOrder_BestFirst pins the total order Drain promises: score
// descending, ties broken by key ascending.
func TestOffer_DrainOrder_BestFirst(t *testing.T) {
	top := newTopK(0)
	top.Offer("b", 5)
	top.Offer("a", 5)
	top.Offer("c", 9)
	top.Offer("d", 1)

	keys, scores := top.Drain()
	wantKeys := []string{"c", "a", "b", "d"}
	wantScores := []float64{9, 5, 5, 1}
	if len(keys) != len(wantKeys) {
		t.Fatalf("Drain() returned %d entries, want %d", len(keys), len(wantKeys))
	}
	for i := range wantKeys {
		if keys[i] != wantKeys[i] || scores[i] != wantScores[i] {
			t.Fatalf("Drain()[%d] = (%q, %v), want (%q, %v)", i, keys[i], scores[i], wantKeys[i], wantScores[i])
		}
	}
}

// TestOffer_KLessEqualZero_RetainsEverything pins the "return every match"
// contract: k <= 0 must never drop an offer.
func TestOffer_KLessEqualZero_RetainsEverything(t *testing.T) {
	for _, k := range []int{0, -1, -100} {
		top := newTopK(k)
		for i := 0; i < 50; i++ {
			top.Offer(string(rune('a'+i%26))+string(rune('A'+i/26)), float64(i))
		}
		keys, _ := top.Drain()
		if len(keys) != 50 {
			t.Fatalf("k=%d: Drain() returned %d entries, want all 50", k, len(keys))
		}
	}
}

// TestOffer_BoundsToK verifies a positive k keeps exactly the k best offers
// and discards the rest.
func TestOffer_BoundsToK(t *testing.T) {
	top := newTopK(3)
	for i, score := range []float64{1, 5, 3, 9, 2, 8} {
		top.Offer(string(rune('a'+i)), score)
	}
	keys, scores := top.Drain()
	wantKeys := []string{"d", "f", "b"} // scores 9, 8, 5
	wantScores := []float64{9, 8, 5}
	if len(keys) != 3 {
		t.Fatalf("Drain() returned %d entries, want 3", len(keys))
	}
	for i := range wantKeys {
		if keys[i] != wantKeys[i] || scores[i] != wantScores[i] {
			t.Fatalf("Drain()[%d] = (%q, %v), want (%q, %v)", i, keys[i], scores[i], wantKeys[i], wantScores[i])
		}
	}
}

// TestOffer_TieBrokenByKeyEvenWhenBounded checks that among equal scores at
// capacity, the smaller key wins admission — the tiebreak the unbounded path
// already applies at Drain must also govern which entries survive eviction.
func TestOffer_TieBrokenByKeyEvenWhenBounded(t *testing.T) {
	top := newTopK(2)
	top.Offer("z", 5)
	top.Offer("m", 5)
	top.Offer("a", 5) // must displace "z", the worst of the three equal scores

	keys, _ := top.Drain()
	want := []string{"a", "m"}
	if len(keys) != 2 || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("Drain() = %v, want %v", keys, want)
	}
}

// ---- equivalence against the sort-then-truncate baseline --------------------

// TestEquivalence_RandomizedAgainstBaseline drives randomized offer sets
// through TopK and the plain sort-then-truncate reference, across several k
// values and offer orderings, and requires an exact match: TopK is meant to
// be invisible to callers, only faster.
func TestEquivalence_RandomizedAgainstBaseline(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for trial := 0; trial < 300; trial++ {
		n := rng.Intn(60)
		keys := make([]string, n)
		scores := make([]float64, n)
		for i := 0; i < n; i++ {
			// A small score alphabet forces frequent ties, exercising the
			// key tiebreak alongside the score ordering.
			keys[i] = string(rune('a'+i%26)) + string(rune('A'+i/26))
			scores[i] = float64(rng.Intn(5))
		}

		order := rng.Perm(n)

		for _, k := range []int{0, 1, 3, n, n + 5} {
			top := newTopK(k)
			for _, i := range order {
				top.Offer(keys[i], scores[i])
			}
			gotKeys, gotScores := top.Drain()
			wantKeys, wantScores := sortThenTruncate(keys, scores, k)

			if len(gotKeys) != len(wantKeys) {
				t.Fatalf("trial %d k=%d: len=%d, want %d", trial, k, len(gotKeys), len(wantKeys))
			}
			for i := range wantKeys {
				if gotKeys[i] != wantKeys[i] || gotScores[i] != wantScores[i] {
					t.Fatalf("trial %d k=%d: entry %d = (%q, %v), want (%q, %v)",
						trial, k, i, gotKeys[i], gotScores[i], wantKeys[i], wantScores[i])
				}
			}
		}
	}
}

// TestEquivalence_OfferOrderIndependent feeds the same offers through TopK in
// several distinct orders and requires an identical Drain every time: the
// retained set and its order must depend only on the offers, never on the
// sequence they arrived in.
func TestEquivalence_OfferOrderIndependent(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	n := 40
	keys := make([]string, n)
	scores := make([]float64, n)
	for i := 0; i < n; i++ {
		keys[i] = string(rune('a'+i%26)) + string(rune('A'+i/26))
		scores[i] = float64(rng.Intn(8))
	}

	const k = 10
	run := func(order []int) ([]string, []float64) {
		top := newTopK(k)
		for _, i := range order {
			top.Offer(keys[i], scores[i])
		}
		return top.Drain()
	}

	baseline, baseScores := run(rng.Perm(n))
	for trial := 0; trial < 20; trial++ {
		gotKeys, gotScores := run(rng.Perm(n))
		if len(gotKeys) != len(baseline) {
			t.Fatalf("trial %d: len=%d, want %d", trial, len(gotKeys), len(baseline))
		}
		for i := range baseline {
			if gotKeys[i] != baseline[i] || gotScores[i] != baseScores[i] {
				t.Fatalf("trial %d: entry %d = (%q, %v), want (%q, %v)",
					trial, i, gotKeys[i], gotScores[i], baseline[i], baseScores[i])
			}
		}
	}
}

// TestDrain_EmptiesTheTopK checks that a second Drain after offers have
// already been drained returns nothing, matching the "drains" name.
func TestDrain_EmptiesTheTopK(t *testing.T) {
	top := newTopK(0)
	top.Offer("a", 1)
	top.Drain()

	keys, scores := top.Drain()
	if len(keys) != 0 || len(scores) != 0 {
		t.Fatalf("second Drain() = (%v, %v), want empty", keys, scores)
	}
}
