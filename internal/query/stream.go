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

// data structure representing a stream: language of the scheduler
// A stream is the language for the engine to the worker
// Streams can be read-only or read and write.
type Stream[K comparable, P float32 | float64] struct {
	Query  Query[K, P]
	Result *QueryResult[K, P]
	Err    error

	// Explain asks the read path to attach each hit's contribution records to
	// the result. It lives on the stream, not the query, on purpose: the
	// engine caches query objects by hash and substitutes them on a hit, so a
	// flag on the query would either leak one request's explain choice into
	// another's or have to widen the cache key for a bit that never changes
	// the plan. The stream is built per request and never cached.
	Explain bool

	done chan struct{}
	once sync.Once
}

// NewStream returns a stream ready to be scheduled for q: Done() blocks until
// the scheduler commits or rolls the stream back.
func NewStream[K comparable, P float32 | float64](q Query[K, P]) *Stream[K, P] {
	return &Stream[K, P]{Query: q, done: make(chan struct{})}
}

// storeAnchor stores an anchor node (entity or topic) and the edge binding it
// to its fact, treating "already stored" as the shared-anchor upsert for both:
// anchors are keyed by value and edges by their endpoints, so a re-remember
// finds them present. Any other failure means the anchor never became
// reachable — anchored recalls resolve a fact's anchors through its stored
// neighbours, so an edge to a half-stored pair makes every entity:/topic:
// filter miss — and the write is rejected rather than committed half-linked.
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

// Commit executes the stream's query against g directly. The caller must hold
// the appropriate lock (the write lock for writes, a read lock for reads — see
// Acquire): the lock is already exclusive for writes, so mutating g in place
// exposes no intermediate state, and the write costs O(fact + incremental
// index updates) regardless of graph size. Copying the graph here (as staging
// once did) would make every single-fact write O(total graph) and lock readers
// out for the duration — that is the failure mode, not the safety mechanism.
//
// Failure ordering: the vector insert runs before any graph mutation, so the
// one realistic commit failure (a vector-dimension mismatch) rejects the write
// with g untouched. Later index errors are pathological; they surface in the
// returned error with the write partially applied.
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

		// Index the vector before touching the graph: the fact's key is
		// derived from its value alone, and a dimension mismatch is the one
		// commit failure a client can realistically trigger — failing here
		// leaves the graph exactly as it was.
		if !remember.Vector.Empty() {
			if err := g.GetVectorIndex().Insert(fact.Key(), remember.Vector); err != nil {
				logger.Error("Failed to index fact vector",
					"value", remember.Value, "error", err)
				return fmt.Errorf("indexing vector for fact %q: %w", remember.Value, err)
			}
		}

		// Facts are content-addressed (keyed by value), so re-remembering one
		// is the temporal "touch": the fresh-timestamp fact replaces the
		// stored one and recency decay restarts — an agent re-asserting a
		// memory strengthens it rather than leaving it decaying from its
		// first write. The embedding refreshes the same way (a changed vector
		// overwrites its index entry above), keeping the two consistent.
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
	nodes, scores, contributions, background := g.Search(
		recall.Keywords,
		recall.Vector,
		recall.Topics,
		recall.Entities,
		recall.Parameters.Depth,
		recall.Parameters.Top,
		recall.Since(time.Now()),
		recall.Until(time.Now()),
	)

	// copy results to Hit object
	n := len(nodes)
	r := QueryResult[K, P]{
		Count: n,
		Hits:  make([]Hit[K, P], n),
	}
	if s.Explain {
		// Explain explains through the anchors, so the payload carries the
		// query-level background rate alongside each hit's breakdown.
		r.Background = background
	}
	for i := 0; i < n; i++ {
		r.Hits[i].Node = nodes[i]
		r.Hits[i].Score = scores[i]
		// Contributions ride on the hit only in explain mode: a nil slice is
		// what keeps them out of the ordinary response (see Hit.MarshalJSON),
		// so the plain query wire format does not change shape. Resolution to
		// the wire form happens here — under the graph lock — because the
		// anchor a graph contribution arrived via is a key, and only the
		// graph can turn it into the topic/entity value a client can read.
		if s.Explain {
			r.Hits[i].Contributions = resolveContributions(g, contributions[i])
		}
	}

	s.Result = &r
	logger.Debug("Read stream committed", "hits", n, "explain", s.Explain)
	return nil
}

// resolveContributions maps a hit's collected contributions to their wire
// form: sources serialized by name and, for graph and anchor entries, the
// funding or filing anchor's key resolved to its stored value — the topic or
// entity name a client can actually read. A vanished anchor falls back to an
// empty via rather than inventing one.
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

func (s *Stream[K, P]) GraphID() uint8 {
	return s.Query.GetGraphID()
}

func (s *Stream[K, P]) Done() <-chan struct{} {
	return s.done
}

func (s *Stream[K, P]) Finish() {
	s.once.Do(func() { close(s.done) })
}

func (s *Stream[K, P]) Acquire(g graph.Graph[K, P]) {
	if s.Query.IsWrite() {
		g.Lock()
	} else {
		g.RLock()
	}
}

func (s *Stream[K, P]) Release(g graph.Graph[K, P]) {
	if s.Query.IsWrite() {
		g.Unlock()
	} else {
		g.RUnlock()
	}
}
