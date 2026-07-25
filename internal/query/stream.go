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
	"sync"
	"time"

	"github.com/RonsenbergVI/fraise/internal/graph"
)

// data structure representing a stream: language of the scheduler
// A stream is the language for the engine to the worker
// Streams can be read-only or read and write.
type Stream[K comparable, P float32 | float64] struct {
	Query  Query[K, P]
	Result *QueryResult[K, P]
	Err    error

	staging graph.Graph[K, P]
	done    chan struct{}
	once    sync.Once
}

// NewStream returns a stream ready to be scheduled for q: Done() blocks until
// the scheduler commits or rolls the stream back.
func NewStream[K comparable, P float32 | float64](q Query[K, P]) *Stream[K, P] {
	return &Stream[K, P]{Query: q, done: make(chan struct{})}
}

func (s *Stream[K, P]) Commit(g graph.Graph[K, P]) error {
	defer s.finish()
	defer s.release(g)

	if s.staging == nil {
		return ErrStreamClosed
	}

	// Write stream
	if s.Query.IsWrite() {
		g.MergeFrom(s.staging)
		s.staging = nil
		return nil
	}

	// Read stream
	recall := s.Query.(*Recall[K, P])
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

	s.staging = nil
	s.Result = &r
	return nil
}

func (s *Stream[K, P]) Rollback(g graph.Graph[K, P]) error {

	defer s.finish()
	defer s.release(g)

	if s.staging == nil {
		return ErrStreamClosed
	}

	if s.Query.IsWrite() {
		s.staging = nil
	}

	s.staging = nil
	return nil
}

func (s *Stream[K, P]) Stage(g graph.Graph[K, P]) error {
	s.acquire(g)

	s.staging = g.Copy()
	if s.Query.IsWrite() {
		// remember logic
	}
	return nil
}

func (s *Stream[K, P]) GraphID() uint8 {
	return s.Query.GetGraphID()
}

func (s *Stream[K, P]) Done() <-chan struct{} {
	return s.done
}

func (s *Stream[K, P]) finish() {
	s.once.Do(func() { close(s.done) })
}

func (s *Stream[K, P]) acquire(g graph.Graph[K, P]) {
	if s.Query.IsWrite() {
		g.Lock()
	} else {
		g.RLock()
	}
}

func (s *Stream[K, P]) release(g graph.Graph[K, P]) {
	if s.Query.IsWrite() {
		g.Unlock()
	} else {
		g.RUnlock()
	}
}
