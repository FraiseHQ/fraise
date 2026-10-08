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

// Package graph is the temporal memory graph: the store each FQL @N selector
// addresses, and the search that runs over it.
//
// A [Graph] holds [Node] values under their own keys. Facts, topics and named
// entities are the vertices; [Mentions] and [IsAbout] are the relationships
// from a fact to the entities it names and the topics it is filed under, and
// are stored as nodes too. Every node key is its Hash, built from a type tag
// and its text, so identity is (type, value): two nodes of different types
// reading the same text never overwrite each other. [InMemoryGraph] is the
// implementation, backing the graph with a full-text index and a vector index.
//
// [Graph.Search] seeds candidates from the text and vector indexes. When the
// query names a topic or entity, a [Traversal] ([ExcessTraversal] or [BFS])
// expands each seed through its anchors; each candidate's contributions are
// then folded by a [scoring.Scorer], and a [Ranking] such as [PageRank] may
// boost the result. Graph methods take no lock: callers hold the graph's read
// or write lock around the calls they compose.
package graph
