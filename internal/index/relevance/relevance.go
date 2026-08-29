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
// number in between — and the corpus statistics those numbers need, which it
// maintains through the lifecycle hooks. Query-side methods are pure and are
// consulted under the index's readers' lock; lifecycle methods run only on
// the write path.
//
// The model is installed at construction time and never swapped mid-corpus:
// a plugin arriving after the first Insert would score against empty
// statistics. This is contract, not a guarded invariant.
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
	// value, computed once per query from live state. It is called exactly
	// once, before the first Increment, and its result is threaded through
	// every Increment call for that query rather than cached on the model:
	// Relevance methods run under the index's readers' lock, so concurrent
	// queries can share one model instance, and a value cached on the
	// receiver would race across them. Models with nothing to precompute
	// return 0.
	Prepare() P

	// Increment is one document's gain for matching one term. prepared is
	// this query's Prepare() result.
	Increment(weight P, key K, tf int, prepared P) P

	// Finalize folds the accumulated score and match breadth into the final
	// relevance; coverage lives here.
	Finalize(score P, matched, terms int) P
}
