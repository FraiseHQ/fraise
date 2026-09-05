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

package query

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/internal/index"
)

// fakeGraph is a controllable graph.Graph used to observe how Stream drives the
// graph: which lock Acquire/Release take, whether the write path writes and the
// read path searches, and what Search returns. copied/merged record calls to
// Copy/MergeFrom, which Commit must never make — the in-place tests pin that a
// staging copy (O(graph) per write) is not reintroduced. Only the methods
// Stream touches carry behaviour; the rest are inert stubs present to satisfy
// the interface.
type fakeGraph struct {
	locks, unlocks   int
	rlocks, runlocks int
	copied, merged   bool
	sets             int
	puts             int
	searchCalled     bool

	searchNodes      []*graph.Node[string]
	searchScores     []float32
	searchContribs   [][]scoring.Contribution[string, float32]
	searchBackground float32
}

var _ graph.Graph[string, float32] = (*fakeGraph)(nil)

func (g *fakeGraph) Lock()    { g.locks++ }
func (g *fakeGraph) Unlock()  { g.unlocks++ }
func (g *fakeGraph) RLock()   { g.rlocks++ }
func (g *fakeGraph) RUnlock() { g.runlocks++ }

func (g *fakeGraph) Copy() graph.Graph[string, float32]            { g.copied = true; return g }
func (g *fakeGraph) MergeFrom(in graph.Graph[string, float32])     { g.merged = true }
func (g *fakeGraph) Set(node graph.Node[string]) error             { g.sets++; return nil }
func (g *fakeGraph) Put(key string, node graph.Node[string]) error { g.puts++; return nil }

func (g *fakeGraph) Search(keywords []string, vector containers.Vector[string, float32], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*graph.Node[string], []float32, [][]scoring.Contribution[string, float32], float32) {
	g.searchCalled = true
	return g.searchNodes, g.searchScores, g.searchContribs, g.searchBackground
}

// --- inert stubs (unused by Stream) ----------------------------------------

// GetHasher returns a real (fake) hasher rather than nil: the write path
// derives the fact's key (fact.Key() -> Hash) before storing it.
func (g *fakeGraph) GetHasher() hash.Hasher[string, string]             { return &fakeHasher{} }
func (g *fakeGraph) Get(key string) graph.Node[string]                  { return nil }
func (g *fakeGraph) Delete(node graph.Node[string]) error               { return nil }
func (g *fakeGraph) GetVectorIndex() index.VectorIndex[string, float32] { return nil }
func (g *fakeGraph) GetTextIndex() index.TextIndex[string, float32]     { return nil }
func (g *fakeGraph) Nodes() map[string]graph.Node[string]               { return nil }
func (g *fakeGraph) AdjacencyMap() map[string]map[string]string         { return nil }
func (g *fakeGraph) PredecessorMap() map[string]map[string]string       { return nil }
func (g *fakeGraph) Neighbours(key string) []string                     { return nil }
func (g *fakeGraph) Order() int                                         { return 0 }
func (g *fakeGraph) Size() int                                          { return 0 }
func (g *fakeGraph) Stats() graph.GraphStats                            { return graph.GraphStats{} }

// --- helpers ---------------------------------------------------------------

func newStream(q Query[string, float32]) *Stream[string, float32] {
	return &Stream[string, float32]{Query: q, done: make(chan struct{})}
}

// readQuery builds a Recall. Its nil time bounds resolve to the zero time, so
// the read path in Commit can call Since/Until safely. It returns a *Recall
// because SetGraphID's pointer receiver means only *Recall satisfies Query (and
// Commit's read path asserts *Recall).
func readQuery() *Recall[string, float32] {
	return &Recall[string, float32]{Keywords: []string{"x"}}
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// --- Commit (read path) ----------------------------------------------------

func TestStreamCommitReadBuildsResult(t *testing.T) {
	g := &fakeGraph{
		searchNodes:  []*graph.Node[string]{nil, nil, nil},
		searchScores: []float32{0.9, 0.8, 0.7},
	}
	s := newStream(readQuery())

	if err := s.Commit(g); err != nil {
		t.Fatalf("Commit() err = %v", err)
	}

	if !g.searchCalled {
		t.Error("Commit did not call Search on a read query")
	}
	if g.copied || g.merged {
		t.Error("Commit copied or merged the graph on a read query, want in-place")
	}
	if s.Result == nil {
		t.Fatal("Commit left Result nil")
	}
	if s.Result.Count != 3 || len(s.Result.Hits) != 3 {
		t.Fatalf("Result count=%d hits=%d, want 3/3", s.Result.Count, len(s.Result.Hits))
	}
	for i, wantScore := range []float32{0.9, 0.8, 0.7} {
		if s.Result.Hits[i].Score != wantScore {
			t.Errorf("Hits[%d].Score = %v, want %v", i, s.Result.Hits[i].Score, wantScore)
		}
	}
}

// TestStreamCommitExplainAttachesContributions pins the explain switch on the
// read path: the same commit, run with Explain set, copies each hit's
// contribution records onto the hit, and without it the hits stay bare — nil
// is what keeps contributions out of the ordinary response. The flag lives on
// the stream rather than the query because the plan cache shares query
// objects across requests; this test drives it exactly where the handler
// sets it.
func TestStreamCommitExplainAttachesContributions(t *testing.T) {
	contributions := [][]scoring.Contribution[string, float32]{
		{{Src: scoring.SrcText, Score: 1, Rank: 0, Count: 1}},
		{{Src: scoring.SrcVector, Score: 0.5, Rank: 1, Count: 1}, {Src: scoring.SrcGraph, Score: 2, Via: "vela-key", Degree: 3, Count: 2}},
		{{Src: scoring.SrcAnchor, Score: 1, Via: "harbour-key", Degree: 4, Count: 1}},
	}
	// The wire form: sources by name, the graph and anchor entries' anchor
	// keys resolved via Get — the fake stores no nodes, so via falls back to
	// empty, which is the contract for a vanished anchor.
	want := [][]HitContribution[float32]{
		{{Source: "text", Score: 1, Rank: 0, Count: 1}},
		{{Source: "vector", Score: 0.5, Rank: 1, Count: 1}, {Source: "graph", Score: 2, Degree: 3, Count: 2}},
		{{Source: "anchor", Score: 1, Degree: 4, Count: 1}},
	}

	for _, explain := range []bool{true, false} {
		t.Run(fmt.Sprintf("explain=%v", explain), func(t *testing.T) {
			g := &fakeGraph{
				searchNodes:      []*graph.Node[string]{nil, nil, nil},
				searchScores:     []float32{0.9, 0.8, 0.7},
				searchContribs:   contributions,
				searchBackground: 0.25,
			}
			s := newStream(readQuery())
			s.Explain = explain

			if err := s.Commit(g); err != nil {
				t.Fatalf("Commit() err = %v", err)
			}

			if explain && s.Result.Background != 0.25 {
				t.Errorf("Result.Background = %v, want the search's 0.25", s.Result.Background)
			}
			if !explain && s.Result.Background != 0 {
				t.Errorf("Result.Background = %v without explain, want 0 (omitted on the wire)", s.Result.Background)
			}
			for i, hit := range s.Result.Hits {
				if explain {
					if !reflect.DeepEqual(hit.Contributions, want[i]) {
						t.Errorf("Hits[%d].Contributions = %+v, want the wire form %+v", i, hit.Contributions, want[i])
					}
				} else if hit.Contributions != nil {
					t.Errorf("Hits[%d].Contributions = %+v without explain, want nil", i, hit.Contributions)
				}
			}
		})
	}
}

// --- Commit (write path) ---------------------------------------------------

// TestStreamCommitWriteInPlace is the O(graph)-per-write regression pin: a
// write commit must mutate the given graph directly — never Copy it into a
// staging graph or MergeFrom one back. Reintroducing either makes every
// single-fact write cost O(total graph size) under the exclusive lock.
func TestStreamCommitWriteInPlace(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(&Remember[string, float32]{Value: "alice"})

	if err := s.Commit(g); err != nil {
		t.Fatalf("Commit() err = %v", err)
	}

	if g.puts == 0 {
		t.Error("Commit did not upsert the fact on a write query")
	}
	if g.copied {
		t.Error("Commit copied the graph on a write query, want in-place")
	}
	if g.merged {
		t.Error("Commit merged a staging graph, want in-place")
	}
	if g.searchCalled {
		t.Error("Commit called Search on a write query")
	}
	if s.Result == nil {
		t.Fatal("Commit left Result nil on a write query")
	}
}

// TestStreamCommitReassertRefreshesRecency pins the temporal "touch"
// semantics: re-remembering an identical fact replaces the stored node with a
// fresh timestamp (no duplicate node), so recency decay restarts and a
// since:-window covering only the re-assertion finds the fact. Regression for
// the silent first-write-wins behavior, where an agent reinforcing a memory
// left it decaying from its original write.
func TestStreamCommitReassertRefreshesRecency(t *testing.T) {
	g := graph.NewGraph[uint64, float32](config.New())
	remember := func() *Remember[uint64, float32] {
		return &Remember[uint64, float32]{Value: "the deploy key lives in vault"}
	}

	if err := NewStream[uint64, float32](remember()).Commit(g); err != nil {
		t.Fatalf("first Commit = %v, want nil", err)
	}
	key := graph.Fact[uint64]{
		NodeAttributes: graph.NodeAttributes{Value: "the deploy key lives in vault"},
		Hasher:         g.GetHasher(),
	}.Key()
	ts1 := g.Get(key).GetTimestamp()
	nodesBefore := g.Stats().Nodes

	// A window opening strictly after the first write: only a refreshed
	// timestamp can land inside it.
	windowStart := ts1.Add(time.Nanosecond)

	if err := NewStream[uint64, float32](remember()).Commit(g); err != nil {
		t.Fatalf("second Commit = %v, want nil", err)
	}

	ts2 := g.Get(key).GetTimestamp()
	if !ts2.After(ts1) {
		t.Errorf("re-assert timestamp = %v, want after the original %v (touch)", ts2, ts1)
	}
	if got := g.Stats().Nodes; got != nodesBefore {
		t.Errorf("re-assert changed node count %d -> %d, want an in-place replace", nodesBefore, got)
	}

	// The report's repro: recall with since covering only the re-assertion.
	nodes, _, _, _ := g.Search([]string{"deploy"}, containers.Vector[uint64, float32]{}, nil, nil, 0, 10, windowStart, time.Time{})
	if len(nodes) != 1 {
		t.Errorf("Search(since=post-first-write) returned %d hits, want the re-asserted fact", len(nodes))
	}
}

// TestStreamCommitVectorMismatchLeavesGraphClean pins the failure ordering of
// the in-place write: the vector insert runs before any graph mutation, so the
// one realistic commit failure — a vector-dimension mismatch — rejects the
// write with the graph untouched (no fact node, no index entry).
func TestStreamCommitVectorMismatchLeavesGraphClean(t *testing.T) {
	g := graph.NewGraph[uint64, float32](config.New())

	// First write fixes the vector index dimension at 3.
	first := &Remember[uint64, float32]{
		Value:  "first fact",
		Vector: containers.NewVector[uint64]([]float32{1, 2, 3}),
	}
	if err := NewStream[uint64, float32](first).Commit(g); err != nil {
		t.Fatalf("first Commit = %v, want nil", err)
	}
	before := g.Stats()

	// Second write carries a dim-4 vector: must fail and change nothing.
	second := &Remember[uint64, float32]{
		Value:  "second fact",
		Vector: containers.NewVector[uint64]([]float32{1, 2, 3, 4}),
	}
	if err := NewStream[uint64, float32](second).Commit(g); err == nil {
		t.Fatal("Commit with mismatched vector dimension = nil error, want error")
	}

	after := g.Stats()
	if after != before {
		t.Errorf("failed commit mutated the graph: before %+v, after %+v", before, after)
	}
}

// --- Acquire / Release -----------------------------------------------------

func TestStreamAcquireReleaseRead(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(readQuery())

	s.Acquire(g)
	if g.rlocks != 1 || g.locks != 0 {
		t.Errorf("read Acquire: rlocks=%d locks=%d, want rlocks=1 locks=0", g.rlocks, g.locks)
	}
	s.Release(g)
	if g.runlocks != 1 {
		t.Errorf("read Release: runlocks=%d, want 1", g.runlocks)
	}
}

func TestStreamAcquireReleaseWrite(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(&Remember[string, float32]{})

	s.Acquire(g)
	if g.locks != 1 || g.rlocks != 0 {
		t.Errorf("write Acquire: locks=%d rlocks=%d, want locks=1 rlocks=0", g.locks, g.rlocks)
	}
	s.Release(g)
	if g.unlocks != 1 {
		t.Errorf("write Release: unlocks=%d, want 1", g.unlocks)
	}
}

// --- GraphID / Done / Finish -----------------------------------------------

func TestStreamGraphID(t *testing.T) {
	r := &Remember[string, float32]{}
	r.SetGraphID(5)
	s := newStream(r)
	if got := s.GraphID(); got != 5 {
		t.Errorf("GraphID() = %d, want 5", got)
	}
}

func TestStreamFinishIsIdempotent(t *testing.T) {
	s := newStream(readQuery())
	s.Finish()
	s.Finish() // second call must not panic or double-close (sync.Once)
	if !isClosed(s.Done()) {
		t.Error("Done channel not closed after Finish")
	}
}

// TestCommitStoresAnchorNodesForFilteredRecall drives the exact production
// write path (an in-place Commit against the live graph) and checks the
// written fact is recallable through its topic:/entity: anchors. Regression
// test for anchored recalls returning nothing: Commit created the
// Mentions/IsAbout edges but never stored the NamedEntity/Topic nodes
// themselves, so the filter could not resolve the anchor values and dropped
// every fact.
func TestCommitStoresAnchorNodesForFilteredRecall(t *testing.T) {
	g := graph.NewGraph[uint64, float32](config.New())

	remember := &Remember[uint64, float32]{
		Value:    "alice moved to paris",
		Topics:   []string{"travel"},
		Entities: []string{"alice"},
	}
	s := NewStream[uint64, float32](remember)

	if err := s.Commit(g); err != nil {
		t.Fatalf("Commit = %v, want nil", err)
	}

	cases := []struct {
		name     string
		topics   []string
		entities []string
	}{
		{"no filter", nil, nil},
		{"topic filter", []string{"travel"}, nil},
		{"entity filter", nil, []string{"alice"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes, _, _, _ := g.Search([]string{"paris"}, containers.Vector[uint64, float32]{}, tc.topics, tc.entities, 2, 10, time.Time{}, time.Time{})
			got := make([]string, 0, len(nodes))
			for _, n := range nodes {
				got = append(got, (*n).GetValue())
			}
			if len(nodes) != 1 || got[0] != "alice moved to paris" {
				t.Errorf("Search(paris, topics=%v entities=%v) = %v, want [alice moved to paris]",
					tc.topics, tc.entities, got)
			}
		})
	}
}

// BenchmarkRememberCommit measures a single-fact write commit against graphs
// of different sizes. The in-place write is O(fact + incremental index
// updates), so ns/op must stay flat as the pre-populated graph grows — the
// old staging path (Copy + MergeFrom) was O(total graph size) per write and
// showed up here as ns/op scaling with the size subtest.
func BenchmarkRememberCommit(b *testing.B) {
	for _, size := range []int{100, 10_000} {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			g := graph.NewGraph[uint64, float32](config.New())
			for i := 0; i < size; i++ {
				pre := &Remember[uint64, float32]{
					Value:  fmt.Sprintf("pre-existing fact number %d", i),
					Topics: []string{fmt.Sprintf("topic%d", i%13)},
				}
				if err := NewStream[uint64, float32](pre).Commit(g); err != nil {
					b.Fatalf("prepopulate Commit = %v", err)
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := &Remember[uint64, float32]{
					Value:  fmt.Sprintf("bench fact %d", i),
					Topics: []string{"bench"},
				}
				if err := NewStream[uint64, float32](w).Commit(g); err != nil {
					b.Fatalf("Commit = %v", err)
				}
			}
		})
	}
}

// TestCommitSeedsFromAnchorsStoredByCommit drives the production write path
// and then the anchor-only read it makes possible: a Recall naming a topic
// and no term reaches Search with nil keywords and an empty vector, and the
// topic node Commit stored is the one the seeding resolves — one identity on
// both sides of the store, which is what this pins (seeding that resolved
// anchors under a different key would return nothing, exactly as the recall
// did before anchors seeded). Every fact filed under the topic comes back and
// nothing else, each scored from its unit anchor mass, and in explain mode
// each breakdown names the topic it was found under. Naming the entity as
// well doubles the mass of every fact filed under both, and a term beside the
// anchor seeds from the text index with the anchor as a filter.
func TestCommitSeedsFromAnchorsStoredByCommit(t *testing.T) {
	g := graph.NewGraph[uint64, float32](config.New())
	facts := []string{"the deploy runs at noon", "the deploy key lives in vault", "the deploy log is archived weekly"}
	for _, value := range facts {
		w := &Remember[uint64, float32]{Value: value, Topics: []string{"deploys"}, Entities: []string{"ops"}}
		if err := NewStream[uint64, float32](w).Commit(g); err != nil {
			t.Fatalf("Commit(%q) = %v, want nil", value, err)
		}
	}
	bystander := &Remember[uint64, float32]{Value: "the invoice is due friday", Topics: []string{"billing"}}
	if err := NewStream[uint64, float32](bystander).Commit(g); err != nil {
		t.Fatalf("Commit(bystander) = %v, want nil", err)
	}

	read := func(t *testing.T, q *Recall[uint64, float32]) *QueryResult[uint64, float32] {
		t.Helper()
		s := NewStream[uint64, float32](q)
		s.Explain = true
		if err := s.Commit(g); err != nil {
			t.Fatalf("Commit(recall) = %v, want nil", err)
		}
		return s.Result
	}
	hitValues := func(r *QueryResult[uint64, float32]) []string {
		out := make([]string, len(r.Hits))
		for i, h := range r.Hits {
			out[i] = (*h.Node).GetValue()
		}
		return out
	}
	sorted := func(values []string) []string {
		out := append([]string(nil), values...)
		sort.Strings(out)
		return out
	}

	t.Run("a topic alone lists its members", func(t *testing.T) {
		r := read(t, &Recall[uint64, float32]{Topics: []string{"deploys"}, Parameters: QueryParameters[uint64]{Top: 10}})
		if got, want := sorted(hitValues(r)), sorted(facts); !reflect.DeepEqual(got, want) {
			t.Fatalf("Search(topic=deploys) = %v, want every fact filed under the topic %v and no other", got, want)
		}
		for i, hit := range r.Hits {
			if hit.Score <= 0 {
				t.Errorf("Hits[%d].Score = %v, want the decayed unit anchor mass, above zero", i, hit.Score)
			}
			if i > 0 && hit.Score > r.Hits[i-1].Score {
				t.Errorf("Hits[%d].Score = %v ranks above Hits[%d].Score = %v; want best first", i, hit.Score, i-1, r.Hits[i-1].Score)
			}
			want := []HitContribution[float32]{{Source: "anchor", Score: 1, Via: "deploys", Degree: 3, Count: 1}}
			if !reflect.DeepEqual(hit.Contributions, want) {
				t.Errorf("Hits[%d].Contributions = %+v, want the one anchor sighting resolved to its topic %+v", i, hit.Contributions, want)
			}
		}
		if r.Background != 0 {
			t.Errorf("Background = %v, want 0: anchor seeding observes no anchor", r.Background)
		}
	})

	t.Run("a topic and an entity double the mass of a fact under both", func(t *testing.T) {
		single := read(t, &Recall[uint64, float32]{Topics: []string{"deploys"}, Parameters: QueryParameters[uint64]{Top: 10}})
		both := read(t, &Recall[uint64, float32]{Topics: []string{"deploys"}, Entities: []string{"ops"}, Parameters: QueryParameters[uint64]{Top: 10}})
		if got, want := sorted(hitValues(both)), sorted(facts); !reflect.DeepEqual(got, want) {
			t.Fatalf("Search(topic=deploys, entity=ops) = %v, want the union %v, each fact once", got, want)
		}
		singleScore := make(map[string]float32, len(single.Hits))
		for _, hit := range single.Hits {
			singleScore[(*hit.Node).GetValue()] = hit.Score
		}
		for i, hit := range both.Hits {
			value := (*hit.Node).GetValue()
			if hit.Score <= singleScore[value] {
				t.Errorf("%q scores %v under both anchors, not above its %v under one", value, hit.Score, singleScore[value])
			}
			want := []HitContribution[float32]{
				{Source: "anchor", Score: 1, Via: "deploys", Degree: 3, Count: 1},
				{Source: "anchor", Score: 1, Via: "ops", Degree: 3, Count: 1},
			}
			if !reflect.DeepEqual(hit.Contributions, want) {
				t.Errorf("Hits[%d].Contributions = %+v, want one sighting per named anchor %+v", i, hit.Contributions, want)
			}
		}
	})

	t.Run("a term beside the anchor searches", func(t *testing.T) {
		r := read(t, &Recall[uint64, float32]{Keywords: []string{"vault"}, Topics: []string{"deploys"}, Parameters: QueryParameters[uint64]{Top: 10}})
		if got, want := hitValues(r), []string{"the deploy key lives in vault"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("Search(vault, topic=deploys) = %v, want the one text match %v: the anchor filters, it does not list", got, want)
		}
		for _, c := range r.Hits[0].Contributions {
			if c.Source == "anchor" {
				t.Errorf("a text-seeded search recorded an anchor sighting %+v; anchors seed only on their own", c)
			}
		}
	})
}
