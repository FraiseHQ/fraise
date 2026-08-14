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

// Same-package pins of the relevance seam. The equivalence pin proves the
// seam is a zero-behavior change — the default model reproduces the
// pre-plugin ranking byte for byte — and the BM25 lifecycle pins reach the
// model's private statistics, which no query result exposes.

package index

import (
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/comparator"
)

// TestMatchCountEquivalentToPrePluginRanking is the heart of the seam: on
// randomized corpora and queries, the default model produces exactly the
// ranking the index shipped with before relevance became pluggable —
// identical keys, identical scores, identical order, including the
// double-count a repeated query term always earned. The oracle reimplements
// that ranking independently from the raw documents (per-document term sets,
// one point per query-term occurrence, count-descending with the key
// tiebreak), so the pin cannot drift with the index's internals.
func TestMatchCountEquivalentToPrePluginRanking(t *testing.T) {
	rng := rand.New(rand.NewSource(7)) // deterministic corpora across runs
	vocabulary := []string{"ash", "brine", "coral", "drift", "eddy", "fjord", "gull", "harbour"}

	for corpus := 0; corpus < 20; corpus++ {
		idx := NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
		docs := make(map[int][]string)
		for key := 0; key < 12; key++ {
			length := 1 + rng.Intn(6)
			tokens := make([]string, length)
			for i := range tokens {
				tokens[i] = vocabulary[rng.Intn(len(vocabulary))]
			}
			docs[key] = tokens
			if err := idx.Insert(key, strings.Join(tokens, " ")); err != nil {
				t.Fatalf("Insert(%d) = %v, want nil", key, err)
			}
		}

		for probe := 0; probe < 10; probe++ {
			queryTokens := make([]string, 1+rng.Intn(4)) // repeats likely
			for i := range queryTokens {
				queryTokens[i] = vocabulary[rng.Intn(len(vocabulary))]
			}
			query := strings.Join(queryTokens, " ")

			gotKeys, gotScores, err := idx.Search(query, 0)
			if err != nil {
				t.Fatalf("Search(%q) = %v, want nil", query, err)
			}

			// The oracle: membership per document, one point per query-term
			// occurrence, count desc then key asc.
			counts := make(map[int]int)
			for _, term := range queryTokens {
				for key, tokens := range docs {
					for _, docTerm := range tokens {
						if docTerm == term {
							counts[key]++
							break
						}
					}
				}
			}
			wantKeys := make([]int, 0, len(counts))
			for key := range counts {
				wantKeys = append(wantKeys, key)
			}
			sort.Slice(wantKeys, func(i, j int) bool {
				if counts[wantKeys[i]] != counts[wantKeys[j]] {
					return counts[wantKeys[i]] > counts[wantKeys[j]]
				}
				return wantKeys[i] < wantKeys[j]
			})
			wantScores := make([]float64, len(wantKeys))
			for i, key := range wantKeys {
				wantScores[i] = float64(counts[key])
			}

			if !reflect.DeepEqual(gotKeys, wantKeys) || !reflect.DeepEqual(gotScores, wantScores) {
				t.Fatalf("corpus %d, Search(%q):\n got %v %v\nwant %v %v",
					corpus, query, gotKeys, gotScores, wantKeys, wantScores)
			}
		}
	}
}

// TestBM25LifecycleBookkeeping pins the statistics across the whole document
// lifecycle: insert admits a length, update retires the old length before
// admitting the new (Removed precedes Indexed, so the key is never counted
// twice), and delete returns the statistics to zero — using the recorded
// length, not a re-tokenization that a tokenizer swap could skew.
func TestBM25LifecycleBookkeeping(t *testing.T) {
	model := NewBM25[int]()
	idx := NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	idx.SetRelevance(model)

	if err := idx.Insert(1, "one two three"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if model.totalLen != 3 || model.lengths[1] != 3 {
		t.Fatalf("after insert: totalLen %d, lengths[1] %d, want 3 and 3", model.totalLen, model.lengths[1])
	}

	if err := idx.Update(1, "one two three four five"); err != nil {
		t.Fatalf("Update = %v, want nil", err)
	}
	if model.totalLen != 5 || model.lengths[1] != 5 {
		t.Fatalf("after update: totalLen %d, lengths[1] %d, want 5 and 5 (old length retired first)", model.totalLen, model.lengths[1])
	}

	if err := idx.Delete(1); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if model.totalLen != 0 || len(model.lengths) != 0 {
		t.Fatalf("after delete: totalLen %d, %d lengths, want the statistics back at zero", model.totalLen, len(model.lengths))
	}
}

// TestBM25TermsDedupsFirstOccurrence pins the query-side stream: distinct
// terms only — idf must not double-count a repeated term — in first
// occurrence order, which is what keeps score accumulation deterministic.
func TestBM25TermsDedupsFirstOccurrence(t *testing.T) {
	model := NewBM25[int]()
	got := model.Terms([]string{"tide", "moon", "tide", "wave", "moon"})
	if want := []string{"tide", "moon", "wave"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Terms = %v, want %v", got, want)
	}
}

// TestBM25CoverageScalesByMatchBreadth pins Finalize's slot: full coverage
// leaves the accumulated score whole, half coverage halves it.
func TestBM25CoverageScalesByMatchBreadth(t *testing.T) {
	model := NewBM25[int]()
	if got := model.Finalize(4, 2, 2); got != 4 {
		t.Errorf("Finalize at full coverage = %v, want 4", got)
	}
	if got := model.Finalize(4, 1, 2); got != 2 {
		t.Errorf("Finalize at half coverage = %v, want 2", got)
	}
}

// TestBM25WeightIsTheStandardIdf pins Weight against the closed form at a
// hand-checkable point: df 1 of 2 documents gives ln 2 exactly.
func TestBM25WeightIsTheStandardIdf(t *testing.T) {
	model := NewBM25[int]()
	if got, want := model.Weight(1, 2), math.Log(2); got != want {
		t.Errorf("Weight(1, 2) = %v, want ln 2 = %v", got, want)
	}
}

// TestSearchPurity pins the query side of the Relevance contract: the same
// query twice against an untouched corpus is byte-identical — no query
// mutates the model's statistics.
func TestSearchPurity(t *testing.T) {
	for name, model := range map[string]Relevance[int]{
		"match count": MatchCount[int]{},
		"bm25":        NewBM25[int](),
	} {
		t.Run(name, func(t *testing.T) {
			idx := NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
			idx.SetRelevance(model)
			for key, doc := range map[int]string{1: "red green", 2: "green blue", 3: "blue red green"} {
				if err := idx.Insert(key, doc); err != nil {
					t.Fatalf("Insert(%d) = %v, want nil", key, err)
				}
			}

			firstKeys, firstScores, err := idx.Search("red green", 0)
			if err != nil {
				t.Fatalf("Search = %v, want nil", err)
			}
			for i := 0; i < 10; i++ {
				keys, scores, err := idx.Search("red green", 0)
				if err != nil || !reflect.DeepEqual(keys, firstKeys) || !reflect.DeepEqual(scores, firstScores) {
					t.Fatalf("call %d: %v %v (err %v), want %v %v every call", i+2, keys, scores, err, firstKeys, firstScores)
				}
			}
		})
	}
}
