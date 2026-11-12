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
	"sync"
	"time"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// Stream is one execution of a planned query. The engine builds one per
// request. The scheduler commits it against the selected graph, under the read
// lock for a recall or a when and the write lock for a remember. The outcome
// lands in Result or Err.
type Stream[K comparable, P float32 | float64] struct {
	Query  Query[K, P]
	Result *QueryResult[K, P]
	Err    error

	// Explain asks the read path to attach each hit's contributions and the
	// query's background rate to the result. It lives on the stream because a
	// Query carries no per-request state (see Query). Hashing the flag into the
	// query instead would widen the cache key for a bit that never changes the
	// plan. The stream is built per request and never cached.
	Explain bool

	// IsGraphEmpty is set when a read matched nothing in a graph that holds no
	// nodes. The server then answers 204 rather than the empty result of a
	// populated graph.
	IsGraphEmpty bool

	// Since and Until are the streams of the queries nested in a recall's time
	// bounds, each nil for a bound that holds none. [Recall.Plan] hangs them
	// here. Commit runs them before the search, on the same graph and under
	// the lock already held, so the bound and the search read one state of
	// the graph.
	Since *Stream[K, P]
	Until *Stream[K, P]

	// Warnings tell the caller that the query ran but may not have read as it
	// was meant: a when that matched no fact has no time to answer with. They
	// are raised while the stream commits, where the parser raises its own
	// while it reads the text, and the server returns both.
	Warnings []string

	done chan struct{}
	once sync.Once
}

// NewStream returns a stream for q, ready to be scheduled.
func NewStream[K comparable, P float32 | float64](q Query[K, P]) *Stream[K, P] {
	return &Stream[K, P]{Query: q, done: make(chan struct{})}
}

// storeAnchor stores an anchor node (entity or topic) and the edge linking it
// to its fact. Either may already be stored, since anchors are keyed by value
// and edges by their endpoints. A shared anchor or a re-remembered fact finds
// them present, which is not an error. Any other failure is returned, so the
// write fails instead of succeeding with the fact half-linked to its anchors.
func storeAnchor[K comparable, P float32 | float64](g graph.Graph[K, P], node, edge graph.Node[K]) error {
	if err := g.Set(node); err != nil && !errors.Is(err, graph.ErrNodeAlreadyExists) {
		logger.Error("Failed to store anchor node", "error", err)
		return fmt.Errorf("storing anchor: %w", err)
	}
	if err := g.Set(edge); err != nil && !errors.Is(err, graph.ErrNodeAlreadyExists) {
		logger.Error("Failed to store anchor edge", "error", err)
		return fmt.Errorf("linking anchor: %w", err)
	}
	return nil
}

// Commit executes the stream's query against g in place. The caller holds the
// lock Acquire takes: a read lock for a read, the exclusive write lock for a
// write. So mutating g directly exposes no intermediate state, and a write
// touches only what it stores. Staging a write on a copy of the graph would
// make every write O(graph), all of it under the write lock.
//
// A write inserts the vector before it touches the graph. So the one failure a
// client can trigger, a vector whose dimension differs from the index's,
// rejects the write with g untouched. Later errors are not expected in
// practice. If one occurs, it is returned with the write partly applied. A read
// fails on the same mismatch rather than answering from the text index alone.
//
// A recall whose time bound holds a nested query commits that query's stream
// first, then searches inside the window its answer draws. A when runs as its
// recall does, and its result also carries the time its hits hold.
//
// Errors and Error lines name the step that failed, never the fact or anchor
// value. A returned error is logged by the scheduler and the server at Error,
// so a value quoted in it would copy a remembered fact into the default log.
func (s *Stream[K, P]) Commit(g graph.Graph[K, P]) error {
	switch q := s.Query.(type) {
	case *Remember[K, P]:
		return s.write(g, q)
	case *Recall[K, P]:
		return s.read(g, q)
	case *When[K, P]:
		if err := s.read(g, &q.Recall); err != nil {
			return err
		}
		s.answer(q.Clock)
		return nil
	default:
		return fmt.Errorf("committing stream: no commit for a %T", s.Query)
	}
}

// write stores remember's fact in g, with its vector and its anchors.
func (s *Stream[K, P]) write(g graph.Graph[K, P], remember *Remember[K, P]) error {
	logger.Debug("Committing write stream",
		"entities", len(remember.Entities),
		"topics", len(remember.Topics),
		"vector", !remember.Vector.Empty())

	// One reading of the clock stamps the fact and resolves the times it was
	// given. So a time given as a duration counts back from the instant the
	// fact is written, and a fact said "now" is said when it is written.
	now := time.Now()
	fact := graph.Fact[K]{
		NodeAttributes: graph.NodeAttributes{
			Value:     remember.Value,
			Timestamp: now,
		},
		Hasher: g.GetHasher(),
	}
	if remember.Said != nil {
		fact.Said = remember.Said.Resolve(now).Start
	}
	for _, occurred := range remember.Occurred {
		fact.Occurred = append(fact.Occurred, occurred.Resolve(now))
	}

	// Index the vector before touching the graph. The fact's key derives
	// from its value alone, so it is known before the fact is stored. A
	// dimension mismatch then fails here, with the graph as it was. A fact
	// the vector index already holds is re-asserted: Update replaces its
	// vector rather than adding it again.
	if !remember.Vector.Empty() {
		vectors := g.GetVectorIndex()
		write := vectors.Insert
		if _, err := vectors.Retrieve(fact.Key()); err == nil {
			write = vectors.Update
		}
		if err := write(fact.Key(), remember.Vector); err != nil {
			logger.Error("Failed to index fact vector", "error", err)
			return fmt.Errorf("indexing fact vector: %w", err)
		}
	}

	// Facts are keyed by value, so re-remembering one is a temporal touch.
	// The fact with a fresh timestamp replaces the stored one, and recency
	// decay restarts. So a memory an agent re-asserts is strengthened
	// rather than left decaying from its first write. A changed vector
	// replaces the fact's index entry the same way, above. The times
	// replace the stored ones too: the fact is said and happened when this
	// remember says, and reads its timestamp on a clock this remember
	// leaves out.
	if err := g.Put(fact.Key(), fact); err != nil {
		logger.Error("Failed to store fact", "error", err)
		return fmt.Errorf("storing fact: %w", err)
	}

	for _, e := range remember.Entities {

		entity := graph.NamedEntity[K]{NodeAttributes: graph.NodeAttributes{
			Value:     e,
			Timestamp: time.Now(),
		},
			Hasher: g.GetHasher(),
		}

		mentions := graph.Mentions[K]{NodeAttributes: graph.NodeAttributes{
			Timestamp: time.Now(),
		},
			Fact:        &fact,
			NamedEntity: &entity,
			Hasher:      g.GetHasher(),
		}

		if err := storeAnchor(g, &entity, mentions); err != nil {
			return err
		}
	}

	for _, t := range remember.Topics {

		topic := graph.Topic[K]{NodeAttributes: graph.NodeAttributes{
			Value:     t,
			Timestamp: time.Now(),
		},
			Hasher: g.GetHasher(),
		}

		about := graph.IsAbout[K]{NodeAttributes: graph.NodeAttributes{
			Timestamp: time.Now(),
		},
			Fact:   &fact,
			Topic:  &topic,
			Hasher: g.GetHasher(),
		}

		if err := storeAnchor(g, &topic, about); err != nil {
			return err
		}
	}

	r := QueryResult[K, P]{
		Count: 0,
		Hits:  make([]Hit[K, P], 0),
	}
	s.Result = &r
	s.IsGraphEmpty = false
	logger.Debug("Write stream committed", "value", remember.Value)
	return nil
}

// read searches g for recall and builds the stream's result. The time window
// is the one recall writes out, with each bound that holds a nested query read
// from that query's answer. since keeps what came after the answer and until
// what came before it, each compared on the clock the when read.
func (s *Stream[K, P]) read(g graph.Graph[K, P], recall *Recall[K, P]) error {
	logger.Debug("Committing read stream",
		"keywords", len(recall.Keywords),
		"vector", !recall.Vector.Empty(),
		"depth", recall.Parameters.Depth,
		"top", recall.Parameters.Top)

	// One reading of the clock resolves both bounds, so a window written as
	// two durations is exactly as wide as they say.
	now := time.Now()
	since, until := recall.Since(now), recall.Until(now)
	answered := true
	if s.Since != nil {
		when, err := s.commitNested(g, s.Since, "since")
		if err != nil {
			return err
		}
		// What came after the answer starts where the answer ends. An answer
		// that has not ended has no such instant, and since reads its start.
		since = containers.Bound{At: s.Since.Result.End, Clock: when.Clock}
		if since.At.IsZero() {
			since.At = s.Since.Result.Start
		}
		answered = answered && !s.Since.Result.Start.IsZero()
	}
	if s.Until != nil {
		when, err := s.commitNested(g, s.Until, "until")
		if err != nil {
			return err
		}
		until = containers.Bound{At: s.Until.Result.Start, Clock: when.Clock}
		answered = answered && !s.Until.Result.Start.IsZero()
	}

	// A bound whose when answered with no time has nothing to compare with.
	// The recall then returns no hits. Searching without the bound would
	// return the facts the query asked to leave out.
	var result graph.Result[K, P]
	if answered {
		var err error
		result, err = g.Search(
			recall.Keywords,
			recall.Vector,
			recall.Topics,
			recall.Entities,
			recall.Parameters.Depth,
			recall.Parameters.Top,
			since,
			until,
		)
		if err != nil {
			return fmt.Errorf("searching with the recall's vector: %w", err)
		}
	}

	n := len(result.Hits)
	r := QueryResult[K, P]{
		Count: n,
		Hits:  make([]Hit[K, P], n),
	}
	if s.Explain {
		// The background rate is per query, so it rides on the result rather
		// than on each hit (see QueryResult.Background). So do the answers of
		// the nested queries, which show where each bound was read from.
		r.Background = result.Background
		r.When = s.nestedResults()
	}
	for i, hit := range result.Hits {
		r.Hits[i].Node = hit.Node
		r.Hits[i].Score = hit.Score
		if s.Explain {
			r.Hits[i].Decay = hit.Decay
			r.Hits[i].Contributions = resolveContributions(g, hit.Contributions)
		}
	}

	s.Result = &r
	// Only a read that matched nothing needs to tell an empty graph from a
	// populated one it missed. So neither a write nor a read with hits asks.
	s.IsGraphEmpty = n == 0 && g.IsEmpty()
	logger.Debug("Read stream committed", "hits", n, "explain", s.Explain)
	return nil
}

// commitNested runs the stream of the query nested in the clause's time bound
// and returns the when it answered for. It commits on g under the lock the
// outer stream holds, so the bound and the search read one state of the graph.
// A time bound reads a time, so the nested query is a when, and any other
// query fails the stream. The nested stream explains itself when the outer
// one does. Its warnings reach the caller under the clause's name, with what
// they cost the recall.
func (s *Stream[K, P]) commitNested(g graph.Graph[K, P], nested *Stream[K, P], clause string) (*When[K, P], error) {
	when, ok := nested.Query.(*When[K, P])
	if !ok {
		return nil, fmt.Errorf("committing the query in %s: a time bound reads a when, not a %T", clause, nested.Query)
	}
	nested.Explain = s.Explain
	if err := nested.Commit(g); err != nil {
		return nil, fmt.Errorf("committing the query in %s: %w", clause, err)
	}
	for _, w := range nested.Warnings {
		s.Warnings = append(s.Warnings, fmt.Sprintf("%s: %s; the recall has no bound to read and returns no hits", clause, w))
	}
	return when, nil
}

// nestedResults returns what the stream's nested queries answered, for an
// explain response. It is nil for a stream that has none.
func (s *Stream[K, P]) nestedResults() *NestedResults[K, P] {
	if s.Since == nil && s.Until == nil {
		return nil
	}
	var nested NestedResults[K, P]
	if s.Since != nil {
		nested.Since = s.Since.Result
	}
	if s.Until != nil {
		nested.Until = s.Until.Result
	}
	return &nested
}

// answer completes a when's result with the time it answers with: the hull of
// what each hit's fact holds on clock (see [graph.Fact.Intervals]). With one
// hit that is the fact's own span. With no hit there is no time, and the
// stream warns, so a caller reading a result with neither start nor end
// learns why.
func (s *Stream[K, P]) answer(clock containers.Clock) {
	var answer containers.Interval
	answered := false
	for _, hit := range s.Result.Hits {
		fact, ok := hit.Node.(graph.Fact[K])
		if !ok {
			continue
		}
		for _, interval := range fact.Intervals(clock) {
			if answered {
				interval = answer.Hull(interval)
			}
			answer, answered = interval, true
		}
	}
	if !answered {
		s.Warnings = append(s.Warnings, "the when matched no fact, so it has no time to answer with")
		return
	}
	s.Result.Start, s.Result.End = answer.Start, answer.End
}

// resolveContributions converts a hit's contributions to their wire form. It
// names each source, and resolves a graph or anchor entry's anchor key to its
// stored topic or entity value. An anchor that is no longer stored leaves via
// empty.
func resolveContributions[K comparable, P float32 | float64](g graph.Graph[K, P], contributions []scoring.Contribution[K, P]) []HitContribution[P] {
	out := make([]HitContribution[P], len(contributions))
	for i, c := range contributions {
		wire := HitContribution[P]{
			Source: c.Src.String(),
			Score:  c.Score,
			Rank:   c.Rank,
			Degree: c.Degree,
			Count:  c.Count,
			Named:  c.Named,
			Held:   c.Held,
		}
		if c.Src == scoring.SrcGraph || c.Src == scoring.SrcAnchor {
			if node := g.Get(c.Via); node != nil {
				wire.Via = node.GetValue()
			}
		}
		out[i] = wire
	}
	return out
}

// GraphID returns the graph the stream's query targets.
func (s *Stream[K, P]) GraphID() uint8 {
	return s.Query.GetGraphID()
}

// Done returns a channel closed once the scheduler has finished with the
// stream, whether it committed or failed. Result and Err are read only after
// it closes.
func (s *Stream[K, P]) Done() <-chan struct{} {
	return s.done
}

// Finish closes Done. It is safe to call more than once. So the scheduler can
// defer it unconditionally, and still wake the waiter of a stream that failed
// before Commit.
func (s *Stream[K, P]) Finish() {
	s.once.Do(func() { close(s.done) })
}

// Acquire takes the lock on g that the query needs: the exclusive lock for a
// write, the shared lock for a read. So reads run concurrently and a write runs
// alone. Commit runs between Acquire and [Stream.Release].
func (s *Stream[K, P]) Acquire(g graph.Graph[K, P]) {
	if s.Query.IsWrite() {
		g.Lock()
	} else {
		g.RLock()
	}
}

// Release drops the lock Acquire took on g.
func (s *Stream[K, P]) Release(g graph.Graph[K, P]) {
	if s.Query.IsWrite() {
		g.Unlock()
	} else {
		g.RUnlock()
	}
}
