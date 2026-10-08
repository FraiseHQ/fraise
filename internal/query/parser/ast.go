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

// MUST, MUST_NOT and LOOSE are the occurrences a [ClauseNode] can carry, each
// commented with the prefix character it stands for.
const (
	MUST     ClauseType = iota // +
	MUST_NOT                   // -
	LOOSE                      // ±
)

// AstNode is the contract every parsed node meets: it prints back as text and
// reports where it sits in the source. A command's String, and each clause's,
// is FQL that parses back to the same node, which is why nodes keep their
// source tokens rather than only the values the query layer reads.
type AstNode interface {
	// String returns the node's text.
	String() string

	// Pos and End return where the node starts and ends in the source, so an
	// error or warning about the node can point at it.
	Pos() lexer.Position
	End() lexer.Position
}

// CommandNode is a parsed command: a recall or a remember.
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

// TimeValueFieldNode is a field whose value is a time bound (since: or
// until:). Value keeps the bound as written, relative or absolute, which is
// what the plan cache hashes; TimeValue resolves it to an instant.
type TimeValueFieldNode[K comparable] interface {
	FieldNode[containers.TimeValue[K]]
	TimeValue() time.Time
}

// RecallCommandNode is a parsed recall.
type RecallCommandNode[K comparable, P float32 | float64] struct {
	key      lexer.Token
	selector GraphSelectorNode
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

// Terms returns the recall's search terms in source order, each folded to
// lower case as the parser read it, or nil when the recall has none.
func (r RecallCommandNode[K, P]) Terms() []string {
	var res []string

	for _, v := range r.terms {
		res = append(res, v.Literal())
	}

	return res
}

// Entities returns the values of the recall's entity: anchors in source order,
// folded to lower case, or nil when it names none.
func (r RecallCommandNode[K, P]) Entities() []string {
	var res []string

	for _, v := range r.entities {
		res = append(res, v.Value())
	}

	return res
}

// Topics returns the values of the recall's topic: anchors in source order,
// folded to lower case, or nil when it names none.
func (r RecallCommandNode[K, P]) Topics() []string {
	var res []string

	for _, v := range r.topics {
		res = append(res, v.Value())
	}

	return res
}

// Top returns the recall's top: value, or v when the recall has no top
// clause. The default is the caller's to supply because it is operator
// configuration, which the parser does not see; see [RecallCommandNode.HasTop].
func (r RecallCommandNode[K, P]) Top(v int) int {
	if !r.HasTop() {
		return v
	}
	return r.top.value
}

// HasTop reports whether the recall carried an explicit top clause, as opposed
// to falling back to the configured default. Presence is the parsed top key,
// not a nonzero value, so query.Parse range-checks an explicit top:0 and
// rejects it instead of answering with the default. Only a client-supplied
// value is range-checked, never the trusted default.
func (r RecallCommandNode[K, P]) HasTop() bool {
	return r.top.key.Type == lexer.TOP
}

// Depth returns the recall's depth: value, or v when the recall has no depth
// clause. As with Top, the default is operator configuration the caller
// supplies; see [RecallCommandNode.HasDepth].
func (r RecallCommandNode[K, P]) Depth(v int) int {
	if !r.HasDepth() {
		return v
	}
	return r.depth.value
}

// HasDepth reports whether the recall carried an explicit depth clause, as
// opposed to falling back to the configured default. Presence is the parsed
// depth key, not a nonzero value, so an explicit depth:0 is honoured as the
// floor lane instead of collapsing to the default.
func (r RecallCommandNode[K, P]) HasDepth() bool {
	return r.depth.key.Type == lexer.DEPTH
}

// Since returns the recall's since: bound as written (relative or absolute,
// unresolved), or nil when the recall has no since clause. It stays
// unresolved so the plan cache keys on the bound, not on the instant it
// resolved to at parse time.
func (r RecallCommandNode[K, P]) Since() containers.TimeValue[K] {
	return r.since.Value()
}

// Until returns the recall's until: bound as written (relative or absolute,
// unresolved), or nil when the recall has no until clause; it stays
// unresolved for the same reason as Since.
func (r RecallCommandNode[K, P]) Until() containers.TimeValue[K] {
	return r.until.Value()
}

// Vector returns the vector bound to the vec: clause, or nil when the recall has
// none. A command without the clause is the common case, so it must not
// dereference the absent field node: a nil vec here would panic every
// vector-less query.
func (r RecallCommandNode[K, P]) Vector() []P {
	if r.vec == nil {
		return nil
	}
	return r.vec.Value()
}

// VecParam reports the name of the vector placeholder (the identifier after
// vec:$) and whether the recall carried one at all.
func (r RecallCommandNode[K, P]) VecParam() (string, bool) {
	if r.vec == nil {
		return "", false
	}
	return r.vec.Param(), true
}

// RememberCommandNode is a parsed remember.
type RememberCommandNode[P float32 | float64] struct {
	key      lexer.Token
	selector GraphSelectorNode
	value    PhraseNode
	anchors  []AnchorFieldNode
	vec      *VecFieldNode[P]
	pos      lexer.Position
	end      lexer.Position
}

// Value returns the fact the remember stores: the quoted phrase as written,
// with its case and spacing kept and doubled quotes decoded.
func (r RememberCommandNode[P]) Value() string {
	return r.value.Literal()
}

// Entities returns the values of the remember's entity: anchors in source
// order, folded to lower case, or nil when it names none. A remember keeps its
// anchors in one list, so this picks the entity fields out of it.
func (r RememberCommandNode[P]) Entities() []string {
	var res []string

	for _, a := range r.anchors {
		if f, ok := a.Field().(EntityFieldNode); ok {
			res = append(res, f.Value())
		}
	}

	return res
}

// Topics returns the values of the remember's topic: anchors in source order,
// folded to lower case, or nil when it names none.
func (r RememberCommandNode[P]) Topics() []string {
	var res []string

	for _, a := range r.anchors {
		if f, ok := a.Field().(TopicFieldNode); ok {
			res = append(res, f.Value())
		}
	}

	return res
}

// Vector returns the vector bound to the vec: clause, or nil when the remember has
// none. A command without the clause is the common case, so it must not
// dereference the absent field node: a nil vec here would panic every
// vector-less query.
func (r RememberCommandNode[P]) Vector() []P {
	if r.vec == nil {
		return nil
	}
	return r.vec.Value()
}

// VecParam reports the name of the vector placeholder (the identifier after
// vec:$) and whether the remember carried one at all. The query layer needs
// the name to say which parameter was missing or malformed.
func (r RememberCommandNode[P]) VecParam() (string, bool) {
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

// ClauseNode is the occurrence marker an anchor clause can carry: its
// [ClauseType] and the prefix token it was written with, which String prints
// in front of the anchor.
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
// String needs: a value that was quoted is folded into value but has to be
// quoted again to parse back.
type EntityFieldNode struct {
	key   lexer.Token
	token lexer.Token
	value string
	pos   lexer.Position
	end   lexer.Position
}

// TopicFieldNode is a topic: field. token is the value as written, which
// String needs: a value that was quoted is folded into value but has to be
// quoted again to parse back.
type TopicFieldNode struct {
	key   lexer.Token
	token lexer.Token
	value string
	pos   lexer.Position
	end   lexer.Position
}

// SinceFieldNode is a since: field. token is the bound as written: a TimeValue
// does not render back to FQL, so String reproduces the source instead.
type SinceFieldNode[K comparable] struct {
	key   lexer.Token
	token lexer.Token
	value containers.TimeValue[K]
	pos   lexer.Position
	end   lexer.Position
}

// UntilFieldNode is an until: field. token is the bound as written: a
// TimeValue does not render back to FQL, so String reproduces the source
// instead.
type UntilFieldNode[K comparable] struct {
	key   lexer.Token
	token lexer.Token
	value containers.TimeValue[K]
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

// quote renders s as an FQL phrase: wrapped in quotes, with each apostrophe
// doubled back into the escape the lexer decoded. Every value that was quoted
// in the source prints through it, so String() is a query that parses back to
// the same node: printed bare, e-mail would not parse and my project would
// read as two words.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// remember impl

// Selector implements [CommandNode]: it returns the graph the remember writes
// to, 0 when the command has no @N selector.
func (n RememberCommandNode[P]) Selector() uint8 {
	return n.selector.value
}

func (n RememberCommandNode[P]) String() string {
	var s []string

	// command + selector
	s = append(s, n.key.Literal+n.selector.String())

	// value, re-quoted as in the source query (PhraseNode.String is unquoted)
	s = append(s, quote(n.value.String()))

	// anchors
	for _, e := range n.anchors {
		s = append(s, e.String())
	}

	// vec
	if n.vec != nil {
		s = append(s, n.vec.String())
	}

	return strings.Join(s, " ")
}

// Pos implements [AstNode].
func (n RememberCommandNode[P]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n RememberCommandNode[P]) End() lexer.Position {
	return n.end
}

// recall impl

// Selector implements [CommandNode]: it returns the graph the recall reads,
// 0 when the command has no @N selector.
func (n RecallCommandNode[K, P]) Selector() uint8 {
	return n.selector.value
}

func (n RecallCommandNode[K, P]) String() string {
	var s []string

	// command + selector
	cmd := n.key.Literal
	if n.selector.key.Type == lexer.AT {
		cmd += n.selector.String()
	}
	s = append(s, cmd)

	// terms
	for _, t := range n.terms {
		s = append(s, t.String())
	}

	// entities
	for _, e := range n.entities {
		s = append(s, e.String())
	}

	// topics
	for _, t := range n.topics {
		s = append(s, t.String())
	}

	// top
	if n.top.key.Type == lexer.TOP {
		s = append(s, n.top.String())
	}

	// depth
	if n.depth.key.Type == lexer.DEPTH {
		s = append(s, n.depth.String())
	}

	// since
	if n.since.key.Type == lexer.SINCE {
		s = append(s, n.since.String())
	}

	// until
	if n.until.key.Type == lexer.UNTIL {
		s = append(s, n.until.String())
	}

	// vec
	if n.vec != nil {
		s = append(s, n.vec.String())
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

// graph selector node impl

func (n GraphSelectorNode) String() string {
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

// Value returns the selected graph number. The parser has already rejected a
// selector outside 0 to 255 rather than let it wrap into another graph; the
// tighter bound on configured graphs is the handler's.
func (n GraphSelectorNode) Value() uint8 {
	return n.value
}

// anchor node impl

// Clause returns the anchor's occurrence marker, or nil when it has none.
func (n AnchorFieldNode) Clause() *ClauseNode {
	return n.clause
}

// Field returns the wrapped field, an [EntityFieldNode] or a [TopicFieldNode];
// its dynamic type is how a remember tells its entities from its topics.
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

// term node impl

// Literal returns the term as the parser interpreted it: folded to lower case.
// The token keeps the source spelling for positions and error text; matching
// and cache keys must see only this folded form, or one search would exist
// under as many plan-cache entries as it has capitalisations.
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

// phrase node impl

// Literal returns the phrase text exactly as written between the quotes, with
// each doubled single quote already decoded to one and no surrounding quotes.
// Interior spacing is preserved verbatim: the phrase is opaque literal text.
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

// entity field node impl

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
// unquoted, so entity:Ada and entity:'ada' name the same anchor.
func (n EntityFieldNode) Value() string {
	return n.value
}

// topic field node impl

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
// unquoted, so topic:Billing and topic:'billing' name the same anchor.
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

// since field node impl

func (n SinceFieldNode[K]) String() string {
	if n.token.Type == lexer.PHRASE {
		return fmt.Sprintf("%s:%s", n.key.Literal, quote(n.token.Literal))
	}
	return fmt.Sprintf("%s:%s", n.key.Literal, n.token.Literal)
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n SinceFieldNode[K]) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the since: bound as written, relative or
// absolute and unresolved, or nil on the zero node.
func (n SinceFieldNode[K]) Value() containers.TimeValue[K] {
	return n.value
}

// TimeValue implements [TimeValueFieldNode]: the bound resolved against the
// current time, so a relative bound gives a different instant on every call.
func (n SinceFieldNode[K]) TimeValue() time.Time {
	return n.value.Resolve(time.Now())
}

// Pos implements [AstNode].
func (n SinceFieldNode[K]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n SinceFieldNode[K]) End() lexer.Position {
	return n.end
}

// until field node impl

func (n UntilFieldNode[K]) String() string {
	if n.token.Type == lexer.PHRASE {
		return fmt.Sprintf("%s:%s", n.key.Literal, quote(n.token.Literal))
	}
	return fmt.Sprintf("%s:%s", n.key.Literal, n.token.Literal)
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n UntilFieldNode[K]) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the until: bound as written, relative or
// absolute and unresolved, or nil on the zero node.
func (n UntilFieldNode[K]) Value() containers.TimeValue[K] {
	return n.value
}

// TimeValue implements [TimeValueFieldNode]: the bound resolved against the
// current time, so a relative bound gives a different instant on every call.
func (n UntilFieldNode[K]) TimeValue() time.Time {
	return n.value.Resolve(time.Now())
}

// Pos implements [AstNode].
func (n UntilFieldNode[K]) Pos() lexer.Position {
	return n.pos
}

// End implements [AstNode].
func (n UntilFieldNode[K]) End() lexer.Position {
	return n.end
}

// top field node impl

func (n TopFieldNode) String() string {
	return fmt.Sprintf("%s:%d", n.key.Literal, n.value)
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n TopFieldNode) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the requested result count. The parser only
// checks it is a whole number that fits an int; the configured ceiling is the
// query layer's.
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

// depth field node impl

func (n DepthFieldNode) String() string {
	return fmt.Sprintf("%s:%d", n.key.Literal, n.value)
}

// Key implements [FieldNode]: the clause keyword as written, case included.
func (n DepthFieldNode) Key() string {
	return n.key.Literal
}

// Value implements [FieldNode]: the requested graph expansion depth. The
// parser only checks it is a whole number that fits an int; the configured
// ceiling is the query layer's.
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

// vec field node impl

// Param implements [RefFieldNode]: the placeholder name after vec:$, which
// String prints back instead of the vector and the query layer names in its
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

// Value implements [FieldNode]: the vector the params map held under Param,
// or nil when it held none; rejecting a missing vector is the query layer's
// call.
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
