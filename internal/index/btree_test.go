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

package index_test

import (
	"errors"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/index"
	"github.com/FraiseHQ/fraise/internal/index/nlp"
	"github.com/FraiseHQ/fraise/internal/index/relevance"
)

func TestBTreeIndexInsertAndRetrieve(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])

	if err := idx.Insert(1, "the quick brown fox"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if got, want := idx.Count(), 1; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}

	got, err := idx.Retrieve(1)
	if err != nil {
		t.Fatalf("Retrieve = %v, want nil", err)
	}
	if want := "the quick brown fox"; got != want {
		t.Errorf("Retrieve() = %q, want %q", got, want)
	}
}

func TestBTreeIndexRetrieveMissing(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	if _, err := idx.Retrieve(99); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Retrieve(99) = %v, want ErrIndexNotFound", err)
	}
}

func TestBTreeIndexSearchEmpty(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	if _, _, err := idx.Search("anything", 0); !errors.Is(err, index.ErrEmptyIndex) {
		t.Errorf("Search on empty index = %v, want ErrEmptyIndex", err)
	}
}

// TestBTreeIndexSearchRanksByRelevance pins the BM25 ranking behaviours a
// match count cannot express: of two documents matching every query term, the
// shorter one ranks first (length normalization — the long document dilutes
// its terms), and a document matching nothing is not a result at all.
func TestBTreeIndexSearchRanksByRelevance(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	idx.SetRelevance(relevance.NewBM25[int, float64]())
	docs := map[int]string{
		1: "the quick brown fox jumps over the lazy dog",
		2: "the quick brown fox",
		3: "a completely unrelated sentence",
	}
	for key, doc := range docs {
		if err := idx.Insert(key, doc); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", key, err)
		}
	}

	got, scores, err := idx.Search("quick brown fox", 0)
	if err != nil {
		t.Fatalf("Search = %v, want nil", err)
	}
	// Both matching documents cover the whole query; the shorter one wins.
	if want := []int{2, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("Search() = %v, want %v (shorter full match first)", got, want)
	}
	if scores[0] <= scores[1] || scores[1] <= 0 {
		t.Errorf("scores = %v, want strictly decreasing and positive", scores)
	}
}

// TestBTreeIndexSearchOrdersTiesByKey pins the tiebreak that makes the ranking a
// total order: documents matching the same number of query terms are ranked by
// key, not in the order the posting map happened to yield them. The search is
// repeated because that map order changes between calls — one pass can agree
// with the expected order by luck — and truncation to k makes the difference
// user-visible, since it keeps the head of whatever order the sort produced.
func TestBTreeIndexSearchOrdersTiesByKey(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	// Every document contains "sky" once, so the match count never separates
	// them; the keys are inserted out of order so ascending order cannot come
	// from insertion order.
	for _, key := range []int{7, 3, 9, 1, 5} {
		if err := idx.Insert(key, "sky"); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", key, err)
		}
	}

	cases := []struct {
		name string
		k    int
		want []int
	}{
		{"every match, in key order", 0, []int{1, 3, 5, 7, 9}},
		{"truncation keeps the lowest key", 1, []int{1}},
		{"truncation keeps the lowest keys", 3, []int{1, 3, 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 20; i++ {
				got, _, err := idx.Search("sky", tc.k)
				if err != nil {
					t.Fatalf("Search(sky, %d) = %v, want nil", tc.k, err)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("Search(sky, %d) = %v on call %d, want %v every call", tc.k, got, i+1, tc.want)
				}
			}
		})
	}
}

func TestBTreeIndexSearchNoMatchOnNonEmptyIndex(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	if err := idx.Insert(1, "apples and oranges"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}

	got, _, err := idx.Search("nonexistent", 0)
	if err != nil {
		t.Fatalf("Search = %v, want nil (no match is not an error)", err)
	}
	if len(got) != 0 {
		t.Errorf("Search() = %v, want empty", got)
	}
}

func TestBTreeIndexUpdateMovesPostings(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	if err := idx.Insert(1, "apples"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Update(1, "oranges"); err != nil {
		t.Fatalf("Update = %v, want nil", err)
	}

	if got, _, err := idx.Search("apples", 0); err != nil || len(got) != 0 {
		t.Errorf("Search(apples) after update = (%v, %v), want (empty, nil)", got, err)
	}
	got, _, err := idx.Search("oranges", 0)
	if err != nil || len(got) != 1 || got[0] != 1 {
		t.Errorf("Search(oranges) after update = (%v, %v), want ([1], nil)", got, err)
	}
}

func TestBTreeIndexUpdateMissing(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	if err := idx.Update(1, "x"); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Update on missing key = %v, want ErrIndexNotFound", err)
	}
}

func TestBTreeIndexDelete(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	if err := idx.Insert(1, "shared term"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Insert(2, "shared term"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}

	if err := idx.Delete(1); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if got, want := idx.Count(), 1; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
	if _, err := idx.Retrieve(1); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Retrieve(1) after delete = %v, want ErrIndexNotFound", err)
	}

	// Term "shared" is still referenced by doc 2, so it must still be
	// searchable.
	got, _, err := idx.Search("shared", 0)
	if err != nil || len(got) != 1 || got[0] != 2 {
		t.Errorf("Search(shared) after deleting doc 1 = (%v, %v), want ([2], nil)", got, err)
	}

	if err := idx.Delete(2); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	// The index itself is now empty, which is a distinct condition from "no
	// document matched the query".
	if _, _, err := idx.Search("shared", 0); !errors.Is(err, index.ErrEmptyIndex) {
		t.Errorf("Search(shared) after deleting all docs = %v, want ErrEmptyIndex", err)
	}
}

func TestBTreeIndexDeleteMissing(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	if err := idx.Delete(1); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Delete on missing key = %v, want ErrIndexNotFound", err)
	}
}

func TestBTreeIndexInsertOverwritesExistingKey(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	if err := idx.Insert(1, "first version"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Insert(1, "second version"); err != nil {
		t.Fatalf("Insert (overwrite) = %v, want nil", err)
	}
	if got, want := idx.Count(), 1; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
	got, err := idx.Retrieve(1)
	if err != nil || got != "second version" {
		t.Errorf("Retrieve(1) = (%q, %v), want (\"second version\", nil)", got, err)
	}
	if got, _, err := idx.Search("first", 0); err != nil || len(got) != 0 {
		t.Errorf("Search(first) after overwrite = (%v, %v), want (empty, nil)", got, err)
	}
}

// textScoresAreBM25TimesCoverage pins the exact score formula at precision P:
// BM25 (idf-weighted, length-normalized term frequency) scaled by the fraction
// of distinct query terms the document matched. The two-document fixture is
// small enough to derive by hand — both documents have length 2, exactly the
// corpus average, so every length norm is exactly 1 and the score reduces to
// the idf sum times coverage: doc 1 matches "red" (df 1) and "green" (df 2)
// with full coverage; doc 2 matches only "green" at half coverage.
func textScoresAreBM25TimesCoverage[P float32 | float64](t *testing.T) {
	t.Helper()
	idx := index.NewBTreeIndex[int, P](comparator.OrderedComparator[int])
	idx.SetRelevance(relevance.NewBM25[int, P]())
	if err := idx.Insert(1, "red green"); err != nil {
		t.Fatalf("Insert(1) = %v, want nil", err)
	}
	if err := idx.Insert(2, "green blue"); err != nil {
		t.Fatalf("Insert(2) = %v, want nil", err)
	}

	keys, scores, err := idx.Search("red green", 0)
	if err != nil {
		t.Fatalf("Search = %v, want nil", err)
	}
	if want := []int{1, 2}; !reflect.DeepEqual(keys, want) {
		t.Errorf("Search keys = %v, want %v (doc 1 covers the query, doc 2 half)", keys, want)
	}
	// idf(red) = ln(1 + (2-1+0.5)/(1+0.5)) = ln 2; idf(green) = ln(1 + 0.5/2.5).
	want := []P{P(math.Log(2) + math.Log(1.2)), P(math.Log(1.2) * (1.0 / 2.0))}
	if !reflect.DeepEqual(scores, want) {
		t.Errorf("Search scores = %v, want %v (BM25 × coverage at %T)", scores, want, *new(P))
	}
}

func TestBTreeIndexScoresAreBM25TimesCoverage_float64(t *testing.T) {
	textScoresAreBM25TimesCoverage[float64](t)
}
func TestBTreeIndexScoresAreBM25TimesCoverage_float32(t *testing.T) {
	textScoresAreBM25TimesCoverage[float32](t)
}

// TestBTreeIndexSearchTopKBounds checks the k parameter: k <= 0 returns every
// match, a positive k caps the result to the top-k best matches. This is the
// same bound the graph now applies to text seeds (SeedSize), so text and vector
// seeds are gathered symmetrically.
func TestBTreeIndexSearchTopKBounds(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	// Four documents, all matching "sky", plus a term that ranks one highest.
	docs := map[int]string{
		1: "sky",
		2: "sky sky blue", // "sky" appears once as a term set, still count 1
		3: "clear sky",
		4: "sky at night",
	}
	for key, doc := range docs {
		if err := idx.Insert(key, doc); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", key, err)
		}
	}

	// k <= 0 returns all four matches.
	all, _, err := idx.Search("sky", 0)
	if err != nil {
		t.Fatalf("Search(sky, 0) = %v, want nil", err)
	}
	if len(all) != 4 {
		t.Errorf("Search(sky, 0) returned %d keys, want all 4: %v", len(all), all)
	}

	// A positive k caps the results.
	for _, k := range []int{1, 2, 3} {
		got, scores, err := idx.Search("sky", k)
		if err != nil {
			t.Fatalf("Search(sky, %d) = %v, want nil", k, err)
		}
		if len(got) != k {
			t.Errorf("Search(sky, %d) returned %d keys, want %d", k, len(got), k)
		}
		if len(scores) != len(got) {
			t.Errorf("Search(sky, %d): %d keys but %d scores", k, len(got), len(scores))
		}
	}
}

// TestBTreeIndexStemmingUnifiesInflections pins the one-tokenizer contract
// end to end: with the stemming tokenizer installed, a document indexed as
// "running" is found by the query "runs", because both sides pass through
// the same stemmer. This is what SetTokenizer exists for.
func TestBTreeIndexStemmingUnifiesInflections(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
	idx.SetTokenizer(nlp.StemmingTokenizer{})
	if err := idx.Insert(1, "the marathon runner was running at dawn"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Insert(2, "an unrelated note about harbours"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}

	keys, _, err := idx.Search("runs", 0)
	if err != nil {
		t.Fatalf("Search = %v, want nil", err)
	}
	if len(keys) != 1 || keys[0] != 1 {
		t.Fatalf("Search(runs) = %v, want the running document alone", keys)
	}
}

// TestBM25LifecycleBookkeeping pins the statistics across the whole document
// lifecycle: insert admits a length, update retires the old length before
// admitting the new (Removed precedes Indexed, so the key is never counted
// twice), and delete returns the statistics to zero — using the recorded
// length, not a re-tokenization that a tokenizer swap could skew.
func TestBM25LifecycleBookkeeping(t *testing.T) {
	model := relevance.NewBM25[int, float32]()
	idx := index.NewBTreeIndex[int, float32](comparator.OrderedComparator[int])
	idx.SetRelevance(model)

	if err := idx.Insert(1, "one two three"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if model.TotalLen() != 3 || model.Lengths()[1] != 3 {
		t.Fatalf("after insert: totalLen %d, lengths[1] %d, want 3 and 3", model.TotalLen(), model.Lengths()[1])
	}

	if err := idx.Update(1, "one two three four five"); err != nil {
		t.Fatalf("Update = %v, want nil", err)
	}
	if model.TotalLen() != 5 || model.Lengths()[1] != 5 {
		t.Fatalf("after update: totalLen %d, lengths[1] %d, want 5 and 5 (old length retired first)", model.TotalLen(), model.Lengths()[1])
	}

	if err := idx.Delete(1); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if model.TotalLen() != 0 || len(model.Lengths()) != 0 {
		t.Fatalf("after delete: totalLen %d, %d lengths, want the statistics back at zero", model.TotalLen(), len(model.Lengths()))
	}
}

// TestSearchPurity pins the query side of the Relevance contract: the same
// query twice against an untouched corpus is byte-identical — no query
// mutates the model's statistics.
func TestSearchPurity(t *testing.T) {
	for name, model := range map[string]relevance.Relevance[int, float32]{
		"match count": relevance.MatchCount[int, float32]{},
		"bm25":        relevance.NewBM25[int, float32](),
	} {
		t.Run(name, func(t *testing.T) {
			idx := index.NewBTreeIndex[int, float32](comparator.OrderedComparator[int])
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
		idx := index.NewBTreeIndex[int, float64](comparator.OrderedComparator[int])
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
