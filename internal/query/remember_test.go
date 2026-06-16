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

import "testing"

// Remember uses a pointer receiver for SetGraphID, so only *Remember satisfies
// the Query interface.
var _ Query[string, float32] = (*Remember[string, float32])(nil)

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
	r := Remember[string, float32]{Value: "hello world"}
	h := &fakeHasher{}

	if got := r.Hash(h); got != "H(hello world)" {
		t.Errorf("Hash() = %q, want %q", got, "H(hello world)")
	}
	if h.last != "hello world" {
		t.Errorf("hasher received %q, want %q", h.last, "hello world")
	}
}

func TestRememberPlan(t *testing.T) {
	var r Remember[string, float32]
	s, err := r.Plan(nil)
	if s != nil {
		t.Errorf("Plan() stream = %v, want nil", s)
	}
	if err != nil {
		t.Errorf("Plan() err = %v, want nil", err)
	}
}
