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

	Parameters QueryParameters[K, P]

	context QueryContext
}

// Plan implements [Query]. A recall needs nothing prepared before the search,
// which runs in [Stream.Commit] under the graph's shared lock. So the plan is a
// fresh stream. A query nested in a time bound is planned here too, and its
// stream hangs on the recall's. The engine and the scheduler then handle one
// stream, and the nested one runs inside its commit.
func (r *Recall[K, P]) Plan(config *config.ConfigSet) (*Stream[K, P], error) {
	stream := NewStream[K, P](r)
	if q := r.Parameters.Since.Query; q != nil {
		nested, err := q.Plan(config)
		if err != nil {
			return nil, err
		}
		stream.Since = nested
	}
	if q := r.Parameters.Until.Query; q != nil {
		nested, err := q.Plan(config)
		if err != nil {
			return nil, err
		}
		stream.Until = nested
	}
	return stream, nil
}

// GetGraphID implements [Query]: it returns the graph the recall reads.
func (r Recall[K, P]) GetGraphID() uint8 {
	return r.context.GraphID
}

// SetGraphID implements [Query]: it selects the graph the recall reads. Hash
// folds the graph in, so it is set before the query is hashed. A query nested
// in a time bound reads the graph of the recall that holds it, so it is given
// the same one.
func (r *Recall[K, P]) SetGraphID(id uint8) {
	r.context.GraphID = id
	if q := r.Parameters.Since.Query; q != nil {
		q.SetGraphID(id)
	}
	if q := r.Parameters.Until.Query; q != nil {
		q.SetGraphID(id)
	}
}

// Hash keys the query for the plan cache. It must fold in everything that
// changes the result set. Two recalls that differ only in graph, depth, top,
// time bounds or the bound vector must not collide. Otherwise the cache would
// hand back another query's plan. A time bound folds in as [TimeOperand.Hash]
// renders it, so a query nested in one contributes its own key. Every keyword
// and anchor is length-prefixed, because a phrase may hold any byte, the
// delimiters included. So ["ab"] and ["a","b"] do not hash alike, and no
// keyword can spell the segments after it.
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
	b.WriteString(r.Parameters.Since.Hash(h))
	b.WriteString("|u=")
	b.WriteString(r.Parameters.Until.Hash(h))
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

// Since resolves the lower time bound the query writes out: the start of the
// time its since clause names, compared with when each fact happened. It
// returns the zero bound, meaning none, when the query has no since clause.
// A since that holds a nested query is read from that query's answer, in
// [Stream.Commit], and has nothing to resolve here.
func (r Recall[K, P]) Since(now time.Time) containers.Bound {
	if r.Parameters.Since.Value == nil {
		return containers.Bound{}
	}
	return containers.Bound{At: r.Parameters.Since.Value.Resolve(now).Start}
}

// Until resolves the upper time bound the query writes out: the start of the
// time its until clause names, compared with when each fact happened. It
// returns the zero bound, meaning none, when the query has no until clause.
// An until that holds a nested query is read from that query's answer, in
// [Stream.Commit], and has nothing to resolve here.
func (r Recall[K, P]) Until(now time.Time) containers.Bound {
	if r.Parameters.Until.Value == nil {
		return containers.Bound{}
	}
	return containers.Bound{At: r.Parameters.Until.Value.Resolve(now).Start}
}
