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

package query

import (
	"time"

	"github.com/RonsenbergVI/fraise/internal/graph"
)

type QueryParameters struct {
	Top   int
	Depth int
	Since TimeValue
	Until TimeValue
}

type Query[P float32 | float64] struct {
	Keywords []string
	Vector   []P
	Entities []string
	Topics   []string

	parameters QueryParameters
}

func (q Query[P]) Since(now time.Time) time.Time {
	return q.parameters.Since.Resolve(now)
}

func (q Query[P]) Until(now time.Time) time.Time {
	return q.parameters.Until.Resolve(now)
}

type QueryResult[K comparable, V string | ~float32 | ~int | ~bool, P float32 | float64] struct {
	Count int
	Hits  []Hit[K, V, P]
}

type Hit[K comparable, V string | ~float32 | ~int | ~bool, P float32 | float64] struct {
	Node  *graph.Node[K, V]
	Score P
}
