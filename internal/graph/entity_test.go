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

package graph_test

import (
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/google/go-cmp/cmp"
)

// TestFactIntervals pins how a fact reads on each clock. A clock the fact was
// given no time on reads the next one, happened then said then written, so a
// window on any clock still places every fact. If the chain broke, a fact
// remembered without times would fall out of every dated recall, and a fact
// given only said would be invisible to the bounds that read happened.
func TestFactIntervals(t *testing.T) {
	written := time.Date(2023, 6, 1, 9, 0, 0, 0, time.UTC)
	said := time.Date(2023, 5, 8, 13, 56, 0, 0, time.UTC)
	race := containers.Interval{Start: time.Date(2023, 5, 7, 0, 0, 0, 0, time.UTC), End: time.Date(2023, 5, 8, 0, 0, 0, 0, time.UTC)}
	camp := containers.Interval{Start: time.Date(2023, 6, 10, 0, 0, 0, 0, time.UTC), End: time.Date(2023, 6, 17, 0, 0, 0, 0, time.UTC)}
	at := func(moment time.Time) []containers.Interval {
		return []containers.Interval{{Start: moment, End: moment}}
	}

	tests := map[string]struct {
		fact graph.Fact[string]
		want map[containers.Clock][]containers.Interval
	}{
		"a fact given no times reads its timestamp on every clock": {
			fact: graph.Fact[string]{NodeAttributes: graph.NodeAttributes{Timestamp: written}},
			want: map[containers.Clock][]containers.Interval{
				containers.Happened: at(written),
				containers.Said:     at(written),
				containers.Written:  at(written),
			},
		},
		"said alone is also when it happened": {
			fact: graph.Fact[string]{NodeAttributes: graph.NodeAttributes{Timestamp: written}, Said: said},
			want: map[containers.Clock][]containers.Interval{
				containers.Happened: at(said),
				containers.Said:     at(said),
				containers.Written:  at(written),
			},
		},
		"occurred alone leaves said on the timestamp": {
			fact: graph.Fact[string]{NodeAttributes: graph.NodeAttributes{Timestamp: written}, Occurred: []containers.Interval{race}},
			want: map[containers.Clock][]containers.Interval{
				containers.Happened: {race},
				containers.Said:     at(written),
				containers.Written:  at(written),
			},
		},
		"each clock reads its own time when all are given": {
			fact: graph.Fact[string]{NodeAttributes: graph.NodeAttributes{Timestamp: written}, Said: said, Occurred: []containers.Interval{race, camp}},
			want: map[containers.Clock][]containers.Interval{
				containers.Happened: {race, camp},
				containers.Said:     at(said),
				containers.Written:  at(written),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for clock, want := range tc.want {
				if diff := cmp.Diff(want, tc.fact.Intervals(clock)); diff != "" {
					t.Errorf("Intervals(%v) mismatch (-want +got):\n%s", clock, diff)
				}
			}
		})
	}
}
