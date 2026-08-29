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

package relevance

import "math"

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
type BM25[K comparable, P float32 | float64] struct {
	lengths  map[K]int // key -> document length in tokens
	totalLen int       // sum of lengths (avgdl = totalLen / len(lengths))
}

// NewBM25 returns a BM25 relevance model with empty corpus statistics.
func NewBM25[K comparable, P float32 | float64]() *BM25[K, P] {
	return &BM25[K, P]{lengths: make(map[K]int)}
}

// Indexed records the document's length into the corpus statistics.
func (b *BM25[K, P]) Indexed(key K, tokens []string) {
	b.lengths[key] = len(tokens)
	b.totalLen += len(tokens)
}

// Removed retires the document's length. The recorded length is used, not
// len(tokens): the tokenizer may have changed since the insert, and the
// statistics must retire exactly what they admitted.
func (b *BM25[K, P]) Removed(key K, _ []string) {
	b.totalLen -= b.lengths[key]
	delete(b.lengths, key)
}

// Terms deduplicates to distinct query terms, first occurrence order: idf
// already prices a term's informativeness, so a term repeated in the query
// must not count twice — and coverage is a fraction of the distinct terms.
func (b *BM25[K, P]) Terms(tokens []string) []string {
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
func (b *BM25[K, P]) Weight(df, docs int) P {
	n := P(docs)
	return P(math.Log(float64(1 + (n-P(df)+0.5)/(P(df)+0.5))))
}

// Prepare folds the corpus's current avgdl into n2 = bm25K1*bm25B/avgdl, the
// length-norm term's avgdl-dependent half: computed once here from live
// state rather than cached on the model, so it stays safe under concurrent
// queries sharing this instance, and Increment turns it into one multiply
// per posting entry instead of a division.
func (b *BM25[K, P]) Prepare() P {
	avgdl := P(b.totalLen) / P(len(b.lengths))
	return bm25K1 * bm25B / avgdl
}

// Increment is the idf-weighted, length-normalized term-frequency gain. n2
// is this query's Prepare() result.
func (b *BM25[K, P]) Increment(weight P, key K, tf int, n2 P) P {
	freq := P(tf)
	return weight * freq * (bm25K1 + 1) / (freq + bm25N1 + n2*P(b.lengths[key]))
}

// Finalize scales by coverage: the fraction of distinct query terms the
// document matched. An alternative coverage multiplier is a one-line swap in
// this slot.
func (b *BM25[K, P]) Finalize(score P, matched, terms int) P {
	return score * P(matched) / P(terms)
}

// TotalLen returns the sum of recorded document lengths in tokens.
func (b BM25[K, P]) TotalLen() int {
	return b.totalLen
}

// Getter for lengths attribute
func (b BM25[K, P]) Lengths() map[K]int {
	return b.lengths
}
