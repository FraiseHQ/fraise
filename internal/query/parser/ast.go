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

package parser

import (
	"fmt"
	"strings"
	"time"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/query/lexer"
)

// ClauseType is the occurrence an anchor clause can be marked with: required,
// excluded or optional.
type ClauseType int

// MUST, MUST_NOT and LOOSE are the occurrences a [ClauseNode] can carry. The
// comment beside each is the prefix character it stands for.
const (
	MUST     ClauseType = iota // +
	MUST_NOT                   // -
	LOOSE                      // ±
)

// AstNode is the contract every parsed node meets. It prints back as text and
// reports where it sits in the source. A command's String, and each clause's,
// is FQL that parses back to the same node. That is why nodes keep their source
// tokens, not only the values the query layer reads.
type AstNode interface {
	// String returns the node's text.
	String() string

	// Pos and End return where the node starts and ends in the source, so an
	// error or warning about the node can point at it.
	Pos() lexer.Position
	End() lexer.Position
}

// CommandNode is a parsed command: a recall, a remember or a when.
type CommandNode interface {
	AstNode
	Selector() uint8
}

// FieldNode is a key:value clause, such as topic:, since: or top:.
type FieldNode[T any] interface {
	AstNode
	Key() string
	Value() T
}

// RefFieldNode is a field whose value is a parameter reference ($name).
type RefFieldNode[T any] interface {
	AstNode
	FieldNode[T]
	Param() string
}

// LiteralFieldNode is a node with a literal value, such as a recall term.
type LiteralFieldNode interface {
	AstNode
	Literal() string
}

// TimeValueFieldNode is a field whose value is a time: a bound of a recall's
// window (since: or until:), or a time a remember gives its fact (said: or
// occurred:). Value keeps the time as written, relative or absolute, and the
// plan cache hashes that form. TimeValue resolves it to the interval it names.
type TimeValueFieldNode[K comparable] interface {
	FieldNode[containers.TimeValue[K]]
	TimeValue() containers.Interval
}

// RecallBodyNode is what a recall searches by: its terms, its anchors, its
// modifiers and its vector. A recall and a when hold the same body, because a
// when is a recall that answers with a time instead of with its hits.
type RecallBodyNode[K comparable, P float32 | float64] struct {
	terms    []LiteralFieldNode
	entities []AnchorFieldNode
	topics   []AnchorFieldNode
	top      TopFieldNode
	depth    DepthFieldNode
	since    SinceFieldNode[K]
	until    UntilFieldNode[K]
	vec      *VecFieldNode[P]
	pos      lexer.Position
	end      lexer.Position
}

// RecallCommandNode is a parsed recall: the verb, its graph selector, and the
// body it searches by.
type RecallCommandNode[K comparable, P float32 | float64] struct {
	RecallBodyNode[K, P]

	key      lexer.Token
	selector GraphSelectorNode
	pos      lexer.Position
	end      lexer.Position
}

// WhenCommandNode is a parsed when: a recall whose answer is the time its hits
// hold on the clock it names. It stands alone as a command, or in parentheses
// as the value of a recall's time bound.
type WhenCommandNode[K comparable, P float32 | float64] struct {
	RecallBodyNode[K, P]

	key   lexer.Token
	clock ClockNode
	pos   lexer.Position
	end   lexer.Position
}

// Terms returns the recall's search terms in source order, folded to lower
// case. It returns nil when the recall has none.
func (r RecallBodyNode[K, P]) Terms() []string {
	var res []string

	for _, v := range r.terms {
		res = append(res, v.Literal())
	}

	return res
}

// Entities returns the values of the recall's entity: anchors in source order,
// folded to lower case. It returns nil when the recall names none.
func (r RecallBodyNode[K, P]) Entities() []string {
	var res []string

	for _, v := range r.entities {
		res = append(res, v.Value())
	}

	return res
}

// Topics returns the values of the recall's topic: anchors in source order,
// folded to lower case. It returns nil when the recall names none.
func (r RecallBodyNode[K, P]) Topics() []string {
	var res []string

	for _, v := range r.topics {
		res = append(res, v.Value())
	}

	return res
}

// Top returns the recall's top: value, or v when the recall has no top clause.
// The caller supplies the default because it is operator configuration, which
// the parser does not see. See [RecallBodyNode.HasTop].
func (r RecallBodyNode[K, P]) Top(v int) int {
	if !r.HasTop() {
		return v
	}
	return r.top.value
}

// HasTop reports whether the recall has an explicit top clause, rather than
// falling back to the configured default. Presence is the parsed top key, not a
// nonzero value. So query.Parse range-checks an explicit top:0 and rejects it,
// instead of answering with the default. Only a client-supplied value is
// range-checked, never the trusted default.
func (r RecallBodyNode[K, P]) HasTop() bool {
	return r.top.key.Type == lexer.TOP
}

// Depth returns the recall's depth: value, or v when the recall has no depth
// clause. As with Top, the default is operator configuration that the caller
// supplies. See [RecallBodyNode.HasDepth].
//
// A recall that names an anchor beside a term or vector runs at depth 1 at
// least. Its anchors take part only through the excess they transmit, and
// depth 0 transmits nothing, so at depth 0 the anchors would mean nothing.
// Anchors alone seed the search and run at any depth.
func (r RecallBodyNode[K, P]) Depth(v int) int {
	if r.HasDepth() {
		v = r.depth.value
	}
	if v < 1 && r.anchoredSearch() {
		return 1
	}
	return v
}

// anchoredSearch reports whether the recall names an anchor beside something
// to match, a term or a vector. Its anchors transmit excess rather than seed.
func (r RecallBodyNode[K, P]) anchoredSearch() bool {
	return (len(r.topics) > 0 || len(r.entities) > 0) && (len(r.terms) > 0 || r.vec != nil)
}

// hasSeed reports whether the recall names something to start the search from:
// a term, an anchor or a vector. top, depth, since and until scope a search and
// start none.
func (r RecallBodyNode[K, P]) hasSeed() bool {
	return len(r.terms) > 0 || len(r.topics) > 0 || len(r.entities) > 0 || r.vec != nil
}

// HasDepth reports whether the recall has an explicit depth clause, rather than
// falling back to the configured default. Presence is the parsed depth key, not
// a nonzero value. So an explicit depth:0 is honoured as the floor lane,
// instead of collapsing to the default.
func (r RecallBodyNode[K, P]) HasDepth() bool {
	return r.depth.key.Type == lexer.DEPTH
}

// Since returns the recall's since: bound as written, relative or absolute and
// unresolved. It returns nil when the recall has no since clause, and when the
// bound is a query (see [RecallBodyNode.SinceQuery]). The bound stays
// unresolved so the plan cache keys on the bound, not on the instant it
// resolved to at parse time.
func (r RecallBodyNode[K, P]) Since() containers.TimeValue[K] {
	return r.since.Value()
}

// SinceQuery returns the query whose answer is the recall's since: bound, the
// command written in parentheses after it. It returns nil when the bound is
// written out, and when the recall has no since clause.
func (r RecallBodyNode[K, P]) SinceQuery() CommandNode {
	return r.since.Query()
}

// Until returns the recall's until: bound as written, relative or absolute and
// unresolved. It returns nil when the recall has no until clause, and when the
// bound is a query (see [RecallBodyNode.UntilQuery]). It stays unresolved for
// the same reason as Since.
func (r RecallBodyNode[K, P]) Until() containers.TimeValue[K] {
	return r.until.Value()
}

// UntilQuery returns the query whose answer is the recall's until: bound, the
// command written in parentheses after it. It returns nil when the bound is
// written out, and when the recall has no until clause.
func (r RecallBodyNode[K, P]) UntilQuery() CommandNode {
	return r.until.Query()
}

// Vector returns the vector bound to the vec: clause, or nil when the recall
// has none. Most commands have no vec: clause. So Vector must not dereference
// the absent field node, or every vector-less query would panic.
func (r RecallBodyNode[K, P]) Vector() []P {
	if r.vec == nil {
		return nil
	}
	return r.vec.Value()
}

// VecParam reports the name of the vector placeholder (the identifier after
// vec:$) and whether the recall carried one at all.
func (r RecallBodyNode[K, P]) VecParam() (string, bool) {
	if r.vec == nil {
		return "", false
	}
	return r.vec.Param(), true
}

// String prints the body's clauses in one fixed order, whatever order they were
// written in: the terms, the anchors, the modifiers, then the vector. A body
// holding nothing prints nothing.
func (r RecallBodyNode[K, P]) String() string {
	var s []string

	for _, t := range r.terms {
		s = append(s, t.String())
	}

	for _, e := range r.entities {
		s = append(s, e.String())
	}

	for _, t := range r.topics {
		s = append(s, t.String())
	}

	if r.top.key.Type == lexer.TOP {
		s = append(s, r.top.String())
	}

	if r.depth.key.Type == lexer.DEPTH {
		s = append(s, r.depth.String())
	}

	if r.since.key.Type == lexer.SINCE {
		s = append(s, r.since.String())
	}

	if r.until.key.Type == lexer.UNTIL {
		s = append(s, r.until.String())
	}

	if r.vec != nil {
		s = append(s, r.vec.String())
	}

	return strings.Join(s, " ")
}

// Pos implements [AstNode].
func (r RecallBodyNode[K, P]) Pos() lexer.Position {
	return r.pos
}

// End implements [AstNode].
func (r RecallBodyNode[K, P]) End() lexer.Position {
	return r.end
}

// RememberCommandNode is a parsed remember.
type RememberCommandNode[K comparable, P float32 | float64] struct {
	key      lexer.Token
	selector GraphSelectorNode
	value    PhraseNode
	anchors  []AnchorFieldNode
	said     SaidFieldNode[K]
	occurred []OccurredFieldNode[K]
	vec      *VecFieldNode[P]
	pos      lexer.Position
	end      lexer.Position
}

// Value returns the fact the remember stores: the quoted phrase as written. Its
// case and spacing are kept, and doubled quotes are decoded.
func (r RememberCommandNode[K, P]) Value() string {
	return r.value.Literal()
}

// Said returns the remember's said: time as written, relative or absolute and
// unresolved. It returns nil when the remember has no said clause. The time
// stays unresolved because a relative one counts back from the moment the fact
// is written, which the query layer knows and the parser does not.
func (r RememberCommandNode[K, P]) Said() containers.TimeValue[K] {
	return r.said.Value()
}

// Occurred returns the remember's occurred: times in source order, as written
// and unresolved. It returns nil when the remember has none. A fact can hold
// several, one for each time what it states took place.
func (r RememberCommandNode[K, P]) Occurred() []containers.TimeValue[K] {
	var res []containers.TimeValue[K]

	for _, o := range r.occurred {
		res = append(res, o.Value())
	}

	return res
}

// Entities returns the values of the remember's entity: anchors in source
// order, folded to lower case. It returns nil when the remember names none. A
// remember keeps its anchors in one list, so this picks the entity fields out
// of it.
func (r RememberCommandNode[K, P]) Entities() []string {
	var res []string

	for _, a := range r.anchors {
		if f, ok := a.Field().(EntityFieldNode); ok {
			res = append(res, f.Value())
		}
	}

	return res
}

// Topics returns the values of the remember's topic: anchors in source order,
// folded to lower case. It returns nil when the remember names none.
func (r RememberCommandNode[K, P]) Topics() []string {
	var res []string

	for _, a := range r.anchors {
		if f, ok := a.Field().(TopicFieldNode); ok {
			res = append(res, f.Value())
		}
	}

	return res
}

// Vector returns the vector bound to the vec: clause, or nil when the remember
// has none. Most commands have no vec: clause. So Vector must not dereference
// the absent field node, or every vector-less query would panic.
func (r RememberCommandNode[K, P]) Vector() []P {
	if r.vec == nil {
		return nil
	}
	return r.vec.Value()
}

// VecParam reports the name of the vector placeholder (the identifier after
// vec:$) and whether the remember carried one at all. The query layer needs
// the name to say which parameter was missing or too long.
func (r RememberCommandNode[K, P]) VecParam() (string, bool) {
	if r.vec == nil {
		return "", false
	}
	return r.vec.Param(), true
}

// GraphSelectorNode is a command's graph selector (@N).
type GraphSelectorNode struct {
	key   lexer.Token
	value uint8
	pos   lexer.Position
	end   lexer.Position
}

// ClockNode is the clock a when names (:said, :happened or :written). key is
// the ':' it was written after, which tells a named clock from the one a when
// reads when it names none.
type ClockNode struct {
	key   lexer.Token
	value containers.Clock
	pos   lexer.Position
	end   lexer.Position
}

// SubqueryNode is a query written in parentheses where a value stands: the
// when whose answer bounds a recall. It holds a whole command, the one the
// top-level production parsed between the parentheses.
type SubqueryNode struct {
	lparen lexer.Token
	query  CommandNode
	rparen lexer.Token
	pos    lexer.Position
	end    lexer.Position
}

// TimeValueNode is the value of a time field, in one of the forms the grammar's
// time_value takes: a duration, a quoted date, a range of quoted dates, or a
// query in parentheses whose answer is the time. token is the time as written,
// or a range's first end, with dots and to holding the rest of the range. A
// TimeValue does not render back to FQL, so String prints the tokens instead.
type TimeValueNode[K comparable] struct {
	token    lexer.Token
	dots     lexer.Token
	to       lexer.Token
	value    containers.TimeValue[K]
	subquery SubqueryNode
	pos      lexer.Position
	end      lexer.Position
}

// ClauseNode is the occurrence marker an anchor clause can carry. It holds the
// [ClauseType] and the prefix token it was written with. String prints that
// token in front of the anchor.
type ClauseNode struct {
	clause ClauseType
	value  lexer.Token
}

// AnchorFieldNode is an anchor clause (topic: or entity:) wrapping its field.
type AnchorFieldNode struct {
	clause *ClauseNode
	token  lexer.Token
	field  FieldNode[string]
}

// TermNode is one recall term: a bare word or a quoted phrase.
type TermNode struct {
	token lexer.Token
	value string
	pos   lexer.Position
	end   lexer.Position
}

// PhraseNode is a quoted phrase: the fact a remember stores.
type PhraseNode struct {
	value string
	pos   lexer.Position
	end   lexer.Position
}

// EntityFieldNode is an entity: field. token is the value as written, which
// String needs. A quoted value is stored folded and unquoted, so String must
// quote it again for it to parse back.
type EntityFieldNode struct {
	key   lexer.Token
	token lexer.Token
	value string
	pos   lexer.Position
	end   lexer.Position
}

// TopicFieldNode is a topic: field. token is the value as written, which String
// needs. A quoted value is stored folded and unquoted, so String must quote it
// again for it to parse back.
type TopicFieldNode struct {
	key   lexer.Token
	token lexer.Token
	value string
	pos   lexer.Position
	end   lexer.Position
}

// SinceFieldNode is a since: field, the start of a recall's time window. Its
// value is a time written out, or a query whose answer is the bound.
type SinceFieldNode[K comparable] struct {
	key   lexer.Token
	value TimeValueNode[K]
	pos   lexer.Position
	end   lexer.Position
}

// UntilFieldNode is an until: field, the end of a recall's time window. Its
// value is a time written out, or a query whose answer is the bound.
type UntilFieldNode[K comparable] struct {
	key   lexer.Token
	value TimeValueNode[K]
	pos   lexer.Position
	end   lexer.Position
}

// SaidFieldNode is a said: field, the time a remembered fact was stated. Its
// value is always a time written out.
type SaidFieldNode[K comparable] struct {
	key   lexer.Token
	value TimeValueNode[K]
	pos   lexer.Position
	end   lexer.Position
}

// OccurredFieldNode is an occurred: field, one time what a remembered fact
// states took place. Its value is always a time written out.
type OccurredFieldNode[K comparable] struct {
	key   lexer.Token
	value TimeValueNode[K]
	pos   lexer.Position
	end   lexer.Position
}

// TopFieldNode is a top: field.
type TopFieldNode struct {
	key   lexer.Token
	value int
	pos   lexer.Position
	end   lexer.Position
}

// DepthFieldNode is a depth: field.
type DepthFieldNode struct {
	key   lexer.Token
	value int
	pos   lexer.Position
	end   lexer.Position
}

// VecFieldNode is a vec:$name field.
type VecFieldNode[P float32 | float64] struct {
	key   lexer.Token
	param lexer.Token
	value []P
	pos   lexer.Position
	end   lexer.Position
}

// quote renders s as an FQL phrase. It wraps s in quotes and doubles each
// apostrophe, restoring the escape the lexer decoded. Every value that was
// quoted in the source prints through it, so String() is a query that parses
// back to the same node. Printed bare, e-mail would not parse, and my project
// would read as two words.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Selector implements [CommandNode]. It returns the graph the remember writes
// to, or 0 when the command has no @N selector.
func (n RememberCommandNode[K, P]) Selector() uint8 {
	return n.selector.value
}

func (n RememberCommandNode[K, P]) String() string {
	var s []string

	s = append(s, n.key.Literal+n.selector.String())

	// The fact, re-quoted as in the source. PhraseNode.String is unquoted.
	s = append(s, quote(n.value.String()))

	for _, e := range n.anchors {
		s = append(s, e.String())
	}

	if n.said.key.Type == lexer.SAID {
		s = append(s, n.said.String())
	}

	for _, o := range n.occurred {
		s = append(s, o.String())
	}

	if n.vec != nil {
		s = append(s, n.vec.String())
	}

	return strings.Join(s, " ")
}

// Pos implements [AstNode].
func (n RememberCommandNode[K, P]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n RememberCommandNode[K, P]) End() lexer.Position {
	return n.end
}

// Selector implements [CommandNode]. It returns the graph the recall reads, or
// 0 when the command has no @N selector.
func (n RecallCommandNode[K, P]) Selector() uint8 {
	return n.selector.value
}

func (n RecallCommandNode[K, P]) String() string {
	s := []string{n.key.Literal + n.selector.String()}

	if body := n.RecallBodyNode.String(); body != "" {
		s = append(s, body)
	}

	return strings.Join(s, " ")
}

// Pos implements [AstNode].
func (n RecallCommandNode[K, P]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n RecallCommandNode[K, P]) End() lexer.Position {
	return n.end
}

// Selector implements [CommandNode]. A when takes no @N selector, so it is
// always 0: on its own a when reads the default graph, and inside a time bound
// it reads the graph of the recall that holds it.
func (n WhenCommandNode[K, P]) Selector() uint8 {
	return 0
}

// Clock returns the clock the when answers on. A when that names none answers
// on happened.
func (n WhenCommandNode[K, P]) Clock() containers.Clock {
	return n.clock.value
}

func (n WhenCommandNode[K, P]) String() string {
	s := []string{n.key.Literal + n.clock.String()}

	if body := n.RecallBodyNode.String(); body != "" {
		s = append(s, body)
	}

	return strings.Join(s, " ")
}

// Pos implements [AstNode].
func (n WhenCommandNode[K, P]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n WhenCommandNode[K, P]) End() lexer.Position {
	return n.end
}

// String prints the selector as written, or nothing when the command has none.
// So a command's String parses back to the query it came from.
func (n GraphSelectorNode) String() string {
	if n.key.Type != lexer.AT {
		return ""
	}
	return fmt.Sprintf("@%d", n.value)
}

// Pos implements [AstNode].
func (n GraphSelectorNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n GraphSelectorNode) End() lexer.Position {
	return n.end
}

// Value returns the selected graph number.
func (n GraphSelectorNode) Value() uint8 {
	return n.value
}

// String prints the clock as written, or nothing when the when names none. So
// a when's String parses back to the query it came from.
func (n ClockNode) String() string {
	if n.key.Type != lexer.COLON {
		return ""
	}
	return n.key.Literal + n.value.String()
}

// Pos implements [AstNode].
func (n ClockNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n ClockNode) End() lexer.Position {
	return n.end
}

// Value returns the clock the node names, happened when it names none.
func (n ClockNode) Value() containers.Clock {
	return n.value
}

// String prints the query between its parentheses, or nothing when the node
// holds none.
func (n SubqueryNode) String() string {
	if n.query == nil {
		return ""
	}
	return n.lparen.Literal + n.query.String() + n.rparen.Literal
}

// Pos implements [AstNode].
func (n SubqueryNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n SubqueryNode) End() lexer.Position {
	return n.end
}

// Query returns the command between the parentheses, or nil when the node
// holds none.
func (n SubqueryNode) Query() CommandNode {
	return n.query
}

// String prints the value in the form it was written: the query in its
// parentheses, or the time with its quotes back on, and for a range the dots
// and the second end after it.
func (n TimeValueNode[K]) String() string {
	if n.subquery.query != nil {
		return n.subquery.String()
	}

	s := n.token.Literal
	if n.token.Type == lexer.PHRASE {
		s = quote(s)
	}

	if n.dots.Type != lexer.RANGE {
		return s
	}

	s += n.dots.Literal
	if n.to.Type == lexer.PHRASE {
		s += quote(n.to.Literal)
	}

	return s
}

// Pos implements [AstNode].
func (n TimeValueNode[K]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n TimeValueNode[K]) End() lexer.Position {
	return n.end
}

// Value returns the time as written, relative or absolute and unresolved. It
// returns nil when the value is a query, whose answer is not known until it
// runs.
func (n TimeValueNode[K]) Value() containers.TimeValue[K] {
	return n.value
}

// Query returns the command whose answer is the time, or nil when the time is
// written out.
func (n TimeValueNode[K]) Query() CommandNode {
	return n.subquery.Query()
}

// Clause returns the anchor's occurrence marker, or nil when it has none.
func (n AnchorFieldNode) Clause() *ClauseNode {
	return n.clause
}

// Field returns the wrapped field, an [EntityFieldNode] or a [TopicFieldNode].
// A remember tells its entities from its topics by the field's dynamic type.
func (n AnchorFieldNode) Field() FieldNode[string] {
	return n.field
}

func (n AnchorFieldNode) String() string {
	var c string
	if n.clause != nil {
		c = n.clause.value.Literal
	}
	return fmt.Sprintf("%s%s%s", c, n.token.Literal, n.field.String())
}

// Pos implements [AstNode] by delegating to the wrapped field.
func (n AnchorFieldNode) Pos() lexer.Position {
	return n.field.Pos()
}

// End implements [AstNode] by delegating to the wrapped field.
func (n AnchorFieldNode) End() lexer.Position {
	return n.field.End()
}

// Key implements [FieldNode]: the wrapped field's keyword as written.
func (n AnchorFieldNode) Key() string {
	return n.field.Key()
}

// Value implements [FieldNode]: the wrapped field's value, folded to lower case.
func (n AnchorFieldNode) Value() string {
	return n.field.Value()
}

// Literal returns the term as the parser read it, folded to lower case. The
// token keeps the source spelling for positions and error text. Matching and
// cache keys must see only the folded form. Otherwise one search would get a
// plan-cache entry for each of its capitalisations.
func (n TermNode) Literal() string {
	return n.value
}

// Pos implements [AstNode].
func (n TermNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n TermNode) End() lexer.Position {
	return n.end
}

func (n TermNode) String() string {
	if n.token.Type == lexer.PHRASE {
		return quote(n.Literal())
	}
	return n.Literal()
}

// Terms is a list of TermNode.
type Terms []TermNode

// Literal implements [LiteralFieldNode]: the folded literals of every term,
// concatenated with no separator.
func (n Terms) Literal() string {
	var s string
	for _, t := range n {
		s += t.Literal()
	}
	return s
}

func (n Terms) String() string {
	return n.Literal()
}

// Pos implements [AstNode]: the first term's Pos, or the zero Position for an
// empty list.
func (n Terms) Pos() lexer.Position {
	if len(n) > 0 {
		return n[0].pos
	} else {
		return lexer.Position{}
	}
}

// End implements [AstNode]: the last term's end, or the zero Position for an
// empty list.
func (n Terms) End() lexer.Position {
	if len(n) > 0 {
		return n[len(n)-1].pos
	} else {
		return lexer.Position{}
	}
}

// Literal returns the phrase text as written between the quotes, without the
// quotes. Each doubled single quote is already decoded to one. Interior spacing
// is kept as is, because the phrase is opaque literal text.
func (n PhraseNode) Literal() string {
	return n.value
}

// Pos implements [AstNode].
func (n PhraseNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n PhraseNode) End() lexer.Position {
	return n.end
}

func (n PhraseNode) String() string {
	return n.Literal()
}

func (n EntityFieldNode) String() string {
	if n.token.Type == lexer.PHRASE {
		return fmt.Sprintf("%s:%s", n.key.Literal, quote(n.value))
	}
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value)
}

// Pos implements [AstNode].
func (n EntityFieldNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n EntityFieldNode) End() lexer.Position {
	return n.end
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n EntityFieldNode) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the entity name, folded to lower case and
// unquoted. So entity:Ada and entity:'ada' name the same anchor.
func (n EntityFieldNode) Value() string {
	return n.value
}

func (n TopicFieldNode) String() string {
	if n.token.Type == lexer.PHRASE {
		return fmt.Sprintf("%s:%s", n.key.Literal, quote(n.value))
	}
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value)
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n TopicFieldNode) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the topic name, folded to lower case and
// unquoted. So topic:Billing and topic:'billing' name the same anchor.
func (n TopicFieldNode) Value() string {
	return n.value
}

// Pos implements [AstNode].
func (n TopicFieldNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n TopicFieldNode) End() lexer.Position {
	return n.end
}

func (n SinceFieldNode[K]) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value.String())
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n SinceFieldNode[K]) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the since: bound as written, relative or
// absolute and unresolved. It is nil when the bound is a query.
func (n SinceFieldNode[K]) Value() containers.TimeValue[K] {
	return n.value.Value()
}

// Query returns the command whose answer is the since: bound, or nil when the
// bound is written out.
func (n SinceFieldNode[K]) Query() CommandNode {
	return n.value.Query()
}

// TimeValue implements [TimeValueFieldNode]: the bound resolved against the
// current time. A relative bound gives a different interval on every call. A
// field holding no written time, an absent clause or a query, resolves to the
// zero Interval, which is all of time.
func (n SinceFieldNode[K]) TimeValue() containers.Interval {
	v := n.value.Value()
	if v == nil {
		return containers.Interval{}
	}
	return v.Resolve(time.Now())
}

// Pos implements [AstNode].
func (n SinceFieldNode[K]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n SinceFieldNode[K]) End() lexer.Position {
	return n.end
}

func (n UntilFieldNode[K]) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value.String())
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n UntilFieldNode[K]) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the until: bound as written, relative or
// absolute and unresolved. It is nil when the bound is a query.
func (n UntilFieldNode[K]) Value() containers.TimeValue[K] {
	return n.value.Value()
}

// Query returns the command whose answer is the until: bound, or nil when the
// bound is written out.
func (n UntilFieldNode[K]) Query() CommandNode {
	return n.value.Query()
}

// TimeValue implements [TimeValueFieldNode]: the bound resolved against the
// current time. A relative bound gives a different interval on every call. A
// field holding no written time, an absent clause or a query, resolves to the
// zero Interval, which is all of time.
func (n UntilFieldNode[K]) TimeValue() containers.Interval {
	v := n.value.Value()
	if v == nil {
		return containers.Interval{}
	}
	return v.Resolve(time.Now())
}

// Pos implements [AstNode].
func (n UntilFieldNode[K]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n UntilFieldNode[K]) End() lexer.Position {
	return n.end
}

func (n SaidFieldNode[K]) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value.String())
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n SaidFieldNode[K]) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the said: time as written, relative or
// absolute and unresolved.
func (n SaidFieldNode[K]) Value() containers.TimeValue[K] {
	return n.value.Value()
}

// TimeValue implements [TimeValueFieldNode]: the time resolved against the
// current time. A relative time gives a different interval on every call. An
// absent clause resolves to the zero Interval, which is all of time.
func (n SaidFieldNode[K]) TimeValue() containers.Interval {
	v := n.value.Value()
	if v == nil {
		return containers.Interval{}
	}
	return v.Resolve(time.Now())
}

// Pos implements [AstNode].
func (n SaidFieldNode[K]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n SaidFieldNode[K]) End() lexer.Position {
	return n.end
}

func (n OccurredFieldNode[K]) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value.String())
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n OccurredFieldNode[K]) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the occurred: time as written, relative or
// absolute and unresolved.
func (n OccurredFieldNode[K]) Value() containers.TimeValue[K] {
	return n.value.Value()
}

// TimeValue implements [TimeValueFieldNode]: the time resolved against the
// current time. A relative time gives a different interval on every call. An
// absent clause resolves to the zero Interval, which is all of time.
func (n OccurredFieldNode[K]) TimeValue() containers.Interval {
	v := n.value.Value()
	if v == nil {
		return containers.Interval{}
	}
	return v.Resolve(time.Now())
}

// Pos implements [AstNode].
func (n OccurredFieldNode[K]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n OccurredFieldNode[K]) End() lexer.Position {
	return n.end
}

func (n TopFieldNode) String() string {
	return fmt.Sprintf("%s:%d", n.key.Literal, n.value)
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n TopFieldNode) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the requested result count.
func (n TopFieldNode) Value() int {
	return n.value
}

// Pos implements [AstNode].
func (n TopFieldNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n TopFieldNode) End() lexer.Position {
	return n.end
}

func (n DepthFieldNode) String() string {
	return fmt.Sprintf("%s:%d", n.key.Literal, n.value)
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n DepthFieldNode) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the requested graph expansion depth.
func (n DepthFieldNode) Value() int {
	return n.value
}

// Pos implements [AstNode].
func (n DepthFieldNode) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n DepthFieldNode) End() lexer.Position {
	return n.end
}

// Param implements [RefFieldNode]: the placeholder name after vec:$. String
// prints it back instead of the vector, and the query layer names it in its
// errors.
func (n VecFieldNode[P]) Param() string {
	return n.param.Literal
}

func (n VecFieldNode[P]) String() string {
	return fmt.Sprintf("%s:$%s", n.key.Literal, n.param.Literal)
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n VecFieldNode[P]) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the vector the params map holds under Param.
func (n VecFieldNode[P]) Value() []P {
	return n.value
}

// Pos implements [AstNode].
func (n VecFieldNode[P]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n VecFieldNode[P]) End() lexer.Position {
	return n.end
}
