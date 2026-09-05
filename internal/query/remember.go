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

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/hash"
)

type Remember[K comparable, P float32 | float64] struct {
	Value    string
	Entities []string
	Topics   []string
	Vector   containers.Vector[K, P]

	context QueryContext
}

func (r *Remember[K, P]) Plan(config *config.ConfigSet) (*Stream[K, P], error) {
	return NewStream(r), nil
}

func (r Remember[K, P]) GetGraphID() uint8 {
	return r.context.GraphID
}

func (r *Remember[K, P]) SetGraphID(id uint8) {
	r.context.GraphID = id
}

// Hash keys the query for the plan cache. Like Recall it must fold in the graph
// selector and every field that changes what gets written — including the bound
// vector: hashing only Value would make `remember@3 'x' topic:a` and
// `remember@5 'x' topic:b` collide, so the second would reuse the first's plan
// and write to the wrong graph.
func (r Remember[K, P]) Hash(h hash.Hasher[K, string]) K {
	var b strings.Builder
	writeField(&b, "g", strconv.Itoa(int(r.context.GraphID)))
	writeField(&b, "v", r.Value)
	writeList(&b, "en", r.Entities)
	writeList(&b, "to", r.Topics)
	writeField(&b, "vec", r.Vector.Hash(h))
	return h.Hash(b.String())
}

// SameAs reports whether other is an equivalent Remember for plan-cache hits.
func (r Remember[K, P]) SameAs(other Query[K, P]) bool {
	o, ok := other.(*Remember[K, P])
	if !ok {
		return false
	}
	if r.context.GraphID != o.context.GraphID || r.Value != o.Value {
		return false
	}
	if len(r.Entities) != len(o.Entities) || len(r.Topics) != len(o.Topics) {
		return false
	}
	for i := range r.Entities {
		if r.Entities[i] != o.Entities[i] {
			return false
		}
	}
	for i := range r.Topics {
		if r.Topics[i] != o.Topics[i] {
			return false
		}
	}
	return r.Vector.Equal(o.Vector)
}

func (r Remember[K, P]) IsWrite() bool {
	return true
}
