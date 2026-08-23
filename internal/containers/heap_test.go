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

// Black-box tests (package containers_test) for the indexed max-heap. Since the
// items slice and lookup map are unexported, correctness is verified through the
// public API only: the item with the largest Priority is always surfaced first,
// draining yields a non-increasing sequence, and Len/Has/Remove stay consistent
// with an independent reference model.
package containers_test

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/FraiseHQ/fraise/internal/containers"
)

// ---- helpers ---------------------------------------------------------------

// newHeap returns an empty max-heap of uint32 keys and int values.
func newHeap() *containers.Heap[uint32, int] {
	return containers.NewHeap[uint32, int]()
}

// drainPriorities pops every item off h and returns their priorities in pop order.
func drainPriorities(h *containers.Heap[uint32, int]) []uint64 {
	out := make([]uint64, 0, h.Len())
	for h.Len() > 0 {
		out = append(out, h.Pop().Priority)
	}
	return out
}

func sItem(key string, pri uint64) containers.Item[string, string] {
	return containers.Item[string, string]{Key: key, Value: key + "-val", Priority: pri}
}

// assertNonIncreasing fails if the priorities are not in max-heap pop order.
func assertNonIncreasing(t *testing.T, got []uint64) {
	t.Helper()
	for i := 1; i < len(got); i++ {
		if got[i] > got[i-1] {
			t.Fatalf("pop order not non-increasing at %d: %d after %d (full: %v)",
				i, got[i], got[i-1], got)
		}
	}
}

// ---- behaviour tests -------------------------------------------------------

func TestLen(t *testing.T) {
	data := []containers.Item[uint32, int]{
		{Key: 1, Priority: 102},
		{Key: 2, Priority: 13},
		{Key: 222, Priority: 1},
		{Key: 3333, Priority: 1045},
	}

	h := containers.NewHeap(data...)

	if h.Len() != 4 {
		t.Errorf("Expected 4 but got %d", h.Len())
	}
}

func TestPush(t *testing.T) {
	h := newHeap()

	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 102})
	h.Push(containers.Item[uint32, int]{Key: 2, Priority: 13})
	h.Push(containers.Item[uint32, int]{Key: 3, Priority: 1})
	h.Push(containers.Item[uint32, int]{Key: 4, Priority: 1045})

	if h.Len() != 4 {
		t.Fatalf("Expected len 4 but got %d", h.Len())
	}

	// The largest priority must sift to the top.
	if top := h.Peek(); top == nil || top.Priority != 1045 {
		t.Fatalf("Expected top priority 1045 but got %v", top)
	}
}

// TestPushDedup covers the keep-best semantics: pushing an existing key only
// replaces its value when the new priority wins the comparison.
func TestPushDedup(t *testing.T) {
	h := newHeap()

	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 10})

	// Smaller priority for the same key is ignored.
	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 5})
	if h.Len() != 1 {
		t.Fatalf("Expected len 1 after ignored push but got %d", h.Len())
	}
	if top := h.Peek(); top == nil || top.Priority != 10 {
		t.Fatalf("Expected priority to stay 10 but got %v", top.Priority)
	}

	// Larger priority for the same key replaces the old one.
	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 20})
	if h.Len() != 1 {
		t.Fatalf("Expected len 1 after replacing same key but got %d", h.Len())
	}
	if top := h.Peek(); top == nil || top.Priority != 20 {
		t.Fatalf("Expected priority 20 after replacing key but got %v", top.Priority)
	}
}

func TestPop(t *testing.T) {
	h := newHeap()

	for i, v := range []int{102, 13, 1, 1045, 7} {
		h.Push(containers.Item[uint32, int]{Key: uint32(i), Priority: uint64(v)})
	}

	got := drainPriorities(h)
	want := []uint64{1045, 102, 13, 7, 1}
	if len(got) != len(want) {
		t.Fatalf("Expected %d popped items but got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Pop order[%d]: expected %d but got %d (full: %v)", i, want[i], got[i], got)
		}
	}

	if h.Len() != 0 {
		t.Errorf("Expected empty heap after draining but len is %d", h.Len())
	}
	if item := h.Pop(); item != nil {
		t.Errorf("Expected nil popping an empty heap but got %v", item)
	}
}

func TestPeek(t *testing.T) {
	h := newHeap()

	if item := h.Peek(); item != nil {
		t.Errorf("Expected nil peeking an empty heap but got %v", item)
	}

	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 42})
	h.Push(containers.Item[uint32, int]{Key: 2, Priority: 8})

	top := h.Peek()
	if top == nil || top.Priority != 42 {
		t.Fatalf("Expected peek priority 42 but got %v", top)
	}
	// Peek must not remove the item.
	if h.Len() != 2 {
		t.Errorf("Expected len 2 after peek but got %d", h.Len())
	}
}

func TestRemove(t *testing.T) {
	h := newHeap()
	for i, v := range []int{50, 40, 30, 20, 10} {
		h.Push(containers.Item[uint32, int]{Key: uint32(i), Priority: uint64(v)})
	}

	// Removing an interior key must keep the heap property intact.
	if !h.Remove(2) {
		t.Fatal("Remove(2) = false, want true")
	}
	if h.Len() != 4 {
		t.Fatalf("Expected len 4 after remove but got %d", h.Len())
	}

	// Removing an absent key is a no-op.
	if h.Remove(999) {
		t.Fatal("Remove(999) = true on absent key, want false")
	}
	if h.Len() != 4 {
		t.Fatalf("Expected len 4 after removing absent key but got %d", h.Len())
	}

	got := drainPriorities(h)
	want := []uint64{50, 40, 20, 10}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("After remove, pop order[%d]: expected %d but got %d (full: %v)", i, want[i], got[i], got)
		}
	}
}

// ---- constructor tests -----------------------------------------------------

func TestNewHeap_EmptyAndSingle(t *testing.T) {
	empty := containers.NewHeap[string, string]()
	if empty.Len() != 0 || empty.Peek() != nil || empty.Pop() != nil {
		t.Fatal("empty heap should have Len 0 and nil Peek/Pop")
	}

	one := containers.NewHeap(sItem("a", 7))
	if p := one.Peek(); p == nil || p.Key != "a" || !one.Has("a") {
		t.Fatalf("single-item heap wrong: peek=%v has=%v", p, one.Has("a"))
	}
}

// TestNewHeap_Heapifies is the regression test for the original constructor
// bug: NewHeap stored items verbatim with an empty lookup, so an unsorted input
// left a non-heap that Peek/Pop/Has read incorrectly.
func TestNewHeap_Heapifies(t *testing.T) {
	h := containers.NewHeap(
		sItem("a", 9), sItem("b", 3), sItem("c", 7),
		sItem("d", 1), sItem("e", 8), sItem("f", 5),
	)

	if p := h.Peek(); p == nil || p.Priority != 9 {
		t.Fatalf("Peek() priority = %v, want 9 (the max)", p)
	}
	for _, k := range []string{"a", "b", "c", "d", "e", "f"} {
		if !h.Has(k) {
			t.Fatalf("Has(%q)=false after NewHeap", k)
		}
	}

	// Draining a heapified input must yield fully ordered output.
	got := make([]uint64, 0, h.Len())
	for h.Len() > 0 {
		got = append(got, h.Pop().Priority)
	}
	assertNonIncreasing(t, got)
	if len(got) != 6 {
		t.Fatalf("drained %d items, want 6", len(got))
	}
}

// TestNewHeap_DoesNotAliasInput guards the slice-aliasing fix: mutating the
// caller's slice after construction must not disturb the heap, and heap
// operations must not write back into the caller's slice.
func TestNewHeap_DoesNotAliasInput(t *testing.T) {
	src := []containers.Item[string, string]{sItem("a", 5), sItem("b", 2), sItem("c", 8)}
	h := containers.NewHeap(src...)

	for i := range src {
		src[i].Priority = 999 // mutate caller's backing array
	}
	if p := h.Peek(); p == nil || p.Priority != 8 {
		t.Fatalf("Peek() priority = %v after mutating source, want 8", p)
	}

	h.Pop() // reorder the heap
	for i := range src {
		if src[i].Priority != 999 {
			t.Fatalf("heap wrote back into caller slice at %d: %d", i, src[i].Priority)
		}
	}
}

// TestRemove_MiddleReheapifies removes interior nodes from a larger heap so the
// replacement element must sometimes sift up and sometimes sift down, verifying
// the heap stays ordered (observed via a non-increasing drain) after removal.
func TestRemove_MiddleReheapifies(t *testing.T) {
	build := func() *containers.Heap[uint32, int] {
		h := newHeap()
		for i, p := range []uint64{50, 10, 40, 5, 30, 35, 20, 1, 3, 25, 8, 45} {
			h.Push(containers.Item[uint32, int]{Key: uint32(i), Priority: p})
		}
		return h
	}

	for _, key := range []uint32{2, 4, 5, 6, 9, 11} {
		h := build()
		before := h.Len()
		if !h.Remove(key) {
			t.Fatalf("Remove(%d) = false", key)
		}
		if h.Len() != before-1 || h.Has(key) {
			t.Fatalf("Remove(%d): len=%d has=%v", key, h.Len(), h.Has(key))
		}
		assertNonIncreasing(t, drainPriorities(h))
	}
}

func TestClear_ThenReuse(t *testing.T) {
	h := containers.NewHeap(
		containers.Item[uint32, int]{Key: 1, Priority: 1},
		containers.Item[uint32, int]{Key: 2, Priority: 2},
	)
	h.Clear()
	if h.Len() != 0 || h.Has(1) || h.Has(2) {
		t.Fatal("Clear did not empty the heap and lookup")
	}
	h.Push(containers.Item[uint32, int]{Key: 3, Priority: 3})
	if p := h.Peek(); p == nil || p.Key != 3 {
		t.Fatalf("Peek()=%v after reuse, want key 3", p)
	}
}

// ---- randomized property tests ---------------------------------------------

// TestFuzz_AgainstReferenceModel drives a long random sequence of operations
// and cross-checks the heap against a plain map used as an oracle, verifying
// after every step that Len/Peek stay consistent and that Pop always surfaces a
// true maximum-priority key. Because Peek/Pop/Has/Remove all consult the
// internal lookup map, agreement with the model exercises its consistency.
func TestFuzz_AgainstReferenceModel(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	h := containers.NewHeap[int, int]()
	model := map[int]uint64{} // key -> current priority

	maxPri := func() (uint64, bool) {
		best, ok := uint64(0), false
		for _, p := range model {
			if !ok || p > best {
				best, ok = p, true
			}
		}
		return best, ok
	}

	const steps = 20000
	for i := 0; i < steps; i++ {
		switch rng.Intn(4) {
		case 0, 1: // Push (weighted so the heap tends to grow)
			key := rng.Intn(200)
			pri := uint64(rng.Intn(1000))
			h.Push(containers.Item[int, int]{Key: key, Value: key, Priority: pri})
			if cur, ok := model[key]; !ok || pri >= cur { // mirror "higher wins"
				model[key] = pri
			}
		case 2: // Pop
			want, ok := maxPri()
			got := h.Pop()
			if ok != (got != nil) {
				t.Fatalf("step %d: Pop presence mismatch (model has=%v got=%v)", i, ok, got)
			}
			if got != nil {
				if got.Priority != want {
					t.Fatalf("step %d: Pop priority=%d, want max=%d", i, got.Priority, want)
				}
				delete(model, got.Key)
			}
		case 3: // Remove present-or-absent key
			key := rng.Intn(200)
			_, inModel := model[key]
			if got := h.Remove(key); got != inModel {
				t.Fatalf("step %d: Remove(%d)=%v, model present=%v", i, key, got, inModel)
			}
			delete(model, key)
		}

		if h.Len() != len(model) {
			t.Fatalf("step %d: Len()=%d, model size=%d", i, h.Len(), len(model))
		}
		if want, ok := maxPri(); ok {
			if p := h.Peek(); p == nil || p.Priority != want {
				t.Fatalf("step %d: Peek()=%v, want max priority %d", i, p, want)
			}
		} else if h.Peek() != nil {
			t.Fatalf("step %d: Peek() != nil but model empty", i)
		}
	}
}

// TestFuzz_HeapSort verifies the classic property: pushing arbitrary items and
// popping them all yields a non-increasing priority sequence. Keys are unique
// so no dedup interferes with the count.
func TestFuzz_HeapSort(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for trial := 0; trial < 200; trial++ {
		n := rng.Intn(300)
		h := containers.NewHeap[int, int]()
		want := make([]uint64, 0, n)
		for i := 0; i < n; i++ {
			pri := uint64(rng.Intn(10000))
			h.Push(containers.Item[int, int]{Key: i, Value: i, Priority: pri})
			want = append(want, pri)
		}
		sort.Slice(want, func(i, j int) bool { return want[i] > want[j] }) // descending

		got := make([]uint64, 0, n)
		for h.Len() > 0 {
			got = append(got, h.Pop().Priority)
		}
		if len(got) != len(want) {
			t.Fatalf("trial %d: popped %d items, want %d", trial, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("trial %d: pop[%d]=%d, want %d", trial, i, got[i], want[i])
			}
		}
	}
}
