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

package lexer_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/FraiseHQ/fraise/internal/query/lexer"
)

func Test_newLexer(t *testing.T) {
	l := lexer.New("recall anna topic:job")
	if got, want := l.Character, rune('r'); got != want {
		t.Errorf("Character = %q, want %q", got, want)
	}
}

func Test_Next(t *testing.T) {
	l := lexer.New("remember anna 'I work at Google' topic:job")
	got := []lexer.Token{l.Next(), l.Next(), l.Next()}
	want := []lexer.Token{
		{Type: lexer.REMEMBER, Literal: "remember"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.LITERAL, Literal: "anna"},
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
		t.Errorf("Next() mismatch (-want +got):\n%s", diff)
	}
}

func Test_NextUntilEol(t *testing.T) {
	l := lexer.New("recall anna topic:job")
	for i := 0; i < 7; i++ {
		_ = l.Next()
	}
	got := l.Next()
	want := lexer.Token{Type: lexer.EOL, Literal: ""}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
		t.Errorf("Next() after the last token mismatch (-want +got):\n%s", diff)
	}
}

func Test_SpecialCharacters(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []lexer.Token
	}{
		"colon token": {
			input: "topic:job",
			want: []lexer.Token{
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"parentheses tokens": {
			input: "(anna)",
			want: []lexer.Token{
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"dollar sign token": {
			input: "$vec",
			want: []lexer.Token{
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.VEC, Literal: "vec"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"single quoted phrase": {
			input: "'hello'",
			want: []lexer.Token{
				{Type: lexer.PHRASE, Literal: "hello"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"consecutive special chars": {
			input: ":::",
			want: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		// The three anchor-clause markers are lexed, but no production
		// accepts them. They exist only to be rejected by name. A word is
		// letters and digits, so a '-' inside one ends it. foo-bar is
		// written 'foo-bar'.
		"anchor markers are their own tokens": {
			input: "+~-foo-bar",
			want: []lexer.Token{
				{Type: lexer.PLUS, Literal: "+"},
				{Type: lexer.TILDE, Literal: "~"},
				{Type: lexer.MINUS, Literal: "-"},
				{Type: lexer.LITERAL, Literal: "foo"},
				{Type: lexer.MINUS, Literal: "-"},
				{Type: lexer.LITERAL, Literal: "bar"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		// Any other character outside a phrase is a token of its own, one
		// character long, so a word ends at it instead of absorbing it.
		"special characters are their own tokens": {
			input: "ferry;é_2",
			want: []lexer.Token{
				{Type: lexer.LITERAL, Literal: "ferry"},
				{Type: lexer.SPECIAL, Literal: ";"},
				{Type: lexer.LITERAL, Literal: "é"},
				{Type: lexer.SPECIAL, Literal: "_"},
				{Type: lexer.LITERAL, Literal: "2"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"nested parentheses": {
			input: "(())",
			want: []lexer.Token{
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			l := lexer.New(tc.input)
			got := make([]lexer.Token, len(tc.want))
			for i := range got {
				got[i] = l.Next()
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

func Test_AllKeyLITERALs(t *testing.T) {
	tests := map[string]struct {
		want lexer.TokenType
	}{
		"recall":   {want: lexer.RECALL},
		"remember": {want: lexer.REMEMBER},
		"forget":   {want: lexer.FORGET},
		"update":   {want: lexer.UPDATE},
		"topic":    {want: lexer.TOPIC},
		"since":    {want: lexer.SINCE},
		"until":    {want: lexer.UNTIL},
		"top":      {want: lexer.TOP},
		"depth":    {want: lexer.DEPTH},
		"vec":      {want: lexer.VEC},
	}

	for input, tc := range tests {
		t.Run(input, func(t *testing.T) {
			got := lexer.New(input).Next()
			want := lexer.Token{Type: tc.want, Literal: input}
			if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", input, diff)
			}
		})
	}
}

func Test_KeyLITERALCaseSensitivity(t *testing.T) {
	tests := map[string]struct {
		want lexer.TokenType
	}{
		"RECALL": {want: lexer.LITERAL},
		"ReCaLl": {want: lexer.LITERAL},
		"Forget": {want: lexer.LITERAL},
		"UPDATE": {want: lexer.LITERAL},
	}

	for input, tc := range tests {
		t.Run(input, func(t *testing.T) {
			if got := lexer.New(input).Next().Type; got != tc.want {
				t.Errorf("lexing %q: type = %v, want %v", input, got, tc.want)
			}
		})
	}
}

func Test_KeyLITERALsAsSubstrings(t *testing.T) {
	tests := map[string]struct{}{
		"remembering": {},
		"recall123":   {},
		"forgetme":    {},
		"andthen":     {},
		"topical":     {},
	}

	for input := range tests {
		t.Run(input, func(t *testing.T) {
			if got, want := lexer.New(input).Next().Type, lexer.LITERAL; got != want {
				t.Errorf("lexing %q: type = %v, want %v", input, got, want)
			}
		})
	}
}

// Test_WhitespaceHandling pins that a run of blanks is kept, not skipped. It
// is one WHITESPACE token that carries the run verbatim, wherever it sits:
// between words, before the first or after the last.
func Test_WhitespaceHandling(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []lexer.Token
	}{
		"multiple spaces": {
			input: "recall    anna",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: "    "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"tabs between tokens": {
			input: "recall\t\tanna",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: "\t\t"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"mixed whitespace": {
			input: "recall \t \r anna",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: " \t \r "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"leading whitespace": {
			input: "   recall anna",
			want: []lexer.Token{
				{Type: lexer.WHITESPACE, Literal: "   "},
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"trailing whitespace": {
			input: "recall anna   ",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.WHITESPACE, Literal: "   "},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			l := lexer.New(tc.input)
			got := make([]lexer.Token, len(tc.want))
			for i := range got {
				got[i] = l.Next()
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

func Test_BoundaryConditions(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []lexer.Token
	}{
		"empty input": {
			input: "",
			want: []lexer.Token{
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"whitespace only": {
			input: "   \t\r  ",
			want: []lexer.Token{
				{Type: lexer.WHITESPACE, Literal: "   \t\r  "},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"single character": {
			input: "a",
			want: []lexer.Token{
				{Type: lexer.LITERAL, Literal: "a"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"single special character": {
			input: ":",
			want: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"only special characters": {
			input: ":$()",
			want: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			l := lexer.New(tc.input)
			got := make([]lexer.Token, len(tc.want))
			for i := range got {
				got[i] = l.Next()
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

func Test_ReadPastEOL(t *testing.T) {
	l := lexer.New("recall")
	l.Next() // consume "recall"

	got := []lexer.TokenType{l.Next().Type, l.Next().Type, l.Next().Type}
	want := []lexer.TokenType{lexer.EOL, lexer.EOL, lexer.EOL}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("reading past EOL should keep returning EOL tokens: mismatch (-want +got):\n%s", diff)
	}
}

func Test_ComplexQueries(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []lexer.Token
	}{
		"query with filter": {
			input: "recall anna topic:job",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		// "or" and "and" are not keywords: they lex as plain literals.
		"former boolean keywords lex as literals": {
			input: "recall (anna or bob) and topic:job",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "or"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "bob"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "and"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"with parameters": {
			input: "recall $vec top:5 depth:3",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.VEC, Literal: "vec"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.TOP, Literal: "top"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "5"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.DEPTH, Literal: "depth"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "3"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"date filters": {
			input: "recall anna since:'2024-01-01' until:'2024-12-31'",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.SINCE, Literal: "since"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.PHRASE, Literal: "2024-01-01"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.UNTIL, Literal: "until"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.PHRASE, Literal: "2024-12-31"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			l := lexer.New(tc.input)
			got := make([]lexer.Token, len(tc.want))
			for i := range got {
				got[i] = l.Next()
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

func Test_StringScanningEdgeCases(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []lexer.Token
	}{
		"empty string between delimiters": {
			input: "topic::job",
			want: []lexer.Token{
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"LITERAL ending with special char": {
			input: "LITERAL:",
			want: []lexer.Token{
				{Type: lexer.LITERAL, Literal: "LITERAL"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"special char at start": {
			input: ":LITERAL",
			want: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "LITERAL"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			l := lexer.New(tc.input)
			got := make([]lexer.Token, len(tc.want))
			for i := range got {
				got[i] = l.Next()
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

func Test_PositionTracking(t *testing.T) {
	l := lexer.New("recall anna")

	if got, want := l.CurrentPos.Column, 0; got != want {
		t.Errorf("initial CurrentPos.Column = %d, want %d", got, want)
	}

	l.Next() // consume "recall"

	if got, want := l.CurrentPos.Column, 6; got != want {
		t.Errorf("after reading 'recall', CurrentPos.Column = %d, want %d", got, want)
	}
}

// TestTokenPosIsLastCharacter pins the contract the parser's error positions
// rest on. A token's Pos is the 1-based column of its last character. The
// parser cannot compute this itself. By the time it rejects a token, its
// lookahead has moved the lexer's CurrentPos past the following token. If
// this drifts, every "parse error at column N" points at the wrong word while
// still reading like a precise message.
func TestTokenPosIsLastCharacter(t *testing.T) {
	type token struct {
		Literal string
		Column  int
	}

	// The columns below are the 1-based index of each token's last character.
	// "recall" ends at 6, "@" at 7, "2" at 8, the space after it at 9, "anna"
	// at 13, and so on.
	input := "recall@2 anna topic:food top:10"
	want := []token{
		{"recall", 6},
		{"@", 7},
		{"2", 8},
		{" ", 9},
		{"anna", 13},
		{" ", 14},
		{"topic", 19},
		{":", 20},
		{"food", 24},
		{" ", 25},
		{"top", 28},
		{":", 29},
		{"10", 31},
	}

	l := lexer.New(input)
	got := make([]token, len(want))
	for i := range got {
		tok := l.Next()
		got[i] = token{tok.Literal, tok.Pos.Column}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("token Pos.Column is not its last character: mismatch (-want +got):\n%s", diff)
	}

	// EOL's Pos is the end of input, the input's length. An error about a
	// clause cut short then points at the end of the query, not at column 0.
	if tok := l.Next(); tok.Type != lexer.EOL || tok.Pos.Column != len(input) {
		t.Errorf("EOL Pos.Column = %d (type %v), want %d", tok.Pos.Column, tok.Type, len(input))
	}
}

// TestPhrasePosIsLastCharacter pins that a phrase is no exception to the rule
// above. Its Pos is its closing quote, so the parser's one blaming rule holds
// for every token type.
func TestPhrasePosIsLastCharacter(t *testing.T) {
	// In "remember 'a fact' topic:x", the closing quote is the 17th character.
	l := lexer.New("remember 'a fact' topic:x")
	l.Next() // remember
	l.Next() // the space before the phrase

	got := l.Next()
	if got.Type != lexer.PHRASE {
		t.Fatalf("got %v %q, want a PHRASE", got.Type, got.Literal)
	}
	if want := 17; got.Pos.Column != want {
		t.Errorf("phrase Pos.Column = %d, want %d (its closing quote)", got.Pos.Column, want)
	}
}

func Test_VeryLongQuery(t *testing.T) {
	input := "recall $vec anna bob charlie +topic:personal ~topic:draft since:'2024-01-01' until:'2024-12-31' top:10 depth:5"

	want := []lexer.Token{
		{Type: lexer.RECALL, Literal: "recall"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.DOLLAR, Literal: "$"},
		{Type: lexer.VEC, Literal: "vec"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.LITERAL, Literal: "anna"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.LITERAL, Literal: "bob"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.LITERAL, Literal: "charlie"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.PLUS, Literal: "+"},
		{Type: lexer.TOPIC, Literal: "topic"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "personal"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.TILDE, Literal: "~"},
		{Type: lexer.TOPIC, Literal: "topic"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "draft"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.SINCE, Literal: "since"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.PHRASE, Literal: "2024-01-01"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.UNTIL, Literal: "until"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.PHRASE, Literal: "2024-12-31"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.TOP, Literal: "top"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "10"},
		{Type: lexer.WHITESPACE, Literal: " "},
		{Type: lexer.DEPTH, Literal: "depth"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "5"},
		{Type: lexer.EOL, Literal: ""},
	}

	l := lexer.New(input)
	got := make([]lexer.Token, len(want))
	for i := range got {
		got[i] = l.Next()
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
		t.Errorf("lexing %q mismatch (-want +got):\n%s", input, diff)
	}
}

func Test_MultipleCommandsInSequence(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []lexer.Token
	}{
		"remember with quoted content": {
			input: "remember anna 'worked at Google from 2020 to 2023' topic:job",
			want: []lexer.Token{
				{Type: lexer.REMEMBER, Literal: "remember"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.PHRASE, Literal: "worked at Google from 2020 to 2023"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"forget with multiple filters": {
			input: "forget anna topic:draft since:'2023-01-01'",
			want: []lexer.Token{
				{Type: lexer.FORGET, Literal: "forget"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "draft"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.SINCE, Literal: "since"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.PHRASE, Literal: "2023-01-01"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"update command": {
			input: "update anna 'new information' topic:job",
			want: []lexer.Token{
				{Type: lexer.UPDATE, Literal: "update"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.PHRASE, Literal: "new information"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			l := lexer.New(tc.input)
			got := make([]lexer.Token, len(tc.want))
			for i := range got {
				got[i] = l.Next()
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

// Test_NewlineIsItsOwnToken pins that a newline is its own token, not part of
// the whitespace around it. So "recall anna\nbob" is two lines, not one
// two-term recall. Spaces, tabs and CR stay blank. They form a WHITESPACE
// token on either side of the newline, never part of it. The parser decides
// whether a trailing newline is a second instruction or the end of the text.
func Test_NewlineIsItsOwnToken(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []lexer.Token
	}{
		"a newline separates the lines it sits between": {
			input: "recall anna\ntopic:job",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"consecutive newlines are consecutive tokens": {
			input: "recall\n\n\nanna",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"spaces and tabs around a newline stay blank": {
			input: "recall  \n\t  anna",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: "  "},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.WHITESPACE, Literal: "\t  "},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"a CRLF is one newline, the CR being blank": {
			input: "recall\r\nanna",
			want: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.WHITESPACE, Literal: "\r"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		"a newline inside a phrase is data, not a separator": {
			input: "remember 'line one\nline two'",
			want: []lexer.Token{
				{Type: lexer.REMEMBER, Literal: "remember"},
				{Type: lexer.WHITESPACE, Literal: " "},
				{Type: lexer.PHRASE, Literal: "line one\nline two"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			l := lexer.New(tc.input)
			got := make([]lexer.Token, len(tc.want))
			for i := range got {
				got[i] = l.Next()
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

// TestScanPhrase covers the opaque single-quoted phrase scanner:
//   - Everything between the quotes is one PHRASE token, taken verbatim,
//     reserved words and symbols included.
//   - A doubled single quote is an escaped quote.
//   - An unclosed quote yields an ILLEGAL token, so the parser can report an
//     unterminated phrase.
func TestScanPhrase(t *testing.T) {
	cases := map[string]struct {
		input string
		want  lexer.Token
	}{
		"reserved words inside":      {"'remember the topic since'", lexer.Token{Type: lexer.PHRASE, Literal: "remember the topic since"}},
		"colon / time":               {"'meeting at 3:30pm'", lexer.Token{Type: lexer.PHRASE, Literal: "meeting at 3:30pm"}},
		"symbols inside":             {"'$ @ ( ) :'", lexer.Token{Type: lexer.PHRASE, Literal: "$ @ ( ) :"}},
		"escaped apostrophe":         {"'alice''s laptop'", lexer.Token{Type: lexer.PHRASE, Literal: "alice's laptop"}},
		"empty phrase":               {"''", lexer.Token{Type: lexer.PHRASE, Literal: ""}},
		"only an escaped quote":      {"''''", lexer.Token{Type: lexer.PHRASE, Literal: "'"}},
		"interior spacing preserved": {"'a   b'", lexer.Token{Type: lexer.PHRASE, Literal: "a   b"}},
		"unterminated":               {"'oops no close", lexer.Token{Type: lexer.ILLEGAL, Literal: "oops no close"}},
		// Ingestion feeds phrases arbitrary prose, so every character JSON
		// can carry is data between the quotes.
		"newline inside":                {"'line one\nline two'", lexer.Token{Type: lexer.PHRASE, Literal: "line one\nline two"}},
		"crlf and tab inside":           {"'a\r\n\tb'", lexer.Token{Type: lexer.PHRASE, Literal: "a\r\n\tb"}},
		"emoji and cjk":                 {"'déjà vu 😀 東京'", lexer.Token{Type: lexer.PHRASE, Literal: "déjà vu 😀 東京"}},
		"backslashes stay literal":      {`'C:\temp\new'`, lexer.Token{Type: lexer.PHRASE, Literal: `C:\temp\new`}},
		"double quotes stay literal":    {`'she said "hi"'`, lexer.Token{Type: lexer.PHRASE, Literal: `she said "hi"`}},
		"nul is data, not end of input": {"'a\x00b'", lexer.Token{Type: lexer.PHRASE, Literal: "a\x00b"}},
		"apostrophe at both edges":      {"'''wow'''", lexer.Token{Type: lexer.PHRASE, Literal: "'wow'"}},
		"apostrophe cluster":            {"'it''s ''fine'''", lexer.Token{Type: lexer.PHRASE, Literal: "it's 'fine'"}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := lexer.New(tc.input).Next()
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
				t.Errorf("lexing %q mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

// TestWordBeforeUnterminatedPhraseLexesAlone checks that the word before an
// unterminated phrase lexes on its own.
func TestWordBeforeUnterminatedPhraseLexesAlone(t *testing.T) {
	got := lexer.New("foo 'bar").Next()
	want := lexer.Token{Type: lexer.LITERAL, Literal: "foo"}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(lexer.Token{}, "Pos")); diff != "" {
		t.Errorf("first token mismatch (-want +got):\n%s", diff)
	}
}

// TestWhitespaceRunIsOneToken pins that a run of blanks is one WHITESPACE
// token, whatever mix of spaces, tabs and carriage returns it holds. The
// parser sees one separator however the caller spaced the query.
func TestWhitespaceRunIsOneToken(t *testing.T) {
	l := lexer.New("recall  \t \r zebras")
	want := []lexer.TokenType{lexer.RECALL, lexer.WHITESPACE, lexer.LITERAL, lexer.EOL}
	got := make([]lexer.TokenType, len(want))
	for i := range got {
		got[i] = l.Next().Type
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("token types mismatch (-want +got):\n%s", diff)
	}
}
