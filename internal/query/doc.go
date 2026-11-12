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

// Package query defines the executable form of a Fraise query: the Recall,
// Remember and When commands, and the Stream that runs one of them for a
// request.
//
// [Parse] turns FQL text into a [Query] through the parser, which binds the
// vector parameters supplied out of band. Parse rejects a top:, depth: or
// vector length outside its configured range. A Query hashes itself for the
// engine's plan cache, through the [hash.Hashable] contract. It plans into a
// Stream, which the scheduler commits against the selected graph. A read runs
// under the shared lock, and a write under the exclusive lock.
//
// A [When] is a recall that answers with a time: the span its hits hold on the
// clock it names. It runs as a statement, and nested in a recall's since: or
// until: bound, where its answer is the bound. A nested query is a Query like
// any other. The recall holds it in a [TimeOperand], folds its key into its
// own, and plans it into a stream that hangs from the recall's. That stream
// commits first, on the same graph and under the same lock, so the bound and
// the search read one state of the graph.
//
// The plan cache hands the same query object to every request equal to the
// one it was planned for. So a Query carries nothing per request. The explain
// flag, the result and the warnings raised while committing live on the
// Stream, and parse warnings are returned beside the query.
// For the same reason, Equal covers every field that changes what a query
// reads or writes. Otherwise a cache hit would run another query's plan. Hash
// covers the same fields, save the times a remember gives its fact: a fact
// stated again with other times keeps its key and takes the cached entry over.
package query
