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
	"fmt"
	"strings"
	"time"

	"github.com/FraiseHQ/fraise/internal/containers"
)

// The renderers produce the model-facing text that rides beside the
// structured payload. They use the voice of the Python integrations: one line
// per hit, best first, the dates the fact holds in front of it and its
// relevance to three decimals after. An empty result says so in words, since
// an empty string reads as a failure to a model. Warnings follow, one per
// line. The query ran, and the model needs them to fix its next one.

// renderRecall renders a recall response for the model. A recall can come
// back with nothing in two ways, and the daemon tells them apart by status.
// Either the graph holds nothing at all, or a populated graph has no matching
// fact. graphEmpty marks the first case. The model must tell them apart. The
// first calls for remembering something, the second for asking differently.
// A single "no match" line would send the model rephrasing a query against a
// graph that was never written to.
//
// A when answers with a time beside its hits, and that time leads the text.
// It is what the model asked for, and the hits under it are where it was read
// from.
func renderRecall(out RecallOutput, graphEmpty bool) string {
	var lines []string
	if !out.Results.Start.IsZero() {
		lines = append(lines, "when: "+renderSpan(containers.Interval{Start: out.Results.Start, End: out.Results.End}))
	}
	if len(out.Results.Hits) == 0 {
		if graphEmpty {
			lines = append(lines, "No stored facts matched: this graph is empty, nothing has been remembered in it yet.")
		} else {
			lines = append(lines, "No stored facts matched.")
		}
	}
	for _, hit := range out.Results.Hits {
		lines = append(lines, renderHit(hit))
	}
	for _, warning := range out.Warnings {
		lines = append(lines, "warning: "+warning)
	}
	return strings.Join(lines, "\n")
}

// renderHit renders one hit as a line: the dates the fact holds, its value,
// then its relevance. The dates are when it was said and when what it states
// happened, each read as a bound reads it: a fact given no occurred: happened
// when it was said, and one given no said: was said when it was written. A
// model answering "what did she do the day before she told me?" needs both,
// and neither is in the value. When the two read the same, the line carries
// that one date, so a fact given no times shows the day it was written.
func renderHit(hit Hit) string {
	said := hit.Said
	if said.IsZero() {
		said = hit.Timestamp
	}
	dates := said.Format(time.DateOnly)
	if len(hit.Occurred) > 0 {
		spans := make([]string, len(hit.Occurred))
		for i, span := range hit.Occurred {
			spans[i] = renderSpan(span)
		}
		if happened := strings.Join(spans, " and "); happened != dates {
			dates = "said " + dates + ", happened " + happened
		}
	}
	return fmt.Sprintf("- [%s] %s (relevance %.3f)", dates, hit.Value, hit.Score)
}

// renderSpan renders an interval in the widest calendar unit that loses none
// of it: a year, a month or a day, as one unit or as first..last with both
// inside the span. So a time reads back as the literal that wrote it:
// occurred:'2023-05' as 2023-05, '2023-05-10'..'2023-05-16' as
// 2023-05-10..2023-05-16, and neither as the day it starts on. A span that
// has not ended is its first day and a trailing "..". A single moment is the
// day it falls on. The day is the finest unit a line carries, and the
// structured payload has the instant.
func renderSpan(span containers.Interval) string {
	if span.End.IsZero() {
		return span.Start.Format(time.DateOnly) + ".."
	}
	if !span.End.After(span.Start) {
		return span.Start.Format(time.DateOnly)
	}
	layout := time.DateOnly
	for _, unit := range []string{"2006-01", "2006"} {
		// An edge sits on a unit's boundary when writing it in that unit and
		// reading it back gives the same instant. A span whose edges both do
		// is whole units of it. A month comes before a year, so the loop
		// stops at the first unit an edge falls inside.
		start, _ := time.ParseInLocation(unit, span.Start.Format(unit), span.Start.Location())
		end, _ := time.ParseInLocation(unit, span.End.Format(unit), span.End.Location())
		if !start.Equal(span.Start) || !end.Equal(span.End) {
			break
		}
		layout = unit
	}
	// End is outside the span, so its last unit is the one the instant before
	// End falls in.
	first, last := span.Start.Format(layout), span.End.Add(-time.Nanosecond).Format(layout)
	if first == last {
		return first
	}
	return first + ".." + last
}

// renderRemember confirms a write for the model. The daemon acknowledges a
// write with a status token rather than a payload. So the confirmation is
// fixed text, and only warnings vary.
func renderRemember(out RememberOutput) string {
	lines := []string{"Remembered."}
	for _, warning := range out.Warnings {
		lines = append(lines, "warning: "+warning)
	}
	return strings.Join(lines, "\n")
}
