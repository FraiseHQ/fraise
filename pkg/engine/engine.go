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
	"github.com/RonsenbergVI/fraise/internal/query"
	"github.com/RonsenbergVI/fraise/internal/query/optimisation"

	"github.com/RonsenbergVI/fraise/pkg/scheduler"
)

type Engine[K comparable, P float32 | float64] struct {
	Config        *config.ConfigSet
	Cache         cache.Cache[K, query.Query[K, P]]
	Scheduler     *scheduler.Scheduler[K, P]
	Optimisations *optimisation.Pipeline[K, P]
}

func NewEngine[K comparable, P float32 | float64](c *config.ConfigSet) *Engine[K, P] {
	e := &Engine[K, P]{
		Config:        c,
		Optimisations: optimisation.NewPipeline[K, P](),
		Scheduler:     scheduler.NewScheduler[K, P](c),
	}
	return e
}

func (e *Engine[K, P]) Start() {
	// allocate cache memory
	e.Cache = cache.NewLRUCache[K, query.Query[K, P]](e.Config.Engine.CacheCapacity)
}

func (e *Engine[K, P]) Stop() {
	// release cache memory
	e.Cache.Clear()
}

func (e *Engine[K, P]) Plan(q query.Query[K, P]) (*query.Stream[K, P], error) {

	if cached, ok := e.Cache.Get(q.Hash()); ok {
		q = cached
	} else {
		// NOTE: run optimisaitons on query and cache optimised query
		// only optimised queries are cached
		optimised := e.Optimisations.Optimise(q)
		e.Cache.Put(q.Hash(), optimised)
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
