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

	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// Stream is one execution of a planned query: the engine builds one per
// request, and the scheduler commits it against the selected graph, under the
// read lock for a recall and the write lock for a remember. The outcome lands
// in Result or Err.
type Stream[K comparable, P float32 | float64] struct {
	Query  Query[K, P]
	Result *QueryResult[K, P]
	Err    error

	// Explain asks the read path to attach each hit's contributions and the
	// query's background rate to the result. It lives on the stream rather
	// than the query because the engine caches query objects by hash and
	// substitutes them on a hit: a flag on the query would leak one request's
	// choice into another's, or widen the cache key for a bit that never
	// changes the plan. The stream is built per request and never cached.
	Explain bool

	// IsGraphEmpty is set when a read matched nothing in a graph that holds no
	// nodes, so the server can answer 204 rather than the empty result of a
	// populated graph.
	IsGraphEmpty bool

	done chan struct{}
	once sync.Once
}

// NewStream returns a stream ready to be scheduled for q. Done is closed when
// the scheduler finishes with it, whether or not it succeeded.
func NewStream[K comparable, P float32 | float64](q Query[K, P]) *Stream[K, P] {
	return &Stream[K, P]{Query: q, done: make(chan struct{})}
}

// storeAnchor stores an anchor node (entity or topic) and the edge linking it
// to its fact. Either may already be stored, since anchors are keyed by value
// and edges by their endpoints: a shared anchor or a re-remembered fact finds
// them present, which is not an error. Any other failure rejects the write
// rather than committing the fact half-linked to its anchors.
func storeAnchor[K comparable, P float32 | float64](g graph.Graph[K, P], node, edge graph.Node[K]) error {
	if err := g.Set(node); err != nil && !errors.Is(err, graph.ErrNodeAlreadyExists) {
		logger.Error("Failed to store anchor node", "anchor", node.GetValue(), "error", err)
		return fmt.Errorf("storing anchor %q: %w", node.GetValue(), err)
	}
	if err := g.Set(edge); err != nil && !errors.Is(err, graph.ErrNodeAlreadyExists) {
		logger.Error("Failed to store anchor edge", "anchor", node.GetValue(), "error", err)
		return fmt.Errorf("linking anchor %q: %w", node.GetValue(), err)
	}
	return nil
}

// Commit executes the stream's query against g in place. The caller holds the
// lock Acquire takes: a read lock for a read, the exclusive write lock for a
// write, so mutating g directly exposes no intermediate state and a write
// touches only what it stores. Copying the graph to stage a write would make
// every write O(graph), all of it under the write lock.
//
// A write inserts the vector before touching the graph, so the one failure a
// client can trigger, a vector whose dimension differs from the index's,
// rejects the write with g untouched. Later errors are not expected in
// practice; one is returned with the write partially applied. A read fails on
// the same mismatch rather than answering from the text index alone.
func (s *Stream[K, P]) Commit(g graph.Graph[K, P]) error {

	// Write stream
	if s.Query.IsWrite() {

		remember := s.Query.(*Remember[K, P])

		logger.Debug("Committing write stream",
			"entities", len(remember.Entities),
			"topics", len(remember.Topics),
			"vector", !remember.Vector.Empty())

		fact := graph.Fact[K]{
			NodeAttributes: graph.NodeAttributes{
				Value:     remember.Value,
				Timestamp: time.Now(),
			},
			Hasher: g.GetHasher(),
		}

		// Index the vector before touching the graph. The fact's key derives
		// from its value alone, so it is known before the fact is stored, and
		// a dimension mismatch fails here with the graph as it was. A fact
		// the vector index already holds is re-asserted: its vector is
		// replaced through Update rather than added again.
		if !remember.Vector.Empty() {
			vectors := g.GetVectorIndex()
			write := vectors.Insert
			if _, err := vectors.Retrieve(fact.Key()); err == nil {
				write = vectors.Update
			}
			if err := write(fact.Key(), remember.Vector); err != nil {
				logger.Error("Failed to index fact vector",
					"value", remember.Value, "error", err)
				return fmt.Errorf("indexing vector for fact %q: %w", remember.Value, err)
			}
		}

		// Facts are keyed by value, so re-remembering one is a temporal touch:
		// the fact with a fresh timestamp replaces the stored one and recency
		// decay restarts, so a memory an agent re-asserts is strengthened
		// rather than left decaying from its first write. A changed vector
		// replaces the fact's index entry the same way, above.
		if err := g.Put(fact.Key(), fact); err != nil {
			logger.Error("Failed to store fact", "value", remember.Value, "error", err)
			return fmt.Errorf("storing fact %q: %w", remember.Value, err)
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

	// Read stream
	recall := s.Query.(*Recall[K, P])
	logger.Debug("Committing read stream",
		"keywords", len(recall.Keywords),
		"vector", !recall.Vector.Empty(),
		"depth", recall.Parameters.Depth,
		"top", recall.Parameters.Top)
	result, err := g.Search(
		recall.Keywords,
		recall.Vector,
		recall.Topics,
		recall.Entities,
		recall.Parameters.Depth,
		recall.Parameters.Top,
		recall.Since(time.Now()),
		recall.Until(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("searching with the recall's vector: %w", err)
	}

	// copy results to Hit object
	n := len(result.Hits)
	r := QueryResult[K, P]{
		Count: n,
		Hits:  make([]Hit[K, P], n),
	}
	if s.Explain {
		// The background rate and the spread are query-level, so they ride
		// on the result rather than on each hit (see QueryResult.Background).
		r.Background = result.Background
		r.Spread = result.Spread
	}
	for i, hit := range result.Hits {
		r.Hits[i].Node = hit.Node
		r.Hits[i].Score = hit.Score
		r.Hits[i].Key = hit.Key
		// A group hit carries its members as hits of their own, so the wire
		// form of a member is the wire form of a fact.
		for j, member := range hit.Members {
			r.Hits[i].Members = append(r.Hits[i].Members, Hit[K, P]{Node: member, Score: hit.MemberScores[j]})
		}
		// Contributions are attached only in explain mode; a nil slice keeps
		// them out of the ordinary response (see Hit.MarshalJSON). They are
		// resolved to wire form here, under the graph lock, because a graph
		// or anchor contribution names its anchor by key, and only the graph
		// can turn that key into the topic or entity value a client reads.
		if s.Explain {
			r.Hits[i].Contributions = resolveContributions(g, hit.Contributions)
		}
	}

	s.Result = &r
	// Only a read that matched nothing needs to tell an empty graph from a
	// populated one it missed, so neither a write nor a read with hits asks.
	s.IsGraphEmpty = n == 0 && g.IsEmpty()
	logger.Debug("Read stream committed", "hits", n, "explain", s.Explain)
	return nil
}

// resolveContributions converts a hit's contributions to their wire form:
// each source by name and, for graph and anchor entries, the anchor's key
// resolved to its stored topic or entity value. An anchor that is no longer
// stored leaves via empty.
func resolveContributions[K comparable, P float32 | float64](g graph.Graph[K, P], contributions []scoring.Contribution[K, P]) []HitContribution[P] {
	out := make([]HitContribution[P], len(contributions))
	for i, c := range contributions {
		wire := HitContribution[P]{
			Source: c.Src.String(),
			Score:  c.Score,
			Rank:   c.Rank,
			Degree: c.Degree,
			Count:  c.Count,
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

// Finish closes Done. It is safe to call more than once, so the scheduler can
// defer it unconditionally and still wake the waiter of a stream that failed
// before Commit.
func (s *Stream[K, P]) Finish() {
	s.once.Do(func() { close(s.done) })
}

// Acquire takes the lock on g that the query needs: the exclusive lock for a
// write, the shared lock for a read, so reads run concurrently and a write
// runs alone. Commit runs between Acquire and [Stream.Release].
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
