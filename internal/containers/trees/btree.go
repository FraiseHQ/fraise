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

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/hash"
)

// ErrDuplicateValue is returned by Insert when an equal value (per the tree's
// comparator) is already stored in the tree.
var ErrDuplicateValue = errors.New("trees: value already exists")

// BTreeNode is a single node of a BTree. Values within a node are kept sorted
// and each internal node has one more child than it has values.
type BTreeNode[K comparable, T any, P float32 | float64] struct {
	values   []T                   // sorted values held by this node
	children []*BTreeNode[K, T, P] // child pointers (len(values)+1 when internal)
}

// isLeaf reports whether n has no children.
func (n *BTreeNode[K, T, P]) isLeaf() bool {
	return len(n.children) == 0
}

// Hash returns the hash key identifying n, computed by h over a canonical
// string representation of the values n holds. It implements
// hash.Hashable[K, string], letting BTreeNode satisfy TreeNode.
func (n *BTreeNode[K, T, P]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash(fmt.Sprint(n.values))
}

// find returns the index of value within n.values and true if present;
// otherwise it returns the index of the first value greater than value
// (either the insertion point in a leaf, or the child to descend into) and
// false.
func (n *BTreeNode[K, T, P]) find(compare comparator.Comparator[T], value T) (int, bool) {
	lo, hi := 0, len(n.values)
	for lo < hi {
		mid := (lo + hi) / 2
		switch c := compare(value, n.values[mid]); {
		case c == 0:
			return mid, true
		case c < 0:
			hi = mid
		default:
			lo = mid + 1
		}
	}
	return lo, false
}

// BTree is a self-balancing, ordered search tree in which every node holds
// between degree-1 and 2*degree-1 keys. Values of type T are kept sorted by
// the tree's comparator; K only parameterizes the tree so it can share the
// Tree/OrderedTree contracts with other containers in this package (BTree
// itself does not index by K).
type BTree[K comparable, T any, P float32 | float64] struct {
	root    *BTreeNode[K, T, P]
	degree  int                      // minimum degree (t >= 2)
	length  int                      // number of stored values
	compare comparator.Comparator[T] // values ordering
}

// NewBTree returns an empty BTree with the given minimum degree and value
// ordering. degree is clamped to 2 (the smallest valid minimum degree) if
// lower.
func NewBTree[K comparable, T any, P float32 | float64](degree int, compare comparator.Comparator[T]) *BTree[K, T, P] {
	if degree < 2 {
		degree = 2
	}
	return &BTree[K, T, P]{
		degree:  degree,
		compare: compare,
		root:    &BTreeNode[K, T, P]{},
	}
}

// Len reports the number of values currently stored in the tree.
func (t *BTree[K, T, P]) Len() int { return t.length }

// maxValues is the maximum number of values a node may hold before it must
// split.
func (t *BTree[K, T, P]) maxValues() int { return 2*t.degree - 1 }

// minValues is the fewest values a non-root node may hold.
func (t *BTree[K, T, P]) minValues() int { return t.degree - 1 }

// Find reports whether value is stored in the tree (per the tree's
// comparator), returning the stored copy if so.
func (t *BTree[K, T, P]) Find(value T) (T, bool) {
	n := t.root
	for {
		i, found := n.find(t.compare, value)
		if found {
			return n.values[i], true
		}
		if n.isLeaf() {
			var zero T
			return zero, false
		}
		n = n.children[i]
	}
}

// Insert adds value to the tree, splitting nodes as needed to preserve the
// BTree invariants. It returns ErrDuplicateValue if an equal value is already
// stored.
func (t *BTree[K, T, P]) Insert(value T) error {
	root := t.root
	if len(root.values) == t.maxValues() {
		newRoot := &BTreeNode[K, T, P]{children: []*BTreeNode[K, T, P]{root}}
		t.splitChild(newRoot, 0)
		t.root = newRoot
		root = newRoot
	}
	if err := t.insertNonFull(root, value); err != nil {
		return err
	}
	t.length++
	return nil
}

// insertNonFull inserts value into the subtree rooted at n, which must not
// already be full.
func (t *BTree[K, T, P]) insertNonFull(n *BTreeNode[K, T, P], value T) error {
	i, found := n.find(t.compare, value)
	if found {
		return ErrDuplicateValue
	}

	if n.isLeaf() {
		n.values = append(n.values, value)
		copy(n.values[i+1:], n.values[i:])
		n.values[i] = value
		return nil
	}

	child := n.children[i]
	if len(child.values) == t.maxValues() {
		t.splitChild(n, i)
		switch c := t.compare(value, n.values[i]); {
		case c == 0:
			return ErrDuplicateValue
		case c > 0:
			i++
		}
	}
	return t.insertNonFull(n.children[i], value)
}

// splitChild splits the full child at parent.children[i] into two nodes
// around their median value, which moves up into parent.
func (t *BTree[K, T, P]) splitChild(parent *BTreeNode[K, T, P], i int) {
	degree := t.degree
	child := parent.children[i]
	mid := child.values[degree-1]

	right := &BTreeNode[K, T, P]{values: append([]T(nil), child.values[degree:]...)}
	if !child.isLeaf() {
		right.children = append([]*BTreeNode[K, T, P](nil), child.children[degree:]...)
		child.children = child.children[:degree]
	}
	child.values = child.values[:degree-1]

	parent.children = append(parent.children, nil)
	copy(parent.children[i+2:], parent.children[i+1:])
	parent.children[i+1] = right

	parent.values = append(parent.values, mid)
	copy(parent.values[i+1:], parent.values[i:])
	parent.values[i] = mid
}

// Delete removes value from the tree (per the tree's comparator), reporting
// whether a value was removed.
func (t *BTree[K, T, P]) Delete(value T) bool {
	if !t.delete(t.root, value) {
		return false
	}
	if len(t.root.values) == 0 && !t.root.isLeaf() {
		t.root = t.root.children[0]
	}
	t.length--
	return true
}

// delete removes value from the subtree rooted at n, rebalancing along the
// way so every non-root node keeps at least minValues() values.
func (t *BTree[K, T, P]) delete(n *BTreeNode[K, T, P], value T) bool {
	i, found := n.find(t.compare, value)

	if found {
		if n.isLeaf() {
			n.values = append(n.values[:i], n.values[i+1:]...)
			return true
		}

		left, right := n.children[i], n.children[i+1]
		switch {
		case len(left.values) > t.minValues():
			pred := t.max(left)
			n.values[i] = pred
			return t.delete(left, pred)
		case len(right.values) > t.minValues():
			succ := t.min(right)
			n.values[i] = succ
			return t.delete(right, succ)
		default:
			t.mergeChildren(n, i)
			return t.delete(left, value)
		}
	}

	if n.isLeaf() {
		return false
	}

	child := n.children[i]
	if len(child.values) == t.minValues() {
		child = t.fill(n, i)
	}
	return t.delete(child, value)
}

// max returns the greatest value stored in the subtree rooted at n.
func (t *BTree[K, T, P]) max(n *BTreeNode[K, T, P]) T {
	for !n.isLeaf() {
		n = n.children[len(n.children)-1]
	}
	return n.values[len(n.values)-1]
}

// min returns the smallest value stored in the subtree rooted at n.
func (t *BTree[K, T, P]) min(n *BTreeNode[K, T, P]) T {
	for !n.isLeaf() {
		n = n.children[0]
	}
	return n.values[0]
}

// mergeChildren merges n.children[i], n.values[i] and n.children[i+1] into a
// single node stored at n.children[i].
func (t *BTree[K, T, P]) mergeChildren(n *BTreeNode[K, T, P], i int) {
	left, right := n.children[i], n.children[i+1]

	left.values = append(left.values, n.values[i])
	left.values = append(left.values, right.values...)
	left.children = append(left.children, right.children...)

	n.values = append(n.values[:i], n.values[i+1:]...)
	n.children = append(n.children[:i+1], n.children[i+2:]...)
}

// fill ensures n.children[i] holds more than minValues() values before a
// value is removed from it, borrowing from a sibling or merging as needed. It
// returns the (possibly merged) node now at n.children[i].
func (t *BTree[K, T, P]) fill(n *BTreeNode[K, T, P], i int) *BTreeNode[K, T, P] {
	switch {
	case i > 0 && len(n.children[i-1].values) > t.minValues():
		t.borrowFromLeft(n, i)
	case i < len(n.children)-1 && len(n.children[i+1].values) > t.minValues():
		t.borrowFromRight(n, i)
	case i < len(n.children)-1:
		t.mergeChildren(n, i)
	default:
		i--
		t.mergeChildren(n, i)
	}
	return n.children[i]
}

// borrowFromLeft moves the last value of n.children[i-1] up into n and the
// separator at n.values[i-1] down into n.children[i].
func (t *BTree[K, T, P]) borrowFromLeft(n *BTreeNode[K, T, P], i int) {
	child, left := n.children[i], n.children[i-1]

	child.values = append([]T{n.values[i-1]}, child.values...)
	if !left.isLeaf() {
		lastChild := left.children[len(left.children)-1]
		child.children = append([]*BTreeNode[K, T, P]{lastChild}, child.children...)
		left.children = left.children[:len(left.children)-1]
	}
	n.values[i-1] = left.values[len(left.values)-1]
	left.values = left.values[:len(left.values)-1]
}

// borrowFromRight moves the first value of n.children[i+1] up into n and the
// separator at n.values[i] down into n.children[i].
func (t *BTree[K, T, P]) borrowFromRight(n *BTreeNode[K, T, P], i int) {
	child, right := n.children[i], n.children[i+1]

	child.values = append(child.values, n.values[i])
	if !right.isLeaf() {
		firstChild := right.children[0]
		child.children = append(child.children, firstChild)
		right.children = right.children[1:]
	}
	n.values[i] = right.values[0]
	right.values = right.values[1:]
}

// Values returns every stored value in ascending order.
func (t *BTree[K, T, P]) Values() []T {
	values := make([]T, 0, t.length)
	var walk func(n *BTreeNode[K, T, P])
	walk = func(n *BTreeNode[K, T, P]) {
		for i, v := range n.values {
			if !n.isLeaf() {
				walk(n.children[i])
			}
			values = append(values, v)
		}
		if !n.isLeaf() {
			walk(n.children[len(n.children)-1])
		}
	}
	walk(t.root)
	return values
}
