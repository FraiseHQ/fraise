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

// MatchCount is the default relevance model: one point per query-term
// occurrence, repeats included — exactly the ranking the index shipped with
// before relevance became pluggable. A match count needs no corpus
// statistics, so every lifecycle hook is a no-op.
type MatchCount[K comparable, P float32 | float64] struct{}

// Indexed records nothing: a match count keeps no statistics.
func (MatchCount[K, P]) Indexed(K, []string) {}

// Removed retires nothing, for the same reason.
func (MatchCount[K, P]) Removed(K, []string) {}

// Terms is the identity: a repeated query term counts every time it appears.
func (MatchCount[K, P]) Terms(tokens []string) []string { return tokens }

// Weight prices every term at one point.
func (MatchCount[K, P]) Weight(int, int) P { return 1 }

// Prepare returns 0: a match count has no per-query statistic to fold.
func (MatchCount[K, P]) Prepare() P { return 0 }

// Increment awards the term's weight regardless of term frequency: matching
// is binary per (document, term-occurrence). prepared is unused.
func (MatchCount[K, P]) Increment(weight P, _ K, _ int, _ P) P { return weight }

// Finalize is the identity: the count is the relevance.
func (MatchCount[K, P]) Finalize(score P, _, _ int) P { return score }
