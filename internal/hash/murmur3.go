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

import "fmt"

const (
	c1 uint32 = 0xcc9e2d51
	c2 uint32 = 0x1b873593
)

type MurmurHash struct {
	offset int
	size   int
	seed   int
}

func (m MurmurHash) Hash(data string) uint32 {
	h, _ := m.hash(data)
	return h
}

/*
Reference implementation: http://code.google.com/p/smhasher/wiki/MurmurHash3
*/
func (m MurmurHash) hash(data string) (uint32, error) {

	d := []byte(data)

	// Check parameters

	if m.offset < 0 || m.size < 0 {
		return 0, fmt.Errorf("Invalid data boundaries; offset: %v; size: %v",
			m.offset, m.size)
	}

	h1 := uint32(m.seed)
	end := m.offset + m.size
	end -= end % 4

	// Check length of available data

	if len(d) <= end {
		return 0, fmt.Errorf("Data out of bounds; set boundary: %v; data length: %v",
			end, len(d))
	}

	for i := m.offset; i < end; i += 4 {

		var k1 = uint32(d[i])
		k1 |= uint32(d[i+1]) << 8
		k1 |= uint32(d[i+2]) << 16
		k1 |= uint32(d[i+3]) << 24

		k1 *= c1
		k1 = (k1 << 15) | (k1 >> 17) // ROTL32(k1,15);
		k1 *= c2

		h1 ^= k1
		h1 = (h1 << 13) | (h1 >> 19) // ROTL32(h1,13);
		h1 = h1*5 + 0xe6546b64
	}

	// Tail

	var k1 uint32

	switch m.size & 3 {
	case 3:
		k1 = uint32(d[end+2]) << 16
		fallthrough
	case 2:
		k1 |= uint32(d[end+1]) << 8
		fallthrough
	case 1:
		k1 |= uint32(d[end])
		k1 *= c1
		k1 = (k1 << 15) | (k1 >> 17) // ROTL32(k1,15);
		k1 *= c2
		h1 ^= k1
	}

	h1 ^= uint32(m.size)

	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16

	return h1, nil
}
