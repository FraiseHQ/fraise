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
	"strconv"
	"strings"
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/hash"
)

type Recall[K comparable, P float32 | float64] struct {
	Keywords []string
	Vector   containers.Vector[K, P]
	Entities []string
	Topics   []string

	Parameters QueryParameters[K]

	context QueryContext
}

func (r *Recall[K, P]) Plan(config *config.ConfigSet) (*Stream[K, P], error) {
	return NewStream(r), nil
}

func (r Recall[K, P]) GetGraphID() uint8 {
	return r.context.GraphID
}

func (r *Recall[K, P]) SetGraphID(id uint8) {
	r.context.GraphID = id
}

// Hash keys the query for the plan cache. It must fold in everything that
// changes the result set: two recalls that differ only in graph, depth, top,
// time bounds or the bound vector must not collide, or the cache would hand
// back a stale plan. The lists are delimited so ["ab"] and ["a","b"] do not
// hash alike.
func (r Recall[K, P]) Hash(h hash.Hasher[K, string]) K {
	var b strings.Builder
	b.WriteString("g=")
	b.WriteString(strconv.Itoa(int(r.context.GraphID)))
	b.WriteString("|kw=")
	b.WriteString(strings.Join(r.Keywords, "\x00"))
	b.WriteString("|en=")
	b.WriteString(strings.Join(r.Entities, "\x00"))
	b.WriteString("|to=")
	b.WriteString(strings.Join(r.Topics, "\x00"))
	b.WriteString("|d=")
	b.WriteString(strconv.Itoa(r.Parameters.Depth))
	b.WriteString("|t=")
	b.WriteString(strconv.Itoa(r.Parameters.Top))
	b.WriteString("|s=")
	if s := r.Parameters.Since; s != nil {
		b.WriteString(s.Hash(h))
	}
	b.WriteString("|u=")
	if u := r.Parameters.Until; u != nil {
		b.WriteString(u.Hash(h))
	}
	b.WriteString("|vec=")
	b.WriteString(r.Vector.Hash(h))
	return h.Hash(b.String())
}

func (r Recall[K, P]) IsWrite() bool {
	return false
}

// Since resolves the query's lower time bound; the zero time (no bound) is
// returned when the query has no since clause.
func (r Recall[K, P]) Since(now time.Time) time.Time {
	if r.Parameters.Since == nil {
		return time.Time{}
	}
	return r.Parameters.Since.Resolve(now)
}

// Until resolves the query's upper time bound; the zero time (no bound) is
// returned when the query has no until clause.
func (r Recall[K, P]) Until(now time.Time) time.Time {
	if r.Parameters.Until == nil {
		return time.Time{}
	}
	return r.Parameters.Until.Resolve(now)
}
