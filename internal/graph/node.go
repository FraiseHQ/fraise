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

package graph

import (
	"time"

	"github.com/FraiseHQ/fraise/internal/hash"
)

// NodeAttributes is the payload every node carries: its text and when it was
// recorded. Search's time window and recency decay read the timestamp.
type NodeAttributes struct {
	Value     string
	Timestamp time.Time
}

// Node is anything the graph stores under a key: the entities (facts, topics,
// named entities) and the relationships between them.
//
// A node's key is the hash of its type tag followed by its material. The tags
// are "fact:", "topic:", "entity:", "mentions:" and "isabout:". So identity is
// (type, value), never value alone: a fact and a topic that read the same are
// two different nodes. Without the tag, the query
// remember 'billing' topic:billing would hash both to one key. The topic node
// would never be stored, because Set finds the fact already there. Its
// IsAbout edge would point from the fact back to itself. A new node type
// takes a tag that no other tag prefixes. The five above differ in their
// first byte.
type Node[K comparable] interface {
	hash.Hashable[K, string]

	Key() K
	GetValue() string
	GetTimestamp() time.Time
	GetAttributes() *NodeAttributes
}

// Entity is a vertex of the graph: a [Fact], [Topic] or [NamedEntity], the
// endpoints a [Relationship] connects. Vertices are what [Graph.Order] counts
// and what traversals visit.
type Entity[K comparable] interface {
	Node[K]

	GetValue() string
}

// Relationship is an edge, stored as a node under its own key. Source is the
// fact the edge leaves, and Target the topic or named entity it points to.
// The graph's adjacency maps record the edge's key between their two keys.
type Relationship[K comparable] interface {
	Node[K]

	Source() *Entity[K]
	Target() *Entity[K]
}
