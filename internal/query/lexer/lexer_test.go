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

	"github.com/RonsenbergVI/fraise/internal/query/lexer"
)

func Test_newLexer(t *testing.T) {
	l := lexer.New("recall anna topic:job")
	if !(l.Character == rune('r')) {
		t.Error("Character should be:", rune('r'), "but got:", l.Character)
	}
}

func Test_Next(t *testing.T) {
	l := lexer.New("remember anna 'I work at Google' topic:job")
	token1 := l.Next()
	token2 := l.Next()
	if !(token1.Literal == "remember" && token1.Type == lexer.REMEMBER) {
		t.Error("Wrong Value")
	}
	if !(token2.Literal == "anna" && token2.Type == lexer.LITERAL) {
		t.Error("Wrong Value")
	}
}

func Test_NextUntilEol(t *testing.T) {
	l := lexer.New("recall anna topic:job")
	for i := 0; i < 5; i++ {
		_ = l.Next()
	}
	token := l.Next()
	if !(token.Literal == "" && token.Type == lexer.EOL) {
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
			name:  "single quote token",
			input: "'hello'",
			expected: []lexer.Token{
				{Type: lexer.COMMA, Literal: "'"},
				{Type: lexer.LITERAL, Literal: "hello"},
				{Type: lexer.COMMA, Literal: "'"},
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
		{"and", lexer.AND},
		{"or", lexer.OR},
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
		{"AND", lexer.LITERAL},
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
			input: ":$'()",
			expected: []lexer.Token{
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.COMMA, Literal: "'"},
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
			name:  "boolean operations",
			input: "recall (anna or bob) and topic:job",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.OR, Literal: "or"},
				{Type: lexer.LITERAL, Literal: "bob"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.AND, Literal: "and"},
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

func Test_VeryLongQuery(t *testing.T) {
	input := "recall $vec (anna or bob or charlie) and (topic:personal or topic:draft) since:2024-01-01 until:2024-12-31 top:10 depth:5"

	expected := []lexer.Token{
		{Type: lexer.RECALL, Literal: "recall"},
		{Type: lexer.DOLLAR, Literal: "$"},
		{Type: lexer.VEC, Literal: "vec"},
		{Type: lexer.LPAREN, Literal: "("},
		{Type: lexer.LITERAL, Literal: "anna"},
		{Type: lexer.OR, Literal: "or"},
		{Type: lexer.LITERAL, Literal: "bob"},
		{Type: lexer.OR, Literal: "or"},
		{Type: lexer.LITERAL, Literal: "charlie"},
		{Type: lexer.RPAREN, Literal: ")"},
		{Type: lexer.AND, Literal: "and"},
		{Type: lexer.LPAREN, Literal: "("},
		{Type: lexer.TOPIC, Literal: "topic"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "personal"},
		{Type: lexer.OR, Literal: "or"},
		{Type: lexer.TOPIC, Literal: "topic"},
		{Type: lexer.COLON, Literal: ":"},
		{Type: lexer.LITERAL, Literal: "draft"},
		{Type: lexer.RPAREN, Literal: ")"},
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
				{Type: lexer.COMMA, Literal: "'"},
				{Type: lexer.LITERAL, Literal: "worked"},
				{Type: lexer.LITERAL, Literal: "at"},
				{Type: lexer.LITERAL, Literal: "Google"},
				{Type: lexer.LITERAL, Literal: "from"},
				{Type: lexer.LITERAL, Literal: "2020"},
				{Type: lexer.LITERAL, Literal: "to"},
				{Type: lexer.LITERAL, Literal: "2023"},
				{Type: lexer.COMMA, Literal: "'"},
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
				{Type: lexer.COMMA, Literal: "'"},
				{Type: lexer.LITERAL, Literal: "new"},
				{Type: lexer.LITERAL, Literal: "information"},
				{Type: lexer.COMMA, Literal: "'"},
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

func Test_MultiLineQueries(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []lexer.Token
	}{
		{
			name:  "query with single newline",
			input: "recall anna\ntopic:job",
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
			name:  "query with multiple newlines",
			input: "recall\n$vec\n(anna or bob)\nand topic:job",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.VEC, Literal: "vec"},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.OR, Literal: "or"},
				{Type: lexer.LITERAL, Literal: "bob"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.AND, Literal: "and"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "job"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name: "long multi-line query",
			input: `recall $vec
(anna or bob or charlie)
and (topic:personal or topic:draft)
since:2024-01-01
until:2024-12-31
top:10 depth:5`,
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.DOLLAR, Literal: "$"},
				{Type: lexer.VEC, Literal: "vec"},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.OR, Literal: "or"},
				{Type: lexer.LITERAL, Literal: "bob"},
				{Type: lexer.OR, Literal: "or"},
				{Type: lexer.LITERAL, Literal: "charlie"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.AND, Literal: "and"},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "personal"},
				{Type: lexer.OR, Literal: "or"},
				{Type: lexer.TOPIC, Literal: "topic"},
				{Type: lexer.COLON, Literal: ":"},
				{Type: lexer.LITERAL, Literal: "draft"},
				{Type: lexer.RPAREN, Literal: ")"},
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
			},
		},
		{
			name:  "consecutive newlines treated as whitespace",
			input: "recall\n\n\nanna",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.EOL, Literal: ""},
			},
		},
		{
			name:  "mixed newlines and spaces",
			input: "recall  \n  anna  \n  topic:job",
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
			name:  "newlines with tabs",
			input: "recall\n\tanna\n\t\ttopic:job",
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
			name:  "newlines in complex boolean expression",
			input: "recall (\n  anna\n  or\n  bob\n) and\ntopic:job",
			expected: []lexer.Token{
				{Type: lexer.RECALL, Literal: "recall"},
				{Type: lexer.LPAREN, Literal: "("},
				{Type: lexer.LITERAL, Literal: "anna"},
				{Type: lexer.OR, Literal: "or"},
				{Type: lexer.LITERAL, Literal: "bob"},
				{Type: lexer.RPAREN, Literal: ")"},
				{Type: lexer.AND, Literal: "and"},
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
