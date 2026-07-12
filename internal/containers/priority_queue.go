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
// highest-Priority item, and Enqueue never drops anything — the queue grows to
// hold whatever is pushed. capacity is the initial size hint used to pre-allocate
// the backing store; it is not a bound, and the queue reallocates past it as
// needed (observe the live size via Cap).
type PriorityQueue[K comparable, T any] struct {
	h        *Heap[K, T]
	capacity uint
	mu       sync.RWMutex
}

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

func (p *PriorityQueue[K, T]) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.h.Len()
}

func (p *PriorityQueue[K, T]) Empty() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.h.Len() == 0
}

func (p *PriorityQueue[K, T]) Peek() *Item[K, T] {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy: h.Peek() aliases the backing array, which a later Enqueue
	// may reallocate out from under the caller once the read lock is released.
	top := p.h.Peek()
	if top == nil {
		return nil
	}
	item := *top
	return &item
}

func (p *PriorityQueue[K, T]) Less(i, j int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.h.items[i].Priority < p.h.items[j].Priority
}

// enqueues an item
func (p *PriorityQueue[K, T]) Enqueue(i Item[K, T]) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.h.Push(i)
}

// Returns the highest priority item from the queue
func (p *PriorityQueue[K, T]) Dequeue() (*Item[K, T], error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.h.Len() == 0 {
		return nil, ErrEmptyPriorityQueue
	}

	item := p.h.Pop()

	return item, nil
}
