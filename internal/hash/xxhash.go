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

// XXH64 prime constants.
const (
	xxp1 uint64 = 0x9E3779B185EBCA87
	xxp2 uint64 = 0xC2B2AE3D27D4EB4F
	xxp3 uint64 = 0x165667B19E3779F9
	xxp4 uint64 = 0x85EBCA77C2B2AE63
	xxp5 uint64 = 0x27D4EB2F165667C5
)

// XxHash implements the Hasher interface using Yann Collet's XXH64,
// producing a 64-bit hash. The zero value is a valid hasher with a zero seed.
type XxHash[K ~uint64] struct {
	seed uint64
}

func (x XxHash[K]) Seed() uint64 {
	return x.seed
}

func (x XxHash[K]) Hash(data string) K {
	return K(x.xxh64([]byte(data), x.seed))
}

func (x XxHash[K]) xxRound(acc, input uint64) uint64 {
	acc += input * xxp2
	acc = bits.RotateLeft64(acc, 31)
	acc *= xxp1
	return acc
}

func (x XxHash[K]) xxMergeRound(acc, val uint64) uint64 {
	val = x.xxRound(0, val)
	acc ^= val
	return acc*xxp1 + xxp4
}

/*
Reference implementation: https://github.com/Cyan4973/xxHash (XXH64)
*/
func (x XxHash[K]) xxh64(data []byte, seed uint64) uint64 {
	n := len(data)
	var h uint64
	p := 0

	if n >= 32 {
		v1 := seed + xxp1 + xxp2
		v2 := seed + xxp2
		v3 := seed
		v4 := seed - xxp1

		for n-p >= 32 {
			v1 = x.xxRound(v1, binary.LittleEndian.Uint64(data[p:]))
			v2 = x.xxRound(v2, binary.LittleEndian.Uint64(data[p+8:]))
			v3 = x.xxRound(v3, binary.LittleEndian.Uint64(data[p+16:]))
			v4 = x.xxRound(v4, binary.LittleEndian.Uint64(data[p+24:]))
			p += 32
		}

		h = bits.RotateLeft64(v1, 1) + bits.RotateLeft64(v2, 7) +
			bits.RotateLeft64(v3, 12) + bits.RotateLeft64(v4, 18)
		h = x.xxMergeRound(h, v1)
		h = x.xxMergeRound(h, v2)
		h = x.xxMergeRound(h, v3)
		h = x.xxMergeRound(h, v4)
	} else {
		h = seed + xxp5
	}

	h += uint64(n)

	for n-p >= 8 {
		k1 := x.xxRound(0, binary.LittleEndian.Uint64(data[p:]))
		h ^= k1
		h = bits.RotateLeft64(h, 27)*xxp1 + xxp4
		p += 8
	}

	if n-p >= 4 {
		h ^= uint64(binary.LittleEndian.Uint32(data[p:])) * xxp1
		h = bits.RotateLeft64(h, 23)*xxp2 + xxp3
		p += 4
	}

	for n-p > 0 {
		h ^= uint64(data[p]) * xxp5
		h = bits.RotateLeft64(h, 11) * xxp1
		p++
	}

	h ^= h >> 33
	h *= xxp2
	h ^= h >> 29
	h *= xxp3
	h ^= h >> 32

	return h
}
