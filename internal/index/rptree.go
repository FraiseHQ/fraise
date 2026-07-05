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

import (
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/containers/trees"
)

// RPTreeIndex is an approximate nearest-neighbour vector index backed by a
// forest of random-projection trees from the containers/trees submodule. Each
// tree indexes the same vectors through an independent random projection; a
// query fans out across the forest and pools the candidates. It implements
// VectorIndex.
type RPTreeIndex[K comparable, P float32 | float64] struct {
	forest []*trees.RPTree[K, containers.Vector[P], P]
}

// NewRPTreeIndex returns an empty RPTreeIndex holding numTrees random-projection
// trees, each mapping dim-dimensional vectors onto projDim random directions.
// seed seeds the first tree; the rest derive from it so every tree gets an
// independent projection.
func NewRPTreeIndex[K comparable, P float32 | float64](dim, projDim, numTrees int, seed uint64) *RPTreeIndex[K, P] {
	forest := make([]*trees.RPTree[K, containers.Vector[P], P], numTrees)
	for i := range forest {
		forest[i] = trees.NewRPTree[K, containers.Vector[P], P](dim, projDim, seed+uint64(i))
	}
	return &RPTreeIndex[K, P]{forest: forest}
}

// Insert validates the vector dimension and adds it to every tree in the forest.
func (idx *RPTreeIndex[K, P]) Insert(key K, value containers.Vector[P]) error {
	panic("not implemented")
}

// Retrieve returns the vector stored under key.
func (idx *RPTreeIndex[K, P]) Retrieve(key K) (containers.Vector[P], error) {
	panic("not implemented")
}

// Update replaces the vector stored under key.
func (idx *RPTreeIndex[K, P]) Update(key K, value containers.Vector[P]) error {
	panic("not implemented")
}

// Delete removes the vector stored under key.
func (idx *RPTreeIndex[K, P]) Delete(key K) error {
	panic("not implemented")
}

// Search fans query out across the forest, pools the candidates, re-ranks them
// by true distance and returns the keys of the k nearest vectors.
func (idx *RPTreeIndex[K, P]) Search(query containers.Vector[P], k int) ([]K, error) {
	panic("not implemented")
}

// Size reports the approximate in-memory footprint of the index in MiB.
func (idx *RPTreeIndex[K, P]) Size() int {
	panic("not implemented")
}

// Count reports the number of indexed vectors.
func (idx *RPTreeIndex[K, P]) Count() int {
	panic("not implemented")
}

// Flush rebuilds the forest, discarding deleted vectors.
func (idx *RPTreeIndex[K, P]) Flush() error {
	panic("not implemented")
}
