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
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/RonsenbergVI/fraise/internal/index"
)

// fakeGraph is a controllable graph.Graph used to observe how Stream drives the
// graph: which lock Acquire/Release take, whether Stage copies, whether the
// write path writes and the read path searches, and what Search returns. Only
// the methods Stream touches carry behaviour; the rest are inert stubs present
// to satisfy the interface.
type fakeGraph struct {
	locks, unlocks   int
	rlocks, runlocks int
	copied, merged   bool
	sets             int
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
func (g *fakeGraph) Set(node graph.Node[string]) error         { g.sets++; return nil }

func (g *fakeGraph) Search(keywords []string, vector containers.Vector[float32], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*graph.Node[string], []float32) {
	g.searchCalled = true
	return g.searchNodes, g.searchScores
}

// --- inert stubs (unused by Stream) ----------------------------------------

func (g *fakeGraph) GetHasher() hash.Hasher[string, string]             { return nil }
func (g *fakeGraph) Get(key string) graph.Node[string]                  { return nil }
func (g *fakeGraph) Put(key string, node graph.Node[string]) error      { return nil }
func (g *fakeGraph) Delete(node graph.Node[string]) error               { return nil }
func (g *fakeGraph) GetVectorIndex() index.VectorIndex[string, float32] { return nil }
func (g *fakeGraph) GetTextIndex() index.TextIndex[string]              { return nil }
func (g *fakeGraph) Nodes() map[string]graph.Node[string]               { return nil }
func (g *fakeGraph) AdjacencyMap() map[string]map[string]string         { return nil }
func (g *fakeGraph) PredecessorMap() map[string]map[string]string       { return nil }
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

// --- Stage -----------------------------------------------------------------

func TestStreamStageReadCommitsInPlace(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(readQuery())

	staged, err := s.Stage(g)
	if err != nil {
		t.Fatalf("Stage() err = %v", err)
	}
	// The read path commits against the live graph: no copy, no staging.
	if g.copied {
		t.Error("read Stage copied the graph, want in-place")
	}
	if s.staging != nil {
		t.Error("read Stage set staging, want nil")
	}
	if !g.searchCalled {
		t.Error("read Stage did not run the query's Search")
	}
	if staged != graph.Graph[string, float32](g) {
		t.Error("read Stage returned a different graph than the one passed")
	}
}

func TestStreamStageWriteCopiesAndWrites(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(&Remember[string, float32]{Value: "alice"})

	staged, err := s.Stage(g)
	if err != nil {
		t.Fatalf("Stage() err = %v", err)
	}
	if !g.copied {
		t.Error("write Stage did not Copy the graph into staging")
	}
	if s.staging == nil {
		t.Error("staging is nil after write Stage")
	}
	if g.sets == 0 {
		t.Error("write Stage did not write the fact into staging")
	}
	if staged == nil {
		t.Error("write Stage returned a nil graph")
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
}

// --- Commit (write path) ---------------------------------------------------

func TestStreamCommitWriteWrites(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(&Remember[string, float32]{Value: "alice"})
	s.staging = g // a staged write stream

	if err := s.Commit(g); err != nil {
		t.Fatalf("Commit() err = %v", err)
	}

	if g.sets == 0 {
		t.Error("Commit did not write the fact on a write query")
	}
	if g.searchCalled {
		t.Error("Commit called Search on a write query")
	}
	if s.Result == nil {
		t.Fatal("Commit left Result nil on a write query")
	}
}

func TestStreamCommitWriteRequiresStaging(t *testing.T) {
	s := newStream(&Remember[string, float32]{}) // no Stage -> staging is nil
	if err := s.Commit(&fakeGraph{}); err != ErrStreamClosed {
		t.Errorf("Commit() on unstaged write stream = %v, want ErrStreamClosed", err)
	}
}

// --- Rollback --------------------------------------------------------------

func TestStreamRollbackClearsStaging(t *testing.T) {
	g := &fakeGraph{}
	s := newStream(&Remember[string, float32]{})
	s.staging = g

	if err := s.Rollback(g); err != nil {
		t.Fatalf("Rollback() err = %v", err)
	}
	if s.staging != nil {
		t.Error("Rollback did not clear staging")
	}
	if g.merged {
		t.Error("Rollback merged staging into the graph")
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
