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

// Point represents a value in a P-dimensional space (with P being float32 or
// float64) that spatial trees can index and query.
type Point[P float32 | float64] interface {
	// Dim reports the number of dimensions of the point.
	Dim() int

	// GetValue returns the coordinate of the point along the given dimension.
	GetValue(dim int) P

	// Distance returns the distance between this point and p.
	Distance(p Point[P]) P

	// PlaneDistance returns the distance from the point to the axis-aligned
	// hyperplane at coordinate val along the given dimension.
	PlaneDistance(val P, dim int) P
}

// BallPoint is a Point that additionally carries its raw coordinate Vector,
// used by ball-tree style structures.
type BallPoint[P float32 | float64] struct {
	Point[P]
	Data Vector[P]
}

// Tree is the common interface implemented by the spatial tree containers.
type Tree interface {
}
