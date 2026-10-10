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

// RPTreeIndex is an approximate nearest-neighbour vector index backed by a
// forest of random-projection trees from the containers/trees package. Each
// tree indexes the same vectors through an independent random projection; a
// query fans out across the forest and pools the candidates. RPTree has no
// delete or update primitive, so the vectors map is the source of truth:
// Delete removes a key from it and Update replaces the key's vector there;
// either leaves the old copy in the forest until the next Flush. Search
// re-ranks every candidate against the map, so stale forest entries are
// filtered out or corrected before results are returned.
//
// Insert is idempotent: re-inserting a key with its current vector is a no-op,
// so re-asserting a fact with the embedding it already carries does not bloat
// the forest. Stale copies left by updates and deletes are bounded
// by an automatic Flush once the forest holds more than flushFactor entries
// per live vector. It implements VectorIndex.
type RPTreeIndex[K comparable, P float32 | float64] struct {
	forest  []trees.SpatialTree[K, containers.Vector[K, P], P]
	vectors map[K]containers.Vector[K, P] // live vectors; source of truth

	dim, projDim, numTrees int
	seed                   uint64

	// flushFactor bounds forest garbage: once a tree holds more than
	// flushFactor entries per live vector, the forest is rebuilt from the
	// live vectors. Comes from config (db.vector-search.flush-factor); the
	// default, 2, keeps rebuild cost amortised O(1) per write while capping
	// memory at twice the live set.
	flushFactor int

	// leafSize and overfetch are the trees' own parameters, held here so
	// newForest hands them to every tree it builds and a compaction rebuild
	// keeps the configured shape. Both come from config
	// (db.vector-search.leaf-size and db.vector-search.overfetch); the trees
	// package has no defaults of its own.
	leafSize  int
	overfetch int

	compare comparator.Comparator[K] // vector key ordering
}

// NewRPTreeIndex returns an empty RPTreeIndex holding numTrees random-projection
// trees, each mapping dim-dimensional vectors onto projDim random directions.
// Tree i is seeded with seed+i, so every tree gets an independent projection.
// A dim of 0 defers forest construction until the first Insert, whose vector
// fixes the index dimensionality. flushFactor is the garbage compaction
// threshold (entries per live vector), leafSize the points a tree's leaf holds
// before splitting, and overfetch the candidates each tree gathers per result
// before it stops probing. A non-positive value for any of these three falls
// back to its default in internal/config, so the trees receive a decided value
// and hold no defaults themselves. compare orders vector keys, the tiebreak
// Search ranks equidistant vectors by.
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

// Insert validates the vector dimension and adds it to every tree in the
// forest. It is idempotent: if key already holds an equal vector, nothing is
// appended, so re-asserting a fact with its current embedding costs no forest
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
	if _, err := idx.Retrieve(key); err != nil {
		return err
	}
	return idx.Insert(key, value)
}

// Delete removes the vector stored under key. The forest copy becomes garbage
// (Search filters it against the live map); the automatic Flush reclaims it
// once garbage exceeds the flushFactor bound.
func (idx *RPTreeIndex[K, P]) Delete(key K) error {
	if _, err := idx.Retrieve(key); err != nil {
		return err
	}
	delete(idx.vectors, key)
	return idx.maybeFlush()
}

// Search fans query out across the forest, pools the candidates, re-ranks them
// by true distance and returns the keys of the k nearest vectors, nearest
// first, together with their distances to the query. Equidistant vectors are
// ordered by key: which of them the trees happen to surface first is an
// artefact of insertion order, and the ranking SearchIndex promises is a total
// order that survives truncation to k.
//
// The re-rank keeps only the k best as it goes: TopK retains the largest
// scores, so each pooled candidate is offered with its distance negated, and
// the pooled union is never sorted. Distances are measured against the live
// vector, not the tree's copy.
func (idx *RPTreeIndex[K, P]) Search(query containers.Vector[K, P], k int) ([]K, []P, error) {
	return idx.search(query, k, nil)
}

// SearchWithin ranks like Search over the vectors admit accepts. The trees
// know nothing of admit, so their candidates are filtered as they are pooled,
// and when fewer than k survive, every tree is asked again for twice as many,
// until k survive or the trees have nothing more to give. Each round pools
// what the earlier ones saw, so the doubling costs at most twice its last
// round. The more of the index admit rejects, the more rounds it takes: a
// subset smaller than k is only known to be complete once the whole forest
// has been walked.
func (idx *RPTreeIndex[K, P]) SearchWithin(query containers.Vector[K, P], k int, admit func(K) bool) ([]K, []P, error) {
	return idx.search(query, k, admit)
}

// search is the fan-out and re-rank behind Search and SearchWithin; a nil
// admit admits every vector, and the forest is asked once.
func (idx *RPTreeIndex[K, P]) search(query containers.Vector[K, P], k int, admit func(K) bool) ([]K, []P, error) {
	if len(idx.vectors) == 0 {
		return nil, nil, ErrEmptyIndex
	}
	if query.Dim() != idx.dim {
		return nil, nil, fmt.Errorf("%w: index expects %d, got %d", ErrInvalidDimension, idx.dim, query.Dim())
	}

	var zeroKey K
	q := trees.NewVectorPoint(zeroKey, query)

	seen := make(map[K]bool)
	nearest := containers.NewTopK[K, P](k, idx.compare)
	var admitted int
	for ask := k; ; ask *= 2 {
		for _, t := range idx.forest {
			for _, node := range t.Nearest(q, ask) {
				key := node.Key()
				if seen[key] {
					continue
				}
				seen[key] = true

				// node may be stale (updated or deleted since it was
				// inserted into this tree); vectors is the source of truth.
				current, err := idx.Retrieve(key)
				if err != nil {
					continue
				}
				if admit != nil && !admit(key) {
					continue
				}
				nearest.Offer(key, -query.Distance(current))
				admitted++
			}
		}
		// Every tree holds the same entries, so one that has been asked for
		// all of them has returned all of them.
		if admit == nil || k <= 0 || admitted >= k || ask >= idx.Entries() {
			break
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
// deleted vectors and any stale copies left behind by Insert/Update. The live
// set is replayed in key order because a tree's splits depend on the order
// points arrive in: replayed in map order, two identical write sequences would
// build different forests, and a fixed seed would no longer reproduce the
// index.
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
