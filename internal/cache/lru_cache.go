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

package cache

import (
	"container/list"
	"sync"
)

type LRUCache[K comparable, T any] struct {
	capacity int
	items    map[K]*list.Element
	order    *list.List
	// PERF: using a RWMutex to distinguish from read/ write actions
	// but a sync.Mutex could have done the job here since the only
	// read method is Len (Get modifies the state), depending on how often len
	// is called, a sync.Mutex will be more efficient
	mu sync.RWMutex
}

// NewLRUCache returns a new LRU cache with the given capacity.
// Capacity must be > 0; the constructor panics otherwise.
func NewLRUCache[K comparable, T any](capacity int) *LRUCache[K, T] {
	if capacity <= 0 {
		panic("cache: capacity must be > 0")
	}
	return &LRUCache[K, T]{
		capacity: capacity,
		items:    make(map[K]*list.Element, capacity),
		order:    list.New(),
	}
}

// Get returns the value for key and true if present, or zero value
// and false otherwise. Get marks the entry as most recently used.
func (c *LRUCache[K, T]) Get(key K) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero T
	elem, ok := c.items[key]
	if !ok {
		return zero, false
	}

	c.order.MoveToFront(elem)
	return elem.Value.(*Entry[K, T]).Value, true
}

// Set inserts a new value in the
func (c *LRUCache[K, T]) Put(key K, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// update path (key already exists)
	if elem, ok := c.items[key]; ok {
		elem.Value.(*Entry[K, T]).Value = value
		c.order.MoveToFront(elem)
		return
	}

	// insert path (new key)
	e := &Entry[K, T]{Key: key, Value: value}
	elem := c.order.PushFront(e)
	c.items[key] = elem

	// Evict if over capacity
	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*Entry[K, T]).Key)
		}
	}
}

// Delete removes key from the cache. Returns true if the key was
// present, false if it wasn't.
func (c *LRUCache[K, T]) Delete(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return false
	}

	c.order.Remove(elem)
	delete(c.items, key)
	return true
}

// Len returns the current number of entries in the cache.
func (c *LRUCache[K, T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}

// Capacity returns the maximum number of entries the cache will hold.
func (c *LRUCache[K, T]) Capacity() int {
	return c.capacity
}

// Clear removes all entries from the cache.
func (c *LRUCache[K, T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[K]*list.Element, c.capacity)
	c.order = list.New()
}

// Resizes LRU cache Returns the number of entries evicted
func (c *LRUCache[K, T]) Resize(capacity int) int {

	if capacity <= 0 {
		panic("cache: capacity must be > 0")
	}
	entries := 0

	c.mu.Lock()
	defer c.mu.Unlock()

	switch {
	// allocate new map (increase size)
	// NOTE: this step could have been skipped
	// as this is a pre-allocation hint for go
	// but it also avoid resizing the map during
	// subsequent puts
	case c.capacity < capacity:
		n := make(map[K]*list.Element, capacity)
		for k, v := range c.items {
			n[k] = v
		}
		c.items = n
	// evict entries (reduce size)
	case c.capacity > capacity:
		for c.order.Len() > c.capacity {
			oldest := c.order.Back()
			if oldest == nil {
				break
			}
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*Entry[K, T]).Key)
			entries++
		}
	}

	c.capacity = capacity
	return entries
}
