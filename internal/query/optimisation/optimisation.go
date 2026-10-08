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

package optimisation

import "github.com/FraiseHQ/fraise/internal/query"

// Optimisation is one stage of a [Pipeline]: Optimise takes a parsed query and
// returns the query to run in its place, which may be the same value rewritten
// in place. Its output is cached and shared by every request that hashes
// alike, so a stage is deterministic and leaves no per-request state on it.
type Optimisation[K comparable, P float32 | float64] interface {
	Optimise(q query.Query[K, P]) query.Query[K, P]
}

// Pipeline runs its stages in order, each on the previous stage's output.
type Pipeline[K comparable, P float32 | float64] struct {
	stages []Optimisation[K, P]
}

// NewPipeline returns the pipeline the engine runs on a plan-cache miss,
// currently a single [Dedupe] stage.
func NewPipeline[K comparable, P float32 | float64]() *Pipeline[K, P] {
	return &Pipeline[K, P]{
		stages: []Optimisation[K, P]{
			&Dedupe[K, P]{},
		},
	}
}

// Optimise runs q through every stage in order and returns the last stage's
// output.
func (d *Pipeline[K, P]) Optimise(q query.Query[K, P]) query.Query[K, P] {
	for _, o := range d.stages {
		q = o.Optimise(q)
	}
	return q
}
