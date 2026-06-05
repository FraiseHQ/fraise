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
	"sync"

	"github.com/RonsenbergVI/fraise/internal/cache"
	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/query"

	"github.com/RonsenbergVI/fraise/pkg/scheduler"
)

type Engine[K comparable, P float32 | float64] struct {
	mu sync.RWMutex

	Config    *config.ConfigSet
	Cache     cache.LRUCache[query.Stream[K, P]]
	Scheduler *scheduler.Scheduler[K, P]
}

func NewEngine[K comparable, P float32 | float64](config *config.ConfigSet) (*Engine[K, P], error) {
	return nil, nil
}

func (e *Engine[K, P]) Start() error {
	return nil
}

func (e *Engine[K, P]) Stop() error {
	return nil
}

func (e *Engine[K, P]) Plan(q query.Query[K, P]) (*query.Stream[K, P], error) {
	stream, err := q.Plan(e.Config)
	if err != nil {
		return nil, ErrQueryPlan
	}
	return stream, nil
}

func (e *Engine[K, P]) Apply(s *query.Stream[K, P]) {
	e.Scheduler.Submit(s)

}

func (e *Engine[K, P]) Lock() error {
	return nil
}

func (e *Engine[K, P]) Release() error {
	return nil
}
