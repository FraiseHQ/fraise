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

package graph

import (
	"time"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/hash"
)

// Fact is a vertex holding one remembered statement. Facts are the nodes the
// text and vector indexes hold, and the ones Search returns as hits. Topics
// and named entities reach them through [IsAbout] and [Mentions] edges.
// Hasher is the graph's hasher, carried so Key can derive the fact's key on
// its own.
//
// A fact tells time on three clocks. Timestamp is when it was written, which
// the server sets. Said is when the statement was made, and Occurred the
// intervals over which what it states took place. Both are the writer's to
// give, and a fact given neither reads the same on every clock (see
// [Fact.Intervals]).
type Fact[K comparable] struct {
	NodeAttributes
	Said     time.Time
	Occurred []containers.Interval

	Hasher hash.Hasher[K, string] `json:"-"`
}

// Key implements [Node]. It is the fact's Hash under its own Hasher, the key
// the graph stores it under.
func (f Fact[K]) Key() K {
	return f.Hash(f.Hasher)
}

// GetValue implements [Node]. It returns the fact's text.
func (f Fact[K]) GetValue() string {
	return f.Value
}

// GetTimestamp implements [Node]. It returns when the fact was recorded.
func (f Fact[K]) GetTimestamp() time.Time {
	return f.Timestamp
}

// GetAttributes implements [Node]. It returns the fact's text and timestamp,
// for readers that handle every node alike.
func (f Fact[K]) GetAttributes() *NodeAttributes {
	return &f.NodeAttributes
}

// Hash keys the fact by its text in the fact namespace. A topic or entity
// reading the same text is a different node (see Node).
func (f Fact[K]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash("fact:" + f.Value)
}

// Intervals returns when the fact holds on clock c. A clock the fact was given
// no time on reads the next one: happened falls back to said, and said to
// written. So a time window and a when query read every fact, whatever it was
// told at write time, and a fact given no times answers with its timestamp on
// all three. Said and written are single instants, an interval that ends where
// it starts.
func (f Fact[K]) Intervals(c containers.Clock) []containers.Interval {
	if c == containers.Happened && len(f.Occurred) > 0 {
		return f.Occurred
	}
	at := f.Timestamp
	if c != containers.Written && !f.Said.IsZero() {
		at = f.Said
	}
	return []containers.Interval{{Start: at, End: at}}
}

// within reports whether the fact lies inside the window since and until
// draw, each on the clock it names. A zero bound is open. Each bound draws
// half of the window: since everything from its instant on, until everything
// before its instant. The fact is inside when one of its intervals overlaps
// both halves, so a fact that happened over a week is inside any window
// touching the week, and one that happened twice, either side of the window,
// is outside it. Bounds on two clocks each read an interval on their own.
// The halves stay two intervals because until never takes in its own
// instant: joined into one, bounds that coincide would read as that instant,
// and a fact beginning on it would enter a window that until alone keeps it
// out of. Every site that windows a search asks here, so the seeds, the pool
// and the ranking agree on which facts a recall can return.
func (f Fact[K]) within(since containers.Bound, until containers.Bound) bool {
	opens := containers.Interval{Start: since.At}
	closes := containers.Interval{End: until.At}
	if since.Clock != until.Clock {
		return f.overlaps(since.Clock, opens) && f.overlaps(until.Clock, closes)
	}
	for _, interval := range f.Intervals(since.Clock) {
		if interval.Overlaps(opens) && interval.Overlaps(closes) {
			return true
		}
	}
	return false
}

// overlaps reports whether one of the fact's intervals on clock c overlaps
// window.
func (f Fact[K]) overlaps(c containers.Clock, window containers.Interval) bool {
	for _, interval := range f.Intervals(c) {
		if interval.Overlaps(window) {
			return true
		}
	}
	return false
}

// NamedEntity is an anchor vertex for a person, place or thing that facts
// mention. A [Mentions] edge links it to each such fact. Naming it in a recall
// lifts and expands the search through those facts. Hasher is the graph's
// hasher, carried so Key can derive the entity's key on its own.
type NamedEntity[K comparable] struct {
	NodeAttributes

	Hasher hash.Hasher[K, string] `json:"-"`
}

// Key implements [Node]. It is the entity's Hash under its own Hasher, the
// key the graph stores it under.
func (n NamedEntity[K]) Key() K {
	return n.Hash(n.Hasher)
}

// GetValue implements [Node]. It returns the entity's text.
func (n NamedEntity[K]) GetValue() string {
	return n.Value
}

// GetTimestamp implements [Node]. It returns when the entity was recorded.
func (n NamedEntity[K]) GetTimestamp() time.Time {
	return n.Timestamp
}

// GetAttributes implements [Node]. It returns the entity's text and
// timestamp, for readers that handle every node alike.
func (n *NamedEntity[K]) GetAttributes() *NodeAttributes {
	return &n.NodeAttributes
}

// Hash keys the entity by its name in the entity namespace. A fact whose
// whole text is that name is a different node (see Node).
func (n NamedEntity[K]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash("entity:" + n.Value)
}

// Topic is an anchor vertex for a subject facts are filed under. An [IsAbout]
// edge links it to each such fact. Naming it in a recall lifts and expands
// the search through those facts. Hasher is the graph's hasher, carried so
// Key can derive the topic's key on its own.
type Topic[K comparable] struct {
	ID K
	NodeAttributes

	Hasher hash.Hasher[K, string] `json:"-"`
}

// Key implements [Node]. It is the topic's Hash under its own Hasher, the key
// the graph stores it under.
func (t Topic[K]) Key() K {
	return t.Hash(t.Hasher)
}

// GetValue implements [Node]. It returns the topic's text.
func (t Topic[K]) GetValue() string {
	return t.Value
}

// GetTimestamp implements [Node]. It returns when the topic was recorded.
func (t Topic[K]) GetTimestamp() time.Time {
	return t.Timestamp
}

// GetAttributes implements [Node]. It returns the topic's text and timestamp,
// for readers that handle every node alike.
func (t *Topic[K]) GetAttributes() *NodeAttributes {
	return &t.NodeAttributes
}

// Hash keys the topic by its name in the topic namespace. A fact whose whole
// text is that name is a different node (see Node).
func (t Topic[K]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash("topic:" + t.Value)
}
