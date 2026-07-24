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

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/hash"
)

// defaultRPLeafSize is the number of points a leaf accumulates before it is
// split into two children.
const defaultRPLeafSize = 32

// ErrDimensionMismatch is returned when a point's dimensionality does not
// match the tree it is being inserted into.
var ErrDimensionMismatch = errors.New("trees: point dimension does not match tree dimension")

// ErrMissingPoint is returned when a node with no spatial coordinates (Point
// returns nil) is inserted into a spatial tree.
var ErrMissingPoint = errors.New("trees: node has no point")

// Projection holds the random basis for one RPTree. Each row is a unit vector
// in the input space; internal tree nodes pick one row as their split direction.
type Projection[K comparable, P float32 | float64] struct {
	rows [][]P // random unit vectors; one per projected coordinate
}

// newProjection draws projDim random unit vectors in dim-dimensional space,
// deterministically from seed.
func newProjection[K comparable, P float32 | float64](dim, projDim int, seed uint64) Projection[K, P] {
	rng := rand.New(rand.NewSource(int64(seed)))
	rows := make([][]P, projDim)
	for i := range rows {
		row := make([]P, dim)
		var norm float64
		for d := range row {
			v := rng.NormFloat64()
			row[d] = P(v)
			norm += v * v
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			norm = 1
		}
		for d := range row {
			row[d] = P(float64(row[d]) / norm)
		}
		rows[i] = row
	}
	return Projection[K, P]{rows: rows}
}

// Apply computes the dot product of p with every row, returning p's coordinates
// in the lower-dimensional projected space.
func (pr Projection[K, P]) Apply(p Point[K, P]) []P {
	out := make([]P, len(pr.rows))
	for i, row := range pr.rows {
		var sum P
		for d, w := range row {
			sum += w * p.GetValue(d)
		}
		out[i] = sum
	}
	return out
}

// rpNode is a node in the RPTree's binary space partition.
// Internal nodes split on a random direction; leaves hold the actual data.
type RPTreeNode[K comparable, T any, P float32 | float64] struct {
	// internal-node fields (unused when leaf)
	splitRow    int // index into Projection.rows
	splitVal    P   // threshold: dot(point, rows[splitRow]) < splitVal → left
	left, right *RPTreeNode[K, T, P]
	// leaf-node fields
	data []TreeNode[K, T, P]
}

// isLeaf reports whether n has no children.
func (n *RPTreeNode[K, T, P]) isLeaf() bool {
	return n.left == nil && n.right == nil
}

// RPTree is a random-projection tree: a binary space partition where each
// internal node splits points by their projection onto one random direction
// drawn from the tree's Projection. It implements SpatialTree; Nearest results
// are approximate.
//
// A single RPTree is a weak approximator; recall improves by querying a forest
// of them with independent projections, assembled at the index layer.
type RPTree[K comparable, T any, P float32 | float64] struct {
	proj     Projection[K, P] // random basis for all splits in this tree
	root     *RPTreeNode[K, T, P]
	dim      int    // dimensionality of input points
	projDim  int    // number of random directions (rows in proj)
	seed     uint64 // seed for reproducible random projections
	length   int    // number of stored nodes
	leafSize int    // max points a leaf holds before splitting
	rng      *rand.Rand
}

// NewRPTree returns an empty RPTree that indexes dim-dimensional points using
// projDim random split directions. seed makes the projection reproducible.
func NewRPTree[K comparable, T any, P float32 | float64](dim, projDim int, seed uint64) *RPTree[K, T, P] {
	if projDim < 1 {
		projDim = 1
	}
	return &RPTree[K, T, P]{
		proj:     newProjection[K, P](dim, projDim, seed),
		root:     &RPTreeNode[K, T, P]{},
		dim:      dim,
		projDim:  projDim,
		seed:     seed,
		leafSize: defaultRPLeafSize,
		rng:      rand.New(rand.NewSource(int64(seed) + 1)),
	}
}

// Len reports the number of stored nodes.
func (t *RPTree[K, T, P]) Len() int { return t.length }

// Insert adds node to the tree, routing it to the correct leaf using the
// random-projection splits.
func (t *RPTree[K, T, P]) Insert(node TreeNode[K, T, P]) error {
	p := node.Point()
	if p == nil {
		return ErrMissingPoint
	}
	if p.Dim() != t.dim {
		return ErrDimensionMismatch
	}

	t.insert(t.root, node)
	t.length++
	return nil
}

func (t *RPTree[K, T, P]) insert(n *RPTreeNode[K, T, P], node TreeNode[K, T, P]) {
	if !n.isLeaf() {
		if t.goesLeft(n, node.Point()) {
			t.insert(n.left, node)
		} else {
			t.insert(n.right, node)
		}
		return
	}

	n.data = append(n.data, node)
	if len(n.data) > t.leafSize {
		t.split(n)
	}
}

// goesLeft reports whether p is routed to n's left child.
func (t *RPTree[K, T, P]) goesLeft(n *RPTreeNode[K, T, P], p Point[K, P]) bool {
	proj := t.proj.Apply(p)
	return proj[n.splitRow] < n.splitVal
}

// split turns the overflowing leaf n into an internal node: it projects every
// point onto a random direction, sorts by that coordinate and divides the
// points evenly between two new leaves. Splitting on the sorted order (rather
// than a fixed threshold) keeps the tree balanced regardless of duplicate
// projected values.
func (t *RPTree[K, T, P]) split(n *RPTreeNode[K, T, P]) {
	row := t.rng.Intn(t.projDim)

	type scored struct {
		node TreeNode[K, T, P]
		val  P
	}
	items := make([]scored, len(n.data))
	for i, node := range n.data {
		items[i] = scored{node: node, val: t.proj.Apply(node.Point())[row]}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].val < items[j].val })

	mid := len(items) / 2
	left := &RPTreeNode[K, T, P]{data: make([]TreeNode[K, T, P], 0, mid)}
	right := &RPTreeNode[K, T, P]{data: make([]TreeNode[K, T, P], 0, len(items)-mid)}
	for i, it := range items {
		if i < mid {
			left.data = append(left.data, it.node)
		} else {
			right.data = append(right.data, it.node)
		}
	}

	n.splitRow = row
	n.splitVal = items[mid].val
	n.left = left
	n.right = right
	n.data = nil
}

// Nearest returns the k nodes whose true distance to p is smallest. The search
// descends the partition using projected coordinates, then re-ranks candidates
// by true distance in the original space.
func (t *RPTree[K, T, P]) Nearest(p Point[K, P], k int) []TreeNode[K, T, P] {
	if k <= 0 || t.length == 0 {
		return nil
	}

	n := t.root
	for !n.isLeaf() {
		if t.goesLeft(n, p) {
			n = n.left
		} else {
			n = n.right
		}
	}
	if len(n.data) == 0 {
		return nil
	}

	pq, _ := containers.NewPriorityQueue[K, TreeNode[K, T, P]](uint(k))
	for _, node := range n.data {
		item := containers.Item[K, TreeNode[K, T, P]]{
			Key:      node.Key(),
			Value:    node,
			Priority: distancePriority(p.Distance(node.Point())),
		}
		if pq.Len() < k {
			pq.Enqueue(item)
			continue
		}
		if top := pq.Peek(); top != nil && item.Priority < top.Priority {
			_, _ = pq.Dequeue()
			pq.Enqueue(item)
		}
	}

	out := make([]TreeNode[K, T, P], pq.Len())
	for i := len(out) - 1; i >= 0; i-- {
		item, _ := pq.Dequeue()
		out[i] = item.Value
	}
	return out
}

// distancePriority maps a non-negative distance to a uint64 that preserves
// ordering, for use as a containers.Item Priority (smaller distance ⇒ smaller
// priority).
func distancePriority[P float32 | float64](d P) uint64 {
	return math.Float64bits(float64(d))
}

// Range returns every node whose point falls within the axis-aligned box bounded
// by min and max, evaluated in the original (unprojected) space.
func (t *RPTree[K, T, P]) Range(min, max Point[K, P]) []TreeNode[K, T, P] {
	var out []TreeNode[K, T, P]
	var walk func(n *RPTreeNode[K, T, P])
	walk = func(n *RPTreeNode[K, T, P]) {
		if n == nil {
			return
		}
		if n.isLeaf() {
			for _, node := range n.data {
				if withinBox(node.Point(), min, max) {
					out = append(out, node)
				}
			}
			return
		}
		walk(n.left)
		walk(n.right)
	}
	walk(t.root)
	return out
}

// withinBox reports whether p falls within the axis-aligned box bounded by
// min and max (inclusive) in every dimension.
func withinBox[K comparable, P float32 | float64](p, min, max Point[K, P]) bool {
	for d := 0; d < p.Dim(); d++ {
		v := p.GetValue(d)
		if v < min.GetValue(d) || v > max.GetValue(d) {
			return false
		}
	}
	return true
}

// Nodes returns every stored node, in no particular order.
func (t *RPTree[K, T, P]) Nodes() []TreeNode[K, T, P] {
	out := make([]TreeNode[K, T, P], 0, t.length)
	var walk func(n *RPTreeNode[K, T, P])
	walk = func(n *RPTreeNode[K, T, P]) {
		if n == nil {
			return
		}
		if n.isLeaf() {
			out = append(out, n.data...)
			return
		}
		walk(n.left)
		walk(n.right)
	}
	walk(t.root)
	return out
}

// VectorPoint adapts a containers.Vector into a Point[K, P], pairing its
// coordinates with a comparable key so it can be indexed by RPTree, KDTree
// and any other SpatialTree.
type VectorPoint[K comparable, P float32 | float64] struct {
	key    K
	vector containers.Vector[P]
}

// NewVectorPoint returns a VectorPoint identified by key, at vector's
// coordinates.
func NewVectorPoint[K comparable, P float32 | float64](key K, vector containers.Vector[P]) VectorPoint[K, P] {
	return VectorPoint[K, P]{key: key, vector: vector}
}

func (v VectorPoint[K, P]) Dim() int         { return v.vector.Dim() }
func (v VectorPoint[K, P]) GetValue(d int) P { return v.vector.Data[d] }
func (v VectorPoint[K, P]) Key() K           { return v.key }

// Distance returns the Euclidean distance between v and p.
func (v VectorPoint[K, P]) Distance(p Point[K, P]) P {
	var sum P
	for d := 0; d < v.Dim(); d++ {
		diff := v.GetValue(d) - p.GetValue(d)
		sum += diff * diff
	}
	return P(math.Sqrt(float64(sum)))
}

// PlaneDistance returns the distance from v to the axis-aligned hyperplane at
// coordinate val along dim.
func (v VectorPoint[K, P]) PlaneDistance(val P, dim int) P {
	diff := v.GetValue(dim) - val
	if diff < 0 {
		return -diff
	}
	return diff
}

// Hash implements hash.Hashable[K, string] by hashing v's coordinates.
func (v VectorPoint[K, P]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash(fmt.Sprint(v.vector.Data))
}

// VectorNode bundles a key and a containers.Vector into a
// TreeNode[K, containers.Vector[P], P], ready to hand to RPTree.Insert (or any
// other SpatialTree). Its Point is a VectorPoint over the same vector.
type VectorNode[K comparable, P float32 | float64] struct {
	key   K
	value containers.Vector[P]
}

// NewVectorNode returns a VectorNode pairing key with value.
func NewVectorNode[K comparable, P float32 | float64](key K, value containers.Vector[P]) *VectorNode[K, P] {
	return &VectorNode[K, P]{key: key, value: value}
}

func (n *VectorNode[K, P]) Key() K                      { return n.key }
func (n *VectorNode[K, P]) Value() containers.Vector[P] { return n.value }
func (n *VectorNode[K, P]) Point() Point[K, P]          { return NewVectorPoint(n.key, n.value) }
func (n *VectorNode[K, P]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash(fmt.Sprint(n.value.Data))
}
