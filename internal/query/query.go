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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/internal/query/parser"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// Query is an executable Fraise query: a parsed command bound to the graph it
// targets. Hash keys it in the engine's plan cache, and Equal confirms a hit,
// because a 64-bit key can be shared by two distinct queries. The cache shares
// one query object across every request that is equal, so a Query carries no
// per-request state. Plan builds the [Stream] that runs it for one request.
// IsWrite tells the scheduler which lock [Stream.Commit] needs on the graph
// that GetGraphID selects.
type Query[K comparable, P float32 | float64] interface {
	Plan(config *config.ConfigSet) (*Stream[K, P], error)
	GetGraphID() uint8
	hash.Hashable[K, string]
	Equal(other Query[K, P]) bool
	IsWrite() bool
	SetGraphID(id uint8)
}

// TimeOperand is one bound of a recall's time window. It is a time written
// out, held in Value, or a nested query whose answer is the bound, held in
// Query. An operand with neither leaves its side of the window open. The
// nested query is a query like any other: it hashes, compares, takes its graph
// and plans through the [Query] contract, and its stream runs before the
// recall's search (see [Stream.Commit]).
type TimeOperand[K comparable, P float32 | float64] struct {
	Value containers.TimeValue[K]
	Query Query[K, P]
}

// Hash renders the operand for its recall's hash material, the way
// [containers.Vector.Hash] renders a vector's key. A time written out renders
// as [containers.TimeValue.Hash] does. A nested query renders as "q" and its
// own key, so a bound that asks the graph never shares material with one
// written out. An operand with neither renders as nothing.
func (o TimeOperand[K, P]) Hash(h hash.Hasher[K, string]) string {
	switch {
	case o.Query != nil:
		return "q" + fmt.Sprint(o.Query.Hash(h))
	case o.Value != nil:
		return o.Value.Hash(h)
	default:
		return ""
	}
}

// Equal reports whether other is the same bound: both open, the same time
// written out, or nested queries that are [Query.Equal]. The engine confirms a
// plan-cache hit with it, so it must never hold for two bounds that draw
// different windows.
func (o TimeOperand[K, P]) Equal(other TimeOperand[K, P]) bool {
	if (o.Value == nil) != (other.Value == nil) || (o.Query == nil) != (other.Query == nil) {
		return false
	}
	return (o.Value == nil || o.Value.Equal(other.Value)) &&
		(o.Query == nil || o.Query.Equal(other.Query))
}

// QueryParameters bounds a recall's search. Top caps the number of hits. Depth
// selects the retrieval lane (see [graph.Graph]). Since and Until restrict the
// search to a time window, and an operand left empty is unbounded. [Parse] has
// already checked an explicit Top and Depth against the configured ceilings.
type QueryParameters[K comparable, P float32 | float64] struct {
	Top   int
	Depth int
	Since TimeOperand[K, P]
	Until TimeOperand[K, P]
}

// Equal reports whether other bounds the search the same way: the same Top and
// Depth, and time bounds that are [TimeOperand.Equal].
func (p QueryParameters[K, P]) Equal(other QueryParameters[K, P]) bool {
	return p.Top == other.Top &&
		p.Depth == other.Depth &&
		p.Since.Equal(other.Since) &&
		p.Until.Equal(other.Until)
}

// QueryContext holds where a query runs, not what it asks. GraphID is the graph
// its selector names. [Recall] and [Remember] hold it in an unexported field,
// so it changes only through SetGraphID. A [When] runs where its recall does.
type QueryContext struct {
	GraphID uint8
}

// writeStrings folds user text into a query's hash material, each string as
// its byte length, a colon, then the string itself. A fact, phrase or anchor
// may hold any byte, so no delimiter can bound it. The length does: the
// material decodes back to exactly one list, so two queries share it only when
// they hold the same strings. An empty list writes nothing, and a list of one
// empty string writes "0:", so the two stay apart.
func writeStrings(b *strings.Builder, ss []string) {
	for _, s := range ss {
		b.WriteString(strconv.Itoa(len(s)))
		b.WriteByte(':')
		b.WriteString(s)
	}
}

// QueryResult is the outcome of a committed stream as the server returns it:
// the ranked hits of a recall and their count. A when adds the time it answers
// with, and an explained recall what its ranking did. A remember commits with
// an empty result.
type QueryResult[K comparable, P float32 | float64] struct {
	Count int         `json:"count"`
	Hits  []Hit[K, P] `json:"hits"`

	// Start and End are the time a when answers with (see [When]), an
	// interval that holds Start and excludes End. A recall leaves both zero,
	// and omitzero keeps them off its response. End alone is zero for an
	// answer that has not ended.
	Start time.Time `json:"start,omitzero"`
	End   time.Time `json:"end,omitzero"`

	// Explain is the query's half of an explained recall: what the search
	// measured and decided about the query as a whole. It is set only when
	// the recall is explained, and nil keeps the key off a plain response. So
	// the key's presence is how a client tells an explained envelope from a
	// plain one, and a plain recall pays nothing for it.
	Explain *RecallExplain[K, P] `json:"explain,omitempty"`
}

// NestedResults is the wire form of what a recall's nested queries answered,
// each under the name of the bound that holds it. A bound written out, or
// absent, has no entry.
type NestedResults[K comparable, P float32 | float64] struct {
	Since *QueryResult[K, P] `json:"since,omitempty"`
	Until *QueryResult[K, P] `json:"until,omitempty"`
}

// RecallExplain is what an explained recall reports about its query beside
// the ranking: the rate and the bars the anchor round measured anchors
// against, how many facts each source seeded, the terms the text index read,
// where the time went and what the queries nested in its time bounds
// answered. Each value is the search's own measurement (see graph.Result),
// never a figure rebuilt from the hits. A client reads them to see why a fact
// ranked or why a word matched nothing, so a value that differed from what
// the search used would send it debugging a ranking that never ran.
//
// Seeds and Timings are keyed by source name, the names a contribution's
// source carries, and list every source, a source that did not run at zero.
// So a client reads one shape whatever the recall asked for.
type RecallExplain[K comparable, P float32 | float64] struct {
	// Background is the query's background rate ρ₀: the seed mass the anchor
	// round observed per unit of degree over every anchor it touched. An
	// unnamed anchor of degree d_A owes d_A·ρ₀ as its fair share before any of
	// its mass transmits. Under the excess fold a hit's relevance, its score
	// before recency decay, is S = m + Σ max(0, (M_A − m − d_A·ρ₀)/d_A) over
	// its graph contributions, m being its own text and vector mass, with the
	// d_A·ρ₀ term dropped for a named anchor. It is zero when no round ran,
	// and serialised at zero too, so a zero rate is never read as a missing
	// one.
	Background P `json:"background"`

	// Admission maps a depth to the bar, per unit of anchor degree, that the
	// mass of a touched anchor the query did not name had to exceed to
	// transmit at that depth: 1 is twice ρ₀, the precision bar, and 2 is ρ₀.
	// One round yields both, because what it observes does not depend on
	// depth, so a client reads what the same recall admits one lane over. An
	// anchor of degree d_A transmits at depth d when its mass M_A exceeds
	// d_A·Admission[d], M_A and d_A being a graph contribution's score and
	// degree. The server decides by the exact cross-multiplied test, not by
	// this division, and a named anchor transmits at either depth. Depth 0
	// runs no round and has no bar. The key is omitted when no round ran:
	// when the recall runs at depth 0, names no anchor, seeds nothing or is
	// seeded by its named anchors alone, or under db.search-algorithm none.
	// A recall naming an anchor beside a term or vector runs at depth 1 at
	// least, so a depth:0 written there still reports both bars.
	Admission map[int]P `json:"admission,omitempty"`

	// Seeds counts, per source, the distinct facts that source seeded: for
	// text and vector the facts among the first max(db.seed-size, top) of any
	// of its readings, each counted once, and for anchor the members of the
	// named anchors when they seed alone. It is not the number of matches,
	// since every match is read and only a reading's head seeds. Graph is
	// zero, since the round admits facts rather than seeding them.
	Seeds map[string]int `json:"seeds"`

	// Terms lists the query's distinct terms as the text index read them,
	// after stop-word cleaning and stemming, in first-occurrence order. A term
	// no stored fact holds is listed too, which is how a client sees why a
	// word matched nothing. It is empty, never omitted, when the recall has no
	// terms or every keyword is a stop word.
	Terms []string `json:"terms"`

	// Timings holds, per source, the wall time it took, in nanoseconds: text
	// the text reading, vector the vector reading, anchor the named anchors'
	// seeding and graph the anchor round. So a client tells a slow reading
	// from a slow round.
	Timings map[string]time.Duration `json:"timings"`

	// When holds the result of each query nested in the recall's time bounds,
	// and is omitted for a recall whose bounds hold none. It shows which fact
	// a bound was read from and the time that fact gave, so a client tells a
	// window drawn from the wrong fact from a ranking that missed inside the
	// right one. A nested query explains itself with the recall, so each
	// result carries its own explain blocks.
	When *NestedResults[K, P] `json:"when,omitempty"`
}

// Hit is one fact in a recall's result: the stored node and its final score,
// after recency decay. It marshals flat (see [Hit.MarshalJSON]), because a
// client needs the fact's value, its times and its score, not the graph node.
type Hit[K comparable, P float32 | float64] struct {
	Node  graph.Node[K]
	Score P

	// Explain is the hit's half of an explained recall, set only when the
	// recall is explained. nil keeps the key off a plain hit.
	Explain *HitExplain[P]
}

// HitExplain is what an explained recall reports about one hit: whether it
// seeded the recall, the recency factor its relevance was decayed by, and the
// contributions its relevance was folded from.
type HitExplain[P float32 | float64] struct {
	// Seed reports whether the hit was one of the recall's seeds: among the
	// first max(db.seed-size, top) facts of a text or vector reading, or a
	// member of a named anchor when the anchors seed alone. Only a seed funds
	// an anchor. A hit that is no seed and carries graph contributions alone
	// was admitted by transmission, and one that is no seed with a text or
	// vector contribution matched below the seed cut. It is serialised when
	// false too, so a client never reads a missing key as either.
	Seed bool `json:"seed"`

	// Decay is the recency factor the hit's relevance was scaled by to give
	// its score. It tells a fact that ranks low for its age from one that
	// ranks low for its match. A returned hit's factor is never zero, so the
	// key is always present.
	Decay P `json:"decay"`

	// Contributions is the hit's per-source breakdown. A recall hit always
	// has at least one, so the list is never empty. The entries are already
	// in wire form, with anchors resolved to their values. Resolving them
	// needs the graph, which only Commit holds.
	Contributions []HitContribution[P] `json:"contributions"`
}

// HitContribution is the wire form of one contribution. The source is
// serialised by name, because clients never see the Go constants.
//   - A text or vector entry carries its raw mass and list position. A text
//     entry also carries held, the share of the query's information the
//     fact's own words hold, omitted when they hold none of it.
//   - A graph entry, one per funding anchor, carries the anchor's full observed
//     mass, its value under via, its degree and how many seeds funded it.
//     named is set when the query named the anchor, which owes no fair share.
//   - An anchor entry, one per named anchor an anchor-seeded fact is filed
//     under, carries a unit mass and the anchor's value and degree.
//
// With the query's background rate (see RecallExplain), these are the
// evidence the hit's relevance is folded from, so a client sees which source
// lifted the fact and through which anchor.
type HitContribution[P float32 | float64] struct {
	Source string `json:"source"`
	Score  P      `json:"score"`
	Rank   uint16 `json:"rank"`
	Via    string `json:"via,omitempty"`
	Degree uint32 `json:"degree,omitempty"`
	Count  uint16 `json:"count"`
	Named  bool   `json:"named,omitempty"`
	Held   P      `json:"held,omitempty"`
}

// MarshalJSON flattens the node into the hit, so the response carries the
// value, the fact's times and the score with no nested Node object. timestamp
// is when the fact was written. said and occurred are the times its writer
// gave it, and each is left out when the writer gave none, so a client can
// tell a time that was stated from one the server would fall back to. The
// explain block follows the score only when the hit carries one, on an
// explained recall. So a plain response has no explain key.
func (h Hit[K, P]) MarshalJSON() ([]byte, error) {
	fact, _ := h.Node.(graph.Fact[K])
	return json.Marshal(struct {
		Value     string                `json:"value"`
		Timestamp time.Time             `json:"timestamp"`
		Said      time.Time             `json:"said,omitzero"`
		Occurred  []containers.Interval `json:"occurred,omitempty"`
		Score     P                     `json:"score"`
		Explain   *HitExplain[P]        `json:"explain,omitempty"`
	}{
		Value:     h.Node.GetValue(),
		Timestamp: h.Node.GetTimestamp(),
		Said:      fact.Said,
		Occurred:  fact.Occurred,
		Score:     h.Score,
		Explain:   h.Explain,
	})
}

// checkVector vets the vector the parser bound to placeholder name. The parser
// leaves it nil when no parameter has that name, which is ErrMissingParameter.
// A vector longer than maxDim is ErrLimitExceeded. Both are client errors
// (400). Checking the length here keeps an oversized vector from reaching the
// index.
func checkVector[P float32 | float64](name string, data []P, maxDim int) error {
	if data == nil {
		return fmt.Errorf("%w: $%s", ErrMissingParameter, name)
	}
	if len(data) > maxDim {
		return fmt.Errorf("%w: vector $%s has %d dimensions, max %d", ErrLimitExceeded, name, len(data), maxDim)
	}
	return nil
}

// Parse turns a raw query string into an executable Query. Vector arguments
// travel out of band in params, keyed by the placeholder name in the query:
// 'vec:$v' binds to params["v"]. The parser binds the vector, and Parse checks
// it against the configured limits.
//
// A recall written with the explain prefix parses into a [Recall] with Explain
// set, under the same top, depth and vector limits as the recall it explains.
// So an explained recall can ask for nothing its plain form could not, and its
// hits are the plain form's.
//
// Warnings flag a reading of a valid query that the client may not have meant
// (see parser.Warning). They are returned beside the query, never stored on it,
// because a Query carries no per-request state.
func Parse[K comparable, P float32 | float64](q string, params map[string][]P, c *config.ConfigSet) (Query[K, P], []parser.Warning, error) {
	cmd, warns, err := parser.Parse[K, P](q, params)
	if err != nil {
		logger.Debug("Query parsing failed", "query", q, "error", err)
		return nil, nil, fmt.Errorf("%w: %w", ErrParsingFailed, err)
	}

	qo, err := fromCommand[K, P](cmd, c)
	if err != nil {
		return nil, nil, err
	}
	return qo, warns, nil
}

// fromCommand builds the executable query for a parsed command. A command
// nested in a time bound comes through here as well, by way of [fromRecallBody],
// so a nested query is checked against the same limits as one sent on its own.
// The selector is set last. A recall hands its graph to the queries it holds,
// so they must be in place when it is told its own.
func fromCommand[K comparable, P float32 | float64](cmd parser.CommandNode, c *config.ConfigSet) (Query[K, P], error) {
	switch n := cmd.(type) {
	case *parser.RememberCommandNode[K, P]:
		qo := &Remember[K, P]{
			Value:    n.Value(),
			Entities: n.Entities(),
			Topics:   n.Topics(),
			Said:     n.Said(),
			Occurred: n.Occurred(),
		}
		qo.SetGraphID(n.Selector())

		if name, ok := n.VecParam(); ok {
			if err := checkVector(name, n.Vector(), c.DB.MaxVectorDimension); err != nil {
				logger.Warn("Rejecting vector parameter for remember", "parameter", name, "error", err)
				return nil, err
			}
			qo.Vector = containers.NewVector[K](n.Vector())
		}

		logger.Debug("Parsed remember query", "graph", qo.GetGraphID(), "value", qo.Value)

		return qo, nil

	case *parser.RecallCommandNode[K, P]:
		qo, err := fromRecallBody(n.RecallBodyNode, n.Top(c.DB.DefaultTop), c)
		if err != nil {
			return nil, err
		}
		qo.Explain = n.Explain()
		qo.SetGraphID(n.Selector())

		logger.Debug("Parsed recall query", "graph", qo.GetGraphID(), "keywords", len(qo.Keywords))
		return qo, nil

	case *parser.WhenCommandNode[K, P]:
		// A when answers with the time of its best match unless it asks for
		// more, so its top defaults to 1 and not to the recall's default.
		recall, err := fromRecallBody(n.RecallBodyNode, n.Top(1), c)
		if err != nil {
			return nil, err
		}
		qo := &When[K, P]{Clock: n.Clock(), Recall: *recall}
		qo.SetGraphID(n.Selector())

		logger.Debug("Parsed when query", "graph", qo.GetGraphID(), "clock", qo.Clock.String(), "keywords", len(recall.Keywords))
		return qo, nil

	default:
		logger.Debug("Unsupported query command", "command", cmd.String())
		return nil, ErrParsingFailed
	}
}

// fromRecallBody builds the recall a recall body describes. A recall command
// and a when command share the body, and differ in the top a body without the
// clause runs with, which the caller passes. A time bound that holds a nested
// command gets the query [fromCommand] builds for it.
func fromRecallBody[K comparable, P float32 | float64](n parser.RecallBodyNode[K, P], top int, c *config.ConfigSet) (*Recall[K, P], error) {
	// Enforce the top and depth ranges. An out-of-range value is a client
	// error, rejected rather than clamped. Only an explicit clause is checked.
	// The configured default is the operator's and is trusted, even above the
	// ceiling.
	if n.HasTop() && (top < 1 || top > c.DB.MaxTop) {
		logger.Warn("Rejecting recall over top ceiling", "top", top, "max", c.DB.MaxTop)
		return nil, fmt.Errorf("%w: top:%d out of range (1-%d)", ErrLimitExceeded, top, c.DB.MaxTop)
	}
	depth := n.Depth(c.DB.DefaultDepth)
	if n.HasDepth() && depth > c.DB.MaxDepth {
		logger.Warn("Rejecting recall over depth ceiling", "depth", depth, "max", c.DB.MaxDepth)
		return nil, fmt.Errorf("%w: depth:%d out of range (0-%d)", ErrLimitExceeded, depth, c.DB.MaxDepth)
	}

	qo := &Recall[K, P]{
		Keywords: n.Terms(),
		Entities: n.Entities(),
		Topics:   n.Topics(),
		Parameters: QueryParameters[K, P]{
			Top:   top,
			Depth: depth,
			Since: TimeOperand[K, P]{Value: n.Since()},
			Until: TimeOperand[K, P]{Value: n.Until()},
		},
	}

	if nested := n.SinceQuery(); nested != nil {
		q, err := fromCommand[K, P](nested, c)
		if err != nil {
			return nil, err
		}
		qo.Parameters.Since.Query = q
	}
	if nested := n.UntilQuery(); nested != nil {
		q, err := fromCommand[K, P](nested, c)
		if err != nil {
			return nil, err
		}
		qo.Parameters.Until.Query = q
	}

	if name, ok := n.VecParam(); ok {
		if err := checkVector(name, n.Vector(), c.DB.MaxVectorDimension); err != nil {
			logger.Warn("Rejecting vector parameter for recall", "parameter", name, "error", err)
			return nil, err
		}
		qo.Vector = containers.NewVector[K](n.Vector())
	}

	return qo, nil
}
