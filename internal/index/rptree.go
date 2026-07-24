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
	"sort"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/containers/trees"
)

// compile-time check that RPTreeIndex is a VectorIndex.
var _ VectorIndex[int, float64] = (*RPTreeIndex[int, float64])(nil)

// RPTreeIndex is an approximate nearest-neighbour vector index backed by a
// forest of random-projection trees from the containers/trees submodule. Each
// tree indexes the same vectors through an independent random projection; a
// query fans out across the forest and pools the candidates. RPTree has no
// delete/update primitive, so vectors map is the source of truth: Delete and
// Update only tombstone/replace it, and Insert appends into the live forest
// (a key may briefly have a stale copy sitting in the forest until the next
// Flush); Search always re-ranks candidates against vectors, so stale forest
// entries are filtered out or corrected before results are returned. It
// implements VectorIndex.
type RPTreeIndex[K comparable, P float32 | float64] struct {
	forest  []*trees.RPTree[K, containers.Vector[P], P]
	vectors map[K]containers.Vector[P] // live vectors; source of truth

	dim, projDim, numTrees int
	seed                   uint64
}

// NewRPTreeIndex returns an empty RPTreeIndex holding numTrees random-projection
// trees, each mapping dim-dimensional vectors onto projDim random directions.
// seed seeds the first tree; the rest derive from it so every tree gets an
// independent projection. A dim of 0 defers forest construction until the
// first Insert, whose vector fixes the index dimensionality.
func NewRPTreeIndex[K comparable, P float32 | float64](dim, projDim, numTrees int, seed uint64) *RPTreeIndex[K, P] {
	idx := &RPTreeIndex[K, P]{
		vectors:  make(map[K]containers.Vector[P]),
		dim:      dim,
		projDim:  projDim,
		numTrees: numTrees,
		seed:     seed,
	}
	if dim > 0 {
		idx.forest = idx.newForest()
	}
	return idx
}

// newForest builds numTrees empty RPTrees for the current dimensionality.
func (idx *RPTreeIndex[K, P]) newForest() []*trees.RPTree[K, containers.Vector[P], P] {
	forest := make([]*trees.RPTree[K, containers.Vector[P], P], idx.numTrees)
	for i := range forest {
		forest[i] = trees.NewRPTree[K, containers.Vector[P], P](idx.dim, idx.projDim, idx.seed+uint64(i))
	}
	return forest
}

// Insert validates the vector dimension and adds it to every tree in the forest.
func (idx *RPTreeIndex[K, P]) Insert(key K, value containers.Vector[P]) error {
	if value.Dim() == 0 {
		return ErrInvalidDimension
	}
	if idx.dim == 0 {
		idx.dim = value.Dim()
		idx.forest = idx.newForest()
	}
	if value.Dim() != idx.dim {
		return ErrInvalidDimension
	}

	idx.vectors[key] = value
	node := trees.NewVectorNode(key, value)
	for _, t := range idx.forest {
		if err := t.Insert(node); err != nil {
			return err
		}
	}
	return nil
}

// Vectors returns a copy of the live key -> vector mapping.
func (idx *RPTreeIndex[K, P]) Vectors() map[K]containers.Vector[P] {
	out := make(map[K]containers.Vector[P], len(idx.vectors))
	for k, v := range idx.vectors {
		out[k] = v
	}
	return out
}

// Retrieve returns the vector stored under key.
func (idx *RPTreeIndex[K, P]) Retrieve(key K) (containers.Vector[P], error) {
	v, ok := idx.vectors[key]
	if !ok {
		return containers.Vector[P]{}, ErrIndexNotFound
	}
	return v, nil
}

// Update replaces the vector stored under key.
func (idx *RPTreeIndex[K, P]) Update(key K, value containers.Vector[P]) error {
	if _, ok := idx.vectors[key]; !ok {
		return ErrIndexNotFound
	}
	return idx.Insert(key, value)
}

// Delete removes the vector stored under key.
func (idx *RPTreeIndex[K, P]) Delete(key K) error {
	if _, ok := idx.vectors[key]; !ok {
		return ErrIndexNotFound
	}
	delete(idx.vectors, key)
	return nil
}

// Search fans query out across the forest, pools the candidates, re-ranks them
// by true distance and returns the keys of the k nearest vectors.
func (idx *RPTreeIndex[K, P]) Search(query containers.Vector[P], k int) ([]K, error) {
	if len(idx.vectors) == 0 {
		return nil, ErrEmptyIndex
	}
	if query.Dim() != idx.dim {
		return nil, ErrInvalidDimension
	}

	var zeroKey K
	q := trees.NewVectorPoint(zeroKey, query)

	type scored struct {
		key K
		d   P
	}
	seen := make(map[K]bool)
	var candidates []scored
	for _, t := range idx.forest {
		for _, node := range t.Nearest(q, k) {
			key := node.Key()
			if seen[key] {
				continue
			}
			seen[key] = true

			// node may be stale (updated or deleted since it was inserted
			// into this tree); vectors is the source of truth.
			current, ok := idx.vectors[key]
			if !ok {
				continue
			}
			candidates = append(candidates, scored{key: key, d: q.Distance(trees.NewVectorPoint(key, current))})
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].d < candidates[j].d })
	if len(candidates) > k {
		candidates = candidates[:k]
	}

	out := make([]K, len(candidates))
	for i, c := range candidates {
		out[i] = c.key
	}
	return out, nil
}

// Size reports the approximate in-memory footprint of the index in MiB.
func (idx *RPTreeIndex[K, P]) Size() int {
	coord := coordSize[P]()

	var bytes int
	for range idx.vectors {
		bytes += idx.dim * coord
	}
	for _, t := range idx.forest {
		bytes += t.Len() * (idx.dim*coord + 32) // rough per-entry node overhead
	}
	return bytes / (1024 * 1024)
}

// coordSize returns the size in bytes of a single P coordinate.
func coordSize[P float32 | float64]() int {
	var zero P
	if _, ok := any(zero).(float32); ok {
		return 4
	}
	return 8
}

// Count reports the number of indexed vectors.
func (idx *RPTreeIndex[K, P]) Count() int {
	return len(idx.vectors)
}

// Flush rebuilds the forest from the currently live vectors, discarding
// deleted vectors and any stale copies left behind by Insert/Update.
func (idx *RPTreeIndex[K, P]) Flush() error {
	forest := idx.newForest()

	for key, value := range idx.vectors {
		node := trees.NewVectorNode(key, value)
		for _, t := range forest {
			if err := t.Insert(node); err != nil {
				return err
			}
		}
	}

	idx.forest = forest
	return nil
}
