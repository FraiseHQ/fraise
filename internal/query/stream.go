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

	"github.com/RonsenbergVI/fraise/internal/graph"
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

func (s *Stream[K, P]) Commit(g graph.Graph[K, P]) {
	defer s.finish()
	defer s.release(g)

}

func (s *Stream[K, P]) Rollback(g graph.Graph[K, P]) {

	defer s.finish()
	defer s.release(g)

}

func (s *Stream[K, P]) Stage(g graph.Graph[K, P]) {
	s.acquire(g)

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
