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
	"math"
	"math/rand"
	"sort"

	"github.com/FraiseHQ/fraise/internal/containers"
)

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

// Apply computes the dot product of p with every row, returning p's
// coordinates in the projected space. Nearest needs all of them, since a
// descent cannot know which rows the splits below will name; code that needs
// one row uses ApplyRow.
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

// ApplyRow computes p's coordinate along a single projection row. The write
// path needs no more: a split divides on one row, and routing a point past an
// internal node reads only that node's row, so ingest cost does not grow with
// projDim.
func (pr Projection[K, P]) ApplyRow(p Point[K, P], row int) P {
	var sum P
	for d, w := range pr.rows[row] {
		sum += w * p.GetValue(d)
	}
	return sum
}

// RPTreeNode is a node in an RPTree's binary space partition. Internal nodes
// split on a random direction; leaves hold the data.
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
// drawn from the tree's Projection. Nearest results are approximate.
//
// A single RPTree is a weak approximator; recall improves by querying a forest
// of them with independent projections, assembled at the index layer.
type RPTree[K comparable, T any, P float32 | float64] struct {
	proj      Projection[K, P] // random basis for all splits in this tree
	root      *RPTreeNode[K, T, P]
	dim       int // dimensionality of input points
	projDim   int // number of random directions (rows in proj)
	length    int // number of stored nodes
	leafSize  int // max points a leaf holds before splitting
	overfetch int // candidates Nearest gathers per result asked for
	rng       *rand.Rand
}

// NewRPTree returns an empty RPTree that indexes dim-dimensional points using
// projDim random split directions. seed makes the projection and the split
// choices reproducible, leafSize is how many points a leaf holds before it
// splits, and overfetch is how many candidates Nearest gathers per result
// asked for.
//
// The tree has no defaults of its own: they are configuration, applied where
// the index is built. The clamps below are validity floors, the smallest value
// each parameter is meaningful at, not defaults.
func NewRPTree[K comparable, T any, P float32 | float64](dim, projDim int, seed uint64, leafSize, overfetch int) *RPTree[K, T, P] {
	if projDim < 1 {
		projDim = 1
	}
	if leafSize < 1 {
		leafSize = 1
	}
	if overfetch < 1 {
		overfetch = 1
	}
	return &RPTree[K, T, P]{
		proj:      newProjection[K, P](dim, projDim, seed),
		root:      &RPTreeNode[K, T, P]{},
		dim:       dim,
		projDim:   projDim,
		leafSize:  leafSize,
		overfetch: overfetch,
		rng:       rand.New(rand.NewSource(int64(seed) + 1)),
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
	return t.proj.ApplyRow(p, n.splitRow) < n.splitVal
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
		items[i] = scored{node: node, val: t.proj.ApplyRow(node.Point(), row)}
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

// probe is a subtree the descent passed over, kept for a later visit. deviation
// is |proj[splitRow] − splitVal| at the node that turned away from it: how
// close the query came to being routed here instead, and therefore how likely
// this side is to hold a true neighbour that the taken side does not.
type probe[K comparable, T any, P float32 | float64] struct {
	node      *RPTreeNode[K, T, P]
	deviation P
}

// Nearest returns up to k nodes near p, nearest first. The search is
// multi-probe: it descends the partition on projected coordinates, deferring
// every subtree it turns away from, then repeatedly descends the deferred
// subtree whose split the query came closest to, until it holds overfetch × k
// candidates or the tree is exhausted. Candidates are re-ranked by true
// distance in the original space.
//
// Probing lets a result span more than one leaf: a single descent sees one
// leaf, so it would return at most leafSize nodes however large k is, with
// nothing to tell the caller the index holds more. Probing also reaches true
// neighbours on the far side of a split near the query, which a single
// descent never sees.
//
// The overfetch factor turns probing into recall. A pool of only k candidates
// leaves the re-ranking nothing to choose between; a larger pool lets the true
// distance overrule the projection, and probing the whole tree gives the exact
// answer. Measured at the shipped defaults (50k uniform 128-d vectors, k=10,
// 16 trees, recall@10 against brute force):
//
//	factor  1     4     8     16    32    64     brute
//	recall  0.05  0.09  0.12  0.24  0.42  0.53   1.00
//	query   1.4ms 1.7ms 2.1ms 3.1ms 4.6ms 8.4ms  19.6ms
//
// Recall per unit of query time is flat to slightly rising across that range,
// so there is no knee and no value right for every corpus: it is configuration
// (db.vector-search.overfetch), not a constant. Uniform vectors are the
// adversarial case for a projection index; clustered embeddings do better at
// every factor.
//
// Ties between equally deviating probes resolve by the order they were deferred,
// so the walk is a function of the tree and the query alone.
func (t *RPTree[K, T, P]) Nearest(p Point[K, P], k int) []TreeNode[K, T, P] {
	if k <= 0 || t.length == 0 {
		return nil
	}

	// Project the query once: every level of every probe then reads its split
	// coordinate in O(1), where goesLeft would recompute it in O(dim).
	proj := t.proj.Apply(p)

	// descend walks from n to a leaf, deferring the far side of every split it
	// passes, and returns the leaf's contents.
	deferred := make([]probe[K, T, P], 0, t.projDim)
	descend := func(n *RPTreeNode[K, T, P]) []TreeNode[K, T, P] {
		for !n.isLeaf() {
			near, far := n.left, n.right
			if proj[n.splitRow] >= n.splitVal {
				near, far = n.right, n.left
			}
			deviation := proj[n.splitRow] - n.splitVal
			if deviation < 0 {
				deviation = -deviation
			}
			deferred = append(deferred, probe[K, T, P]{node: far, deviation: deviation})
			n = near
		}
		return n.data
	}

	budget := k * t.overfetch
	candidates := descend(t.root)
	// The deferred set stays small — one entry per level per probe — so the
	// closest is found by scan rather than by carrying a second heap.
	for len(candidates) < budget && len(deferred) > 0 {
		best := 0
		for i, d := range deferred[1:] {
			if d.deviation < deferred[best].deviation {
				best = i + 1
			}
		}
		next := deferred[best]
		deferred = append(deferred[:best], deferred[best+1:]...)
		// Probes are subtrees the descent turned away from, so they partition
		// what has not been visited: no leaf is reached twice and the pool needs
		// no deduplication.
		candidates = append(candidates, descend(next.node)...)
	}
	if len(candidates) == 0 {
		return nil
	}

	pq, _ := containers.NewPriorityQueue[K, TreeNode[K, T, P]](uint(k))
	for _, node := range candidates {
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

// VectorPoint adapts a containers.Vector into a Point[K, P], pairing its
// coordinates with a comparable key so RPTree can index and query it.
type VectorPoint[K comparable, P float32 | float64] struct {
	key    K
	vector containers.Vector[K, P]
}

// NewVectorPoint returns a VectorPoint identified by key, at vector's
// coordinates.
func NewVectorPoint[K comparable, P float32 | float64](key K, vector containers.Vector[K, P]) VectorPoint[K, P] {
	return VectorPoint[K, P]{key: key, vector: vector}
}

func (v VectorPoint[K, P]) Dim() int         { return v.vector.Dim() }
func (v VectorPoint[K, P]) GetValue(d int) P { return v.vector.Data[d] }
func (v VectorPoint[K, P]) Key() K           { return v.key }

// Distance returns the Euclidean distance between v and p. Against another
// VectorPoint it uses containers.Vector.Distance; any other Point is read
// coordinate by coordinate through the interface.
func (v VectorPoint[K, P]) Distance(p Point[K, P]) P {
	if o, ok := p.(VectorPoint[K, P]); ok {
		return v.vector.Distance(o.vector)
	}
	var sum P
	for d := 0; d < v.Dim(); d++ {
		diff := v.GetValue(d) - p.GetValue(d)
		sum += diff * diff
	}
	return P(math.Sqrt(float64(sum)))
}

// VectorNode bundles a key and a containers.Vector into a
// TreeNode[K, containers.Vector[K, P], P], ready to hand to RPTree.Insert. Its
// Point is a VectorPoint over the same vector.
type VectorNode[K comparable, P float32 | float64] struct {
	key   K
	value containers.Vector[K, P]
}

// NewVectorNode returns a VectorNode pairing key with value.
func NewVectorNode[K comparable, P float32 | float64](key K, value containers.Vector[K, P]) *VectorNode[K, P] {
	return &VectorNode[K, P]{key: key, value: value}
}

func (n *VectorNode[K, P]) Key() K                         { return n.key }
func (n *VectorNode[K, P]) Value() containers.Vector[K, P] { return n.value }
func (n *VectorNode[K, P]) Point() Point[K, P]             { return NewVectorPoint(n.key, n.value) }
