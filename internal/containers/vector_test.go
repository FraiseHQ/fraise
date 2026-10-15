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

package containers

import (
	"errors"
	"testing"
)

// fakeHasher returns a deterministic, readable key for the hash tests, like
// the fake in the query package's tests.
type fakeHasher struct{}

func (fakeHasher) Hash(s string) string { return "H(" + s + ")" }
func (fakeHasher) Seed() uint64         { return 0 }

func TestVectorHash(t *testing.T) {
	// Coordinates render as exact hex floats, NUL-delimited, then key through
	// the provided hasher.
	if got, want := NewVector[string]([]float32{0.5, 0.25}).Hash(fakeHasher{}), "H(0x1p-01\x000x1p-02)"; got != want {
		t.Errorf("Hash() = %q, want %q", got, want)
	}
	if got, want := (Vector[string, float64]{}).Hash(fakeHasher{}), "H()"; got != want {
		t.Errorf("empty vector Hash() = %q, want %q", got, want)
	}
}

// TestVectorHashDistinguishesVectors pins the contract the plan cache relies
// on. Two vectors with different coordinates must not share a hash. If they
// did, a recall bound to vector B would silently reuse the cached query for
// vector A.
func TestVectorHashDistinguishesVectors(t *testing.T) {
	a := NewVector[string]([]float64{1, 0}).Hash(fakeHasher{})
	b := NewVector[string]([]float64{0, 1}).Hash(fakeHasher{})
	if a == b {
		t.Errorf("Hash collision: [1,0] and [0,1] both produced %q", a)
	}
}

// TestVectorDistance pins the metric as Euclidean: the 3-4-5 triangle, zero
// for a vector against itself, and symmetric. Vectors of different dimensions
// have no distance, whichever one is shorter.
func TestVectorDistance(t *testing.T) {
	cases := map[string]struct {
		a, b []float64
		want float64
		err  error
	}{
		"3-4-5 triangle": {[]float64{0, 0}, []float64{3, 4}, 5, nil},
		"symmetric":      {[]float64{3, 4}, []float64{0, 0}, 5, nil},
		"itself":         {[]float64{3, 4}, []float64{3, 4}, 0, nil},
		"shorter other":  {[]float64{3, 4}, []float64{3}, 0, ErrDimMismatch},
		"longer other":   {[]float64{3, 4}, []float64{3, 4, 5}, 0, ErrDimMismatch},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := NewVector[string](tc.a).Distance(NewVector[string](tc.b))
			if !errors.Is(err, tc.err) {
				t.Fatalf("Distance(%v, %v) error = %v, want %v", tc.a, tc.b, err, tc.err)
			}
			if got != tc.want {
				t.Errorf("Distance(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
