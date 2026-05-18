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

package db

import (
	"sync"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/graph"
)

// the db hols the logic of translating low level calls to the memory Graphs
// from and to the transaction object (that the server directly serialises to the client)
type DB[K comparable, V any, P float32 | float64] struct {
	mu sync.RWMutex

	buf    []byte
	Config config.ConfigSet
	Graphs []*graph.Graph[K, P]

	currentGraph *graph.Graph[K, P]
	stats        Stats
}

type Stats struct {
}

func (d *DB[K, V, P]) Start() error {
	return nil
}

func (d *DB[K, V, P]) Stop() error {
	return nil
}

func (d *DB[K, V, P]) Stats() error {
	return nil
}

// selects with graph to use
func (d *DB[K, V, P]) Select(index int) error {
	return nil
}

func (d *DB[K, V, P]) Get(key K) K {
	entity := (*(d.currentGraph)).Get(key)
	return (*entity).GetID()
}

func (d *DB[K, V, P]) Set(key K, value V) {
	d.currentGraph.Set(key, value)
}

func (d *DB[K, V, P]) Put(key K, value V) {
	d.currentGraph.Put(key, value)
}

func (d *DB[K, V, P]) Search(keywords []string, vector []P, topics []string, entities []string) {
	results := d.currentGraph.Search(keywords, vector, topics, entities)

}
