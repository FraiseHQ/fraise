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

// TestPlanCacheKeysAgreeForDuplicateTerms checks that a query with repeated
// keywords, topics or entities is cached under the hash of the *optimised*
// query, so a second identical request hits rather than storing an unreachable
// LRU entry.
func TestPlanCacheKeysAgreeForDuplicateTerms(t *testing.T) {
	cases := []struct {
		name  string
		dup   string
		canon string
	}{
		{"keywords", "recall@0 foo foo top:5", "recall@0 foo top:5"},
		{"topics", "recall@0 foo topic:color topic:color", "recall@0 foo topic:color"},
		{"entities", "recall@0 foo entity:alice entity:alice", "recall@0 foo entity:alice"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.New()
			e := newEngine(t, cfg)
			if err := e.Start(); err != nil {
				t.Fatalf("Start returned error: %v", err)
			}
			defer e.Stop()

			first, _, err := query.Parse[uint64, float32](tc.dup, nil, cfg)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.dup, err)
			}
			rawKey := first.Hash(e.Hasher)

			if _, err := e.Plan(first); err != nil {
				t.Fatalf("first Plan returned error: %v", err)
			}
			optKey := first.Hash(e.Hasher)
			if rawKey == optKey {
				t.Fatal("duplicate terms did not change the cache key after Optimise")
			}
			if _, ok := e.Cache.Get(rawKey); ok {
				t.Fatal("cache stored under the raw pre-dedupe key")
			}
			if _, ok := e.Cache.Get(optKey); !ok {
				t.Fatal("cache did not store under the optimised key")
			}
			if got := e.Cache.Len(); got != 1 {
				t.Fatalf("cache Len after first Plan = %d, want 1", got)
			}

			second, _, err := query.Parse[uint64, float32](tc.dup, nil, cfg)
			if err != nil {
				t.Fatalf("Parse(%q) second returned error: %v", tc.dup, err)
			}
			if _, err := e.Plan(second); err != nil {
				t.Fatalf("second Plan returned error: %v", err)
			}
			if got := e.Cache.Len(); got != 1 {
				t.Fatalf("second identical duplicate query missed the cache: Len=%d, want 1", got)
			}

			canon, _, err := query.Parse[uint64, float32](tc.canon, nil, cfg)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.canon, err)
			}
			if _, err := e.Plan(canon); err != nil {
				t.Fatalf("canonical Plan returned error: %v", err)
			}
			if got := e.Cache.Len(); got != 1 {
				t.Fatalf("canonical form missed the duplicate query's cache entry: Len=%d, want 1", got)
			}
		})
	}
}
