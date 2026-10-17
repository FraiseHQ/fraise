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

// Package relevance holds the pluggable scoring models of the text index.
//
// A [Relevance] model owns every number between a term match and a
// document's final score. It also owns the corpus statistics those numbers
// need, and the index keeps them current through the Indexed and Removed
// hooks. Keeping the model apart from the index lets scoring change without
// touching tokenization, postings or the ranking order.
//
// [BM25] is the model the server installs by default. It is Robertson–Walker
// BM25 with fixed, untuned constants, scaled by the share of the query's idf
// mass a document matched. [MatchCount] counts matching query terms and keeps
// no statistics. It is the index's own default and stays selectable for
// comparison runs.
package relevance
