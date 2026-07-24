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
	"math/rand"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/index"
)

func randVector(rng *rand.Rand, dim int) containers.Vector[float64] {
	data := make([]float64, dim)
	for i := range data {
		data[i] = rng.Float64() * 100
	}
	return containers.NewVector(data)
}

func TestRPTreeIndexInsertAndRetrieve(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1)
	v := containers.NewVector([]float64{1, 2, 3})

	if err := idx.Insert(1, v); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if got, want := idx.Count(), 1; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}

	got, err := idx.Retrieve(1)
	if err != nil {
		t.Fatalf("Retrieve = %v, want nil", err)
	}
	if got.Dim() != v.Dim() || got.Data[0] != v.Data[0] {
		t.Errorf("Retrieve() = %v, want %v", got, v)
	}
}

func TestRPTreeIndexInsertRejectsWrongDimension(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1)
	if err := idx.Insert(1, containers.NewVector([]float64{1, 2})); !errors.Is(err, index.ErrInvalidDimension) {
		t.Errorf("Insert with wrong dimension = %v, want ErrInvalidDimension", err)
	}
	if got, want := idx.Count(), 0; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
}

func TestRPTreeIndexRetrieveMissing(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1)
	if _, err := idx.Retrieve(1); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Retrieve(1) = %v, want ErrIndexNotFound", err)
	}
}

func TestRPTreeIndexSearchEmpty(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1)
	if _, err := idx.Search(containers.NewVector([]float64{0, 0, 0}), 3); !errors.Is(err, index.ErrEmptyIndex) {
		t.Errorf("Search on empty index = %v, want ErrEmptyIndex", err)
	}
}

func TestRPTreeIndexSearchRejectsWrongDimension(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1)
	if err := idx.Insert(1, containers.NewVector([]float64{1, 2, 3})); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if _, err := idx.Search(containers.NewVector([]float64{1, 2}), 3); !errors.Is(err, index.ErrInvalidDimension) {
		t.Errorf("Search with wrong dimension = %v, want ErrInvalidDimension", err)
	}
}

// TestRPTreeIndexSearchFindsNearestCluster keeps the corpus small (under a
// single tree leaf) and checks that querying near a tight cluster of vectors
// returns that cluster's keys ahead of a far-away outlier.
func TestRPTreeIndexSearchFindsNearestCluster(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](2, 4, 5, 7)

	cluster := map[int][]float64{
		1: {0, 0},
		2: {1, 0},
		3: {0, 1},
	}
	for key, coord := range cluster {
		if err := idx.Insert(key, containers.NewVector(coord)); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", key, err)
		}
	}
	if err := idx.Insert(4, containers.NewVector([]float64{1000, 1000})); err != nil {
		t.Fatalf("Insert(4) = %v, want nil", err)
	}

	got, err := idx.Search(containers.NewVector([]float64{0, 0}), 3)
	if err != nil {
		t.Fatalf("Search = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Fatalf("Search returned %d keys, want 3: %v", len(got), got)
	}
	for _, key := range got {
		if key == 4 {
			t.Errorf("Search() = %v, the far outlier (key 4) should not be among the 3 nearest", got)
		}
	}
}

func TestRPTreeIndexUpdateAndDelete(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](2, 4, 3, 1)
	if err := idx.Insert(1, containers.NewVector([]float64{1, 1})); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}

	if err := idx.Update(1, containers.NewVector([]float64{9, 9})); err != nil {
		t.Fatalf("Update = %v, want nil", err)
	}
	got, err := idx.Retrieve(1)
	if err != nil || got.Data[0] != 9 {
		t.Errorf("Retrieve(1) after update = (%v, %v), want vector starting with 9", got, err)
	}

	if err := idx.Update(99, containers.NewVector([]float64{0, 0})); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Update on missing key = %v, want ErrIndexNotFound", err)
	}

	if err := idx.Delete(1); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if got, want := idx.Count(), 0; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
	if _, err := idx.Retrieve(1); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Retrieve(1) after delete = %v, want ErrIndexNotFound", err)
	}
	if err := idx.Delete(1); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Delete on already-deleted key = %v, want ErrIndexNotFound", err)
	}
}

// TestRPTreeIndexSearchIgnoresDeletedVectors checks that a deleted vector,
// even though it still physically sits inside the forest until Flush, is
// filtered out of Search results by the vectors ground truth.
func TestRPTreeIndexSearchIgnoresDeletedVectors(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](2, 4, 5, 3)
	if err := idx.Insert(1, containers.NewVector([]float64{0, 0})); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Insert(2, containers.NewVector([]float64{0.1, 0.1})); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Delete(1); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}

	got, err := idx.Search(containers.NewVector([]float64{0, 0}), 5)
	if err != nil {
		t.Fatalf("Search = %v, want nil", err)
	}
	for _, key := range got {
		if key == 1 {
			t.Errorf("Search() = %v, deleted key 1 should not appear", got)
		}
	}
}

func TestRPTreeIndexFlushRebuildsForest(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 5)

	const n = 50
	for i := 0; i < n; i++ {
		if err := idx.Insert(i, randVector(rng, 3)); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}
	for i := 0; i < n/2; i++ {
		if err := idx.Delete(i); err != nil {
			t.Fatalf("Delete(%d) = %v, want nil", i, err)
		}
	}

	if err := idx.Flush(); err != nil {
		t.Fatalf("Flush = %v, want nil", err)
	}
	if got, want := idx.Count(), n-n/2; got != want {
		t.Errorf("Count() after Flush = %d, want %d", got, want)
	}

	// After Flush, deleted keys must be absent even from raw Nearest scans:
	// Search over the whole remaining corpus should never surface them.
	got, err := idx.Search(randVector(rng, 3), n)
	if err != nil {
		t.Fatalf("Search = %v, want nil", err)
	}
	for _, key := range got {
		if key < n/2 {
			t.Errorf("Search() after Flush returned deleted key %d", key)
		}
	}
}

func TestRPTreeIndexSize(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1)
	if got := idx.Size(); got != 0 {
		t.Errorf("Size() on empty index = %d, want 0", got)
	}
	for i := 0; i < 100; i++ {
		if err := idx.Insert(i, containers.NewVector([]float64{1, 2, 3})); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}
	if got := idx.Size(); got < 0 {
		t.Errorf("Size() = %d, want >= 0", got)
	}
}
