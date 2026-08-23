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
		{"PHRASE", lexer.PHRASE, "phrase"},
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
		lexer.COLON, lexer.PHRASE, lexer.LPAREN, lexer.RPAREN, lexer.DOLLAR,
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

// TestIsKeyword pins the reserved-word set IsKeyword reports. Every type in
// KeywordsMap must answer true — the parser relies on this to read a reserved
// word as data in value position, so a keyword missing here regresses to the
// entity:top 400. Every other type must answer false; LITERAL especially,
// since calling the bare-word type itself a keyword would re-reserve every
// word.
func TestIsKeyword(t *testing.T) {
	for literal, tokenType := range lexer.KeywordsMap {
		t.Run(literal, func(t *testing.T) {
			if !tokenType.IsKeyword() {
				t.Errorf("IsKeyword(%v) = false, want true for reserved word %q", tokenType, literal)
			}
		})
	}

	nonKeywords := []lexer.TokenType{
		lexer.ILLEGAL, lexer.EOL, lexer.LITERAL, lexer.PHRASE,
		lexer.PLUS, lexer.TILDE, lexer.MINUS,
		lexer.AT, lexer.COLON, lexer.LPAREN, lexer.RPAREN, lexer.DOLLAR, lexer.NEWLINE,
	}
	for _, tokenType := range nonKeywords {
		if tokenType.IsKeyword() {
			t.Errorf("IsKeyword(%v) = true, want false", tokenType)
		}
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

// TestIsCommand pins which tokens open a query. The parser asks this to tell a
// second command apart from a stray word — "recall x recall y" is one
// instruction too many, not an unexpected token — so a field wrongly reporting
// as a command would turn a repairable typo into a misleading message.
func TestIsCommand(t *testing.T) {
	commands := []lexer.TokenType{lexer.RECALL, lexer.REMEMBER, lexer.FORGET, lexer.UPDATE}
	for _, tt := range commands {
		if !tt.IsCommand() {
			t.Errorf("%v.IsCommand() = false, want true", tt)
		}
	}

	// Every other type, including the fields that are keywords but not verbs.
	others := []lexer.TokenType{
		lexer.TOPIC, lexer.ENTITY, lexer.SINCE, lexer.UNTIL, lexer.TOP, lexer.DEPTH,
		lexer.VEC, lexer.LITERAL, lexer.PHRASE, lexer.COLON, lexer.AT, lexer.EOL,
		lexer.NEWLINE, lexer.ILLEGAL, lexer.LPAREN, lexer.RPAREN, lexer.DOLLAR,
	}
	for _, tt := range others {
		if tt.IsCommand() {
			t.Errorf("%v.IsCommand() = true, want false", tt)
		}
	}
}

// TestTokenDescribe pins how an error message names a token. The two tokens
// with no literal are the whole point: %q renders them as `""`, an empty string
// the caller never wrote and cannot act on, which is what turned "recall ferry
// top" into a message that read like a parser bug.
func TestTokenDescribe(t *testing.T) {
	cases := []struct {
		name string
		tok  lexer.Token
		want string
	}{
		{"end of input has a name, not an empty literal", lexer.Token{Type: lexer.EOL}, "end of input"},
		{"a newline has a name too", lexer.Token{Type: lexer.NEWLINE, Literal: "\n"}, "a new line"},
		{"a word is quoted", lexer.Token{Type: lexer.LITERAL, Literal: "ferry"}, `"ferry"`},
		{"a keyword is quoted like any other word", lexer.Token{Type: lexer.TOP, Literal: "top"}, `"top"`},
		{"punctuation is quoted", lexer.Token{Type: lexer.COLON, Literal: ":"}, `":"`},
		{"a quote inside the literal is escaped, not left to close the message",
			lexer.Token{Type: lexer.PHRASE, Literal: `it"s`}, `"it\"s"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tok.Describe(); got != tc.want {
				t.Errorf("Describe() = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestIsMisCasedKeyword pins the parser's view of a mis-cased keyword. The
// lexer types a keyword by spelling and forgives casing only before a ':', so
// everywhere else one arrives as an ordinary literal — and wherever a clause
// could have started, that literal is a mistake rather than a term. A correctly
// cased keyword is not mis-cased, and neither is a phrase: quoting is how a
// caller says they meant the word.
func TestIsMisCasedKeyword(t *testing.T) {
	cases := []struct {
		name string
		tok  lexer.Token
		want bool
	}{
		{"a literal spelling a keyword", lexer.Token{Type: lexer.LITERAL, Literal: "Top"}, true},
		{"shouted", lexer.Token{Type: lexer.LITERAL, Literal: "DEPTH"}, true},
		{"a command spelling counts too", lexer.Token{Type: lexer.LITERAL, Literal: "Recall"}, true},
		{"an ordinary word", lexer.Token{Type: lexer.LITERAL, Literal: "ferry"}, false},
		{"a word merely containing one", lexer.Token{Type: lexer.LITERAL, Literal: "topical"}, false},
		{"the keyword itself is not mis-cased", lexer.Token{Type: lexer.TOP, Literal: "top"}, false},
		{"a phrase is data, whatever it spells", lexer.Token{Type: lexer.PHRASE, Literal: "Top"}, false},
		{"end of input spells nothing", lexer.Token{Type: lexer.EOL}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tok.IsMisCasedKeyword(); got != tc.want {
				t.Errorf("IsMisCasedKeyword() = %v, want %v", got, tc.want)
			}
		})
	}
}
