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

package scoring

import "math"

// ClampRank bounds a source position to Contribution.Rank's range: a result
// list longer than the field would otherwise wrap, ranking overflow positions
// as if they were the best.
func ClampRank(rank int) uint16 {
	if rank > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(rank)
}

// clampCount bounds a funding-seed count to Contribution.Count's range, for
// the same reason as clampRank: a wrapped count would misreport a heavily
// funded anchor as barely funded.
func ClampCount(count int) uint16 {
	if count > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(count)
}

// clampDegree bounds an anchor degree to Contribution.Degree's range.
func ClampDegree(degree int) uint32 {
	if int64(degree) > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(degree)
}
