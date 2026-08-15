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

package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/index"
	"github.com/RonsenbergVI/fraise/internal/query"
	"github.com/RonsenbergVI/fraise/pkg/db"
	"github.com/RonsenbergVI/fraise/pkg/scheduler"
)

// newStartedDB builds a store with populated graphs, ready to back a scheduler.
func newStartedDB(t *testing.T, cfg *config.ConfigSet) *db.DB[uint64, float32] {
	t.Helper()
	d, err := db.NewDB[uint64, float32](cfg)
	if err != nil {
		t.Fatalf("NewDB returned error: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("db.Start returned error: %v", err)
	}
	return d
}

// planStream parses q, binding any vector placeholders from params, and
// returns the executable stream the scheduler runs.
func planStream(t *testing.T, cfg *config.ConfigSet, q string, params map[string][]float32) *query.Stream[uint64, float32] {
	t.Helper()
	parsed, _, err := query.Parse[uint64, float32](q, params, cfg)
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", q, err)
	}
	stream, err := parsed.Plan(cfg)
	if err != nil {
		t.Fatalf("Plan(%q) returned error: %v", q, err)
	}
	return stream
}

// TestStartStop checks the scheduler starts its worker pool and stops cleanly.
func TestStartStop(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if s.Queue == nil {
		t.Fatal("Start did not allocate the queue")
	}
	s.Stop()
	if s.Queue != nil {
		t.Error("Stop did not release the queue")
	}
}

// TestStopWithoutStart checks that stopping a scheduler that never started is a
// safe no-op rather than a panic.
func TestStopWithoutStart(t *testing.T) {
	s := scheduler.NewScheduler[uint64, float32](config.New())
	s.Stop() // must not panic on a nil queue.
}

// TestSubmitExecutesReadStream checks that a submitted read stream is executed
// by a worker and signals completion via Done without an error.
func TestSubmitExecutesReadStream(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop()

	stream := planStream(t, cfg, "recall@0 anna", nil)
	if err := s.Submit(context.Background(), stream); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	select {
	case <-stream.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not finish within the timeout")
	}

	if stream.Err != nil {
		t.Errorf("stream.Err = %v, want nil", stream.Err)
	}
	if stream.Result == nil {
		t.Error("stream.Result = nil, want a result set")
	}
}

// TestSubmitOutOfRangeGraph checks that a stream targeting a graph past the
// allocated range still signals completion (Done closes) and records the error,
// so a caller waiting on Done never blocks forever.
func TestSubmitOutOfRangeGraph(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop()

	// Selector past the last valid graph index.
	stream := planStream(t, cfg, "recall@0 anna", nil)
	stream.Query.SetGraphID(uint8(s.DB.NumGraphs()))
	if err := s.Submit(context.Background(), stream); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	select {
	case <-stream.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("out-of-range stream did not finish within the timeout")
	}

	if stream.Err == nil {
		t.Error("stream.Err = nil, want an out-of-range error")
	}
}

// TestSubmitCommitErrorPreservesCause checks that a failed commit records both
// the scheduler's own sentinel and the underlying cause on the stream: the HTTP
// boundary classifies errors with errors.Is, so if the scheduler collapsed the
// chain to a bare ErrStreamCommit a client fault (here a vector-dimension
// mismatch) would surface as an internal 500.
func TestSubmitCommitErrorPreservesCause(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop()

	// The first vector fixes graph 0's dimension at 3; the second, at 4, can
	// only fail at commit time.
	for i, q := range []struct {
		query  string
		params map[string][]float32
	}{
		{"remember@0 'vec one' vec:$v", map[string][]float32{"v": {1, 2, 3}}},
		{"remember@0 'vec two' vec:$v", map[string][]float32{"v": {1, 2, 3, 4}}},
	} {
		stream := planStream(t, cfg, q.query, q.params)
		if err := s.Submit(context.Background(), stream); err != nil {
			t.Fatalf("Submit(%q) returned error: %v", q.query, err)
		}
		select {
		case <-stream.Done():
		case <-time.After(2 * time.Second):
			t.Fatalf("stream %d did not finish within the timeout", i)
		}

		if i == 0 {
			if stream.Err != nil {
				t.Fatalf("first write stream.Err = %v, want nil", stream.Err)
			}
			continue
		}
		if !errors.Is(stream.Err, scheduler.ErrStreamCommit) {
			t.Errorf("stream.Err = %v, want it to wrap ErrStreamCommit", stream.Err)
		}
		if !errors.Is(stream.Err, index.ErrInvalidDimension) {
			t.Errorf("stream.Err = %v, want it to preserve ErrInvalidDimension", stream.Err)
		}
	}
}

// TestSubmitNeverStartedReturnsShutdown checks that submitting to a scheduler
// that never started returns ErrShutdown instead of hanging on a nil queue.
func TestSubmitNeverStartedReturnsShutdown(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	stream := planStream(t, cfg, "recall@0 anna", nil)
	if err := s.Submit(context.Background(), stream); !errors.Is(err, scheduler.ErrShutdown) {
		t.Fatalf("Submit before Start = %v, want ErrShutdown", err)
	}
}

// TestSubmitAfterStopReturnsShutdown checks that submitting after Stop returns
// ErrShutdown rather than panicking on a closed queue or hanging on a nil one.
func TestSubmitAfterStopReturnsShutdown(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	s.Stop()

	stream := planStream(t, cfg, "recall@0 anna", nil)
	if err := s.Submit(context.Background(), stream); !errors.Is(err, scheduler.ErrShutdown) {
		t.Fatalf("Submit after Stop = %v, want ErrShutdown", err)
	}
}

// TestSubmitCancelledContextDoesNotBlock checks that Submit is context-aware:
// with no worker to drain a full queue, a submit whose context is already
// cancelled returns promptly with an error rather than blocking forever.
func TestSubmitCancelledContextDoesNotBlock(t *testing.T) {
	cfg := config.New()
	cfg.Scheduler.Workers = 0 // no drain, so the buffer stays full
	cfg.Scheduler.BufferSize = 1
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop() // drains the buffered stream

	// Fill the single buffer slot.
	if err := s.Submit(context.Background(), planStream(t, cfg, "recall@0 anna", nil)); err != nil {
		t.Fatalf("first Submit returned error: %v", err)
	}

	// The queue is now full; a cancelled context must unblock the send.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- s.Submit(ctx, planStream(t, cfg, "recall@0 anna", nil)) }()

	select {
	case err := <-done:
		if !errors.Is(err, scheduler.ErrEnqueueStream) {
			t.Fatalf("Submit with cancelled context = %v, want ErrEnqueueStream", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Submit did not return after its context was cancelled")
	}
}

// TestSubmitFullQueueTimesOut checks that Submit sheds load instead of blocking
// without bound: with no worker to drain a full queue and a live context, the
// send gives up after the configured enqueue timeout and reports ErrQueueFull.
func TestSubmitFullQueueTimesOut(t *testing.T) {
	cfg := config.New()
	cfg.Scheduler.Workers = 0 // no drain, so the buffer stays full
	cfg.Scheduler.BufferSize = 1
	cfg.Scheduler.EnqueueTimeout = 50 * time.Millisecond
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop() // drains the buffered stream

	// Fill the single buffer slot.
	if err := s.Submit(context.Background(), planStream(t, cfg, "recall@0 anna", nil)); err != nil {
		t.Fatalf("first Submit returned error: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- s.Submit(context.Background(), planStream(t, cfg, "recall@0 anna", nil)) }()

	select {
	case err := <-done:
		if !errors.Is(err, scheduler.ErrQueueFull) {
			t.Fatalf("Submit on a saturated queue = %v, want ErrQueueFull", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Submit did not give up on a saturated queue within the enqueue timeout")
	}
}

// TestStopDrainsBufferedWrite checks that a graceful Stop executes work already
// accepted into the buffer rather than dropping it: with no worker running, the
// only chance for the buffered write to run is Stop's drain.
func TestStopDrainsBufferedWrite(t *testing.T) {
	cfg := config.New()
	cfg.Scheduler.Workers = 0
	cfg.Scheduler.BufferSize = 4
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	stream := planStream(t, cfg, "remember@0 'the sky is blue' topic:color", nil)
	if err := s.Submit(context.Background(), stream); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	s.Stop() // must drain and execute the buffered write

	select {
	case <-stream.Done():
	default:
		t.Fatal("Stop did not execute the buffered stream (Done never closed)")
	}
	if stream.Err != nil {
		t.Errorf("stream.Err = %v, want nil", stream.Err)
	}

	// The write must have landed in the live graph — commits are in place, so
	// a fact that only ever reached a discarded staging copy is a regression.
	g, err := s.DB.Select(0)
	if err != nil {
		t.Fatalf("Select(0) returned error: %v", err)
	}
	g.RLock()
	nodes := g.Stats().Nodes
	g.RUnlock()
	if nodes == 0 {
		t.Error("committed write left the live graph empty, want the fact stored in place")
	}
}

// TestConcurrentSubmitStop is a race-detector guard: many submits racing a Stop
// must never panic (send on a closed queue) or hang (send on a nil queue). Any
// per-submit outcome is acceptable; the invariant is that the scheduler tears
// down cleanly. Run with -race.
func TestConcurrentSubmitStop(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	const n = 64
	streams := make([]*query.Stream[uint64, float32], n)
	for i := range streams {
		streams[i] = planStream(t, cfg, "recall@0 anna", nil)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(st *query.Stream[uint64, float32]) {
			defer wg.Done()
			_ = s.Submit(context.Background(), st)
		}(streams[i])
	}

	s.Stop()
	wg.Wait()
}
