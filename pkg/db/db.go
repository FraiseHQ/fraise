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
	"sort"
	"sync"
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
)

// the db hols the logic of translating low level calls to the memory Graphs
// from and to the transaction object (that the server directly serialises to the client)
type DB[K comparable, P float32 | float64] struct {
	mu sync.RWMutex

	Config *config.ConfigSet
	Graphs []*graph.Graph[K, P]

	currentGraph *graph.Graph[K, P]
	stats        Stats
}

type Stats struct {
	Memory int
}

func NewDB[K comparable, P float32 | float64](config *config.ConfigSet) (*DB[K, P], error) {
	return nil, nil
}

func (d *DB[K, P]) Start() error {
	return nil
}

func (d *DB[K, P]) Stop() error {
	return nil
}

func (d *DB[K, P]) Stats() error {
	return nil
}

func (d *DB[K, P]) CurrentGraph() *graph.Graph[K, P] {
	return d.currentGraph
}

// selects with graph to use
func (d *DB[K, P]) Select(index int) error {
	return nil
}

func (d *DB[K, P]) Get(key K) *graph.Node[K] {
	entity := (*(d.currentGraph)).Get(key)
	if entity != nil {
		return nil
	}
	return entity
}

func (d *DB[K, P]) Set(node *graph.Node[K]) error {
	// err := d.currentGraph.Set(key, value)
	// if err != nil {
	// 	return err,
	// }
	// return errors
	return nil
}

func (d *DB[K, P]) Put(key K, node *graph.Node[K]) error {

	return nil
}

func (d *DB[K, P]) Search(keywords []string, vector containers.Vector[P], topics []string, entities []string, depth int, since time.Time, until time.Time, top int) ([]*graph.Node[K], []P) {
	// A. Search starts with gathering seeds for the graph search.
	// Seeds are found from
	// 1. Vector search (top K - default = 10)
	// 2. Matching keywords
	seeds, scores := d.gatherSeeds(keywords, vector)

	// B. Walking the graph from all searchs and uinioning the found facts
	neighbors, scores := d.findNeighbours(seeds, topics, entities, depth)

	// C. Time filtered (since or until)

	filtered, scores := d.timeFilter(neighbors, since, until)

	// D. Truncate search results

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})
	limit := top
	if limit > len(filtered) {
		limit = len(filtered)
	}

	return filtered[:limit]
}

func (d *DB[K, P]) gatherSeeds(keywords []string, vector containers.Vector[P]) ([]*graph.Node[K], []P) {
	return nil, []P{}
}

func (d *DB[K, P]) findNeighbours(seeds []*graph.Node[K], topics []string, entities []string, depth int) ([]*graph.Node[K], []P) {
	return nil, []P{}
}

func (d *DB[K, P]) timeFilter(nodes []*graph.Node[K], since time.Time, until time.Time) ([]*graph.Node[K], []P) {
	return nil, []P{}
}
