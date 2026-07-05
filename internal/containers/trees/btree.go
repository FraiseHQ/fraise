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

// BTreeNode is a single node of a BTree. Keys within a node are kept sorted and
// each internal node has one more child than it has keys.
type BTreeNode[K comparable, T any, P float32 | float64] struct {
	keys     []K                   // sorted keys held by this node
	values   []T                   // payloads, parallel to keys
	children []*BTreeNode[K, T, P] // child pointers (len(keys)+1 when internal)
	leaf     bool                  // whether the node has no children
}

// BTree is a self-balancing, ordered search tree in which every node holds
// between degree-1 and 2*degree-1 keys. It implements OrderedTree.
type BTree[K comparable, T any, P float32 | float64] struct {
	root   *BTreeNode[K, T, P]
	degree int               // minimum degree (branching factor)
	length int               // number of stored nodes
	less   func(a, b K) bool // key ordering
}

// NewBTree returns an empty BTree with the given minimum degree and key ordering.
func NewBTree[K comparable, T any, P float32 | float64](degree int, less func(a, b K) bool) *BTree[K, T, P] {
	return &BTree[K, T, P]{degree: degree, less: less}
}

func (t *BTree[K, T, P]) Len() int { return t.length }

func (t *BTree[K, T, P]) Insert(node TreeNode[K, T, P]) error {
	panic("not implemented")
}

func (t *BTree[K, T, P]) Iterator() TreeIterator[K, T, P] {
	panic("not implemented")
}

func (t *BTree[K, T, P]) Get(key K) TreeNode[K, T, P] {
	panic("not implemented")
}

func (t *BTree[K, T, P]) Find(key K, exact bool) (int, TreeNode[K, T, P], []int) {
	panic("not implemented")
}

func (t *BTree[K, T, P]) Delete(key K) bool {
	panic("not implemented")
}
