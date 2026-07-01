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

import (
	"encoding/binary"
	"math/bits"
)

// t1ha1 little-endian prime constants.
const (
	t1p0 uint64 = 0xEC99BF0D8372CAAB
	t1p1 uint64 = 0x82434FE90EDCEF39
	t1p2 uint64 = 0xD4F06DB99D67BE4B
	t1p3 uint64 = 0xBD9CACC22C6E9571
	t1p4 uint64 = 0x9C06FAF4D023E3AB
	t1p5 uint64 = 0xC060724A8424F345
	t1p6 uint64 = 0xCB5AF53AE3AAAC31
)

// T1haHash implements the Hasher interface using Leonid Yuriev's t1ha1
// (little-endian variant), producing a 64-bit hash. The zero value is a
// valid hasher with a zero seed.
type T1haHash struct {
	seed uint64
}

func (t T1haHash) Hash(data string) uint64 {
	return t1ha1LE([]byte(data), t.seed)
}

// rot64 rotates v right by s bits.
func rot64(v uint64, s int) uint64 {
	return bits.RotateLeft64(v, -s)
}

// mux64 multiplies v by prime as a 128-bit product and folds the two halves.
func mux64(v, prime uint64) uint64 {
	hi, lo := bits.Mul64(v, prime)
	return lo ^ hi
}

// mix64 multiplies v by p and mixes in a rotated copy of the result.
func mix64(v, p uint64) uint64 {
	v *= p
	return v ^ rot64(v, 41)
}

func finalWeakAvalanche(a, b uint64) uint64 {
	return mux64(rot64(a+b, 17), t1p4) + mix64(a^b, t1p0)
}

// tail64LE reads the trailing tail (1..8) bytes of p as a little-endian word.
func tail64LE(p []byte, tail int) uint64 {
	var r uint64
	switch tail & 7 {
	case 0:
		return binary.LittleEndian.Uint64(p)
	case 7:
		r = uint64(p[6]) << 8
		fallthrough
	case 6:
		r += uint64(p[5])
		r <<= 8
		fallthrough
	case 5:
		r += uint64(p[4])
		r <<= 32
		fallthrough
	case 4:
		return r + uint64(binary.LittleEndian.Uint32(p))
	case 3:
		r = uint64(p[2]) << 16
		fallthrough
	case 2:
		return r + uint64(binary.LittleEndian.Uint16(p))
	case 1:
		return uint64(p[0])
	}
	return r
}

/*
Reference implementation: https://github.com/erthink/t1ha (t1ha1_le)
*/
func t1ha1LE(data []byte, seed uint64) uint64 {
	length := len(data)
	a := seed
	b := uint64(length)

	if length > 32 {
		c := rot64(uint64(length), 17) + seed
		d := uint64(length) ^ rot64(seed, 17)

		for len(data) >= 32 {
			w0 := binary.LittleEndian.Uint64(data[0:])
			w1 := binary.LittleEndian.Uint64(data[8:])
			w2 := binary.LittleEndian.Uint64(data[16:])
			w3 := binary.LittleEndian.Uint64(data[24:])

			d02 := w0 ^ rot64(w2+d, 17)
			c13 := w1 ^ rot64(w3+c, 17)
			c += a ^ rot64(w0, 41)
			d -= b ^ rot64(w1, 31)
			a ^= t1p1 * (d02 + w3)
			b ^= t1p0 * (c13 + w2)

			data = data[32:]
		}

		a ^= t1p6 * (rot64(c, 17) + d)
		b ^= t1p5 * (c + rot64(d, 17))
	}

	// Remaining 0..31 bytes; each block below falls through to the next,
	// mirroring the switch in the reference implementation.
	rl := len(data)
	v := data
	if rl > 24 {
		b += mux64(binary.LittleEndian.Uint64(v), t1p4)
		v = v[8:]
	}
	if rl > 16 {
		a += mux64(binary.LittleEndian.Uint64(v), t1p3)
		v = v[8:]
	}
	if rl > 8 {
		b += mux64(binary.LittleEndian.Uint64(v), t1p2)
		v = v[8:]
	}
	if rl > 0 {
		a += mux64(tail64LE(v, rl), t1p1)
	}

	return finalWeakAvalanche(a, b)
}
