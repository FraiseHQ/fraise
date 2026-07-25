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

package graph

import (
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/RonsenbergVI/fraise/internal/index"
)

// GraphStats is a point-in-time snapshot of a graph's shape.
type GraphStats struct {
	Order int // number of entities (vertices)
	Size  int // number of relationships (edges)
	Nodes int // total stored nodes
}

// Graph is a temporal memory graph: the storage atomic component of the
// database server. A la Redis, every database server holds multiple
// in-memory graphs addressed by index (see the '@n' graph selector in FQL).
//
// K is the node key type; P is the floating-point precision used for
// embedding vectors and ranking scores.
//
// Implementations are guarded by the embedded read-write lock methods;
// callers are responsible for acquiring the appropriate lock around the
// operations they compose (see the locking section below).
type Graph[K comparable, P float32 | float64] interface {
	GetHasher() hash.Hasher[K, string]

	// Get returns the node stored under key, or nil if absent.
	Get(key K) Node[K]

	// Set inserts a new node, deriving its key via the graph's hash
	// function.
	Set(node Node[K]) error

	// Put replaces the node stored under key with the given node.
	Put(key K, node Node[K]) error

	// Delete removes the node (and, by extension, its index entries and
	// incident relationships).
	Delete(node Node[K]) error

	// GetVectorIndex returns the graph's vector (semantic) index, keyed
	// by node key and storing embedding vectors of precision P.
	GetVectorIndex() index.VectorIndex[K, P]

	// GetTextIndex returns the graph's full-text search index.
	GetTextIndex() index.TextIndex[K]

	// MergeFrom merges the contents of g into this graph: nodes,
	// relationships and index entries. Nodes with colliding keys are
	// resolved by the implementation.
	MergeFrom(g Graph[K, P])

	// Copy returns a deep copy of the graph, independent of the
	// original: mutating one never affects the other.
	Copy() Graph[K, P]

	Nodes() map[K]Node[K]

	// AdjacencyMap returns the outgoing-edge view of the graph:
	// AdjacencyMap()[from][to] is the relationship from -> to.
	AdjacencyMap() map[K]map[K]K

	// PredecessorMap returns the incoming-edge view of the graph:
	// PredecessorMap()[to][from] is the relationship from -> to. It is
	// the transpose of AdjacencyMap and serves reverse traversal.
	PredecessorMap() map[K]map[K]K

	// Order returns the number of entities (vertices) in the graph.
	Order() int

	// Size returns the number of relationships (edges) in the graph.
	Size() int

	// Stats returns a point-in-time snapshot of the graph's shape.
	Stats() GraphStats

	// Search runs a hybrid query over the graph and returns matching
	// nodes alongside their ranking scores (parallel slices, ordered
	// best-first, at most top entries).
	//
	// All criteria are optional and combine to narrow the result:
	//   - keywords: full-text terms matched against the text index
	//   - vector:   query embedding for nearest-neighbor search; nil
	//               (or empty) skips the vector index
	//   - topics:   restrict results to facts tagged with these topics
	//   - entities: restrict results to facts involving these entities
	//   - depth:    maximum graph-hop distance explored from direct hits
	//   - top:      maximum number of results returned
	//   - since:    inclusive lower time bound; zero value = unbounded
	//   - until:    exclusive upper time bound; zero value = unbounded
	Search(keywords []string, vector containers.Vector[P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*Node[K], []P)

	// Graphs expose their read-write lock so callers can hold a single
	// lock across a sequence of calls (e.g. Get-then-Put) instead of
	// locking per call. The usual sync.RWMutex contract applies.

	// RLock acquires the lock for reading.
	RLock()

	// Lock acquires the lock for writing.
	Lock()

	// RUnlock releases a read lock.
	RUnlock()

	// Unlock releases a write lock.
	Unlock()
}
