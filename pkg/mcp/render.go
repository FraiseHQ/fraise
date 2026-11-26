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
	"maps"
	"slices"
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
// from. A describe answers with a catalogue and no hits, rendered in their
// place.
//
// An explained recall carries its explanation in the text too, not only in
// the structured payload. Many clients hand the model the text alone, and an
// explain whose breakdown never reaches the model answers the question it
// asked with the plain ranking it already had.
func renderRecall(out RecallOutput, graphEmpty bool) string {
	lines := renderResult(out.Results, graphEmpty)
	for _, warning := range out.Warnings {
		lines = append(lines, "warning: "+warning)
	}
	return strings.Join(lines, "\n")
}

// renderResult renders one results envelope: the time a when answered with,
// the explain header of an explained recall, then the catalogue or the hits,
// each hit of an explained recall followed by its breakdown.
func renderResult(r Result, graphEmpty bool) []string {
	var lines []string
	if !r.Start.IsZero() {
		lines = append(lines, "when: "+renderSpan(containers.Interval{Start: r.Start, End: r.End}))
	}
	if r.Explain != nil {
		lines = append(lines, renderRecallExplain(*r.Explain))
	}
	if c := r.Catalogue; c != nil {
		lines = append(lines, renderCatalogue(*c)...)
	} else if len(r.Hits) == 0 {
		if graphEmpty {
			lines = append(lines, "No stored facts matched: this graph is empty, nothing has been remembered in it yet.")
		} else {
			lines = append(lines, "No stored facts matched.")
		}
	}
	for _, hit := range r.Hits {
		lines = append(lines, renderHit(hit))
		if hit.Explain != nil {
			lines = append(lines, renderHitExplain(*hit.Explain))
		}
	}
	return lines
}

// renderRecallExplain renders what an explained recall measured about the
// query as a whole, on one line above its hits: the background rate and the
// admission bars its anchors had to clear, how many facts each source seeded,
// the terms as the text index read them, and the facts and time each when in
// its bounds was read from. Sources are listed by name and depths in order,
// so the same recall renders the same text. The time each source took stays
// in the structured payload: it differs on every run and gives the model
// nothing to act on. A term no fact holds is listed too, which is how a model
// sees why a word matched nothing, and a bound read from the wrong fact is
// how it sees a window drawn in the wrong place. A when that kept several
// facts answers with the span that covers them all, so the bound names every
// one of them. The rates are written in full: they are small, and to three
// decimals most of them read as zero. A list with nothing in it says none,
// since an empty one reads as cut off.
func renderRecallExplain(e RecallExplain) string {
	parts := []string{fmt.Sprintf("background %g", e.Background)}
	if len(e.Admission) > 0 {
		bars := make([]string, 0, len(e.Admission))
		for _, depth := range slices.Sorted(maps.Keys(e.Admission)) {
			bars = append(bars, fmt.Sprintf("%g at depth %d", e.Admission[depth], depth))
		}
		parts = append(parts, "admission "+strings.Join(bars, ", "))
	}
	seeds := "none"
	if len(e.Seeds) > 0 {
		counts := make([]string, 0, len(e.Seeds))
		for _, source := range slices.Sorted(maps.Keys(e.Seeds)) {
			counts = append(counts, fmt.Sprintf("%s %d", source, e.Seeds[source]))
		}
		seeds = strings.Join(counts, ", ")
	}
	parts = append(parts, "seeds "+seeds)
	terms := "none"
	if len(e.Terms) > 0 {
		terms = strings.Join(e.Terms, ", ")
	}
	parts = append(parts, "terms "+terms)
	if e.When != nil {
		for _, bound := range []struct {
			name   string
			result *Result
		}{{"since", e.When.Since}, {"until", e.When.Until}} {
			if bound.result == nil {
				continue
			}
			hits := bound.result.Hits
			if len(hits) == 0 {
				parts = append(parts, bound.name+" read from no fact")
				continue
			}
			values := make([]string, len(hits))
			for i, hit := range hits {
				values[i] = fmt.Sprintf("%q", hit.Value)
			}
			read := values[0]
			if len(values) > 1 {
				read = strings.Join(values[:len(values)-1], ", ") + " and " + values[len(values)-1]
			}
			parts = append(parts, fmt.Sprintf("%s read from %s → %s", bound.name, read, renderSpan(containers.Interval{Start: bound.result.Start, End: bound.result.End})))
		}
	}
	return "explain: " + strings.Join(parts, "; ")
}

// renderHitExplain renders how one hit reached its score, on one indented
// line under it: whether it seeded the recall, the recency factor its
// relevance was decayed by, then each contribution its relevance was folded
// from. The decay is written in full, not to three decimals. Facts written
// seconds apart differ in it by a few millionths, and that difference is
// what orders them when their relevance ties.
func renderHitExplain(e HitExplain) string {
	parts := []string{"not a seed"}
	if e.Seed {
		parts[0] = "seed"
	}
	parts = append(parts, fmt.Sprintf("decay %g", e.Decay))
	for _, c := range e.Contributions {
		parts = append(parts, renderContribution(c))
	}
	return "    " + strings.Join(parts, " · ")
}

// renderContribution renders one source's sighting of a hit. A text or vector
// sighting reads as its mass and its position in that source's list, a text
// one with the share of the query's information the fact's own words hold.
// A text sighting whose words hold none of it matched only through its
// window neighbours, and reads as borrowed, so a model does not take the
// fact for one that says what was asked. A
// graph or anchor sighting reads as its mass, the anchor it came through and
// how many facts that anchor files; a graph one also says whether the query
// named the anchor, which waives its fair share, and how many seeds funded
// it. With the query's background rate these are the evidence the relevance
// is folded from, so a model sees which source lifted the fact and through
// which anchor.
func renderContribution(c Contribution) string {
	line := fmt.Sprintf("%s %.3f", c.Source, c.Score)
	var details []string
	if c.Via == "" {
		details = append(details, fmt.Sprintf("rank %d", c.Rank))
		switch {
		case c.Source != "text":
		case c.Held == 0:
			details = append(details, "borrowed")
		default:
			details = append(details, fmt.Sprintf("held %.3f", c.Held))
		}
	} else {
		line += " via " + c.Via
		if c.Named {
			details = append(details, "named")
		}
		details = append(details, renderCount(c.Degree, "fact"))
		if c.Source == "graph" {
			details = append(details, renderCount(c.Count, "seed"))
		}
	}
	return line + " (" + strings.Join(details, ", ") + ")"
}

// renderCatalogue renders a describe's answer: a line naming what was
// described and its size, then the topics and the entities its facts are
// filed under, one per line in the order the catalogue lists them. The first
// line spells the describe as the server read it, so the model can widen or
// narrow it by editing what it reads. Each anchor carries, in front, when its
// facts happened, as a hit carries its dates. A model planning its first
// recall reads which anchors exist, how much each files and how recent it is.
// A kind with no anchor says so, since a missing section reads as a list cut
// off.
func renderCatalogue(c Catalogue) []string {
	statement := fmt.Sprintf("describe recall@%d", c.Graph)
	if c.Scope != "" {
		statement += " " + c.Scope
	}
	if c.Facts == 0 {
		return []string{statement + ": no fact reached."}
	}
	lines := []string{fmt.Sprintf("%s: %s under %s, happened %s", statement, renderCount(c.Facts, "fact"), renderCount(c.Anchors, "anchor"), renderSpan(c.Happened))}
	for _, kind := range []struct {
		name    string
		entries []CatalogueEntry
	}{{"topics", c.Topics}, {"entities", c.Entities}} {
		if len(kind.entries) == 0 {
			lines = append(lines, kind.name+": none")
			continue
		}
		lines = append(lines, kind.name+":")
		for _, entry := range kind.entries {
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", renderSpan(entry.Happened), entry.Value, renderCount(entry.Facts, "fact")))
		}
	}
	return lines
}

// renderCount renders a count with its noun, singular for one, so a line
// reads as a sentence a model would write: 1 fact, 2 facts.
func renderCount(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
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
