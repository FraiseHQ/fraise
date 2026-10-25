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

// pool builds a query's candidate pool from per-key contribution lists, keyed
// 1..n in order, with the ascending key order Search binds.
func pool(lists ...[]scoring.Contribution[uint64, float64]) (scoring.Candidates[uint64, float64], []uint64) {
	candidates := make(scoring.Candidates[uint64, float64], len(lists))
	keys := make([]uint64, len(lists))
	for i, list := range lists {
		keys[i] = uint64(i + 1)
		candidates[keys[i]] = list
	}
	return candidates, keys
}

// text is a text sighting of mass score whose fact's own words hold the share
// held of the query's information.
func text(score, held float64) scoring.Contribution[uint64, float64] {
	return scoring.Contribution[uint64, float64]{Src: scoring.SrcText, Score: score, Count: 1, Held: held}
}

func vector(similarity float64) scoring.Contribution[uint64, float64] {
	return scoring.Contribution[uint64, float64]{Src: scoring.SrcVector, Score: similarity, Count: 1}
}

// stats is a channel's mean and population standard deviation over a pool.
func stats(values ...float64) (mean, sd float64) {
	var sum, squares float64
	for _, v := range values {
		sum += v
		squares += v * v
	}
	mean = sum / float64(len(values))
	return mean, math.Sqrt(squares/float64(len(values)) - mean*mean)
}

// TestInformationScorerFoldsZScores pins the fold on a pool small enough to
// work by hand. Four candidates: text masses 3, 1, 0, 0; similarities 0.6, 0.5,
// 0.4 and none; and a graph term of 2 on the fourth alone (an anchor of degree
// 1 observing 2, at background zero). Each channel's z is taken over all four,
// a fact the channel did not see counting at zero, and only the part above the
// mean counts. The vector's best z is under the bar √(2 ln 4), so it is not
// trusted further. A fact scores (1 + z_text⁺)(1 + z_vector⁺) − 1 + z_graph⁺:
// the first is corroborated by both readings, the second's text is at the
// mean and adds nothing, and the fourth stands on its graph term alone.
func TestInformationScorerFoldsZScores(t *testing.T) {
	candidates, keys := pool(
		[]scoring.Contribution[uint64, float64]{text(3, 1), vector(0.6)},
		[]scoring.Contribution[uint64, float64]{text(1, 0.5), vector(0.5)},
		[]scoring.Contribution[uint64, float64]{vector(0.4)},
		[]scoring.Contribution[uint64, float64]{{Src: scoring.SrcGraph, Score: 2, Via: 9, Degree: 1, Count: 1}},
	)
	scorer := scoring.NewInformationScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{Candidates: candidates, Keys: keys})

	tm, ts := stats(3, 1, 0, 0)
	vm, vs := stats(0.6, 0.5, 0.4, 0)
	gm, gs := stats(0, 0, 0, 2)
	if (0.6-vm)/vs >= math.Sqrt(2*math.Log(4)) {
		t.Fatal("the fixture's best similarity clears the bar; this case pins the untrusted vector")
	}
	want := map[uint64]float64{
		1: (1+(3-tm)/ts)*(1+(0.6-vm)/vs) - 1,
		2: (0.5 - vm) / vs,
		3: (0.4 - vm) / vs,
		4: (2 - gm) / gs,
	}
	for key, w := range want {
		if got := scorer.Score(candidates[key]); math.Abs(got-w) > 1e-12 {
			t.Errorf("Score(candidate %d) = %v, want %v", key, got, w)
		}
	}
}

// TestInformationScorerNeverTakesAway pins the floor: a channel that places a
// fact at or below the pool's mean adds nothing to it, so no channel can lower
// what the others give. The second fact's text is under the mean of 1.5 and it
// has nothing else, so it scores zero, and a fact no channel saw scores zero.
func TestInformationScorerNeverTakesAway(t *testing.T) {
	candidates, keys := pool(
		[]scoring.Contribution[uint64, float64]{text(4, 1)},
		[]scoring.Contribution[uint64, float64]{text(1, 1)},
		[]scoring.Contribution[uint64, float64]{text(1, 1)},
		[]scoring.Contribution[uint64, float64]{{Src: scoring.SrcGraph, Score: 0, Via: 9, Degree: 4, Count: 1}},
	)
	scorer := scoring.NewInformationScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{Candidates: candidates, Keys: keys})
	for _, key := range []uint64{2, 4} {
		if got := scorer.Score(candidates[key]); got != 0 {
			t.Errorf("Score(candidate %d) = %v, want 0: a channel below its mean adds nothing", key, got)
		}
	}
}

// TestInformationScorerTrustsAConfidentVectorTheWordsMiss pins the vector's
// trust. Four candidates hold the same text mass, so the text tells them apart
// not at all, and the first alone has a similarity: its vector z is √3, over
// the bar √(2 ln 4) ≈ 1.67 a chance maximum over four would reach. Its vector
// then counts (1 + √3)^(2 − h), h being the share of the question its own
// words hold: squared when they hold none of it, once when they hold all of
// it. With three candidates the same reading peaks at √2, under √(2 ln 3), and
// is not trusted further whatever the words hold.
func TestInformationScorerTrustsAConfidentVectorTheWordsMiss(t *testing.T) {
	scored := func(n int, held float64) float64 {
		lists := [][]scoring.Contribution[uint64, float64]{{text(1, held), vector(0.5)}}
		for range n - 1 {
			lists = append(lists, []scoring.Contribution[uint64, float64]{text(1, 1)})
		}
		candidates, keys := pool(lists...)
		return scoring.NewInformationScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{Candidates: candidates, Keys: keys}).Score(candidates[1])
	}
	cases := map[string]struct {
		n    int
		held float64
		want float64
	}{
		"confident, words hold none":     {4, 0, math.Pow(1+math.Sqrt(3), 2) - 1},
		"confident, words hold half":     {4, 0.5, math.Pow(1+math.Sqrt(3), 1.5) - 1},
		"confident, words hold all":      {4, 1, math.Sqrt(3)},
		"under the bar, words hold none": {3, 0, math.Sqrt(2)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := scored(tc.n, tc.held); math.Abs(got-tc.want) > 1e-12 {
				t.Errorf("Score = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestInformationScorerSeedFusionIsTheSeedMass pins the fold before a pool is
// bound: the traversal is seeded with the excess methodology's seed mass, the
// text mass plus the similarity weighted by the text's weakness, so the graph
// observes the same masses whichever fold ranks the hits.
func TestInformationScorerSeedFusionIsTheSeedMass(t *testing.T) {
	list := []scoring.Contribution[uint64, float64]{text(2, 1), vector(0.5)}
	seeding := scoring.NewInformationScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{TextMissed: 1})
	if got := seeding.Score(list); got != 3 {
		t.Errorf("seed fold = %v, want text 2 + (1 + 1)·0.5 = 3", got)
	}
}

// TestInformationScorerIsScaleFreePerChannel pins that a channel's units drop
// out: scaling every text mass by four leaves every score unchanged, since a
// z-score is free of its channel's units. The channels are commensurable
// without being calibrated against each other.
func TestInformationScorerIsScaleFreePerChannel(t *testing.T) {
	build := func(scale float64) (scoring.Candidates[uint64, float64], []uint64) {
		return pool(
			[]scoring.Contribution[uint64, float64]{text(3*scale, 1), vector(0.6)},
			[]scoring.Contribution[uint64, float64]{text(1*scale, 1), vector(0.5)},
			[]scoring.Contribution[uint64, float64]{vector(0.4)},
		)
	}
	plain, keys := build(1)
	scaled, _ := build(4)
	scorer := scoring.NewInformationScorer[uint64, float64]()
	before := scorer.Observe(scoring.Observation[uint64, float64]{Candidates: plain, Keys: keys})
	after := scorer.Observe(scoring.Observation[uint64, float64]{Candidates: scaled, Keys: keys})
	for _, key := range keys {
		if got, want := after.Score(scaled[key]), before.Score(plain[key]); math.Abs(got-want) > 1e-12 {
			t.Errorf("Score(candidate %d) with text ×4 = %v, want the unscaled %v", key, got, want)
		}
	}
}

// TestInformationScorerScoresAnchorsAloneByCount pins the fold of a query with
// nothing to match: named anchors alone leave every channel empty, so a fact
// scores one unit per named anchor it is filed under, as under the
// ExcessScorer, and the newest of equally filed facts still comes first once
// decay applies.
func TestInformationScorerScoresAnchorsAloneByCount(t *testing.T) {
	sighting := func(via uint64) scoring.Contribution[uint64, float64] {
		return scoring.Contribution[uint64, float64]{Src: scoring.SrcAnchor, Score: 1, Via: via, Degree: 3, Count: 1}
	}
	candidates, keys := pool(
		[]scoring.Contribution[uint64, float64]{sighting(7), sighting(8)},
		[]scoring.Contribution[uint64, float64]{sighting(7)},
	)
	scorer := scoring.NewInformationScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{Candidates: candidates, Keys: keys})
	want := map[uint64]float64{1: 2, 2: 1}
	for key, w := range want {
		if got := scorer.Score(candidates[key]); got != w {
			t.Errorf("Score(candidate %d) = %v, want %v, one unit per named anchor", key, got, w)
		}
	}
}

// TestInformationScorerPurity pins the Scorer contract's purity clause:
// binding a pool returns a fresh scorer and leaves the shared instance
// unbound, which Search shares across concurrent queries, and folding twice
// gives the identical result.
func TestInformationScorerPurity(t *testing.T) {
	candidates, keys := pool(
		[]scoring.Contribution[uint64, float64]{text(3, 1), vector(0.6)},
		[]scoring.Contribution[uint64, float64]{text(1, 1), vector(0.4)},
	)
	shared := scoring.NewInformationScorer[uint64, float64]()
	bound := shared.Observe(scoring.Observation[uint64, float64]{Candidates: candidates, Keys: keys})

	if first, second := bound.Score(candidates[1]), bound.Score(candidates[1]); first != second {
		t.Errorf("two folds of one list = %v then %v, want identical", first, second)
	}
	if got := shared.Score(candidates[1]); got != 3.6 {
		t.Errorf("shared instance folds %v after a binding, want the unbound seed mass 3.6", got)
	}
}
