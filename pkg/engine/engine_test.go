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
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/graph"
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

// TestNewEngine checks that construction wires up the scheduler and the
// optimisation pipeline.
func TestNewEngine(t *testing.T) {
	e := engine.NewEngine[uint64, float32](config.New(), hash.NewHasher[uint64](config.New()))
	if e.Scheduler == nil {
		t.Error("NewEngine left Scheduler nil")
	}
	if e.Optimisations == nil {
		t.Error("NewEngine left Optimisations nil")
	}
}

// TestStartInitialisesCache checks that Start allocates the plan cache, so Plan
// can memoise optimised queries.
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

// TestStartReturnsCacheError checks that Start returns a cache-init failure
// rather than swallowing it, and leaves no unusable cache behind. A caller then
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

// TestApplyAfterStopReturnsShutdown checks that Apply returns the scheduler's
// ErrShutdown once the engine has stopped. A handler then learns the stream was
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

// TestPlanCachesQuery checks that Plan caches the query under its hash, and
// that planning the same query again succeeds.
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
	if _, ok := e.Cache.Get(q.Hash(e.Hasher)); !ok {
		t.Fatal("Plan did not cache the query")
	}
	if _, err := e.Plan(q); err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
}

// TestPlanCachesUnderTheParsedQuerysHash pins the cache key to the query as
// parsed, not as optimised. Dedupe rewrites a recall in place. Keyed by the
// rewritten query's hash, "anna anna" would be stored under the hash of "anna".
// The next request for "anna anna" looks up its own parsed hash, so it would
// miss and be optimised again on every call.
func TestPlanCachesUnderTheParsedQuerysHash(t *testing.T) {
	cfg := config.New()
	e := newEngine(t, cfg)
	if err := e.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer e.Stop()

	const text = "recall@0 anna anna topic:color topic:color"
	q, _, err := query.Parse[uint64, float32](text, nil, cfg)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if _, err := e.Plan(q); err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	again, _, err := query.Parse[uint64, float32](text, nil, cfg)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	cached, ok := e.Cache.Get(again.Hash(e.Hasher))
	if !ok {
		t.Fatal("a repeat of a query Dedupe rewrote missed the plan cache")
	}
	stream, err := e.Plan(again)
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
	if stream.Query != cached.Optimised {
		t.Error("a repeat of a query Dedupe rewrote was planned afresh, not served from the cache")
	}
}

// collidingHasher keys every value alike, standing in for two distinct
// queries a client crafted to share a 64-bit key.
type collidingHasher struct{}

func (collidingHasher) Hash(string) uint64 { return 0 }

func (collidingHasher) Seed() uint64 { return 0 }

// TestPlanConfirmsAHitByEquality pins the plan cache against key collisions.
// Two writes to different graphs share a key here. Trusting the key, Plan
// would run the first write's plan for the second, writing its fact to its
// graph. Planned afresh, the second runs as itself and takes the entry over.
func TestPlanConfirmsAHitByEquality(t *testing.T) {
	cfg := config.New()
	e := newEngine(t, cfg)
	e.Hasher = collidingHasher{}
	if err := e.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer e.Stop()

	first, _, err := query.Parse[uint64, float32]("remember@1 'the parrot is turquoise'", nil, cfg)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if _, err := e.Plan(first); err != nil {
		t.Fatalf("first Plan returned error: %v", err)
	}

	second, _, err := query.Parse[uint64, float32]("remember@2 'the kiwi is green'", nil, cfg)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	stream, err := e.Plan(second)
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
	if !stream.Query.Equal(second) {
		t.Errorf("Plan ran %+v for a colliding query, want %+v", stream.Query, second)
	}
	if cached, _ := e.Cache.Get(0); cached.Parsed != second {
		t.Error("the colliding query did not take the cache entry over")
	}
}

// TestPlanRunsARememberWithItsOwnTimes pins the plan cache against a fact
// re-asserted with another time. A remember's key leaves its times out, so the
// same fact said on two days shares one key. Trusting the key, Plan would run
// the first remember's plan for the second and store the first day again. A
// hit is confirmed by equality, which compares the times. So the second is
// planned as itself, and its day is the one the fact holds.
func TestPlanRunsARememberWithItsOwnTimes(t *testing.T) {
	cfg := config.New()
	e := newEngine(t, cfg)
	if err := e.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer e.Stop()

	run := func(text string) (query.Query[uint64, float32], *query.Stream[uint64, float32]) {
		t.Helper()
		q, _, err := query.Parse[uint64, float32](text, nil, cfg)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", text, err)
		}
		stream, err := e.Plan(q)
		if err != nil {
			t.Fatalf("Plan(%q) returned error: %v", text, err)
		}
		if err := e.Apply(context.Background(), stream); err != nil {
			t.Fatalf("Apply(%q) returned error: %v", text, err)
		}
		<-stream.Done()
		if stream.Err != nil {
			t.Fatalf("%q failed: %v", text, stream.Err)
		}
		return q, stream
	}

	first, _ := run("remember 'the parrot is turquoise' said:'2023-05-08'")
	second, _ := run("remember 'the parrot is turquoise' said:'2023-05-25'")
	if first.Hash(e.Hasher) != second.Hash(e.Hasher) {
		t.Fatal("the two remembers have different keys, so the second never met the first's cache entry")
	}

	_, recalled := run("recall parrot")

	if len(recalled.Result.Hits) != 1 {
		t.Fatalf("recall returned %d hits, want the one fact remembered", len(recalled.Result.Hits))
	}
	fact, ok := recalled.Result.Hits[0].Node.(graph.Fact[uint64])
	if !ok {
		t.Fatalf("recall returned a %T, want a fact", recalled.Result.Hits[0].Node)
	}
	want := time.Date(2023, time.May, 25, 0, 0, 0, 0, time.UTC)
	if diff := cmp.Diff(want, fact.Said); diff != "" {
		t.Errorf("Said mismatch (-want +got):\n%s", diff)
	}
}
