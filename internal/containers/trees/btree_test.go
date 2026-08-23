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

package trees_test

import (
	"errors"
	"math/rand"
	"sort"
	"testing"

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/containers/trees"
)

func newIntBTree(degree int) *trees.BTree[int, int, float64] {
	return trees.NewBTree[int, int, float64](degree, comparator.OrderedComparator[int])
}

func TestBTreeEmpty(t *testing.T) {
	bt := newIntBTree(2)
	if got := bt.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if _, found := bt.Find(1); found {
		t.Errorf("Find(1) on empty tree reported found")
	}
	if bt.Delete(1) {
		t.Errorf("Delete(1) on empty tree reported deleted")
	}
	if got := bt.Values(); len(got) != 0 {
		t.Errorf("Values() = %v, want empty", got)
	}
}

func TestBTreeInsertAndFind(t *testing.T) {
	bt := newIntBTree(2)
	values := []int{10, 20, 5, 6, 12, 30, 7, 17}

	for _, v := range values {
		if err := bt.Insert(v); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", v, err)
		}
	}
	if got := bt.Len(); got != len(values) {
		t.Errorf("Len() = %d, want %d", got, len(values))
	}

	for _, v := range values {
		got, found := bt.Find(v)
		if !found || got != v {
			t.Errorf("Find(%d) = (%d, %v), want (%d, true)", v, got, found, v)
		}
	}

	for _, missing := range []int{0, 8, 11, 100} {
		if _, found := bt.Find(missing); found {
			t.Errorf("Find(%d) reported found, want not found", missing)
		}
	}
}

func TestBTreeInsertDuplicate(t *testing.T) {
	bt := newIntBTree(2)
	if err := bt.Insert(42); err != nil {
		t.Fatalf("Insert(42) = %v, want nil", err)
	}
	if err := bt.Insert(42); !errors.Is(err, trees.ErrDuplicateValue) {
		t.Errorf("Insert(42) again = %v, want ErrDuplicateValue", err)
	}
	if got := bt.Len(); got != 1 {
		t.Errorf("Len() = %d, want 1 after duplicate insert", got)
	}
}

func TestBTreeValuesAreSorted(t *testing.T) {
	bt := newIntBTree(3)
	rng := rand.New(rand.NewSource(1))
	want := make([]int, 0, 200)
	seen := make(map[int]bool)
	for len(want) < 200 {
		v := rng.Intn(10000)
		if seen[v] {
			continue
		}
		seen[v] = true
		want = append(want, v)
		if err := bt.Insert(v); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", v, err)
		}
	}
	sort.Ints(want)

	got := bt.Values()
	if len(got) != len(want) {
		t.Fatalf("Values() has %d elements, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Values()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestBTreeDeleteLeaf(t *testing.T) {
	bt := newIntBTree(2)
	for _, v := range []int{1, 2, 3, 4, 5} {
		if err := bt.Insert(v); err != nil {
			t.Fatalf("Insert(%d) = %v", v, err)
		}
	}
	if !bt.Delete(1) {
		t.Fatalf("Delete(1) = false, want true")
	}
	if _, found := bt.Find(1); found {
		t.Errorf("Find(1) after delete reported found")
	}
	if got := bt.Len(); got != 4 {
		t.Errorf("Len() = %d, want 4", got)
	}
	if bt.Delete(1) {
		t.Errorf("Delete(1) again reported deleted")
	}
}

func TestBTreeDeleteMissing(t *testing.T) {
	bt := newIntBTree(2)
	for _, v := range []int{1, 2, 3} {
		_ = bt.Insert(v)
	}
	if bt.Delete(99) {
		t.Errorf("Delete(99) reported deleted for a value never inserted")
	}
	if got := bt.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
}

// TestBTreeDeleteAll inserts a shuffled range and deletes every value in a
// different shuffled order, checking Find/Len/Values invariants after each
// deletion. This exercises leaf deletion, predecessor/successor swaps at
// internal nodes, and the borrow/merge rebalancing paths.
func TestBTreeDeleteAll(t *testing.T) {
	const n = 300
	rng := rand.New(rand.NewSource(2))

	inserted := rng.Perm(n)
	deleted := rng.Perm(n)

	bt := newIntBTree(2)
	for _, v := range inserted {
		if err := bt.Insert(v); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", v, err)
		}
	}

	remaining := n
	for idx, v := range deleted {
		if !bt.Delete(v) {
			t.Fatalf("Delete(%d) = false, want true (step %d)", v, idx)
		}
		remaining--

		if got := bt.Len(); got != remaining {
			t.Fatalf("Len() = %d, want %d (step %d)", got, remaining, idx)
		}
		if _, found := bt.Find(v); found {
			t.Fatalf("Find(%d) reported found right after deleting it (step %d)", v, idx)
		}

		values := bt.Values()
		if len(values) != remaining {
			t.Fatalf("Values() has %d elements, want %d (step %d)", len(values), remaining, idx)
		}
		for i := 1; i < len(values); i++ {
			if values[i-1] >= values[i] {
				t.Fatalf("Values() not strictly sorted at index %d: %v (step %d)", i, values[i-1:i+1], idx)
			}
		}
	}

	if got := bt.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0 after deleting everything", got)
	}
	for _, v := range inserted {
		if _, found := bt.Find(v); found {
			t.Errorf("Find(%d) reported found after deleting everything", v)
		}
	}
}

// TestBTreeAgainstReferenceMap performs a randomized sequence of inserts and
// deletes, cross-checking BTree's membership and size against a plain Go map
// used as the reference model.
func TestBTreeAgainstReferenceMap(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	bt := newIntBTree(2)
	reference := make(map[int]bool)

	const ops = 5000
	const universe = 500
	for i := 0; i < ops; i++ {
		v := rng.Intn(universe)
		if rng.Intn(2) == 0 {
			err := bt.Insert(v)
			if reference[v] {
				if !errors.Is(err, trees.ErrDuplicateValue) {
					t.Fatalf("Insert(%d) = %v, want ErrDuplicateValue (op %d)", v, err, i)
				}
			} else {
				if err != nil {
					t.Fatalf("Insert(%d) = %v, want nil (op %d)", v, err, i)
				}
				reference[v] = true
			}
		} else {
			deleted := bt.Delete(v)
			if deleted != reference[v] {
				t.Fatalf("Delete(%d) = %v, want %v (op %d)", v, deleted, reference[v], i)
			}
			delete(reference, v)
		}

		if got, want := bt.Len(), len(reference); got != want {
			t.Fatalf("Len() = %d, want %d (op %d)", got, want, i)
		}
	}

	for v := 0; v < universe; v++ {
		_, found := bt.Find(v)
		if found != reference[v] {
			t.Errorf("Find(%d) = %v, want %v", v, found, reference[v])
		}
	}
}

func TestBTreeStringValues(t *testing.T) {
	bt := trees.NewBTree[int, string, float64](2, comparator.OrderedComparator[string])
	words := []string{"pear", "apple", "grape", "fig", "date", "banana", "kiwi"}
	for _, w := range words {
		if err := bt.Insert(w); err != nil {
			t.Fatalf("Insert(%q) = %v, want nil", w, err)
		}
	}

	sorted := append([]string(nil), words...)
	sort.Strings(sorted)
	if got := bt.Values(); !equalStrings(got, sorted) {
		t.Errorf("Values() = %v, want %v", got, sorted)
	}

	if _, found := bt.Find("banana"); !found {
		t.Errorf("Find(%q) reported not found", "banana")
	}
	if !bt.Delete("banana") {
		t.Errorf("Delete(%q) reported not deleted", "banana")
	}
	if _, found := bt.Find("banana"); found {
		t.Errorf("Find(%q) reported found after delete", "banana")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
