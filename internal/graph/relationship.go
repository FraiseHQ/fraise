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

import "time"

type Relationship[K comparable] interface {
	Source() *Entity[K]
	Target() *Entity[K]
	GetTimestamp() time.Time
	GetProperties() RelationshipProperties
	Hash() uint32
}

type RelationshipAttributes struct {
	Timestamp time.Time
}

type RelationshipProperties struct {
	Attributes map[string]string
}

type Mentions[K comparable] struct {
	Fact        *Fact[K]
	NamedEntity *NamedEntity[K]
	RelationshipProperties
}

type IsAbout[K comparable] struct {
	Fact  *Fact[K]
	Topic *Topic[K]
	RelationshipProperties
}
