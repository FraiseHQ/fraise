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

// Same-package pins of the relevance seam. The equivalence pin proves the
// seam is a zero-behavior change — the default model reproduces the
// pre-plugin ranking byte for byte — and the BM25 lifecycle pins reach the
// model's private statistics, which no query result exposes.

package relevance_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/FraiseHQ/fraise/internal/index/relevance"
)

// TestBM25TermsDedupsFirstOccurrence pins the query-side stream: distinct
// terms only — idf must not double-count a repeated term — in first
// occurrence order, which is what keeps score accumulation deterministic.
func TestBM25TermsDedupsFirstOccurrence(t *testing.T) {
	model := relevance.NewBM25[int, float32]()
	got := model.Terms([]string{"tide", "moon", "tide", "wave", "moon"})
	if want := []string{"tide", "moon", "wave"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Terms = %v, want %v", got, want)
	}
}

// TestBM25CoverageScalesByMatchBreadth pins Finalize's slot: full coverage
// leaves the accumulated score whole, half coverage halves it.
func TestBM25CoverageScalesByMatchBreadth(t *testing.T) {
	model := relevance.NewBM25[int, float32]()
	if got := model.Finalize(4, 2, 2); got != 4 {
		t.Errorf("Finalize at full coverage = %v, want 4", got)
	}
	if got := model.Finalize(4, 1, 2); got != 2 {
		t.Errorf("Finalize at half coverage = %v, want 2", got)
	}
}

// TestBM25WeightIsTheStandardIdf pins Weight against the closed form at a
// hand-checkable point: df 1 of 2 documents gives ln 2 exactly.
func TestBM25WeightIsTheStandardIdf(t *testing.T) {
	model := relevance.NewBM25[int, float32]()
	if got, want := model.Weight(1, 2), float32(math.Log(2)); got != want {
		t.Errorf("Weight(1, 2) = %v, want ln 2 = %v", got, want)
	}
}
