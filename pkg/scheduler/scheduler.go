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
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/query"

	"github.com/FraiseHQ/fraise/pkg/db"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// Scheduler executes planned query streams on a pool of workers fed by a
// bounded queue. A stream holds its graph's lock while it runs, so writes to
// one graph are serialised.
type Scheduler[K ~uint64, P float32 | float64] struct {
	Config *config.ConfigSet
	Queue  chan *query.Stream[K, P]
	DB     *db.DB[K, P]

	// quit is closed by Stop to signal shutdown: workers drain the queue and
	// exit, and Submit refuses new work. Stop closes quit rather than Queue,
	// because a Submit still sending on a closed channel would panic.
	quit chan struct{}

	// mu guards Queue and quit, which Start sets and Stop clears, so a Submit
	// racing Stop reads a consistent pair rather than a half-torn-down
	// scheduler.
	mu sync.RWMutex

	wg sync.WaitGroup
}

// NewScheduler returns a scheduler with no queue and no workers;
// [Scheduler.Start] allocates both. The scheduler does not own the database:
// the caller sets DB before Start, so streams have a store to run against.
func NewScheduler[K ~uint64, P float32 | float64](config *config.ConfigSet) *Scheduler[K, P] {
	s := &Scheduler[K, P]{
		Config: config,
	}
	return s
}

// Start allocates the queue and starts the worker pool.
func (s *Scheduler[K, P]) Start() error {
	s.mu.Lock()
	queue := make(chan *query.Stream[K, P], s.Config.Scheduler.BufferSize)
	quit := make(chan struct{})
	s.Queue = queue
	s.quit = quit
	s.mu.Unlock()

	for i := 0; i < s.Config.Scheduler.Workers; i++ {
		s.wg.Add(1)
		go s.worker(queue, quit)
	}
	logger.Info("Scheduler started",
		"workers", s.Config.Scheduler.Workers, "buffer", s.Config.Scheduler.BufferSize)
	return nil
}

// Stop signals shutdown, runs the streams already accepted and releases the
// queue. It is idempotent and safe on a scheduler that never started.
func (s *Scheduler[K, P]) Stop() {
	s.mu.Lock()
	queue, quit := s.Queue, s.quit
	if queue == nil {
		s.mu.Unlock()
		return
	}
	// Refuse new work; unblock any Submit parked on a full queue.
	close(quit)
	s.mu.Unlock()

	// Workers drain what they can, then exit.
	s.wg.Wait()

	// Drain any straggler that a Submit raced into the buffer as quit closed:
	// once the workers are gone this runs single-threaded, so an accepted write
	// is executed rather than silently dropped.
	for {
		select {
		case stream := <-queue:
			if err := s.execute(stream); err != nil {
				logger.Error("Failed to execute stream", "error", err)
			}
		default:
			s.mu.Lock()
			s.Queue = nil
			s.quit = nil
			s.mu.Unlock()
			logger.Info("Scheduler stopped")
			return
		}
	}
}

// worker executes streams (read or write) until shutdown. On quit it drains the
// streams already buffered before returning, so work accepted by Submit is not
// lost to a graceful Stop.
func (s *Scheduler[K, P]) worker(queue chan *query.Stream[K, P], quit chan struct{}) {
	defer s.wg.Done()
	for {
		select {
		case stream := <-queue:
			if err := s.execute(stream); err != nil {
				logger.Error("Failed to execute stream", "error", err)
			}
		case <-quit:
			for {
				select {
				case stream := <-queue:
					if err := s.execute(stream); err != nil {
						logger.Error("Failed to execute stream", "error", err)
					}
				default:
					return
				}
			}
		}
	}
}

// Submit enqueues a stream for execution. It blocks only until the queue has
// room, the enqueue timeout lapses, ctx ends or the scheduler shuts down. It
// returns ErrQueueFull on timeout, ErrEnqueueStream wrapping ctx's error if ctx
// ends first, and ErrShutdown if the scheduler is stopping, stopped or was
// never started, so a caller is never left blocked on a full or nil queue and
// can shed load instead.
func (s *Scheduler[K, P]) Submit(ctx context.Context, stream *query.Stream[K, P]) error {
	s.mu.RLock()
	queue, quit := s.Queue, s.quit
	s.mu.RUnlock()

	if queue == nil {
		return ErrShutdown
	}

	// Fast path: already shutting down, refuse before parking on the queue.
	select {
	case <-quit:
		return ErrShutdown
	default:
	}

	timeout := time.NewTimer(s.Config.Scheduler.EnqueueTimeout)
	defer timeout.Stop()

	select {
	case queue <- stream:
		return nil
	case <-quit:
		return ErrShutdown
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrEnqueueStream, ctx.Err())
	case <-timeout.C:
		return ErrQueueFull
	}
}

// execute runs stream against its graph under the graph's lock and records any
// error on the stream.
func (s *Scheduler[K, P]) execute(stream *query.Stream[K, P]) error {

	// Always signal completion, even on an early error, so a caller waiting on
	// Done() never blocks forever (e.g. a request for an out-of-range graph).
	defer stream.Finish()

	g, err := s.DB.Select(stream.Query.GetGraphID())

	if err != nil {
		stream.Err = err
		return err
	}

	defer stream.Release(g)

	stream.Acquire(g)

	// Commit runs in place against the live graph: Acquire holds the
	// exclusive lock for writes, so no staging copy is needed and a write
	// costs O(fact), not O(graph). The commit error is wrapped rather than
	// replaced, so the HTTP boundary can still tell a client fault (a
	// vector-dimension mismatch) from an internal one.
	if err := stream.Commit(g); err != nil {
		err = fmt.Errorf("%w: %w", ErrStreamCommit, err)
		stream.Err = err
		return err
	}
	return nil
}
