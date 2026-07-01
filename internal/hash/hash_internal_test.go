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

package hash

import "testing"

// TestSeedChangesOutput verifies the (unexported) seed actually perturbs the
// digest for both hashers. It is white-box because the seed has no exported
// setter, matching MurmurHash's unexported seed field.
func TestSeedChangesOutput(t *testing.T) {
	const in = "the same input"
	if (T1haHash{seed: 0}).Hash(in) == (T1haHash{seed: 1}).Hash(in) {
		t.Error("T1haHash: different seeds produced the same hash")
	}
	if (XxHash{seed: 0}).Hash(in) == (XxHash{seed: 1}).Hash(in) {
		t.Error("XxHash: different seeds produced the same hash")
	}
}

// TestT1haSeededVector locks a seeded t1ha1 value as a regression anchor.
func TestT1haSeededVector(t *testing.T) {
	if got := t1ha1LE([]byte("abc"), 42); got != 0x90f18c6ab3c1c1de {
		t.Errorf("t1ha1LE(\"abc\", 42) = %#016x, want 0x90f18c6ab3c1c1de", got)
	}
}

// TestXXH64InternalMatchesMethod ensures the internal function and the exported
// Hash method agree, and re-checks a canonical seed-0 vector at the function level.
func TestXXH64InternalMatchesMethod(t *testing.T) {
	const in = "The quick brown fox jumps over the lazy dog"
	if got := xxh64([]byte(in), 0); got != 0x0b242d361fda71bc {
		t.Errorf("xxh64(%q, 0) = %#016x, want 0x0b242d361fda71bc", in, got)
	}
	if xxh64([]byte(in), 0) != (XxHash{}).Hash(in) {
		t.Error("xxh64 and XxHash.Hash disagree at seed 0")
	}
}

// TestTail64LE checks the little-endian tail reader across every remainder
// length (1..8) against a straightforward reference decode.
func TestTail64LE(t *testing.T) {
	full := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	for n := 1; n <= 8; n++ {
		var want uint64
		for i := 0; i < n; i++ {
			want |= uint64(full[i]) << (8 * i)
		}
		if got := tail64LE(full, n); got != want {
			t.Errorf("tail64LE(len=%d) = %#016x, want %#016x", n, got, want)
		}
	}
}
