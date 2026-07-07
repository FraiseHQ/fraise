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

package containers_test

import (
	"testing"

	"github.com/RonsenbergVI/fraise/internal/containers"
)

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

// minHeap returns an empty min-heap of uint32 keys and int values.
func heap() *containers.Heap[uint32, int] {
	return containers.NewHeap[uint32, int]()
}

// drainValues pops every item off h and returns their values in pop order.
func drainValues(h *containers.Heap[uint32, int]) []uint64 {
	out := make([]uint64, 0, h.Len())
	for h.Len() > 0 {
		out = append(out, h.Pop().Priority)
	}
	return out
}

func TestPush(t *testing.T) {
	h := heap()

	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 102})
	h.Push(containers.Item[uint32, int]{Key: 2, Priority: 13})
	h.Push(containers.Item[uint32, int]{Key: 3, Priority: 1})
	h.Push(containers.Item[uint32, int]{Key: 4, Priority: 1045})

	if h.Len() != 4 {
		t.Fatalf("Expected len 4 but got %d", h.Len())
	}

	// The smallest value must sift to the top.
	if top := h.Peek(); top == nil || top.Priority != 1 {
		t.Fatalf("Expected top value 1 but got %v", top)
	}
}

// TestPushDedup covers the keep-best semantics: pushing an existing key only
// replaces its value when the new value wins the comparison.
func TestPushDedup(t *testing.T) {
	h := heap()

	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 10})

	// Smaller value for the same key is ignored.
	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 5})
	if h.Len() != 1 {
		t.Fatalf("Expected len 1 after replacing same key but got %d", h.Len())
	}
	if top := h.Peek(); top == nil || top.Priority != 10 {
		t.Fatalf("Expected value 10 after improving key but got %v", top.Priority)
	}

	// Larger value for the same key replaces the old one.
	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 20})
	if h.Len() != 1 {
		t.Fatalf("Expected len 1 after ignored push but got %d", h.Len())
	}
	if top := h.Peek(); top == nil || top.Priority != 20 {
		t.Fatalf("Expected value to stay 20 but got %v", top.Priority)
	}
}

func TestPop(t *testing.T) {

	h := heap()

	for i, v := range []int{102, 13, 1, 1045, 7} {
		h.Push(containers.Item[uint32, int]{Key: uint32(i), Priority: uint64(v)})
	}

	got := drainValues(h)
	want := []uint64{1, 7, 13, 102, 1045}
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
	h := heap()

	if item := h.Peek(); item != nil {
		t.Errorf("Expected nil peeking an empty heap but got %v", item)
	}

	h.Push(containers.Item[uint32, int]{Key: 1, Priority: 42})
	h.Push(containers.Item[uint32, int]{Key: 2, Priority: 8})

	top := h.Peek()
	if top == nil || top.Priority != 8 {
		t.Fatalf("Expected peek value 8 but got %v", top)
	}
	// Peek must not remove the item.
	if h.Len() != 2 {
		t.Errorf("Expected len 2 after peek but got %d", h.Len())
	}
}

func TestRemove(t *testing.T) {
	h := heap()
	for i, v := range []int{50, 40, 30, 20, 10} {
		h.Push(containers.Item[uint32, int]{Key: uint32(i), Priority: uint64(v)})
	}

	// Removing an interior key must keep the heap property intact.
	h.Remove(2)
	if h.Len() != 4 {
		t.Fatalf("Expected len 4 after remove but got %d", h.Len())
	}

	// Removing an absent key is a no-op.
	h.Remove(999)
	if h.Len() != 4 {
		t.Fatalf("Expected len 4 after removing absent key but got %d", h.Len())
	}

	got := drainValues(h)
	want := []uint64{10, 20, 40, 50}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("After remove, pop order[%d]: expected %d but got %d (full: %v)", i, want[i], got[i], got)
		}
	}
}
