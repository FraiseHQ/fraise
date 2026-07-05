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

package trees

import "github.com/RonsenbergVI/fraise/internal/hash"

// Projection is the random linear map an RPTree applies to points before it
// hands them to its underlying KDTree. Each row is a random direction in the
// input space; a point is projected by taking its dot product with every row.
// Projecting the data through random directions makes the KDTree's axis-aligned
// splits behave like the arbitrary-direction splits of a classic
// random-projection tree.
type Projection[P float32 | float64] struct {
	rows [][]P // random directions; one per projected (output) coordinate
}

// Apply projects p through the random directions, returning its coordinates in
// the projected space (dimension len(rows)).
func (pr Projection[P]) Apply(p Point[P]) Point[P] {
	panic("not implemented")
}

// projectedNode adapts an original TreeNode so the coordinates the inner KDTree
// splits and searches on are the projected ones, while the key, payload and the
// original (unprojected) point stay reachable for result re-ranking.
type projectedNode[K comparable, T any, P float32 | float64] struct {
	orig      TreeNode[K, T, P] // original node: Key, Value, Hash, true Point
	projected Point[P]          // coordinates in the projected space
}

func (n projectedNode[K, T, P]) Key() K                          { return n.orig.Key() }
func (n projectedNode[K, T, P]) Value() T                        { return n.orig.Value() }
func (n projectedNode[K, T, P]) Point() Point[P]                 { return n.projected }
func (n projectedNode[K, T, P]) Hash(h hash.Hasher[K, string]) K { return n.orig.Hash(h) }

// RPTree is a random-projection tree built on top of a KDTree. Points are first
// mapped through a random Projection, then indexed by an axis-aligned KDTree in
// the projected space. The random rotation lets the KDTree's cheap axis-aligned
// splits approximate arbitrary-direction splits, which keeps nearest-neighbour
// search effective in high dimensions where a plain KDTree degrades. It
// implements SpatialTree; its Nearest results are approximate.
//
// A single RPTree is a weak approximator; recall comes from querying a forest of
// them with independent projections. That forest is assembled at the index
// layer, not here.
type RPTree[K comparable, T any, P float32 | float64] struct {
	proj    Projection[P]    // random projection applied before indexing
	kd      *KDTree[K, T, P] // KDTree over the projected coordinates
	dim     int              // dimensionality of input points
	projDim int              // dimensionality after projection
	seed    uint64           // seed for reproducible random projections
}

// NewRPTree returns an empty RPTree that indexes points of the given input
// dimension, projecting them onto projDim random directions before handing them
// to an inner KDTree. seed makes the projection reproducible.
func NewRPTree[K comparable, T any, P float32 | float64](dim, projDim int, seed uint64) *RPTree[K, T, P] {
	return &RPTree[K, T, P]{
		proj:    Projection[P]{}, // TODO: fill with projDim random directions seeded by seed
		kd:      NewKDTree[K, T, P](projDim),
		dim:     dim,
		projDim: projDim,
		seed:    seed,
	}
}

// Len reports the number of stored nodes, delegating to the inner KDTree.
func (t *RPTree[K, T, P]) Len() int { return t.kd.Len() }

// Insert projects node's point and inserts a projectedNode into the inner
// KDTree, keeping the original node for result re-ranking.
func (t *RPTree[K, T, P]) Insert(node TreeNode[K, T, P]) error {
	panic("not implemented")
}

// Iterator walks the underlying KDTree.
func (t *RPTree[K, T, P]) Iterator() TreeIterator[K, T, P] { return t.kd.Iterator() }

// Nearest projects p, asks the inner KDTree for candidates in the projected
// space, then re-ranks them by true distance in the original space and returns
// the k closest original nodes.
func (t *RPTree[K, T, P]) Nearest(p Point[P], k int) []TreeNode[K, T, P] {
	panic("not implemented")
}

// Range returns the points within the axis-aligned box bounded by min and max.
// NOTE: a random projection does not map an axis-aligned box to an axis-aligned
// box, so this cannot simply delegate to the inner KDTree's Range; the box must
// be answered in the original space. See the interface discussion on whether
// Range belongs on SpatialTree at all.
func (t *RPTree[K, T, P]) Range(min, max Point[P]) []TreeNode[K, T, P] {
	panic("not implemented")
}
