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

	"github.com/FraiseHQ/fraise/internal/query/lexer"
)

func Test_newLexer(t *testing.T) {
	l := lexer.New("recall anna topic:job")
	if l.Character != rune('r') {
		t.Error("Character should be:", rune('r'), "but got:", l.Character)
	}
}

func Test_Next(t *testing.T) {
	l := lexer.New("remember anna 'I work at Google' topic:job")
	token1 := l.Next()
	token2 := l.Next()
	if token1.Literal != "remember" || token1.Type != lexer.REMEMBER {
		t.Error("Wrong Value")
	}
	if token2.Literal != "anna" || token2.Type != lexer.LITERAL {
		t.Error("Wrong Value")
	}
}

func Test_NextUntilEol(t *testing.T) {
	l := lexer.New("recall anna topic:job")
	for i := 0; i < 5; i++ {
		_ = l.Next()
	}
	token := l.Next()
	if token.Literal != "" || token.Type != lexer.EOL {
		t.Error("Wrong Value")
	}
}

func Test_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []lexer.Token
	}{
		{
			name:  "colon token",
			input: "topic:job",
			expected: []lexer.Token{
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "parentheses tokens",
			input: "(anna)",
			expected: []lexer.Token{
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "dollar sign token",
			input: "$vec",
			expected: []lexer.Token{
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.VEC, Literal: "vec"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "single quoted phrase",
			input: "'hello'",
			expected: []lexer.Token{
				{Type: lexer.PHRASE, Literal: "hello"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "consecutive special chars",
			input: ":::",
			expected: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			// The three anchor-clause markers are lexed but no production
			// accepts them, so they exist only to be rejected by name. A term
			// may still contain '-' and '+' — only a leading one is a token.
			name:  "anchor markers are their own tokens",
			input: "+~- foo-bar",
			expected: []lexer.Token{
				{Type: lexer.PLUS, Literal: "+"},
				{Type: lexer.TILDE, Literal: "~"},
				{Type: lexer.MINUS, Literal: "-"},
				{Type: lexer.LITERAL, Literal: "foo-bar"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "nested parentheses",
			input: "(())",
			expected: []lexer.Token{
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.New(tt.input)
			for i, expected := range tt.expected {
				token := l.Next()
				if token.Type != expected.Type || token.Literal != expected.Literal {
					t.Errorf("Token %d: expected {Type: %v, Literal: %q}, got {Type: %v, Literal: %q}",
						i, expected.Type, expected.Literal, token.Type, token.Literal)
				}
			}
		})
	}
}

func Test_AllKeyLITERALs(t *testing.T) {
	tests := []struct {
		input    string
		expected lexer.TokenType
	}{
		{"recall", lexer.RECALL},
		{"remember", lexer.REMEMBER},
		{"forget", lexer.FORGET},
		{"update", lexer.UPDATE},
		{"topic", lexer.TOPIC},
		{"since", lexer.SINCE},
		{"until", lexer.UNTIL},
		{"top", lexer.TOP},
		{"depth", lexer.DEPTH},
		{"vec", lexer.VEC},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := lexer.New(tt.input)
			token := l.Next()
			if token.Type != tt.expected {
				t.Errorf("Expected token type %v for %q, got %v", tt.expected, tt.input, token.Type)
			}
			if token.Literal != tt.input {
				t.Errorf("Expected literal %q, got %q", tt.input, token.Literal)
			}
		})
	}
}

func Test_KeyLITERALCaseSensitivity(t *testing.T) {
	tests := []struct {
		input    string
		expected lexer.TokenType
	}{
		{"RECALL", lexer.LITERAL},
		{"ReCaLl", lexer.LITERAL},
		{"Forget", lexer.LITERAL},
		{"UPDATE", lexer.LITERAL},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := lexer.New(tt.input)
			token := l.Next()
			if token.Type != tt.expected {
				t.Errorf("Expected %q to be tokenized as %v, got %v", tt.input, tt.expected, token.Type)
			}
		})
	}
}

func Test_KeyLITERALsAsSubstrings(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"remembering"},
		{"recall123"},
		{"forget_me"},
		{"and_then"},
		{"topical"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := lexer.New(tt.input)
			token := l.Next()
			if token.Type != lexer.LITERAL {
				t.Errorf("Expected %q to be tokenized as LITERAL, got %v", tt.input, token.Type)
			}
		})
	}
}

func Test_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []lexer.Token
	}{
		{
			name:  "multiple spaces",
			input: "recall    anna",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "tabs between tokens",
			input: "recall\t\tanna",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "mixed whitespace",
			input: "recall \t \r anna",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "leading whitespace",
			input: "   recall anna",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "trailing whitespace",
			input: "recall anna   ",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.New(tt.input)
			for i, expected := range tt.expected {
				token := l.Next()
				if token.Type != expected.Type || token.Literal != expected.Literal {
					t.Errorf("Token %d: expected {Type: %v, Literal: %q}, got {Type: %v, Literal: %q}",
						i, expected.Type, expected.Literal, token.Type, token.Literal)
				}
			}
		})
	}
}

func Test_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []lexer.Token
	}{
		{
			name:  "empty input",
			input: "",
			expected: []lexer.Token{
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "whitespace only",
			input: "   \t\r  ",
			expected: []lexer.Token{
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "single character",
			input: "a",
			expected: []lexer.Token{
				{Type: lexer.LITERAL, Literal: "a"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "single special character",
			input: ":",
			expected: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "only special characters",
			input: ":$()",
			expected: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.New(tt.input)
			for i, expected := range tt.expected {
				token := l.Next()
				if token.Type != expected.Type || token.Literal != expected.Literal {
					t.Errorf("Token %d: expected {Type: %v, Literal: %q}, got {Type: %v, Literal: %q}",
						i, expected.Type, expected.Literal, token.Type, token.Literal)
				}
			}
		})
	}
}

func Test_ReadPastEOL(t *testing.T) {
	l := lexer.New("recall")
	l.Next() // consume "recall"

	token1 := l.Next() // first EOL
	token2 := l.Next() // second EOL
	token3 := l.Next() // third EOL

	if token1.Type != lexer.EOL || token2.Type != lexer.EOL || token3.Type != lexer.EOL {
		t.Error("Reading past EOL should continue returning EOL tokens")
	}
}

func Test_ComplexQueries(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []lexer.Token
	}{
		{
			name:  "query with filter",
			input: "recall anna topic:job",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			// "or"/"and" are no longer keywords; they now scan as plain literals.
			name:  "former boolean keywords lex as literals",
			input: "recall (anna or bob) and topic:job",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.LITERAL, Literal: "or"},
				{Type: lexer.LITERAL, Literal: "bob"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.LITERAL, Literal: "and"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "with parameters",
			input: "recall $vec top:5 depth:3",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.VEC, Literal: "vec"},
				{Type: lexer.TOP, Literal: "top"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "5"},
				{Type: lexer.DEPTH, Literal: "depth"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "3"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "date filters",
			input: "recall anna since:2024-01-01 until:2024-12-31",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.SINCE, Literal: "since"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "2024-01-01"},
				{Type: lexer.UNTIL, Literal: "until"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "2024-12-31"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.New(tt.input)
			for i, expected := range tt.expected {
				token := l.Next()
				if token.Type != expected.Type || token.Literal != expected.Literal {
					t.Errorf("Token %d: expected {Type: %v, Literal: %q}, got {Type: %v, Literal: %q}",
						i, expected.Type, expected.Literal, token.Type, token.Literal)
				}
			}
		})
	}
}

func Test_StringScanningEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []lexer.Token
	}{
		{
			name:  "empty string between delimiters",
			input: "topic::job",
			expected: []lexer.Token{
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "LITERAL ending with special char",
			input: "LITERAL:",
			expected: []lexer.Token{
				{Type: lexer.LITERAL, Literal: "LITERAL"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "special char at start",
			input: ":LITERAL",
			expected: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "LITERAL"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.New(tt.input)
			for i, expected := range tt.expected {
				token := l.Next()
				if token.Type != expected.Type || token.Literal != expected.Literal {
					t.Errorf("Token %d: expected {Type: %v, Literal: %q}, got {Type: %v, Literal: %q}",
						i, expected.Type, expected.Literal, token.Type, token.Literal)
				}
			}
		})
	}
}

func Test_PositionTracking(t *testing.T) {
	l := lexer.New("recall anna")

	if l.CurrentPos.Column != 0 {
		t.Errorf("Expected initial CurrentPos.Column to be 0, got %d", l.CurrentPos.Column)
	}

	l.Next() // consume "recall"

	if l.CurrentPos.Column != 6 {
		t.Errorf("After reading 'recall', expected CurrentPos.Column to be 6, got %d", l.CurrentPos.Column)
	}
}

// TestTokenPosIsLastCharacter pins the contract the parser's error positions
// rest on: a token's Pos is the 1-based column of its last character. The
// parser cannot compute this itself — cur/peek read a token ahead, so by the
// time it rejects a token the lexer's CurrentPos has passed the following one
// — so if this drifts, every "parse error at column N" silently points at the
// wrong word while still reading like a precise message.
func TestTokenPosIsLastCharacter(t *testing.T) {
	// The columns below are the 1-based index of each token's final character:
	// "recall" ends at 6, "@" at 7, "2" at 8, "anna" at 13, and so on.
	input := "recall@2 anna topic:food top:10"
	want := []struct {
		literal string
		column  int
	}{
		{"recall", 6},
		{"@", 7},
		{"2", 8},
		{"anna", 13},
		{"topic", 19},
		{":", 20},
		{"food", 24},
		{"top", 28},
		{":", 29},
		{"10", 31},
	}

	l := lexer.New(input)
	for _, w := range want {
		tok := l.Next()
		if tok.Literal != w.literal {
			t.Fatalf("token stream diverged: got %q, want %q", tok.Literal, w.literal)
		}
		if tok.Pos.Column != w.column {
			t.Errorf("%q: Pos.Column = %d, want %d (its last character)", w.literal, tok.Pos.Column, w.column)
		}
	}

	// EOL carries the end of input, so an error about a clause cut short lands
	// past the last character rather than at column 0.
	if tok := l.Next(); tok.Type != lexer.EOL || tok.Pos.Column != len(input) {
		t.Errorf("EOL Pos.Column = %d (type %v), want %d", tok.Pos.Column, tok.Type, len(input))
	}
}

// TestPhrasePosIsLastCharacter pins that a phrase is no exception to the rule
// above: its Pos is its closing quote, so the parser's one blaming rule holds
// for every token type.
//
// NOTE: scanPhrase does record the *opening* quote (`Pos: start`), meaning to
// report an unterminated phrase where it began, but Next() assigns tok.Pos
// after the switch and overwrites it — so that position never reaches the
// parser. If that assignment is ever made conditional, the phrase cases here
// and in TestScanPhraseUnterminatedRecordsPosition are what should change,
// together with the Token.Pos contract.
func TestPhrasePosIsLastCharacter(t *testing.T) {
	// "remember 'a fact' topic:x" — the closing quote is the 17th character.
	l := lexer.New("remember 'a fact' topic:x")
	l.Next() // remember

	tok := l.Next()
	if tok.Type != lexer.PHRASE {
		t.Fatalf("got %v %q, want a PHRASE", tok.Type, tok.Literal)
	}
	if tok.Pos.Column != 17 {
		t.Errorf("phrase Pos.Column = %d, want 17 (its closing quote)", tok.Pos.Column)
	}
}

func Test_VeryLongQuery(t *testing.T) {
	input := "recall $vec anna bob charlie +topic:personal ~topic:draft since:2024-01-01 until:2024-12-31 top:10 depth:5"

	expected := []lexer.Token{
		{Type: lexer.RECALL, Literal: "recall"},
		{Type: lexer.DOLLAR, Literal: "$"},
		{Type: lexer.VEC, Literal: "vec"},
		{Type: lexer.LITERAL, Literal: "anna"},
		{Type: lexer.LITERAL, Literal: "bob"},
		{Type: lexer.LITERAL, Literal: "charlie"},
		{Type: lexer.PLUS, Literal: "+"},
		{Type: lexer.TOPIC, Literal: "topic"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "personal"},
		{Type: lexer.TILDE, Literal: "~"},
		{Type: lexer.TOPIC, Literal: "topic"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "draft"},
		{Type: lexer.SINCE, Literal: "since"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "2024-01-01"},
		{Type: lexer.UNTIL, Literal: "until"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "2024-12-31"},
		{Type: lexer.TOP, Literal: "top"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "10"},
		{Type: lexer.DEPTH, Literal: "depth"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "5"},
		{Type: lexer.EOL, Literal: ""},
	}

	l := lexer.New(input)
	for i, expected := range expected {
		token := l.Next()
		if token.Type != expected.Type || token.Literal != expected.Literal {
			t.Errorf("Token %d: expected {Type: %v, Literal: %q}, got {Type: %v, Literal: %q}",
				i, expected.Type, expected.Literal, token.Type, token.Literal)
		}
	}
}

func Test_MultipleCommandsInSequence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []lexer.Token
	}{
		{
			name:  "remember with quoted content",
			input: "remember anna 'worked at Google from 2020 to 2023' topic:job",
			expected: []lexer.Token{
				{Type: lexer.REMEMBER, Literal: "remember"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.PHRASE, Literal: "worked at Google from 2020 to 2023"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "forget with multiple filters",
			input: "forget anna topic:draft since:2023-01-01",
			expected: []lexer.Token{
				{Type: lexer.FORGET, Literal: "forget"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "draft"},
				{Type: lexer.SINCE, Literal: "since"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "2023-01-01"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "update command",
			input: "update anna 'new information' topic:job",
			expected: []lexer.Token{
				{Type: lexer.UPDATE, Literal: "update"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.PHRASE, Literal: "new information"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.New(tt.input)
			for i, expected := range tt.expected {
				token := l.Next()
				if token.Type != expected.Type || token.Literal != expected.Literal {
					t.Errorf("Token %d: expected {Type: %v, Literal: %q}, got {Type: %v, Literal: %q}",
						i, expected.Type, expected.Literal, token.Type, token.Literal)
				}
			}
		})
	}
}

// Test_NewlineIsItsOwnToken pins that a newline ends an instruction instead of
// blending into the whitespace around it. Folded into blank, "recall anna\nbob"
// lexed as one two-term recall — a second line silently joining the first, which
// is exactly the multi-command shape the grammar forbids. Spaces, tabs and CR
// stay blank; only the newline is a token, and the parser decides whether a
// trailing one is a second instruction or just the end of the text.
func Test_NewlineIsItsOwnToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []lexer.Token
	}{
		{
			name:  "a newline separates the lines it sits between",
			input: "recall anna\ntopic:job",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "consecutive newlines are consecutive tokens",
			input: "recall\n\n\nanna",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "spaces and tabs around a newline stay blank",
			input: "recall  \n\t  anna",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "a CRLF is one newline, the CR being blank",
			input: "recall\r\nanna",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.NEWLINE, Literal: "\n"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "a newline inside a phrase is data, not a separator",
			input: "remember 'line one\nline two'",
			expected: []lexer.Token{
				{Type: lexer.REMEMBER, Literal: "remember"},
				{Type: lexer.PHRASE, Literal: "line one\nline two"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.New(tt.input)
			for i, expected := range tt.expected {
				token := l.Next()
				if token.Type != expected.Type || token.Literal != expected.Literal {
					t.Errorf("Token %d: expected {Type: %v, Literal: %q}, got {Type: %v, Literal: %q}",
						i, expected.Type, expected.Literal, token.Type, token.Literal)
				}
			}
		})
	}
}

// TestScanPhrase covers the opaque single-quoted phrase scanner: everything
// between the quotes is one PHRASE token taken verbatim (reserved words and
// symbols included), ” is an escaped quote, and an unclosed quote yields an
// ILLEGAL token so the parser can report an unterminated phrase.
func TestScanPhrase(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantTyp lexer.TokenType
		wantLit string
	}{
		{"reserved words inside", "'remember the topic since'", lexer.PHRASE, "remember the topic since"},
		{"colon / time", "'meeting at 3:30pm'", lexer.PHRASE, "meeting at 3:30pm"},
		{"symbols inside", "'$ @ ( ) :'", lexer.PHRASE, "$ @ ( ) :"},
		{"escaped apostrophe", "'alice''s laptop'", lexer.PHRASE, "alice's laptop"},
		{"empty phrase", "''", lexer.PHRASE, ""},
		{"only an escaped quote", "''''", lexer.PHRASE, "'"},
		{"interior spacing preserved", "'a   b'", lexer.PHRASE, "a   b"},
		{"unterminated", "'oops no close", lexer.ILLEGAL, "oops no close"},
		// Free-flowing text: ingestion feeds phrases arbitrary prose, so every
		// JSON-transportable character is data between the quotes.
		{"newline inside", "'line one\nline two'", lexer.PHRASE, "line one\nline two"},
		{"crlf and tab inside", "'a\r\n\tb'", lexer.PHRASE, "a\r\n\tb"},
		{"emoji and cjk", "'déjà vu 😀 東京'", lexer.PHRASE, "déjà vu 😀 東京"},
		{"backslashes stay literal", `'C:\temp\new'`, lexer.PHRASE, `C:\temp\new`},
		{"double quotes stay literal", `'she said "hi"'`, lexer.PHRASE, `she said "hi"`},
		{"nul is data, not end of input", "'a\x00b'", lexer.PHRASE, "a\x00b"},
		{"apostrophe at both edges", "'''wow'''", lexer.PHRASE, "'wow'"},
		{"apostrophe cluster", "'it''s ''fine'''", lexer.PHRASE, "it's 'fine'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lexer.New(tc.input).Next()
			if got.Type != tc.wantTyp || got.Literal != tc.wantLit {
				t.Errorf("Next() = {%v %q}, want {%v %q}",
					got.Type, got.Literal, tc.wantTyp, tc.wantLit)
			}
		})
	}
}

// TestScanPhraseUnterminatedRecordsPosition checks that an unterminated phrase
// reports the position of its opening quote (used by the parser's error).
func TestScanPhraseUnterminatedRecordsPosition(t *testing.T) {
	// opening quote is the 5th rune (0-indexed column 4): "foo <'>bar"
	got := lexer.New("foo 'bar").Next() // first token is the bare word "foo"
	if got.Type != lexer.LITERAL || got.Literal != "foo" {
		t.Fatalf("first token = {%v %q}, want LITERAL \"foo\"", got.Type, got.Literal)
	}
}
