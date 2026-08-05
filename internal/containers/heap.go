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

package containers

// Item is a single entry in the Heap. Ordering is by Priority: the Heap is a
// max-heap, so the item with the largest Priority is surfaced first.
type Item[K comparable, T any] struct {
	Key      K
	Value    T
	Priority uint64
}

// Heap is a max-heap keyed by Item.Priority with O(1) key lookup, allowing
// Remove and duplicate-key handling by Key. It is not safe for concurrent use.
type Heap[K comparable, T any] struct {
	items  []Item[K, T]
	lookup map[K]int
}

// NewHeap builds a heap from the given items. The items are copied (the caller's
// slice is never retained or mutated), the key lookup is populated, and the heap
// invariant is established in O(n) via Floyd's bottom-up heapify.
//
// If two items share a Key, both are inserted; behaviour of subsequent lookups
// on that Key is undefined, so callers should pass distinct keys.
func NewHeap[K comparable, T any](items ...Item[K, T]) *Heap[K, T] {
	return NewHeapCap(len(items), items...)
}

// NewHeapCap behaves like NewHeap but pre-allocates the backing slice and lookup
// map to hold at least capacity items, so pushes up to that size do not trigger a
// reallocation. The heap stays growable: it is a hint, not a bound, and capacity
// is silently raised to len(items) when the initial items already exceed it.
func NewHeapCap[K comparable, T any](capacity int, items ...Item[K, T]) *Heap[K, T] {
	n := len(items)
	if capacity < n {
		capacity = n
	}
	h := &Heap[K, T]{
		lookup: make(map[K]int, capacity),
		items:  make([]Item[K, T], n, capacity),
	}
	copy(h.items, items)
	for i, item := range h.items {
		h.lookup[item.Key] = i
	}
	// Floyd's heapify: sift down every internal node, from the last parent up.
	for i := len(h.items)/2 - 1; i >= 0; i-- {
		h.percolateDown(i)
	}
	return h
}

// Cap reports the current capacity of the backing slice: the number of items the
// heap can hold before its next reallocation. It grows as the heap grows.
func (h *Heap[K, T]) Cap() int {
	return cap(h.items)
}

func (h *Heap[K, T]) Len() int {
	return len(h.items)
}

func (h *Heap[K, T]) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
	h.lookup[h.items[i].Key] = i
	h.lookup[h.items[j].Key] = j
}

// Push inserts item, maintaining the heap invariant. If an item with the same
// Key already exists, Push keeps whichever has the higher Priority: the existing
// item is retained when its Priority is greater, otherwise it is replaced by the
// new item. Priorities are therefore monotonically non-decreasing per key.
func (h *Heap[K, T]) Push(item Item[K, T]) {
	if i, ok := h.lookup[item.Key]; ok {
		if h.items[i].Priority > item.Priority {
			return
		}
		h.Remove(item.Key)
	}

	h.items = append(h.items, item)
	size := h.Len()
	h.lookup[item.Key] = size - 1
	h.percolateUp(size - 1)
}

func (h *Heap[K, T]) Clear() {
	if len(h.items) > 0 {
		h.items = h.items[:0]
		h.lookup = make(map[K]int)
	}
}

func (h *Heap[K, T]) Peek() *Item[K, T] {
	if h.Len() == 0 {
		return nil
	}
	return &h.items[0]
}

func (h *Heap[K, T]) Has(key K) bool {
	_, ok := h.lookup[key]
	return ok
}

// Remove a value by key
func (h *Heap[K, T]) Remove(key K) bool {
	index, ok := h.lookup[key]
	if !ok {
		return false
	}

	if index == 0 {
		i := h.Pop()
		return i != nil
	}

	delete(h.lookup, key)
	size := len(h.items)

	if index == size-1 {
		h.items = h.items[:size-1]
		return true
	}

	// replace item at index with last element of the heap
	h.remove(index, size)

	// and ensure the max-heap property (parent >= children) is respected
	parent := (index - 1) / 2
	if h.items[index].Priority > h.items[parent].Priority {
		h.percolateUp(index)
	} else {
		h.percolateDown(index)
	}

	return true
}

// remove item at index and replace by last item in the tree
func (h *Heap[K, T]) remove(index int, size int) {
	h.items[index] = h.items[size-1]
	h.items = h.items[:size-1]
	h.lookup[h.items[index].Key] = index
}

func (h *Heap[K, T]) Pop() *Item[K, T] {
	size := h.Len()

	if size == 0 {
		return nil
	}

	first := h.items[0]
	delete(h.lookup, first.Key)

	if size == 1 {
		h.items = h.items[:0]
	} else {
		h.remove(0, size)
		h.percolateDown(0)
	}
	return &first
}

// used when a child node doesn't follow the heap propoerty with its parent
// restore the heap property
func (h *Heap[K, T]) percolateUp(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if h.items[parent].Priority >= h.items[index].Priority {
			break
		}

		h.Swap(index, parent)
		index = parent
	}
}

// used when a parent node doesn't follow the heap propoerty with its parent
// restore the heap property
func (h *Heap[K, T]) percolateDown(index int) {
	size := len(h.items)
	for index < size {
		left, right := 2*index+1, 2*index+2
		largest := index

		if left < size && h.items[left].Priority > h.items[largest].Priority {
			largest = left
		}

		if right < size && h.items[right].Priority > h.items[largest].Priority {
			largest = right
		}

		if largest == index {
			break
		}

		h.Swap(index, largest)
		index = largest
	}
}
