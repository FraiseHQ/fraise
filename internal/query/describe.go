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

package query

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/hash"
)

// Describe is a read query whose answer is the shape of what Recall reaches,
// rather than its members: how many facts, when they hold, and the topics and
// entities they are filed under. The set described is every candidate the
// recall would rank, after admission and before its top, so the answer
// pictures what memory holds and not the first few hits. Recall carries no
// top of its own, which keeps every match (see [graph.Graph.Search]). A
// recall that names no seed reaches every fact in its time window, so a
// describe of one describes the whole graph.
//
// [Parse] builds one from a describe command. Under describe the recall's top
// clause caps the anchors listed per kind, and its since and until bound the
// facts counted.
type Describe[K comparable, P float32 | float64] struct {
	// Scope is the recall's body as the parser prints it: terms and anchor
	// values folded to lower case, clauses in their fixed order (see
	// [parser.RecallBodyNode.String]). It travels back with the answer, so a
	// caller reads what was described as the server read it, whatever order
	// the clauses were written in.
	Scope string

	// Top caps how many topics, and how many entities, the answer lists.
	Top int

	Recall Recall[K, P]
}

// Plan implements [Query]. A describe runs as its recall does, so the plan is
// the recall's stream, with the describe as the query [Stream.Commit] answers
// for.
func (d *Describe[K, P]) Plan(config *config.ConfigSet) (*Stream[K, P], error) {
	stream, err := d.Recall.Plan(config)
	if err != nil {
		return nil, err
	}
	stream.Query = d
	return stream, nil
}

// GetGraphID implements [Query]: it returns the graph the describe reads,
// which is the one its recall reads.
func (d Describe[K, P]) GetGraphID() uint8 {
	return d.Recall.GetGraphID()
}

// SetGraphID implements [Query]: it selects the graph the describe reads, by
// selecting it for the recall. Hash folds the graph in through the recall's
// key, so it is set before the query is hashed.
func (d *Describe[K, P]) SetGraphID(id uint8) {
	d.Recall.SetGraphID(id)
}

// Hash keys the query for the plan cache. It must fold in the cap, the scope
// and everything the recall's own key folds in. Two describes that differ only
// in the anchors they list, or in the scope they answer with, must not
// collide, or the cache would answer one with the other's catalogue. The
// material opens with the prefix, which no recall's, remember's or when's
// does. So a describe never shares a key with the recall it runs, and the
// cache never hands it a plan that answers with hits. The scope is user text,
// so it is length-prefixed like every other string the material holds.
func (d Describe[K, P]) Hash(h hash.Hasher[K, string]) K {
	var b strings.Builder
	b.WriteString("p=describe|t=")
	b.WriteString(strconv.Itoa(d.Top))
	b.WriteString("|sc=")
	writeStrings(&b, []string{d.Scope})
	b.WriteString("|r=")
	fmt.Fprint(&b, d.Recall.Hash(h))
	return h.Hash(b.String())
}

// Equal implements [Query]: other is a describe with the same cap and scope
// over an equal recall. The engine runs a cached plan only for an equal query,
// so Equal must never hold for two describes that answer differently.
func (d Describe[K, P]) Equal(other Query[K, P]) bool {
	o, ok := other.(*Describe[K, P])
	return ok && o.Top == d.Top && o.Scope == d.Scope && d.Recall.Equal(&o.Recall)
}

// IsWrite implements [Query]. A describe only reads, so it runs under the
// graph's shared lock, concurrently with other reads.
func (d Describe[K, P]) IsWrite() bool {
	return false
}

// Catalogue is what a describe answers with: the shape of the set of facts its
// recall reaches. Facts counts them. Anchors counts the topics and entities
// they are filed under, before Topics and Entities are capped at the
// describe's top. The times are the hull of what the facts hold on each clock
// (see [Times]), the aggregate a when answers with for its matches, here over
// the whole set.
type Catalogue struct {
	Graph   uint8  `json:"graph"`
	Scope   string `json:"scope"`
	Facts   int    `json:"facts"`
	Anchors int    `json:"anchors"`
	Times

	// Topics and Entities list the anchors the facts are filed under, the
	// most filed first and ties by value. So the anchor that files most of
	// the set leads, and the same graph lists in the same order every time.
	Topics   []CatalogueEntry `json:"topics"`
	Entities []CatalogueEntry `json:"entities"`
}

// CatalogueEntry is one anchor of a [Catalogue]: its value, how many of the
// described facts are filed under it, and when those facts hold. Under a
// describe of one anchor, the other anchors' counts are the facts they share
// with it.
type CatalogueEntry struct {
	Value string `json:"value"`
	Facts int    `json:"facts"`
	Times
}

// Times is when a set of facts holds on each clock: the hull of every interval
// the facts hold on it (see [graph.Fact.Intervals]). Happened runs from the
// earliest start to the latest end of what the facts state took place, and has
// no end when one of them has none. Said and written run from the first fact
// to the last. A fact given no time on a clock reads the next one down, so a
// set written with no times reads the same on all three. Each is left out
// when the set holds no fact.
type Times struct {
	Happened containers.Interval `json:"happened,omitzero"`
	Said     containers.Interval `json:"said,omitzero"`
	Written  containers.Interval `json:"written,omitzero"`
}

// hold widens the times to take in a fact, given as what it holds on each
// clock. An empty set holds no interval yet, and takes the fact's own.
func (t *Times) hold(intervals func(containers.Clock) []containers.Interval) {
	for clock, span := range map[containers.Clock]*containers.Interval{
		containers.Happened: &t.Happened,
		containers.Said:     &t.Said,
		containers.Written:  &t.Written,
	} {
		for _, interval := range intervals(clock) {
			if span.Start.IsZero() {
				*span = interval
				continue
			}
			*span = span.Hull(interval)
		}
	}
}

// catalogue folds the facts the describe's recall reached into its answer:
// how many there are, the hull of what they hold on each clock, and every
// topic and entity they are filed under, with the facts each files among them.
// Anchors counts every anchor before the lists are capped at Top. A fact's
// anchors are its neighbours in the graph, since every edge runs from a fact
// to the topic or entity it is filed under.
func (d *Describe[K, P]) catalogue(g graph.Graph[K, P], facts map[K]graph.Fact[K]) Catalogue {
	c := Catalogue{Graph: d.GetGraphID(), Scope: d.Scope, Facts: len(facts)}
	topics, entities := filings{}, filings{}
	for key, fact := range facts {
		c.hold(fact.Intervals)
		for _, anchor := range g.Neighbours(key) {
			node := g.Get(anchor)
			var filed filings
			switch node.(type) {
			case *graph.Topic[K]:
				filed = topics
			case *graph.NamedEntity[K]:
				filed = entities
			default:
				continue
			}
			filed.hold(node.GetValue(), fact.Intervals)
		}
	}
	c.Anchors = len(topics) + len(entities)
	c.Topics = topics.list(d.Top)
	c.Entities = entities.list(d.Top)
	return c
}

// filings gathers a catalogue's anchors of one kind, by value.
type filings map[string]*CatalogueEntry

// hold files one more fact under the anchor with value, given as what the fact
// holds on each clock.
func (f filings) hold(value string, intervals func(containers.Clock) []containers.Interval) {
	entry, ok := f[value]
	if !ok {
		entry = &CatalogueEntry{Value: value}
		f[value] = entry
	}
	entry.Facts++
	entry.hold(intervals)
}

// list returns the anchors the most filed first, ties by value, and at most
// top of them. A non-positive top lists every one, as it keeps every match in
// a search. The list is empty rather than nil, so an answer with no anchor of
// a kind still carries the key.
func (f filings) list(top int) []CatalogueEntry {
	out := make([]CatalogueEntry, 0, len(f))
	for _, entry := range f {
		out = append(out, *entry)
	}
	slices.SortFunc(out, func(a, b CatalogueEntry) int {
		if a.Facts != b.Facts {
			return b.Facts - a.Facts
		}
		return strings.Compare(a.Value, b.Value)
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}
