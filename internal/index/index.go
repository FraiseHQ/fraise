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

// SearchIndex maps keys of type K to values of type V and ranks those keys by
// relevance to a query. The query has the same type V the index stores — a
// keyword string for a text index, a vector for a vector index — because in
// both cases you search with an example of what you stored.
type SearchIndex[K comparable, V any, P float32 | float64] interface {
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

	// Search returns the keys most relevant to query, best match first, with a
	// parallel slice of their scores. k bounds the number of results; k <= 0
	// returns all matches. Score direction is index-specific (see the aliases).
	Search(query V, k int) ([]K, []P, error)
}

// TextIndex is a full-text index: it tokenizes string documents on insert and
// answers keyword queries. Its score is the number of query terms a document
// matches, so higher is a better match.
type TextIndex[K comparable, P float32 | float64] = SearchIndex[K, string, P]

// VectorIndex is an (approximate) nearest-neighbour index over dense vectors of
// precision P. Its score is the distance from the query to each vector, so
// lower is nearer.
type VectorIndex[K comparable, P float32 | float64] = SearchIndex[K, containers.Vector[P], P]
