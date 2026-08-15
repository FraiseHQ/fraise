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

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/query/lexer"
)

type ClauseType int

const (
	MUST     ClauseType = iota // +
	MUST_NOT                   // -
	LOOSE                      // ±
)

type AstNode interface {
	// Text returns the original text of the element.
	String() string

	Pos() lexer.Position
	End() lexer.Position
}

// Node representing a command (remember, recall)
type CommandNode interface {
	AstNode
	Selector() uint8
}

// Node representing a field (topic, since, until, )
type FieldNode[T any] interface {
	AstNode
	Key() string
	Value() T
}

// Ref field
type RefFieldNode[T any] interface {
	AstNode
	FieldNode[T]
	Param() string
}

// literal field
type LiteralFieldNode interface {
	AstNode
	Literal() string
}

type TimeValueFieldNode[K comparable] interface {
	FieldNode[containers.TimeValue[K]]
	TimeValue() time.Time
}

// recall command node
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

func (r RecallCommandNode[K, P]) Terms() []string {
	var res []string

	for _, v := range r.terms {
		res = append(res, v.Literal())
	}

	return res
}

func (r RecallCommandNode[K, P]) Entities() []string {
	var res []string

	for _, v := range r.entities {
		res = append(res, v.Value())
	}

	return res
}

func (r RecallCommandNode[K, P]) Topics() []string {
	var res []string

	for _, v := range r.topics {
		res = append(res, v.Value())
	}

	return res
}

func (r RecallCommandNode[K, P]) Top(v int) int {
	if r.top.value == 0 {
		return v
	}
	return r.top.value
}

// HasTop reports whether the recall carried an explicit top clause, as opposed
// to falling back to the configured default. Ceiling checks apply only to a
// client-supplied value, never to the trusted default.
func (r RecallCommandNode[K, P]) HasTop() bool {
	return r.top.value != 0
}

func (r RecallCommandNode[K, P]) Depth(v int) int {
	if r.depth.value == 0 {
		return v
	}
	return r.depth.value
}

// HasDepth reports whether the recall carried an explicit depth clause, as
// opposed to falling back to the configured default.
func (r RecallCommandNode[K, P]) HasDepth() bool {
	return r.depth.value != 0
}

func (r RecallCommandNode[K, P]) Since() containers.TimeValue[K] {
	return r.since.Value()
}

func (r RecallCommandNode[K, P]) Until() containers.TimeValue[K] {
	return r.until.Value()
}

func (r RecallCommandNode[K, P]) Vector() []P {
	return r.vec.Value()
}

// VecParam reports the name of the vector placeholder (the identifier after
// `vec:$`) and whether the recall carried one at all. The parser only records
// the placeholder; the real vector is bound later from the request parameters.
func (r RecallCommandNode[K, P]) VecParam() (string, bool) {
	if r.vec == nil {
		return "", false
	}
	return r.vec.Param(), true
}

// remember command node
type RememberCommandNode[P float32 | float64] struct {
	key      lexer.Token
	selector GraphSelectorNode
	value    PhraseNode
	anchors  []AnchorFieldNode
	vec      *VecFieldNode[P]
	pos      lexer.Position
	end      lexer.Position
}

func (r RememberCommandNode[P]) Value() string {
	return r.value.Literal()
}

func (r RememberCommandNode[P]) Entities() []string {
	var res []string

	for _, a := range r.anchors {
		if f, ok := a.Field().(EntityFieldNode); ok {
			res = append(res, f.Value())
		}
	}

	return res
}

func (r RememberCommandNode[P]) Topics() []string {
	var res []string

	for _, a := range r.anchors {
		if f, ok := a.Field().(TopicFieldNode); ok {
			res = append(res, f.Value())
		}
	}

	return res
}

func (r RememberCommandNode[P]) Vector() []P {
	return r.vec.Value()
}

func (r RememberCommandNode[P]) VecParam() (string, bool) {
	if r.vec == nil {
		return "", false
	}
	return r.vec.Param(), true
}

// Selector node is the graph selection statement
type GraphSelectorNode struct {
	key   lexer.Token
	value uint8
	pos   lexer.Position
	end   lexer.Position
}

type ClauseNode struct {
	clause ClauseType
	value  lexer.Token
}

// Search nodes are particular nodes using for graph and
// full text search (entity, topic)
type AnchorFieldNode struct {
	clause *ClauseNode
	token  lexer.Token
	field  FieldNode[string]
}

// a term is a search keyword or phrase. A query supports
// multiple terms or one phrase
type TermNode struct {
	token lexer.Token
	value string
	pos   lexer.Position
	end   lexer.Position
}

// Phrase representation. A phrase is a quoted text search
type PhraseNode struct {
	value string
	pos   lexer.Position
	end   lexer.Position
}

// Entity field
type EntityFieldNode struct {
	key   lexer.Token
	value string
	pos   lexer.Position
	end   lexer.Position
}

// Topic field
type TopicFieldNode struct {
	key   lexer.Token
	value string
	pos   lexer.Position
	end   lexer.Position
}

// Since field
type SinceFieldNode[K comparable] struct {
	key   lexer.Token
	value containers.TimeValue[K]
	pos   lexer.Position
	end   lexer.Position
}

// Until field
type UntilFieldNode[K comparable] struct {
	key   lexer.Token
	value containers.TimeValue[K]
	pos   lexer.Position
	end   lexer.Position
}

// Top field
type TopFieldNode struct {
	key   lexer.Token
	value int
	pos   lexer.Position
	end   lexer.Position
}

// Depth field
type DepthFieldNode struct {
	key   lexer.Token
	value int
	pos   lexer.Position
	end   lexer.Position
}

// Ref field
type VecFieldNode[P float32 | float64] struct {
	key   lexer.Token
	param lexer.Token
	value []P
	pos   lexer.Position
	end   lexer.Position
}

// remember impl

func (n RememberCommandNode[P]) Selector() uint8 {
	return n.selector.value
}

func (n RememberCommandNode[P]) String() string {
	var s []string

	// command + selector
	s = append(s, n.key.Literal+n.selector.String())

	// value, re-quoted as in the source query (PhraseNode.String is unquoted);
	// inner quotes are re-escaped ('') so the reconstruction is valid FQL.
	s = append(s, "'"+strings.ReplaceAll(n.value.String(), "'", "''")+"'")

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

func (n RememberCommandNode[P]) Pos() lexer.Position {
	return n.pos
}

func (n RememberCommandNode[P]) End() lexer.Position {
	return n.end
}

// recall impl

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
		s = append(s, t.Literal())
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

func (n RecallCommandNode[K, P]) Pos() lexer.Position {
	return n.pos
}

func (n RecallCommandNode[K, P]) End() lexer.Position {
	return n.end
}

// graph selector node impl

func (n GraphSelectorNode) String() string {
	return fmt.Sprintf("@%d", n.value)
}

func (n GraphSelectorNode) Pos() lexer.Position {
	return n.pos
}

func (n GraphSelectorNode) End() lexer.Position {
	return n.end
}

func (n GraphSelectorNode) Value() uint8 {
	return n.value
}

// anchor node impl

func (n AnchorFieldNode) Clause() *ClauseNode {
	return n.clause
}

func (n AnchorFieldNode) Field() FieldNode[string] {
	return n.field
}

func (n AnchorFieldNode) String() string {
	var c string
	if n.clause != nil {
		c = n.clause.value.Literal
	}
	return fmt.Sprintf("%s%s%s:%s", c, n.token.Literal, n.field.Key(), n.field.Value())
}

func (n AnchorFieldNode) Pos() lexer.Position {
	return n.field.Pos()
}

func (n AnchorFieldNode) End() lexer.Position {
	return n.field.End()
}

func (n AnchorFieldNode) Key() string {
	return n.field.Key()
}

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

func (n TermNode) Pos() lexer.Position {
	return n.pos
}

func (n TermNode) End() lexer.Position {
	return n.end
}

func (n TermNode) String() string {
	return n.Literal()
}

// makes it easier to manage a list of TermNode
type Terms []TermNode

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

func (n Terms) Pos() lexer.Position {
	if len(n) > 0 {
		return n[0].pos
	} else {
		return lexer.Position{}
	}
}

func (n Terms) End() lexer.Position {
	if len(n) > 0 {
		return n[len(n)-1].pos
	} else {
		return lexer.Position{}
	}
}

// phrase node impl

// Literal returns the phrase text exactly as written between the quotes, with
// the escape (”) already decoded and no surrounding quotes. Interior spacing
// is preserved verbatim — the phrase is opaque literal text.
func (n PhraseNode) Literal() string {
	return n.value
}

func (n PhraseNode) Pos() lexer.Position {
	return n.pos
}

func (n PhraseNode) End() lexer.Position {
	return n.end
}

func (n PhraseNode) String() string {
	return n.Literal()
}

// entity field node impl

func (n EntityFieldNode) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value)
}

func (n EntityFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n EntityFieldNode) End() lexer.Position {
	return n.end
}

func (n EntityFieldNode) Key() string {
	return n.key.Literal
}

func (n EntityFieldNode) Value() string {
	return n.value
}

// topic field node impl

func (n TopicFieldNode) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value)
}

func (n TopicFieldNode) Key() string {
	return n.key.Literal
}

func (n TopicFieldNode) Value() string {
	return n.value
}

func (n TopicFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n TopicFieldNode) End() lexer.Position {
	return n.end
}

// since field node impl

func (n SinceFieldNode[K]) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value)
}

func (n SinceFieldNode[K]) Key() string {
	return n.key.Literal
}

func (n SinceFieldNode[K]) Value() containers.TimeValue[K] {
	return n.value
}

func (n SinceFieldNode[K]) TimeValue() time.Time {
	return n.value.Resolve(time.Now())
}

func (n SinceFieldNode[K]) Pos() lexer.Position {
	return n.pos
}

func (n SinceFieldNode[K]) End() lexer.Position {
	return n.end
}

// until field node impl

func (n UntilFieldNode[K]) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value)
}

func (n UntilFieldNode[K]) Key() string {
	return n.key.Literal
}

func (n UntilFieldNode[K]) Value() containers.TimeValue[K] {
	return n.value
}

func (n UntilFieldNode[K]) TimeValue() time.Time {
	return n.value.Resolve(time.Now())
}

func (n UntilFieldNode[K]) Pos() lexer.Position {
	return n.pos
}

func (n UntilFieldNode[K]) End() lexer.Position {
	return n.end
}

// top field node impl

func (n TopFieldNode) String() string {
	return fmt.Sprintf("%s:%d", n.key.Literal, n.value)
}

func (n TopFieldNode) Key() string {
	return n.key.Literal
}

func (n TopFieldNode) Value() int {
	return n.value
}

func (n TopFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n TopFieldNode) End() lexer.Position {
	return n.end
}

// depth field node impl

func (n DepthFieldNode) String() string {
	return fmt.Sprintf("%s:%d", n.key.Literal, n.value)
}

func (n DepthFieldNode) Key() string {
	return n.key.Literal
}

func (n DepthFieldNode) Value() int {
	return n.value
}

func (n DepthFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n DepthFieldNode) End() lexer.Position {
	return n.end
}

// vec field node impl

func (n VecFieldNode[P]) Param() string {
	return n.param.Literal
}

func (n VecFieldNode[P]) String() string {
	return fmt.Sprintf("%s:$%s", n.key.Literal, n.param.Literal)
}

func (n VecFieldNode[P]) Key() string {
	return n.key.Literal
}

func (n VecFieldNode[P]) Value() []P {
	return n.value
}

func (n VecFieldNode[P]) Pos() lexer.Position {
	return n.pos
}

func (n VecFieldNode[P]) End() lexer.Position {
	return n.end
}
