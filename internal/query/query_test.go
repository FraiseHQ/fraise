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

package query_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/query"
)

// TestParseRemember checks that a remember query parses to a *query.Remember.
// It carries the phrase value and the graph selector, and reports as a write.
func TestParseRemember(t *testing.T) {
	q := "remember@1 'anne loves the color orange' topic:color topic:preference entity:anne"

	got, _, err := query.Parse[string, float32](q, nil, config.New())
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
	}

	r, ok := got.(*query.Remember[string, float32])
	if !ok {
		t.Fatalf("Parse(%q) returned %T, want *query.Remember", q, got)
	}

	// The phrase value is the raw text: the quotes are query syntax, not data.
	if want := "anne loves the color orange"; r.Value != want {
		t.Errorf("Value = %q, want %q", r.Value, want)
	}
	if want := []string{"anne"}; !reflect.DeepEqual(r.Entities, want) {
		t.Errorf("Entities = %v, want %v", r.Entities, want)
	}
	if want := []string{"color", "preference"}; !reflect.DeepEqual(r.Topics, want) {
		t.Errorf("Topics = %v, want %v", r.Topics, want)
	}
	if id := r.GetGraphID(); id != 1 {
		t.Errorf("GetGraphID() = %d, want 1", id)
	}
	if !r.IsWrite() {
		t.Error("Remember.IsWrite() = false, want true")
	}
}

// TestParseRecall checks that a recall query parses to a *query.Recall. It
// carries its terms, entities, topics and graph selector, and reports as a
// read.
func TestParseRecall(t *testing.T) {
	q := "recall@2 anna bob entity:alice topic:job"

	got, _, err := query.Parse[string, float32](q, nil, config.New())
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
	}

	r, ok := got.(*query.Recall[string, float32])
	if !ok {
		t.Fatalf("Parse(%q) returned %T, want *query.Recall", q, got)
	}

	if want := []string{"anna", "bob"}; !reflect.DeepEqual(r.Keywords, want) {
		t.Errorf("Keywords = %v, want %v", r.Keywords, want)
	}
	if want := []string{"alice"}; !reflect.DeepEqual(r.Entities, want) {
		t.Errorf("Entities = %v, want %v", r.Entities, want)
	}
	if want := []string{"job"}; !reflect.DeepEqual(r.Topics, want) {
		t.Errorf("Topics = %v, want %v", r.Topics, want)
	}
	if id := r.GetGraphID(); id != 2 {
		t.Errorf("GetGraphID() = %d, want 2", id)
	}
	if r.IsWrite() {
		t.Error("Recall.IsWrite() = true, want false")
	}
}

// TestParseRememberCarriesItsTimes pins the times a remember parses. said: is
// the one time the fact was said, and each occurred: a span in which it
// happened, kept in the order written. Each is held as written, at the
// precision of its literal, and resolved when the fact is stored. A remember
// with neither clause holds neither.
func TestParseRememberCarriesItsTimes(t *testing.T) {
	type times struct {
		Said     containers.TimeValue[string]
		Occurred []containers.TimeValue[string]
	}
	day := func(d int) containers.AbsoluteTime[string] {
		return containers.AbsoluteTime[string]{T: may(d), Precision: containers.Day}
	}
	tests := map[string]times{
		"remember 'lap'": {},
		"remember 'lap' said:'2023-05-08'": {
			Said: day(8),
		},
		"remember 'lap' said:2d": {
			Said: containers.RelativeTime[string]{Dur: 48 * time.Hour},
		},
		"remember 'lap' occurred:'2023-05'": {
			Occurred: []containers.TimeValue[string]{
				containers.AbsoluteTime[string]{T: may(1), Precision: containers.Month},
			},
		},
		"remember 'lap' said:'2023-05-08' occurred:'2023-05-07' occurred:'2023-05-10'..'2023-05-16' occurred:'2023-05-20'..": {
			Said: day(8),
			Occurred: []containers.TimeValue[string]{
				day(7),
				containers.RangeTime[string]{From: day(10), To: day(16)},
				containers.RangeTime[string]{From: day(20)},
			},
		},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			parsed, _, err := query.Parse[string, float32](q, nil, config.New())
			if err != nil {
				t.Fatalf("Parse(%q) = %v, want nil", q, err)
			}
			r, ok := parsed.(*query.Remember[string, float32])
			if !ok {
				t.Fatalf("Parse(%q) returned %T, want *query.Remember", q, parsed)
			}

			got := times{Said: r.Said, Occurred: r.Occurred}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("Parse(%q) times mismatch (-want +got):\n%s", q, diff)
			}
		})
	}
}

// TestParseWhen pins what a when statement parses to: a *query.When that reads
// the clock named after its colon, the happened clock when none is named, and
// runs a recall built from the rest of the statement. Without a top clause it
// keeps its best match alone, where a recall keeps the configured default, so
// the time it answers with is one fact's. A when statement takes no selector
// and reads the default graph. It is a read.
func TestParseWhen(t *testing.T) {
	type when struct {
		Clock    containers.Clock
		Keywords []string
		Entities []string
		Topics   []string
		Top      int
		Depth    int
		Graph    uint8
	}
	tests := map[string]when{
		"when race": {
			Clock: containers.Happened, Keywords: []string{"race"}, Top: 1,
		},
		"when:happened race": {
			Clock: containers.Happened, Keywords: []string{"race"}, Top: 1,
		},
		"when:said race top:3": {
			Clock: containers.Said, Keywords: []string{"race"}, Top: 3,
		},
		"when:written race entity:alice topic:running depth:1": {
			Clock:    containers.Written,
			Keywords: []string{"race"},
			Entities: []string{"alice"},
			Topics:   []string{"running"},
			Top:      1,
			Depth:    1,
		},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			parsed, _, err := query.Parse[string, float32](q, nil, config.New())
			if err != nil {
				t.Fatalf("Parse(%q) = %v, want nil", q, err)
			}
			w, ok := parsed.(*query.When[string, float32])
			if !ok {
				t.Fatalf("Parse(%q) returned %T, want *query.When", q, parsed)
			}

			got := when{
				Clock:    w.Clock,
				Keywords: w.Recall.Keywords,
				Entities: w.Recall.Entities,
				Topics:   w.Recall.Topics,
				Top:      w.Recall.Parameters.Top,
				Depth:    w.Recall.Parameters.Depth,
				Graph:    w.GetGraphID(),
			}
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Parse(%q) mismatch (-want +got):\n%s", q, diff)
			}
			if w.IsWrite() {
				t.Errorf("Parse(%q).IsWrite() = true, want false", q)
			}
		})
	}
}

// TestParseRecallNestsTheWhenInItsBound pins what a when in a time bound
// parses to. The bound holds the when as a query and holds no value, and the
// when reads the graph of the recall that holds it, so the bound and the
// search answer from the same facts. A bound written out keeps its value and
// holds no query.
func TestParseRecallNestsTheWhenInItsBound(t *testing.T) {
	type nested struct {
		Clock    containers.Clock
		Keywords []string
		Top      int
		Graph    uint8
	}
	type bounds struct {
		Since, Until           *nested
		SinceValue, UntilValue bool
	}
	tests := map[string]bounds{
		"recall@2 lap until:(when race)": {
			Until: &nested{Clock: containers.Happened, Keywords: []string{"race"}, Top: 1, Graph: 2},
		},
		"recall@2 lap since:(when:said race top:3)": {
			Since: &nested{Clock: containers.Said, Keywords: []string{"race"}, Top: 3, Graph: 2},
		},
		"recall lap since:(when race) until:(when:written trip)": {
			Since: &nested{Clock: containers.Happened, Keywords: []string{"race"}, Top: 1},
			Until: &nested{Clock: containers.Written, Keywords: []string{"trip"}, Top: 1},
		},
		"recall@2 lap since:7d until:(when race)": {
			SinceValue: true,
			Until:      &nested{Clock: containers.Happened, Keywords: []string{"race"}, Top: 1, Graph: 2},
		},
		"recall@2 lap until:'2023-05-07'": {
			UntilValue: true,
		},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			parsed, _, err := query.Parse[string, float32](q, nil, config.New())
			if err != nil {
				t.Fatalf("Parse(%q) = %v, want nil", q, err)
			}
			r, ok := parsed.(*query.Recall[string, float32])
			if !ok {
				t.Fatalf("Parse(%q) returned %T, want *query.Recall", q, parsed)
			}

			describe := func(o query.TimeOperand[string, float32]) *nested {
				if o.Query == nil {
					return nil
				}
				w, ok := o.Query.(*query.When[string, float32])
				if !ok {
					t.Fatalf("Parse(%q) nested a %T in a bound, want *query.When", q, o.Query)
				}
				return &nested{
					Clock:    w.Clock,
					Keywords: w.Recall.Keywords,
					Top:      w.Recall.Parameters.Top,
					Graph:    w.GetGraphID(),
				}
			}
			got := bounds{
				Since:      describe(r.Parameters.Since),
				Until:      describe(r.Parameters.Until),
				SinceValue: r.Parameters.Since.Value != nil,
				UntilValue: r.Parameters.Until.Value != nil,
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("Parse(%q) bounds mismatch (-want +got):\n%s", q, diff)
			}
		})
	}
}

// TestParseErrors checks that malformed or empty queries are rejected with
// ErrParsingFailed and a nil query.
func TestParseErrors(t *testing.T) {
	queries := []string{
		"",                               // empty input
		"recall",                         // a recall must carry at least one seed
		"recall top:3",                   // a modifier scopes a search; it cannot start one
		"forget anna",                    // unknown command keyword
		"when",                           // a when must carry at least one seed, as its recall does
		"when@1 race",                    // a when takes no selector
		"when:heard race",                // a clock the grammar does not name
		"recall lap until:(recall race)", // only a when answers with a time
		"when lap until:(when race)",     // a when holds no when
		"remember 'lap' said:'2023-05-08' said:'2023-05-09'", // a fact is said once
		"remember 'lap' occurred:(when race)",                // a remember is given its times
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			got, _, err := query.Parse[string, float32](q, nil, config.New())
			if !errors.Is(err, query.ErrParsingFailed) {
				t.Errorf("Parse(%q) err = %v, want it to wrap ErrParsingFailed", q, err)
			}
			if got != nil {
				t.Errorf("Parse(%q) = %v, want nil query", q, got)
			}
		})
	}
}

// TestParseRecallBindsVector checks that a vec:$v placeholder binds the
// supplied vector into the resulting *query.Recall.
func TestParseRecallBindsVector(t *testing.T) {
	q := "recall@0 amelia entity:amelia topic:preferences vec:$v"
	params := map[string][]float32{"v": {0.1, 0.2, 0.3}}

	got, _, err := query.Parse[string, float32](q, params, config.New())
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
	}

	r, ok := got.(*query.Recall[string, float32])
	if !ok {
		t.Fatalf("Parse(%q) returned %T, want *query.Recall", q, got)
	}

	if want := []float32{0.1, 0.2, 0.3}; !reflect.DeepEqual(r.Vector.Data, want) {
		t.Errorf("Vector.Data = %v, want %v", r.Vector.Data, want)
	}
}

// TestParseRecallMissingParameter checks that referencing a placeholder with no
// matching parameter is rejected with ErrMissingParameter. A when binds its
// vector as a recall does, as a statement and nested in a bound, so the
// placeholder of a nested when is checked with the query that holds it.
func TestParseRecallMissingParameter(t *testing.T) {
	queries := []string{
		"recall@0 amelia vec:$v",
		"when amelia vec:$v",
		"recall@0 amelia until:(when race vec:$v)",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			got, _, err := query.Parse[string, float32](q, nil, config.New())
			if !errors.Is(err, query.ErrMissingParameter) {
				t.Errorf("Parse(%q) err = %v, want ErrMissingParameter", q, err)
			}
			if got != nil {
				t.Errorf("Parse(%q) = %v, want nil query", q, got)
			}
		})
	}
}

// TestParseRejectsOverLimits checks that Parse enforces the configured ranges.
// A recall outside the top or depth range is rejected with ErrLimitExceeded and
// no query. So is a bound vector longer than the dimension ceiling. top:0 is
// the floor case. Presence-keyed parsing keeps it visible, so it is rejected
// with the range error instead of silently answering with the default. A when
// is held to the same ranges, as a statement and nested in a bound: the search
// it runs costs what a recall's does.
func TestParseRejectsOverLimits(t *testing.T) {
	cfg := config.New()
	cfg.DB.MaxTop = 10
	cfg.DB.MaxDepth = 2
	cfg.DB.MaxVectorDimension = 3

	cases := []struct {
		name   string
		query  string
		params map[string][]float32
	}{
		{"top over ceiling", "recall@0 anna top:99", nil},
		{"top under floor", "recall@0 anna top:0", nil},
		{"depth over ceiling", "recall@0 anna depth:9", nil},
		{"recall vector too long", "recall@0 anna vec:$v", map[string][]float32{"v": {1, 2, 3, 4}}},
		{"remember vector too long", "remember@0 'a fact' vec:$v", map[string][]float32{"v": {1, 2, 3, 4}}},
		{"when top over ceiling", "when anna top:99", nil},
		{"when top under floor", "when anna top:0", nil},
		{"when depth over ceiling", "when anna depth:9", nil},
		{"when vector too long", "when anna vec:$v", map[string][]float32{"v": {1, 2, 3, 4}}},
		{"nested top over ceiling", "recall@0 anna until:(when race top:99)", nil},
		{"nested depth over ceiling", "recall@0 anna since:(when race depth:9)", nil},
		{"nested vector too long", "recall@0 anna until:(when race vec:$v)", map[string][]float32{"v": {1, 2, 3, 4}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := query.Parse[string, float32](tc.query, tc.params, cfg)
			if !errors.Is(err, query.ErrLimitExceeded) {
				t.Errorf("Parse(%q) err = %v, want ErrLimitExceeded", tc.query, err)
			}
			if got != nil {
				t.Errorf("Parse(%q) = %v, want nil query", tc.query, got)
			}
		})
	}

	// A request within every ceiling still parses cleanly.
	if _, _, err := query.Parse[string, float32]("recall@0 anna top:5 depth:1", nil, cfg); err != nil {
		t.Errorf("within-limit recall returned error: %v", err)
	}
	if _, _, err := query.Parse[string, float32]("recall@0 anna until:(when race top:5 depth:1)", nil, cfg); err != nil {
		t.Errorf("within-limit nested when returned error: %v", err)
	}
}

// marshalHit builds a Hit over a fact with a fixed timestamp and returns its
// JSON. The fact needs no hasher: marshalling reads only value and timestamp.
func marshalHit(t *testing.T, contributions []query.HitContribution[float32]) string {
	t.Helper()
	var node graph.Node[string] = graph.Fact[string]{NodeAttributes: graph.NodeAttributes{
		Value:     "the parrot is turquoise",
		Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}}
	h := query.Hit[string, float32]{Node: node, Score: 0.5, Contributions: contributions}
	out, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("Marshal(Hit) = %v, want nil", err)
	}
	return string(out)
}

// TestHitMarshalOmitsContributionsByDefault pins the ordinary wire shape. A hit
// without contributions serialises as its value, timestamp and score alone,
// with no "contributions" key.
func TestHitMarshalOmitsContributionsByDefault(t *testing.T) {
	got := marshalHit(t, nil)
	want := `{"value":"the parrot is turquoise","timestamp":"2026-01-02T03:04:05Z","score":0.5}`
	if got != want {
		t.Errorf("Marshal(Hit) = %s, want %s", got, want)
	}
}

// TestHitMarshalSerializesContributions pins the explain wire shape. Each
// contribution carries its source by name, its raw score, rank and seed count.
// A graph entry also carries the funding anchor's value under via, and its
// degree, and named when the query named that anchor. A text entry carries
// held, the share of the query its fact's own words hold. via, degree, named
// and held are omitted where they mean nothing.
func TestHitMarshalSerializesContributions(t *testing.T) {
	got := marshalHit(t, []query.HitContribution[float32]{
		{Source: "text", Score: 1, Rank: 0, Count: 1, Held: 0.5},
		{Source: "text", Score: 0.25, Rank: 1, Count: 1},
		{Source: "vector", Score: 0.5, Rank: 1, Count: 1},
		{Source: "graph", Score: 2, Via: "weather", Degree: 3, Count: 2},
		{Source: "graph", Score: 4, Via: "archive", Degree: 8, Count: 1, Named: true},
	})
	want := `{"value":"the parrot is turquoise","timestamp":"2026-01-02T03:04:05Z","score":0.5,` +
		`"contributions":[` +
		`{"source":"text","score":1,"rank":0,"count":1,"held":0.5},` +
		`{"source":"text","score":0.25,"rank":1,"count":1},` +
		`{"source":"vector","score":0.5,"rank":1,"count":1},` +
		`{"source":"graph","score":2,"rank":0,"via":"weather","degree":3,"count":2},` +
		`{"source":"graph","score":4,"rank":0,"via":"archive","degree":8,"count":1,"named":true}]}`
	if got != want {
		t.Errorf("Marshal(Hit) = %s, want %s", got, want)
	}
}

// TestHitMarshalCarriesTheTimesAFactWasGiven pins the wire form of a dated
// fact. said and occurred appear only when the fact was given them, each
// occurrence as its start and its end, and the end is left out of one that is
// not over. They are the fact's own fields, not what a clock falls back to, so
// a client can tell a fact said on a date from one that reads its timestamp.
// decay is the recency factor of an explain response, and absent otherwise.
func TestHitMarshalCarriesTheTimesAFactWasGiven(t *testing.T) {
	tests := map[string]struct {
		said     time.Time
		occurred []containers.Interval
		decay    float32
		want     string
	}{
		"said": {
			said: may(8),
			want: `{"value":"lap","timestamp":"2026-01-02T03:04:05Z","said":"2023-05-08T00:00:00Z","score":0.5}`,
		},
		"occurred on a day": {
			occurred: []containers.Interval{{Start: may(7), End: may(8)}},
			want: `{"value":"lap","timestamp":"2026-01-02T03:04:05Z",` +
				`"occurred":[{"start":"2023-05-07T00:00:00Z","end":"2023-05-08T00:00:00Z"}],"score":0.5}`,
		},
		"occurred at an instant": {
			occurred: []containers.Interval{{Start: may(7), End: may(7)}},
			want: `{"value":"lap","timestamp":"2026-01-02T03:04:05Z",` +
				`"occurred":[{"start":"2023-05-07T00:00:00Z","end":"2023-05-07T00:00:00Z"}],"score":0.5}`,
		},
		"occurred and not over": {
			occurred: []containers.Interval{{Start: may(20)}},
			want: `{"value":"lap","timestamp":"2026-01-02T03:04:05Z",` +
				`"occurred":[{"start":"2023-05-20T00:00:00Z"}],"score":0.5}`,
		},
		"said and occurred twice": {
			said:     may(8),
			occurred: []containers.Interval{{Start: may(1), End: may(2)}, {Start: may(7), End: may(8)}},
			want: `{"value":"lap","timestamp":"2026-01-02T03:04:05Z","said":"2023-05-08T00:00:00Z",` +
				`"occurred":[{"start":"2023-05-01T00:00:00Z","end":"2023-05-02T00:00:00Z"},` +
				`{"start":"2023-05-07T00:00:00Z","end":"2023-05-08T00:00:00Z"}],"score":0.5}`,
		},
		"explained": {
			said:  may(8),
			decay: 0.25,
			want: `{"value":"lap","timestamp":"2026-01-02T03:04:05Z","said":"2023-05-08T00:00:00Z",` +
				`"score":0.5,"decay":0.25}`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var node graph.Node[string] = graph.Fact[string]{
				NodeAttributes: graph.NodeAttributes{
					Value:     "lap",
					Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				},
				Said:     tc.said,
				Occurred: tc.occurred,
			}

			got, err := json.Marshal(query.Hit[string, float32]{Node: node, Score: 0.5, Decay: tc.decay})
			if err != nil {
				t.Fatalf("Marshal(Hit) = %v, want nil", err)
			}

			if diff := cmp.Diff(tc.want, string(got)); diff != "" {
				t.Errorf("Marshal(Hit) mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestQueryResultMarshal pins the response envelope. A recall's result is its
// count and its hits. A when's also carries the time it answers with: start
// and end, the end left out of an answer that is not over, and both left out
// of a when that matched no fact. An explain response adds the background
// rate, and under when the result of each query nested in a bound, keyed by
// the bound that held it.
func TestQueryResultMarshal(t *testing.T) {
	const hit = `{"value":"lap","timestamp":"2026-01-02T03:04:05Z","score":0.5}`
	hits := func() []query.Hit[string, float32] {
		var node graph.Node[string] = graph.Fact[string]{NodeAttributes: graph.NodeAttributes{
			Value:     "lap",
			Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}}
		return []query.Hit[string, float32]{{Node: node, Score: 0.5}}
	}
	tests := map[string]struct {
		result query.QueryResult[string, float32]
		want   string
	}{
		"a recall": {
			result: query.QueryResult[string, float32]{Count: 1, Hits: hits()},
			want:   `{"count":1,"hits":[` + hit + `]}`,
		},
		"no hits": {
			result: query.QueryResult[string, float32]{Hits: []query.Hit[string, float32]{}},
			want:   `{"count":0,"hits":[]}`,
		},
		"a when": {
			result: query.QueryResult[string, float32]{Count: 1, Hits: hits(), Start: may(7), End: may(8)},
			want:   `{"count":1,"hits":[` + hit + `],"start":"2023-05-07T00:00:00Z","end":"2023-05-08T00:00:00Z"}`,
		},
		"a when at an instant": {
			result: query.QueryResult[string, float32]{Count: 1, Hits: hits(), Start: may(8), End: may(8)},
			want:   `{"count":1,"hits":[` + hit + `],"start":"2023-05-08T00:00:00Z","end":"2023-05-08T00:00:00Z"}`,
		},
		"a when that is not over": {
			result: query.QueryResult[string, float32]{Count: 1, Hits: hits(), Start: may(20)},
			want:   `{"count":1,"hits":[` + hit + `],"start":"2023-05-20T00:00:00Z"}`,
		},
		"explained": {
			result: query.QueryResult[string, float32]{Count: 1, Hits: hits(), Background: 0.125},
			want:   `{"count":1,"hits":[` + hit + `],"background":0.125}`,
		},
		"explained with a when in each bound": {
			result: query.QueryResult[string, float32]{
				Count:      1,
				Hits:       hits(),
				Background: 0.125,
				When: &query.NestedResults[string, float32]{
					Since: &query.QueryResult[string, float32]{Count: 1, Hits: hits(), Start: may(7), End: may(8)},
					Until: &query.QueryResult[string, float32]{Hits: []query.Hit[string, float32]{}},
				},
			},
			want: `{"count":1,"hits":[` + hit + `],"background":0.125,"when":{` +
				`"since":{"count":1,"hits":[` + hit + `],"start":"2023-05-07T00:00:00Z","end":"2023-05-08T00:00:00Z"},` +
				`"until":{"count":0,"hits":[]}}}`,
		},
		"explained with a when in until": {
			result: query.QueryResult[string, float32]{
				Count: 1,
				Hits:  hits(),
				When: &query.NestedResults[string, float32]{
					Until: &query.QueryResult[string, float32]{Count: 1, Hits: hits(), Start: may(20)},
				},
			},
			want: `{"count":1,"hits":[` + hit + `],"when":{` +
				`"until":{"count":1,"hits":[` + hit + `],"start":"2023-05-20T00:00:00Z"}}}`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := json.Marshal(tc.result)
			if err != nil {
				t.Fatalf("Marshal(QueryResult) = %v, want nil", err)
			}

			if diff := cmp.Diff(tc.want, string(got)); diff != "" {
				t.Errorf("Marshal(QueryResult) mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// storedTimes is what a fact holds of the times a remember gave it.
type storedTimes struct {
	Said     time.Time
	Occurred []containers.Interval
}

// storedFact returns the one fact g holds under the term lap, read back
// through a recall as a client reads it.
func storedFact(t *testing.T, g graph.Graph[uint64, float32], cfg *config.ConfigSet) graph.Fact[uint64] {
	t.Helper()
	result := commit(t, g, cfg, "recall lap", false).Result
	if len(result.Hits) != 1 {
		t.Fatalf("recall lap returned %d hits, want the one fact remembered", len(result.Hits))
	}
	fact, ok := result.Hits[0].Node.(graph.Fact[uint64])
	if !ok {
		t.Fatalf("recall lap returned a %T, want a fact", result.Hits[0].Node)
	}
	return fact
}

// TestRememberStoresTheTimesItWasGiven pins what a remember writes on the
// fact. said: is stored as the instant its literal starts at. Each occurred:
// is stored as the whole span its literal names: a day, a month, a year, an
// instant, a range from the start of its first end to the end of its last, or
// a span with no end. A remember with neither clause stores neither, and the
// fact reads its timestamp on every clock.
func TestRememberStoresTheTimesItWasGiven(t *testing.T) {
	year := func(y int) time.Time { return time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC) }
	june := time.Date(2023, time.June, 1, 0, 0, 0, 0, time.UTC)
	instant := time.Date(2023, time.May, 7, 9, 30, 0, 0, time.UTC)
	tests := map[string]storedTimes{
		"remember 'lap'":                             {},
		"remember 'lap' said:'2023-05-08'":           {Said: may(8)},
		"remember 'lap' said:'2023-05'":              {Said: may(1)},
		"remember 'lap' said:'2023-05-07T09:30:00Z'": {Said: instant},
		"remember 'lap' occurred:'2023-05-07'": {
			Occurred: []containers.Interval{{Start: may(7), End: may(8)}},
		},
		"remember 'lap' occurred:'2023-05'": {
			Occurred: []containers.Interval{{Start: may(1), End: june}},
		},
		"remember 'lap' occurred:'2023'": {
			Occurred: []containers.Interval{{Start: year(2023), End: year(2024)}},
		},
		"remember 'lap' occurred:'2023-05-07T09:30:00Z'": {
			Occurred: []containers.Interval{{Start: instant, End: instant}},
		},
		"remember 'lap' occurred:'2023-05-10'..'2023-05-16'": {
			Occurred: []containers.Interval{{Start: may(10), End: may(17)}},
		},
		"remember 'lap' occurred:'2023-05-20'..": {
			Occurred: []containers.Interval{{Start: may(20)}},
		},
		"remember 'lap' said:'2023-05-08' occurred:'2023-05-28' occurred:'2023-05-01'": {
			Said: may(8),
			Occurred: []containers.Interval{
				{Start: may(28), End: may(29)},
				{Start: may(1), End: may(2)},
			},
		},
	}
	for statement, want := range tests {
		t.Run(statement, func(t *testing.T) {
			cfg := config.New()
			g := graph.NewGraph[uint64, float32](cfg)
			commit(t, g, cfg, statement, false)

			fact := storedFact(t, g, cfg)

			got := storedTimes{Said: fact.Said, Occurred: fact.Occurred}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("%s stored times mismatch (-want +got):\n%s", statement, diff)
			}
		})
	}
}

// TestRememberCountsADurationBackFromTheWrite pins the instant a duration is
// measured from. One reading of the clock stamps the fact and resolves its
// times, so a fact said 2d ago is said exactly 48 hours before its timestamp.
// Two readings would leave the gap between them in every stored time.
func TestRememberCountsADurationBackFromTheWrite(t *testing.T) {
	cfg := config.New()
	g := graph.NewGraph[uint64, float32](cfg)
	commit(t, g, cfg, "remember 'lap' said:2d occurred:3d", false)

	fact := storedFact(t, g, cfg)

	happened := fact.Timestamp.Add(-72 * time.Hour)
	want := storedTimes{
		Said:     fact.Timestamp.Add(-48 * time.Hour),
		Occurred: []containers.Interval{{Start: happened, End: happened}},
	}
	got := storedTimes{Said: fact.Said, Occurred: fact.Occurred}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("stored times mismatch (-want +got):\n%s", diff)
	}
}

// TestRememberAgainReplacesTheTimes pins what re-asserting a fact does to its
// times. A fact is keyed by its value, so remembering it again replaces the
// stored fact, and the times it holds are the ones the last remember gave. A
// clause the last remember leaves out is not kept from an earlier one: the
// fact then reads its fresh timestamp on that clock.
func TestRememberAgainReplacesTheTimes(t *testing.T) {
	cfg := config.New()
	g := graph.NewGraph[uint64, float32](cfg)
	steps := []struct {
		statement string
		want      storedTimes
	}{
		{
			statement: "remember 'lap' said:'2023-05-08' occurred:'2023-05-07'",
			want:      storedTimes{Said: may(8), Occurred: []containers.Interval{{Start: may(7), End: may(8)}}},
		},
		{
			statement: "remember 'lap' said:'2023-05-25' occurred:'2023-05-10'..'2023-05-16'",
			want:      storedTimes{Said: may(25), Occurred: []containers.Interval{{Start: may(10), End: may(17)}}},
		},
		{
			statement: "remember 'lap' said:'2023-05-26'",
			want:      storedTimes{Said: may(26)},
		},
		{
			statement: "remember 'lap'",
			want:      storedTimes{},
		},
	}
	for _, step := range steps {
		commit(t, g, cfg, step.statement, false)

		fact := storedFact(t, g, cfg)

		got := storedTimes{Said: fact.Said, Occurred: fact.Occurred}
		if diff := cmp.Diff(step.want, got); diff != "" {
			t.Errorf("after %s, stored times mismatch (-want +got):\n%s", step.statement, diff)
		}
	}
}
