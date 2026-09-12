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

package engine_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/internal/query"
	"github.com/FraiseHQ/fraise/pkg/db"
	"github.com/FraiseHQ/fraise/pkg/engine"
	"github.com/FraiseHQ/fraise/pkg/scheduler"
)

// newEngine builds an engine wired to a started store, mirroring how the server
// assembles the two.
func newEngine(t *testing.T, cfg *config.ConfigSet) *engine.Engine[uint64, float32] {
	t.Helper()
	hasher := hash.NewHasher[uint64](cfg)
	e := engine.NewEngine[uint64, float32](cfg, hasher)

	d, err := db.NewDB[uint64, float32](cfg)
	if err != nil {
		t.Fatalf("NewDB returned error: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("db.Start returned error: %v", err)
	}
	e.Scheduler.DB = d
	return e
}

// TestNewEngine checks that construction wires up the scheduler and its
// dependencies.
func TestNewEngine(t *testing.T) {
	e := engine.NewEngine[uint64, float32](config.New(), hash.NewHasher[uint64](config.New()))
	if e.Scheduler == nil {
		t.Error("NewEngine left Scheduler nil")
	}
	if e.Optimisations == nil {
		t.Error("NewEngine left Optimisations nil")
	}
}

// TestStartInitialisesCache checks that Start allocates the query cache so that
// Plan can memoise optimised queries.
func TestStartInitialisesCache(t *testing.T) {
	cfg := config.New()
	e := newEngine(t, cfg)
	if err := e.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer e.Stop()

	if e.Cache == nil {
		t.Fatal("Start did not initialise the cache")
	}
}

// TestStartReturnsCacheError checks that a cache-init failure is propagated
// rather than swallowed, and that no unusable cache is left behind — so a caller
// never brings up a live server backed by a nil cache.
func TestStartReturnsCacheError(t *testing.T) {
	cfg := config.New()
	cfg.Engine.CacheCapacity = 0 // NewLRUCache rejects a non-positive capacity
	e := newEngine(t, cfg)

	if err := e.Start(); !errors.Is(err, engine.ErrCacheInit) {
		t.Fatalf("Start with zero cache capacity = %v, want ErrCacheInit", err)
	}
	if e.Cache != nil {
		t.Error("Start assigned a cache despite the init failure")
	}
}

// TestApplyAfterStopReturnsShutdown checks that Apply surfaces the scheduler's
// ErrShutdown once the engine has stopped, so a handler learns the stream was
// never enqueued and does not wait on a completion that will never come.
func TestApplyAfterStopReturnsShutdown(t *testing.T) {
	cfg := config.New()
	e := newEngine(t, cfg)
	if err := e.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	q, _, err := query.Parse[uint64, float32]("recall@0 anna", nil, cfg)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	stream, err := e.Plan(q)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	e.Stop()

	if err := e.Apply(context.Background(), stream); !errors.Is(err, scheduler.ErrShutdown) {
		t.Fatalf("Apply after Stop = %v, want ErrShutdown", err)
	}
}

// TestPlanReturnsStream checks that Plan produces an executable stream for a
// parsed query.
func TestPlanReturnsStream(t *testing.T) {
	cfg := config.New()
	e := newEngine(t, cfg)
	if err := e.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer e.Stop()

	q, _, err := query.Parse[uint64, float32]("recall@0 anna", nil, cfg)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	stream, err := e.Plan(q)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if stream == nil {
		t.Fatal("Plan returned a nil stream")
	}
}

// TestPlanCachesQuery checks that planning the same query twice serves the
// second call from the cache: the cached, optimised query is returned rather
// than re-optimised, and both plans succeed.
func TestPlanCachesQuery(t *testing.T) {
	cfg := config.New()
	e := newEngine(t, cfg)
	if err := e.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer e.Stop()

	q, _, err := query.Parse[uint64, float32]("recall@0 anna topic:color", nil, cfg)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if _, err := e.Plan(q); err != nil {
		t.Fatalf("first Plan returned error: %v", err)
	}
	// The key must now be present in the cache.
	if _, ok := e.Cache.Get(q.Hash(e.Hasher)); !ok {
		t.Fatal("Plan did not cache the query")
	}
	if _, err := e.Plan(q); err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
}

// TestPlanCacheHitWithDuplicateKeywords guards #238: Dedupe.Optimise mutates the
// query in place, so Plan must hash after Optimise for both Put and Get. A fresh
// parse of the same duplicate-keyword recall must hit the entry stored by the
// first Plan (same cached query object), not miss forever under a raw-key lookup.
func TestPlanCacheHitWithDuplicateKeywords(t *testing.T) {
	cfg := config.New()
	e := newEngine(t, cfg)
	if err := e.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer e.Stop()

	q1, _, err := query.Parse[uint64, float32]("recall@0 foo foo top:5", nil, cfg)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if _, err := e.Plan(q1); err != nil {
		t.Fatalf("first Plan returned error: %v", err)
	}
	// q1 is now deduplicated in place; its hash is the cache key.
	cached, ok := e.Cache.Get(q1.Hash(e.Hasher))
	if !ok {
		t.Fatal("first Plan did not cache the optimised query")
	}

	q2, _, err := query.Parse[uint64, float32]("recall@0 foo foo top:5", nil, cfg)
	if err != nil {
		t.Fatalf("second Parse returned error: %v", err)
	}
	stream, err := e.Plan(q2)
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
	if stream.Query != cached {
		t.Fatal("second Plan missed the cache for a duplicate-keyword query; Put/Get keys disagree")
	}
}
