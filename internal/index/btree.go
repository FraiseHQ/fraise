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

package index

import "github.com/RonsenbergVI/fraise/internal/containers/trees"

// BTreeIndex is a full-text index backed by an ordered BTree from the
// containers/trees submodule. The tree is used as the term dictionary, mapping
// each indexed term to the keys of the documents that contain it. It implements
// TextIndex.
type BTreeIndex[K comparable] struct {
	tree *trees.BTree[string, []K, float32] // term -> document keys (P is unused)
}

// NewBTreeIndex returns an empty BTreeIndex whose term dictionary is an ordered
// BTree over lexicographically sorted terms.
func NewBTreeIndex[K comparable]() *BTreeIndex[K] {
	return &BTreeIndex[K]{
		tree: trees.NewBTree[string, []K, float32](32, func(a, b string) bool { return a < b }),
	}
}

// Insert tokenizes value and adds key to the term dictionary, then records the
// raw document for retrieval.
func (idx *BTreeIndex[K]) Insert(key K, value string) error {
	panic("not implemented")
}

// Retrieve returns the raw document stored under key.
func (idx *BTreeIndex[K]) Retrieve(key K) (string, error) {
	panic("not implemented")
}

// Update re-tokenizes the document under key, adjusting the affected posting
// lists.
func (idx *BTreeIndex[K]) Update(key K, value string) error {
	panic("not implemented")
}

// Delete removes key from every posting list it appears in and drops the stored
// document.
func (idx *BTreeIndex[K]) Delete(key K) error {
	panic("not implemented")
}

// Search tokenizes query, combines the matching posting lists and returns the
// document keys ranked by relevance.
func (idx *BTreeIndex[K]) Search(query string) ([]K, error) {
	panic("not implemented")
}

// Size reports the approximate in-memory footprint of the index in MiB.
func (idx *BTreeIndex[K]) Size() int {
	panic("not implemented")
}

// Count reports the number of indexed documents.
func (idx *BTreeIndex[K]) Count() int {
	panic("not implemented")
}

// Flush compacts the term dictionary and posting lists.
func (idx *BTreeIndex[K]) Flush() error {
	panic("not implemented")
}
