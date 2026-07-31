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
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
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

// planStream parses q and returns the executable stream the scheduler runs.
func planStream(t *testing.T, cfg *config.ConfigSet, q string) *query.Stream[uint64, float32] {
	t.Helper()
	parsed, err := query.Parse[uint64, float32](q, nil, cfg)
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

	stream := planStream(t, cfg, "recall@0 anna")
	s.Submit(stream)

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
	stream := planStream(t, cfg, "recall@0 anna")
	stream.Query.SetGraphID(uint8(s.DB.NumGraphs()))
	s.Submit(stream)

	select {
	case <-stream.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("out-of-range stream did not finish within the timeout")
	}

	if stream.Err == nil {
		t.Error("stream.Err = nil, want an out-of-range error")
	}
}
