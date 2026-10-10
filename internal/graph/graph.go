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

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/internal/index"
)

// GraphStats is a point-in-time snapshot of a graph's shape.
type GraphStats struct {
	Order   int `json:"order"`   // number of vertices: facts, topics, entities
	Size    int `json:"size"`    // number of relationships (edges)
	Nodes   int `json:"nodes"`   // total stored nodes
	Vectors int `json:"vectors"` // total vectors indexed
	// ForestEntries is how many entries the vector forest holds (live vectors
	// plus garbage awaiting compaction); bounded by the index's flush factor
	// times Vectors. 0 for index implementations without a forest.
	ForestEntries int `json:"forest_entries"`
}

// Graph is a temporal memory graph, the unit of storage in the database. As
// with Redis databases, a server holds several graphs addressed by index (the
// @N selector in FQL).
//
// K is the node key type; P is the floating-point precision of embedding
// vectors and ranking scores.
//
// Methods do not lock. Callers hold the graph's read or write lock around the
// calls they compose (see RLock and Lock below).
type Graph[K comparable, P float32 | float64] interface {
	GetHasher() hash.Hasher[K, string]

	// Get returns the node stored under key, or nil if absent.
	Get(key K) Node[K]

	// Set inserts a new node under its own key and fails if the key is
	// already taken.
	Set(node Node[K]) error

	// Put stores node under key, replacing whatever was there.
	Put(key K, node Node[K]) error

	// Delete removes the node, its index entries and its incident
	// relationships.
	Delete(node Node[K]) error

	// GetVectorIndex returns the graph's vector (semantic) index, keyed
	// by node key and storing embedding vectors of precision P.
	GetVectorIndex() index.VectorIndex[K, P]

	// GetTextIndex returns the graph's full-text search index.
	GetTextIndex() index.TextIndex[K, P]

	// Nodes returns the graph's live node map, keyed by node key: the map
	// itself, not a copy. Callers read it under the graph lock and never
	// write it; only the graph's own writes keep it consistent with the edge
	// maps and the indexes.
	Nodes() map[K]Node[K]

	// AdjacencyMap returns the outgoing-edge view of the graph:
	// AdjacencyMap()[from][to] is the relationship from -> to.
	AdjacencyMap() map[K]map[K]K

	// PredecessorMap returns the incoming-edge view of the graph:
	// PredecessorMap()[to][from] is the relationship from -> to. It is
	// the transpose of AdjacencyMap and serves reverse traversal.
	PredecessorMap() map[K]map[K]K

	// Neighbours returns the keys adjacent to key in either direction, in
	// unspecified order. It copies only that node's edges, where
	// AdjacencyMap and PredecessorMap copy every edge in the graph, so a
	// traversal can read one node's neighbourhood cheaply.
	Neighbours(key K) []K

	// Order returns the number of vertices (facts, topics and entities) in
	// the graph.
	Order() int

	// Size returns the number of relationships (edges) in the graph.
	Size() int

	// Stats returns a point-in-time snapshot of the graph's shape.
	Stats() GraphStats

	// Search runs a hybrid query over the graph. It returns the hits best
	// first as three parallel slices of at most top entries (nodes, scores,
	// and the contributions each score was folded from) and the query's
	// background rate, which explain serializes so a client can recompute
	// each hit's relevance. An implementation may return fewer than top hits
	// when its own retrieval policy, such as a score cutoff, drops the tail.
	//
	// The criteria combine to narrow the result:
	//   - keywords: full-text terms matched against the text index
	//   - vector:   query embedding for nearest-neighbour search; nil or
	//               empty skips the vector index
	//   - topics:   keep only facts filed under at least one of these topics
	//   - entities: keep only facts that mention at least one of these
	//               entities
	//   - depth:    the retrieval lane. 0 ranks by seed mass alone and skips
	//               the anchor traversal. 1 and 2 run the same single
	//               anchor-mediated round and differ only in the admission
	//               bar: 1 admits only strongly above-chance anchors
	//               (precision), 2 any anchor above its fair share (recall).
	//               The round runs only when a topic or entity is named.
	//   - top:      maximum number of results
	//   - since:    inclusive lower time bound; zero is unbounded
	//   - until:    exclusive upper time bound; zero is unbounded
	//
	// With no keywords and no vector, the named topics and entities seed the
	// search themselves, for questions like "what do I know about billing?".
	// Every fact filed under any of them is a candidate, with one unit of mass
	// per named anchor it is filed under, and is then scored, decayed, time
	// filtered and capped like any other candidate, so under a single anchor
	// the newest facts come first. No traversal runs, so depth has no effect,
	// and the background is zero. An anchor nothing is filed under seeds
	// nothing.
	//
	// The error reports a question that cannot be answered as asked: a vector
	// whose dimension differs from the vector index's is
	// index.ErrInvalidDimension, rather than a text-only answer that silently
	// ignores it. A graph with nothing indexed is not an error; it returns no
	// hits.
	Search(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]Node[K], []P, [][]scoring.Contribution[K, P], P, error)

	// The graph's read-write lock, exposed so a caller can hold one lock
	// across a sequence of calls (e.g. Get then Put). The usual
	// sync.RWMutex contract applies.

	// RLock acquires the lock for reading.
	RLock()

	// Lock acquires the lock for writing.
	Lock()

	// RUnlock releases a read lock.
	RUnlock()

	// Unlock releases a write lock.
	Unlock()

	// IsEmpty reports whether the graph holds no nodes.
	IsEmpty() bool
}
