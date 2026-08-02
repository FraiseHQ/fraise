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
	"fmt"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/graph"
)

// the db hols the logic of translating low level calls to the memory Graphs
// from and to the transaction object (that the server directly serialises to the client)
type DB[K ~uint64, P float32 | float64] struct {
	Config *config.ConfigSet
	Graphs []graph.Graph[K, P]

	stats *Stats
}

type Stats struct {
	Memory int
}

func NewDB[K ~uint64, P float32 | float64](cfg *config.ConfigSet) (*DB[K, P], error) {
	d := &DB[K, P]{
		Config: cfg,
		Graphs: make([]graph.Graph[K, P], config.DefaultNumGraph),
	}
	return d, nil
}

func (d *DB[K, P]) Start() error {
	for i := range d.Graphs {
		g := graph.NewGraph[K, P](d.Config)

		// The search algorithms are injected from configuration; unknown
		// names keep the graph's built-in defaults.
		if d.Config.DB.SearchAlgorithm.Name == "bfs" {
			g.SetTraversal(graph.NewBFSTraversal[K, P](graph.Both))
		}
		if d.Config.DB.RankingAlgorithm.Name == "pagerank" {
			g.SetRanking(graph.NewPageRank[K, P](
				P(d.Config.DB.RankingAlgorithm.PageRankDamping),
				d.Config.DB.RankingAlgorithm.PageRankMaxIter,
				P(d.Config.DB.RankingAlgorithm.PageRankTol),
			))
		}

		d.Graphs[i] = g
	}
	return nil
}

func (d *DB[K, P]) Stop() error {
	// Reinitialise graphs
	d.Graphs = make([]graph.Graph[K, P], config.DefaultNumGraph)
	return nil
}

func (d *DB[K, P]) Stats() Stats {
	return *d.stats
}

// NumGraphs reports how many graphs the store holds. Valid selectors are in
// [0, NumGraphs).
func (d *DB[K, P]) NumGraphs() int {
	return len(d.Graphs)
}

func (d *DB[K, P]) Select(index uint8) (graph.Graph[K, P], error) {
	if int(index) >= len(d.Graphs) {
		return nil, fmt.Errorf("%w: index %d for %d graphs", ErrIndexOutOfBounds, index, len(d.Graphs))
	}
	return d.Graphs[index], nil
}
