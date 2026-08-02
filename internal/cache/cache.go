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

// Cache is a fixed-capacity key/value store. Implementations decide which
// entry to evict when an insertion would exceed the capacity (for example, the
// least-recently-used entry). Implementations are expected to be safe for
// concurrent use.
type Cache[K comparable, T any] interface {
	// Capacity returns the maximum number of entries the cache will hold.
	Capacity() int

	// Resize changes the cache's capacity to capacity, evicting entries as
	// needed to fit the new bound, and returns the number of entries evicted.
	// It returns ErrCacheCapacity if capacity is not strictly positive.
	Resize(capacity int) (int, error)

	// Put inserts or updates the value stored under key. If adding a new key
	// would exceed the capacity, an existing entry is evicted to make room.
	Put(key K, value T)

	// Get returns the value stored under key and true if present, or the zero
	// value and false otherwise. A successful lookup counts as a use of the
	// entry (relevant to usage-based eviction policies).
	Get(key K) (T, bool)

	// Delete removes key from the cache, returning true if it was present and
	// false if it was not.
	Delete(key K) bool

	// Len returns the current number of entries held in the cache.
	Len() int

	// Clear removes all entries from the cache.
	Clear()
}

// Entry is a single key/value pair held by a Cache.
type Entry[K comparable, T any] struct {
	Key   K
	Value T
}
