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

// External pins of the default relevance model. MatchCount is the ranking
// the index shipped with before relevance became pluggable — one point per
// query-term occurrence, repeats included — so these pins spell that
// contract out method by method. The randomized equivalence pin against the
// pre-plugin ranking lives with the index, which owns the drive loop.

package relevance_test

import (
	"reflect"
	"testing"

	"github.com/FraiseHQ/fraise/internal/index/relevance"
)

// TestMatchCountTermsKeepsRepeats pins the query-side stream as the
// identity: a repeated query term stays in the stream, in order, because a
// repeat always earned its own point in the pre-plugin ranking — the
// contrast with BM25's distinct-first-occurrence dedup.
func TestMatchCountTermsKeepsRepeats(t *testing.T) {
	model := relevance.MatchCount[int, float32]{}
	tokens := []string{"tide", "moon", "tide", "wave", "moon"}
	if got := model.Terms(tokens); !reflect.DeepEqual(got, tokens) {
		t.Errorf("Terms = %v, want %v unchanged", got, tokens)
	}
}

// TestMatchCountWeightPricesEveryTermAtOne pins Weight to the constant 1:
// document frequency and corpus size never move a match count, so a rare
// term buys no more than a ubiquitous one.
func TestMatchCountWeightPricesEveryTermAtOne(t *testing.T) {
	model := relevance.MatchCount[int, float32]{}
	for _, c := range []struct{ df, docs int }{{1, 1}, {1, 1000}, {999, 1000}} {
		if got := model.Weight(c.df, c.docs); got != 1 {
			t.Errorf("Weight(%d, %d) = %v, want 1", c.df, c.docs, got)
		}
	}
}

// TestMatchCountIncrementIgnoresTermFrequency pins the binary match: a
// document repeating a term ten times gains exactly what one occurrence
// gains — the term's weight, passed through whole — with Prepare's zero
// threaded in, since a match count has no corpus statistic to fold.
func TestMatchCountIncrementIgnoresTermFrequency(t *testing.T) {
	model := relevance.MatchCount[int, float32]{}
	prepared := model.Prepare()
	if prepared != 0 {
		t.Fatalf("Prepare = %v, want 0", prepared)
	}
	if got := model.Increment(1, 7, 1, prepared); got != 1 {
		t.Errorf("Increment at tf 1 = %v, want 1", got)
	}
	if got := model.Increment(1, 7, 10, prepared); got != 1 {
		t.Errorf("Increment at tf 10 = %v, want 1 (matching is binary)", got)
	}
	if got := model.Increment(2.5, 7, 3, prepared); got != 2.5 {
		t.Errorf("Increment with weight 2.5 = %v, want the weight through whole", got)
	}
}

// TestMatchCountFinalizeIgnoresCoverage pins Finalize as the identity: the
// count is the relevance, and match breadth never scales it — the contrast
// with BM25's coverage multiplier.
func TestMatchCountFinalizeIgnoresCoverage(t *testing.T) {
	model := relevance.MatchCount[int, float32]{}
	if got := model.Finalize(3, 3, 3); got != 3 {
		t.Errorf("Finalize at full coverage = %v, want 3", got)
	}
	if got := model.Finalize(3, 1, 3); got != 3 {
		t.Errorf("Finalize at partial coverage = %v, want 3", got)
	}
}

// TestMatchCountLifecycleKeepsNoStatistics pins the no-op hooks: a corpus
// churning through inserts and deletes leaves every query-side answer where
// it was, because a match count consults no statistics — which is what makes
// the model safe to install on an already-populated index.
func TestMatchCountLifecycleKeepsNoStatistics(t *testing.T) {
	model := relevance.MatchCount[int, float32]{}
	model.Indexed(1, []string{"tide", "moon"})
	model.Indexed(2, []string{"wave"})
	model.Removed(1, []string{"tide", "moon"})
	if got := model.Weight(1, 2); got != 1 {
		t.Errorf("Weight after churn = %v, want 1", got)
	}
	if got := model.Prepare(); got != 0 {
		t.Errorf("Prepare after churn = %v, want 0", got)
	}
}
