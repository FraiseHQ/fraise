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
}

// Stats is a point-in-time snapshot of the store: one entry per graph, in
// selector order. It is computed on demand from the live graphs (never cached)
// and serialised as-is by the server's stats endpoint.
type Stats struct {
	Graphs []GraphStats `json:"graphs"`
}

// GraphStats is one graph's snapshot, tagged with its selector.
type GraphStats struct {
	ID int `json:"id"`
	graph.GraphStats
}

// numGraphs resolves the configured graph count, falling back to the default
// for hand-built configs that never went through Parse/adjust.
func numGraphs(cfg *config.ConfigSet) int {
	if cfg.DB.NumGraphs <= 0 {
		return config.DefaultNumGraph
	}
	return cfg.DB.NumGraphs
}

func NewDB[K ~uint64, P float32 | float64](cfg *config.ConfigSet) (*DB[K, P], error) {
	d := &DB[K, P]{
		Config: cfg,
		Graphs: make([]graph.Graph[K, P], numGraphs(cfg)),
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
	d.Graphs = make([]graph.Graph[K, P], numGraphs(d.Config))
	return nil
}

// Stats snapshots every graph in selector order. Graphs not yet populated
// (before Start, or after Stop) contribute zero-valued entries, so the method
// is safe to call at any point in the store's lifecycle. Each live graph is
// read-locked for its snapshot, consistent with the scheduler holding the
// write lock during merges.
func (d *DB[K, P]) Stats() Stats {
	stats := Stats{Graphs: make([]GraphStats, len(d.Graphs))}
	for i, g := range d.Graphs {
		stats.Graphs[i].ID = i
		if g == nil {
			continue
		}
		g.RLock()
		stats.Graphs[i].GraphStats = g.Stats()
		g.RUnlock()
	}
	return stats
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
