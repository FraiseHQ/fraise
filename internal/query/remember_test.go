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
	"testing"

	"github.com/RonsenbergVI/fraise/internal/containers"
)

func TestRememberIsWrite(t *testing.T) {
	var r Remember[string, float32]
	if !r.IsWrite() {
		t.Error("Remember.IsWrite() = false, want true")
	}
}

// Unlike Recall, Remember.SetGraphID has a pointer receiver, so the update
// persists when called through a pointer.
func TestRememberSetGraphIDPersists(t *testing.T) {
	r := &Remember[string, float32]{}
	r.SetGraphID(7)
	if got := r.GetGraphID(); got != 7 {
		t.Errorf("after SetGraphID(7), GetGraphID() = %d, want 7", got)
	}
}

func TestRememberGetGraphID(t *testing.T) {
	r := Remember[string, float32]{context: QueryContext{GraphID: 3}}
	if got := r.GetGraphID(); got != 3 {
		t.Errorf("GetGraphID() = %d, want 3", got)
	}
}

func TestRememberHash(t *testing.T) {
	r := Remember[string, float32]{
		Value:    "hello world",
		Entities: []string{"alice"},
		Topics:   []string{"greeting"},
		Vector:   containers.NewVector[string]([]float32{0.5}),
		context:  QueryContext{GraphID: 2},
	}
	h := &fakeHasher{}

	// Hash folds in graph, value, the delimited entity/topic lists and the
	// bound vector so writes that differ in any of those get distinct cache
	// keys.
	const want = "g=2|v=hello world|en=alice|to=greeting|vec=H(0x1p-01)"
	if got := r.Hash(h); got != "H("+want+")" {
		t.Errorf("Hash() = %q, want %q", got, "H("+want+")")
	}
	if h.last != want {
		t.Errorf("hasher received %q, want %q", h.last, want)
	}
}

// TestRememberHashDistinguishesGraphAndTags is the real contract: writes that
// differ only in graph, entities, topics or the bound vector must not share a
// cache key, or the engine reuses a stale plan and writes to the wrong
// graph/tags (or with the wrong embedding).
func TestRememberHashDistinguishesGraphAndTags(t *testing.T) {
	base := func() Remember[string, float32] {
		return Remember[string, float32]{Value: "the parrot is turquoise"}
	}
	variants := map[string]Remember[string, float32]{
		"base":   base(),
		"graph":  func() Remember[string, float32] { r := base(); r.context.GraphID = 5; return r }(),
		"topic":  func() Remember[string, float32] { r := base(); r.Topics = []string{"birds"}; return r }(),
		"entity": func() Remember[string, float32] { r := base(); r.Entities = []string{"polly"}; return r }(),
		"vector-a": func() Remember[string, float32] {
			r := base()
			r.Vector = containers.NewVector[string]([]float32{1, 0})
			return r
		}(),
		"vector-b": func() Remember[string, float32] {
			r := base()
			r.Vector = containers.NewVector[string]([]float32{0, 1})
			return r
		}(),
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

func TestRememberPlan(t *testing.T) {
	var r Remember[string, float32]
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
