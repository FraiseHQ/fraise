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
	"sync"

	"github.com/RonsenbergVI/fraise/internal/index"
)

type GraphStats struct {
}

// Hashing fuinction that takes as input an entity (or vertex)
// and returns a hash.
type Hash[K comparable] func(Entity[K]) K

type Graph[K comparable, P float32 | float64] interface {
	Stats() GraphStats
	Locked() bool

	GetVectorIndex() index.Index[K, float32, P]
	GetTextIndex() index.Index[K, string, P]

	AddEntity(e Entity[K], options ...func(*EntityProperties)) error

	AddRelationship(relationship Relationship[K], options ...func(*RelationshipProperties)) error

	GetNeighbours(e []*Entity[K], keywords []string, depth int, top int) []*Entity[K]

	Index(fact Fact[K]) error
}

type InMemoryGraph[K comparable, P float32 | float64] struct {
	Entities      map[K]*Entity[K]
	Relationships map[K]*Relationship[K]

	Vector *index.Index[K, float32, P]
	Text   *index.Index[K, string, P]

	mu sync.RWMutex
}

func (g *InMemoryGraph[K, P]) GetVectorIndex() *index.Index[K, float32, P] {
	return nil
}

func (g *InMemoryGraph[K, P]) GetTextIndex() *index.Index[K, string, P] {
	return nil
}

func (g *InMemoryGraph[K, P]) Index(fact Fact[K]) error {
	return nil
}

func (g *InMemoryGraph[K, P]) Set(key K, value map[string]any) error {
	return nil
}

func (g *InMemoryGraph[K, P]) Get(key K) *Entity[K] {
	return nil
}

func (g *InMemoryGraph[K, P]) Put(key K, entity Entity[K]) error {
	return nil
}

// find neighbours of an entity
func (g *InMemoryGraph[K, P]) GetNeighbours(e []*Entity[K], keywords []string, depth int, top int) []*Entity[K] {
	return nil
}
