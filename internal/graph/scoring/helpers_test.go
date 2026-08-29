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

package scoring_test

import (
	"math"
	"testing"

	"github.com/FraiseHQ/fraise/internal/graph/scoring"
)

// TestClampsSaturateInsteadOfWrapping pins the overflow guards on the
// Contribution fields: a position or hop beyond the field's range saturates at
// the maximum (worst) value. A plain cast would wrap, ranking an overflow
// position as if it were among the best.
func TestClampsSaturateInsteadOfWrapping(t *testing.T) {
	if got := scoring.ClampRank(3); got != 3 {
		t.Errorf("clampRank(3) = %d, want 3", got)
	}
	if got := scoring.ClampRank(math.MaxUint16 + 1); got != math.MaxUint16 {
		t.Errorf("clampRank(MaxUint16+1) = %d, want %d", got, math.MaxUint16)
	}
	if got := scoring.ClampCount(2); got != 2 {
		t.Errorf("clampCount(2) = %d, want 2", got)
	}
	if got := scoring.ClampCount(math.MaxUint16 + 1); got != math.MaxUint16 {
		t.Errorf("clampCount(MaxUint16+1) = %d, want %d", got, math.MaxUint16)
	}
	if got := scoring.ClampDegree(7); got != 7 {
		t.Errorf("clampDegree(7) = %d, want 7", got)
	}
	if got := scoring.ClampDegree(math.MaxUint32 + 1); got != math.MaxUint32 {
		t.Errorf("clampDegree(MaxUint32+1) = %d, want %d", got, uint32(math.MaxUint32))
	}
}
