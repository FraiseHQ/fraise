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
	"testing"

	"github.com/RonsenbergVI/fraise/internal/index"
)

func TestBTreeIndexInsertAndRetrieve(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()

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
	idx := index.NewBTreeIndex[int, float64]()
	if _, err := idx.Retrieve(99); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Retrieve(99) = %v, want ErrIndexNotFound", err)
	}
}

func TestBTreeIndexSearchEmpty(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()
	if _, err := idx.Search("anything"); !errors.Is(err, index.ErrEmptyIndex) {
		t.Errorf("Search on empty index = %v, want ErrEmptyIndex", err)
	}
}

func TestBTreeIndexSearchRanksByMatchCount(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()
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

	got, err := idx.Search("quick brown fox")
	if err != nil {
		t.Fatalf("Search = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("Search returned %d keys, want 2: %v", len(got), got)
	}
	// Both docs 1 and 2 match all three terms equally; doc 3 matches none.
	seen := map[int]bool{got[0]: true, got[1]: true}
	if !seen[1] || !seen[2] {
		t.Errorf("Search() = %v, want {1,2} in some order", got)
	}
}

func TestBTreeIndexSearchNoMatchOnNonEmptyIndex(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()
	if err := idx.Insert(1, "apples and oranges"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}

	got, err := idx.Search("nonexistent")
	if err != nil {
		t.Fatalf("Search = %v, want nil (no match is not an error)", err)
	}
	if len(got) != 0 {
		t.Errorf("Search() = %v, want empty", got)
	}
}

func TestBTreeIndexUpdateMovesPostings(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()
	if err := idx.Insert(1, "apples"); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Update(1, "oranges"); err != nil {
		t.Fatalf("Update = %v, want nil", err)
	}

	if got, err := idx.Search("apples"); err != nil || len(got) != 0 {
		t.Errorf("Search(apples) after update = (%v, %v), want (empty, nil)", got, err)
	}
	got, err := idx.Search("oranges")
	if err != nil || len(got) != 1 || got[0] != 1 {
		t.Errorf("Search(oranges) after update = (%v, %v), want ([1], nil)", got, err)
	}
}

func TestBTreeIndexUpdateMissing(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()
	if err := idx.Update(1, "x"); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Update on missing key = %v, want ErrIndexNotFound", err)
	}
}

func TestBTreeIndexDelete(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()
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
	got, err := idx.Search("shared")
	if err != nil || len(got) != 1 || got[0] != 2 {
		t.Errorf("Search(shared) after deleting doc 1 = (%v, %v), want ([2], nil)", got, err)
	}

	if err := idx.Delete(2); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	// The index itself is now empty, which is a distinct condition from "no
	// document matched the query".
	if _, err := idx.Search("shared"); !errors.Is(err, index.ErrEmptyIndex) {
		t.Errorf("Search(shared) after deleting all docs = %v, want ErrEmptyIndex", err)
	}
}

func TestBTreeIndexDeleteMissing(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()
	if err := idx.Delete(1); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Delete on missing key = %v, want ErrIndexNotFound", err)
	}
}

func TestBTreeIndexInsertOverwritesExistingKey(t *testing.T) {
	idx := index.NewBTreeIndex[int, float64]()
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
	if got, err := idx.Search("first"); err != nil || len(got) != 0 {
		t.Errorf("Search(first) after overwrite = (%v, %v), want (empty, nil)", got, err)
	}
}
