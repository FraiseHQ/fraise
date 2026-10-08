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

// Package trees provides the search trees behind the indexes: an ordered
// B-tree and a random-projection tree for approximate nearest-neighbour search.
//
// [BTree] keeps values sorted by a caller-supplied comparator; the text index
// uses one as its term dictionary. [RPTree] is a [SpatialTree] that partitions
// points by their projection onto random directions. A single RPTree is a weak
// approximator, so the vector index queries a forest of them built from
// independent seeds. Nodes enter a spatial tree as [TreeNode] values exposing
// a [Point]; [VectorNode] and [VectorPoint] fit a [containers.Vector] to those
// contracts.
package trees
