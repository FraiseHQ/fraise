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

	"github.com/google/go-cmp/cmp"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/query/lexer"
)

func tok(t lexer.TokenType, lit string) lexer.Token {
	return lexer.Token{Type: t, Literal: lit}
}

func pos(col int) lexer.Position { return lexer.Position{Column: col} }

func TestGraphSelectorNode(t *testing.T) {
	n := GraphSelectorNode{
		key:   tok(lexer.AT, "@"),
		value: 3,
		pos:   pos(1),
		end:   pos(2),
	}

	if got, want := n.Value(), uint8(3); got != want {
		t.Errorf("Value() = %d, want %d", got, want)
	}
	if got, want := n.Pos(), pos(1); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(2); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestRememberCommandNode(t *testing.T) {
	sel := GraphSelectorNode{value: 5}
	n := RememberCommandNode[float32]{
		key:      tok(lexer.REMEMBER, "remember"),
		selector: sel,
		pos:      pos(0),
		end:      pos(42),
	}

	if got, want := n.selector, sel; got != want {
		t.Errorf("selector = %+v, want %+v", got, want)
	}
	if got, want := n.Selector(), uint8(5); got != want {
		t.Errorf("Selector() = %d, want %d", got, want)
	}
	if got, want := n.Pos(), pos(0); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(42); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestRecallCommandNode(t *testing.T) {
	sel := GraphSelectorNode{value: 7}
	n := RecallCommandNode[uint64, float32]{
		key:      tok(lexer.RECALL, "recall"),
		selector: sel,
		pos:      pos(3),
		end:      pos(9),
	}

	if got, want := n.selector, sel; got != want {
		t.Errorf("selector = %+v, want %+v", got, want)
	}
	if got, want := n.Pos(), pos(3); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(9); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestTermNode(t *testing.T) {
	n := TermNode{
		token: tok(lexer.LITERAL, "hello"),
		value: "hello",
		pos:   pos(4),
		end:   pos(9),
	}

	if got, want := n.Literal(), "hello"; got != want {
		t.Errorf("Literal() = %q, want %q", got, want)
	}
	if got, want := n.String(), "hello"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := n.Pos(), pos(4); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(9); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

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
	// Pos comes from the first term, End from the last term's Pos.
	if got, want := terms.Pos(), pos(0); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := terms.End(), pos(12); got != want {
		t.Errorf("End() = %v, want %v", got, want)
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
	if got, want := n.Pos(), pos(2); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(20); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestPhraseNodeEmpty(t *testing.T) {
	var n PhraseNode
	if got := n.Literal(); got != "" {
		t.Errorf("Literal() = %q, want empty", got)
	}
}

func TestEntityFieldNode(t *testing.T) {
	n := EntityFieldNode{
		key:   tok(lexer.ENTITY, "entity"),
		value: "alice",
		pos:   pos(1),
		end:   pos(8),
	}

	if got, want := n.Key(), "entity"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Value(), "alice"; got != want {
		t.Errorf("Value() = %q, want %q", got, want)
	}
	if got, want := n.Pos(), pos(1); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(8); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestTopicFieldNode(t *testing.T) {
	n := TopicFieldNode{
		key:   tok(lexer.TOPIC, "topic"),
		value: "weather",
		pos:   pos(3),
		end:   pos(10),
	}

	if got, want := n.Key(), "topic"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Value(), "weather"; got != want {
		t.Errorf("Value() = %q, want %q", got, want)
	}
	if got, want := n.Pos(), pos(3); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(10); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

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

	if got, want := n.Clause(), &clause; got != want {
		t.Errorf("Clause() = %+v, want %+v", got, want)
	}
	if got, want := n.Field(), FieldNode[string](field); got != want {
		t.Errorf("Field() = %+v, want %+v", got, want)
	}
	if got, want := n.Key(), "entity"; got != want {
		t.Errorf("Key() = %q, want %q (delegated)", got, want)
	}
	if got, want := n.Value(), "bob"; got != want {
		t.Errorf("Value() = %q, want %q (delegated)", got, want)
	}
	if got, want := n.Pos(), pos(5); got != want {
		t.Errorf("Pos() = %v, want %v (delegated)", got, want)
	}
	if got, want := n.End(), pos(12); got != want {
		t.Errorf("End() = %v, want %v (delegated)", got, want)
	}
}

func TestSinceFieldNode(t *testing.T) {
	tv := containers.AbsoluteTime[uint64]{T: time.Date(2026, time.June, 14, 0, 0, 0, 0, time.UTC)}
	n := SinceFieldNode[uint64]{
		key:   tok(lexer.SINCE, "since"),
		value: tv,
		pos:   pos(1),
		end:   pos(6),
	}

	if got, want := n.Key(), "since"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Value(), containers.TimeValue[uint64](tv); got != want {
		t.Errorf("Value() = %v, want %v", got, want)
	}
	if got, want := n.Pos(), pos(1); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(6); got != want {
		t.Errorf("End() = %v, want %v", got, want)
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

	if got, want := n.Key(), "until"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Value(), containers.TimeValue[uint64](tv); got != want {
		t.Errorf("Value() = %v, want %v", got, want)
	}
	if got, want := n.Pos(), pos(2); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(7); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestTopFieldNode(t *testing.T) {
	n := TopFieldNode{
		key:   tok(lexer.TOP, "top"),
		value: 10,
		pos:   pos(1),
		end:   pos(4),
	}

	if got, want := n.Key(), "top"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Value(), 10; got != want {
		t.Errorf("Value() = %d, want %d", got, want)
	}
	if got, want := n.Pos(), pos(1); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(4); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestDepthFieldNode(t *testing.T) {
	n := DepthFieldNode{
		key:   tok(lexer.DEPTH, "depth"),
		value: 2,
		pos:   pos(1),
		end:   pos(6),
	}

	if got, want := n.Key(), "depth"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Value(), 2; got != want {
		t.Errorf("Value() = %d, want %d", got, want)
	}
	if got, want := n.Pos(), pos(1); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(6); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestVecFieldNode(t *testing.T) {
	vec := []float32{0.1, 0.2, 0.3}
	n := VecFieldNode[float32]{
		key:   tok(lexer.DOLLAR, "vec"),
		param: tok(lexer.LITERAL, "q"),
		value: vec,
		pos:   pos(1),
		end:   pos(5),
	}

	if got, want := n.Key(), "vec"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Param(), "q"; got != want {
		t.Errorf("Param() = %q, want %q", got, want)
	}
	if diff := cmp.Diff(vec, n.Value()); diff != "" {
		t.Errorf("Value() mismatch (-want +got):\n%s", diff)
	}
	if got, want := n.Pos(), pos(1); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(5); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}
