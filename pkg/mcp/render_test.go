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

package mcp

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/FraiseHQ/fraise/internal/containers"
)

// TestRenderRecall pins the model-facing text exactly. Each hit gets one line,
// in response order, with the dates its fact holds in front and its relevance
// to three decimals after. An empty result is said in words, since an empty
// string reads as failure to a model. Warnings follow the hits, one per line.
// The wording matches the Python integrations' voice on purpose. An agent that
// switches transports should not meet a second dialect.
//
// The dates are the fact's said and happened times as a bound reads them, so
// a fact given neither shows the day it was written, and the line names both
// only where they differ. A span reads in the unit it was written in. If a
// month rendered as its first day, a model would answer "on the first of May"
// to a fact that only says "in May".
//
// A describe's catalogue renders in place of the hits. Its first line spells
// the describe the server read, graph and scope, then how many facts it
// reached under how many anchors and when they happened. The topics follow,
// then the entities, each in the order the server listed them with when its
// facts happened in front. A kind with no anchor says none, since a missing
// section reads as a list cut off. A describe that reached no fact says so on
// its one line. Warnings follow, as they follow hits.
func TestRenderRecall(t *testing.T) {
	written := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	date := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	evening := time.Date(2023, time.May, 7, 18, 30, 0, 0, time.UTC)

	cases := map[string]struct {
		out RecallOutput
		// graphEmpty is the daemon's 204, which the bridge reads off the
		// status line. The recall found nothing because the graph holds
		// nothing, not because the query missed.
		graphEmpty bool
		want       string
	}{
		"hits render one line each, best first, under the day they were written": {
			out: RecallOutput{Results: Result{Count: 2, Hits: []Hit{
				{Value: "the barometer falls before the storm", Timestamp: written, Score: 1.25},
				{Value: "storm clouds gather at sea", Timestamp: written, Score: 0.412},
			}}},
			want: "- [2026-01-02] the barometer falls before the storm (relevance 1.250)\n" +
				"- [2026-01-02] storm clouds gather at sea (relevance 0.412)",
		},
		"a fact said on another day than it happened carries both": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna ran the charity race",
				Timestamp: written,
				Said:      date(2023, time.May, 8),
				Occurred:  []containers.Interval{{Start: date(2023, time.May, 7), End: date(2023, time.May, 8)}},
				Score:     1,
			}}}},
			want: "- [said 2023-05-08, happened 2023-05-07] anna ran the charity race (relevance 1.000)",
		},
		"a fact given only the day it was said happened that day": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna ran the charity race",
				Timestamp: written,
				Said:      date(2023, time.May, 8),
				Score:     1,
			}}}},
			want: "- [2023-05-08] anna ran the charity race (relevance 1.000)",
		},
		"a fact that happened the day it was said carries one date": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna ran the charity race",
				Timestamp: written,
				Said:      date(2023, time.May, 8),
				Occurred:  []containers.Interval{{Start: date(2023, time.May, 8), End: date(2023, time.May, 9)}},
				Score:     1,
			}}}},
			want: "- [2023-05-08] anna ran the charity race (relevance 1.000)",
		},
		"a fact given only when it happened was said when it was written": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna ran the charity race",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2023, time.May, 7), End: date(2023, time.May, 8)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05-07] anna ran the charity race (relevance 1.000)",
		},
		"a month reads as the month": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2023, time.May, 1), End: date(2023, time.June, 1)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05] anna trained for the race (relevance 1.000)",
		},
		"a year reads as the year": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2023, time.January, 1), End: date(2024, time.January, 1)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023] anna trained for the race (relevance 1.000)",
		},
		"a range of days reads from its first day to its last": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2023, time.May, 10), End: date(2023, time.May, 17)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05-10..2023-05-16] anna trained for the race (relevance 1.000)",
		},
		"a range of months reads from its first month to its last": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2023, time.May, 1), End: date(2023, time.July, 1)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05..2023-06] anna trained for the race (relevance 1.000)",
		},
		"a range of years reads from its first year to its last": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2022, time.January, 1), End: date(2024, time.January, 1)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2022..2023] anna trained for the race (relevance 1.000)",
		},
		"a span that has not ended trails off after its first day": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna lives by the harbour",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2023, time.May, 20)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05-20..] anna lives by the harbour (relevance 1.000)",
		},
		"a moment reads as the day it falls on": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna crossed the line",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: evening, End: evening}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05-07] anna crossed the line (relevance 1.000)",
		},
		"a span that starts inside a day reads in days": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna sailed through the night",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: evening, End: date(2023, time.June, 1)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05-07..2023-05-31] anna sailed through the night (relevance 1.000)",
		},
		"a range of months across a new year reads in months": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2022, time.November, 1), End: date(2023, time.February, 1)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2022-11..2023-01] anna trained for the race (relevance 1.000)",
		},
		"a span that ends inside a month reads in days": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: date(2023, time.May, 1), End: date(2023, time.May, 17)}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05-01..2023-05-16] anna trained for the race (relevance 1.000)",
		},
		"a span inside one day reads as that day": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna sailed all afternoon",
				Timestamp: written,
				Occurred:  []containers.Interval{{Start: time.Date(2023, time.May, 7, 9, 0, 0, 0, time.UTC), End: evening}},
				Score:     1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05-07] anna sailed all afternoon (relevance 1.000)",
		},
		"a month is judged in the offset it was written with": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred: []containers.Interval{{
					Start: time.Date(2023, time.May, 1, 0, 0, 0, 0, time.FixedZone("", 2*60*60)),
					End:   time.Date(2023, time.June, 1, 0, 0, 0, 0, time.FixedZone("", 2*60*60)),
				}},
				Score: 1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-05] anna trained for the race (relevance 1.000)",
		},
		"the same month read in another offset is a range of days": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna trained for the race",
				Timestamp: written,
				Occurred: []containers.Interval{{
					Start: time.Date(2023, time.April, 30, 22, 0, 0, 0, time.UTC),
					End:   time.Date(2023, time.May, 31, 22, 0, 0, 0, time.UTC),
				}},
				Score: 1,
			}}}},
			want: "- [said 2026-01-02, happened 2023-04-30..2023-05-31] anna trained for the race (relevance 1.000)",
		},
		"what happened more than once names each time": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna ran the harbour drills",
				Timestamp: written,
				Said:      date(2023, time.June, 2),
				Occurred: []containers.Interval{
					{Start: date(2023, time.May, 1), End: date(2023, time.May, 2)},
					{Start: date(2023, time.May, 28), End: date(2023, time.May, 29)},
				},
				Score: 1,
			}}}},
			want: "- [said 2023-06-02, happened 2023-05-01 and 2023-05-28] anna ran the harbour drills (relevance 1.000)",
		},
		"a date is read in the offset it was written with": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{{
				Value:     "anna called late that night",
				Timestamp: written,
				Said:      time.Date(2023, time.May, 8, 23, 30, 0, 0, time.FixedZone("", -7*60*60)),
				Score:     1,
			}}}},
			want: "- [2023-05-08] anna called late that night (relevance 1.000)",
		},
		"a when leads with the time it answers with": {
			out: RecallOutput{Results: Result{
				Count: 1,
				Hits: []Hit{{
					Value:     "anna ran the charity race",
					Timestamp: written,
					Said:      date(2023, time.May, 8),
					Occurred:  []containers.Interval{{Start: date(2023, time.May, 7), End: date(2023, time.May, 8)}},
					Score:     1,
				}},
				Start: date(2023, time.May, 7),
				End:   date(2023, time.May, 8),
			}},
			want: "when: 2023-05-07\n" +
				"- [said 2023-05-08, happened 2023-05-07] anna ran the charity race (relevance 1.000)",
		},
		"a when over several facts leads with the span that holds them all": {
			out: RecallOutput{Results: Result{
				Count: 2,
				Hits: []Hit{
					{
						Value:     "anna ran the charity race",
						Timestamp: written,
						Occurred:  []containers.Interval{{Start: date(2023, time.May, 7), End: date(2023, time.May, 8)}},
						Score:     1,
					},
					{
						Value:     "anna trained for the race",
						Timestamp: written,
						Occurred:  []containers.Interval{{Start: date(2023, time.May, 10), End: date(2023, time.May, 17)}},
						Score:     0.5,
					},
				},
				Start: date(2023, time.May, 7),
				End:   date(2023, time.May, 17),
			}},
			want: "when: 2023-05-07..2023-05-16\n" +
				"- [said 2026-01-02, happened 2023-05-07] anna ran the charity race (relevance 1.000)\n" +
				"- [said 2026-01-02, happened 2023-05-10..2023-05-16] anna trained for the race (relevance 0.500)",
		},
		"a when that reads what has not ended leads with an open span": {
			out: RecallOutput{Results: Result{
				Count: 1,
				Hits: []Hit{{
					Value:     "anna lives by the harbour",
					Timestamp: written,
					Occurred:  []containers.Interval{{Start: date(2023, time.May, 20)}},
					Score:     1,
				}},
				Start: date(2023, time.May, 20),
			}},
			want: "when: 2023-05-20..\n" +
				"- [said 2026-01-02, happened 2023-05-20..] anna lives by the harbour (relevance 1.000)",
		},
		"an empty result says so in words": {
			out:  RecallOutput{Results: Result{Count: 0, Hits: []Hit{}}},
			want: "No stored facts matched.",
		},
		"an empty graph says that instead, so the model writes rather than rephrases": {
			out:        RecallOutput{Results: Result{Count: 0, Hits: []Hit{}}},
			graphEmpty: true,
			want:       "No stored facts matched: this graph is empty, nothing has been remembered in it yet.",
		},
		"warnings follow the hits, one per line": {
			out: RecallOutput{
				Results:  Result{Count: 1, Hits: []Hit{{Value: "since the storm", Timestamp: written, Score: 1}}},
				Warnings: []string{`term "since" is also a keyword`},
			},
			want: "- [2026-01-02] since the storm (relevance 1.000)\n" +
				`warning: term "since" is also a keyword`,
		},
		"a when that matched nothing has no time to lead with": {
			out: RecallOutput{
				Results:  Result{Count: 0, Hits: []Hit{}},
				Warnings: []string{"the when matched no fact, so it has no time to answer with"},
			},
			want: "No stored facts matched.\n" +
				"warning: the when matched no fact, so it has no time to answer with",
		},
		"a describe of the whole graph names its size, then each kind, most filed first": {
			out: RecallOutput{Results: Result{Count: 0, Hits: []Hit{}, Catalogue: &Catalogue{
				Graph:    0,
				Facts:    4,
				Anchors:  4,
				Happened: containers.Interval{Start: date(2023, time.May, 1), End: date(2023, time.July, 1)},
				Topics: []CatalogueEntry{
					{Value: "birds", Facts: 3, Happened: containers.Interval{Start: date(2023, time.May, 1), End: date(2023, time.July, 1)}},
					{Value: "lake", Facts: 2, Happened: containers.Interval{Start: date(2023, time.May, 1), End: date(2023, time.May, 8)}},
				},
				Entities: []CatalogueEntry{
					{Value: "heron", Facts: 2, Happened: containers.Interval{Start: date(2023, time.May, 1), End: date(2023, time.May, 8)}},
					{Value: "swan", Facts: 2, Happened: containers.Interval{Start: date(2023, time.June, 1), End: date(2023, time.July, 1)}},
				},
			}}},
			want: "describe recall@0: 4 facts under 4 anchors, happened 2023-05..2023-06\n" +
				"topics:\n" +
				"- [2023-05..2023-06] birds: 3 facts\n" +
				"- [2023-05-01..2023-05-07] lake: 2 facts\n" +
				"entities:\n" +
				"- [2023-05-01..2023-05-07] heron: 2 facts\n" +
				"- [2023-06] swan: 2 facts",
		},
		"a scoped describe spells its scope, and what has not ended trails off": {
			out: RecallOutput{Results: Result{Count: 0, Hits: []Hit{}, Catalogue: &Catalogue{
				Graph:    2,
				Scope:    "topic:weather top:2",
				Facts:    3,
				Anchors:  5,
				Happened: containers.Interval{Start: date(2023, time.May, 1)},
				Topics: []CatalogueEntry{
					{Value: "weather", Facts: 3, Happened: containers.Interval{Start: date(2023, time.May, 1)}},
					{Value: "storms", Facts: 2, Happened: containers.Interval{Start: date(2023, time.May, 1), End: date(2023, time.May, 3)}},
				},
				Entities: []CatalogueEntry{
					{Value: "harbour", Facts: 2, Happened: containers.Interval{Start: date(2023, time.May, 2), End: date(2023, time.May, 3)}},
				},
			}}},
			want: "describe recall@2 topic:weather top:2: 3 facts under 5 anchors, happened 2023-05-01..\n" +
				"topics:\n" +
				"- [2023-05-01..] weather: 3 facts\n" +
				"- [2023-05-01..2023-05-02] storms: 2 facts\n" +
				"entities:\n" +
				"- [2023-05-02] harbour: 2 facts",
		},
		"a describe that reached no fact says so and lists nothing": {
			out: RecallOutput{Results: Result{Count: 0, Hits: []Hit{}, Catalogue: &Catalogue{
				Graph:    1,
				Scope:    "zebras",
				Topics:   []CatalogueEntry{},
				Entities: []CatalogueEntry{},
			}}},
			want: "describe recall@1 zebras: no fact reached.",
		},
		"a kind no fact is filed under says none": {
			out: RecallOutput{Results: Result{Count: 0, Hits: []Hit{}, Catalogue: &Catalogue{
				Graph:    0,
				Scope:    "harbour",
				Facts:    2,
				Anchors:  2,
				Happened: containers.Interval{Start: date(2023, time.May, 2), End: date(2023, time.May, 4)},
				Topics:   []CatalogueEntry{},
				Entities: []CatalogueEntry{
					{Value: "anna", Facts: 2, Happened: containers.Interval{Start: date(2023, time.May, 2), End: date(2023, time.May, 4)}},
					{Value: "harbour", Facts: 2, Happened: containers.Interval{Start: date(2023, time.May, 2), End: date(2023, time.May, 4)}},
				},
			}}},
			want: "describe recall@0 harbour: 2 facts under 2 anchors, happened 2023-05-02..2023-05-03\n" +
				"topics: none\n" +
				"entities:\n" +
				"- [2023-05-02..2023-05-03] anna: 2 facts\n" +
				"- [2023-05-02..2023-05-03] harbour: 2 facts",
		},
		"a count of one reads in the singular": {
			out: RecallOutput{Results: Result{Count: 0, Hits: []Hit{}, Catalogue: &Catalogue{
				Graph:    5,
				Facts:    1,
				Anchors:  1,
				Happened: containers.Interval{Start: date(2023, time.May, 2), End: date(2023, time.May, 2)},
				Topics:   []CatalogueEntry{},
				Entities: []CatalogueEntry{
					{Value: "bob", Facts: 1, Happened: containers.Interval{Start: date(2023, time.May, 2), End: date(2023, time.May, 2)}},
				},
			}}},
			want: "describe recall@5: 1 fact under 1 anchor, happened 2023-05-02\n" +
				"topics: none\n" +
				"entities:\n" +
				"- [2023-05-02] bob: 1 fact",
		},
		"facts filed under nothing list no anchor of either kind": {
			out: RecallOutput{Results: Result{Count: 0, Hits: []Hit{}, Catalogue: &Catalogue{
				Graph:    0,
				Facts:    2,
				Happened: containers.Interval{Start: date(2023, time.April, 20), End: date(2023, time.April, 22)},
				Topics:   []CatalogueEntry{},
				Entities: []CatalogueEntry{},
			}}},
			want: "describe recall@0: 2 facts under 0 anchors, happened 2023-04-20..2023-04-21\n" +
				"topics: none\n" +
				"entities: none",
		},
		"warnings follow the catalogue, one per line": {
			out: RecallOutput{
				Results: Result{Count: 0, Hits: []Hit{}, Catalogue: &Catalogue{
					Graph:    0,
					Scope:    "heron depth:2",
					Facts:    3,
					Anchors:  2,
					Happened: containers.Interval{Start: date(2023, time.May, 1), End: date(2023, time.July, 1)},
					Topics: []CatalogueEntry{
						{Value: "birds", Facts: 3, Happened: containers.Interval{Start: date(2023, time.May, 1), End: date(2023, time.July, 1)}},
					},
					Entities: []CatalogueEntry{
						{Value: "heron", Facts: 2, Happened: containers.Interval{Start: date(2023, time.May, 1), End: date(2023, time.May, 8)}},
					},
				}},
				Warnings: []string{"parse warning at column 29: depth:2 has no effect: the graph is searched only through a topic:/entity: anchor and this recall names none, so it runs on the text and vector indices alone"},
			},
			want: "describe recall@0 heron depth:2: 3 facts under 2 anchors, happened 2023-05..2023-06\n" +
				"topics:\n" +
				"- [2023-05..2023-06] birds: 3 facts\n" +
				"entities:\n" +
				"- [2023-05-01..2023-05-07] heron: 2 facts\n" +
				"warning: parse warning at column 29: depth:2 has no effect: the graph is searched only through a topic:/entity: anchor and this recall names none, so it runs on the text and vector indices alone",
		},
		"a describe whose bound read no time reached no fact, and the warning says why": {
			out: RecallOutput{
				Results: Result{Count: 0, Hits: []Hit{}, Catalogue: &Catalogue{
					Graph:    0,
					Scope:    "until:(when zebras)",
					Topics:   []CatalogueEntry{},
					Entities: []CatalogueEntry{},
				}},
				Warnings: []string{"until: the when matched no fact, so it has no time to answer with; the recall has no bound to read and returns no hits"},
			},
			want: "describe recall@0 until:(when zebras): no fact reached.\n" +
				"warning: until: the when matched no fact, so it has no time to answer with; the recall has no bound to read and returns no hits",
		},
		"an explained recall heads its hits with the query and breaks each hit down under its line": {
			out: RecallOutput{Results: Result{Count: 2, Hits: []Hit{
				{Value: "the barometer falls before the storm", Timestamp: written, Score: 2.5, Explain: &HitExplain{
					Seed:  true,
					Decay: 0.9999519,
					Contributions: []Contribution{
						{Source: "text", Score: 1.5, Rank: 0, Count: 1, Held: 0.75},
						{Source: "graph", Score: 3, Via: "weather", Degree: 3, Count: 2, Named: true},
					},
				}},
				{Value: "storm clouds gather at sea", Timestamp: written, Score: 0.25, Explain: &HitExplain{
					Decay: 0.5,
					Contributions: []Contribution{
						{Source: "graph", Score: 3, Via: "sea", Degree: 6, Count: 1},
					},
				}},
			}, Explain: &RecallExplain{
				Background: 0.125,
				Admission:  map[int]float64{2: 0.125, 1: 0.25},
				Seeds:      map[string]int{"text": 1, "vector": 0, "graph": 0, "anchor": 0},
				Terms:      []string{"barometer", "zebra"},
				Timings:    map[string]time.Duration{"text": 38 * time.Microsecond, "vector": 0, "graph": 2 * time.Millisecond, "anchor": 0},
			}}},
			want: "explain: background 0.125; admission 0.25 at depth 1, 0.125 at depth 2; seeds anchor 0, graph 0, text 1, vector 0; terms barometer, zebra; timings anchor 0s, graph 2ms, text 38µs, vector 0s\n" +
				"- [2026-01-02] the barometer falls before the storm (relevance 2.500)\n" +
				"    seed · decay 0.9999519 · text 1.500 (rank 0, held 0.750) · graph 3.000 via weather (named, 3 facts, 2 seeds)\n" +
				"- [2026-01-02] storm clouds gather at sea (relevance 0.250)\n" +
				"    not a seed · decay 0.5 · graph 3.000 via sea (6 facts, 1 seed)",
		},
		"an anchor-seeded recall reads each anchor a hit is filed under, and no admission": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{
				{Value: "the barometer falls before the storm", Timestamp: written, Score: 1, Explain: &HitExplain{
					Seed:          true,
					Decay:         1,
					Contributions: []Contribution{{Source: "anchor", Score: 1, Via: "weather", Degree: 7, Count: 1}},
				}},
			}, Explain: &RecallExplain{
				Seeds:   map[string]int{"text": 0, "vector": 0, "graph": 0, "anchor": 7},
				Terms:   []string{},
				Timings: map[string]time.Duration{"text": 0, "vector": 0, "graph": 0, "anchor": 7 * time.Microsecond},
			}}},
			want: "explain: background 0; seeds anchor 7, graph 0, text 0, vector 0; terms none; timings anchor 7µs, graph 0s, text 0s, vector 0s\n" +
				"- [2026-01-02] the barometer falls before the storm (relevance 1.000)\n" +
				"    seed · decay 1 · anchor 1.000 via weather (7 facts)",
		},
		"an explained bound whose when matched nothing says so": {
			out: RecallOutput{Results: Result{Count: 0, Hits: []Hit{}, Explain: &RecallExplain{
				Seeds:   map[string]int{"text": 0},
				Terms:   []string{"train"},
				Timings: map[string]time.Duration{"text": 0},
				When:    &NestedResults{Since: &Result{Count: 0, Hits: []Hit{}}},
			}}},
			want: "explain: background 0; seeds text 0; terms train; timings text 0s; since read from no fact\n" +
				"No stored facts matched.",
		},
		"an explained recall names the fact and the time each when in its bounds was read from": {
			out: RecallOutput{Results: Result{Count: 1, Hits: []Hit{
				{Value: "anna trained for weeks", Timestamp: written, Score: 1, Explain: &HitExplain{
					Seed:          true,
					Decay:         1,
					Contributions: []Contribution{{Source: "text", Score: 1, Rank: 0, Count: 1}},
				}},
			}, Explain: &RecallExplain{
				Seeds:   map[string]int{"text": 1},
				Terms:   []string{"train"},
				Timings: map[string]time.Duration{"text": time.Microsecond},
				When: &NestedResults{Until: &Result{
					Count: 1,
					Start: date(2023, time.May, 7),
					End:   date(2023, time.May, 8),
					Hits: []Hit{{Value: "anna ran the charity race", Timestamp: written, Said: date(2023, time.May, 8), Occurred: []containers.Interval{{Start: date(2023, time.May, 7), End: date(2023, time.May, 8)}}, Score: 2, Explain: &HitExplain{
						Seed:          true,
						Decay:         1,
						Contributions: []Contribution{{Source: "text", Score: 2, Rank: 0, Count: 1}},
					}}},
					Explain: &RecallExplain{
						Seeds:   map[string]int{"text": 1},
						Terms:   []string{"chariti", "race"},
						Timings: map[string]time.Duration{"text": time.Microsecond},
					},
				}},
			}}},
			want: "explain: background 0; seeds text 1; terms train; timings text 1µs; until read from \"anna ran the charity race\" → 2023-05-07\n" +
				"- [2026-01-02] anna trained for weeks (relevance 1.000)\n" +
				"    seed · decay 1 · text 1.000 (rank 0, held 0.000)",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := renderRecall(tc.out, tc.graphEmpty)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("renderRecall mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRenderRecallDistinguishesAnExplainedRecall pins that an explanation
// reaches the text. Each explained result must render differently from the
// same result with its explain blocks stripped, on the hits and on the query
// alike. Many clients hand the model the text alone, so an explain that
// renders as the plain recall answers the model's question with the ranking
// it already had, while the structured half carries what it asked for.
func TestRenderRecallDistinguishesAnExplainedRecall(t *testing.T) {
	written := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	hit := func(explain *HitExplain) Hit {
		return Hit{Value: "the barometer falls before the storm", Timestamp: written, Score: 1, Explain: explain}
	}
	seeded := &HitExplain{Seed: true, Decay: 1, Contributions: []Contribution{{Source: "text", Score: 1, Count: 1}}}
	query := &RecallExplain{Seeds: map[string]int{"text": 1}, Terms: []string{"baromet"}, Timings: map[string]time.Duration{"text": 0}}

	cases := map[string]Result{
		"a hit explained":         {Count: 1, Hits: []Hit{hit(seeded)}},
		"the query explained":     {Count: 1, Hits: []Hit{hit(nil)}, Explain: query},
		"both explained":          {Count: 1, Hits: []Hit{hit(seeded)}, Explain: query},
		"an explained empty read": {Count: 0, Hits: []Hit{}, Explain: query},
	}

	for name, explained := range cases {
		t.Run(name, func(t *testing.T) {
			plain := Result{Count: explained.Count, Hits: make([]Hit, len(explained.Hits))}
			for i, h := range explained.Hits {
				h.Explain = nil
				plain.Hits[i] = h
			}
			got := renderRecall(RecallOutput{Results: explained}, false)
			if want := renderRecall(RecallOutput{Results: plain}, false); got == want {
				t.Errorf("explained recall rendered as the plain one:\n%s", got)
			}
		})
	}
}

// TestRenderRemember pins the write confirmation. It is fixed text, since a
// write is acknowledged with a status token rather than a payload. Warnings
// are appended when the parse had something to say.
func TestRenderRemember(t *testing.T) {
	cases := map[string]struct {
		out  RememberOutput
		want string
	}{
		"a clean write is a bare confirmation": {
			out:  RememberOutput{Status: "ok"},
			want: "Remembered.",
		},
		"warnings ride under the confirmation": {
			out: RememberOutput{
				Status:   "ok",
				Warnings: []string{"parse warning at column 15"},
			},
			want: "Remembered.\nwarning: parse warning at column 15",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := renderRemember(tc.out); got != tc.want {
				t.Errorf("renderRemember = %q, want %q", got, tc.want)
			}
		})
	}
}
