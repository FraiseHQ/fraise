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
	Selector() GraphSelectorNode
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
	Set(value T)
}

// literal field
type LiteralFieldNode interface {
	AstNode
	Literal() string
}

// recall command node
type RecallCommandNode struct {
	key      lexer.Token
	selector GraphSelectorNode
	terms    []LiteralFieldNode
	entities []AnchorFieldNode
	topics   []AnchorFieldNode
	top      TopFieldNode
	depth    DepthFieldNode
	since    SinceFieldNode
	until    UntilFieldNode
	vec      RefFieldNode[[]float32]
	pos      lexer.Position
	end      lexer.Position
}

// remember command node
type RememberCommandNode struct {
	key      lexer.Token
	selector GraphSelectorNode
	value    PhraseNode
	anchors  []AnchorFieldNode
	vec      RefFieldNode[[]float32]
	pos      lexer.Position
	end      lexer.Position
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
	value lexer.Token
	pos   lexer.Position
	end   lexer.Position
}

// Topic field
type TopicFieldNode struct {
	key   lexer.Token
	value lexer.Token
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
type VecFieldNode struct {
	key   lexer.Token
	param lexer.Token
	value []float32
	pos   lexer.Position
	end   lexer.Position
}

// remember impl

func (n RememberCommandNode) Selector() GraphSelectorNode {
	return n.selector
}

func (n RememberCommandNode) String() string {
	var s string

	// command
	s += n.key.Literal

	// selector
	s += n.selector.String()

	// value
	s += n.value.Literal()

	// anchors
	for _, e := range n.anchors {
		s += e.String()
	}

	// vec
	s += n.vec.String()

	return s
}

func (n RememberCommandNode) Pos() lexer.Position {
	return n.pos
}

func (n RememberCommandNode) End() lexer.Position {
	return n.end
}

// recall impl

func (n RecallCommandNode) Selector() GraphSelectorNode {
	return n.selector
}

func (n RecallCommandNode) String() string {
	var s string

	// command
	s += n.key.Literal

	// selector
	s += n.selector.String()

	// terms
	for _, t := range n.terms {
		s += t.Literal()
	}

	// entities
	for _, e := range n.entities {
		s += e.String()
	}

	// topics
	for _, t := range n.topics {
		s += t.String()
	}

	// top
	s += n.top.String()

	// depth
	s += n.depth.String()

	// since
	s += n.since.String()

	// until
	s += n.until.String()

	// vec
	s += n.vec.String()

	return s
}

func (n RecallCommandNode) Pos() lexer.Position {
	return n.pos
}

func (n RecallCommandNode) End() lexer.Position {
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

func (n AnchorFieldNode) Clause() ClauseNode {
	return *n.clause
}

func (n AnchorFieldNode) Field() FieldNode[string] {
	return n.field
}

func (n AnchorFieldNode) String() string {
	return fmt.Sprintf("%s%s:%s", n.token.Literal, n.field.Key(), n.field.Value())
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
	var s string

	for _, t := range n.tokens {
		s += t.Literal
	}
	return s
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
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value.Literal)
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
	return n.value.Literal
}

// topic field node impl

func (n TopicFieldNode) String() string {
	return fmt.Sprintf("%s:%s", n.key.Literal, n.value.Literal)
}

func (n TopicFieldNode) Key() string {
	return n.key.Literal
}

func (n TopicFieldNode) Value() string {
	return n.value.Literal
}

func (n TopicFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n TopicFieldNode) End() lexer.Position {
	return n.end
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

func (n SinceFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n SinceFieldNode) End() lexer.Position {
	return n.end
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

func (n UntilFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n UntilFieldNode) End() lexer.Position {
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

func (n VecFieldNode) Param() string {
	return n.param.Literal
}

func (n VecFieldNode) String() string {
	return fmt.Sprintf("%s:$%s", n.key.Literal, n.param.Literal)
}

func (n VecFieldNode) Key() string {
	return n.key.Literal
}

func (n VecFieldNode) Value() []float32 {
	return n.value
}

func (n VecFieldNode) Pos() lexer.Position {
	return n.pos
}

func (n VecFieldNode) End() lexer.Position {
	return n.end
}

func (n VecFieldNode) Set(v []float32) {
	n.value = v
}
