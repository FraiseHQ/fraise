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

	"github.com/RonsenbergVI/fraise/internal/graph"
	"github.com/RonsenbergVI/fraise/pkg/logger"
)

// data structure representing a stream: language of the scheduler
// A stream is the language for the engine to the worker
// Streams can be read-only or read and write.
type Stream[K comparable, P float32 | float64] struct {
	Query  Query[K, P]
	Result *QueryResult[K, P]
	Err    error

	done chan struct{}
	once sync.Once
}

// NewStream returns a stream ready to be scheduled for q: Done() blocks until
// the scheduler commits or rolls the stream back.
func NewStream[K comparable, P float32 | float64](q Query[K, P]) *Stream[K, P] {
	return &Stream[K, P]{Query: q, done: make(chan struct{})}
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

			// Store the entity node itself: anchored recalls resolve a fact's
			// tags through its neighbours in idToNodes, so an edge to a
			// never-stored node makes every entity: filter miss. Entities are
			// shared across facts (keyed by value), so an already-stored node
			// is the normal upsert case, not an error.
			if err := g.Set(&entity); err != nil && !errors.Is(err, graph.ErrNodeAlreadyExists) {
				logger.Error("Failed to store entity node", "entity", e, "error", err)
				return fmt.Errorf("storing entity %q: %w", e, err)
			}

			mentions := graph.Mentions[K]{NodeAttributes: graph.NodeAttributes{
				Timestamp: time.Now(),
			},
				Fact:        &fact,
				NamedEntity: &entity,
				Hasher:      g.GetHasher(),
			}

			g.Set(mentions)
		}

		for _, t := range remember.Topics {

			topic := graph.Topic[K]{NodeAttributes: graph.NodeAttributes{
				Value:     t,
				Timestamp: time.Now(),
			},
				Hasher: g.GetHasher(),
			}

			// Same as entities above: the topic node must exist for topic:
			// filters to resolve; re-storing a shared topic is the upsert case.
			if err := g.Set(&topic); err != nil && !errors.Is(err, graph.ErrNodeAlreadyExists) {
				logger.Error("Failed to store topic node", "topic", t, "error", err)
				return fmt.Errorf("storing topic %q: %w", t, err)
			}

			about := graph.IsAbout[K]{NodeAttributes: graph.NodeAttributes{
				Timestamp: time.Now(),
			},
				Fact:   &fact,
				Topic:  &topic,
				Hasher: g.GetHasher(),
			}

			g.Set(about)
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
	nodes, scores := g.Search(
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
	for i := 0; i < n; i++ {
		r.Hits[i].Node = nodes[i]
		r.Hits[i].Score = scores[i]
	}

	s.Result = &r
	logger.Debug("Read stream committed", "hits", n)
	return nil
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
