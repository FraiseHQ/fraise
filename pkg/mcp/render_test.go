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

// TestRenderRemember pins the write confirmation. It is fixed text, since a
// write is acknowledged with a status token rather than a payload. Warnings
// are appended when the parse had something to say.
func TestRenderRemember(t *testing.T) {
	cases := []struct {
		name string
		out  RememberOutput
		want string
	}{
		{
			name: "a clean write is a bare confirmation",
			out:  RememberOutput{Status: "ok"},
			want: "Remembered.",
		},
		{
			name: "warnings ride under the confirmation",
			out: RememberOutput{
				Status:   "ok",
				Warnings: []string{"parse warning at column 15"},
			},
			want: "Remembered.\nwarning: parse warning at column 15",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderRemember(tc.out); got != tc.want {
				t.Errorf("renderRemember = %q, want %q", got, tc.want)
			}
		})
	}
}
