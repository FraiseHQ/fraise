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

	// quit is closed by Stop to signal shutdown. Workers then drain the queue
	// and exit, and Submit refuses new work. Stop closes quit, not Queue,
	// because a Submit still sending on a closed channel would panic.
	quit chan struct{}

	// mu guards Queue and quit. Start sets them and Stop clears them. A Submit
	// racing Stop reads both under mu, so it sees a consistent pair, never a
	// half-torn-down scheduler.
	mu sync.RWMutex

	// submits counts the Submits that passed the shutdown check. Stop waits
	// for them before its final drain, so none can enqueue a stream after it.
	submits sync.WaitGroup

	// stopping serialises Stop. A second call waits for the first to finish,
	// then finds nothing left to stop.
	stopping sync.Mutex

	wg sync.WaitGroup
}

// NewScheduler returns a scheduler for config. [Scheduler.Start] allocates
// its queue and workers. The caller attaches the database as DB before Start.
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
	s.stopping.Lock()
	defer s.stopping.Unlock()

	s.mu.Lock()
	queue, quit := s.Queue, s.quit
	if queue == nil {
		s.mu.Unlock()
		return
	}
	// Closing quit refuses new work and unblocks any Submit parked on a full
	// queue.
	close(quit)
	s.mu.Unlock()

	// Every Submit counted before quit closed now either enqueues its stream
	// or returns ErrShutdown.
	s.submits.Wait()

	// Workers drain what they can, then exit.
	s.wg.Wait()

	// A Submit may have enqueued a stream after the workers' last drain. Run
	// any such straggler here, so an accepted write is executed, not silently
	// dropped. The workers are gone, so this runs single-threaded.
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

// worker executes streams, reads and writes, until shutdown. On quit it first
// drains the streams already buffered, so a graceful Stop does not lose work
// that Submit accepted.
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
// returns:
//   - ErrQueueFull when the enqueue timeout lapses,
//   - ErrEnqueueStream, wrapping ctx's error, when ctx ends first,
//   - ErrShutdown when the scheduler is stopping, stopped or never started.
//
// So a caller is never left blocked on a full or nil queue, and can shed load
// instead.
func (s *Scheduler[K, P]) Submit(ctx context.Context, stream *query.Stream[K, P]) error {
	s.mu.RLock()
	queue, quit := s.Queue, s.quit
	if queue == nil {
		s.mu.RUnlock()
		return ErrShutdown
	}

	// If shutdown has begun, refuse before parking on the queue.
	select {
	case <-quit:
		s.mu.RUnlock()
		return ErrShutdown
	default:
	}

	// Count this Submit while the read lock is held. Stop closes quit under
	// the write lock, so it waits for every Submit counted here before its
	// final drain.
	s.submits.Add(1)
	s.mu.RUnlock()
	defer s.submits.Done()

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

	// Signal completion even on an early error, such as an out-of-range graph,
	// so a caller waiting on Done() never blocks forever.
	defer stream.Finish()

	g, err := s.DB.Select(stream.GraphID())

	if err != nil {
		stream.Err = err
		return err
	}

	defer stream.Release(g)

	stream.Acquire(g)

	// Commit runs in place on the live graph. Acquire holds the exclusive
	// lock for a write, so no staging copy is needed, and a write costs
	// O(fact), not O(graph). The commit error is wrapped, not replaced, so the
	// HTTP boundary can still tell a client fault from an internal one.
	if err := stream.Commit(g); err != nil {
		err = fmt.Errorf("%w: %w", ErrStreamCommit, err)
		stream.Err = err
		return err
	}
	return nil
}
