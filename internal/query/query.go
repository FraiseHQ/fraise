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

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
)

type QueryParameters struct {
	Top   int
	Depth int
	Since TimeValue
	Until TimeValue
}

type Query[K comparable, P float32 | float64] interface {
	Plan(config *config.ConfigSet) *Stream[K, P]
}

type Remember[K comparable, P float32 | float64] struct {
	Keywords []string
	Vector   containers.Vector[P]
	Entities []string
	Topics   []string

	Parameters QueryParameters
}

type Recall[K comparable, P float32 | float64] struct {
	Value    string
	Entities []string
	Topics   []string
}

type Select[K comparable, P float32 | float64] struct {
	Index int
}

func (q Remember[K, P]) Since(now time.Time) time.Time {
	return q.Parameters.Since.Resolve(now)
}

func (q Remember[K, P]) Until(now time.Time) time.Time {
	return q.Parameters.Until.Resolve(now)
}

func (q Remember[K, P]) Plan(config *config.ConfigSet) *Stream[K, P] {
	return nil
}

func (q Recall[K, P]) Plan(config *config.ConfigSet) *Stream[K, P] {
	return nil
}

func (q Select[K, P]) Plan(config *config.ConfigSet) *Stream[K, P] {
	return nil
}
