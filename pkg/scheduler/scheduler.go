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

package scheduler

import (
	"fmt"
	"sync"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/query"

	"github.com/RonsenbergVI/fraise/pkg/db"
	"github.com/RonsenbergVI/fraise/pkg/logger"
)

// The scheduler decides when to run a stream.
// one concurrent write stream is supported at a time
// The scheduler decides when to wait for a write operation to finish
type Scheduler[K ~uint64, P float32 | float64] struct {
	Config *config.ConfigSet
	Queue  chan *query.Stream[K, P]
	DB     *db.DB[K, P]

	writeInFlight bool
	wg            sync.WaitGroup
}

func NewScheduler[K ~uint64, P float32 | float64](config *config.ConfigSet) *Scheduler[K, P] {
	s := &Scheduler[K, P]{
		Config:        config,
		writeInFlight: false,
	}
	return s
}

// Starts scheduler: allocates memory for queue and initializes workers
func (s *Scheduler[K, P]) Start() error {
	s.Queue = make(chan *query.Stream[K, P], s.Config.Scheduler.BufferSize)
	for i := 0; i < s.Config.Scheduler.Workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return nil
}

// Stops scheduler: frees resources and stops coroutines
func (s *Scheduler[K, P]) Stop() {
	if s.Queue != nil {
		close(s.Queue)
		s.wg.Wait()
		s.Queue = nil
	}
}

// worker executes stream (read or write in database)
func (s *Scheduler[K, P]) worker() {
	defer s.wg.Done()
	for stream := range s.Queue {
		err := s.execute(stream)
		if err != nil {
			logger.Error("Failed to execute stream", "error:", err)
		}
	}

}

// Submits new stream in queue to be executed
func (s *Scheduler[K, P]) Submit(stream *query.Stream[K, P]) {
	s.Queue <- stream
}

// Executes stream
func (s *Scheduler[K, P]) execute(stream *query.Stream[K, P]) error {

	g, err := s.DB.Select(stream.Query.GetGraphID())

	if err != nil {
		return err
	}

	defer stream.Release(g)
	defer stream.Finish()

	stream.Acquire(g)

	stg, err := stream.Stage(g)

	fmt.Println(stream.Result)
	fmt.Println(g.PredecessorMap())
	fmt.Println(g.AdjacencyMap())
	fmt.Println(g.Nodes())

	if err != nil {
		stream.Rollback(g)
		return ErrStreamCommit
	}
	g.MergeFrom(stg)
	return nil
}
