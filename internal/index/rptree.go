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
	"fmt"
	"sort"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/containers/trees"
	"github.com/RonsenbergVI/fraise/pkg/logger"
)

// compile-time check that RPTreeIndex is a VectorIndex.
var _ VectorIndex[int, float64] = (*RPTreeIndex[int, float64])(nil)

// RPTreeIndex is an approximate nearest-neighbour vector index backed by a
// forest of random-projection trees from the containers/trees submodule. Each
// tree indexes the same vectors through an independent random projection; a
// query fans out across the forest and pools the candidates. RPTree has no
// delete/update primitive, so vectors map is the source of truth: Delete
// tombstones it and an Update leaves the old copy in the forest until the next
// Flush; Search always re-ranks candidates against vectors, so stale forest
// entries are filtered out or corrected before results are returned.
//
// Insert is idempotent: re-inserting a key with its current vector is a no-op,
// so callers that replay whole vector sets (e.g. Graph.MergeFrom after a
// staged write) do not bloat the forest. Stale copies left by updates and
// deletes are bounded by an automatic Flush once the forest holds more than
// flushFactor entries per live vector. It implements VectorIndex.
type RPTreeIndex[K comparable, P float32 | float64] struct {
	forest  []*trees.RPTree[K, containers.Vector[K, P], P]
	vectors map[K]containers.Vector[K, P] // live vectors; source of truth

	dim, projDim, numTrees int
	seed                   uint64

	// flushFactor bounds forest garbage: once a tree holds more than
	// flushFactor entries per live vector, the forest is rebuilt from the
	// live vectors. Comes from config (db.vector-search.flush-factor); 2x
	// keeps rebuild cost amortised O(1) per write while capping memory at
	// twice the live set.
	flushFactor int
}

// NewRPTreeIndex returns an empty RPTreeIndex holding numTrees random-projection
// trees, each mapping dim-dimensional vectors onto projDim random directions.
// seed seeds the first tree; the rest derive from it so every tree gets an
// independent projection. A dim of 0 defers forest construction until the
// first Insert, whose vector fixes the index dimensionality. flushFactor is the
// garbage compaction threshold (entries per live vector); a value <= 0 falls
// back to the config default.
func NewRPTreeIndex[K comparable, P float32 | float64](dim, projDim, numTrees int, seed uint64, flushFactor int) *RPTreeIndex[K, P] {
	if flushFactor <= 0 {
		flushFactor = config.DefaultFlushFactor
	}
	idx := &RPTreeIndex[K, P]{
		vectors:     make(map[K]containers.Vector[K, P]),
		dim:         dim,
		projDim:     projDim,
		numTrees:    numTrees,
		seed:        seed,
		flushFactor: flushFactor,
	}
	if dim > 0 {
		idx.forest = idx.newForest()
	}
	return idx
}

// newForest builds numTrees empty RPTrees for the current dimensionality.
func (idx *RPTreeIndex[K, P]) newForest() []*trees.RPTree[K, containers.Vector[K, P], P] {
	forest := make([]*trees.RPTree[K, containers.Vector[K, P], P], idx.numTrees)
	for i := range forest {
		forest[i] = trees.NewRPTree[K, containers.Vector[K, P], P](idx.dim, idx.projDim, idx.seed+uint64(i))
	}
	return forest
}

// Insert validates the vector dimension and adds it to every tree in the
// forest. It is idempotent: if key already holds an equal vector, nothing is
// appended, so replaying a whole vector set (Copy/MergeFrom) costs no forest
// growth. Inserting a different vector under an existing key replaces it in
// the live map; the old forest copy becomes garbage that the next (automatic
// or explicit) Flush discards.
func (idx *RPTreeIndex[K, P]) Insert(key K, value containers.Vector[K, P]) error {
	if value.Dim() == 0 {
		return ErrInvalidDimension
	}
	if idx.dim == 0 {
		idx.dim = value.Dim()
		idx.forest = idx.newForest()
		logger.Info("Vector index dimension established",
			"dimension", idx.dim, "trees", idx.numTrees, "projection", idx.projDim)
	}
	if value.Dim() != idx.dim {
		// The first inserted vector fixes the index dimension; report it so
		// callers know the size every subsequent vector must match.
		logger.Warn("Rejecting vector of mismatched dimension",
			"expected", idx.dim, "got", value.Dim())
		return fmt.Errorf("%w: index expects %d, got %d", ErrInvalidDimension, idx.dim, value.Dim())
	}

	// Idempotence: the key already holds exactly this vector — the forest
	// already indexes it, appending again would only duplicate it.
	if existing, ok := idx.vectors[key]; ok && existing.Equal(value) {
		return nil
	}

	idx.vectors[key] = value
	node := trees.NewVectorNode(key, value)
	for _, t := range idx.forest {
		if err := t.Insert(node); err != nil {
			return err
		}
	}
	return idx.maybeFlush()
}

// maybeFlush rebuilds the forest when it holds more than flushFactor entries
// per live vector — garbage accumulated from updates and deletes. Called after
// every mutation, it keeps forest size O(live vectors) with amortised-constant
// rebuild cost.
func (idx *RPTreeIndex[K, P]) maybeFlush() error {
	if len(idx.forest) == 0 {
		return nil
	}
	if idx.forest[0].Len() <= idx.flushFactor*len(idx.vectors) {
		return nil
	}
	logger.Debug("Vector index compaction",
		"live", len(idx.vectors), "forest", idx.forest[0].Len())
	return idx.Flush()
}

// Vectors returns a copy of the live key -> vector mapping.
func (idx *RPTreeIndex[K, P]) Vectors() map[K]containers.Vector[K, P] {
	out := make(map[K]containers.Vector[K, P], len(idx.vectors))
	for k, v := range idx.vectors {
		out[k] = v
	}
	return out
}

// Retrieve returns the vector stored under key.
func (idx *RPTreeIndex[K, P]) Retrieve(key K) (containers.Vector[K, P], error) {
	v, ok := idx.vectors[key]
	if !ok {
		return containers.Vector[K, P]{}, ErrIndexNotFound
	}
	return v, nil
}

// Update replaces the vector stored under key.
func (idx *RPTreeIndex[K, P]) Update(key K, value containers.Vector[K, P]) error {
	if _, ok := idx.vectors[key]; !ok {
		return ErrIndexNotFound
	}
	return idx.Insert(key, value)
}

// Delete removes the vector stored under key. The forest copy becomes garbage
// (Search filters it against the live map); the automatic Flush reclaims it
// once garbage exceeds the flushFactor bound.
func (idx *RPTreeIndex[K, P]) Delete(key K) error {
	if _, ok := idx.vectors[key]; !ok {
		return ErrIndexNotFound
	}
	delete(idx.vectors, key)
	return idx.maybeFlush()
}

// Search fans query out across the forest, pools the candidates, re-ranks them
// by true distance and returns the keys of the k nearest vectors, nearest
// first, together with their distances to the query.
func (idx *RPTreeIndex[K, P]) Search(query containers.Vector[K, P], k int) ([]K, []P, error) {
	if len(idx.vectors) == 0 {
		return nil, nil, ErrEmptyIndex
	}
	if query.Dim() != idx.dim {
		return nil, nil, ErrInvalidDimension
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
	scores := make([]P, len(candidates))
	for i, c := range candidates {
		out[i] = c.key
		scores[i] = c.d
	}
	logger.Debug("Vector search returned neighbours", "k", k, "found", len(out))
	return out, scores, nil
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

// Entries reports how many entries each tree currently holds — live vectors
// plus garbage awaiting compaction. The automatic Flush keeps it bounded by
// flushFactor * Count().
func (idx *RPTreeIndex[K, P]) Entries() int {
	if len(idx.forest) == 0 {
		return 0
	}
	return idx.forest[0].Len()
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
