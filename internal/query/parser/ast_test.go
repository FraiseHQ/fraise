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

// These tests live in package parser, not parser_test. The AST node structs
// keep their data in unexported fields, which an external test package could
// not set.
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
	n := RememberCommandNode[uint64, float32]{
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

// A when has no selector to hold, so Selector is 0 whatever graph the recall
// around it reads. The clock it names is printed back glued to the verb.
func TestWhenCommandNode(t *testing.T) {
	n := WhenCommandNode[uint64, float32]{
		RecallBodyNode: RecallBodyNode[uint64, float32]{
			terms: []LiteralFieldNode{TermNode{token: tok(lexer.LITERAL, "race"), value: "race"}},
		},
		key:   tok(lexer.WHEN, "when"),
		clock: ClockNode{key: tok(lexer.COLON, ":"), value: containers.Said},
		pos:   pos(1),
		end:   pos(14),
	}

	if got, want := n.Clock(), containers.Said; got != want {
		t.Errorf("Clock() = %v, want %v", got, want)
	}
	if got, want := n.Selector(), uint8(0); got != want {
		t.Errorf("Selector() = %d, want %d", got, want)
	}
	if got, want := n.String(), "when:said race"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := n.Pos(), pos(1); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(14); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

// A when that names no clock answers on happened, and prints no clock: the
// zero ClockNode is the absent one.
func TestWhenCommandNodeWithoutAClock(t *testing.T) {
	n := WhenCommandNode[uint64, float32]{key: tok(lexer.WHEN, "when")}

	if got, want := n.Clock(), containers.Happened; got != want {
		t.Errorf("Clock() = %v, want %v", got, want)
	}
	if got, want := n.String(), "when"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestClockNode(t *testing.T) {
	n := ClockNode{
		key:   tok(lexer.COLON, ":"),
		value: containers.Written,
		pos:   pos(5),
		end:   pos(12),
	}

	if got, want := n.Value(), containers.Written; got != want {
		t.Errorf("Value() = %v, want %v", got, want)
	}
	if got, want := n.String(), ":written"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := n.Pos(), pos(5); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(12); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestSubqueryNode(t *testing.T) {
	when := &WhenCommandNode[uint64, float32]{
		RecallBodyNode: RecallBodyNode[uint64, float32]{
			terms: []LiteralFieldNode{TermNode{token: tok(lexer.PHRASE, "the race"), value: "the race"}},
		},
		key: tok(lexer.WHEN, "when"),
	}
	n := SubqueryNode{
		lparen: tok(lexer.LPAREN, "("),
		query:  when,
		rparen: tok(lexer.RPAREN, ")"),
		pos:    pos(16),
		end:    pos(32),
	}

	if got, want := n.Query(), CommandNode(when); got != want {
		t.Errorf("Query() = %v, want %v", got, want)
	}
	if got, want := n.String(), "(when 'the race')"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := n.Pos(), pos(16); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(32); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

// The zero SubqueryNode is the one a written-out time carries. It holds no
// query and prints nothing, parentheses included.
func TestSubqueryNodeEmpty(t *testing.T) {
	var n SubqueryNode

	if got := n.Query(); got != nil {
		t.Errorf("Query() = %v, want nil", got)
	}
	if got := n.String(); got != "" {
		t.Errorf("String() = %q, want empty", got)
	}
}

// TestTimeValueNodeString pins that a time value prints in the form it was
// written, in each of the four forms the grammar gives it. The text is what a
// command's String reparses. So a date keeps its quotes, a range its dots, and
// a query its parentheses.
func TestTimeValueNodeString(t *testing.T) {
	when := &WhenCommandNode[uint64, float32]{
		RecallBodyNode: RecallBodyNode[uint64, float32]{
			terms: []LiteralFieldNode{TermNode{token: tok(lexer.LITERAL, "race"), value: "race"}},
		},
		key:   tok(lexer.WHEN, "when"),
		clock: ClockNode{key: tok(lexer.COLON, ":"), value: containers.Said},
	}

	cases := map[string]struct {
		node TimeValueNode[uint64]
		want string
	}{
		"a duration": {
			node: TimeValueNode[uint64]{token: tok(lexer.LITERAL, "7d")},
			want: "7d",
		},
		"a quoted date": {
			node: TimeValueNode[uint64]{token: tok(lexer.PHRASE, "2023-05-07")},
			want: "'2023-05-07'",
		},
		"a range": {
			node: TimeValueNode[uint64]{
				token: tok(lexer.PHRASE, "2023-05-07"),
				dots:  tok(lexer.RANGE, ".."),
				to:    tok(lexer.PHRASE, "2023-05-09"),
			},
			want: "'2023-05-07'..'2023-05-09'",
		},
		"a range with no end": {
			node: TimeValueNode[uint64]{
				token: tok(lexer.PHRASE, "2023-05-07"),
				dots:  tok(lexer.RANGE, ".."),
			},
			want: "'2023-05-07'..",
		},
		"a query": {
			node: TimeValueNode[uint64]{subquery: SubqueryNode{
				lparen: tok(lexer.LPAREN, "("),
				query:  when,
				rparen: tok(lexer.RPAREN, ")"),
			}},
			want: "(when:said race)",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.node.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A time value is a time written out or a query, never both. Value is nil for
// the query and Query is nil for the time, which is how the query layer tells
// which one bounds the recall.
func TestTimeValueNodeIsATimeOrAQuery(t *testing.T) {
	tv := containers.AbsoluteTime[uint64]{T: time.Date(2023, time.May, 7, 0, 0, 0, 0, time.UTC), Precision: containers.Day}
	when := &WhenCommandNode[uint64, float32]{key: tok(lexer.WHEN, "when")}

	written := TimeValueNode[uint64]{token: tok(lexer.PHRASE, "2023-05-07"), value: tv, pos: pos(7), end: pos(18)}
	if got, want := written.Value(), containers.TimeValue[uint64](tv); got != want {
		t.Errorf("Value() = %v, want %v", got, want)
	}
	if got := written.Query(); got != nil {
		t.Errorf("Query() = %v, want nil for a time written out", got)
	}
	if got, want := written.Pos(), pos(7); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := written.End(), pos(18); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}

	asked := TimeValueNode[uint64]{subquery: SubqueryNode{query: when}}
	if got := asked.Value(); got != nil {
		t.Errorf("Value() = %v, want nil for a query", got)
	}
	if got, want := asked.Query(), CommandNode(when); got != want {
		t.Errorf("Query() = %v, want %v", got, want)
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
		value: TimeValueNode[uint64]{value: tv},
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
		value: TimeValueNode[uint64]{value: tv},
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

// A bound written as a query holds no time of its own. The field hands the
// query over through Query, and its Value is nil.
func TestTimeBoundHeldByAQuery(t *testing.T) {
	when := &WhenCommandNode[uint64, float32]{key: tok(lexer.WHEN, "when")}
	value := TimeValueNode[uint64]{subquery: SubqueryNode{
		lparen: tok(lexer.LPAREN, "("),
		query:  when,
		rparen: tok(lexer.RPAREN, ")"),
	}}

	since := SinceFieldNode[uint64]{key: tok(lexer.SINCE, "since"), value: value}
	if got, want := since.Query(), CommandNode(when); got != want {
		t.Errorf("since Query() = %v, want %v", got, want)
	}
	if got := since.Value(); got != nil {
		t.Errorf("since Value() = %v, want nil for a query", got)
	}
	if got, want := since.String(), "since:(when)"; got != want {
		t.Errorf("since String() = %q, want %q", got, want)
	}

	until := UntilFieldNode[uint64]{key: tok(lexer.UNTIL, "until"), value: value}
	if got, want := until.Query(), CommandNode(when); got != want {
		t.Errorf("until Query() = %v, want %v", got, want)
	}
	if got := until.Value(); got != nil {
		t.Errorf("until Value() = %v, want nil for a query", got)
	}
	if got, want := until.String(), "until:(when)"; got != want {
		t.Errorf("until String() = %q, want %q", got, want)
	}
}

func TestSaidFieldNode(t *testing.T) {
	tv := containers.AbsoluteTime[uint64]{T: time.Date(2023, time.May, 8, 0, 0, 0, 0, time.UTC), Precision: containers.Day}
	n := SaidFieldNode[uint64]{
		key:   tok(lexer.SAID, "said"),
		value: TimeValueNode[uint64]{token: tok(lexer.PHRASE, "2023-05-08"), value: tv},
		pos:   pos(14),
		end:   pos(30),
	}

	if got, want := n.Key(), "said"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Value(), containers.TimeValue[uint64](tv); got != want {
		t.Errorf("Value() = %v, want %v", got, want)
	}
	if got, want := n.String(), "said:'2023-05-08'"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	want := containers.Interval{
		Start: time.Date(2023, time.May, 8, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2023, time.May, 9, 0, 0, 0, 0, time.UTC),
	}
	if diff := cmp.Diff(want, n.TimeValue()); diff != "" {
		t.Errorf("TimeValue() mismatch (-want +got):\n%s", diff)
	}
	if got, want := n.Pos(), pos(14); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(30); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

func TestOccurredFieldNode(t *testing.T) {
	from := containers.AbsoluteTime[uint64]{T: time.Date(2023, time.May, 1, 0, 0, 0, 0, time.UTC), Precision: containers.Month}
	tv := containers.RangeTime[uint64]{From: from}
	n := OccurredFieldNode[uint64]{
		key: tok(lexer.OCCURRED, "occurred"),
		value: TimeValueNode[uint64]{
			token: tok(lexer.PHRASE, "2023-05"),
			dots:  tok(lexer.RANGE, ".."),
			value: tv,
		},
		pos: pos(14),
		end: pos(33),
	}

	if got, want := n.Key(), "occurred"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
	if got, want := n.Value(), containers.TimeValue[uint64](tv); got != want {
		t.Errorf("Value() = %v, want %v", got, want)
	}
	if got, want := n.String(), "occurred:'2023-05'.."; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	// A range with no end has no End: it runs on from its start.
	want := containers.Interval{Start: time.Date(2023, time.May, 1, 0, 0, 0, 0, time.UTC)}
	if diff := cmp.Diff(want, n.TimeValue()); diff != "" {
		t.Errorf("TimeValue() mismatch (-want +got):\n%s", diff)
	}
	if got, want := n.Pos(), pos(14); got != want {
		t.Errorf("Pos() = %v, want %v", got, want)
	}
	if got, want := n.End(), pos(33); got != want {
		t.Errorf("End() = %v, want %v", got, want)
	}
}

// TestTimeFieldWithoutATimeIsAllOfTime pins what a time field resolves to when
// it holds no written time: the clause is absent, or its bound is a query. Both
// leave the field's value nil, and resolving a nil value would panic. The zero
// Interval is all of time, the window of a recall that names no bound.
func TestTimeFieldWithoutATimeIsAllOfTime(t *testing.T) {
	asked := TimeValueNode[uint64]{subquery: SubqueryNode{
		query: &WhenCommandNode[uint64, float32]{key: tok(lexer.WHEN, "when")},
	}}

	cases := map[string]struct {
		got containers.Interval
	}{
		"since, absent":    {SinceFieldNode[uint64]{}.TimeValue()},
		"since, a query":   {SinceFieldNode[uint64]{key: tok(lexer.SINCE, "since"), value: asked}.TimeValue()},
		"until, absent":    {UntilFieldNode[uint64]{}.TimeValue()},
		"until, a query":   {UntilFieldNode[uint64]{key: tok(lexer.UNTIL, "until"), value: asked}.TimeValue()},
		"said, absent":     {SaidFieldNode[uint64]{}.TimeValue()},
		"occurred, absent": {OccurredFieldNode[uint64]{}.TimeValue()},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(containers.Interval{}, tc.got); diff != "" {
				t.Errorf("TimeValue() mismatch (-want +got):\n%s", diff)
			}
		})
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
