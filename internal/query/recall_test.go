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
		Keywords: []string{"qu", "ick"},
		Entities: []string{"alice"},
		Topics:   []string{"weather"},
	}
	h := &fakeHasher{}

	// Hash joins keywords, then entities, then topics with no separator.
	const want = "quickaliceweather"
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
	if got := r.Hash(h); got != "H()" {
		t.Errorf("Hash() = %q, want %q", got, "H()")
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
	if s != nil {
		t.Errorf("Plan() stream = %v, want nil", s)
	}
	if err != nil {
		t.Errorf("Plan() err = %v, want nil", err)
	}
}
