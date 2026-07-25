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

// These tests live in the internal `query` package (not query_test) so they can
// populate the unexported `context` field of the query types.
package query

import (
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/hash"
)

// fakeHasher records the last value it was asked to hash and returns a
// deterministic, inspectable key. Shared by the recall/remember tests.
type fakeHasher struct{ last string }

func (h *fakeHasher) Hash(s string) string {
	h.last = s
	return "H(" + s + ")"
}

func (h *fakeHasher) Seed() uint64 {
	return 0
}

var (
	_ hash.Hasher[string, string] = (*fakeHasher)(nil)
	// SetGraphID has a pointer receiver, so only *Recall satisfies Query.
	_ Query[string, float32] = (*Recall[string, float32])(nil)
)

func TestRecallIsWrite(t *testing.T) {
	var r Recall[string, float32]
	if r.IsWrite() {
		t.Error("Recall.IsWrite() = true, want false")
	}
}

func TestRecallGetGraphID(t *testing.T) {
	r := Recall[string, float32]{context: QueryContext{GraphID: 4}}
	if got := r.GetGraphID(); got != 4 {
		t.Errorf("GetGraphID() = %d, want 4", got)
	}
}

// SetGraphID has a pointer receiver, so the update persists when called
// through a pointer (matching Remember.SetGraphID).
func TestRecallSetGraphIDPersists(t *testing.T) {
	r := &Recall[string, float32]{context: QueryContext{GraphID: 1}}
	r.SetGraphID(9)
	if got := r.GetGraphID(); got != 9 {
		t.Errorf("after SetGraphID(9), GetGraphID() = %d, want 9", got)
	}
}

func TestRecallHash(t *testing.T) {
	r := Recall[string, float32]{
		Keywords:   []string{"qu", "ick"},
		Entities:   []string{"alice"},
		Topics:     []string{"weather"},
		Parameters: QueryParameters{Depth: 2, Top: 5},
		context:    QueryContext{GraphID: 3},
	}
	h := &fakeHasher{}

	// Hash folds in graph, delimited keyword/entity/topic lists, depth and top
	// so queries that differ in any of those get distinct cache keys.
	const want = "g=3|kw=qu\x00ick|en=alice|to=weather|d=2|t=5"
	if got := r.Hash(h); got != "H("+want+")" {
		t.Errorf("Hash() = %q, want %q", got, "H("+want+")")
	}
	if h.last != want {
		t.Errorf("hasher received %q, want %q", h.last, want)
	}
}

func TestRecallHashEmpty(t *testing.T) {
	var r Recall[string, float32]
	h := &fakeHasher{}
	const want = "g=0|kw=|en=|to=|d=0|t=0"
	if got := r.Hash(h); got != "H("+want+")" {
		t.Errorf("Hash() = %q, want %q", got, "H("+want+")")
	}
}

// TestRecallHashDistinguishesParameters is the real contract: recalls that
// differ only in graph, depth or top must not share a cache key, otherwise the
// engine hands back a stale plan (e.g. a depth:1 result for a depth:2 query).
func TestRecallHashDistinguishesParameters(t *testing.T) {
	base := func() Recall[string, float32] {
		return Recall[string, float32]{Keywords: []string{"mercury"}}
	}
	variants := map[string]Recall[string, float32]{
		"base":    base(),
		"depth":   func() Recall[string, float32] { r := base(); r.Parameters.Depth = 2; return r }(),
		"top":     func() Recall[string, float32] { r := base(); r.Parameters.Top = 5; return r }(),
		"graph":   func() Recall[string, float32] { r := base(); r.context.GraphID = 1; return r }(),
		"keyword": func() Recall[string, float32] { r := base(); r.Keywords = []string{"venus"}; return r }(),
	}

	seen := make(map[string]string)
	for name, r := range variants {
		key := r.Hash(&fakeHasher{})
		if other, clash := seen[key]; clash {
			t.Errorf("hash collision: %q and %q both produced %q", name, other, key)
		}
		seen[key] = name
	}
}

func TestRecallSinceUntil(t *testing.T) {
	now := time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC)
	absUntil := now.Add(48 * time.Hour)
	r := Recall[string, float32]{
		Parameters: QueryParameters{
			Since: containers.RelativeTime{Dur: time.Hour}, // relative: now - 1h
			Until: containers.AbsoluteTime{T: absUntil},    // absolute: fixed instant
		},
	}

	if got := r.Since(now); !got.Equal(now.Add(-time.Hour)) {
		t.Errorf("Since(now) = %v, want %v", got, now.Add(-time.Hour))
	}
	if got := r.Until(now); !got.Equal(absUntil) {
		t.Errorf("Until(now) = %v, want %v", got, absUntil)
	}
}

func TestRecallPlan(t *testing.T) {
	var r Recall[string, float32]
	s, err := r.Plan(nil)
	if err != nil {
		t.Fatalf("Plan() err = %v, want nil", err)
	}
	if s == nil {
		t.Fatal("Plan() stream = nil, want a ready stream")
	}
	if s.Query != Query[string, float32](&r) {
		t.Errorf("Plan() stream.Query = %v, want the receiver", s.Query)
	}
	select {
	case <-s.Done():
		t.Error("Plan() stream is already done; it must stay open until committed")
	default:
	}
}
