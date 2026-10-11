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

func TestTokenTypeString(t *testing.T) {
	tests := map[string]struct {
		token lexer.TokenType
		want  string
	}{
		"ILLEGAL":  {lexer.ILLEGAL, ""},
		"EOL":      {lexer.EOL, "eol"},
		"LITERAL":  {lexer.LITERAL, "literal"},
		"EXPLAIN":  {lexer.EXPLAIN, "explain"},
		"DESCRIBE": {lexer.DESCRIBE, "describe"},
		"RECALL":   {lexer.RECALL, "recall"},
		"REMEMBER": {lexer.REMEMBER, "remember"},
		"FORGET":   {lexer.FORGET, "forget"},
		"UPDATE":   {lexer.UPDATE, "update"},
		"PLUS":     {lexer.PLUS, "+"},
		"TILDE":    {lexer.TILDE, "~"},
		"MINUS":    {lexer.MINUS, "-"},
		"COLON":    {lexer.COLON, ":"},
		"NEWLINE":  {lexer.NEWLINE, "\n"},
		"PHRASE":   {lexer.PHRASE, "phrase"},
		"LPAREN":   {lexer.LPAREN, "("},
		"RPAREN":   {lexer.RPAREN, ")"},
		"DOLLAR":   {lexer.DOLLAR, "$"},
		"TOPIC":    {lexer.TOPIC, "topic"},
		"SINCE":    {lexer.SINCE, "since"},
		"UNTIL":    {lexer.UNTIL, "until"},
		"TOP":      {lexer.TOP, "top"},
		"DEPTH":    {lexer.DEPTH, "depth"},
		"VEC":      {lexer.VEC, "vec"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.token.String(); got != tc.want {
				t.Errorf("TokenType.String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTokenMapCompleteness(t *testing.T) {
	tests := map[string]struct {
		token lexer.TokenType
	}{
		"ILLEGAL":  {lexer.ILLEGAL},
		"EOL":      {lexer.EOL},
		"LITERAL":  {lexer.LITERAL},
		"RECALL":   {lexer.RECALL},
		"REMEMBER": {lexer.REMEMBER},
		"FORGET":   {lexer.FORGET},
		"UPDATE":   {lexer.UPDATE},
		"PLUS":     {lexer.PLUS},
		"TILDE":    {lexer.TILDE},
		"MINUS":    {lexer.MINUS},
		"COLON":    {lexer.COLON},
		"PHRASE":   {lexer.PHRASE},
		"LPAREN":   {lexer.LPAREN},
		"RPAREN":   {lexer.RPAREN},
		"DOLLAR":   {lexer.DOLLAR},
		"TOPIC":    {lexer.TOPIC},
		"SINCE":    {lexer.SINCE},
		"UNTIL":    {lexer.UNTIL},
		"TOP":      {lexer.TOP},
		"DEPTH":    {lexer.DEPTH},
		"VEC":      {lexer.VEC},
		"EXPLAIN":  {lexer.EXPLAIN},
		"DESCRIBE": {lexer.DESCRIBE},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, exists := lexer.TokenMap[tc.token]
			if !exists && tc.token != lexer.ILLEGAL {
				t.Errorf("TokenMap missing entry for token type %v", tc.token)
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
	tests := map[string]struct {
		want  lexer.TokenType
		found bool
	}{
		"recall":      {lexer.RECALL, true},
		"remember":    {lexer.REMEMBER, true},
		"forget":      {lexer.FORGET, true},
		"update":      {lexer.UPDATE, true},
		"topic":       {lexer.TOPIC, true},
		"since":       {lexer.SINCE, true},
		"until":       {lexer.UNTIL, true},
		"top":         {lexer.TOP, true},
		"depth":       {lexer.DEPTH, true},
		"vec":         {lexer.VEC, true},
		"nonexistent": {lexer.ILLEGAL, false},
		"RECALL":      {lexer.ILLEGAL, false},
		"Recall":      {lexer.ILLEGAL, false},
		"and":         {lexer.ILLEGAL, false},
		"or":          {lexer.ILLEGAL, false},
		"+":           {lexer.ILLEGAL, false},
		"~":           {lexer.ILLEGAL, false},
		"-":           {lexer.ILLEGAL, false},
	}

	for keyword, tc := range tests {
		t.Run(keyword, func(t *testing.T) {
			got, found := lexer.KeywordsMap[keyword]
			if found != tc.found {
				t.Errorf("KeywordsMap[%q] found = %v, want %v", keyword, found, tc.found)
			}
			if found && got != tc.want {
				t.Errorf("KeywordsMap[%q] = %v, want %v", keyword, got, tc.want)
			}
		})
	}
}

// TestIsKeyword pins the reserved-word set IsKeyword reports. Every type in
// KeywordsMap must answer true: the parser relies on this to read a reserved
// word as data after a field's ':', so a keyword missing here would make
// entity:top a parse error. Every other type must answer false; LITERAL
// especially, since calling the bare-word type itself a keyword would
// re-reserve every word.
func TestIsKeyword(t *testing.T) {
	for literal, tokenType := range lexer.KeywordsMap {
		t.Run(literal, func(t *testing.T) {
			if !tokenType.IsKeyword() {
				t.Errorf("IsKeyword(%v) = false, want true for reserved word %q", tokenType, literal)
			}
		})
	}

	nonKeywords := map[string]struct {
		token lexer.TokenType
	}{
		"ILLEGAL": {lexer.ILLEGAL},
		"EOL":     {lexer.EOL},
		"LITERAL": {lexer.LITERAL},
		"PHRASE":  {lexer.PHRASE},
		"PLUS":    {lexer.PLUS},
		"TILDE":   {lexer.TILDE},
		"MINUS":   {lexer.MINUS},
		"AT":      {lexer.AT},
		"COLON":   {lexer.COLON},
		"LPAREN":  {lexer.LPAREN},
		"RPAREN":  {lexer.RPAREN},
		"DOLLAR":  {lexer.DOLLAR},
		"NEWLINE": {lexer.NEWLINE},
	}
	for name, tc := range nonKeywords {
		t.Run(name, func(t *testing.T) {
			if tc.token.IsKeyword() {
				t.Errorf("IsKeyword(%v) = true, want false", tc.token)
			}
		})
	}
}

func TestKeyLITERALsMapCaseSensitivity(t *testing.T) {
	tests := map[string]struct{}{
		"RECALL":   {},
		"Recall":   {},
		"ReCaLl":   {},
		"REMEMBER": {},
		"Update":   {},
	}

	for keyword := range tests {
		t.Run(keyword, func(t *testing.T) {
			if _, found := lexer.KeywordsMap[keyword]; found {
				t.Errorf("KeywordsMap should be case-sensitive, but found %q", keyword)
			}
		})
	}
}

func TestTokenCreation(t *testing.T) {
	tests := map[string]struct {
		tokenType lexer.TokenType
		literal   string
	}{
		"Command token": {lexer.RECALL, "recall"},
		"LITERAL token": {lexer.LITERAL, "example"},
		"Empty literal": {lexer.LITERAL, ""},
		"Special chars": {lexer.LITERAL, "hello-world_123"},
		"Punctuation":   {lexer.COLON, ":"},
		"Anchor":        {lexer.PLUS, "+"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := lexer.Token{
				Type:    tc.tokenType,
				Literal: tc.literal,
			}
			if got.Type != tc.tokenType {
				t.Errorf("Token.Type = %v, want %v", got.Type, tc.tokenType)
			}
			if got.Literal != tc.literal {
				t.Errorf("Token.Literal = %q, want %q", got.Literal, tc.literal)
			}
		})
	}
}

func TestTokenZeroValue(t *testing.T) {
	var got lexer.Token
	if want := lexer.ILLEGAL; got.Type != want {
		t.Errorf("zero value Token.Type = %v, want %v (ILLEGAL)", got.Type, want)
	}
	if got.Literal != "" {
		t.Errorf("zero value Token.Literal = %q, want empty string", got.Literal)
	}
}

func TestIllegalTokenIsZero(t *testing.T) {
	if got := lexer.ILLEGAL; got != 0 {
		t.Errorf("ILLEGAL = %v, want 0 (the iota zero value)", got)
	}
}

func TestTokenTypeOutOfRange(t *testing.T) {
	got := lexer.TokenType(9999).String()
	if got != "" {
		t.Logf("Out of range TokenType.String() = %q (expected empty string)", got)
	}
}

func TestKeyLITERALsMapAndTokenMapConsistency(t *testing.T) {
	for keyword, tokenType := range lexer.KeywordsMap {
		t.Run(keyword, func(t *testing.T) {
			got, exists := lexer.TokenMap[tokenType]
			if !exists {
				t.Errorf("KeywordsMap[%q] = %v, but TokenMap has no entry for %v", keyword, tokenType, tokenType)
				return
			}
			if got != keyword {
				t.Errorf("Inconsistency: KeywordsMap[%q] = %v, but TokenMap[%v] = %q",
					keyword, tokenType, tokenType, got)
			}
		})
	}
}

func TestRoundTripTokenTypeToStringToKeyLITERAL(t *testing.T) {
	tests := map[string]struct {
		token lexer.TokenType
	}{
		"recall":   {lexer.RECALL},
		"remember": {lexer.REMEMBER},
		"forget":   {lexer.FORGET},
		"update":   {lexer.UPDATE},
		"topic":    {lexer.TOPIC},
		"since":    {lexer.SINCE},
		"until":    {lexer.UNTIL},
		"top":      {lexer.TOP},
		"depth":    {lexer.DEPTH},
		"vec":      {lexer.VEC},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			literal := tc.token.String()
			if literal == "" {
				t.Fatalf("TokenType %v has empty string representation", tc.token)
			}

			got, found := lexer.KeywordsMap[literal]
			if !found {
				t.Errorf("Round trip failed: %v → %q, but %q not in KeywordsMap",
					tc.token, literal, literal)
				return
			}

			if got != tc.token {
				t.Errorf("Round trip failed: %v → %q → %v", tc.token, literal, got)
			}
		})
	}
}

// TestIsCommand pins which tokens are command verbs. The parser asks this to
// report a command word after the first position as a second command or a word
// to quote, so a field wrongly reporting as a command would turn a repairable
// typo into a misleading message.
func TestIsCommand(t *testing.T) {
	commands := map[string]struct {
		token lexer.TokenType
	}{
		"RECALL":   {lexer.RECALL},
		"REMEMBER": {lexer.REMEMBER},
		"FORGET":   {lexer.FORGET},
		"UPDATE":   {lexer.UPDATE},
	}
	for name, tc := range commands {
		t.Run(name, func(t *testing.T) {
			if !tc.token.IsCommand() {
				t.Errorf("%v.IsCommand() = false, want true", tc.token)
			}
		})
	}

	// Every other type, including the fields that are keywords but not verbs.
	others := map[string]struct {
		token lexer.TokenType
	}{
		"TOPIC":    {lexer.TOPIC},
		"ENTITY":   {lexer.ENTITY},
		"SINCE":    {lexer.SINCE},
		"UNTIL":    {lexer.UNTIL},
		"TOP":      {lexer.TOP},
		"DEPTH":    {lexer.DEPTH},
		"VEC":      {lexer.VEC},
		"LITERAL":  {lexer.LITERAL},
		"PHRASE":   {lexer.PHRASE},
		"COLON":    {lexer.COLON},
		"AT":       {lexer.AT},
		"EOL":      {lexer.EOL},
		"NEWLINE":  {lexer.NEWLINE},
		"ILLEGAL":  {lexer.ILLEGAL},
		"LPAREN":   {lexer.LPAREN},
		"RPAREN":   {lexer.RPAREN},
		"DOLLAR":   {lexer.DOLLAR},
		"EXPLAIN":  {lexer.EXPLAIN},
		"DESCRIBE": {lexer.DESCRIBE},
	}
	for name, tc := range others {
		t.Run(name, func(t *testing.T) {
			if tc.token.IsCommand() {
				t.Errorf("%v.IsCommand() = true, want false", tc.token)
			}
		})
	}
}

// TestIsPrefix pins which tokens stand in front of a command. The parser asks
// this to keep a clause repair out of a prefix's message, so a field wrongly
// reporting as a prefix would lose its filter repair, and a prefix wrongly
// reporting as a field would be offered explain:<value>, itself an error.
func TestIsPrefix(t *testing.T) {
	prefixes := map[string]struct {
		token lexer.TokenType
	}{
		"EXPLAIN":  {lexer.EXPLAIN},
		"DESCRIBE": {lexer.DESCRIBE},
	}
	for name, tc := range prefixes {
		t.Run(name, func(t *testing.T) {
			if !tc.token.IsPrefix() {
				t.Errorf("%v.IsPrefix() = false, want true", tc.token)
			}
		})
	}

	// Every other type, including the commands and fields that are keywords.
	others := map[string]struct {
		token lexer.TokenType
	}{
		"RECALL":   {lexer.RECALL},
		"REMEMBER": {lexer.REMEMBER},
		"FORGET":   {lexer.FORGET},
		"UPDATE":   {lexer.UPDATE},
		"TOPIC":    {lexer.TOPIC},
		"ENTITY":   {lexer.ENTITY},
		"SINCE":    {lexer.SINCE},
		"UNTIL":    {lexer.UNTIL},
		"TOP":      {lexer.TOP},
		"DEPTH":    {lexer.DEPTH},
		"VEC":      {lexer.VEC},
		"LITERAL":  {lexer.LITERAL},
		"PHRASE":   {lexer.PHRASE},
		"COLON":    {lexer.COLON},
		"AT":       {lexer.AT},
		"EOL":      {lexer.EOL},
		"NEWLINE":  {lexer.NEWLINE},
		"ILLEGAL":  {lexer.ILLEGAL},
		"LPAREN":   {lexer.LPAREN},
		"RPAREN":   {lexer.RPAREN},
		"DOLLAR":   {lexer.DOLLAR},
	}
	for name, tc := range others {
		t.Run(name, func(t *testing.T) {
			if tc.token.IsPrefix() {
				t.Errorf("%v.IsPrefix() = true, want false", tc.token)
			}
		})
	}
}

// TestTokenDescribe pins how an error message names a token. The two tokens
// with no literal are the whole point: %q renders them as `""`, an empty string
// the caller never wrote and cannot act on, which is what turned "recall ferry
// top" into a message that read like a parser bug.
func TestTokenDescribe(t *testing.T) {
	cases := map[string]struct {
		tok  lexer.Token
		want string
	}{
		"end of input has a name, not an empty literal": {lexer.Token{Type: lexer.EOL}, "end of input"},
		"a newline has a name too":                      {lexer.Token{Type: lexer.NEWLINE, Literal: "\n"}, "a new line"},
		"a word is quoted":                              {lexer.Token{Type: lexer.LITERAL, Literal: "ferry"}, `"ferry"`},
		"a keyword is quoted like any other word":       {lexer.Token{Type: lexer.TOP, Literal: "top"}, `"top"`},
		"punctuation is quoted":                         {lexer.Token{Type: lexer.COLON, Literal: ":"}, `":"`},
		"a quote inside the literal is escaped, not left to close the message": {
			lexer.Token{Type: lexer.PHRASE, Literal: `it"s`}, `"it\"s"`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
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
	cases := map[string]struct {
		tok  lexer.Token
		want bool
	}{
		"a literal spelling a keyword":         {lexer.Token{Type: lexer.LITERAL, Literal: "Top"}, true},
		"shouted":                              {lexer.Token{Type: lexer.LITERAL, Literal: "DEPTH"}, true},
		"a command spelling counts too":        {lexer.Token{Type: lexer.LITERAL, Literal: "Recall"}, true},
		"an ordinary word":                     {lexer.Token{Type: lexer.LITERAL, Literal: "ferry"}, false},
		"a word merely containing one":         {lexer.Token{Type: lexer.LITERAL, Literal: "topical"}, false},
		"the keyword itself is not mis-cased":  {lexer.Token{Type: lexer.TOP, Literal: "top"}, false},
		"a phrase is data, whatever it spells": {lexer.Token{Type: lexer.PHRASE, Literal: "Top"}, false},
		"end of input spells nothing":          {lexer.Token{Type: lexer.EOL}, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.tok.IsMisCasedKeyword(); got != tc.want {
				t.Errorf("IsMisCasedKeyword() = %v, want %v", got, tc.want)
			}
		})
	}
}
