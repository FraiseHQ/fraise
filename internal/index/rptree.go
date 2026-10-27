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

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/containers/trees"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// RPTreeIndex is an approximate nearest-neighbour index that implements
// VectorIndex. It is backed by a forest of random-projection trees from the
// containers/trees package. Each tree indexes the same vectors through an
// independent random projection. A query fans out across the forest and pools
// the candidates.
//
// RPTree has no delete or update primitive, so the vectors map is the source
// of truth. Delete removes a key from the map, and Update replaces the key's
// vector there. Either one leaves the old copy in the forest until the next
// Flush. Search re-ranks every candidate against the map, so stale copies are
// dropped or corrected before results are returned. An automatic Flush bounds
// the stale copies once the forest holds more than flushFactor entries per
// live vector.
type RPTreeIndex[K comparable, P float32 | float64] struct {
	forest  []trees.SpatialTree[K, containers.Vector[K, P], P]
	vectors map[K]containers.Vector[K, P] // live vectors; source of truth

	dim, projDim, numTrees int
	seed                   uint64

	// flushFactor bounds forest garbage. Once a tree holds more than
	// flushFactor entries per live vector, the forest is rebuilt from the
	// live vectors. It comes from db.vector-search.flush-factor. The default,
	// 2, keeps rebuild cost amortised O(1) per write and caps memory at twice
	// the live set.
	flushFactor int

	// leafSize and overfetch are the trees' own parameters. They are held
	// here so newForest passes them to every tree it builds, and a rebuild
	// keeps the configured shape. They come from db.vector-search.leaf-size
	// and db.vector-search.overfetch. The trees package has no defaults of
	// its own.
	leafSize  int
	overfetch int

	compare comparator.Comparator[K] // vector key ordering
}

// NewRPTreeIndex returns an empty RPTreeIndex holding numTrees
// random-projection trees. Each tree maps dim-dimensional vectors onto
// projDim random directions. Tree i is seeded with seed+i, so every tree gets
// an independent projection. A dim of 0 defers building the forest until the
// first Insert, whose vector fixes the dimension.
//
// flushFactor is the compaction threshold, in entries per live vector.
// leafSize is how many points a leaf holds before it splits. overfetch is how
// many candidates each tree gathers per result before it stops probing. A
// non-positive value for any of these three falls back to its default in
// internal/config, so the trees get a decided value and hold no defaults.
// compare orders vector keys, and Search uses it to rank equidistant vectors.
func NewRPTreeIndex[K comparable, P float32 | float64](dim, projDim, numTrees int, seed uint64, flushFactor, leafSize, overfetch int, compare comparator.Comparator[K]) *RPTreeIndex[K, P] {
	if flushFactor <= 0 {
		flushFactor = config.DefaultFlushFactor
	}
	if leafSize <= 0 {
		leafSize = config.DefaultLeafSize
	}
	if overfetch <= 0 {
		overfetch = config.DefaultOverfetch
	}
	idx := &RPTreeIndex[K, P]{
		vectors:     make(map[K]containers.Vector[K, P]),
		dim:         dim,
		projDim:     projDim,
		numTrees:    numTrees,
		seed:        seed,
		flushFactor: flushFactor,
		leafSize:    leafSize,
		overfetch:   overfetch,
		compare:     compare,
	}
	if dim > 0 {
		idx.forest = idx.newForest()
	}
	return idx
}

// newForest builds numTrees empty RPTrees for the current dimensionality.
func (idx *RPTreeIndex[K, P]) newForest() []trees.SpatialTree[K, containers.Vector[K, P], P] {
	forest := make([]trees.SpatialTree[K, containers.Vector[K, P], P], idx.numTrees)
	for i := range forest {
		forest[i] = trees.NewRPTree[K, containers.Vector[K, P], P](idx.dim, idx.projDim, idx.seed+uint64(i), idx.leafSize, idx.overfetch)
	}
	return forest
}

// Insert checks the vector's dimension and adds it to every tree in the
// forest. It is idempotent: if key already holds an equal vector, nothing is
// added. So re-asserting a fact with its current embedding does not grow the
// forest. A different vector under an existing key replaces it in the live
// map, and the old forest copy stays as garbage until the next Flush.
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
		// Log the expected dimension so callers know the size every vector
		// must match.
		logger.Warn("Rejecting vector of mismatched dimension",
			"expected", idx.dim, "got", value.Dim())
		return fmt.Errorf("%w: index expects %d, got %d", ErrInvalidDimension, idx.dim, value.Dim())
	}

	if existing, err := idx.Retrieve(key); err == nil && existing.Equal(value) {
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

// maybeFlush rebuilds the forest once it holds more than flushFactor entries
// per live vector. It runs after every mutation, which keeps the forest
// O(live vectors) at amortised-constant rebuild cost.
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
	if _, err := idx.Retrieve(key); err != nil {
		return err
	}
	return idx.Insert(key, value)
}

// Delete removes the vector stored under key. Its forest copy stays as
// garbage until the next Flush.
func (idx *RPTreeIndex[K, P]) Delete(key K) error {
	if _, err := idx.Retrieve(key); err != nil {
		return err
	}
	delete(idx.vectors, key)
	return idx.maybeFlush()
}

// Search fans query out across the forest and pools the candidates. It
// re-ranks them by true distance to the live vector, not the tree's copy, and
// returns the k nearest. Which equidistant vectors the trees surface first
// depends on insertion order, so ties are ranked by key.
//
// The re-rank keeps only the k best as it goes, and the pooled union is never
// sorted. TopK keeps the largest scores, so each candidate is offered with
// its distance negated. keep goes down to every tree, and each tree probes
// until it holds enough accepted candidates.
func (idx *RPTreeIndex[K, P]) Search(query containers.Vector[K, P], k int, keep func(K) bool) ([]K, []P, error) {
	if len(idx.vectors) == 0 {
		return nil, nil, ErrEmptyIndex
	}
	if query.Dim() != idx.dim {
		return nil, nil, fmt.Errorf("%w: index expects %d, got %d", ErrInvalidDimension, idx.dim, query.Dim())
	}

	// k <= 0 asks for every match, and every live vector is one. The live
	// vectors are the source of truth the forest is re-ranked against, so
	// reading them all directly gives the same answer as probing every tree
	// for every entry, without building a tree point per entry per tree.
	if k <= 0 {
		every := containers.NewTopK[K, P](0, idx.compare)
		for key, current := range idx.vectors {
			if keep != nil && !keep(key) {
				continue
			}
			d, err := query.Distance(current)
			if err != nil {
				return nil, nil, err
			}
			every.Offer(key, -d)
		}
		out, scores := every.Drain()
		for i, score := range scores {
			scores[i] = -score
		}
		logger.Debug("Vector search returned every match", "found", len(out))
		return out, scores, nil
	}

	var zeroKey K
	q := trees.NewVectorPoint(zeroKey, query)

	seen := make(map[K]bool)
	nearest := containers.NewTopK[K, P](k, idx.compare)
	for _, t := range idx.forest {
		nodes, err := t.Nearest(q, k, keep)
		if err != nil {
			return nil, nil, err
		}
		for _, node := range nodes {
			key := node.Key()
			if seen[key] {
				continue
			}
			seen[key] = true

			// node may be stale: updated or deleted since this tree got it.
			// The vectors map is the source of truth.
			current, err := idx.Retrieve(key)
			if err != nil {
				continue
			}
			d, err := query.Distance(current)
			if err != nil {
				return nil, nil, err
			}
			nearest.Offer(key, -d)
		}
	}

	out, scores := nearest.Drain()
	for i, score := range scores {
		scores[i] = -score
	}
	logger.Debug("Vector search returned neighbours", "k", k, "found", len(out))
	return out, scores, nil
}

// Count reports the number of indexed vectors.
func (idx *RPTreeIndex[K, P]) Count() int {
	return len(idx.vectors)
}

// Entries reports how many entries each tree holds, live or stale. The
// automatic Flush keeps it at most flushFactor * Count().
func (idx *RPTreeIndex[K, P]) Entries() int {
	if len(idx.forest) == 0 {
		return 0
	}
	return idx.forest[0].Len()
}

// Flush rebuilds the forest from the live vectors, which drops deleted
// vectors and stale copies. A tree's splits depend on the order points arrive
// in, so the live set is replayed in key order. In map order, two identical
// write sequences would build different forests, and a fixed seed would no
// longer reproduce the index.
func (idx *RPTreeIndex[K, P]) Flush() error {
	forest := idx.newForest()

	vectors := idx.Vectors()
	keys := make([]K, 0, len(vectors))
	for key := range vectors {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return idx.compare(keys[i], keys[j]) < 0 })

	for _, key := range keys {
		node := trees.NewVectorNode(key, vectors[key])
		for _, t := range forest {
			if err := t.Insert(node); err != nil {
				return err
			}
		}
	}

	idx.forest = forest
	return nil
}
