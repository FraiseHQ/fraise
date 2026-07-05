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
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
	"github.com/RonsenbergVI/fraise/internal/index"
)

// fakeGraph is a controllable graph.Graph used to observe how Stream drives
// the graph: which lock it takes, whether it copies/merges/searches, and what
// Search returns. Only the methods Stream touches carry behaviour; the rest are
// inert stubs present to satisfy the interface.
type fakeGraph struct {
	locks, unlocks   int
	rlocks, runlocks int
	copied, merged   bool
	searchCalled     bool

	searchNodes  []*graph.Node[string]
	searchScores []float32
}

var _ graph.Graph[string, float32] = (*fakeGraph)(nil)

func (g *fakeGraph) Lock()    { g.locks++ }
func (g *fakeGraph) Unlock()  { g.unlocks++ }
func (g *fakeGraph) RLock()   { g.rlocks++ }
func (g *fakeGraph) RUnlock() { g.runlocks++ }

func (g *fakeGraph) Copy() graph.Graph[string, float32]        { g.copied = true; return g }
func (g *fakeGraph) MergeFrom(in graph.Graph[string, float32]) { g.merged = true }

func (g *fakeGraph) Search(keywords []string, vector containers.Vector[float32], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*graph.Node[string], []float32) {
	g.searchCalled = true
	return g.searchNodes, g.searchScores
}

// --- inert stubs (unused by Stream) ----------------------------------------

func (g *fakeGraph) Get(key string) *graph.Node[string]             { return nil }
func (g *fakeGraph) Set(node *graph.Node[string]) error             { return nil }
func (g *fakeGraph) Put(key string, node *graph.Node[string]) error { return nil }
func (g *fakeGraph) Delete(node *graph.Node[string]) error          { return nil }
func (g *fakeGraph) GetVectorIndex() index.VectorIndex[string, float32] {
	return nil
}
func (g *fakeGraph) GetTextIndex() index.TextIndex[string]                             { return nil }
func (g *fakeGraph) Entities() []*graph.Entity[string]                                 { return nil }
func (g *fakeGraph) Relationships() []*graph.Relationship[string]                      { return nil }
func (g *fakeGraph) AdjacencyMap() map[string]map[string]*graph.Relationship[string]   { return nil }
func (g *fakeGraph) PredecessorMap() map[string]map[string]*graph.Relationship[string] { return nil }
func (g *fakeGraph) Order() int                                                        { return 0 }
func (g *fakeGraph) Size() int                                                         { return 0 }
func (g *fakeGraph) Stats() graph.GraphStats                                           { return graph.GraphStats{} }

// --- helpers ---------------------------------------------------------------

func newStream(q Query[string, float32]) *Stream[string, float32] {
	return &Stream[string, float32]{Query: q, done: make(chan struct{})}
}

// readQuery builds a Recall with non-nil time bounds so the read path in
// Commit can call Since/Until without dereferencing a nil TimeValue. It returns
// a *Recall because SetGraphID's pointer receiver means only *Recall satisfies
// Query (and Commit's read path asserts *Recall).
func readQuery() *Recall[string, float32] {
	return &Recall[string, float32]{
		Keywords: []string{"x"},
		Parameters: QueryParameters{
			Since: containers.AbsoluteTime{},
			Until: containers.AbsoluteTime{},
		},
	}
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// --- Stage -----------------------------------------------------------------

func TestStreamStageReadTakesReadLock(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(readQuery())

	if err := s.Stage(g); err != nil {
		t.Fatalf("Stage() err = %v", err)
	}
	if g.rlocks != 1 || g.locks != 0 {
		t.Errorf("read Stage locks: rlocks=%d locks=%d, want rlocks=1 locks=0", g.rlocks, g.locks)
	}
	if !g.copied {
		t.Error("Stage did not Copy the graph into staging")
	}
	if s.staging == nil {
		t.Error("staging is nil after Stage")
	}
}

func TestStreamStageWriteTakesWriteLock(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(&Remember[string, float32]{})

	if err := s.Stage(g); err != nil {
		t.Fatalf("Stage() err = %v", err)
	}
	if g.locks != 1 || g.rlocks != 0 {
		t.Errorf("write Stage locks: locks=%d rlocks=%d, want locks=1 rlocks=0", g.locks, g.rlocks)
	}
}

// --- Commit (read path) ----------------------------------------------------

func TestStreamCommitReadBuildsResult(t *testing.T) {
	g := &fakeGraph{
		searchNodes:  []*graph.Node[string]{nil, nil, nil},
		searchScores: []float32{0.9, 0.8, 0.7},
	}
	s := newStream(readQuery())
	if err := s.Stage(g); err != nil {
		t.Fatalf("Stage() err = %v", err)
	}

	if err := s.Commit(g); err != nil {
		t.Fatalf("Commit() err = %v", err)
	}

	if !g.searchCalled {
		t.Error("Commit did not call Search on a read query")
	}
	if g.merged {
		t.Error("Commit called MergeFrom on a read query")
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
	if s.staging != nil {
		t.Error("Commit did not clear staging")
	}
	if g.runlocks != 1 {
		t.Errorf("read lock released %d times, want 1", g.runlocks)
	}
	if !isClosed(s.Done()) {
		t.Error("Done channel not closed after Commit")
	}
}

// --- Commit (write path) ---------------------------------------------------

func TestStreamCommitWriteMerges(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(&Remember[string, float32]{})
	if err := s.Stage(g); err != nil {
		t.Fatalf("Stage() err = %v", err)
	}

	if err := s.Commit(g); err != nil {
		t.Fatalf("Commit() err = %v", err)
	}

	if !g.merged {
		t.Error("Commit did not MergeFrom on a write query")
	}
	if g.searchCalled {
		t.Error("Commit called Search on a write query")
	}
	if s.staging != nil {
		t.Error("Commit did not clear staging")
	}
	if g.unlocks != 1 {
		t.Errorf("write lock released %d times, want 1", g.unlocks)
	}
	if !isClosed(s.Done()) {
		t.Error("Done channel not closed after Commit")
	}
}

func TestStreamCommitClosedStream(t *testing.T) {
	s := newStream(readQuery()) // no Stage -> staging is nil
	if err := s.Commit(&fakeGraph{}); err != ErrStreamClosed {
		t.Errorf("Commit() on unstaged stream = %v, want ErrStreamClosed", err)
	}
}

// --- Rollback --------------------------------------------------------------

func TestStreamRollback(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(readQuery())
	if err := s.Stage(g); err != nil {
		t.Fatalf("Stage() err = %v", err)
	}

	if err := s.Rollback(g); err != nil {
		t.Fatalf("Rollback() err = %v", err)
	}
	if s.staging != nil {
		t.Error("Rollback did not clear staging")
	}
	if g.merged {
		t.Error("Rollback merged staging into the graph")
	}
	if g.runlocks != 1 {
		t.Errorf("read lock released %d times, want 1", g.runlocks)
	}
	if !isClosed(s.Done()) {
		t.Error("Done channel not closed after Rollback")
	}
}

func TestStreamRollbackClosedStream(t *testing.T) {
	s := newStream(readQuery()) // no Stage -> staging is nil
	if err := s.Rollback(&fakeGraph{}); err != ErrStreamClosed {
		t.Errorf("Rollback() on unstaged stream = %v, want ErrStreamClosed", err)
	}
}

// --- GraphID / Done / finish -----------------------------------------------

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
	s.finish()
	s.finish() // second call must not panic or double-close (sync.Once)
	if !isClosed(s.Done()) {
		t.Error("Done channel not closed after finish")
	}
}
