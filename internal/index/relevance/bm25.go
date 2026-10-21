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

import (
	"math"

	"github.com/FraiseHQ/fraise/internal/config"
)

// BM25 constants are the standard Robertson–Walker defaults, and they are not
// configurable on purpose. The retrieval methodology needs raw, untuned BM25
// units. The anchor-level excess statistic sums seed masses across documents.
// A per-dataset knob here would put the text channel and those sums on
// different scales. The model is documented at
// https://docs.getfraise.dev/docs/retrieval/ranking/hybrid-retrieval.
const (
	// bm25K1 saturates term frequency: each repeat of a term adds less.
	bm25K1 = 1.2
	// bm25B sets document-length normalization. 0 ignores length, and 1
	// fully penalizes long documents. 0.75 is the standard operating point.
	bm25B = 0.75
	// bm25N1 is the corpus-independent half of the length norm,
	// bm25K1*(1-bm25B). Unlike the avgdl-dependent half that Prepare folds
	// into n2, it needs no per-query recomputation.
	bm25N1 = bm25K1 * (1 - bm25B)
)

// BM25 is the Robertson–Walker relevance model scaled by query coverage. The
// score is idf-weighted, length-normalized term frequency, times the share of
// the query's idf mass the document matched. So a document that matches more
// of the query, and its rarer terms, outranks one that repeats a single term.
// BM25 keeps the length statistics its normalization needs and updates them
// through the lifecycle hooks.
type BM25[K comparable, P float32 | float64] struct {
	lengths  map[K]int // key -> document length in tokens
	totalLen int       // sum of lengths (avgdl = totalLen / len(lengths))
}

// NewBM25 returns a BM25 relevance model with empty corpus statistics.
func NewBM25[K comparable, P float32 | float64]() *BM25[K, P] {
	return &BM25[K, P]{lengths: make(map[K]int)}
}

// Name is [config.RelevanceBM25].
func (b *BM25[K, P]) Name() string { return config.RelevanceBM25 }

// Indexed records the document's length into the corpus statistics.
func (b *BM25[K, P]) Indexed(key K, tokens []string) {
	b.lengths[key] = len(tokens)
	b.totalLen += len(tokens)
}

// Removed retires the document's recorded length, not len(tokens). The
// statistics must retire exactly what they admitted.
func (b *BM25[K, P]) Removed(key K, _ []string) {
	b.totalLen -= b.Lengths()[key]
	delete(b.lengths, key)
}

// Terms keeps the distinct query terms, in first-occurrence order. A term's
// idf already prices how informative it is, so a repeated term must not count
// twice. Coverage is also a share of the distinct terms' idf mass.
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

// Prepare returns n2 = bm25K1*bm25B/avgdl, the avgdl-dependent half of the
// length norm, from the current statistics. It leaves Increment one multiply
// per posting entry instead of a division.
func (b *BM25[K, P]) Prepare() P {
	lengths, totalLen := b.Lengths(), b.TotalLen()
	if len(lengths) == 0 || totalLen == 0 {
		return 0
	}
	avgdl := P(totalLen) / P(len(lengths))
	return bm25K1 * bm25B / avgdl
}

// Length is the document's recorded length in tokens, the value Increment
// normalises a plain search by. A key never indexed has length 0.
func (b *BM25[K, P]) Length(key K) P {
	return P(b.Lengths()[key])
}

// Increment is Gain at the posting's count and the document's own recorded
// length. n2 is this query's Prepare() result.
func (b *BM25[K, P]) Increment(weight P, key K, tf int, n2 P) P {
	return b.Gain(weight, P(tf), P(b.Lengths()[key]), n2)
}

// Gain is the idf-weighted, length-normalized term-frequency gain at term
// frequency tf and document length length.
func (b *BM25[K, P]) Gain(weight P, tf P, length P, n2 P) P {
	return weight * tf * (bm25K1 + 1) / (tf + bm25N1 + n2*length)
}

// Finalize scales score by coverage, matched over total. Coverage is the
// share of the query's idf mass the document matched, in the fixed point that
// Relevance.Finalize describes. A full match scales by just under 1. A rare
// term covers more of the query than a common one.
func (b *BM25[K, P]) Finalize(score P, matched, total int) P {
	return score * P(matched) / P(total)
}

// TotalLen returns the sum of recorded document lengths in tokens.
func (b BM25[K, P]) TotalLen() int {
	return b.totalLen
}

// Lengths returns the recorded document lengths in tokens, by key. It is the
// model's own map, not a copy.
func (b BM25[K, P]) Lengths() map[K]int {
	return b.lengths
}
