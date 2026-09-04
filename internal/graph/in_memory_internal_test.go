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

// Same-package pins of the collection layer: findNeighbours is handed
// hand-built seed contributions, so the observation arithmetic — which
// anchors weigh in the background, which are expanded, what each graph
// contribution records — turns on exact hinge values that BM25 masses from
// the public surface cannot be chosen to hit.

package graph

import (
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
)

// collectFixture builds the smallest graph on which every collection theorem
// is visible:
//
//	cluster (topic, degree 3): members f1, f2, f3
//	hub     (topic, degree 6): members h1 .. h6
//	giant   (topic, degree 10): members u1 .. u10, never touched by a seed
//
// Hand-built seed contributions give masses m(f1)=8, m(f2)=6, m(h1)=2 — exact
// binary values, so every sum below is exact regardless of accumulation
// order — and the arithmetic is:
//
//	ρ₀      = (8+6+2) / (3+6) = 16/9   (the giant holds no seed: untouched)
//	cluster = M 14 > 3·ρ₀ ≈ 5.33       (admitted: expanded to its members)
//	hub     = M  2 ≤ 6·ρ₀ ≈ 10.67      (silent: pruned, but still in the null)
type collectFixture struct {
	g            *InMemoryGraph[uint64, float64]
	f1, f2, f3   uint64
	h            [6]uint64
	u            [10]uint64
	cluster, hub uint64
	candidates   scoring.Candidates[uint64, float64]
	seeds        []uint64
	background   float64
}

func newCollectFixture(t *testing.T) *collectFixture {
	t.Helper()
	g := NewGraph[uint64, float64](config.New())
	g.SetTraversal(NewExcessTraversal[uint64, float64]())
	now := time.Now()

	fact := func(value string) uint64 {
		f := Fact[uint64]{NodeAttributes: NodeAttributes{Value: value, Timestamp: now}, Hasher: g.hasher}
		if err := g.Set(f); err != nil {
			t.Fatalf("Set(%q) = %v, want nil", value, err)
		}
		return f.Key()
	}
	topic := func(value string, members ...uint64) uint64 {
		tp := &Topic[uint64]{NodeAttributes: NodeAttributes{Value: value, Timestamp: now}, Hasher: g.hasher}
		if err := g.Set(tp); err != nil {
			t.Fatalf("Set(topic %q) = %v, want nil", value, err)
		}
		for _, m := range members {
			f, _ := g.idToNodes[m].(Fact[uint64])
			edge := IsAbout[uint64]{NodeAttributes: NodeAttributes{Timestamp: now}, Fact: &f, Topic: tp, Hasher: g.hasher}
			if err := g.Set(edge); err != nil {
				t.Fatalf("Set(edge to %q) = %v, want nil", value, err)
			}
		}
		return tp.Key()
	}

	fx := &collectFixture{g: g}
	fx.f1 = fact("cluster fact one")
	fx.f2 = fact("cluster fact two")
	fx.f3 = fact("cluster fact three")
	for i := range fx.h {
		fx.h[i] = fact("hub fact " + string(rune('a'+i)))
	}
	for i := range fx.u {
		fx.u[i] = fact("giant fact " + string(rune('a'+i)))
	}
	fx.cluster = topic("cluster", fx.f1, fx.f2, fx.f3)
	fx.hub = topic("hub", fx.h[:]...)
	topic("giant", fx.u[:]...)

	// Hand-built seed contributions: the scorer's seed fusion (background 0)
	// sums them, so these ARE the masses.
	fx.candidates = scoring.Candidates[uint64, float64]{
		fx.f1:   {{Src: scoring.SrcText, Score: 8, Rank: 0, Count: 1}},
		fx.f2:   {{Src: scoring.SrcText, Score: 6, Rank: 1, Count: 1}},
		fx.h[0]: {{Src: scoring.SrcText, Score: 2, Rank: 2, Count: 1}},
	}
	fx.seeds = []uint64{fx.f1, fx.f2, fx.h[0]}
	// The fixture's topics are named: the traversal runs only through an
	// anchor the query names, and naming all three keeps the filter from
	// narrowing anything.
	fx.background = fx.g.findNeighbours(fx.seeds, fx.candidates, []string{"cluster", "hub", "giant"}, nil, 2)
	return fx
}

// graphContributions returns the SrcGraph entries pooled for key.
func graphContributions(candidates scoring.Candidates[uint64, float64], key uint64) []scoring.Contribution[uint64, float64] {
	var out []scoring.Contribution[uint64, float64]
	for _, c := range candidates[key] {
		if c.Src == scoring.SrcGraph {
			out = append(out, c)
		}
	}
	return out
}

// TestCollectBackgroundWeighsAllTouchedAnchors pins the null model's
// denominator, the exact seam where a regression would hide between the
// admission prune and the background: the silent hub is pruned from
// expansion, yet its mass and degree still weigh in ρ₀ — while the untouched
// giant, which the traversal never saw, does not. Excluding the hub would
// give 14/3; counting the giant would give 16/19; the pin is 16/9 exactly.
func TestCollectBackgroundWeighsAllTouchedAnchors(t *testing.T) {
	fx := newCollectFixture(t)
	if want := 16.0 / 9.0; fx.background != want {
		t.Errorf("background = %v, want exactly %v (all touched anchors, silent included; untouched excluded)", fx.background, want)
	}
}

// TestCollectExpandsOnlyAboveBackgroundAnchors is the admission prune,
// Property 5.3's collection half: the fair-share hub is never expanded — its
// members pool nothing, its own seed keeps a graph-free list — while the
// cluster is expanded to every member.
func TestCollectExpandsOnlyAboveBackgroundAnchors(t *testing.T) {
	fx := newCollectFixture(t)

	for i, member := range fx.h[1:] {
		if _, pooled := fx.candidates[member]; pooled {
			t.Errorf("hub member h[%d] was pooled by a silent anchor, want nothing", i+1)
		}
	}
	if got := graphContributions(fx.candidates, fx.h[0]); len(got) != 0 {
		t.Errorf("the silent hub's own seed carries graph observations %+v, want none", got)
	}
	for _, member := range []uint64{fx.f1, fx.f2, fx.f3} {
		if got := graphContributions(fx.candidates, member); len(got) != 1 {
			t.Errorf("cluster member %d has %d graph observations, want exactly 1", member, len(got))
		}
	}
	for i, member := range fx.u {
		if _, pooled := fx.candidates[member]; pooled {
			t.Errorf("giant member u[%d] was pooled; untouched anchors hold no mass to observe", i)
		}
	}
}

// TestCollectRecordsObservationsNotPolicy pins what a graph contribution
// carries: the funding anchor's full observed mass, its identity, degree and
// funding-seed count — and nothing hinged, subtracted or attenuated. The
// cluster's non-seed member and its strongest seed record the identical
// observation; only the scorer's self-exclusion will treat them differently.
func TestCollectRecordsObservationsNotPolicy(t *testing.T) {
	fx := newCollectFixture(t)

	want := scoring.Contribution[uint64, float64]{Src: scoring.SrcGraph, Score: 14, Via: fx.cluster, Degree: 3, Count: 2}
	for _, member := range []uint64{fx.f1, fx.f3} {
		got := graphContributions(fx.candidates, member)
		if len(got) != 1 || got[0] != want {
			t.Errorf("member %d observation = %+v, want %+v — collection must not apply policy", member, got, want)
		}
	}
}

// TestCollectMemberOfTwoAnchorsIsTwoObservations pins Parents-driven
// incidence end to end: a fact on two admitted anchors pools one observation
// per anchor, because each anchor's surplus is a separate claim about it.
// Two small clusters alone can never both clear a background they define
// between them, so a large silent hub supplies the ballast that lowers ρ₀
// under both: left and right observe 16 each (degree 2, share 2·ρ₀ = 6.8),
// the hub observes 2 on degree 6, ρ₀ = 34/10.
func TestCollectMemberOfTwoAnchorsIsTwoObservations(t *testing.T) {
	g := NewGraph[uint64, float64](config.New())
	g.SetTraversal(NewExcessTraversal[uint64, float64]())
	now := time.Now()

	fact := func(value string) (uint64, Fact[uint64]) {
		f := Fact[uint64]{NodeAttributes: NodeAttributes{Value: value, Timestamp: now}, Hasher: g.hasher}
		if err := g.Set(f); err != nil {
			t.Fatalf("Set(%q) = %v, want nil", value, err)
		}
		return f.Key(), f
	}
	s1, s1f := fact("seed one")
	s2, s2f := fact("seed two")
	s3, s3f := fact("seed three on the hub")
	shared, sharedf := fact("the shared member")

	link := func(name string, facts ...Fact[uint64]) {
		tp := &Topic[uint64]{NodeAttributes: NodeAttributes{Value: name, Timestamp: now}, Hasher: g.hasher}
		if err := g.Set(tp); err != nil {
			t.Fatalf("Set(topic %q) = %v, want nil", name, err)
		}
		for i := range facts {
			edge := IsAbout[uint64]{NodeAttributes: NodeAttributes{Timestamp: now}, Fact: &facts[i], Topic: tp, Hasher: g.hasher}
			if err := g.Set(edge); err != nil {
				t.Fatalf("Set(edge %q) = %v, want nil", name, err)
			}
		}
	}
	link("left", s1f, sharedf)
	link("right", s2f, sharedf)
	ballast := make([]Fact[uint64], 0, 6)
	ballast = append(ballast, s3f)
	for i := 0; i < 5; i++ {
		_, f := fact("hub ballast " + string(rune('a'+i)))
		ballast = append(ballast, f)
	}
	link("hub", ballast...)

	candidates := scoring.Candidates[uint64, float64]{
		s1: {{Src: scoring.SrcText, Score: 16, Rank: 0, Count: 1}},
		s2: {{Src: scoring.SrcText, Score: 16, Rank: 1, Count: 1}},
		s3: {{Src: scoring.SrcText, Score: 2, Rank: 2, Count: 1}},
	}
	g.findNeighbours([]uint64{s1, s2, s3}, candidates, []string{"left", "right", "hub"}, nil, 2)

	got := graphContributions(candidates, shared)
	if len(got) != 2 {
		t.Fatalf("shared member pooled %d graph observations, want one per funding anchor (2): %+v", len(got), got)
	}
	if got[0].Via == got[1].Via {
		t.Errorf("both observations name anchor %d, want one per distinct anchor", got[0].Via)
	}
	for _, c := range got {
		if c.Score != 16 || c.Degree != 2 || c.Count != 1 {
			t.Errorf("observation %+v, want each cluster's own mass 16, degree 2, one funding seed", c)
		}
	}
}
