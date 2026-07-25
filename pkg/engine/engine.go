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
	"github.com/RonsenbergVI/fraise/internal/cache"
	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/RonsenbergVI/fraise/internal/query"
	"github.com/RonsenbergVI/fraise/internal/query/optimisation"

	"github.com/RonsenbergVI/fraise/pkg/logger"
	"github.com/RonsenbergVI/fraise/pkg/scheduler"
)

type Engine[K ~uint64, P float32 | float64] struct {
	Config        *config.ConfigSet
	Cache         cache.Cache[K, query.Query[K, P]]
	Scheduler     *scheduler.Scheduler[K, P]
	Optimisations *optimisation.Pipeline[K, P]
	Hasher        hash.Hasher[K, string]
}

func NewEngine[K ~uint64, P float32 | float64](c *config.ConfigSet, hasher hash.Hasher[K, string]) *Engine[K, P] {
	e := &Engine[K, P]{
		Config:        c,
		Optimisations: optimisation.NewPipeline[K, P](),
		Scheduler:     scheduler.NewScheduler[K, P](c),
		Hasher:        hasher,
	}
	return e
}

func (e *Engine[K, P]) Start() {
	// allocate cache memory
	c, err := cache.NewLRUCache[K, query.Query[K, P]](e.Config.Engine.CacheCapacity)
	if err != nil {
		logger.Error("Error while initialising cache", "error", err)
		return
	}
	e.Cache = c

	// start the scheduler workers that execute planned streams
	if err := e.Scheduler.Start(); err != nil {
		logger.Error("Error while starting scheduler", "error", err)
	}
}

func (e *Engine[K, P]) Stop() {
	// stop scheduler workers, then release cache memory
	e.Scheduler.Stop()
	e.Cache.Clear()
}

func (e *Engine[K, P]) Plan(q query.Query[K, P]) (*query.Stream[K, P], error) {

	if cached, ok := e.Cache.Get(q.Hash(e.Hasher)); ok {
		q = cached
	} else {
		// NOTE: run optimisaitons on query and cache optimised query
		// only optimised queries are cached
		optimised := e.Optimisations.Optimise(q)
		e.Cache.Put(q.Hash(e.Hasher), optimised)
		q = optimised
	}

	stream, err := q.Plan(e.Config)
	if err != nil {
		return nil, ErrQueryPlan
	}
	return stream, nil
}

func (e *Engine[K, P]) Apply(s *query.Stream[K, P]) {
	e.Scheduler.Submit(s)
}
