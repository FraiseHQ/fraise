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

// These tests live in the internal `parser` package (not parser_test) because
// the AST node structs expose their data only through unexported fields, which
// an external test package could not populate.
package parser

import (
	"time"

	"testing"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/query/lexer"
)

func tok(t lexer.TokenType, lit string) lexer.Token {
	return lexer.Token{Type: t, Literal: lit}
}

func pos(col int) lexer.Position { return lexer.Position{Column: col} }

// --- GraphSelectorNode ------------------------------------------------------

func TestGraphSelectorNode(t *testing.T) {
	n := GraphSelectorNode{
		key:   tok(lexer.AT, "@"),
		value: 3,
		pos:   pos(1),
		end:   pos(2),
	}

	if got := n.Value(); got != 3 {
		t.Errorf("Value() = %d, want 3", got)
	}
	if got := n.Pos(); got != pos(1) {
		t.Errorf("Pos() = %v, want %v", got, pos(1))
	}
	if got := n.End(); got != pos(2) {
		t.Errorf("End() = %v, want %v", got, pos(2))
	}
}

// --- RememberCommandNode ----------------------------------------------------

func TestRememberCommandNode(t *testing.T) {
	sel := GraphSelectorNode{value: 5}
	n := RememberCommandNode[float32]{
		key:      tok(lexer.REMEMBER, "remember"),
		selector: sel,
		pos:      pos(0),
		end:      pos(42),
	}

	if got := n.selector; got != sel {
		t.Errorf("Selector() = %+v, want %+v", got, sel)
	}
	if got := n.Selector(); got != 5 {
		t.Errorf("Selector().Value() = %d, want 5", got)
	}
	if got := n.Pos(); got != pos(0) {
		t.Errorf("Pos() = %v, want %v", got, pos(0))
	}
	if got := n.End(); got != pos(42) {
		t.Errorf("End() = %v, want %v", got, pos(42))
	}
}

// --- RecallCommandNode ------------------------------------------------------

func TestRecallCommandNode(t *testing.T) {
	sel := GraphSelectorNode{value: 7}
	n := RecallCommandNode[uint64, float32]{
		key:      tok(lexer.RECALL, "recall"),
		selector: sel,
		pos:      pos(3),
		end:      pos(9),
	}

	if got := n.selector; got != sel {
		t.Errorf("Selector() = %+v, want %+v", got, sel)
	}
	if got := n.Pos(); got != pos(3) {
		t.Errorf("Pos() = %v, want %v", got, pos(3))
	}
	if got := n.End(); got != pos(9) {
		t.Errorf("End() = %v, want %v", got, pos(9))
	}
}

// --- TermNode ---------------------------------------------------------------

func TestTermNode(t *testing.T) {
	n := TermNode{
		token: tok(lexer.LITERAL, "hello"),
		value: "hello",
		pos:   pos(4),
		end:   pos(9),
	}

	if got := n.Literal(); got != "hello" {
		t.Errorf("Literal() = %q, want %q", got, "hello")
	}
	if got := n.String(); got != "hello" {
		t.Errorf("String() = %q, want %q", got, "hello")
	}
	if got := n.Pos(); got != pos(4) {
		t.Errorf("Pos() = %v, want %v", got, pos(4))
	}
	if got := n.End(); got != pos(9) {
		t.Errorf("End() = %v, want %v", got, pos(9))
	}
}

// --- Terms ------------------------------------------------------------------

func TestTerms(t *testing.T) {
	terms := Terms{
		{token: tok(lexer.LITERAL, "quick"), value: "quick", pos: pos(0), end: pos(5)},
		{token: tok(lexer.LITERAL, "brown"), value: "brown", pos: pos(6), end: pos(11)},
		{token: tok(lexer.LITERAL, "fox"), value: "fox", pos: pos(12), end: pos(15)},
	}

	if got, want := terms.Literal(), "quickbrownfox"; got != want {
		t.Errorf("Literal() = %q, want %q", got, want)
	}
	if got, want := terms.String(), "quickbrownfox"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	// Pos comes from the first term, End from the last.
	if got := terms.Pos(); got != pos(0) {
		t.Errorf("Pos() = %v, want %v", got, pos(0))
	}
	if got := terms.End(); got != pos(12) {
		t.Errorf("End() = %v, want %v", got, pos(12))
	}
}

func TestTermsEmpty(t *testing.T) {
	var terms Terms

	if got := terms.Literal(); got != "" {
		t.Errorf("Literal() = %q, want empty", got)
	}
	if got := terms.Pos(); got != (lexer.Position{}) {
		t.Errorf("Pos() = %v, want zero value", got)
	}
	if got := terms.End(); got != (lexer.Position{}) {
		t.Errorf("End() = %v, want zero value", got)
	}
}

// --- PhraseNode -------------------------------------------------------------

func TestPhraseNode(t *testing.T) {
	n := PhraseNode{
		value: "the lazy dog",
		pos:   pos(2),
		end:   pos(20),
	}

	if got, want := n.Literal(), "the lazy dog"; got != want {
		t.Errorf("Literal() = %q, want %q", got, want)
	}
	if got, want := n.String(), "the lazy dog"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got := n.Pos(); got != pos(2) {
		t.Errorf("Pos() = %v, want %v", got, pos(2))
	}
	if got := n.End(); got != pos(20) {
		t.Errorf("End() = %v, want %v", got, pos(20))
	}
}

func TestPhraseNodeEmpty(t *testing.T) {
	var n PhraseNode
	if got := n.Literal(); got != "" {
		t.Errorf("Literal() = %q, want empty", got)
	}
}

// --- EntityFieldNode / TopicFieldNode ---------------------------------------

func TestEntityFieldNode(t *testing.T) {
	n := EntityFieldNode{
		key:   tok(lexer.ENTITY, "entity"),
		value: "alice",
		pos:   pos(1),
		end:   pos(8),
	}

	if got := n.Key(); got != "entity" {
		t.Errorf("Key() = %q, want %q", got, "entity")
	}
	if got := n.Value(); got != "alice" {
		t.Errorf("Value() = %q, want %q", got, "alice")
	}
	if got := n.Pos(); got != pos(1) {
		t.Errorf("Pos() = %v, want %v", got, pos(1))
	}
	if got := n.End(); got != pos(8) {
		t.Errorf("End() = %v, want %v", got, pos(8))
	}
}

func TestTopicFieldNode(t *testing.T) {
	n := TopicFieldNode{
		key:   tok(lexer.TOPIC, "topic"),
		value: "weather",
		pos:   pos(3),
		end:   pos(10),
	}

	if got := n.Key(); got != "topic" {
		t.Errorf("Key() = %q, want %q", got, "topic")
	}
	if got := n.Value(); got != "weather" {
		t.Errorf("Value() = %q, want %q", got, "weather")
	}
	if got := n.Pos(); got != pos(3) {
		t.Errorf("Pos() = %v, want %v", got, pos(3))
	}
	if got := n.End(); got != pos(10) {
		t.Errorf("End() = %v, want %v", got, pos(10))
	}
}

// --- AnchorFieldNode --------------------------------------------------------
//
// AnchorFieldNode delegates Key/Value/Pos/End to its wrapped FieldNode.

func TestAnchorFieldNode(t *testing.T) {
	field := EntityFieldNode{
		key:   tok(lexer.ENTITY, "entity"),
		value: "bob",
		pos:   pos(5),
		end:   pos(12),
	}
	clause := ClauseNode{clause: MUST, value: tok(lexer.PLUS, "+")}
	n := AnchorFieldNode{
		clause: &clause,
		token:  tok(lexer.ENTITY, "entity"),
		field:  field,
	}

	if got := n.Clause(); got != &clause {
		t.Errorf("Clause() = %+v, want %+v", got, clause)
	}
	if got := n.Field(); got != FieldNode[string](field) {
		t.Errorf("Field() = %+v, want %+v", got, field)
	}
	if got := n.Key(); got != "entity" {
		t.Errorf("Key() = %q, want %q (delegated)", got, "entity")
	}
	if got := n.Value(); got != "bob" {
		t.Errorf("Value() = %q, want %q (delegated)", got, "bob")
	}
	if got := n.Pos(); got != pos(5) {
		t.Errorf("Pos() = %v, want %v (delegated)", got, pos(5))
	}
	if got := n.End(); got != pos(12) {
		t.Errorf("End() = %v, want %v (delegated)", got, pos(12))
	}
}

// --- SinceFieldNode / UntilFieldNode ----------------------------------------

func TestSinceFieldNode(t *testing.T) {
	tv := containers.AbsoluteTime[uint64]{T: time.Date(2026, time.June, 14, 0, 0, 0, 0, time.UTC)}
	n := SinceFieldNode[uint64]{
		key:   tok(lexer.SINCE, "since"),
		value: tv,
		pos:   pos(1),
		end:   pos(6),
	}

	if got := n.Key(); got != "since" {
		t.Errorf("Key() = %q, want %q", got, "since")
	}
	if got := n.Value(); got != containers.TimeValue[uint64](tv) {
		t.Errorf("Value() = %v, want %v", got, tv)
	}
	if got := n.Pos(); got != pos(1) {
		t.Errorf("Pos() = %v, want %v", got, pos(1))
	}
	if got := n.End(); got != pos(6) {
		t.Errorf("End() = %v, want %v", got, pos(6))
	}
}

func TestUntilFieldNode(t *testing.T) {
	tv := containers.RelativeTime[uint64]{Dur: 24 * time.Hour}
	n := UntilFieldNode[uint64]{
		key:   tok(lexer.UNTIL, "until"),
		value: tv,
		pos:   pos(2),
		end:   pos(7),
	}

	if got := n.Key(); got != "until" {
		t.Errorf("Key() = %q, want %q", got, "until")
	}
	if got := n.Value(); got != containers.TimeValue[uint64](tv) {
		t.Errorf("Value() = %v, want %v", got, tv)
	}
	if got := n.Pos(); got != pos(2) {
		t.Errorf("Pos() = %v, want %v", got, pos(2))
	}
	if got := n.End(); got != pos(7) {
		t.Errorf("End() = %v, want %v", got, pos(7))
	}
}

// --- TopFieldNode / DepthFieldNode ------------------------------------------

func TestTopFieldNode(t *testing.T) {
	n := TopFieldNode{
		key:   tok(lexer.TOP, "top"),
		value: 10,
		pos:   pos(1),
		end:   pos(4),
	}

	if got := n.Key(); got != "top" {
		t.Errorf("Key() = %q, want %q", got, "top")
	}
	if got := n.Value(); got != 10 {
		t.Errorf("Value() = %d, want 10", got)
	}
	if got := n.Pos(); got != pos(1) {
		t.Errorf("Pos() = %v, want %v", got, pos(1))
	}
	if got := n.End(); got != pos(4) {
		t.Errorf("End() = %v, want %v", got, pos(4))
	}
}

func TestDepthFieldNode(t *testing.T) {
	n := DepthFieldNode{
		key:   tok(lexer.DEPTH, "depth"),
		value: 2,
		pos:   pos(1),
		end:   pos(6),
	}

	if got := n.Key(); got != "depth" {
		t.Errorf("Key() = %q, want %q", got, "depth")
	}
	if got := n.Value(); got != 2 {
		t.Errorf("Value() = %d, want 2", got)
	}
	if got := n.Pos(); got != pos(1) {
		t.Errorf("Pos() = %v, want %v", got, pos(1))
	}
	if got := n.End(); got != pos(6) {
		t.Errorf("End() = %v, want %v", got, pos(6))
	}
}

// --- VecFieldNode -----------------------------------------------------------

func TestVecFieldNode(t *testing.T) {
	vec := []float32{0.1, 0.2, 0.3}
	n := VecFieldNode[float32]{
		key:   tok(lexer.DOLLAR, "vec"),
		param: tok(lexer.LITERAL, "q"),
		value: vec,
		pos:   pos(1),
		end:   pos(5),
	}

	if got := n.Key(); got != "vec" {
		t.Errorf("Key() = %q, want %q", got, "vec")
	}
	if got := n.Param(); got != "q" {
		t.Errorf("Param() = %q, want %q", got, "q")
	}
	if got := n.Value(); len(got) != 3 || got[0] != 0.1 || got[2] != 0.3 {
		t.Errorf("Value() = %v, want %v", got, vec)
	}
	if got := n.Pos(); got != pos(1) {
		t.Errorf("Pos() = %v, want %v", got, pos(1))
	}
	if got := n.End(); got != pos(5) {
		t.Errorf("End() = %v, want %v", got, pos(5))
	}
}
