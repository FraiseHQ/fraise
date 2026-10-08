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

// Package index defines the search-index contract and its two in-memory
// implementations, a full-text index and an approximate nearest-neighbour
// vector index.
//
// A [SearchIndex] maps keys to values and ranks keys against a query of the
// same type as its values: a [TextIndex] searches strings, a [VectorIndex]
// vectors. Search returns a total order, ties broken by the index's key
// comparator, so truncating to k keeps the same keys for the same query every
// time. [BTreeIndex] implements TextIndex with posting lists in a B-tree term
// dictionary, leaving tokenization to an [nlp.Tokenizer] and scoring to a
// [relevance.Relevance], so higher scores are better. [RPTreeIndex] implements
// VectorIndex with a forest of random-projection trees and scores by distance,
// so lower is nearer.
//
// Updates and deletes leave garbage in both structures; Flush compacts it, and
// each index flushes itself once its garbage outgrows a bound tied to the live
// set. Entries minus Count is the compaction debt outstanding.
package index
