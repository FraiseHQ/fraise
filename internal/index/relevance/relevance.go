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
// tokenization, postings and the total-order ranking. Relevance owns every
// number in between, and the corpus statistics those numbers need, which it
// keeps through the lifecycle hooks. Concurrent searches call the query-side
// methods under the graph's read lock, so those methods only read the
// model's state. Lifecycle methods run only on the write path.
//
// The model is installed before the first Insert and never swapped. One
// installed later would score against statistics missing every document
// already indexed. The index does not enforce this. It is part of the
// contract.
type Relevance[K comparable, P float32 | float64] interface {
	// Name is the model's db.relevance-model.name, one of
	// config.RelevanceModels. Startup compares it with the configured name.
	// A model that reports the wrong name would hide a wiring failure that
	// every other test would miss: ranking degrades and nothing fails.
	Name() string

	// Indexed records a document's statistics. On update the index calls
	// Removed with the old tokens first, so key is always absent here.
	Indexed(key K, tokens []string)

	// Removed retires a document. Implementations trust their own
	// bookkeeping over tokens, so a tokenizer swap between insert and delete
	// cannot skew them.
	Removed(key K, tokens []string)

	// Terms prepares the query token stream. It returns the tokens unchanged
	// if repetition should count, or the distinct tokens in first-occurrence
	// order if weights must not double-count.
	Terms(tokens []string) []string

	// Weight prices one query term from its document frequency and the
	// corpus size. Search calls it once per query term that a live document
	// holds.
	Weight(df, docs int) P

	// Prepare folds the corpus-wide statistic Increment needs into one value,
	// read from live state. Search calls it once per query, before the first
	// Increment, and passes the result to every Increment of that query. The
	// value is not cached on the model: concurrent queries share one model
	// under the graph's read lock, so a cached value would race. Models with
	// nothing to precompute return 0.
	Prepare() P

	// Increment is one document's gain for matching one term. prepared is
	// this query's Prepare() result.
	Increment(weight P, key K, tf int, prepared P) P

	// Finalize folds the accumulated score and match breadth into the final
	// relevance. Coverage is applied here. matched and total carry idf mass
	// (summed Weight) in 1/1024 fixed point:
	//   - m is the mass of the query terms the document matched, and matched
	//     is ⌊1024·m⌋.
	//   - W is the mass of every query term a live document holds, and total
	//     is ⌊1024·W⌋ + 1, which is never zero.
	Finalize(score P, matched, total int) P
}
