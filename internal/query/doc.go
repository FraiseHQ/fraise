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

// Package query defines the executable form of a Fraise query: the Recall
// and Remember commands, and the Stream that runs one of them for a request.
//
// [Parse] turns FQL text into a [Query] through the parser, binds the vector
// parameters supplied out of band, and rejects a top:, depth: or vector length
// outside its configured range. A Query hashes itself for the engine's plan
// cache, through the [hash.Hashable] contract, and plans into a Stream that
// the scheduler commits against the selected graph, under the shared lock for
// a read and the exclusive lock for a write.
//
// The plan cache hands the same query object to every request that hashes
// alike, so a Query carries nothing per request: the explain flag and the
// result live on the Stream, and parse warnings are returned beside the
// query. For the same reason Hash must fold in every field that changes what a
// query reads or writes, or a cache hit would run another query's plan.
package query
