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

// Fact is a vertex holding one remembered statement. Facts are the nodes the
// text and vector indexes hold and the ones Search returns as hits; topics and
// named entities reach them through [IsAbout] and [Mentions] edges. Hasher is
// the graph's hasher, carried so Key can derive the fact's key on its own.
type Fact[K comparable] struct {
	NodeAttributes
	Hasher hash.Hasher[K, string] `json:"-"`
}

// Key implements [Node]: the fact's Hash under its own Hasher, the key the
// graph stores it under.
func (f Fact[K]) Key() K {
	return f.Hash(f.Hasher)
}

// GetValue implements [Node]: it returns the fact's text.
func (f Fact[K]) GetValue() string {
	return f.Value
}

// GetTimestamp implements [Node]: it returns when the fact was recorded.
func (f Fact[K]) GetTimestamp() time.Time {
	return f.Timestamp
}

// GetAttributes implements [Node]: it returns the fact's text and timestamp,
// for readers that handle every node alike.
func (f Fact[K]) GetAttributes() *NodeAttributes {
	return &f.NodeAttributes
}

// Hash keys the fact by its text in the fact namespace, so a topic or entity
// reading the same text is a different node (see Node).
func (f Fact[K]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash("fact:" + f.Value)
}

// NamedEntity is an anchor vertex for a person, place or thing that facts
// mention, linked to each such fact by a [Mentions] edge. Naming it in a
// recall filters and expands the search through those facts. Hasher is the
// graph's hasher, carried so Key can derive the entity's key on its own.
type NamedEntity[K comparable] struct {
	NodeAttributes

	Hasher hash.Hasher[K, string] `json:"-"`
}

// Key implements [Node]: the entity's Hash under its own Hasher, the key the
// graph stores it under.
func (n NamedEntity[K]) Key() K {
	return n.Hash(n.Hasher)
}

// GetValue implements [Node]: it returns the entity's text.
func (n NamedEntity[K]) GetValue() string {
	return n.Value
}

// GetTimestamp implements [Node]: it returns when the entity was recorded.
func (n NamedEntity[K]) GetTimestamp() time.Time {
	return n.Timestamp
}

// GetAttributes implements [Node]: it returns the entity's text and timestamp,
// for readers that handle every node alike.
func (n *NamedEntity[K]) GetAttributes() *NodeAttributes {
	return &n.NodeAttributes
}

// Hash keys the entity by its name in the entity namespace, so a fact whose
// whole text is that name is a different node (see Node).
func (n NamedEntity[K]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash("entity:" + n.Value)
}

// Topic is an anchor vertex for a subject facts are filed under, linked to
// each such fact by an [IsAbout] edge. Naming it in a recall filters and
// expands the search through those facts. Hasher is the graph's hasher,
// carried so Key can derive the topic's key on its own.
type Topic[K comparable] struct {
	ID K
	NodeAttributes

	Hasher hash.Hasher[K, string] `json:"-"`
}

// Key implements [Node]: the topic's Hash under its own Hasher, the key the
// graph stores it under.
func (t Topic[K]) Key() K {
	return t.Hash(t.Hasher)
}

// GetValue implements [Node]: it returns the topic's text.
func (t Topic[K]) GetValue() string {
	return t.Value
}

// GetTimestamp implements [Node]: it returns when the topic was recorded.
func (t Topic[K]) GetTimestamp() time.Time {
	return t.Timestamp
}

// GetAttributes implements [Node]: it returns the topic's text and timestamp,
// for readers that handle every node alike.
func (t *Topic[K]) GetAttributes() *NodeAttributes {
	return &t.NodeAttributes
}

// Hash keys the topic by its name in the topic namespace, so a fact whose whole
// text is that name is a different node (see Node).
func (t Topic[K]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash("topic:" + t.Value)
}
