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

func text(score float64) scoring.Contribution[uint64, float64] {
	return scoring.Contribution[uint64, float64]{Src: scoring.SrcText, Score: score, Count: 1}
}

func vector(similarity float64) scoring.Contribution[uint64, float64] {
	return scoring.Contribution[uint64, float64]{Src: scoring.SrcVector, Score: similarity, Count: 1}
}

// TestInformationScorerFoldsWeightedBits pins the fold on a pool small enough
// to work by hand. Four candidates; text masses 3, 1, 0, 0; vector similarities
// 0.6, 0.5, 0.4 and none, so the reading's floor is 0.4 and the vector channel
// is 0.2, 0.1, 0, 0 above it. With n = 4 each channel's share p gives
// r = 4p and its information I = Σ p log₂ 4p: text p = (3/4, 1/4), vector
// p = (2/3, 1/3). The weights are the informations' shares, and a fact scores
// Σ w log₂(1 + r). The floor hit carries no vector bits, and a fact neither
// channel saw scores 0.
func TestInformationScorerFoldsWeightedBits(t *testing.T) {
	candidates, keys := pool(
		[]scoring.Contribution[uint64, float64]{text(3), vector(0.6)},
		[]scoring.Contribution[uint64, float64]{text(1), vector(0.5)},
		[]scoring.Contribution[uint64, float64]{vector(0.4)},
		[]scoring.Contribution[uint64, float64]{{Src: scoring.SrcGraph, Score: 0, Via: 9, Degree: 4, Count: 1}},
	)
	scorer := scoring.NewInformationScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{Candidates: candidates, Keys: keys})

	textInfo := 0.75*math.Log2(3) + 0.25*math.Log2(1)
	floor := 0.4
	vectorTotal := (0.6 - floor) + (0.5 - floor)
	p1, p2 := (0.6-floor)/vectorTotal, (0.5-floor)/vectorTotal
	vectorInfo := p1*math.Log2(4*p1) + p2*math.Log2(4*p2)
	wText, wVector := textInfo/(textInfo+vectorInfo), vectorInfo/(textInfo+vectorInfo)

	want := map[uint64]float64{
		1: wText*math.Log2(1+4*3/4.0) + wVector*math.Log2(1+4*p1),
		2: wText*math.Log2(1+4*1/4.0) + wVector*math.Log2(1+4*p2),
		3: 0,
		4: 0,
	}
	for key, w := range want {
		if got := scorer.Score(candidates[key]); math.Abs(got-w) > 1e-12 {
			t.Errorf("Score(candidate %d) = %v, want %v", key, got, w)
		}
	}
}

// TestInformationScorerSeedFusionIsTheSeedMass pins the fold before a pool is
// bound: the traversal is seeded with the excess methodology's seed mass, the
// text mass plus the similarity weighted by the text's weakness, so the graph
// observes the same masses whichever fold ranks the hits.
func TestInformationScorerSeedFusionIsTheSeedMass(t *testing.T) {
	list := []scoring.Contribution[uint64, float64]{text(2), vector(0.5)}
	seeding := scoring.NewInformationScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{TextMissed: 1})
	if got := seeding.Score(list); got != 3 {
		t.Errorf("seed fold = %v, want text 2 + (1 + 1)·0.5 = 3", got)
	}
}

// TestInformationScorerIgnoresAChannelThatCannotTellCandidatesApart pins the
// weights' reason to exist: candidates the text scores alike give the text no
// information about which matters, I = Σ (1/3) log₂ 1 = 0, so the text gets no
// weight and the vector, which tells them apart, ranks them alone: log₂(1 + 2)
// and log₂(1 + 1) for the hits 0.2 and 0.1 above the floor, 0 for the floor
// itself. Equal text mass must not dilute the one channel with something to
// say.
func TestInformationScorerIgnoresAChannelThatCannotTellCandidatesApart(t *testing.T) {
	candidates, keys := pool(
		[]scoring.Contribution[uint64, float64]{text(2), vector(0.6)},
		[]scoring.Contribution[uint64, float64]{text(2), vector(0.5)},
		[]scoring.Contribution[uint64, float64]{text(2), vector(0.4)},
	)
	scorer := scoring.NewInformationScorer[uint64, float64]().Observe(scoring.Observation[uint64, float64]{Candidates: candidates, Keys: keys})

	want := map[uint64]float64{1: math.Log2(3), 2: 1, 3: 0}
	for key, w := range want {
		if got := scorer.Score(candidates[key]); math.Abs(got-w) > 1e-12 {
			t.Errorf("Score(candidate %d) = %v, want the vector's bits alone %v", key, got, w)
		}
	}
}

// TestInformationScorerWeighsTheVectorByTheTextsWeakness pins where the vector
// weight comes from: the text's weakness, not the vector's own scores. The same
// pool scored with the text holding half the question's information gives the
// vector-only candidate more and the text-only candidate less than with the
// text holding all of it.
func TestInformationScorerWeighsTheVectorByTheTextsWeakness(t *testing.T) {
	candidates, keys := pool(
		[]scoring.Contribution[uint64, float64]{text(2)},
		[]scoring.Contribution[uint64, float64]{vector(0.6)},
		[]scoring.Contribution[uint64, float64]{vector(0.4)},
	)
	scorer := scoring.NewInformationScorer[uint64, float64]()
	strong := scorer.Observe(scoring.Observation[uint64, float64]{Candidates: candidates, Keys: keys})
	weak := scorer.Observe(scoring.Observation[uint64, float64]{TextMissed: 1, Candidates: candidates, Keys: keys})

	if s, w := strong.Score(candidates[2]), weak.Score(candidates[2]); w <= s {
		t.Errorf("vector-only candidate scores %v with weak text, want above its %v with strong text", w, s)
	}
	if s, w := strong.Score(candidates[1]), weak.Score(candidates[1]); w >= s {
		t.Errorf("text-only candidate scores %v with weak text, want below its %v with strong text", w, s)
	}
}

// TestInformationScorerIsScaleFreePerChannel pins that a channel's units drop
// out: scaling every text mass by four leaves every score unchanged, since a
// fact's share of its channel and the channel's information are ratios. The
// channels are commensurable without being calibrated against each other.
func TestInformationScorerIsScaleFreePerChannel(t *testing.T) {
	build := func(scale float64) (scoring.Candidates[uint64, float64], []uint64) {
		return pool(
			[]scoring.Contribution[uint64, float64]{text(3 * scale), vector(0.6)},
			[]scoring.Contribution[uint64, float64]{text(1 * scale), vector(0.5)},
			[]scoring.Contribution[uint64, float64]{vector(0.4)},
		)
	}
	plain, keys := build(1)
	scaled, _ := build(4)
	scorer := scoring.NewInformationScorer[uint64, float64]()
	before := scorer.Observe(scoring.Observation[uint64, float64]{Candidates: plain, Keys: keys})
	after := scorer.Observe(scoring.Observation[uint64, float64]{Candidates: scaled, Keys: keys})
	for _, key := range keys {
		if got, want := after.Score(scaled[key]), before.Score(plain[key]); got != want {
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
		[]scoring.Contribution[uint64, float64]{text(3), vector(0.6)},
		[]scoring.Contribution[uint64, float64]{text(1), vector(0.4)},
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
