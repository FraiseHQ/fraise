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

type Optimisation[K comparable, P float32 | float64] interface {
	Optimise(q query.Query[K, P]) query.Query[K, P]
}

type Pipeline[K comparable, P float32 | float64] struct {
	stages []Optimisation[K, P]
}

func NewPipeline[K comparable, P float32 | float64]() *Pipeline[K, P] {
	return &Pipeline[K, P]{
		stages: []Optimisation[K, P]{
			&Dedupe[K, P]{},
		},
	}
}

func (d *Pipeline[K, P]) Optimise(q query.Query[K, P]) query.Query[K, P] {
	for _, o := range d.stages {
		q = o.Optimise(q)
	}
	return q
}
