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

	"github.com/RonsenbergVI/fraise/internal/hash"
)

type NodeAttributes struct {
	Value     string
	Timestamp time.Time
}

// Node is anything the graph stores under a key: the entities (facts, topics,
// named entities) and the relationships between them.
//
// A node's key is the hash of its own type tag followed by its material —
// "fact:", "topic:", "entity:", "mentions:", "isabout:" — so identity is
// (type, value), never value alone: a fact and a topic that read the same are
// two different nodes. Without the tag, `remember 'billing' topic:billing`
// hashes both to one key, and then the topic node is never stored (Set finds
// the fact already there) while its IsAbout edge points from the fact back to
// itself. A new participant takes a tag that no other one prefixes; the five
// above differ in their first byte.
type Node[K comparable] interface {
	hash.Hashable[K, string]

	Key() K
	GetValue() string
	GetTimestamp() time.Time
	GetAttributes() *NodeAttributes
}

type Entity[K comparable] interface {
	Node[K]

	GetValue() string
}

type Relationship[K comparable] interface {
	Node[K]

	Source() *Entity[K]
	Target() *Entity[K]
}
