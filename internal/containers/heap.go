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

// Item is one entry in a Heap. The Heap is a max-heap ordered by Priority, so
// the item with the largest Priority comes out first.
type Item[K comparable, T any] struct {
	Key      K
	Value    T
	Priority uint64
}

// Heap is a max-heap ordered by Item.Priority. It finds items by Key in O(1),
// which Remove and duplicate-key handling rely on. It is not safe for
// concurrent use.
type Heap[K comparable, T any] struct {
	items  []Item[K, T]
	lookup map[K]int
}

// NewHeap builds a heap from items in O(n), with Floyd's bottom-up heapify. It
// copies items, so it never keeps or changes the caller's slice.
//
// Callers should pass distinct keys. If two items share a Key, both are
// inserted, and later lookups on that Key are undefined.
func NewHeap[K comparable, T any](items ...Item[K, T]) *Heap[K, T] {
	return NewHeapCap(len(items), items...)
}

// NewHeapCap is like NewHeap but pre-allocates room for at least capacity
// items, so pushes up to that size do not reallocate. The heap still grows
// past capacity, which is a hint, not a bound. A capacity below len(items) is
// raised to len(items).
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

// Cap reports how many items the heap can hold before it next reallocates. It
// grows as the heap grows.
func (h *Heap[K, T]) Cap() int {
	return cap(h.items)
}

// Len reports the number of items in the heap.
func (h *Heap[K, T]) Len() int {
	return len(h.items)
}

// Swap exchanges the items in slots i and j and keeps the key lookup pointing
// at each item's slot.
func (h *Heap[K, T]) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
	h.lookup[h.items[i].Key] = i
	h.lookup[h.items[j].Key] = j
}

// Push inserts item. If an item with the same Key exists, the one with the
// higher Priority stays. On a tie, the new item replaces the old one. So a
// key's Priority never decreases.
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

// Clear removes every item. The backing slice keeps its capacity, so a heap
// refilled to its previous size does not reallocate it.
func (h *Heap[K, T]) Clear() {
	if len(h.items) > 0 {
		h.items = h.items[:0]
		h.lookup = make(map[K]int)
	}
}

// Peek returns the highest-Priority item without removing it, or nil if the
// heap is empty.
func (h *Heap[K, T]) Peek() *Item[K, T] {
	if h.Len() == 0 {
		return nil
	}
	return &h.items[0]
}

// Has reports whether an item is stored under key. It runs in O(1).
func (h *Heap[K, T]) Has(key K) bool {
	_, ok := h.lookup[key]
	return ok
}

// Remove deletes the item stored under key and reports whether there was one.
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

	h.remove(index, size)

	// The moved item may break the max-heap property (parent >= children) in
	// either direction.
	parent := (index - 1) / 2
	if h.items[index].Priority > h.items[parent].Priority {
		h.percolateUp(index)
	} else {
		h.percolateDown(index)
	}

	return true
}

// remove moves the last item into index and shrinks the heap by one. index
// must not be the last slot. The caller drops the removed key from lookup and
// restores the heap property.
func (h *Heap[K, T]) remove(index int, size int) {
	h.items[index] = h.items[size-1]
	h.items = h.items[:size-1]
	h.lookup[h.items[index].Key] = index
}

// Pop removes and returns the highest-Priority item, or nil if the heap is
// empty.
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

// percolateUp moves the item at index up while its Priority exceeds its
// parent's, restoring the heap property above it.
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

// percolateDown moves the item at index down, swapping it with its larger
// child, until no child has a greater Priority.
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
