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

package index

import "github.com/RonsenbergVI/fraise/internal/containers"

// Index is the storage/lifecycle contract shared by every index. It maps keys
// of type K to values of type V; how a value is queried is left to the
// search-specific interfaces below (TextIndex, VectorIndex), because a keyword
// query and a nearest-neighbour query take different arguments.
type Index[K comparable, V any] interface {
	// Insert adds value under key.
	Insert(key K, value V) error

	// Retrieve returns the value stored under key, or ErrIndexNotFound.
	Retrieve(key K) (V, error)

	// Update replaces the value stored under key.
	Update(key K, value V) error

	// Delete removes the entry stored under key.
	Delete(key K) error

	// Size reports the approximate in-memory footprint of the index in MiB.
	Size() int

	// Count reports the number of entries currently held.
	Count() int

	// Flush persists/compacts the index, releasing pending buffers.
	Flush() error
}

// TextIndex is a full-text index: it tokenizes string values on insert and
// answers keyword queries, returning the keys of matching documents.
type TextIndex[K comparable] interface {
	Index[K, string]

	// Search returns the keys of documents matching the query, best matches
	// first.
	Search(query string) ([]K, error)
}

// VectorIndex is an (approximate) nearest-neighbour index over dense vectors.
// P is the coordinate type of the indexed vectors.
type VectorIndex[K comparable, P float32 | float64] interface {
	Index[K, containers.Vector[P]]

	// Search returns the keys of the k vectors closest to query, nearest first.
	Search(query containers.Vector[P], k int) ([]K, error)
}
