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
	"fmt"
	"strconv"
	"strings"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/hash"
)

// When is a read query whose answer is a time. It runs Recall, and answers
// with the span its hits hold on Clock: the interval of the best match when
// the recall's top is 1, and the hull of every hit's intervals when it asks
// for more. A fact given no time on Clock reads the next clock down (see
// [graph.Fact.Intervals]), so every hit has a time to answer with.
//
// [Parse] builds one from a when command, which stands alone as a statement or
// inside a recall's since: or until:. Nested, it is the Query of a
// [TimeOperand], and [Stream.Commit] reads its answer as the recall's bound.
type When[K comparable, P float32 | float64] struct {
	Clock  containers.Clock
	Recall Recall[K, P]
}

// Plan implements [Query]. A when runs as its recall does, so the plan is the
// recall's stream, with the when as the query [Stream.Commit] answers for.
func (w *When[K, P]) Plan(config *config.ConfigSet) (*Stream[K, P], error) {
	stream, err := w.Recall.Plan(config)
	if err != nil {
		return nil, err
	}
	stream.Query = w
	return stream, nil
}

// GetGraphID implements [Query]: it returns the graph the when reads, which is
// the one its recall reads.
func (w When[K, P]) GetGraphID() uint8 {
	return w.Recall.GetGraphID()
}

// SetGraphID implements [Query]: it selects the graph the when reads, by
// selecting it for the recall. Hash folds the graph in through the recall's
// key, so it is set before the query is hashed.
func (w *When[K, P]) SetGraphID(id uint8) {
	w.Recall.SetGraphID(id)
}

// Hash keys the query for the plan cache. It must fold in the clock and
// everything the recall's own key folds in. Two whens that differ only in the
// clock they read must not collide, or the cache would answer one with the
// other's time. The material opens with the clock, which no recall's and no
// remember's does. So a when never shares a key with the recall it runs, and
// the cache never hands a statement a plan that answers with hits alone.
func (w When[K, P]) Hash(h hash.Hasher[K, string]) K {
	var b strings.Builder
	b.WriteString("c=")
	b.WriteString(strconv.Itoa(int(w.Clock)))
	b.WriteString("|r=")
	fmt.Fprint(&b, w.Recall.Hash(h))
	return h.Hash(b.String())
}

// Equal implements [Query]: other is a when that reads the same clock over an
// equal recall. The engine runs a cached plan only for an equal query, so
// Equal must never hold for two whens that answer differently.
func (w When[K, P]) Equal(other Query[K, P]) bool {
	o, ok := other.(*When[K, P])
	return ok && o.Clock == w.Clock && w.Recall.Equal(&o.Recall)
}

// IsWrite implements [Query]. A when only reads, so it runs under the graph's
// shared lock, concurrently with other reads.
func (w When[K, P]) IsWrite() bool {
	return false
}
