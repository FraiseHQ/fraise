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

// Projection holds the random basis for one RPTree. Each row is a unit vector
// in the input space; internal tree nodes pick one row as their split direction.
type Projection[K comparable, P float32 | float64] struct {
	rows [][]P // random unit vectors; one per projected coordinate
}

// Apply computes the dot product of p with every row, returning p's coordinates
// in the lower-dimensional projected space.
func (pr Projection[K, P]) Apply(p Point[K, P]) []P {
	panic("not implemented")
}

// rpNode is a node in the RPTree's binary space partition.
// Internal nodes split on a random direction; leaves hold the actual data.
type RPTreeNode[K comparable, T any, P float32 | float64] struct {
	// internal-node fields (unused when leaf is true)
	splitRow    int // index into Projection.rows
	splitVal    P   // threshold: dot(point, rows[splitRow]) < splitVal → left
	left, right *RPTreeNode[K, T, P]
	// leaf-node fields
	leaf bool
	data []TreeNode[K, T, P]
}

// RPTree is a random-projection tree: a binary space partition where each
// internal node splits points by their projection onto one random direction
// drawn from the tree's Projection. It implements SpatialTree; Nearest results
// are approximate.
//
// A single RPTree is a weak approximator; recall improves by querying a forest
// of them with independent projections, assembled at the index layer.
type RPTree[K comparable, T any, P float32 | float64] struct {
	proj    Projection[K, P] // random basis for all splits in this tree
	root    *RPTreeNode[K, T, P]
	dim     int    // dimensionality of input points
	projDim int    // number of random directions (rows in proj)
	seed    uint64 // seed for reproducible random projections
	length  int    // number of stored nodes
}

// NewRPTree returns an empty RPTree that indexes dim-dimensional points using
// projDim random split directions. seed makes the projection reproducible.
func NewRPTree[K comparable, T any, P float32 | float64](dim, projDim int, seed uint64) *RPTree[K, T, P] {
	return &RPTree[K, T, P]{
		proj:    Projection[K, P]{}, // TODO: fill with projDim random unit vectors seeded by seed
		dim:     dim,
		projDim: projDim,
		seed:    seed,
	}
}

// Len reports the number of stored nodes.
func (t *RPTree[K, T, P]) Len() int { return t.length }

// Insert adds node to the tree, routing it to the correct leaf using the
// random-projection splits.
func (t *RPTree[K, T, P]) Insert(node TreeNode[K, T, P]) error {
	panic("not implemented")
}

// Iterator walks the tree nodes in insertion order.
func (t *RPTree[K, T, P]) Iterator() TreeIterator[K, T, P] {
	panic("not implemented")
}

// Nearest returns the k nodes whose true distance to p is smallest. The search
// descends the partition using projected coordinates, then re-ranks candidates
// by true distance in the original space.
func (t *RPTree[K, T, P]) Nearest(p Point[K, P], k int) []TreeNode[K, T, P] {
	panic("not implemented")
}

// Range returns every node whose point falls within the axis-aligned box bounded
// by min and max, evaluated in the original (unprojected) space.
func (t *RPTree[K, T, P]) Range(min, max Point[K, P]) []TreeNode[K, T, P] {
	panic("not implemented")
}
