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

// Package containers holds the value types and generic data structures that
// the query, graph and index layers share.
//
// [Vector] and [TimeValue] are query values. Each carries the key type K of
// the query holding it, so it hashes itself through that query's
// [hash.Hasher] with lossless material: exact hex floats, RFC3339Nano
// instants. A component that hashed lossily would let two different queries
// share one cached plan. [ParseTimeValue] reads the since: and until: bounds,
// refusing a duration a time.Duration cannot hold rather than letting it wrap
// into the future.
//
// [Heap], [PriorityQueue] and [TopK] rank keys by score. Heap is a max-heap
// holding one item per key, so an item can be found and removed by key;
// PriorityQueue wraps it in a lock for concurrent callers. TopK keeps the k
// best pairs under a total order, score then key, so the pairs it keeps never
// depend on the order they were offered.
package containers
