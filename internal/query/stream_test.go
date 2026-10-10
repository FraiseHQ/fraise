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
	"errors"
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
// read path searches, and what Search returns. The methods that record calls
// or return configured results carry the behaviour; the rest return fixed
// answers.
type fakeGraph struct {
	locks, unlocks   int
	rlocks, runlocks int
	sets             int
	puts             int
	searchCalled     bool

	searchNodes      []*graph.Node[string]
	searchScores     []float32
	searchContribs   [][]scoring.Contribution[string, float32]
	searchBackground float32
	searchErr        error
}

func (g *fakeGraph) Lock()    { g.locks++ }
func (g *fakeGraph) Unlock()  { g.unlocks++ }
func (g *fakeGraph) RLock()   { g.rlocks++ }
func (g *fakeGraph) RUnlock() { g.runlocks++ }

func (g *fakeGraph) Set(node graph.Node[string]) error             { g.sets++; return nil }
func (g *fakeGraph) Put(key string, node graph.Node[string]) error { g.puts++; return nil }

func (g *fakeGraph) Search(keywords []string, vector containers.Vector[string, float32], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) (graph.Result[string, float32], error) {
	g.searchCalled = true
	if g.searchErr != nil {
		return graph.Result[string, float32]{}, g.searchErr
	}
	result := graph.Result[string, float32]{Background: g.searchBackground, Hits: make([]graph.Hit[string, float32], len(g.searchNodes))}
	for i, node := range g.searchNodes {
		result.Hits[i] = graph.Hit[string, float32]{Node: node, Score: g.searchScores[i]}
		if i < len(g.searchContribs) {
			result.Hits[i].Contributions = g.searchContribs[i]
		}
	}
	return result, nil
}

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
func (g *fakeGraph) IsEmpty() bool                                      { return false }

func newStream(q Query[string, float32]) *Stream[string, float32] {
	return &Stream[string, float32]{Query: q, done: make(chan struct{})}
}

// readQuery builds a Recall. Its nil time bounds resolve to the zero time, so
// the read path in Commit can call Since/Until safely. It returns a *Recall
// because Plan and SetGraphID have pointer receivers, so only *Recall
// satisfies Query (and Commit's read path asserts *Recall).
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

// TestStreamCommitReadSurfacesASearchError pins that a read fails the way a
// write does: a search the graph cannot answer as asked (its vector's
// dimension disagrees with the graph's) is returned from Commit, wrapped so
// the cause survives to the HTTP boundary, and no result is built from it.
func TestStreamCommitReadSurfacesASearchError(t *testing.T) {
	g := &fakeGraph{searchErr: fmt.Errorf("%w: index expects 3, got 2", index.ErrInvalidDimension)}
	s := newStream(readQuery())

	err := s.Commit(g)

	if !errors.Is(err, index.ErrInvalidDimension) {
		t.Fatalf("Commit() err = %v, want it to wrap ErrInvalidDimension", err)
	}
	if s.Result != nil {
		t.Errorf("Commit built a result %+v from a failed search, want nil", s.Result)
	}
}

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
// read path: run with Explain set, the same commit attaches each hit's
// contributions in wire form and the background rate; without it the hits
// stay bare, since nil is what keeps contributions out of the ordinary
// response. The flag lives on the stream rather than the query because the
// plan cache shares query objects across requests; this test sets it where
// the handler does.
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

// TestStreamCommitWriteInPlace pins that a write commit upserts into the given
// graph directly and never searches it.
func TestStreamCommitWriteInPlace(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(&Remember[string, float32]{Value: "alice"})

	if err := s.Commit(g); err != nil {
		t.Fatalf("Commit() err = %v", err)
	}

	if g.puts == 0 {
		t.Error("Commit did not upsert the fact on a write query")
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
// since:-window covering only the re-assertion finds the fact. Keeping the
// first write would leave a memory an agent reinforces decaying from its
// original write.
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

	// A recall whose since: window covers only the re-assertion.
	result, _ := g.Search([]string{"deploy"}, containers.Vector[uint64, float32]{}, nil, nil, 0, 10, windowStart, time.Time{})
	if len(result.Hits) != 1 {
		t.Errorf("Search(since=post-first-write) returned %d hits, want the re-asserted fact", len(result.Hits))
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

// TestCommitStoresAnchorNodesForFilteredRecall drives the production write
// path (an in-place Commit against a real graph) and checks the written fact
// is recallable with its topic: and entity: anchors as filters.
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

	topic := g.Get(graph.Topic[uint64]{NodeAttributes: graph.NodeAttributes{Value: "travel"}, Hasher: g.GetHasher()}.Key())
	if got, ok := topic.(*graph.Topic[uint64]); !ok || got.GetValue() != "travel" {
		t.Errorf("Get(topic travel) = %T %v, want the stored *graph.Topic", topic, topic)
	}
	entity := g.Get(graph.NamedEntity[uint64]{NodeAttributes: graph.NodeAttributes{Value: "alice"}, Hasher: g.GetHasher()}.Key())
	if got, ok := entity.(*graph.NamedEntity[uint64]); !ok || got.GetValue() != "alice" {
		t.Errorf("Get(entity alice) = %T %v, want the stored *graph.NamedEntity", entity, entity)
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
			result, _ := g.Search([]string{"paris"}, containers.Vector[uint64, float32]{}, tc.topics, tc.entities, 2, 10, time.Time{}, time.Time{})
			nodes := result.Nodes()
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
// of different sizes. The in-place write touches only what it stores, so
// ns/op must not scale with the size subtest the way a staging copy of the
// graph, O(graph) per write, would.
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

// BenchmarkRecallCommit measures a two-keyword read commit against graphs of
// different sizes. The search grows with the postings it scores, but a read
// that returned hits must not also walk the whole graph, as an emptiness check
// derived from Stats would.
func BenchmarkRecallCommit(b *testing.B) {
	for _, size := range []int{1_000, 10_000} {
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
				r := &Recall[uint64, float32]{
					Keywords:   []string{"fact", "number"},
					Parameters: QueryParameters[uint64]{Top: 10},
				}
				s := NewStream[uint64, float32](r)
				if err := s.Commit(g); err != nil {
					b.Fatalf("Commit = %v", err)
				}
				if s.Result.Count == 0 {
					b.Fatalf("Result.Count = 0, want hits: the benchmark measures a read that matched")
				}
			}
		})
	}
}

// TestCommitSeedsFromAnchorsStoredByCommit drives the production write path
// and then the anchor-only read it makes possible. A Recall naming a topic
// and no term reaches Search with nil keywords and an empty vector, and the
// seeding must resolve the topic to the node Commit stored: under any other
// key it would seed nothing. Every fact filed under the topic comes back and
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
