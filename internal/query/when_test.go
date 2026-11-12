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
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/index"
	"github.com/FraiseHQ/fraise/internal/query"
)

// fakeHasher returns its input as the key, wrapped in H(). So a test reads the
// material a query hashed out of the key it got back.
type fakeHasher struct{}

func (fakeHasher) Hash(s string) string { return "H(" + s + ")" }
func (fakeHasher) Seed() uint64         { return 0 }

// newWhen builds the when a query "when[:clock] <keywords> top:<top>" parses
// to, bound to graph id.
func newWhen(clock containers.Clock, id uint8, top int, keywords ...string) *query.When[string, float32] {
	w := &query.When[string, float32]{
		Clock: clock,
		Recall: query.Recall[string, float32]{
			Keywords:   keywords,
			Parameters: query.QueryParameters[string, float32]{Top: top},
		},
	}
	w.SetGraphID(id)
	return w
}

// TestWhenHash pins a when's hash material: the clock it reads, then the key of
// the recall it runs. The recall's key covers the graph, the terms and every
// parameter, so the when adds only what it holds beyond them.
func TestWhenHash(t *testing.T) {
	w := newWhen(containers.Said, 3, 1, "race")

	const want = "H(c=1|r=H(g=3|kw=4:race|en=|to=|d=0|t=1|s=|u=|vec=H()))"
	if diff := cmp.Diff(want, w.Hash(fakeHasher{})); diff != "" {
		t.Errorf("Hash() mismatch (-want +got):\n%s", diff)
	}
}

// TestWhenHashDistinguishes pins what must not share a cache key. Whens that
// read another clock, or run another recall, answer with another time. A when
// and the recall it runs differ in what they return: the recall's key is the
// when's inner key, never the when's own. A shared key would have the engine
// answer one from the plan of the other.
func TestWhenHashDistinguishes(t *testing.T) {
	variants := map[string]query.Query[string, float32]{
		"happened":      newWhen(containers.Happened, 0, 1, "race"),
		"said":          newWhen(containers.Said, 0, 1, "race"),
		"written":       newWhen(containers.Written, 0, 1, "race"),
		"another term":  newWhen(containers.Happened, 0, 1, "wedding"),
		"another top":   newWhen(containers.Happened, 0, 3, "race"),
		"another graph": newWhen(containers.Happened, 2, 1, "race"),
		"its recall":    &newWhen(containers.Happened, 0, 1, "race").Recall,
	}

	seen := make(map[string]string)
	for name, q := range variants {
		key := q.Hash(fakeHasher{})
		if other, clash := seen[key]; clash {
			t.Errorf("hash collision: %q and %q both produced %q", name, other, key)
		}
		seen[key] = name
	}
}

// TestWhenEqual pins the check the engine confirms a plan-cache hit with. A
// when is equal to another that reads the same clock through an equal recall,
// and to nothing else. The recall it runs is not equal to it, from either
// side, though the two search alike: one answers with hits and the other with
// a time.
func TestWhenEqual(t *testing.T) {
	base := func() *query.When[string, float32] { return newWhen(containers.Said, 2, 1, "race") }
	tests := map[string]struct {
		other query.Query[string, float32]
		want  bool
	}{
		"same":          {base(), true},
		"another clock": {newWhen(containers.Happened, 2, 1, "race"), false},
		"another term":  {newWhen(containers.Said, 2, 1, "wedding"), false},
		"another top":   {newWhen(containers.Said, 2, 3, "race"), false},
		"another graph": {newWhen(containers.Said, 4, 1, "race"), false},
		"its recall":    {&base().Recall, false},
		"a remember":    {&query.Remember[string, float32]{Value: "race"}, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := base().Equal(tc.other); got != tc.want {
				t.Errorf("Equal() = %v, want %v", got, tc.want)
			}
			if got := tc.other.Equal(base()); got != tc.want {
				t.Errorf("Equal() from the other side = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestWhenRunsWhereItsRecallDoes pins that a when keeps no graph of its own.
// It reads and sets the graph of the recall it runs, so the two never disagree
// on where the search happens. It is a read, which the scheduler runs under
// the shared lock.
func TestWhenRunsWhereItsRecallDoes(t *testing.T) {
	w := newWhen(containers.Happened, 0, 1, "race")

	w.SetGraphID(4)

	if got := w.GetGraphID(); got != 4 {
		t.Errorf("GetGraphID() = %d after SetGraphID(4), want 4", got)
	}
	if got := w.Recall.GetGraphID(); got != 4 {
		t.Errorf("Recall.GetGraphID() = %d after SetGraphID(4), want 4", got)
	}
	if w.IsWrite() {
		t.Error("IsWrite() = true, want false: a when reads")
	}
}

// TestWhenPlan pins that a when plans into a stream that holds the when, not
// the recall it runs. Commit tells the two apart by the stream's query. A
// stream that held the recall would answer with hits and no time.
func TestWhenPlan(t *testing.T) {
	w := newWhen(containers.Said, 0, 1, "race")

	stream, err := w.Plan(nil)
	if err != nil {
		t.Fatalf("Plan() err = %v, want nil", err)
	}

	if stream.Query != query.Query[string, float32](w) {
		t.Errorf("Plan() stream.Query = %v, want the when", stream.Query)
	}
	select {
	case <-stream.Done():
		t.Error("Plan() stream is already done; it must stay open until committed")
	default:
	}
}

// may returns midnight UTC on a day of May 2023, the month the timeline's
// facts are dated in.
func may(day int) time.Time { return time.Date(2023, time.May, day, 0, 0, 0, 0, time.UTC) }

// timeline builds the graph the commit tests read, written through remember
// statements so the times reach the facts the way a client's do. Six facts
// share the term lap and differ in the times they were given:
//
//	undated  no time
//	told     said 8 May
//	race     said 8 May, happened on 7 May
//	twice    happened on 1 May and on 28 May
//	move     happened from 20 May, and is not over
//	trip     said 25 May, happened 10 to 16 May
//
// told, race and trip share the term spring as well. trip is written last, so
// it is the newest of the three on every clock and ranks first among them.
// The window is off, so a term returns the facts that hold it and no
// neighbour of theirs.
func timeline(t *testing.T) (graph.Graph[uint64, float32], *config.ConfigSet) {
	t.Helper()
	cfg := config.New()
	cfg.DB.WindowGamma = -1
	g := graph.NewGraph[uint64, float32](cfg)
	for _, statement := range []string{
		"remember 'lap undated'",
		"remember 'lap told spring' said:'2023-05-08'",
		"remember 'lap race spring' said:'2023-05-08' occurred:'2023-05-07'",
		"remember 'lap twice' occurred:'2023-05-01' occurred:'2023-05-28'",
		"remember 'lap move' occurred:'2023-05-20'..",
		"remember 'lap trip spring' said:'2023-05-25' occurred:'2023-05-10'..'2023-05-16'",
	} {
		commit(t, g, cfg, statement, false)
	}
	return g, cfg
}

// commit runs q the way a request does: parsed, planned, then committed
// against g. It returns the committed stream.
func commit(t *testing.T, g graph.Graph[uint64, float32], cfg *config.ConfigSet, q string, explain bool) *query.Stream[uint64, float32] {
	t.Helper()
	parsed, _, err := query.Parse[uint64, float32](q, nil, cfg)
	if err != nil {
		t.Fatalf("Parse(%q) = %v, want nil", q, err)
	}
	stream, err := parsed.Plan(cfg)
	if err != nil {
		t.Fatalf("Plan(%q) = %v, want nil", q, err)
	}
	stream.Explain = explain
	if err := stream.Commit(g); err != nil {
		t.Fatalf("Commit(%q) = %v, want nil", q, err)
	}
	return stream
}

// hitValues returns the values of a result's hits in sorted order, for a test
// that pins which facts came back and not how they rank.
func hitValues(r *query.QueryResult[uint64, float32]) []string {
	values := make([]string, len(r.Hits))
	for i, hit := range r.Hits {
		values[i] = hit.Node.GetValue()
	}
	sort.Strings(values)
	return values
}

// TestWhenAnswersWithTheTimeItsHitsHold pins what a when statement returns:
// the hits of the recall it runs, and the time they hold on the clock it
// names. A bare when reads when the fact happened, which falls back to when it
// was said. A fact holding several occurrences answers with their hull, and
// one that is not over answers without an end. Without a top clause the when
// keeps its best match alone, so the answer is that fact's time. With top:k
// it is the hull over the k it keeps.
func TestWhenAnswersWithTheTimeItsHitsHold(t *testing.T) {
	g, cfg := timeline(t)
	type answer struct {
		Time containers.Interval
		Hits []string
	}
	tests := map[string]answer{
		"when race": {
			Time: containers.Interval{Start: may(7), End: may(8)},
			Hits: []string{"lap race spring"},
		},
		"when:happened race": {
			Time: containers.Interval{Start: may(7), End: may(8)},
			Hits: []string{"lap race spring"},
		},
		"when:said race": {
			Time: containers.Interval{Start: may(8), End: may(8)},
			Hits: []string{"lap race spring"},
		},
		"when told": {
			Time: containers.Interval{Start: may(8), End: may(8)},
			Hits: []string{"lap told spring"},
		},
		"when trip": {
			Time: containers.Interval{Start: may(10), End: may(17)},
			Hits: []string{"lap trip spring"},
		},
		"when twice": {
			Time: containers.Interval{Start: may(1), End: may(29)},
			Hits: []string{"lap twice"},
		},
		"when move": {
			Time: containers.Interval{Start: may(20)},
			Hits: []string{"lap move"},
		},
		"when spring": {
			Time: containers.Interval{Start: may(10), End: may(17)},
			Hits: []string{"lap trip spring"},
		},
		"when spring top:10": {
			Time: containers.Interval{Start: may(7), End: may(17)},
			Hits: []string{"lap race spring", "lap told spring", "lap trip spring"},
		},
		"when:said spring top:10": {
			Time: containers.Interval{Start: may(8), End: may(25)},
			Hits: []string{"lap race spring", "lap told spring", "lap trip spring"},
		},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			got := answer{
				Time: containers.Interval{Start: stream.Result.Start, End: stream.Result.End},
				Hits: hitValues(stream.Result),
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("%s mismatch (-want +got):\n%s", q, diff)
			}
			if stream.Result.Count != len(want.Hits) {
				t.Errorf("%s: Count = %d, want %d", q, stream.Result.Count, len(want.Hits))
			}
			if len(stream.Warnings) != 0 {
				t.Errorf("%s: Warnings = %q, want none for a when that answered", q, stream.Warnings)
			}
		})
	}
}

// TestWhenFallsBackToWhenTheFactWasWritten pins the end of the fallback. A
// fact given no time reads its timestamp on every clock, and one given no said
// time reads it on the said clock. The written clock reads it whatever else
// the fact holds. So a when always has a time for a fact it matched.
func TestWhenFallsBackToWhenTheFactWasWritten(t *testing.T) {
	g, cfg := timeline(t)
	tests := map[string]string{
		"when undated":      "lap undated",
		"when:said undated": "lap undated",
		"when:said twice":   "lap twice",
		"when:written race": "lap race spring",
		"when:written trip": "lap trip spring",
	}
	for q, value := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			if diff := cmp.Diff([]string{value}, hitValues(stream.Result)); diff != "" {
				t.Fatalf("%s hits mismatch (-want +got):\n%s", q, diff)
			}
			written := stream.Result.Hits[0].Node.GetTimestamp()
			want := containers.Interval{Start: written, End: written}
			got := containers.Interval{Start: stream.Result.Start, End: stream.Result.End}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("%s mismatch (-want +got):\n%s", q, diff)
			}
		})
	}
}

// TestWhenWithoutAMatchWarns pins the empty when. It matched no fact, so it
// has no time: the result carries no hit, no start and no end, and the stream
// warns, so a caller reading an answer without a time learns why. In a graph
// that holds nothing the stream also reports the graph empty, as a recall
// does.
func TestWhenWithoutAMatchWarns(t *testing.T) {
	populated, cfg := timeline(t)
	tests := map[string]struct {
		graph     graph.Graph[uint64, float32]
		wantEmpty bool
	}{
		"a populated graph": {graph: populated},
		"an empty graph":    {graph: graph.NewGraph[uint64, float32](cfg), wantEmpty: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stream := commit(t, tc.graph, cfg, "when zebras", false)

			if stream.Result.Count != 0 || len(stream.Result.Hits) != 0 {
				t.Errorf("Result = %d hits, count %d, want none", len(stream.Result.Hits), stream.Result.Count)
			}
			if !stream.Result.Start.IsZero() || !stream.Result.End.IsZero() {
				t.Errorf("Result time = %v to %v, want neither start nor end", stream.Result.Start, stream.Result.End)
			}
			want := []string{"the when matched no fact, so it has no time to answer with"}
			if diff := cmp.Diff(want, stream.Warnings); diff != "" {
				t.Errorf("Warnings mismatch (-want +got):\n%s", diff)
			}
			if stream.IsGraphEmpty != tc.wantEmpty {
				t.Errorf("IsGraphEmpty = %v, want %v", stream.IsGraphEmpty, tc.wantEmpty)
			}
		})
	}
}

// TestRecallReadsABoundFromAWhen pins the window a nested when draws. until
// keeps what came before the answer, so it reads where the answer starts.
// since keeps what came after it, so it reads where the answer ends, and the
// start of an answer that is not over. Each bound is compared on the clock its
// own when read, so two bounds on two clocks each keep what their clock says.
// The fact the when matched is itself outside a window on the clock it
// happened on, and inside one that starts at the instant it was said.
func TestRecallReadsABoundFromAWhen(t *testing.T) {
	g, cfg := timeline(t)
	tests := map[string][]string{
		"recall lap until:(when race)":                         {"lap twice"},
		"recall lap since:(when race)":                         {"lap move", "lap told spring", "lap trip spring", "lap twice", "lap undated"},
		"recall lap until:(when trip)":                         {"lap race spring", "lap told spring", "lap twice"},
		"recall lap until:(when:said trip)":                    {"lap race spring", "lap told spring"},
		"recall lap since:(when:said trip)":                    {"lap move", "lap trip spring", "lap twice", "lap undated"},
		"recall lap since:(when:said race)":                    {"lap move", "lap race spring", "lap told spring", "lap trip spring", "lap twice", "lap undated"},
		"recall lap since:(when move)":                         {"lap move", "lap twice", "lap undated"},
		"recall lap until:(when move)":                         {"lap race spring", "lap told spring", "lap trip spring", "lap twice"},
		"recall lap since:(when race) until:(when:said trip)":  {"lap told spring"},
		"recall lap since:'2023-05-09' until:(when:said trip)": {},
		"recall lap since:(when spring top:10)":                {"lap move", "lap twice", "lap undated"},
		"recall lap until:(when spring top:10)":                {"lap twice"},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			if diff := cmp.Diff(want, hitValues(stream.Result)); diff != "" {
				t.Errorf("%s mismatch (-want +got):\n%s", q, diff)
			}
			if len(stream.Warnings) != 0 {
				t.Errorf("%s: Warnings = %q, want none when every bound was answered", q, stream.Warnings)
			}
		})
	}
}

// TestRecallWithAnUnansweredBoundReturnsNoHits pins what a recall does when a
// when in one of its bounds matched no fact. It has no time to compare with,
// and searching without the bound would return the facts the query asked to
// leave out. So the recall returns no hits, and warns once per bound left
// unanswered, naming the clause. A bound that was answered does not make up
// for one that was not. The graph is populated, so the stream does not report
// it empty.
func TestRecallWithAnUnansweredBoundReturnsNoHits(t *testing.T) {
	const unanswered = "the when matched no fact, so it has no time to answer with; the recall has no bound to read and returns no hits"
	g, cfg := timeline(t)
	tests := map[string][]string{
		"recall lap until:(when zebras)":                       {"until: " + unanswered},
		"recall lap since:(when zebras)":                       {"since: " + unanswered},
		"recall lap since:(when zebras) until:(when unicorns)": {"since: " + unanswered, "until: " + unanswered},
		"recall lap since:(when race) until:(when zebras)":     {"until: " + unanswered},
		"recall lap since:(when zebras) until:'2023-05-09'":    {"since: " + unanswered},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			if stream.Result.Count != 0 || len(stream.Result.Hits) != 0 {
				t.Errorf("%s returned %v, want no hits", q, hitValues(stream.Result))
			}
			if diff := cmp.Diff(want, stream.Warnings); diff != "" {
				t.Errorf("%s warnings mismatch (-want +got):\n%s", q, diff)
			}
			if stream.IsGraphEmpty {
				t.Errorf("%s: IsGraphEmpty = true, want false in a graph that holds facts", q)
			}
		})
	}
}

// TestRecallExplainsTheWhenItsBoundRead pins the when resolution in explain
// mode. The recall's result carries the result of each nested query under the
// bound that holds it: the fact the when matched and the time it gave. Each
// hit carries the recency factor its relevance was scaled by. Without explain
// the result carries neither, so an ordinary response does not grow.
func TestRecallExplainsTheWhenItsBoundRead(t *testing.T) {
	type resolution struct {
		Time containers.Interval
		Hits []string
	}
	g, cfg := timeline(t)
	for _, explain := range []bool{true, false} {
		t.Run(fmt.Sprintf("explain=%v", explain), func(t *testing.T) {
			stream := commit(t, g, cfg, "recall lap until:(when trip)", explain)

			if !explain {
				if stream.Result.When != nil {
					t.Errorf("Result.When = %+v without explain, want nil", stream.Result.When)
				}
				for i, hit := range stream.Result.Hits {
					if hit.Decay != 0 {
						t.Errorf("Hits[%d].Decay = %v without explain, want 0 (omitted on the wire)", i, hit.Decay)
					}
				}
				return
			}

			if stream.Result.When == nil || stream.Result.When.Until == nil {
				t.Fatalf("Result.When = %+v, want the result of the when in until", stream.Result.When)
			}
			if stream.Result.When.Since != nil {
				t.Errorf("Result.When.Since = %+v, want nil: since holds no query", stream.Result.When.Since)
			}
			nested := stream.Result.When.Until
			want := resolution{
				Time: containers.Interval{Start: may(10), End: may(17)},
				Hits: []string{"lap trip spring"},
			}
			got := resolution{
				Time: containers.Interval{Start: nested.Start, End: nested.End},
				Hits: hitValues(nested),
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("Result.When.Until mismatch (-want +got):\n%s", diff)
			}
			if len(nested.Hits) == 1 && nested.Hits[0].Decay <= 0 {
				t.Errorf("Result.When.Until.Hits[0].Decay = %v, want the when explained as its recall is", nested.Hits[0].Decay)
			}
			if len(stream.Result.Hits) == 0 {
				t.Fatal("the recall returned no hits, want the facts before the trip")
			}
			for i, hit := range stream.Result.Hits {
				if hit.Decay <= 0 || hit.Decay > 1 {
					t.Errorf("Hits[%d].Decay = %v, want a recency factor in (0, 1]", i, hit.Decay)
				}
			}
		})
	}
}

// TestCommitFailsOnANestedSearchError pins that a bound that cannot be read
// fails the recall. The when in until carries a vector of another dimension
// than the index holds, which the graph refuses. The recall fails with that
// cause and builds no result. Answering from the outer search alone would
// drop the bound without saying so.
func TestCommitFailsOnANestedSearchError(t *testing.T) {
	cfg := config.New()
	g := graph.NewGraph[uint64, float32](cfg)
	for q, params := range map[string]map[string][]float32{
		"remember 'lap race' vec:$v": {"v": {1, 2, 3}},
	} {
		parsed, _, err := query.Parse[uint64, float32](q, params, cfg)
		if err != nil {
			t.Fatalf("Parse(%q) = %v, want nil", q, err)
		}
		if err := query.NewStream[uint64, float32](parsed).Commit(g); err != nil {
			t.Fatalf("Commit(%q) = %v, want nil", q, err)
		}
	}

	const q = "recall lap until:(when race vec:$v)"
	parsed, _, err := query.Parse[uint64, float32](q, map[string][]float32{"v": {1, 2}}, cfg)
	if err != nil {
		t.Fatalf("Parse(%q) = %v, want nil", q, err)
	}
	stream, err := parsed.Plan(cfg)
	if err != nil {
		t.Fatalf("Plan(%q) = %v, want nil", q, err)
	}

	err = stream.Commit(g)

	if !errors.Is(err, index.ErrInvalidDimension) {
		t.Fatalf("Commit(%q) err = %v, want it to wrap ErrInvalidDimension", q, err)
	}
	if stream.Result != nil {
		t.Errorf("Commit built a result %+v from a bound it could not read, want nil", stream.Result)
	}
}

// TestCommitRefusesABoundThatIsNotAWhen pins what a stream does with a nested
// stream that answers with no time. A time bound reads a when. A recall hung
// in its place has hits and no time, so the commit fails rather than search
// with a bound it made up.
func TestCommitRefusesABoundThatIsNotAWhen(t *testing.T) {
	g, _ := timeline(t)
	recall := &query.Recall[uint64, float32]{
		Keywords:   []string{"lap"},
		Parameters: query.QueryParameters[uint64, float32]{Top: 10},
	}
	stream := query.NewStream[uint64, float32](recall)
	stream.Until = query.NewStream[uint64, float32](&query.Recall[uint64, float32]{
		Keywords:   []string{"race"},
		Parameters: query.QueryParameters[uint64, float32]{Top: 1},
	})

	err := stream.Commit(g)

	if err == nil {
		t.Fatal("Commit() = nil, want an error for a recall hung in a time bound")
	}
	if stream.Result != nil {
		t.Errorf("Commit built a result %+v from a bound it refused, want nil", stream.Result)
	}
}
