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

type CompareFunc func(a, b uint64) int

type Item[K comparable, T any] struct {
	Key      K
	Value    T
	Priority uint64
}

type Heap[K comparable, T any] struct {
	items  []Item[K, T]
	lookup map[K]int
}

func NewHeap[K comparable, T any](items ...Item[K, T]) *Heap[K, T] {
	return &Heap[K, T]{
		lookup: make(map[K]int),
		items:  items,
	}
}

func (h *Heap[K, T]) Len() int {
	return len(h.items)
}

func (h *Heap[K, T]) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
	h.lookup[h.items[i].Key] = i
	h.lookup[h.items[j].Key] = j
}

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
	h.siftUp(size - 1)
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

	// and ensure the property of the data structure (parent > children) is respected
	parent := (index - 1) / 2
	if h.items[parent].Priority > h.items[index].Priority {
		h.siftUp(index)
	} else {
		h.siftDown(index)
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
		h.siftDown(0)
	}
	return &first
}

func (h *Heap[K, V]) siftUp(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if h.items[parent].Priority <= h.items[index].Priority {
			break
		}

		h.Swap(index, parent)
		index = parent
	}
}

func (h *Heap[K, V]) siftDown(index int) {
	size := len(h.items)
	for index < size {
		left, right := 2*index+1, 2*index+2
		parent := index

		if left < size && h.items[left].Priority < h.items[parent].Priority {
			parent = left
		}

		if right < size && h.items[right].Priority < h.items[parent].Priority {
			parent = right
		}

		if parent == index {
			break
		}

		h.Swap(index, parent)
		index = parent
	}
}
