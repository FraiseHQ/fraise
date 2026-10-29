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

// Point is a position in a multi-dimensional space that spatial trees can
// index and query. Its coordinates have precision P.
type Point[K comparable, P float32 | float64] interface {
	// Dim reports the number of dimensions of the point.
	Dim() int

	// Coordinates returns the point's coordinates, one per dimension. A tree
	// projects a point onto a split direction at every level it passes, a dot
	// product over every coordinate, so the point hands over its slice whole
	// rather than answering one call per coordinate. Callers only read it.
	Coordinates() []P

	// Distance returns the distance between this point and p. Points of
	// different dimensions have no distance, and Distance returns
	// containers.ErrDimMismatch.
	Distance(p Point[K, P]) (P, error)

	Key() K
}

// TreeNode is one element stored in a tree. It exposes its key, its payload
// and, for spatial trees, its Point.
//
//	K is the comparable lookup key.
//	T is the stored payload.
//	P is the floating-point type used for spatial coordinates.
type TreeNode[K comparable, T any, P float32 | float64] interface {
	// Key returns the comparable key that identifies the node.
	Key() K

	// Value returns the payload carried by the node.
	Value() T

	// Point returns the node's spatial coordinates, or nil if it has none. A
	// spatial tree rejects a node without a Point with ErrMissingPoint.
	Point() Point[K, P]
}

// Tree is the contract common to spatial trees. SpatialTree adds their
// lookups.
type Tree[K comparable, T any, P float32 | float64] interface {
	// Len reports the number of nodes stored in the tree.
	Len() int

	// Insert adds node to the tree, returning an error if it cannot be stored.
	Insert(node TreeNode[K, T, P]) error
}

// SpatialTree is a Tree searched by Point. It finds nodes by proximity in
// coordinate space, not by exact key.
type SpatialTree[K comparable, T any, P float32 | float64] interface {
	Tree[K, T, P]

	// Nearest returns up to k nodes closest to p, nearest first. A non-nil
	// keep limits the search to nodes whose key it accepts, before the k
	// nearest are chosen. A caller that filtered the result afterwards would
	// lose every accepted node ranked below k among all nodes. A nil keep
	// accepts every node. A p whose dimension differs from the tree's returns
	// ErrDimensionMismatch, as Insert does for such a point.
	Nearest(p Point[K, P], k int, keep func(K) bool) ([]TreeNode[K, T, P], error)

	// Range returns every node whose Point falls within the axis-aligned box
	// bounded by the min and max corners (inclusive).
	Range(min, max Point[K, P]) []TreeNode[K, T, P]
}
