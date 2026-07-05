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

// HTreeNode is a single entry of an HTree. Entries that hash to the same key are
// chained together through next, forming a collision list.
type HTreeNode[K comparable, T any, P float32 | float64] struct {
	key   K                   // hash key of the entry
	value T                   // stored payload
	next  *HTreeNode[K, T, P] // next entry in the collision chain
}

// HTree is a hash-tree: values are located by the key produced when hashing
// them with the configured Hasher. It implements OrderedTree.
type HTree[K comparable, T any, P float32 | float64] struct {
	root      *HTreeNode[K, T, P] // head of the entry list
	hasher    hash.Hasher[K, T]   // hashing algorithm used to derive keys
	length    int                 // number of nodes
	conflicts int                 // number of hash collisions observed
}

// NewHTree returns an empty HTree that derives keys with the given hasher.
func NewHTree[K comparable, T any, P float32 | float64](hasher hash.Hasher[K, T]) *HTree[K, T, P] {
	return &HTree[K, T, P]{hasher: hasher}
}

func (t *HTree[K, T, P]) Len() int { return t.length }

func (t *HTree[K, T, P]) Insert(node TreeNode[K, T, P]) error {
	panic("not implemented")
}

func (t *HTree[K, T, P]) Iterator() TreeIterator[K, T, P] {
	panic("not implemented")
}

func (t *HTree[K, T, P]) Get(key K) TreeNode[K, T, P] {
	panic("not implemented")
}

func (t *HTree[K, T, P]) Find(key K, exact bool) (int, TreeNode[K, T, P], []int) {
	panic("not implemented")
}

func (t *HTree[K, T, P]) Delete(key K) bool {
	panic("not implemented")
}
