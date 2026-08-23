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
	"reflect"
	"testing"

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/index"
)

func randVector(rng *rand.Rand, dim int) containers.Vector[int, float64] {
	data := make([]float64, dim)
	for i := range data {
		data[i] = rng.Float64() * 100
	}
	return containers.NewVector[int](data)
}

func TestRPTreeIndexInsertAndRetrieve(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1, 2, comparator.OrderedComparator[int])
	v := containers.NewVector[int]([]float64{1, 2, 3})

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
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1, 2, comparator.OrderedComparator[int])
	if err := idx.Insert(1, containers.NewVector[int]([]float64{1, 2})); !errors.Is(err, index.ErrInvalidDimension) {
		t.Errorf("Insert with wrong dimension = %v, want ErrInvalidDimension", err)
	}
	if got, want := idx.Count(), 0; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
}

func TestRPTreeIndexRetrieveMissing(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1, 2, comparator.OrderedComparator[int])
	if _, err := idx.Retrieve(1); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("Retrieve(1) = %v, want ErrIndexNotFound", err)
	}
}

func TestRPTreeIndexSearchEmpty(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1, 2, comparator.OrderedComparator[int])
	if _, _, err := idx.Search(containers.NewVector[int]([]float64{0, 0, 0}), 3); !errors.Is(err, index.ErrEmptyIndex) {
		t.Errorf("Search on empty index = %v, want ErrEmptyIndex", err)
	}
}

func TestRPTreeIndexSearchRejectsWrongDimension(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1, 2, comparator.OrderedComparator[int])
	if err := idx.Insert(1, containers.NewVector[int]([]float64{1, 2, 3})); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if _, _, err := idx.Search(containers.NewVector[int]([]float64{1, 2}), 3); !errors.Is(err, index.ErrInvalidDimension) {
		t.Errorf("Search with wrong dimension = %v, want ErrInvalidDimension", err)
	}
}

// TestRPTreeIndexSearchFindsNearestCluster keeps the corpus small (under a
// single tree leaf) and checks that querying near a tight cluster of vectors
// returns that cluster's keys ahead of a far-away outlier.
func TestRPTreeIndexSearchFindsNearestCluster(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](2, 4, 5, 7, 2, comparator.OrderedComparator[int])

	cluster := map[int][]float64{
		1: {0, 0},
		2: {1, 0},
		3: {0, 1},
	}
	for key, coord := range cluster {
		if err := idx.Insert(key, containers.NewVector[int](coord)); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", key, err)
		}
	}
	if err := idx.Insert(4, containers.NewVector[int]([]float64{1000, 1000})); err != nil {
		t.Fatalf("Insert(4) = %v, want nil", err)
	}

	got, _, err := idx.Search(containers.NewVector[int]([]float64{0, 0}), 3)
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

// TestRPTreeIndexSearchOrdersTiesByKey pins the tiebreak that makes the ranking
// a total order: vectors sitting at the same distance from the query are ranked
// by key. Distance alone leaves them in the order the forest pooled them in —
// an artefact of how each tree's priority queue happens to break ties — and
// that order decides which of them survives truncation to k. The corpus stays
// under a single leaf so every candidate reaches the re-rank.
func TestRPTreeIndexSearchOrdersTiesByKey(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](2, 4, 5, 7, 2, comparator.OrderedComparator[int])

	// The four unit vectors are all exactly distance 1 from the origin query,
	// and their keys are inserted out of order.
	equidistant := []struct {
		key   int
		coord []float64
	}{
		{7, []float64{1, 0}},
		{3, []float64{0, 1}},
		{9, []float64{-1, 0}},
		{1, []float64{0, -1}},
	}
	for _, v := range equidistant {
		if err := idx.Insert(v.key, containers.NewVector[int](v.coord)); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", v.key, err)
		}
	}

	got, scores, err := idx.Search(containers.NewVector[int]([]float64{0, 0}), 4)
	if err != nil {
		t.Fatalf("Search = %v, want nil", err)
	}
	if want := []float64{1, 1, 1, 1}; !reflect.DeepEqual(scores, want) {
		t.Fatalf("Search scores = %v, want %v — the four vectors must tie for the tiebreak to be under test", scores, want)
	}
	if want := []int{1, 3, 7, 9}; !reflect.DeepEqual(got, want) {
		t.Errorf("Search() = %v, want %v (equidistant vectors ranked by key)", got, want)
	}
}

func TestRPTreeIndexUpdateAndDelete(t *testing.T) {
	idx := index.NewRPTreeIndex[int, float64](2, 4, 3, 1, 2, comparator.OrderedComparator[int])
	if err := idx.Insert(1, containers.NewVector[int]([]float64{1, 1})); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}

	if err := idx.Update(1, containers.NewVector[int]([]float64{9, 9})); err != nil {
		t.Fatalf("Update = %v, want nil", err)
	}
	got, err := idx.Retrieve(1)
	if err != nil || got.Data[0] != 9 {
		t.Errorf("Retrieve(1) after update = (%v, %v), want vector starting with 9", got, err)
	}

	if err := idx.Update(99, containers.NewVector[int]([]float64{0, 0})); !errors.Is(err, index.ErrIndexNotFound) {
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
	idx := index.NewRPTreeIndex[int, float64](2, 4, 5, 3, 2, comparator.OrderedComparator[int])
	if err := idx.Insert(1, containers.NewVector[int]([]float64{0, 0})); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Insert(2, containers.NewVector[int]([]float64{0.1, 0.1})); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}
	if err := idx.Delete(1); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}

	got, _, err := idx.Search(containers.NewVector[int]([]float64{0, 0}), 5)
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
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 5, 2, comparator.OrderedComparator[int])

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
	got, _, err := idx.Search(randVector(rng, 3), n)
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
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 1, 2, comparator.OrderedComparator[int])
	if got := idx.Size(); got != 0 {
		t.Errorf("Size() on empty index = %d, want 0", got)
	}
	for i := 0; i < 100; i++ {
		if err := idx.Insert(i, containers.NewVector[int]([]float64{1, 2, 3})); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}
	if got := idx.Size(); got < 0 {
		t.Errorf("Size() = %d, want >= 0", got)
	}
}

// ranksTenByDistance indexes ten vectors whose distances to the query are all
// distinct and known ahead of time, then checks that Search returns them
// nearest-first and that the returned scores are exactly those distances.
//
// It is generic over P so the identical assertions run for both float32 and
// float64: key i sits at (i, 0) and the query is the origin, so key i is exactly
// distance i away — a small integer representable exactly in either precision,
// which is what lets the score equality hold at both.
//
// Ten points sit well under a single leaf (defaultRPLeafSize is 32), so every
// tree in the forest holds the whole corpus and Search re-ranks the pooled
// candidates by true distance: the result is exact, not approximate.
func ranksTenByDistance[P float32 | float64](t *testing.T) {
	t.Helper()
	idx := index.NewRPTreeIndex[int, P](2, 2, 5, 7, 2, comparator.OrderedComparator[int])

	for i := 1; i <= 10; i++ {
		if err := idx.Insert(i, containers.NewVector[int]([]P{P(i), 0})); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}
	if got, want := idx.Count(), 10; got != want {
		t.Fatalf("Count() = %d, want %d", got, want)
	}

	query := containers.NewVector[int]([]P{0, 0})

	// Full search returns every key, nearest first: 1, 2, ..., 10.
	got, scores, err := idx.Search(query, 10)
	if err != nil {
		t.Fatalf("Search(k=10) = %v, want nil", err)
	}
	want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Search(k=10) order = %v, want %v", got, want)
	}
	if len(scores) != len(got) {
		t.Fatalf("Search returned %d keys but %d scores; slices must be parallel", len(got), len(scores))
	}

	// Each returned score is the actual distance driving the ranking: key i is
	// exactly distance i from the origin, and scores strictly increase.
	var prev P = -1
	for rank, key := range got {
		if want := P(key); scores[rank] != want {
			t.Errorf("rank %d, key %d: score = %v, want distance %v", rank, key, scores[rank], want)
		}
		if scores[rank] <= prev {
			t.Errorf("rank %d: score %v not greater than previous %v; not ordered by distance", rank, scores[rank], prev)
		}
		prev = scores[rank]
	}

	// A smaller k truncates to exactly the k nearest, with their distances.
	got3, scores3, err := idx.Search(query, 3)
	if err != nil {
		t.Fatalf("Search(k=3) = %v, want nil", err)
	}
	if want3 := []int{1, 2, 3}; !reflect.DeepEqual(got3, want3) {
		t.Errorf("Search(k=3) keys = %v, want %v", got3, want3)
	}
	if want3 := []P{1, 2, 3}; !reflect.DeepEqual(scores3, want3) {
		t.Errorf("Search(k=3) scores = %v, want %v", scores3, want3)
	}
}

func TestRPTreeIndexSearchRanksTenByDistance_float64(t *testing.T) { ranksTenByDistance[float64](t) }
func TestRPTreeIndexSearchRanksTenByDistance_float32(t *testing.T) { ranksTenByDistance[float32](t) }

// nearestKeys indexes pts (integer coordinates, exact in both precisions) at
// precision P and returns the keys of the k nearest to query, nearest first.
func nearestKeys[P float32 | float64](pts [][]int, query []int, k int) []int {
	idx := index.NewRPTreeIndex[int, P](len(query), 2, 5, 7, 2, comparator.OrderedComparator[int])
	for i, p := range pts {
		vec := make([]P, len(p))
		for d, v := range p {
			vec[d] = P(v)
		}
		_ = idx.Insert(i, containers.NewVector[int](vec))
	}
	q := make([]P, len(query))
	for d, v := range query {
		q[d] = P(v)
	}
	keys, _, _ := idx.Search(containers.NewVector[int](q), k)
	return keys
}

// TestRPTreeSearchIdenticalAcrossPrecision indexes the same well-separated
// corpus at float32 and float64 and asserts both return the identical
// nearest-neighbour ordering. Precision changes the stored coordinate width,
// not which neighbours a search finds for a cleanly separated corpus — this is
// the "results are the same at both precisions" guarantee.
func TestRPTreeSearchIdenticalAcrossPrecision(t *testing.T) {
	pts := [][]int{{0, 0}, {3, 1}, {1, 4}, {8, 8}, {2, 2}, {9, 1}, {5, 7}, {6, 3}}
	k := len(pts)

	got32 := nearestKeys[float32](pts, []int{0, 0}, k)
	got64 := nearestKeys[float64](pts, []int{0, 0}, k)

	if !reflect.DeepEqual(got32, got64) {
		t.Errorf("nearest-neighbour order differs by precision:\n float32 = %v\n float64 = %v", got32, got64)
	}
}

// TestRPTreeIndexInsertIdempotent checks that re-inserting a key with its
// current vector never grows the forest. This is the regression guard for the
// quadratic bloat bug: Graph.MergeFrom replays the whole vector set into the
// live index after every staged write, so a non-idempotent Insert turned W
// writes into O(W^2) forest entries.
func TestRPTreeIndexInsertIdempotent(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 5, 2, comparator.OrderedComparator[int])

	const n = 20
	vecs := make([]containers.Vector[int, float64], n)
	for i := 0; i < n; i++ {
		vecs[i] = randVector(rng, 3)
		if err := idx.Insert(i, vecs[i]); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}
	if got := idx.Entries(); got != n {
		t.Fatalf("Entries() after %d inserts = %d, want %d", n, got, n)
	}

	// Replay the full set 50 times — the MergeFrom pattern. Forest must not grow.
	for round := 0; round < 50; round++ {
		for i := 0; i < n; i++ {
			if err := idx.Insert(i, vecs[i]); err != nil {
				t.Fatalf("re-Insert(%d) = %v, want nil", i, err)
			}
		}
	}
	if got := idx.Entries(); got != n {
		t.Errorf("Entries() after 50 replays = %d, want %d (forest must not grow on re-insert)", got, n)
	}
	if got := idx.Count(); got != n {
		t.Errorf("Count() = %d, want %d", got, n)
	}
}

// TestRPTreeIndexForestBounded checks the automatic compaction: sustained
// updates and deletes leave garbage in the forest, but Entries must stay
// within the flushFactor bound of the live count instead of growing forever.
func TestRPTreeIndexForestBounded(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	idx := index.NewRPTreeIndex[int, float64](3, 4, 3, 5, 2, comparator.OrderedComparator[int])

	const n = 30
	for i := 0; i < n; i++ {
		if err := idx.Insert(i, randVector(rng, 3)); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}

	// 500 in-place updates with fresh vectors: each appends one garbage entry.
	for round := 0; round < 500; round++ {
		key := round % n
		if err := idx.Update(key, randVector(rng, 3)); err != nil {
			t.Fatalf("Update(%d) = %v, want nil", key, err)
		}
	}
	if got, bound := idx.Entries(), 2*idx.Count(); got > bound {
		t.Errorf("Entries() after 500 updates = %d, want <= %d (auto-flush bound)", got, bound)
	}

	// Delete everything: compaction must reclaim the forest as well.
	for i := 0; i < n; i++ {
		if err := idx.Delete(i); err != nil {
			t.Fatalf("Delete(%d) = %v, want nil", i, err)
		}
	}
	if got := idx.Entries(); got != 0 {
		t.Errorf("Entries() after deleting all = %d, want 0", got)
	}

	// The index must still work after repeated compactions.
	if err := idx.Insert(1000, randVector(rng, 3)); err != nil {
		t.Fatalf("Insert after compactions = %v, want nil", err)
	}
	keys, _, err := idx.Search(randVector(rng, 3), 1)
	if err != nil || len(keys) != 1 || keys[0] != 1000 {
		t.Errorf("Search after compactions = (%v, err=%v), want key 1000", keys, err)
	}
}
