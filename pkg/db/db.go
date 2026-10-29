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

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// DB holds the store's graphs, addressed by selector. Start builds each graph
// with the search algorithms the configuration names. Streams then run against
// the graph Select returns.
type DB[K ~uint64, P float32 | float64] struct {
	Config *config.ConfigSet
	Graphs []graph.Graph[K, P]
}

// Stats is a point-in-time snapshot of the store, with one entry per graph in
// selector order. It is computed on demand from the live graphs, never cached.
// The server's stats endpoint serialises it as is.
type Stats struct {
	Graphs []GraphStats `json:"graphs"`
}

// GraphStats is one graph's snapshot, tagged with its selector.
type GraphStats struct {
	ID int `json:"id"`
	graph.GraphStats
}

// numGraphs returns the configured graph count. A hand-built config that never
// went through Parse gets the default.
func numGraphs(cfg *config.ConfigSet) int {
	if cfg.DB.NumGraphs <= 0 {
		return config.DefaultNumGraph
	}
	return cfg.DB.NumGraphs
}

// NewDB returns a store with one empty slot per configured graph. [DB.Start]
// builds the graphs, so the store can be wired into the scheduler before any
// graph memory is allocated.
func NewDB[K ~uint64, P float32 | float64](cfg *config.ConfigSet) (*DB[K, P], error) {
	d := &DB[K, P]{
		Config: cfg,
		Graphs: make([]graph.Graph[K, P], numGraphs(cfg)),
	}
	return d, nil
}

// Start builds a fresh graph in every slot, with the traversal and scorer the
// configuration names. The store is in-memory, so a Start after [DB.Stop]
// begins from empty graphs.
//
// Start fails with [ErrRelevanceModelMismatch] when a graph's text index does
// not rank with the configured relevance model. Nothing downstream would
// notice, because a wrong model still answers every query, only worse.
func (d *DB[K, P]) Start() error {
	want := d.Config.DB.RelevanceModel.Name
	for i := range d.Graphs {
		g := graph.NewGraph[K, P](d.Config)
		if got := g.Stats().Relevance; got != want {
			return fmt.Errorf("%w: graph %d ranks with %q, configured %q", ErrRelevanceModelMismatch, i, got, want)
		}

		// Startup rejects any algorithm name outside config's accepted lists.
		// So a stage left at the graph's built-in default (no traversal, the
		// excess scorer) is what was configured, not an unrecognised name.
		switch d.Config.DB.SearchAlgorithm.Name {
		case config.SearchExcess:
			g.SetTraversal(graph.NewExcessTraversal[K, P]())
		case config.SearchBFS:
			g.SetTraversal(graph.NewBFSTraversal[K, P](graph.Both))
		}
		switch d.Config.DB.ScoringAlgorithm.Name {
		case config.ScoringInformation:
			g.SetScorer(scoring.NewInformationScorer[K, P]())
		case config.ScoringRRF:
			g.SetScorer(scoring.NewRRFScorer[K, P](scoring.DefaultRRFK))
		}

		d.Graphs[i] = g
	}
	logger.Info("Text index relevance model resolved", "relevance", want)
	return nil
}

// Stop drops every graph and their contents, leaving empty slots for the next
// [DB.Start].
func (d *DB[K, P]) Stop() error {
	d.Graphs = make([]graph.Graph[K, P], numGraphs(d.Config))
	return nil
}

// Stats snapshots every graph in selector order. An empty slot, before Start
// or after Stop, gives a zero-valued entry. So Stats is safe to call at any
// point in the store's lifecycle. Each live graph is read-locked for its
// snapshot, so a snapshot never sees a write in progress.
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

// Select returns the graph at selector index. When index is outside
// [0, NumGraphs), it returns an error wrapping [ErrIndexOutOfBounds], so a bad
// selector is an error, not a panic.
func (d *DB[K, P]) Select(index uint8) (graph.Graph[K, P], error) {
	if int(index) >= len(d.Graphs) {
		return nil, fmt.Errorf("%w: index %d for %d graphs", ErrIndexOutOfBounds, index, len(d.Graphs))
	}
	return d.Graphs[index], nil
}
