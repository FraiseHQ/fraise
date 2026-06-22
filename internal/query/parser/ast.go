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
	Set(key lexer.Token, value T)
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

type TimeValueFieldNode interface {
	FieldNode[containers.TimeValue]
	TimeValue() time.Time
}

// recall command node
type RecallCommandNode[P float32 | float64] struct {
	key      lexer.Token
	selector GraphSelectorNode
	terms    []LiteralFieldNode
	entities []AnchorFieldNode
	topics   []AnchorFieldNode
	top      TopFieldNode
	depth    DepthFieldNode
	since    SinceFieldNode
	until    UntilFieldNode
	vec      *VecFieldNode[P]
	pos      lexer.Position
	end      lexer.Position
}

func (r RecallCommandNode[P]) Terms() []string {
	var res []string

	for _, v := range r.terms {
		res = append(res, v.Literal())
	}

	return res
}

func (r RecallCommandNode[P]) Entities() []string {
	var res []string

	for _, v := range r.entities {
		res = append(res, v.Value())
	}

	return res
}

func (r RecallCommandNode[P]) Topics() []string {
	var res []string

	for _, v := range r.topics {
		res = append(res, v.Value())
	}

	return res
}

func (r RecallCommandNode[P]) Top() int {
	return r.top.value
}

func (r RecallCommandNode[P]) Depth() int {
	return r.depth.value
}

func (r RecallCommandNode[P]) Since() containers.TimeValue {
	return r.since.Value()
}

func (r RecallCommandNode[P]) Until() containers.TimeValue {
	return r.until.Value()
}

func (r RecallCommandNode[P]) Vector() []P {
	return r.vec.Value()
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
	return []string{}
}

func (r RememberCommandNode[P]) Topics() []string {
	return []string{}
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
	tokens []lexer.Token
	value  string
	pos    lexer.Position
	end    lexer.Position
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
type SinceFieldNode struct {
	key   lexer.Token
	value containers.TimeValue
	pos   lexer.Position
	end   lexer.Position
}

// Until field
type UntilFieldNode struct {
	key   lexer.Token
	value containers.TimeValue
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

	// value
	s = append(s, n.value.String())

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

func (n RecallCommandNode[P]) Selector() uint8 {
	return n.selector.value
}

func (n RecallCommandNode[P]) String() string {
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

func (n RecallCommandNode[P]) Pos() lexer.Position {
	return n.pos
}

func (n RecallCommandNode[P]) End() lexer.Position {
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

func (n TermNode) Literal() string {
	return n.token.Literal
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
		s += t.token.Literal
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

func (n PhraseNode) Literal() string {
	var s []string

	for _, t := range n.tokens {
		s = append(s, t.Literal)
	}
	return "'" + strings.Join(s, " ") + "'"
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

func (n EntityFieldNode) Set(key lexer.Token, value string) {
	n.key = key
	n.value = value
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

func (n TopicFieldNode) Set(key lexer.Token, value string) {
	n.key = key
	n.value = value
}

// since field node impl

func (n SinceFieldNode) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value)
}

func (n SinceFieldNode) Key() string {
	return n.key.Literal
}

func (n SinceFieldNode) Value() containers.TimeValue {
	return n.value
}

func (n SinceFieldNode) TimeValue() time.Time {
	return n.value.Resolve(time.Now())
}

func (n SinceFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n SinceFieldNode) End() lexer.Position {
	return n.end
}

func (n SinceFieldNode) Set(key lexer.Token, value containers.TimeValue) {
	n.key = key
	n.value = value
}

// until field node impl

func (n UntilFieldNode) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value)
}

func (n UntilFieldNode) Key() string {
	return n.key.Literal
}

func (n UntilFieldNode) Value() containers.TimeValue {
	return n.value
}

func (n UntilFieldNode) TimeValue() time.Time {
	return n.value.Resolve(time.Now())
}

func (n UntilFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n UntilFieldNode) End() lexer.Position {
	return n.end
}

func (n UntilFieldNode) Set(key lexer.Token, value containers.TimeValue) {
	n.key = key
	n.value = value
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

func (n TopFieldNode) Set(key lexer.Token, value int) {
	n.key = key
	n.value = value
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

func (n DepthFieldNode) Set(key lexer.Token, value int) {
	n.key = key
	n.value = value
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

func (n VecFieldNode[P]) Set(key lexer.Token, v []P) {
	n.key = key
	n.value = v
}
