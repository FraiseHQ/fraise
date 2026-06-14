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

func TestTokenTypeString(t *testing.T) {
	tests := []struct {
		name     string
		token    lexer.TokenType
		expected string
	}{
		{"ILLEGAL", lexer.ILLEGAL, ""},
		{"EOL", lexer.EOL, "eol"},
		{"LITERAL", lexer.LITERAL, "literal"},
		{"RECALL", lexer.RECALL, "recall"},
		{"REMEMBER", lexer.REMEMBER, "remember"},
		{"FORGET", lexer.FORGET, "forget"},
		{"UPDATE", lexer.UPDATE, "update"},
		{"PLUS", lexer.PLUS, "+"},
		{"TILDE", lexer.TILDE, "~"},
		{"MINUS", lexer.MINUS, "-"},
		{"COLON", lexer.COLON, ":"},
		{"NEWLINE", lexer.NEWLINE, "\n"},
		{"COMMA", lexer.COMMA, "'"},
		{"LPAREN", lexer.LPAREN, "("},
		{"RPAREN", lexer.RPAREN, ")"},
		{"DOLLAR", lexer.DOLLAR, "$"},
		{"TOPIC", lexer.TOPIC, "topic"},
		{"SINCE", lexer.SINCE, "since"},
		{"UNTIL", lexer.UNTIL, "until"},
		{"TOP", lexer.TOP, "top"},
		{"DEPTH", lexer.DEPTH, "depth"},
		{"VEC", lexer.VEC, "vec"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.token.String()
			if result != tt.expected {
				t.Errorf("TokenType.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTokenMapCompleteness(t *testing.T) {
	allTokenTypes := []lexer.TokenType{
		lexer.ILLEGAL, lexer.EOL, lexer.LITERAL,
		lexer.RECALL, lexer.REMEMBER, lexer.FORGET, lexer.UPDATE,
		lexer.PLUS, lexer.TILDE, lexer.MINUS,
		lexer.COLON, lexer.COMMA, lexer.LPAREN, lexer.RPAREN, lexer.DOLLAR,
		lexer.TOPIC, lexer.SINCE, lexer.UNTIL, lexer.TOP, lexer.DEPTH,
		lexer.VEC,
	}

	for _, tokenType := range allTokenTypes {
		t.Run(tokenType.String(), func(t *testing.T) {
			_, exists := lexer.TokenMap[tokenType]
			if !exists && tokenType != lexer.ILLEGAL {
				t.Errorf("TokenMap missing entry for token type %v", tokenType)
			}
		})
	}
}

func TestTokenMapNoDuplicates(t *testing.T) {
	seen := make(map[string]lexer.TokenType)
	for tokenType, literal := range lexer.TokenMap {
		if literal == "" {
			continue
		}
		if existing, exists := seen[literal]; exists {
			t.Errorf("Duplicate literal %q maps to both %v and %v", literal, existing, tokenType)
		}
		seen[literal] = tokenType
	}
}

func TestKeyLITERALsMapLookup(t *testing.T) {
	tests := []struct {
		keyLITERAL string
		expected   lexer.TokenType
		found      bool
	}{
		{"recall", lexer.RECALL, true},
		{"remember", lexer.REMEMBER, true},
		{"forget", lexer.FORGET, true},
		{"update", lexer.UPDATE, true},
		{"topic", lexer.TOPIC, true},
		{"since", lexer.SINCE, true},
		{"until", lexer.UNTIL, true},
		{"top", lexer.TOP, true},
		{"depth", lexer.DEPTH, true},
		{"vec", lexer.VEC, true},
		{"nonexistent", lexer.ILLEGAL, false},
		{"RECALL", lexer.ILLEGAL, false},
		{"Recall", lexer.ILLEGAL, false},
		{"and", lexer.ILLEGAL, false},
		{"or", lexer.ILLEGAL, false},
		{"+", lexer.ILLEGAL, false},
		{"~", lexer.ILLEGAL, false},
		{"-", lexer.ILLEGAL, false},
	}

	for _, tt := range tests {
		t.Run(tt.keyLITERAL, func(t *testing.T) {
			result, found := lexer.KeywordsMap[tt.keyLITERAL]
			if found != tt.found {
				t.Errorf("KeyLITERALsMap[%q] found = %v, want %v", tt.keyLITERAL, found, tt.found)
			}
			if found && result != tt.expected {
				t.Errorf("KeyLITERALsMap[%q] = %v, want %v", tt.keyLITERAL, result, tt.expected)
			}
		})
	}
}

func TestKeyLITERALsMapCaseSensitivity(t *testing.T) {
	caseSensitiveTests := []string{"RECALL", "Recall", "ReCaLl", "REMEMBER", "Update"}

	for _, keyLITERAL := range caseSensitiveTests {
		t.Run(keyLITERAL, func(t *testing.T) {
			_, found := lexer.KeywordsMap[keyLITERAL]
			if found {
				t.Errorf("KeyLITERALsMap should be case-sensitive, but found %q", keyLITERAL)
			}
		})
	}
}

func TestTokenCreation(t *testing.T) {
	tests := []struct {
		name      string
		tokenType lexer.TokenType
		literal   string
	}{
		{"Command token", lexer.RECALL, "recall"},
		{"LITERAL token", lexer.LITERAL, "example"},
		{"Empty literal", lexer.LITERAL, ""},
		{"Special chars", lexer.LITERAL, "hello-world_123"},
		{"Punctuation", lexer.COLON, ":"},
		{"Anchor", lexer.PLUS, "+"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := lexer.Token{
				Type:    tt.tokenType,
				Literal: tt.literal,
			}
			if token.Type != tt.tokenType {
				t.Errorf("Token.Type = %v, want %v", token.Type, tt.tokenType)
			}
			if token.Literal != tt.literal {
				t.Errorf("Token.Literal = %q, want %q", token.Literal, tt.literal)
			}
		})
	}
}

func TestTokenZeroValue(t *testing.T) {
	var token lexer.Token
	if token.Type != lexer.ILLEGAL {
		t.Errorf("Zero value Token.Type = %v, want %v (ILLEGAL)", token.Type, lexer.ILLEGAL)
	}
	if token.Literal != "" {
		t.Errorf("Zero value Token.Literal = %q, want empty string", token.Literal)
	}
}

func TestIllegalTokenIsZero(t *testing.T) {
	if lexer.ILLEGAL != 0 {
		t.Errorf("ILLEGAL token should be 0 (iota zero value), got %v", lexer.ILLEGAL)
	}
}

func TestTokenTypeOutOfRange(t *testing.T) {
	outOfRangeToken := lexer.TokenType(9999)
	result := outOfRangeToken.String()
	if result != "" {
		t.Logf("Out of range TokenType.String() = %q (expected empty string)", result)
	}
}

func TestKeyLITERALsMapAndTokenMapConsistency(t *testing.T) {
	for keyLITERAL, tokenType := range lexer.KeywordsMap {
		t.Run(keyLITERAL, func(t *testing.T) {
			mappedLiteral, exists := lexer.TokenMap[tokenType]
			if !exists {
				t.Errorf("KeyLITERALsMap[%q] = %v, but TokenMap has no entry for %v", keyLITERAL, tokenType, tokenType)
				return
			}
			if mappedLiteral != keyLITERAL {
				t.Errorf("Inconsistency: KeyLITERALsMap[%q] = %v, but TokenMap[%v] = %q",
					keyLITERAL, tokenType, tokenType, mappedLiteral)
			}
		})
	}
}

func TestRoundTripTokenTypeToStringToKeyLITERAL(t *testing.T) {
	keyLITERALTokens := []lexer.TokenType{
		lexer.RECALL, lexer.REMEMBER, lexer.FORGET, lexer.UPDATE,
		lexer.TOPIC, lexer.SINCE, lexer.UNTIL, lexer.TOP, lexer.DEPTH,
		lexer.VEC,
	}

	for _, original := range keyLITERALTokens {
		t.Run(original.String(), func(t *testing.T) {
			literal := original.String()
			if literal == "" {
				t.Fatalf("TokenType %v has empty string representation", original)
			}

			lookedUp, found := lexer.KeywordsMap[literal]
			if !found {
				t.Errorf("Round trip failed: %v → %q, but %q not in KeyLITERALsMap",
					original, literal, literal)
				return
			}

			if lookedUp != original {
				t.Errorf("Round trip failed: %v → %q → %v", original, literal, lookedUp)
			}
		})
	}
}
