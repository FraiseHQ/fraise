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

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/query"

	"github.com/RonsenbergVI/fraise/pkg/db"
	"github.com/RonsenbergVI/fraise/pkg/logger"
)

// The scheduler executes planned query streams on a pool of workers fed by a
// bounded queue; per-graph locking during execution serialises writes.
type Scheduler[K ~uint64, P float32 | float64] struct {
	Config *config.ConfigSet
	Queue  chan *query.Stream[K, P]
	DB     *db.DB[K, P]

	// quit is closed by Stop to signal shutdown. Workers drain the queue and
	// exit when it closes; Submit selects on it to refuse new work. It is never
	// the Queue channel itself, so Stop never closes a channel a Submit might
	// still be sending on (which would panic).
	quit chan struct{}

	// mu guards the Queue/quit fields against the Stop that nils them, so a
	// Submit racing Stop reads a consistent pair instead of a half-torn-down
	// scheduler (a send on a nil channel would otherwise hang forever).
	mu sync.RWMutex

	wg sync.WaitGroup
}

func NewScheduler[K ~uint64, P float32 | float64](config *config.ConfigSet) *Scheduler[K, P] {
	s := &Scheduler[K, P]{
		Config: config,
	}
	return s
}

// Starts scheduler: allocates memory for queue and initializes workers
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

// Stops scheduler: signals shutdown, drains accepted work, and releases the
// queue. It is idempotent and safe on a scheduler that never started. The queue
// is never closed — shutdown is signalled by closing quit — so a Submit racing
// Stop can never send on a closed channel.
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

// Submit enqueues a stream for execution. It is bounded and context-aware: it
// blocks only until the queue has room, the configured enqueue timeout lapses,
// the context is cancelled, or the scheduler shuts down. It returns ErrShutdown
// if the scheduler is not running (never started, or stopped) and ErrQueueFull
// if the queue stays saturated past the timeout, so a caller is never left
// blocked on a full, nil, or closed queue and can shed load instead.
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

// Executes stream
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

	// Commit executes in place against the live graph: Acquire already holds
	// the exclusive lock for writes, so no staging copy is needed and the write
	// costs O(fact) rather than O(graph) (copy + merge-back did the latter).
	//
	// The commit error is wrapped, not replaced: the cause must survive so the
	// HTTP boundary can tell a client fault (a vector-dimension mismatch) from
	// an internal one — collapsing it here turned every rejected write into an
	// opaque 500.
	if err := stream.Commit(g); err != nil {
		err = fmt.Errorf("%w: %w", ErrStreamCommit, err)
		stream.Err = err
		return err
	}
	return nil
}
