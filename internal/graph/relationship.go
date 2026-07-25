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

// Fact mentions NamedEntity relationship
type Mentions[K comparable] struct {
	Fact        *Fact[K]
	NamedEntity *NamedEntity[K]
	NodeAttributes

	Hasher hash.Hasher[K, string]
}

func (m Mentions[K]) Key() K {
	return m.Hash(m.Hasher)
}

func (m Mentions[K]) GetAttributes() *NodeAttributes {
	return &m.NodeAttributes
}

func (m Mentions[K]) GetTimestamp() time.Time {
	return m.NodeAttributes.Timestamp
}

func (m Mentions[K]) GetValue() string {
	return m.NodeAttributes.Value
}

func (m Mentions[K]) Source() *Entity[K] {
	var e Entity[K] = m.Fact
	return &e
}

func (m Mentions[K]) Target() *Entity[K] {
	var e Entity[K] = m.NamedEntity
	return &e
}

func (m Mentions[K]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash(m.NodeAttributes.Value)
}

// Fact is about Topic relationship
type IsAbout[K comparable] struct {
	Fact  *Fact[K]
	Topic *Topic[K]
	NodeAttributes

	Hasher hash.Hasher[K, string]
}

func (a IsAbout[K]) Key() K {
	return a.Hash(a.Hasher)
}

func (a IsAbout[K]) GetAttributes() *NodeAttributes {
	return &a.NodeAttributes
}

func (a IsAbout[K]) GetTimestamp() time.Time {
	return a.NodeAttributes.Timestamp
}

func (a IsAbout[K]) GetValue() string {
	return a.NodeAttributes.Value
}

func (a IsAbout[K]) Source() *Entity[K] {
	var e Entity[K] = a.Fact
	return &e
}

func (a IsAbout[K]) Target() *Entity[K] {
	var e Entity[K] = a.Topic
	return &e
}

func (a IsAbout[K]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash(a.NodeAttributes.Value)
}
