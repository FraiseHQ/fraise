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

import (
	"sync"
)

// PriorityQueue is a growable max-priority queue: Dequeue and Peek surface the
// highest-Priority item. capacity is only a size hint for pre-allocating the
// backing store: the queue grows past it rather than evicting (Cap reports the
// live capacity). Keys are unique, as in Heap: enqueueing a key already queued
// keeps whichever item has the higher Priority.
type PriorityQueue[K comparable, T any] struct {
	h        *Heap[K, T]
	capacity uint
	mu       sync.RWMutex
}

// NewPriorityQueue returns a queue pre-sized for capacity items and seeded with
// items, which should carry distinct keys as for [NewHeap]. capacity is a
// size hint rather than a bound, but zero is refused with
// [ErrPriorityQueueCapacity].
func NewPriorityQueue[K comparable, T any](capacity uint, items ...Item[K, T]) (*PriorityQueue[K, T], error) {
	if capacity == 0 {
		return nil, ErrPriorityQueueCapacity
	}
	return &PriorityQueue[K, T]{
		capacity: capacity,
		h:        NewHeapCap(int(capacity), items...),
	}, nil
}

// Cap reports the current capacity of the backing store: how many items the queue
// can hold before its next reallocation. It starts at the constructor hint and
// grows as the queue grows past it.
func (p *PriorityQueue[K, T]) Cap() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.h.Cap()
}

// Len reports the number of queued items.
func (p *PriorityQueue[K, T]) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.h.Len()
}

// Empty reports whether the queue holds no items.
func (p *PriorityQueue[K, T]) Empty() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.h.Len() == 0
}

// Peek returns a copy of the highest-Priority item without removing it, or nil
// if the queue is empty. It copies rather than aliasing [Heap.Peek] because
// another goroutine may Enqueue or Dequeue as soon as the read lock is
// released.
func (p *PriorityQueue[K, T]) Peek() *Item[K, T] {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy: h.Peek() points into the backing array, which a later
	// Enqueue or Dequeue rewrites once the read lock is released.
	top := p.h.Peek()
	if top == nil {
		return nil
	}
	item := *top
	return &item
}

// Less reports whether the item in heap slot i has a lower Priority than the
// item in slot j. Slots index the heap's internal layout, not dequeue order,
// and both must be below Len.
func (p *PriorityQueue[K, T]) Less(i, j int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.h.items[i].Priority < p.h.items[j].Priority
}

// Enqueue adds i to the queue. If an item with the same Key is already queued,
// the one with the higher Priority stays (see Heap.Push).
func (p *PriorityQueue[K, T]) Enqueue(i Item[K, T]) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.h.Push(i)
}

// Dequeue removes and returns the highest-Priority item, or
// ErrEmptyPriorityQueue if the queue is empty.
func (p *PriorityQueue[K, T]) Dequeue() (*Item[K, T], error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.h.Len() == 0 {
		return nil, ErrEmptyPriorityQueue
	}

	item := p.h.Pop()

	return item, nil
}
