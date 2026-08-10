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

import (
	"errors"
	"sort"

	"github.com/RonsenbergVI/fraise/internal/comparator"
	"github.com/RonsenbergVI/fraise/internal/containers/trees"
	"github.com/RonsenbergVI/fraise/pkg/logger"
)

// compile-time check that BTreeIndex is a TextIndex.
var _ TextIndex[int, float64] = (*BTreeIndex[int, float64])(nil)

// BTreeIndex is a full-text index backed by an ordered BTree from the
// containers/trees submodule. The tree holds the set of indexed terms in
// sorted order; the term -> document-keys mapping itself (the postings) is
// kept in a plain map, since BTree only tracks membership, not payloads. It
// implements TextIndex.
type BTreeIndex[K comparable, P float32 | float64] struct {
	tree      *trees.BTree[K, string, P] // ordered term dictionary (P is unused)
	postings  map[string]map[K]struct{}  // term -> keys of documents containing it
	documents map[K]string               // key -> raw document text
	tokenizer Tokenizer
	compare   comparator.Comparator[K] // document key ordering
}

// NewBTreeIndex returns an empty BTreeIndex whose term dictionary is an ordered
// BTree over lexicographically sorted terms. compare orders document keys, the
// tiebreak Search ranks equally-matching documents by.
func NewBTreeIndex[K comparable, P float32 | float64](compare comparator.Comparator[K]) *BTreeIndex[K, P] {
	return &BTreeIndex[K, P]{
		tree:      trees.NewBTree[K, string, P](32, comparator.OrderedComparator[string]),
		postings:  make(map[string]map[K]struct{}),
		documents: make(map[K]string),
		tokenizer: SimpleTokenizer{},
		compare:   compare,
	}
}

// Insert tokenizes value and adds key to the term dictionary, then records the
// raw document for retrieval. If key is already indexed, its previous document
// is replaced.
func (idx *BTreeIndex[K, P]) Insert(key K, value string) error {
	return idx.index(key, value)
}

// Retrieve returns the raw document stored under key.
func (idx *BTreeIndex[K, P]) Retrieve(key K) (string, error) {
	value, ok := idx.documents[key]
	if !ok {
		return "", ErrIndexNotFound
	}
	return value, nil
}

// Update re-tokenizes the document under key, adjusting the affected posting
// lists.
func (idx *BTreeIndex[K, P]) Update(key K, value string) error {
	if _, ok := idx.documents[key]; !ok {
		return ErrIndexNotFound
	}
	return idx.index(key, value)
}

// Delete removes key from every posting list it appears in and drops the stored
// document.
func (idx *BTreeIndex[K, P]) Delete(key K) error {
	value, ok := idx.documents[key]
	if !ok {
		return ErrIndexNotFound
	}
	idx.removePostings(key, value)
	delete(idx.documents, key)
	return nil
}

// Search tokenizes query, combines the matching posting lists and returns the
// document keys ranked by relevance (the number of query terms they contain),
// best first, with a parallel slice of those match counts as scores. Documents
// matching the same number of terms are ordered by key, so the ranking is the
// total order SearchIndex promises: the candidates come out of the posting maps
// in an arbitrary order, and it is the whole ranking — not just the tied group —
// that would otherwise vary between identical queries once k truncates it.
// k bounds the number of results; k <= 0 returns every match.
func (idx *BTreeIndex[K, P]) Search(query string, k int) ([]K, []P, error) {
	if len(idx.documents) == 0 {
		return nil, nil, ErrEmptyIndex
	}

	counts := make(map[K]int)
	for _, term := range idx.tokenizer.Tokenize(query) {
		for key := range idx.postings[term] {
			counts[key]++
		}
	}

	keys := make([]K, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return idx.compare(keys[i], keys[j]) < 0
	})
	if k > 0 && len(keys) > k {
		keys = keys[:k]
	}

	scores := make([]P, len(keys))
	for i, key := range keys {
		scores[i] = P(counts[key])
	}
	logger.Debug("Text search matched documents", "matches", len(keys), "k", k)
	return keys, scores, nil
}

// Size reports the approximate in-memory footprint of the index in MiB.
func (idx *BTreeIndex[K, P]) Size() int {
	var bytes int
	for _, doc := range idx.documents {
		bytes += len(doc)
	}
	for term, keys := range idx.postings {
		bytes += len(term) + len(keys)*8
	}
	return bytes / (1024 * 1024)
}

// Count reports the number of indexed documents.
func (idx *BTreeIndex[K, P]) Count() int {
	return len(idx.documents)
}

// Entries equals Count: this index compacts eagerly (deletes remove postings
// immediately), so it holds no garbage between Flushes.
func (idx *BTreeIndex[K, P]) Entries() int {
	return idx.Count()
}

// Flush is a no-op: this index keeps no buffered state to compact.
func (idx *BTreeIndex[K, P]) Flush() error {
	return nil
}

// index tokenizes value, removes any postings left over from a previous
// document stored under key, and records the new term postings and document.
func (idx *BTreeIndex[K, P]) index(key K, value string) error {
	if old, ok := idx.documents[key]; ok {
		idx.removePostings(key, old)
	}

	for _, term := range idx.tokenizer.Tokenize(value) {
		keys, ok := idx.postings[term]
		if !ok {
			if err := idx.tree.Insert(term); err != nil && !errors.Is(err, trees.ErrDuplicateValue) {
				return err
			}
			keys = make(map[K]struct{})
			idx.postings[term] = keys
		}
		keys[key] = struct{}{}
	}

	idx.documents[key] = value
	return nil
}

// removePostings drops key from the posting list of every term in value,
// removing terms from the dictionary once no document references them.
func (idx *BTreeIndex[K, P]) removePostings(key K, value string) {
	for _, term := range idx.tokenizer.Tokenize(value) {
		keys, ok := idx.postings[term]
		if !ok {
			continue
		}
		delete(keys, key)
		if len(keys) == 0 {
			delete(idx.postings, term)
			idx.tree.Delete(term)
		}
	}
}
