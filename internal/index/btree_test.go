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
	"math"
	"errors"
	"reflect"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/comparator"
	"github.com/RonsenbergVI/fraise/internal/index"
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
	idx.SetRelevance(index.NewBM25[int]())
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
	idx.SetRelevance(index.NewBM25[int]())
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
