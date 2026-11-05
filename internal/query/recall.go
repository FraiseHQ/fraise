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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/hash"
)

// Recall is a read query. It searches the selected graph for the facts that
// best match its Keywords and Vector. Its Entities and Topics narrow or seed
// the search, and its Parameters rank and bound it. [Parse] builds one from a
// recall command. The engine's optimisation pipeline may rewrite its lists
// before it is cached.
type Recall[K comparable, P float32 | float64] struct {
	Keywords []string
	Vector   containers.Vector[K, P]
	Entities []string
	Topics   []string

	Parameters QueryParameters[K]

	context QueryContext
}

// Plan implements [Query]. A recall needs nothing prepared before the search,
// which runs in [Stream.Commit] under the graph's shared lock. So the plan is a
// fresh stream.
func (r *Recall[K, P]) Plan(config *config.ConfigSet) (*Stream[K, P], error) {
	return NewStream(r), nil
}

// GetGraphID implements [Query]: it returns the graph the recall reads.
func (r Recall[K, P]) GetGraphID() uint8 {
	return r.context.GraphID
}

// SetGraphID implements [Query]: it selects the graph the recall reads. Hash
// folds the graph in, so it is set before the query is hashed.
func (r *Recall[K, P]) SetGraphID(id uint8) {
	r.context.GraphID = id
}

// Hash keys the query for the plan cache. It must fold in everything that
// changes the result set. Two recalls that differ only in graph, depth, top,
// time bounds or the bound vector must not collide. Otherwise the cache would
// hand back another query's plan. Every keyword and anchor is length-prefixed,
// because a phrase may hold any byte, the delimiters included. So ["ab"] and
// ["a","b"] do not hash alike, and no keyword can spell the segments after it.
func (r Recall[K, P]) Hash(h hash.Hasher[K, string]) K {
	var b strings.Builder
	b.WriteString("g=")
	b.WriteString(strconv.Itoa(int(r.context.GraphID)))
	b.WriteString("|kw=")
	writeStrings(&b, r.Keywords)
	b.WriteString("|en=")
	writeStrings(&b, r.Entities)
	b.WriteString("|to=")
	writeStrings(&b, r.Topics)
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

// Equal implements [Query]: other is a recall with every field Hash folds in
// the same. The engine runs a cached plan only for an equal query, so Equal
// must never hold for two recalls that read differently. A recall equal under
// a key another recall planned would read that recall's graph and terms.
func (r Recall[K, P]) Equal(other Query[K, P]) bool {
	o, ok := other.(*Recall[K, P])
	return ok &&
		o.context == r.context &&
		slices.Equal(o.Keywords, r.Keywords) &&
		slices.Equal(o.Entities, r.Entities) &&
		slices.Equal(o.Topics, r.Topics) &&
		o.Parameters.Equal(r.Parameters) &&
		o.Vector.Equal(r.Vector)
}

// IsWrite implements [Query]. A recall only reads, so it runs under the
// graph's shared lock, concurrently with other reads.
func (r Recall[K, P]) IsWrite() bool {
	return false
}

// Since resolves the query's lower time bound. It returns the zero time,
// meaning no bound, when the query has no since clause.
func (r Recall[K, P]) Since(now time.Time) time.Time {
	if r.Parameters.Since == nil {
		return time.Time{}
	}
	return r.Parameters.Since.Resolve(now)
}

// Until resolves the query's upper time bound. It returns the zero time,
// meaning no bound, when the query has no until clause.
func (r Recall[K, P]) Until(now time.Time) time.Time {
	if r.Parameters.Until == nil {
		return time.Time{}
	}
	return r.Parameters.Until.Resolve(now)
}
