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
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/query"
)

// newDescribe builds the describe a query "describe recall@<id> <scope>"
// parses to, where scope holds keywords and at most a top clause, and top is
// that clause or the default. Its recall carries no top, as the parser leaves
// it, so it keeps every match.
func newDescribe(id uint8, top int, scope string, keywords ...string) *query.Describe[string, float32] {
	d := &query.Describe[string, float32]{
		Scope:  scope,
		Top:    top,
		Recall: query.Recall[string, float32]{Keywords: keywords},
	}
	d.SetGraphID(id)
	return d
}

// newRecall builds the recall a query "recall@<id> <keywords>" parses to under
// a default top of top.
func newRecall(id uint8, top int, keywords ...string) *query.Recall[string, float32] {
	r := &query.Recall[string, float32]{
		Keywords:   keywords,
		Parameters: query.QueryParameters[string, float32]{Top: top},
	}
	r.SetGraphID(id)
	return r
}

// TestDescribeHash pins a describe's hash material: the prefix, the cap on the
// anchors it lists, the scope it answers with, then the key of the recall it
// runs. The recall's key covers the graph, the terms and every parameter, and
// its top is 0, so the cap is the describe's own segment and not the recall's.
func TestDescribeHash(t *testing.T) {
	d := newDescribe(3, 5, "heron top:5", "heron")

	const want = "H(p=describe|t=5|sc=11:heron top:5|r=H(g=3|kw=5:heron|en=|to=|d=0|t=0|s=|u=|vec=H()))"
	if diff := cmp.Diff(want, d.Hash(fakeHasher{})); diff != "" {
		t.Errorf("Hash() mismatch (-want +got):\n%s", diff)
	}
}

// TestDescribeHashDistinguishes pins what must not share a cache key.
// Describes that list another number of anchors, answer with another scope or
// run another recall answer with another catalogue. The scope differs alone
// between describe recall heron and describe recall heron top:10 under a
// default top of 10. A describe, the recall it runs, a when over that recall
// and the recall its text would be on its own read the same facts and answer
// with a catalogue, hits or a time. A shared key would have the engine answer
// one from the plan of another.
func TestDescribeHashDistinguishes(t *testing.T) {
	base := func() *query.Describe[string, float32] { return newDescribe(0, 10, "heron", "heron") }
	variants := map[string]query.Query[string, float32]{
		"base":                        base(),
		"another top":                 newDescribe(0, 5, "heron", "heron"),
		"another scope":               newDescribe(0, 10, "heron top:10", "heron"),
		"another term":                newDescribe(0, 10, "egret", "egret"),
		"another graph":               newDescribe(2, 10, "heron", "heron"),
		"the whole graph":             newDescribe(0, 10, ""),
		"its recall":                  &base().Recall,
		"a when over its recall":      &query.When[string, float32]{Clock: containers.Happened, Recall: base().Recall},
		"a recall with the same text": newRecall(0, 10, "heron"),
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

// TestDescribeEqual pins the check the engine confirms a plan-cache hit with. A
// describe is equal to another with the same cap and scope over an equal
// recall, and to nothing else. The recall it runs, a when over that recall and
// the recall its text would be on its own are not equal to it, from either
// side, though they search alike: each answers with something else.
func TestDescribeEqual(t *testing.T) {
	base := func() *query.Describe[string, float32] { return newDescribe(2, 10, "heron", "heron") }
	tests := map[string]struct {
		other query.Query[string, float32]
		want  bool
	}{
		"same":                        {base(), true},
		"another top":                 {newDescribe(2, 5, "heron", "heron"), false},
		"another scope":               {newDescribe(2, 10, "heron top:10", "heron"), false},
		"another term":                {newDescribe(2, 10, "egret", "egret"), false},
		"another graph":               {newDescribe(4, 10, "heron", "heron"), false},
		"its recall":                  {&base().Recall, false},
		"a when over its recall":      {&query.When[string, float32]{Clock: containers.Happened, Recall: base().Recall}, false},
		"a recall with the same text": {newRecall(2, 10, "heron"), false},
		"a remember":                  {&query.Remember[string, float32]{Value: "heron"}, false},
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

// describeOverAWhen builds the describe "describe recall heron until:(when
// race)" parses to: its recall holds a when in its until bound.
func describeOverAWhen() (*query.Describe[string, float32], *query.When[string, float32]) {
	nested := &query.When[string, float32]{
		Recall: query.Recall[string, float32]{
			Keywords:   []string{"race"},
			Parameters: query.QueryParameters[string, float32]{Top: 1},
		},
	}
	d := newDescribe(0, 10, "heron until:(when race)", "heron")
	d.Recall.Parameters.Until.Query = nested
	return d, nested
}

// TestDescribeRunsWhereItsRecallDoes pins that a describe keeps no graph of
// its own. It reads and sets the graph of the recall it runs, and through it
// the graph of a when in the recall's bound, so the catalogue, the search and
// the bound never disagree on where they read. It is a read, which the
// scheduler runs under the shared lock.
func TestDescribeRunsWhereItsRecallDoes(t *testing.T) {
	d, nested := describeOverAWhen()

	d.SetGraphID(4)

	got := map[string]uint8{
		"describe": d.GetGraphID(),
		"recall":   d.Recall.GetGraphID(),
		"nested":   nested.GetGraphID(),
	}
	want := map[string]uint8{"describe": 4, "recall": 4, "nested": 4}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("graphs after SetGraphID(4) mismatch (-want +got):\n%s", diff)
	}
	if d.IsWrite() {
		t.Error("IsWrite() = true, want false: a describe reads")
	}
}

// TestDescribePlan pins that a describe plans into its recall's stream with
// the describe as its query. Commit tells a describe from the recall it runs
// by the stream's query: a stream that held the recall would answer with hits
// and no catalogue. The when in the recall's bound hangs on the stream as it
// does on the recall's own, so the window is read before the facts are.
func TestDescribePlan(t *testing.T) {
	d, nested := describeOverAWhen()

	stream, err := d.Plan(nil)
	if err != nil {
		t.Fatalf("Plan() err = %v, want nil", err)
	}

	if stream.Query != query.Query[string, float32](d) {
		t.Errorf("Plan() stream.Query = %v, want the describe", stream.Query)
	}
	if stream.Since != nil {
		t.Errorf("Plan() stream.Since = %v, want nil: since holds no query", stream.Since)
	}
	if stream.Until == nil || stream.Until.Query != query.Query[string, float32](nested) {
		t.Errorf("Plan() stream.Until = %v, want the stream of the when in until", stream.Until)
	}
	select {
	case <-stream.Done():
		t.Error("Plan() stream is already done; it must stay open until committed")
	default:
	}
}

// aviary builds the graph the describe commit tests read, written through
// remember statements so the times and anchors reach the facts the way a
// client's do. Four facts, written in this order:
//
//	nests  birds lake     heron       said 8 May, happened on 7 May
//	fish   birds fishing  heron       said 12 May, happened 10 to 11 May
//	dives  birds fishing  kingfisher  said 20 May
//	leaks  boats          otto        no time
//
// So the set reads differently on each clock: dives happened when it was
// said, and leaks reads its timestamp on all three. The window is off, so a
// term reaches the facts that hold it and no neighbour of theirs.
func aviary(t *testing.T) (graph.Graph[uint64, float32], *config.ConfigSet) {
	t.Helper()
	cfg := config.New()
	cfg.DB.WindowGamma = -1
	g := graph.NewGraph[uint64, float32](cfg)
	for _, statement := range []string{
		"remember 'the heron nests by the lake' topic:birds topic:lake entity:heron said:'2023-05-08' occurred:'2023-05-07'",
		"remember 'the heron caught a fish' topic:birds topic:fishing entity:heron said:'2023-05-12' occurred:'2023-05-10'..'2023-05-11'",
		"remember 'the kingfisher dives' topic:birds topic:fishing entity:kingfisher said:'2023-05-20'",
		"remember 'the boat leaks' topic:boats entity:otto",
	} {
		commit(t, g, cfg, statement, false)
	}
	return g, cfg
}

// written returns the hull of the instants g wrote the facts with values at:
// from the first written to the last. The server stamps a fact when it stores
// it, so a test reads the written clock back rather than spelling it.
func written(t *testing.T, g graph.Graph[uint64, float32], values ...string) containers.Interval {
	t.Helper()
	var hull containers.Interval
	for i, value := range values {
		node := g.Get(graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: value}, Hasher: g.GetHasher()}.Key())
		if node == nil {
			t.Fatalf("the graph holds no fact %q", value)
		}
		at := node.GetTimestamp()
		if i == 0 || at.Before(hull.Start) {
			hull.Start = at
		}
		if i == 0 || at.After(hull.End) {
			hull.End = at
		}
	}
	return hull
}

// span returns the interval from midnight UTC on day start of May 2023 to
// midnight on day end, which the interval leaves out.
func span(start, end int) containers.Interval {
	return containers.Interval{Start: may(start), End: may(end)}
}

// ignoreTimes narrows a catalogue comparison to its counts and its lists, for
// a test that pins which facts were reached and not when they hold.
var ignoreTimes = cmp.Options{
	cmpopts.IgnoreFields(query.Catalogue{}, "Times"),
	cmpopts.IgnoreFields(query.CatalogueEntry{}, "Times"),
}

// TestDescribeCataloguesTheWholeGraph pins the answer of a describe that names
// no seed: every fact in the graph, every anchor they are filed under with the
// facts each files, and when the facts hold on each clock. Each time is the
// hull of what the facts hold on that clock: happened falls back to said and
// said to written, so a fact given no time widens all three to the instant it
// was written. The lists lead with the anchor filing most facts, ties by
// value. The answer carries no hit and a zero count, and the selector names
// the graph the catalogue is of.
//
// NOTE: a span whose last fact holds a single instant ends at that instant,
// which the half-open interval leaves out: [containers.Interval.Hull] ends the
// hull of two instants at the later one, not past it, and a when over several
// hits answers the same way. Once Hull holds an instant beyond its end, these
// ends move past the last fact's instant.
func TestDescribeCataloguesTheWholeGraph(t *testing.T) {
	g, cfg := aviary(t)
	const (
		nests = "the heron nests by the lake"
		fish  = "the heron caught a fish"
		dives = "the kingfisher dives"
		leaks = "the boat leaks"
	)
	leaked := written(t, g, leaks)
	at := func(clock containers.Interval) query.Times {
		return query.Times{Happened: clock, Said: clock, Written: clock}
	}
	catalogue := func(id uint8) query.Catalogue {
		return query.Catalogue{
			Graph:   id,
			Facts:   4,
			Anchors: 7,
			Times: query.Times{
				Happened: containers.Interval{Start: may(7), End: leaked.End},
				Said:     containers.Interval{Start: may(8), End: leaked.End},
				Written:  written(t, g, nests, fish, dives, leaks),
			},
			Topics: []query.CatalogueEntry{
				{Value: "birds", Facts: 3, Times: query.Times{Happened: span(7, 20), Said: span(8, 20), Written: written(t, g, nests, fish, dives)}},
				{Value: "fishing", Facts: 2, Times: query.Times{Happened: span(10, 20), Said: span(12, 20), Written: written(t, g, fish, dives)}},
				{Value: "boats", Facts: 1, Times: at(leaked)},
				{Value: "lake", Facts: 1, Times: query.Times{Happened: span(7, 8), Said: span(8, 8), Written: written(t, g, nests)}},
			},
			Entities: []query.CatalogueEntry{
				{Value: "heron", Facts: 2, Times: query.Times{Happened: span(7, 12), Said: span(8, 12), Written: written(t, g, nests, fish)}},
				{Value: "kingfisher", Facts: 1, Times: query.Times{Happened: span(20, 20), Said: span(20, 20), Written: written(t, g, dives)}},
				{Value: "otto", Facts: 1, Times: at(leaked)},
			},
		}
	}
	tests := map[string]query.Catalogue{
		"describe recall":   catalogue(0),
		"describe recall@2": catalogue(2),
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			if stream.Result.Catalogue == nil {
				t.Fatalf("%s: Result.Catalogue = nil, want the catalogue of the graph", q)
			}
			if diff := cmp.Diff(want, *stream.Result.Catalogue); diff != "" {
				t.Errorf("%s catalogue mismatch (-want +got):\n%s", q, diff)
			}
			if stream.Result.Count != 0 || stream.Result.Hits == nil || len(stream.Result.Hits) != 0 {
				t.Errorf("%s: Result = %d hits (nil %v), count %d, want an empty list and a zero count", q, len(stream.Result.Hits), stream.Result.Hits == nil, stream.Result.Count)
			}
			if len(stream.Warnings) != 0 || stream.IsGraphEmpty {
				t.Errorf("%s: Warnings = %q, IsGraphEmpty = %v, want neither", q, stream.Warnings, stream.IsGraphEmpty)
			}
		})
	}
}

// TestDescribeKeepsTheFactsInsideItsWindow pins the window a describe reads
// inside, whichever form its recall takes. A fact is counted when it holds
// inside since and until on the happened clock, as a recall's hit must, and
// so are only the anchors those facts are filed under. A bound read from a
// when draws the window it draws for a recall: since keeps what came after
// the answer, from the instant the answer ends.
func TestDescribeKeepsTheFactsInsideItsWindow(t *testing.T) {
	g, cfg := aviary(t)
	tests := map[string]query.Catalogue{
		"describe recall since:'2023-05-11'": {
			Scope:   "since:'2023-05-11'",
			Facts:   3,
			Anchors: 6,
			Topics: []query.CatalogueEntry{
				{Value: "birds", Facts: 2},
				{Value: "fishing", Facts: 2},
				{Value: "boats", Facts: 1},
			},
			Entities: []query.CatalogueEntry{
				{Value: "heron", Facts: 1},
				{Value: "kingfisher", Facts: 1},
				{Value: "otto", Facts: 1},
			},
		},
		"describe recall until:'2023-05-11'": {
			Scope:   "until:'2023-05-11'",
			Facts:   2,
			Anchors: 4,
			Topics: []query.CatalogueEntry{
				{Value: "birds", Facts: 2},
				{Value: "fishing", Facts: 1},
				{Value: "lake", Facts: 1},
			},
			Entities: []query.CatalogueEntry{{Value: "heron", Facts: 2}},
		},
		"describe recall since:'2023-05-09' until:'2023-05-15'": {
			Scope:    "since:'2023-05-09' until:'2023-05-15'",
			Facts:    1,
			Anchors:  3,
			Topics:   []query.CatalogueEntry{{Value: "birds", Facts: 1}, {Value: "fishing", Facts: 1}},
			Entities: []query.CatalogueEntry{{Value: "heron", Facts: 1}},
		},
		"describe recall since:(when kingfisher)": {
			Scope:    "since:(when kingfisher)",
			Facts:    2,
			Anchors:  5,
			Topics:   []query.CatalogueEntry{{Value: "birds", Facts: 1}, {Value: "boats", Facts: 1}, {Value: "fishing", Facts: 1}},
			Entities: []query.CatalogueEntry{{Value: "kingfisher", Facts: 1}, {Value: "otto", Facts: 1}},
		},
		"describe recall entity:heron since:'2023-05-09'": {
			Scope:    "entity:heron since:'2023-05-09'",
			Facts:    1,
			Anchors:  3,
			Topics:   []query.CatalogueEntry{{Value: "birds", Facts: 1}, {Value: "fishing", Facts: 1}},
			Entities: []query.CatalogueEntry{{Value: "heron", Facts: 1}},
		},
		"describe recall heron until:'2023-05-09'": {
			Scope:    "heron until:'2023-05-09'",
			Facts:    1,
			Anchors:  3,
			Topics:   []query.CatalogueEntry{{Value: "birds", Facts: 1}, {Value: "lake", Facts: 1}},
			Entities: []query.CatalogueEntry{{Value: "heron", Facts: 1}},
		},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			if diff := cmp.Diff(want, *stream.Result.Catalogue, ignoreTimes); diff != "" {
				t.Errorf("%s catalogue mismatch (-want +got):\n%s", q, diff)
			}
			if len(stream.Warnings) != 0 {
				t.Errorf("%s: Warnings = %q, want none when every bound was answered", q, stream.Warnings)
			}
		})
	}
}

// TestDescribeOfAnchorsCountsWhatTheyFile pins a describe whose recall names
// anchors alone. It reaches every fact filed under one of them, as the
// recall's search seeds them, and a fact filed under two of them is one fact
// of the set. Every other anchor counts the facts it shares with the named
// ones. top caps each list and not the anchors counted, and an anchor named in
// the query has no claim to lead: under topic:fishing, birds files as many of
// the facts and leads by value.
func TestDescribeOfAnchorsCountsWhatTheyFile(t *testing.T) {
	g, cfg := aviary(t)
	tests := map[string]query.Catalogue{
		"describe recall entity:heron": {
			Scope:   "entity:heron",
			Facts:   2,
			Anchors: 4,
			Topics: []query.CatalogueEntry{
				{Value: "birds", Facts: 2},
				{Value: "fishing", Facts: 1},
				{Value: "lake", Facts: 1},
			},
			Entities: []query.CatalogueEntry{{Value: "heron", Facts: 2}},
		},
		"describe recall entity:heron top:1": {
			Scope:    "entity:heron top:1",
			Facts:    2,
			Anchors:  4,
			Topics:   []query.CatalogueEntry{{Value: "birds", Facts: 2}},
			Entities: []query.CatalogueEntry{{Value: "heron", Facts: 2}},
		},
		"describe recall entity:heron topic:fishing": {
			Scope:   "entity:heron topic:fishing",
			Facts:   3,
			Anchors: 5,
			Topics: []query.CatalogueEntry{
				{Value: "birds", Facts: 3},
				{Value: "fishing", Facts: 2},
				{Value: "lake", Facts: 1},
			},
			Entities: []query.CatalogueEntry{{Value: "heron", Facts: 2}, {Value: "kingfisher", Facts: 1}},
		},
		"describe recall topic:fishing top:1": {
			Scope:    "topic:fishing top:1",
			Facts:    2,
			Anchors:  4,
			Topics:   []query.CatalogueEntry{{Value: "birds", Facts: 2}},
			Entities: []query.CatalogueEntry{{Value: "heron", Facts: 1}},
		},
		"describe recall topic:boats": {
			Scope:    "topic:boats",
			Facts:    1,
			Anchors:  2,
			Topics:   []query.CatalogueEntry{{Value: "boats", Facts: 1}},
			Entities: []query.CatalogueEntry{{Value: "otto", Facts: 1}},
		},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			if diff := cmp.Diff(want, *stream.Result.Catalogue, ignoreTimes); diff != "" {
				t.Errorf("%s catalogue mismatch (-want +got):\n%s", q, diff)
			}
		})
	}
}

// laps builds a graph of twelve facts holding the term lap, filed under
// running, and three holding swim, filed under swimming and the pool. Twelve
// is more than the default top, so a describe that stopped at the recall's
// top would count fewer laps than the graph holds. The window is off, so a
// term reaches the facts that hold it and no neighbour of theirs.
func laps(t *testing.T) (graph.Graph[uint64, float32], *config.ConfigSet) {
	t.Helper()
	cfg := config.New()
	cfg.DB.WindowGamma = -1
	g := graph.NewGraph[uint64, float32](cfg)
	for i := range 12 {
		commit(t, g, cfg, fmt.Sprintf("remember 'lap %d' topic:running", i+1), false)
	}
	for i := range 3 {
		commit(t, g, cfg, fmt.Sprintf("remember 'swim %d' topic:swimming entity:pool", i+1), false)
	}
	return g, cfg
}

// TestDescribeOfTermsCountsEveryMatch pins a describe whose recall names
// terms. It reaches every fact the recall would rank, not the top few it
// would return: the recall of the same terms stops at the default top, and
// the describe counts every match beyond it. It reaches only the facts that
// match, so the anchors of the others stay out of the lists.
func TestDescribeOfTermsCountsEveryMatch(t *testing.T) {
	g, cfg := laps(t)
	tests := map[string]query.Catalogue{
		"describe recall lap": {
			Scope:    "lap",
			Facts:    12,
			Anchors:  1,
			Topics:   []query.CatalogueEntry{{Value: "running", Facts: 12}},
			Entities: []query.CatalogueEntry{},
		},
		"describe recall swim": {
			Scope:    "swim",
			Facts:    3,
			Anchors:  2,
			Topics:   []query.CatalogueEntry{{Value: "swimming", Facts: 3}},
			Entities: []query.CatalogueEntry{{Value: "pool", Facts: 3}},
		},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			if diff := cmp.Diff(want, *stream.Result.Catalogue, ignoreTimes); diff != "" {
				t.Errorf("%s catalogue mismatch (-want +got):\n%s", q, diff)
			}
		})
	}

	if got := commit(t, g, cfg, "recall lap", false).Result.Count; got != cfg.DB.DefaultTop {
		t.Errorf("recall lap returned %d hits, want the default top %d the describe reaches past", got, cfg.DB.DefaultTop)
	}
}

// TestDescribeReachesWhatItsRecallRanks pins the set a describe counts to the
// one its recall ranks, read through the same search with no top. With a score
// cutoff set, a recall of anchors alone drops a fact filed under one of two
// named anchors, since its relevance is half the best. A describe that read
// the anchors' members off the graph would count it anyway, and picture a set
// the recall never ranks. So each describe here counts exactly the facts its
// recall returns uncapped, cutoff included.
func TestDescribeReachesWhatItsRecallRanks(t *testing.T) {
	cfg := config.New()
	cfg.DB.WindowGamma = -1
	cfg.DB.MinScoreRatio = 0.6
	g := graph.NewGraph[uint64, float32](cfg)
	for _, statement := range []string{
		"remember 'tern over the cliff' topic:coast entity:tern",
		"remember 'cliff path at low tide' topic:coast",
		"remember 'tern chick fed' entity:tern",
	} {
		commit(t, g, cfg, statement, false)
	}

	bodies := map[string]struct{}{
		"topic:coast entity:tern": {},
		"cliff":                   {},
		"tern topic:coast":        {},
	}
	for body := range bodies {
		t.Run(body, func(t *testing.T) {
			recalled := commit(t, g, cfg, fmt.Sprintf("recall %s top:%d", body, cfg.DB.MaxTop), false).Result
			described := commit(t, g, cfg, "describe recall "+body, false).Result.Catalogue

			if got, want := described.Facts, recalled.Count; got != want {
				t.Errorf("describe recall %s counts %d facts, want the %d its recall ranks", body, got, want)
			}
		})
	}
}

// TestDescribeOfAVectorReadsEveryVectoredFact pins a describe whose recall
// names a vector. The vector index ranks every fact that holds a vector,
// however far from the query, so the describe reaches each of them, more than
// the seed budget and the default top, and none of the facts written without
// one.
func TestDescribeOfAVectorReadsEveryVectoredFact(t *testing.T) {
	cfg := config.New()
	cfg.DB.WindowGamma = -1
	g := graph.NewGraph[uint64, float32](cfg)
	write := func(q string, params map[string][]float32) {
		t.Helper()
		parsed, _, err := query.Parse[uint64, float32](q, params, cfg)
		if err != nil {
			t.Fatalf("Parse(%q) = %v, want nil", q, err)
		}
		if err := query.NewStream[uint64, float32](parsed).Commit(g); err != nil {
			t.Fatalf("Commit(%q) = %v, want nil", q, err)
		}
	}
	for i := range 12 {
		write(fmt.Sprintf("remember 'heron %d' topic:birds vec:$v", i+1), map[string][]float32{"v": {1, float32(i), 0}})
	}
	write("remember 'the boat leaks' topic:boats entity:otto", nil)

	const q = "describe recall vec:$v"
	parsed, _, err := query.Parse[uint64, float32](q, map[string][]float32{"v": {0, 0, 1}}, cfg)
	if err != nil {
		t.Fatalf("Parse(%q) = %v, want nil", q, err)
	}
	stream, err := parsed.Plan(cfg)
	if err != nil {
		t.Fatalf("Plan(%q) = %v, want nil", q, err)
	}
	if err := stream.Commit(g); err != nil {
		t.Fatalf("Commit(%q) = %v, want nil", q, err)
	}

	want := query.Catalogue{
		Scope:    "vec:$v",
		Facts:    12,
		Anchors:  1,
		Topics:   []query.CatalogueEntry{{Value: "birds", Facts: 12}},
		Entities: []query.CatalogueEntry{},
	}
	if diff := cmp.Diff(want, *stream.Result.Catalogue, ignoreTimes); diff != "" {
		t.Errorf("%s catalogue mismatch (-want +got):\n%s", q, diff)
	}
}

// TestDescribeWithAnUnansweredBoundReachesNothing pins what a describe does
// when a when in one of its recall's bounds matched no fact. The window has a
// bound with nothing to compare with, and describing without it would count
// the facts the query asked to leave out, so the describe reaches no fact,
// whichever form its recall takes. The stream warns once per bound left
// unanswered, naming the clause, as a recall's does. The graph is populated,
// so the stream does not report it empty.
func TestDescribeWithAnUnansweredBoundReachesNothing(t *testing.T) {
	const unanswered = "the when matched no fact, so it has no time to answer with; the recall has no bound to read and returns no hits"
	g, cfg := aviary(t)
	tests := map[string][]string{
		"describe recall until:(when zebras)":                                      {"until: " + unanswered},
		"describe recall heron since:(when zebras)":                                {"since: " + unanswered},
		"describe recall entity:heron since:(when kingfisher) until:(when zebras)": {"until: " + unanswered},
		"describe recall since:(when zebras) until:(when unicorns)":                {"since: " + unanswered, "until: " + unanswered},
	}
	for q, want := range tests {
		t.Run(q, func(t *testing.T) {
			stream := commit(t, g, cfg, q, false)

			got := *stream.Result.Catalogue
			if got.Facts != 0 || got.Anchors != 0 || len(got.Topics) != 0 || len(got.Entities) != 0 {
				t.Errorf("%s catalogue = %+v, want no fact and no anchor", q, got)
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

// TestDescribeThatReachesNothing pins the empty catalogue. It counts no fact
// and no anchor, holds no time, and still carries both lists, empty, so a
// caller reads the same keys whatever the graph holds. Its result is a zero
// count and an empty list of hits, as a recall's is. In a graph that holds
// nothing the stream reports the graph empty, so the server answers 204 as it
// does for a recall. A populated graph the describe missed is not reported
// empty, and none of these warns.
func TestDescribeThatReachesNothing(t *testing.T) {
	populated, cfg := aviary(t)
	empty := graph.NewGraph[uint64, float32](cfg)
	tests := map[string]struct {
		graph     graph.Graph[uint64, float32]
		query     string
		scope     string
		wantEmpty bool
	}{
		"the whole of an empty graph":      {graph: empty, query: "describe recall", wantEmpty: true},
		"a window of an empty graph":       {graph: empty, query: "describe recall since:7d", scope: "since:7d", wantEmpty: true},
		"a term in an empty graph":         {graph: empty, query: "describe recall heron", scope: "heron", wantEmpty: true},
		"an anchor in an empty graph":      {graph: empty, query: "describe recall entity:heron", scope: "entity:heron", wantEmpty: true},
		"a term no fact holds":             {graph: populated, query: "describe recall zebras", scope: "zebras"},
		"an anchor nothing is filed under": {graph: populated, query: "describe recall entity:zebra", scope: "entity:zebra"},
		"a window no fact holds in":        {graph: populated, query: "describe recall until:'2023-05-01'", scope: "until:'2023-05-01'"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stream := commit(t, tc.graph, cfg, tc.query, false)

			want := query.Catalogue{
				Scope:    tc.scope,
				Topics:   []query.CatalogueEntry{},
				Entities: []query.CatalogueEntry{},
			}
			if stream.Result.Catalogue == nil {
				t.Fatalf("%s: Result.Catalogue = nil, want an empty catalogue", tc.query)
			}
			if diff := cmp.Diff(want, *stream.Result.Catalogue); diff != "" {
				t.Errorf("%s catalogue mismatch (-want +got):\n%s", tc.query, diff)
			}
			if stream.Result.Count != 0 || stream.Result.Hits == nil || len(stream.Result.Hits) != 0 {
				t.Errorf("%s: Result = %d hits (nil %v), count %d, want an empty list and a zero count", tc.query, len(stream.Result.Hits), stream.Result.Hits == nil, stream.Result.Count)
			}
			if stream.IsGraphEmpty != tc.wantEmpty {
				t.Errorf("%s: IsGraphEmpty = %v, want %v", tc.query, stream.IsGraphEmpty, tc.wantEmpty)
			}
			if len(stream.Warnings) != 0 {
				t.Errorf("%s: Warnings = %q, want none", tc.query, stream.Warnings)
			}
		})
	}
}

// TestDescribeOfAFactNotOverHasNoEnd pins the hull of a set holding a fact
// that has not ended. It has no end on the happened clock, so neither has the
// set's time, nor the time of an anchor filing it. The said and written
// clocks read instants, which always end.
func TestDescribeOfAFactNotOverHasNoEnd(t *testing.T) {
	cfg := config.New()
	g := graph.NewGraph[uint64, float32](cfg)
	for _, statement := range []string{
		"remember 'the heron nests' topic:birds said:'2023-05-08' occurred:'2023-05-07'",
		"remember 'the heron moved in' topic:birds said:'2023-05-21' occurred:'2023-05-20'..",
	} {
		commit(t, g, cfg, statement, false)
	}

	stream := commit(t, g, cfg, "describe recall", false)

	want := query.Times{
		Happened: containers.Interval{Start: may(7)},
		Said:     span(8, 21),
		Written:  written(t, g, "the heron nests", "the heron moved in"),
	}
	got := stream.Result.Catalogue
	if diff := cmp.Diff(want, got.Times); diff != "" {
		t.Errorf("catalogue times mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]query.CatalogueEntry{{Value: "birds", Facts: 2, Times: want}}, got.Topics); diff != "" {
		t.Errorf("topics mismatch (-want +got):\n%s", diff)
	}
}

// TestDescribeIsTheSameExplained pins that explain adds nothing to a
// describe. Its answer carries no hit to attach contributions to, and the
// catalogue it answers with is the one an ordinary describe gives, with no
// background rate and no nested results beside it.
func TestDescribeIsTheSameExplained(t *testing.T) {
	g, cfg := aviary(t)
	const q = "describe recall entity:heron since:(when kingfisher)"

	plain := commit(t, g, cfg, q, false)
	explained := commit(t, g, cfg, q, true)

	if diff := cmp.Diff(plain.Result, explained.Result); diff != "" {
		t.Errorf("%s explained mismatch (-plain +explained):\n%s", q, diff)
	}
}
