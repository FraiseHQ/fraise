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
	"fmt"
	"strconv"
	"strings"

	"github.com/FraiseHQ/fraise/internal/hash"
)

// Vector carries K so it can hash itself with the same Hasher[K, string] its
// enclosing query uses, mirroring the Recall/Remember pattern.
type Vector[K comparable, P float32 | float64] struct {
	Data []P
}

func NewVector[K comparable, P float32 | float64](data []P) Vector[K, P] {
	return Vector[K, P]{Data: data}
}

func (v Vector[K, P]) Dim() int {
	return len(v.Data)
}

func (v Vector[K, P]) Empty() bool {
	if v.Data == nil {
		return true
	}
	return len(v.Data) == 0
}

// Equal reports whether v and other hold exactly the same coordinates.
func (v Vector[K, P]) Equal(other Vector[K, P]) bool {
	if len(v.Data) != len(other.Data) {
		return false
	}
	for i, x := range v.Data {
		if x != other.Data[i] {
			return false
		}
	}
	return true
}

// Hash keys the vector through h and renders the key for folding into an
// enclosing query's hash material. The coordinates hash in a stable, lossless
// form: exact hex floats so distinct vectors never render alike, delimited so
// [1, 23] and [12, 3] do not collide.
func (v Vector[K, P]) Hash(h hash.Hasher[K, string]) string {
	var b strings.Builder
	for i, x := range v.Data {
		if i > 0 {
			b.WriteByte('\x00')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'x', -1, 64))
	}
	return fmt.Sprint(h.Hash(b.String()))
}
