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

// KDTreeNode is a single node of a KDTree. Each node splits the space along one
// axis: points on the low side of point[axis] go left, the rest go right.
type KDTreeNode[K comparable, T any, P float32 | float64] struct {
	key   K                    // key identifying the node
	value T                    // stored payload
	point Point[K, P]          // coordinates used for splitting and search
	axis  int                  // dimension this node splits on
	left  *KDTreeNode[K, T, P] // subtree with smaller coordinate on axis
	right *KDTreeNode[K, T, P] // subtree with larger coordinate on axis
}

// KDTree is a k-dimensional binary search tree used for nearest-neighbour and
// range queries over points in a dim-dimensional space. It implements
// SpatialTree.
type KDTree[K comparable, T any, P float32 | float64] struct {
	root   *KDTreeNode[K, T, P]
	dim    int // number of dimensions of indexed points
	length int // number of stored nodes
}

// NewKDTree returns an empty KDTree that indexes points of the given dimension.
func NewKDTree[K comparable, T any, P float32 | float64](dim int) *KDTree[K, T, P] {
	return &KDTree[K, T, P]{dim: dim}
}

func (t *KDTree[K, T, P]) Len() int { return t.length }

func (t *KDTree[K, T, P]) Insert(node TreeNode[K, T, P]) error {
	panic("not implemented")
}

func (t *KDTree[K, T, P]) Iterator() TreeIterator[K, T, P] {
	panic("not implemented")
}

func (t *KDTree[K, T, P]) Nearest(p Point[K, P], k int) []TreeNode[K, T, P] {
	panic("not implemented")
}

func (t *KDTree[K, T, P]) Range(min, max Point[K, P]) []TreeNode[K, T, P] {
	panic("not implemented")
}
