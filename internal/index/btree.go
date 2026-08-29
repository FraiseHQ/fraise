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

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/containers/trees"
	"github.com/FraiseHQ/fraise/internal/index/nlp"
	"github.com/FraiseHQ/fraise/internal/index/relevance"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// BTreeIndex is a full-text index backed by an ordered BTree from the
// containers/trees submodule. The tree holds the set of indexed terms in
// sorted order; the postings (term -> document key -> term frequency) are
// kept in a plain map, since BTree only tracks membership, not payloads. The
// installed Relevance model owns every scoring number and whatever corpus
// statistics it needs; the index owns tokenization, postings and the
// total-order ranking. It implements TextIndex.
type BTreeIndex[K comparable, P float32 | float64] struct {
	tree      *trees.BTree[K, string, P] // ordered term dictionary (P is unused)
	postings  map[string]map[K]int       // term -> document key -> term frequency
	documents map[K]string               // key -> raw document text
	tokenizer nlp.Tokenizer
	relevance relevance.Relevance[K, P]
	compare   comparator.Comparator[K] // document key ordering
}

// NewBTreeIndex returns an empty BTreeIndex whose term dictionary is an ordered
// BTree over lexicographically sorted terms. compare orders document keys, the
// tiebreak Search ranks equally-scoring documents by. The index starts on the
// MatchCount relevance model — the ranking it always had.
func NewBTreeIndex[K comparable, P float32 | float64](compare comparator.Comparator[K]) *BTreeIndex[K, P] {
	return &BTreeIndex[K, P]{
		tree:      trees.NewBTree[K, string, P](32, comparator.OrderedComparator[string]),
		postings:  make(map[string]map[K]int),
		documents: make(map[K]string),
		tokenizer: nlp.SimpleTokenizer{},
		relevance: relevance.MatchCount[K, P]{},
		compare:   compare,
	}
}

// SetTokenizer installs the tokenizer, mirroring SetRelevance. It must be
// called before the first Insert and never after: postings tokenized under
// the old scheme would be unreachable under the new one. nil is ignored
// rather than stored: an index without a tokenizer cannot index.
func (idx *BTreeIndex[K, P]) SetTokenizer(t nlp.Tokenizer) {
	if t == nil {
		return
	}
	idx.tokenizer = t
}

// SetRelevance installs the relevance model, mirroring the graph's Set*
// precedent. It must be called before the first Insert and never after: the
// model maintains its own corpus statistics through the lifecycle hooks, so
// one arriving mid-corpus would score against empty statistics. nil is
// ignored rather than stored: an index without a relevance model cannot rank.
func (idx *BTreeIndex[K, P]) SetRelevance(r relevance.Relevance[K, P]) {
	if r == nil {
		return
	}
	idx.relevance = r
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

// Search tokenizes query and returns the matching document keys ranked by
// the installed Relevance model, best first, with the scores in a parallel
// slice. The index runs the loop — term stream, postings, accumulation — and
// the model supplies every number: how the query tokens become terms, what a
// term is worth, what a document gains per match, and how match breadth folds
// into the final relevance.
//
// Documents of equal score are ordered by key, so the ranking is the total
// order SearchIndex promises: the candidates come out of the posting maps in
// an arbitrary order, and it is the whole ranking — not just the tied group —
// that would otherwise vary between identical queries once k truncates it.
// k bounds the number of results; k <= 0 returns every match.
func (idx *BTreeIndex[K, P]) Search(query string, k int) ([]K, []P, error) {
	if len(idx.documents) == 0 {
		return nil, nil, ErrEmptyIndex
	}

	terms := idx.relevance.Terms(idx.tokenizer.Tokenize(query))

	// scores/matched are sized to the query's largest posting list up front:
	// every key that can possibly appear in either map comes from one of
	// these postings, so this is the exact worst case, not a guess.
	var maxPosting int
	for _, term := range terms {
		if n := len(idx.postings[term]); n > maxPosting {
			maxPosting = n
		}
	}

	scores := make(map[K]P, maxPosting)
	matched := make(map[K]P, maxPosting)
	prepared := idx.relevance.Prepare()

	var totalW P
	for _, term := range terms {
		posting := idx.postings[term]
		if len(posting) == 0 {
			continue
		}
		weight := idx.relevance.Weight(len(posting), len(idx.documents))
		totalW += weight
		for key, tf := range posting {
			scores[key] += idx.relevance.Increment(weight, key, tf, prepared)
			matched[key]++
		}
	}
	for key := range scores {
		scores[key] = idx.relevance.Finalize(scores[key], int(matched[key]*1024), int(totalW*1024)+1)
	}

	top := containers.NewTopK[K, P](k, idx.compare)

	for key, score := range scores {
		top.Offer(key, score)
	}
	keys, ranked := top.Drain()

	out := make([]P, len(ranked))
	for i, score := range ranked {
		out[i] = P(score)
	}
	logger.Debug("Text search matched documents", "matches", len(keys), "k", k)
	return keys, out, nil
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
// document stored under key (retiring it from the relevance model's
// statistics), then records the new term frequencies, document text, and the
// document's statistics with the model.
func (idx *BTreeIndex[K, P]) index(key K, value string) error {
	if old, ok := idx.documents[key]; ok {
		idx.removePostings(key, old)
	}

	tokens := idx.tokenizer.Tokenize(value)
	for _, term := range tokens {
		keys, ok := idx.postings[term]
		if !ok {
			if err := idx.tree.Insert(term); err != nil && !errors.Is(err, trees.ErrDuplicateValue) {
				return err
			}
			keys = make(map[K]int)
			idx.postings[term] = keys
		}
		keys[key]++
	}

	idx.documents[key] = value
	idx.relevance.Indexed(key, tokens)
	return nil
}

// removePostings drops key from the posting list of every term in value and
// retires the document from the relevance model's statistics, removing terms
// from the dictionary once no document references them. Both callers — an
// update re-indexing under the same key and Delete — retire through here, so
// the model sees exactly one Removed per Indexed.
func (idx *BTreeIndex[K, P]) removePostings(key K, value string) {
	tokens := idx.tokenizer.Tokenize(value)
	for _, term := range tokens {
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
	idx.relevance.Removed(key, tokens)
}
