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

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/hash"
)

// Remember is a write query. It stores Value as a fact in the selected graph
// and files it under its Entities and Topics. It indexes Vector for the fact
// when one is bound. Facts are keyed by value, so remembering a stored fact
// again refreshes its timestamp rather than duplicating it (see
// [Stream.Commit]).
//
// Said and Occurred are the times the writer gives the fact: when it was
// stated, and each span over which what it states took place. Either may be
// absent, and the fact then reads the next clock down (see
// [graph.Fact.Intervals]). They are resolved when the write commits, so a
// duration counts back from the moment the fact is stored.
type Remember[K comparable, P float32 | float64] struct {
	Value    string
	Entities []string
	Topics   []string
	Vector   containers.Vector[K, P]
	Said     containers.TimeValue[K]
	Occurred []containers.TimeValue[K]

	context QueryContext
}

// Plan implements [Query]. A remember needs nothing prepared before the write,
// which runs in [Stream.Commit] under the graph's exclusive lock. So the plan
// is a fresh stream.
func (r *Remember[K, P]) Plan(config *config.ConfigSet) (*Stream[K, P], error) {
	return NewStream(r), nil
}

// GetGraphID implements [Query]: it returns the graph the remember writes to.
func (r Remember[K, P]) GetGraphID() uint8 {
	return r.context.GraphID
}

// SetGraphID implements [Query]: it selects the graph the remember writes to.
// Hash folds the graph in, so it is set before the query is hashed.
func (r *Remember[K, P]) SetGraphID(id uint8) {
	r.context.GraphID = id
}

// Hash keys the query for the plan cache. It must fold in the graph selector
// and every field that changes what gets written, the bound vector included.
// Hashing only Value would make these two collide: "remember@3 'x' topic:a" and
// "remember@5 'x' topic:b". The second would then reuse the first's plan and
// write to the wrong graph under the wrong topic. The value and every anchor
// are length-prefixed, because a fact may hold any byte, the delimiters
// included. Otherwise "remember 'x|en=y'" and "remember 'x' entity:'y|en='"
// would share material, and the second would write the first's fact.
//
// Said and Occurred stay out of the material. A fact stated again with new
// times is the same write under the same key, and takes the place of the one
// cached before it. Equal tells the two apart, so the new times are the ones
// written.
func (r Remember[K, P]) Hash(h hash.Hasher[K, string]) K {
	var b strings.Builder
	b.WriteString("g=")
	b.WriteString(strconv.Itoa(int(r.context.GraphID)))
	b.WriteString("|v=")
	writeStrings(&b, []string{r.Value})
	b.WriteString("|en=")
	writeStrings(&b, r.Entities)
	b.WriteString("|to=")
	writeStrings(&b, r.Topics)
	b.WriteString("|vec=")
	b.WriteString(r.Vector.Hash(h))
	return h.Hash(b.String())
}

// Equal implements [Query]: other is a remember with every field Hash folds in
// the same, and the same Said and Occurred. The engine runs a cached plan only
// for an equal query, so Equal must never hold for two remembers that write
// differently. A remember equal under a key another remember planned would
// write that remember's fact, at that remember's times.
func (r Remember[K, P]) Equal(other Query[K, P]) bool {
	o, ok := other.(*Remember[K, P])
	return ok &&
		o.context == r.context &&
		o.Value == r.Value &&
		slices.Equal(o.Entities, r.Entities) &&
		slices.Equal(o.Topics, r.Topics) &&
		o.Vector.Equal(r.Vector) &&
		(o.Said == nil) == (r.Said == nil) &&
		(o.Said == nil || o.Said.Equal(r.Said)) &&
		slices.EqualFunc(o.Occurred, r.Occurred, containers.TimeValue[K].Equal)
}

// IsWrite implements [Query]. A remember writes, so it runs under the graph's
// exclusive lock and no read observes it half applied.
func (r Remember[K, P]) IsWrite() bool {
	return true
}
