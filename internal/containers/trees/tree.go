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

// Point is a position in a multi-dimensional space, with coordinates of
// precision P, that spatial trees can index and query.
type Point[K comparable, P float32 | float64] interface {
	// Dim reports the number of dimensions of the point.
	Dim() int

	// GetValue returns the coordinate of the point along the given dimension.
	GetValue(dim int) P

	// Distance returns the distance between this point and p.
	Distance(p Point[K, P]) P

	Key() K
}

// TreeNode is a single element stored in a tree: a node exposing its key, its
// payload and, for spatial trees, its Point coordinates.
//
//	K is the comparable lookup key.
//	T is the stored payload.
//	P is the floating-point type used for spatial coordinates.
type TreeNode[K comparable, T any, P float32 | float64] interface {
	// Key returns the comparable key that identifies the node.
	Key() K

	// Value returns the payload carried by the node.
	Value() T

	// Point returns the spatial coordinates of the node, or nil if it has
	// none, which a spatial tree rejects with ErrMissingPoint.
	Point() Point[K, P]
}

// Tree is the contract common to spatial trees; their lookups live on the
// SpatialTree extension below.
type Tree[K comparable, T any, P float32 | float64] interface {
	// Len reports the number of nodes currently stored in the tree.
	Len() int

	// Insert adds node to the tree, returning an error if it cannot be stored.
	Insert(node TreeNode[K, T, P]) error
}

// SpatialTree is a Point-addressable Tree: nodes are located by proximity in
// coordinate space rather than by an exact key.
type SpatialTree[K comparable, T any, P float32 | float64] interface {
	Tree[K, T, P]

	// Nearest returns up to k nodes closest to p, ordered from nearest to
	// farthest.
	Nearest(p Point[K, P], k int) []TreeNode[K, T, P]

	// Range returns every node whose Point falls within the axis-aligned box
	// bounded by the min and max corners (inclusive).
	Range(min, max Point[K, P]) []TreeNode[K, T, P]
}
