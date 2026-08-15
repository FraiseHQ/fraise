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

import "math"

// Relevance is the pluggable relevance model of a text index. The index owns
// tokenization, postings and the total-order ranking; Relevance owns every
// number in between — and the corpus statistics those numbers need, which it
// maintains through the lifecycle hooks. Query-side methods are pure and are
// consulted under the index's readers' lock; lifecycle methods run only on
// the write path.
//
// The model is installed at construction time and never swapped mid-corpus:
// a plugin arriving after the first Insert would score against empty
// statistics. This is contract, not a guarded invariant.
type Relevance[K comparable] interface {
	// Indexed records a document's statistics. On update the index calls
	// Removed with the old tokens first, so key is always absent here.
	Indexed(key K, tokens []string)

	// Removed retires a document. Implementations trust their own
	// bookkeeping over tokens, staying immune to a tokenizer swap between
	// insert and delete.
	Removed(key K, tokens []string)

	// Terms prepares the query token stream — identity if repetition should
	// count, distinct-first-occurrence if weights must not double-count.
	Terms(tokens []string) []string

	// Weight prices one query term from its document frequency and the
	// corpus size; called once per term with a non-empty posting.
	Weight(df, docs int) float64

	// Prepare folds whatever corpus-wide statistic Increment needs into one
	// value, computed once per query from live state. It is called exactly
	// once, before the first Increment, and its result is threaded through
	// every Increment call for that query rather than cached on the model:
	// Relevance methods run under the index's readers' lock, so concurrent
	// queries can share one model instance, and a value cached on the
	// receiver would race across them. Models with nothing to precompute
	// return 0.
	Prepare() float64

	// Increment is one document's gain for matching one term. prepared is
	// this query's Prepare() result.
	Increment(weight float64, key K, tf int, prepared float64) float64

	// Finalize folds the accumulated score and match breadth into the final
	// relevance; coverage lives here.
	Finalize(score float64, matched, terms int) float64
}

// MatchCount is the default relevance model: one point per query-term
// occurrence, repeats included — exactly the ranking the index shipped with
// before relevance became pluggable. A match count needs no corpus
// statistics, so every lifecycle hook is a no-op.
type MatchCount[K comparable] struct{}

// Indexed records nothing: a match count keeps no statistics.
func (MatchCount[K]) Indexed(K, []string) {}

// Removed retires nothing, for the same reason.
func (MatchCount[K]) Removed(K, []string) {}

// Terms is the identity: a repeated query term counts every time it appears.
func (MatchCount[K]) Terms(tokens []string) []string { return tokens }

// Weight prices every term at one point.
func (MatchCount[K]) Weight(int, int) float64 { return 1 }

// Prepare returns 0: a match count has no per-query statistic to fold.
func (MatchCount[K]) Prepare() float64 { return 0 }

// Increment awards the term's weight regardless of term frequency: matching
// is binary per (document, term-occurrence). prepared is unused.
func (MatchCount[K]) Increment(weight float64, _ K, _ int, _ float64) float64 { return weight }

// Finalize is the identity: the count is the relevance.
func (MatchCount[K]) Finalize(score float64, _, _ int) float64 { return score }

// BM25 constants: the standard Robertson–Walker defaults, internal on
// purpose. The retrieval methodology (see docs/design.md) requires raw,
// untuned BM25 units — the anchor-level excess statistic sums seed masses
// across documents, and a per-dataset knob here would break the
// commensurability that keeps the text channel and those sums on one scale.
const (
	// bm25K1 saturates term frequency: repetitions of a term add ever less.
	bm25K1 = 1.2
	// bm25B is document-length normalization: 0 ignores length, 1 fully
	// penalizes long documents; 0.75 is the standard operating point.
	bm25B = 0.75
	// bm25N1 is the length-norm term's corpus-independent half,
	// bm25K1*(1-bm25B). It needs no per-query recomputation, unlike the
	// avgdl-dependent half Prepare folds into n2.
	bm25N1 = bm25K1 * (1 - bm25B)
)

// BM25 is the Robertson–Walker relevance model scaled by query coverage:
// idf-weighted, length-normalized term frequency, times the fraction of
// distinct query terms the document matched, so a document matching more of
// the query outranks one repeating a single term. It owns the length
// statistics its normalization needs and maintains them through the
// lifecycle hooks.
type BM25[K comparable] struct {
	lengths  map[K]int // key -> document length in tokens
	totalLen int       // sum of lengths (avgdl = totalLen / len(lengths))
}

// NewBM25 returns a BM25 relevance model with empty corpus statistics.
func NewBM25[K comparable]() *BM25[K] {
	return &BM25[K]{lengths: make(map[K]int)}
}

// Indexed records the document's length into the corpus statistics.
func (b *BM25[K]) Indexed(key K, tokens []string) {
	b.lengths[key] = len(tokens)
	b.totalLen += len(tokens)
}

// Removed retires the document's length. The recorded length is used, not
// len(tokens): the tokenizer may have changed since the insert, and the
// statistics must retire exactly what they admitted.
func (b *BM25[K]) Removed(key K, _ []string) {
	b.totalLen -= b.lengths[key]
	delete(b.lengths, key)
}

// Terms deduplicates to distinct query terms, first occurrence order: idf
// already prices a term's informativeness, so a term repeated in the query
// must not count twice — and coverage is a fraction of the distinct terms.
func (b *BM25[K]) Terms(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	terms := make([]string, 0, len(tokens))
	for _, term := range tokens {
		if _, dup := seen[term]; dup {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}
	return terms
}

// Weight is the BM25 idf: log(1 + (n − df + 0.5) / (df + 0.5)).
func (b *BM25[K]) Weight(df, docs int) float64 {
	n := float64(docs)
	return math.Log(1 + (n-float64(df)+0.5)/(float64(df)+0.5))
}

// Prepare folds the corpus's current avgdl into n2 = bm25K1*bm25B/avgdl, the
// length-norm term's avgdl-dependent half: computed once here from live
// state rather than cached on the model, so it stays safe under concurrent
// queries sharing this instance, and Increment turns it into one multiply
// per posting entry instead of a division.
func (b *BM25[K]) Prepare() float64 {
	avgdl := float64(b.totalLen) / float64(len(b.lengths))
	return bm25K1 * bm25B / avgdl
}

// Increment is the idf-weighted, length-normalized term-frequency gain. n2
// is this query's Prepare() result.
func (b *BM25[K]) Increment(weight float64, key K, tf int, n2 float64) float64 {
	freq := float64(tf)
	return weight * freq * (bm25K1 + 1) / (freq + bm25N1 + n2*float64(b.lengths[key]))
}

// Finalize scales by coverage: the fraction of distinct query terms the
// document matched. An alternative coverage multiplier is a one-line swap in
// this slot.
func (b *BM25[K]) Finalize(score float64, matched, terms int) float64 {
	return score * float64(matched) / float64(terms)
}
