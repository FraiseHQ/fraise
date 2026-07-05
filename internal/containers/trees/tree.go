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

// Point represents a value in a P-dimensional space (with P being float32 or
// float64) that spatial trees can index and query.
type Point[P float32 | float64] interface {
	// Dim reports the number of dimensions of the point.
	Dim() int

	// GetValue returns the coordinate of the point along the given dimension.
	GetValue(dim int) P

	// Distance returns the distance between this point and p.
	Distance(p Point[P]) P

	// PlaneDistance returns the distance from the point to the axis-aligned
	// hyperplane at coordinate val along the given dimension.
	PlaneDistance(val P, dim int) P
}

// TreeNode is a single element stored in a Tree. Every node is Hashable, so it
// can be located by hash-based structures, and additionally exposes its key,
// payload and — for spatial trees — its Point coordinates.
//
//	K is the comparable lookup key.
//	T is the stored payload.
//	P is the floating-point type used for spatial coordinates.
type TreeNode[K comparable, T any, P float32 | float64] interface {
	hash.Hashable[K, string]

	// Key returns the comparable key that identifies the node.
	Key() K

	// Value returns the payload carried by the node.
	Value() T

	// Point returns the spatial coordinates of the node. Non-spatial trees
	// (BTree, HTree) may return nil.
	Point() Point[P]
}

// Tree is the contract shared by every tree container in this package (BTree,
// HTree, KDTree and BKDTree). It only covers operations that are meaningful for
// all of them; keyed and spatial lookups live on the OrderedTree and
// SpatialTree extensions below.
type Tree[K comparable, T any, P float32 | float64] interface {
	// Len reports the number of nodes currently stored in the tree.
	Len() int

	// Insert adds node to the tree, returning an error if it cannot be stored.
	Insert(node TreeNode[K, T, P]) error

	// Iterator returns an iterator that walks the tree in its natural order.
	Iterator() TreeIterator[K, T, P]
}

// OrderedTree is a key-addressable Tree, implemented by BTree and HTree. Nodes
// are located directly by their key.
type OrderedTree[K comparable, T any, P float32 | float64] interface {
	Tree[K, T, P]

	// Get returns the node stored under key, or nil if no such node exists.
	Get(key K) TreeNode[K, T, P]

	// Find locates key in the tree. When exact is true only an exact match is
	// reported; otherwise the closest node is returned. It reports the depth at
	// which the node was found, the node itself and the search path taken.
	Find(key K, exact bool) (int, TreeNode[K, T, P], []int)

	// Delete removes the node stored under key, reporting whether a node was
	// removed.
	Delete(key K) bool
}

// SpatialTree is a Point-addressable Tree, implemented by KDTree and BKDTree.
// Nodes are located by proximity in the P-dimensional coordinate space rather
// than by an exact key.
type SpatialTree[K comparable, T any, P float32 | float64] interface {
	Tree[K, T, P]

	// Nearest returns up to k nodes closest to p, ordered from nearest to
	// farthest.
	Nearest(p Point[P], k int) []TreeNode[K, T, P]

	// Range returns every node whose Point falls within the axis-aligned box
	// bounded by the min and max corners (inclusive).
	Range(min, max Point[P]) []TreeNode[K, T, P]
}

// TreeIterator performs an ordered traversal over the nodes of a Tree.
type TreeIterator[K comparable, T any, P float32 | float64] interface {
	// Tree returns the tree being iterated.
	Tree() Tree[K, T, P]

	// Next advances the iterator to the following node, returning an error if
	// the traversal cannot continue.
	Next() error

	// Valid reports whether the iterator currently points at a node.
	Valid() bool

	// Key returns the key of the node under the cursor.
	Key() K

	// Value returns the raw payload of the node under the cursor.
	Value() []byte

	// Close releases any resources held by the iterator.
	Close()
}
