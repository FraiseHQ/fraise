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

import "math"

type Vector[P float32 | float64] struct {
	Data []P
}

func NewVector[P float32 | float64](data []P) Vector[P] {
	return Vector[P]{Data: data}
}

func (v Vector[P]) Dim() int {
	return len(v.Data)
}

// Cosine distance between 2 vectors
func Cosine[P float32 | float64](lhs, rhs Vector[P]) (P, error) {
	if len(lhs.Data) != len(rhs.Data) {
		return 0, DimMismatchError
	}
	var dot, na, nb P
	for i := range lhs.Data {
		dot += lhs.Data[i] * rhs.Data[i]
		na += lhs.Data[i] * lhs.Data[i]
		nb += rhs.Data[i] * rhs.Data[i]
	}
	// NOTE: isn't it expensive to convert to float64 if P is of type float32?
	denom := P(math.Sqrt(float64(na)) + math.Sqrt(float64(nb)))
	if denom == 0 {
		return 0, nil
	}
	return dot / denom, nil
}
