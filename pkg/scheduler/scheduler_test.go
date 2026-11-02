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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/index"
	"github.com/FraiseHQ/fraise/internal/query"
	"github.com/FraiseHQ/fraise/pkg/db"
	"github.com/FraiseHQ/fraise/pkg/scheduler"
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

// TestStartStop checks the scheduler opens its read queue and one write lane
// per graph, and releases them on Stop.
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
	if got, want := len(s.Lanes), s.DB.NumGraphs(); got != want {
		t.Fatalf("Start opened %d lanes, want one per graph (%d)", got, want)
	}
	s.Stop()
	if s.Queue != nil {
		t.Error("Stop did not release the queue")
	}
	if s.Lanes != nil {
		t.Error("Stop did not release the lanes")
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

// TestSubmitOutOfRangeGraph checks that a stream for a graph past the
// allocated range still closes Done and records the error. A caller waiting
// on Done must never block forever.
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
// the scheduler's own sentinel and the underlying cause on the stream. The
// HTTP boundary classifies errors with errors.Is. If the scheduler collapsed
// the chain to a bare ErrStreamCommit, a client fault (here a vector-dimension
// mismatch) would surface as an internal 500.
func TestSubmitCommitErrorPreservesCause(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop()

	// The first vector fixes graph 0's dimension at 3. The second, at 4, can
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

// TestSubmitCancelledContextDoesNotBlock checks that Submit honours its
// context. With no worker to drain a full queue, a submit whose context is
// already cancelled returns an error promptly instead of blocking forever.
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

	// The queue is full. A cancelled context must unblock the send.
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
// without bound. With no worker to drain a full queue and a live context, the
// send gives up after the enqueue timeout and reports ErrQueueFull.
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

// TestWriteBlockedOnItsGraphLeavesReadsRunning checks that reads do not queue
// behind writes. With one read worker and graph 0's lock held, a write to
// graph 0 parks on the lock. A write that occupied the worker would leave the
// read of graph 1 waiting for as long as graph 0 stays locked.
func TestWriteBlockedOnItsGraphLeavesReadsRunning(t *testing.T) {
	cfg := config.New()
	cfg.Scheduler.Workers = 1
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop()

	g, err := s.DB.Select(0)
	if err != nil {
		t.Fatalf("Select(0) returned error: %v", err)
	}
	g.Lock()
	write := planStream(t, cfg, "remember@0 'the gate is shut'", nil)
	if err := s.Submit(context.Background(), write); err != nil {
		g.Unlock()
		t.Fatalf("Submit(write) returned error: %v", err)
	}

	read := planStream(t, cfg, "recall@1 gate", nil)
	if err := s.Submit(context.Background(), read); err != nil {
		g.Unlock()
		t.Fatalf("Submit(read) returned error: %v", err)
	}
	select {
	case <-read.Done():
	case <-time.After(2 * time.Second):
		t.Error("the read of graph 1 waited on graph 0's lock")
	}
	g.Unlock()

	select {
	case <-write.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the write did not run once its graph was unlocked")
	}
}

// TestWriteRunsWithoutReadWorkers checks that writes do not queue behind
// reads. With no read worker at all, so reads only wait, a write still runs on
// its graph's own writer.
func TestWriteRunsWithoutReadWorkers(t *testing.T) {
	cfg := config.New()
	cfg.Scheduler.Workers = 0
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop()

	if err := s.Submit(context.Background(), planStream(t, cfg, "recall@0 anna", nil)); err != nil {
		t.Fatalf("Submit(read) returned error: %v", err)
	}
	write := planStream(t, cfg, "remember@0 'the sky is blue'", nil)
	if err := s.Submit(context.Background(), write); err != nil {
		t.Fatalf("Submit(write) returned error: %v", err)
	}
	select {
	case <-write.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the write waited for a read worker")
	}
	if write.Err != nil {
		t.Errorf("write.Err = %v, want nil", write.Err)
	}
}

// TestWritesToOneGraphRunInSubmitOrder checks the lane's ordering contract.
// Writes one caller submits to a graph, each after the previous Submit
// returned, run in that order. They queue behind graph 0's held lock, so all
// are buffered at once and nothing but the lane orders them. The facts'
// timestamps record when each ran.
func TestWritesToOneGraphRunInSubmitOrder(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop()

	g, err := s.DB.Select(0)
	if err != nil {
		t.Fatalf("Select(0) returned error: %v", err)
	}
	values := []string{"first", "second", "third", "fourth", "fifth", "sixth"}
	streams := make([]*query.Stream[uint64, float32], len(values))
	g.Lock()
	for i, v := range values {
		streams[i] = planStream(t, cfg, "remember@0 '"+v+"'", nil)
		if err := s.Submit(context.Background(), streams[i]); err != nil {
			g.Unlock()
			t.Fatalf("Submit(%q) returned error: %v", v, err)
		}
	}
	g.Unlock()

	for i, stream := range streams {
		select {
		case <-stream.Done():
		case <-time.After(2 * time.Second):
			t.Fatalf("write %q did not finish within the timeout", values[i])
		}
	}

	g.RLock()
	defer g.RUnlock()
	var last time.Time
	for _, v := range values {
		fact := graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: v}, Hasher: g.GetHasher()}
		node := g.Get(fact.Key())
		if node == nil {
			t.Fatalf("fact %q was not stored", v)
		}
		if ran := node.GetTimestamp(); !ran.After(last) {
			t.Errorf("fact %q ran at %v, not after the write submitted before it (%v)", v, ran, last)
		}
		last = node.GetTimestamp()
	}
}

// TestStopDrainsBufferedWrite checks that a graceful Stop executes work already
// accepted into a lane rather than dropping it. Graph 0's held lock parks its
// writer on the first write and keeps the second buffered while Stop begins.
// Stop must not return until both have run.
func TestStopDrainsBufferedWrite(t *testing.T) {
	cfg := config.New()
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	g, err := s.DB.Select(0)
	if err != nil {
		t.Fatalf("Select(0) returned error: %v", err)
	}
	g.Lock()
	streams := []*query.Stream[uint64, float32]{
		planStream(t, cfg, "remember@0 'the sky is blue' topic:color", nil),
		planStream(t, cfg, "remember@0 'the grass is green' topic:color", nil),
	}
	for _, stream := range streams {
		if err := s.Submit(context.Background(), stream); err != nil {
			g.Unlock()
			t.Fatalf("Submit returned error: %v", err)
		}
	}

	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned while an accepted write was still waiting to run")
	case <-time.After(50 * time.Millisecond):
	}
	g.Unlock()
	<-stopped

	for i, stream := range streams {
		select {
		case <-stream.Done():
		default:
			t.Fatalf("Stop did not execute buffered write %d (Done never closed)", i)
		}
		if stream.Err != nil {
			t.Errorf("write %d: stream.Err = %v, want nil", i, stream.Err)
		}
	}

	// The writes must land in the live graph. Commits run in place, not on a
	// copy.
	g.RLock()
	nodes := g.Stats().Nodes
	g.RUnlock()
	if nodes == 0 {
		t.Error("committed writes left the live graph empty, want the facts stored in place")
	}
}

// TestConcurrentSubmitStop races many submits, reads and writes across the
// graphs, against a Stop. None may panic (send on a closed queue) or hang
// (send on a nil queue). A submit may be refused, but a stream it accepted
// must have run once Stop returns, or its caller waits on a Done that never
// closes. The race is narrow, so the test runs it many times. Run with -race.
func TestConcurrentSubmitStop(t *testing.T) {
	cfg := config.New()
	for round := range 1000 {
		s := scheduler.NewScheduler[uint64, float32](cfg)
		s.DB = newStartedDB(t, cfg)

		if err := s.Start(); err != nil {
			t.Fatalf("Start returned error: %v", err)
		}

		const n = 64
		streams := make([]*query.Stream[uint64, float32], n)
		for i := range streams {
			q := fmt.Sprintf("recall@%d anna", i%4)
			if i%2 == 1 {
				q = fmt.Sprintf("remember@%d 'anna %d'", i%4, i)
			}
			streams[i] = planStream(t, cfg, q, nil)
		}
		errs := make([]error, n)

		var wg sync.WaitGroup
		for i := range streams {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = s.Submit(context.Background(), streams[i])
			}()
		}

		s.Stop()
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				continue
			}
			select {
			case <-streams[i].Done():
			default:
				t.Fatalf("round %d: Submit accepted stream %d, but Stop returned without running it", round, i)
			}
		}
	}
}

// TestConcurrentStops checks that Stop is safe to call from several goroutines
// at once. None closes the shutdown signal a second time, and each returns
// only once the accepted stream has run. The stream is a read and there is no
// read worker, so only Stop's drain can run it.
func TestConcurrentStops(t *testing.T) {
	cfg := config.New()
	cfg.Scheduler.Workers = 0
	s := scheduler.NewScheduler[uint64, float32](cfg)
	s.DB = newStartedDB(t, cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	stream := planStream(t, cfg, "recall@0 tide", nil)
	if err := s.Submit(context.Background(), stream); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Stop()
			select {
			case <-stream.Done():
			default:
				t.Error("Stop returned before the accepted stream ran")
			}
		}()
	}
	wg.Wait()
}
