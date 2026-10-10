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

// Relevance is the pluggable relevance model of a text index. The index owns
// tokenization, postings and the total-order ranking; Relevance owns every
// number in between, and the corpus statistics those numbers need, which it
// maintains through the lifecycle hooks. Query-side methods only read the
// model's state, since concurrent searches call them under the graph's read
// lock; lifecycle methods run only on the write path.
//
// The model is installed before the first Insert and never swapped: one
// installed later would score against statistics missing every document
// already indexed. The index does not enforce this; it is part of the
// contract.
type Relevance[K comparable, P float32 | float64] interface {
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
	Weight(df, docs int) P

	// Prepare folds whatever corpus-wide statistic Increment needs into one
	// value, computed from live state once per query, before the first
	// Increment. The result is passed to every Increment of that query rather
	// than cached on the model: concurrent queries share one model under the
	// graph's read lock, and a value cached on the receiver would race across
	// them. Models with nothing to precompute return 0.
	Prepare() P

	// Increment is one document's gain for matching one term at the
	// posting's term frequency, normalised by the document's own recorded
	// length. prepared is this query's Prepare() result.
	Increment(weight P, key K, tf int, prepared P) P

	// Gain is Increment generalised to a term frequency and a document
	// length the index measured itself: a window search's, where a
	// document's count and length include a share of its temporal
	// neighbours', which is why both are P rather than int. Increment is Gain
	// at the posting's count and the document's own Length.
	Gain(weight P, tf P, length P, prepared P) P

	// Length is the document length the model normalises by, in tokens as
	// it recorded them at Indexed, and 0 for a model that does not
	// normalise; the window's length is built from it.
	Length(key K) P

	// Finalize folds the accumulated score and match breadth into the final
	// relevance; coverage lives here. matched and total carry the idf mass
	// (summed Weight) of the query terms the document matched, m, and of every
	// query term a live document holds, W, in 1/1024 fixed point: matched is
	// ⌊1024·m⌋ and total is ⌊1024·W⌋ + 1, which is never zero.
	Finalize(score P, matched, total int) P
}
