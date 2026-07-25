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

package hash_test

import (
	"fmt"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/hash"
)

// TestMurmur3KnownVectors pins the implementation to the canonical
// MurmurHash3 x86_32 test vectors (seed 0). These prove correctness, not just
// stability.
func TestMurmur3KnownVectors(t *testing.T) {
	cases := []struct {
		in   string
		want uint32
	}{
		{"", 0x00000000},
		{"a", 0x3c2569b2},
		{"abc", 0xb3dd93fa},
		{"test", 0xba6bd213},
		{"Hello, world!", 0xc0363e43},
		{"The quick brown fox jumps over the lazy dog", 0x2e4ff723},
	}
	for _, c := range cases {
		if got := (hash.MurmurHash{}).Hash(c.in); got != c.want {
			t.Errorf("MurmurHash.Hash(%q) = %#08x, want %#08x", c.in, got, c.want)
		}
	}
}

// TestXXH64KnownVectors pins the implementation to the canonical XXH64 test
// vectors published by the reference (seed 0). These prove correctness, not
// just stability.
func TestXXH64KnownVectors(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"", 0xef46db3751d8e999},
		{"a", 0xd24ec4f1a98c6e5b},
		{"abc", 0x44bc2cf5ad770999},
		{"The quick brown fox jumps over the lazy dog", 0x0b242d361fda71bc},
	}
	for _, c := range cases {
		if got := (hash.XxHash{}).Hash(c.in); got != c.want {
			t.Errorf("XxHash.Hash(%q) = %#016x, want %#016x", c.in, got, c.want)
		}
	}
}

// TestT1haRegressionVectors locks the t1ha1 little-endian output for a set of
// fixed inputs (seed 0). There is no widely-published short-string vector
// table for t1ha1, so these are regression anchors captured from the
// implementation: a change to any of them signals an accidental behavioural
// change.
func TestT1haRegressionVectors(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"", 0x0000000000000000},
		{"a", 0x10a90771c9c0828c},
		{"abc", 0xaaf29d916709fed8},
		{"message digest", 0xe84ee2de03aba67d},
		{"The quick brown fox jumps over the lazy dog", 0x86235f2773f9ada1},
		{"0123456789012345678901234567890123456789", 0x9843d2619726b77c},
	}
	for _, c := range cases {
		if got := (hash.T1haHash{}).Hash(c.in); got != c.want {
			t.Errorf("T1haHash.Hash(%q) = %#016x, want %#016x", c.in, got, c.want)
		}
	}
}

// TestT1haCoversAllTailLengths exercises the 32-byte block loop and every tail
// branch by covering lengths that straddle the block boundary and each
// remainder. Distinct hashes across a run of increasing lengths confirm each
// tail branch actually mixes the trailing bytes.
func TestT1haCoversAllTailLengths(t *testing.T) {
	seen := make(map[uint64]int)
	h := hash.T1haHash{}
	for n := 0; n <= 80; n++ {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i%251 + 1)
		}
		got := h.Hash(string(b))
		if prev, ok := seen[got]; ok {
			t.Errorf("T1haHash collision between length %d and %d (%#016x)", prev, n, got)
		}
		seen[got] = n
	}
}

func TestHashDeterminism(t *testing.T) {
	inputs := []string{"", "x", "fraise", "The quick brown fox jumps over the lazy dog", "0123456789012345678901234567890123456789"}
	for _, in := range inputs {
		if a, b := (hash.T1haHash{}).Hash(in), (hash.T1haHash{}).Hash(in); a != b {
			t.Errorf("T1haHash not deterministic for %q: %#x vs %#x", in, a, b)
		}
		if a, b := (hash.XxHash{}).Hash(in), (hash.XxHash{}).Hash(in); a != b {
			t.Errorf("XxHash not deterministic for %q: %#x vs %#x", in, a, b)
		}
	}
}

// TestHashNoCollisionsOverCorpus is a distribution sanity check: a large set
// of distinct short strings should hash without collision for either hasher.
func TestHashNoCollisionsOverCorpus(t *testing.T) {
	const n = 50000
	t1seen := make(map[uint64]string, n)
	xxseen := make(map[uint64]string, n)
	t1, xx := hash.T1haHash{}, hash.XxHash{}
	for i := 0; i < n; i++ {
		s := fmt.Sprintf("key-%d-value", i)

		h1 := t1.Hash(s)
		if prev, ok := t1seen[h1]; ok {
			t.Errorf("T1haHash collision: %q and %q both -> %#016x", prev, s, h1)
		}
		t1seen[h1] = s

		h2 := xx.Hash(s)
		if prev, ok := xxseen[h2]; ok {
			t.Errorf("XxHash collision: %q and %q both -> %#016x", prev, s, h2)
		}
		xxseen[h2] = s
	}
}

func BenchmarkT1haHash(b *testing.B) {
	data := "The quick brown fox jumps over the lazy dog"
	h := hash.T1haHash{}
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		_ = h.Hash(data)
	}
}

func BenchmarkXxHash(b *testing.B) {
	data := "The quick brown fox jumps over the lazy dog"
	h := hash.XxHash{}
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		_ = h.Hash(data)
	}
}
