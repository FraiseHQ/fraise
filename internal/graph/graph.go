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
	"github.com/RonsenbergVI/fraise/internal/index"
)

type GraphStats struct {
	Order int
	Size  int
	Nodes int
}

// Hashing fuinction that takes as input a node and returns a hash.
type Hash[K comparable, V string | ~float32 | ~int | ~bool] func(*Node[K, V]) K

// A knowledge graph is the storage atomic component of the database server.
// A la Redis, every single database server has multiple in memory graphs (default 13)
type KnowledgeGraph[K comparable, V string | ~float32 | ~int | ~bool, P float32 | float64] interface {
	// Generic Graph methods
	// Retrieves Node
	Get(key K) *Node[K, V]

	// Sets Node
	Set(node *Node[K, V]) error

	// Updates node
	Put(key K, node *Node[K, V], options ...func(*Properties[V])) error

	// Delete node
	Delete(node *Node[K, V]) error

	// Returns the graph vector index
	GetVectorIndex() index.Index[K, float32, P]

	// Returns the graph full text search index
	GetTextIndex() index.Index[K, string, P]

	// Checks if graph is locked
	Locked() bool

	// Entities
	Entities() []*Entity[K, V]

	// Relationships
	Relationships() []*Relationship[K, V]

	// Adjacency map
	AdjacencyMap() map[K]map[K]*Relationship[K, V]

	// Predecessor map
	PredecessorMap() map[K]map[K]*Relationship[K, V]

	// Number of Entities in the graph
	Order() (int error)

	// Number of Relationships in the graph
	Size() (int error)

	// Returns statistics about the graph
	Stats() GraphStats
}
