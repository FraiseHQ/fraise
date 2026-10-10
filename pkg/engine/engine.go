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

package engine

import (
	"context"
	"fmt"

	"github.com/FraiseHQ/fraise/internal/cache"
	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/internal/query"
	"github.com/FraiseHQ/fraise/internal/query/optimisation"

	"github.com/FraiseHQ/fraise/pkg/logger"
	"github.com/FraiseHQ/fraise/pkg/scheduler"
)

// Engine plans queries and hands the resulting streams to its scheduler. K is
// the node key type and P the floating-point precision of embeddings and
// scores. Cache maps a query's Hash, computed with Hasher, to the optimised
// query planned for it; it is allocated by [Engine.Start].
type Engine[K ~uint64, P float32 | float64] struct {
	Config        *config.ConfigSet
	Cache         cache.Cache[K, query.Query[K, P]]
	Scheduler     *scheduler.Scheduler[K, P]
	Optimisations *optimisation.Pipeline[K, P]
	Hasher        hash.Hasher[K, string]
}

// NewEngine builds an engine with its optimisation pipeline and scheduler.
// The caller attaches the database as Scheduler.DB before [Engine.Start].
func NewEngine[K ~uint64, P float32 | float64](c *config.ConfigSet, hasher hash.Hasher[K, string]) *Engine[K, P] {
	e := &Engine[K, P]{
		Config:        c,
		Optimisations: optimisation.NewPipeline[K, P](),
		Scheduler:     scheduler.NewScheduler[K, P](c),
		Hasher:        hasher,
	}
	return e
}

// Start allocates the plan cache and starts the scheduler workers. It returns
// an error if either fails, so a caller never brings up a live server backed by
// a nil cache: the cache is assigned only once it is known good.
func (e *Engine[K, P]) Start() error {
	// allocate cache memory
	c, err := cache.NewLRUCache[K, query.Query[K, P]](e.Config.Engine.CacheCapacity)
	if err != nil {
		logger.Error("Error while initialising cache", "error", err)
		return fmt.Errorf("%w: %w", ErrCacheInit, err)
	}
	e.Cache = c

	logger.Info("Engine cache initialised", "capacity", c.Capacity())
	logger.Debug("Plan cache keys hashed",
		"function", e.Config.DB.HashingFunction.Name, "seed", e.Hasher.Seed())

	// start the scheduler workers that execute planned streams
	if err := e.Scheduler.Start(); err != nil {
		logger.Error("Error while starting scheduler", "error", err)
		return err
	}
	return nil
}

// Stop stops the scheduler, which runs every stream it has already accepted
// before returning, and then clears the plan cache.
func (e *Engine[K, P]) Stop() {
	// stop scheduler workers, then release cache memory
	logger.Info("Stopping engine")
	e.Scheduler.Stop()
	e.Cache.Clear()
}

// Plan returns an executable stream for q. A cached query with the same Hash
// stands in for q; on a miss q is optimised and the result cached under q's
// own Hash, the parsed query's rather than the optimised one's, since that is
// the key the next request for it looks up. Each distinct query therefore goes
// through the optimisation pipeline once. The stream is
// built fresh on every call because it carries per-execution state (its
// results, error and completion signal). It returns [ErrQueryPlan] if the
// query cannot be planned.
func (e *Engine[K, P]) Plan(q query.Query[K, P]) (*query.Stream[K, P], error) {

	key := q.Hash(e.Hasher)
	if cached, ok := e.Cache.Get(key); ok {
		q = cached
	} else {
		// Only optimised queries are cached, so a hit skips optimisation.
		optimised := e.Optimisations.Optimise(q)
		e.Cache.Put(key, optimised)
		q = optimised
	}

	stream, err := q.Plan(e.Config)
	if err != nil {
		logger.Error("Failed to plan query", "graph", q.GetGraphID(), "error", err)
		return nil, ErrQueryPlan
	}
	return stream, nil
}

// Apply hands a planned stream to the scheduler for execution. It returns the
// scheduler's error (e.g. ErrShutdown, ErrQueueFull, or a wrapped context
// cancellation) so a caller knows the stream was never enqueued and must not
// wait on its completion.
func (e *Engine[K, P]) Apply(ctx context.Context, s *query.Stream[K, P]) error {
	return e.Scheduler.Submit(ctx, s)
}
