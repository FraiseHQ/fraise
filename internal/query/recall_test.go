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

// These tests live in the internal `query` package (not query_test) so they can
// populate the unexported `context` field of the query types.
package query

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/FraiseHQ/fraise/internal/containers"
)

// fakeHasher records the last value it hashed and returns a deterministic,
// readable key. The recall, remember and stream tests share it.
type fakeHasher struct{ last string }

func (h *fakeHasher) Hash(s string) string {
	h.last = s
	return "H(" + s + ")"
}

func (h *fakeHasher) Seed() uint64 {
	return 0
}

func TestRecallIsWrite(t *testing.T) {
	var r Recall[string, float32]
	if r.IsWrite() {
		t.Error("Recall.IsWrite() = true, want false")
	}
}

func TestRecallGetGraphID(t *testing.T) {
	r := Recall[string, float32]{context: QueryContext{GraphID: 4}}
	if got := r.GetGraphID(); got != 4 {
		t.Errorf("GetGraphID() = %d, want 4", got)
	}
}

// SetGraphID has a pointer receiver, so the update persists when called
// through a pointer (matching Remember.SetGraphID).
func TestRecallSetGraphIDPersists(t *testing.T) {
	r := &Recall[string, float32]{context: QueryContext{GraphID: 1}}
	r.SetGraphID(9)
	if got := r.GetGraphID(); got != 9 {
		t.Errorf("after SetGraphID(9), GetGraphID() = %d, want 9", got)
	}
}

func TestRecallHash(t *testing.T) {
	r := Recall[string, float32]{
		Keywords: []string{"qu", "ick"},
		Vector:   containers.NewVector[string]([]float32{0.5, 0.25}),
		Entities: []string{"alice"},
		Topics:   []string{"weather"},
		Parameters: QueryParameters[string, float32]{
			Depth: 2,
			Top:   5,
			Since: TimeOperand[string, float32]{Value: containers.RelativeTime[string]{Dur: time.Hour}},
			Until: TimeOperand[string, float32]{Value: containers.AbsoluteTime[string]{T: time.Date(2026, time.June, 18, 12, 0, 0, 0, time.UTC)}},
		},
		context: QueryContext{GraphID: 3},
	}
	h := &fakeHasher{}

	// Hash folds in the graph, the length-prefixed keyword, entity and topic
	// lists, depth, top, the time bounds and the bound vector. Queries that
	// differ in any of these get distinct cache keys.
	const want = "g=3|kw=2:qu3:ick|en=5:alice|to=7:weather|d=2|t=5" +
		"|s=H(r1h0m0s)|u=H(a2026-06-18T12:00:00Z)|vec=H(0x1p-01\x000x1p-02)"
	if got := r.Hash(h); got != "H("+want+")" {
		t.Errorf("Hash() = %q, want %q", got, "H("+want+")")
	}
	if h.last != want {
		t.Errorf("hasher received %q, want %q", h.last, want)
	}
}

func TestRecallHashEmpty(t *testing.T) {
	var r Recall[string, float32]
	h := &fakeHasher{}
	const want = "g=0|kw=|en=|to=|d=0|t=0|s=|u=|vec=H()"
	if got := r.Hash(h); got != "H("+want+")" {
		t.Errorf("Hash() = %q, want %q", got, "H("+want+")")
	}
}

// TestRecallHashFoldsInANestedQuery pins the material of a bound that holds a
// nested query. The bound contributes "q" and the nested query's own key, where
// a time written out contributes its own rendering. So the nested query's
// clock, terms and graph all reach the recall's cache key, and a recall bounded
// by the answer of one when never runs the plan of a recall bounded by another.
func TestRecallHashFoldsInANestedQuery(t *testing.T) {
	r := Recall[string, float32]{
		Keywords: []string{"alice"},
		Parameters: QueryParameters[string, float32]{
			Top: 5,
			Until: TimeOperand[string, float32]{Query: &When[string, float32]{
				Clock: containers.Said,
				Recall: Recall[string, float32]{
					Keywords:   []string{"race"},
					Parameters: QueryParameters[string, float32]{Top: 1},
					context:    QueryContext{GraphID: 3},
				},
			}},
		},
		context: QueryContext{GraphID: 3},
	}
	h := &fakeHasher{}

	const want = "g=3|kw=5:alice|en=|to=|d=0|t=5|s=" +
		"|u=qH(c=1|r=H(g=3|kw=4:race|en=|to=|d=0|t=1|s=|u=|vec=H()))|vec=H()"
	got := r.Hash(h)
	if diff := cmp.Diff("H("+want+")", got); diff != "" {
		t.Errorf("Hash() mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want, h.last); diff != "" {
		t.Errorf("hasher material mismatch (-want +got):\n%s", diff)
	}
}

// TestRecallHashDistinguishesParameters pins the contract. Recalls that differ
// only in graph, depth, top, time bounds, the bound vector or how their text
// splits into keywords and anchors must not share a cache key. Otherwise the
// engine hands back another query's plan. A depth:2 query could get a depth:1
// result, results seeded by another query's vector, or another query's terms.
func TestRecallHashDistinguishesParameters(t *testing.T) {
	base := func() Recall[string, float32] {
		return Recall[string, float32]{Keywords: []string{"mercury"}}
	}
	variants := map[string]Recall[string, float32]{
		"base":    base(),
		"depth":   func() Recall[string, float32] { r := base(); r.Parameters.Depth = 2; return r }(),
		"top":     func() Recall[string, float32] { r := base(); r.Parameters.Top = 5; return r }(),
		"graph":   func() Recall[string, float32] { r := base(); r.context.GraphID = 1; return r }(),
		"keyword": func() Recall[string, float32] { r := base(); r.Keywords = []string{"venus"}; return r }(),
		"keywords-joined": func() Recall[string, float32] {
			r := base()
			r.Keywords = []string{"ab"}
			return r
		}(),
		"keywords-split": func() Recall[string, float32] {
			r := base()
			r.Keywords = []string{"a", "b"}
			return r
		}(),
		"keywords-nul": func() Recall[string, float32] {
			r := base()
			r.Keywords = []string{"a\x00b"}
			return r
		}(),
		"keywords-none": func() Recall[string, float32] {
			r := base()
			r.Keywords = nil
			return r
		}(),
		"keywords-empty-phrase": func() Recall[string, float32] {
			r := base()
			r.Keywords = []string{""}
			return r
		}(),
		"keyword-spells-entity": func() Recall[string, float32] {
			r := base()
			r.Keywords = []string{"mercury|en=venus"}
			return r
		}(),
		"entity": func() Recall[string, float32] {
			r := base()
			r.Entities = []string{"venus"}
			return r
		}(),
		"since-1h": func() Recall[string, float32] {
			r := base()
			r.Parameters.Since.Value = containers.RelativeTime[string]{Dur: time.Hour}
			return r
		}(),
		"since-30d": func() Recall[string, float32] {
			r := base()
			r.Parameters.Since.Value = containers.RelativeTime[string]{Dur: 30 * 24 * time.Hour}
			return r
		}(),
		"until": func() Recall[string, float32] {
			r := base()
			r.Parameters.Until.Value = containers.AbsoluteTime[string]{T: time.Date(2026, time.June, 18, 0, 0, 0, 0, time.UTC)}
			return r
		}(),
		"since-when": func() Recall[string, float32] {
			r := base()
			r.Parameters.Since.Query = &When[string, float32]{Recall: Recall[string, float32]{Keywords: []string{"race"}}}
			return r
		}(),
		"until-when": func() Recall[string, float32] {
			r := base()
			r.Parameters.Until.Query = &When[string, float32]{Recall: Recall[string, float32]{Keywords: []string{"race"}}}
			return r
		}(),
		"since-when-said": func() Recall[string, float32] {
			r := base()
			r.Parameters.Since.Query = &When[string, float32]{Clock: containers.Said, Recall: Recall[string, float32]{Keywords: []string{"race"}}}
			return r
		}(),
		"since-when-other-term": func() Recall[string, float32] {
			r := base()
			r.Parameters.Since.Query = &When[string, float32]{Recall: Recall[string, float32]{Keywords: []string{"wedding"}}}
			return r
		}(),
		"since-when-top": func() Recall[string, float32] {
			r := base()
			r.Parameters.Since.Query = &When[string, float32]{Recall: Recall[string, float32]{
				Keywords:   []string{"race"},
				Parameters: QueryParameters[string, float32]{Top: 3},
			}}
			return r
		}(),
		"vector-a": func() Recall[string, float32] {
			r := base()
			r.Vector = containers.NewVector[string]([]float32{1, 0})
			return r
		}(),
		"vector-b": func() Recall[string, float32] {
			r := base()
			r.Vector = containers.NewVector[string]([]float32{0, 1})
			return r
		}(),
	}

	seen := make(map[string]string)
	for name, r := range variants {
		key := r.Hash(&fakeHasher{})
		if other, clash := seen[key]; clash {
			t.Errorf("hash collision: %q and %q both produced %q", name, other, key)
		}
		seen[key] = name
	}
}

// TestRecallEqual pins the check the engine confirms a plan-cache hit with.
// A recall differing from base in any field Hash folds in is not equal to it,
// so a key it happens to share with base never runs base's plan. Keyword order
// counts, as it does in the material. A remember is never equal to a recall.
func TestRecallEqual(t *testing.T) {
	base := func() *Recall[string, float32] {
		return &Recall[string, float32]{
			Keywords: []string{"mercury", "venus"},
			Vector:   containers.NewVector[string]([]float32{1, 0}),
			Entities: []string{"alice"},
			Topics:   []string{"planets"},
			Parameters: QueryParameters[string, float32]{
				Depth: 1,
				Top:   5,
				Since: TimeOperand[string, float32]{Value: containers.RelativeTime[string]{Dur: time.Hour}},
				Until: TimeOperand[string, float32]{Value: containers.AbsoluteTime[string]{T: time.Date(2026, time.June, 18, 0, 0, 0, 0, time.UTC)}},
			},
			context: QueryContext{GraphID: 2},
		}
	}
	with := func(f func(r *Recall[string, float32])) *Recall[string, float32] {
		r := base()
		f(r)
		return r
	}
	tests := map[string]struct {
		other Query[string, float32]
		want  bool
	}{
		"same":          {base(), true},
		"graph":         {with(func(r *Recall[string, float32]) { r.context.GraphID = 3 }), false},
		"keywords":      {with(func(r *Recall[string, float32]) { r.Keywords = []string{"mercury"} }), false},
		"keyword order": {with(func(r *Recall[string, float32]) { r.Keywords = []string{"venus", "mercury"} }), false},
		"entities":      {with(func(r *Recall[string, float32]) { r.Entities = []string{"bob"} }), false},
		"topics":        {with(func(r *Recall[string, float32]) { r.Topics = nil }), false},
		"depth":         {with(func(r *Recall[string, float32]) { r.Parameters.Depth = 2 }), false},
		"top":           {with(func(r *Recall[string, float32]) { r.Parameters.Top = 6 }), false},
		"since": {with(func(r *Recall[string, float32]) {
			r.Parameters.Since.Value = containers.RelativeTime[string]{Dur: time.Minute}
		}), false},
		"no since": {with(func(r *Recall[string, float32]) { r.Parameters.Since = TimeOperand[string, float32]{} }), false},
		"since a when": {with(func(r *Recall[string, float32]) {
			r.Parameters.Since = TimeOperand[string, float32]{Query: &When[string, float32]{Recall: Recall[string, float32]{Keywords: []string{"race"}}}}
		}), false},
		"until": {with(func(r *Recall[string, float32]) {
			r.Parameters.Until.Value = containers.RelativeTime[string]{Dur: time.Hour}
		}), false},
		"no until":  {with(func(r *Recall[string, float32]) { r.Parameters.Until = TimeOperand[string, float32]{} }), false},
		"vector":    {with(func(r *Recall[string, float32]) { r.Vector = containers.NewVector[string]([]float32{0, 1}) }), false},
		"no vector": {with(func(r *Recall[string, float32]) { r.Vector = containers.Vector[string, float32]{} }), false},
		"remember":  {&Remember[string, float32]{Value: "mercury venus", context: QueryContext{GraphID: 2}}, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := base().Equal(tt.other); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRecallEqualComparesNestedQueries pins Equal over a bound that holds a
// nested query. Two recalls are equal when the queries in their bounds are, and
// differ as soon as those read another clock, search other terms, or sit in the
// other bound. The plan cache would otherwise bound one recall by the answer of
// another's when.
func TestRecallEqualComparesNestedQueries(t *testing.T) {
	when := func(clock containers.Clock, keyword string) *When[string, float32] {
		return &When[string, float32]{Clock: clock, Recall: Recall[string, float32]{Keywords: []string{keyword}}}
	}
	base := func() *Recall[string, float32] {
		return &Recall[string, float32]{
			Keywords:   []string{"alice"},
			Parameters: QueryParameters[string, float32]{Until: TimeOperand[string, float32]{Query: when(containers.Happened, "race")}},
		}
	}
	with := func(f func(r *Recall[string, float32])) *Recall[string, float32] {
		r := base()
		f(r)
		return r
	}
	tests := map[string]struct {
		other *Recall[string, float32]
		want  bool
	}{
		"same": {base(), true},
		"another clock": {with(func(r *Recall[string, float32]) {
			r.Parameters.Until.Query = when(containers.Said, "race")
		}), false},
		"another term": {with(func(r *Recall[string, float32]) {
			r.Parameters.Until.Query = when(containers.Happened, "wedding")
		}), false},
		"the other bound": {with(func(r *Recall[string, float32]) {
			r.Parameters.Since, r.Parameters.Until = r.Parameters.Until, TimeOperand[string, float32]{}
		}), false},
		"written out": {with(func(r *Recall[string, float32]) {
			r.Parameters.Until = TimeOperand[string, float32]{Value: containers.RelativeTime[string]{Dur: time.Hour}}
		}), false},
		"no bound": {with(func(r *Recall[string, float32]) {
			r.Parameters.Until = TimeOperand[string, float32]{}
		}), false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := base().Equal(tc.other); got != tc.want {
				t.Errorf("Equal() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRecallSinceUntil pins the bound a recall resolves for a time it writes
// out: the start of that time, on the clock a bare bound reads, which is when
// a fact happened. A duration counts back from now. A date at day precision
// names the day's first instant. A clause that is absent, or that holds a
// nested query, resolves to the zero bound, which the graph reads as open.
func TestRecallSinceUntil(t *testing.T) {
	now := time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC)
	day := time.Date(2026, time.June, 18, 0, 0, 0, 0, time.UTC)
	when := &When[string, float32]{Recall: Recall[string, float32]{Keywords: []string{"race"}}}

	tests := map[string]struct {
		operand TimeOperand[string, float32]
		want    containers.Bound
	}{
		"a duration counts back from now": {
			operand: TimeOperand[string, float32]{Value: containers.RelativeTime[string]{Dur: time.Hour}},
			want:    containers.Bound{At: now.Add(-time.Hour)},
		},
		"an instant is itself": {
			operand: TimeOperand[string, float32]{Value: containers.AbsoluteTime[string]{T: now.Add(48 * time.Hour)}},
			want:    containers.Bound{At: now.Add(48 * time.Hour)},
		},
		"a day is its first instant": {
			operand: TimeOperand[string, float32]{Value: containers.AbsoluteTime[string]{T: day, Precision: containers.Day}},
			want:    containers.Bound{At: day},
		},
		"a range is its start": {
			operand: TimeOperand[string, float32]{Value: containers.RangeTime[string]{
				From: containers.AbsoluteTime[string]{T: day, Precision: containers.Day},
				To:   containers.AbsoluteTime[string]{T: day.AddDate(0, 0, 3), Precision: containers.Day},
			}},
			want: containers.Bound{At: day},
		},
		"an absent clause is open": {
			operand: TimeOperand[string, float32]{},
			want:    containers.Bound{},
		},
		"a nested query is read from its answer": {
			operand: TimeOperand[string, float32]{Query: when},
			want:    containers.Bound{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := Recall[string, float32]{Parameters: QueryParameters[string, float32]{Since: tc.operand, Until: tc.operand}}

			if diff := cmp.Diff(tc.want, r.Since(now)); diff != "" {
				t.Errorf("Since(now) mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.want, r.Until(now)); diff != "" {
				t.Errorf("Until(now) mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRecallSetGraphIDReachesNestedQueries pins that a query nested in a time
// bound reads the graph of the recall that holds it. The selector is written
// once, on the recall, and the nested query takes no selector of its own. If
// it kept its own graph, the bound would be read from one graph and applied to
// the facts of another.
func TestRecallSetGraphIDReachesNestedQueries(t *testing.T) {
	since := &When[string, float32]{Recall: Recall[string, float32]{Keywords: []string{"race"}}}
	until := &When[string, float32]{Recall: Recall[string, float32]{Keywords: []string{"wedding"}}}
	r := &Recall[string, float32]{
		Keywords: []string{"alice"},
		Parameters: QueryParameters[string, float32]{
			Since: TimeOperand[string, float32]{Query: since},
			Until: TimeOperand[string, float32]{Query: until},
		},
	}

	r.SetGraphID(4)

	got := map[string]uint8{"recall": r.GetGraphID(), "since": since.GetGraphID(), "until": until.GetGraphID()}
	want := map[string]uint8{"recall": 4, "since": 4, "until": 4}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("graph ids after SetGraphID(4) mismatch (-want +got):\n%s", diff)
	}
}

// TestRecallPlanHangsNestedStreams pins how a nested query is planned: through
// its own Plan, into a stream of its own that hangs on the recall's stream
// under the bound that holds it. The engine and the scheduler handle the one
// outer stream. A bound written out has no stream.
func TestRecallPlanHangsNestedStreams(t *testing.T) {
	nested := &When[string, float32]{Clock: containers.Said, Recall: Recall[string, float32]{Keywords: []string{"race"}}}
	tests := map[string]struct {
		parameters QueryParameters[string, float32]
		wantSince  Query[string, float32]
		wantUntil  Query[string, float32]
	}{
		"since": {
			parameters: QueryParameters[string, float32]{Since: TimeOperand[string, float32]{Query: nested}},
			wantSince:  nested,
		},
		"until": {
			parameters: QueryParameters[string, float32]{Until: TimeOperand[string, float32]{Query: nested}},
			wantUntil:  nested,
		},
		"written out": {
			parameters: QueryParameters[string, float32]{
				Since: TimeOperand[string, float32]{Value: containers.RelativeTime[string]{Dur: time.Hour}},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Recall[string, float32]{Keywords: []string{"alice"}, Parameters: tc.parameters}

			s, err := r.Plan(nil)
			if err != nil {
				t.Fatalf("Plan() err = %v, want nil", err)
			}

			if s.Query != Query[string, float32](r) {
				t.Errorf("Plan() stream.Query = %v, want the recall", s.Query)
			}
			for clause, pair := range map[string]struct {
				got  *Stream[string, float32]
				want Query[string, float32]
			}{"since": {s.Since, tc.wantSince}, "until": {s.Until, tc.wantUntil}} {
				switch {
				case pair.want == nil && pair.got != nil:
					t.Errorf("Plan() hung a %s stream for %v, want none", clause, pair.got.Query)
				case pair.want != nil && pair.got == nil:
					t.Errorf("Plan() hung no %s stream, want one for the nested query", clause)
				case pair.want != nil && pair.got.Query != pair.want:
					t.Errorf("Plan() %s stream.Query = %v, want the nested query", clause, pair.got.Query)
				}
			}
		})
	}
}

func TestRecallPlan(t *testing.T) {
	var r Recall[string, float32]
	s, err := r.Plan(nil)
	if err != nil {
		t.Fatalf("Plan() err = %v, want nil", err)
	}
	if s == nil {
		t.Fatal("Plan() stream = nil, want a ready stream")
	}
	if s.Query != Query[string, float32](&r) {
		t.Errorf("Plan() stream.Query = %v, want the receiver", s.Query)
	}
	select {
	case <-s.Done():
		t.Error("Plan() stream is already done; it must stay open until committed")
	default:
	}
}
