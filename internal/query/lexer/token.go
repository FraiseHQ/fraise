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

package lexer

import (
	"strconv"
	"strings"
)

// Token is one token of a query, as the lexer returns it.
type Token struct {
	Type    TokenType
	Literal string
	// Pos is the 1-based column of the token's last character, for every token
	// type (a phrase's closing quote; for EOL, the end of input), and is where
	// a parse error blaming the token is reported. The parser cannot use the
	// lexer's CurrentPos instead: its one-token lookahead has already moved it
	// past the following token.
	Pos Position
}

type TokenType int

const (
	ILLEGAL TokenType = iota

	// literal
	LITERAL
	// PHRASE is an opaque single-quoted string, scanned verbatim: reserved
	// words and symbols inside it carry no special meaning, and a doubled
	// quote ('') is an escaped literal quote.
	PHRASE

	// prefixes
	DESCRIBE
	EXPLAIN

	// commands
	RECALL
	REMEMBER
	FORGET
	UPDATE

	// operators
	PLUS
	TILDE
	MINUS

	// punctuation
	LPAREN
	RPAREN
	COMMA

	// separators: '@' before a graph selector, ':' between a clause keyword
	// and its value, '$' before a parameter name
	AT
	COLON
	DOLLAR

	// blank characters
	WHITESPACE
	// NUL is a NUL character outside a phrase. It is its own token, rather
	// than read as the end of input, so the rest of the query cannot be
	// silently dropped after it.
	NUL
	// SPECIAL is any other character outside a phrase that is not a letter, a
	// digit, whitespace or punctuation. It is its own token so a word ends at
	// it and the parser can name it; absorbed into a word, "ferry;" would be
	// a search term.
	SPECIAL

	// end of input, and the newline that ends an instruction
	EOL
	NEWLINE

	// fields
	TOPIC
	ENTITY
	SINCE
	UNTIL
	TOP
	DEPTH

	// vector field, whose value is a parameter reference ($name)
	VEC
)

var TokenMap = map[TokenType]string{
	DESCRIBE:   "describe",
	EXPLAIN:    "explain",
	RECALL:     "recall",
	REMEMBER:   "remember",
	FORGET:     "forget",
	UPDATE:     "update",
	COLON:      ":",
	LPAREN:     "(",
	RPAREN:     ")",
	DOLLAR:     "$",
	PHRASE:     "phrase",
	NEWLINE:    "\n",
	NUL:        "\x00",
	PLUS:       "+",
	TILDE:      "~",
	MINUS:      "-",
	TOPIC:      "topic",
	ENTITY:     "entity",
	SINCE:      "since",
	UNTIL:      "until",
	TOP:        "top",
	DEPTH:      "depth",
	LITERAL:    "literal",
	VEC:        "vec",
	EOL:        "eol",
	AT:         "@",
	COMMA:      "'",
	WHITESPACE: "whitespace",
}

var KeywordsMap = map[string]TokenType{
	"recall":     RECALL,
	"remember":   REMEMBER,
	"forget":     FORGET,
	"update":     UPDATE,
	"explain":    EXPLAIN,
	"describe":   DESCRIBE,
	"topic":      TOPIC,
	"entity":     ENTITY,
	"since":      SINCE,
	"until":      UNTIL,
	"top":        TOP,
	"depth":      DEPTH,
	"vec":        VEC,
}

// IsKeyword reports whether t is a reserved word: a type the lexer assigns by
// spelling alone. Whether the word is syntax depends on where it stands, which
// the parser decides: after a field's ':' it is data (entity:top names the
// anchor "top"), while among a recall's terms it is an error.
func (t TokenType) IsKeyword() bool {
	switch t {
	case RECALL, REMEMBER, FORGET, UPDATE, TOPIC, ENTITY, SINCE, UNTIL, TOP, DEPTH, VEC, EXPLAIN, DESCRIBE:
		return true
	default:
		return false
	}
}

// IsCommand reports whether t is a command verb: recall, remember, forget or
// update. A query is one instruction, so the parser reports a command word
// after the first position as a second command or a word to quote, never as a
// stray token or the start of a clause.
func (t TokenType) IsCommand() bool {
	switch t {
	case RECALL, REMEMBER, FORGET, UPDATE:
		return true
	default:
		return false
	}
}

// IsPrefix reports whether t is one of the words that stand in front of a
// command. A prefix is reserved like a command and has no ':' form either, so
// the parser asks this to keep a clause repair out of its message: offering
// explain:<value> sent the caller to a query that is itself an error.
func (t TokenType) IsPrefix() bool {
	switch t {
	case EXPLAIN, DESCRIBE:
		return true
	default:
		return false
	}
}

func (t TokenType) String() string {
	return TokenMap[t]
}

// Describe names the token as an error message should: end of input and a
// newline by name, anything else quoted. End of input has no literal, and
// quoted it would read as `""`, a string the caller never wrote. Naming tokens
// here keeps error messages agreeing on what to call one, as Pos keeps them
// agreeing on where it is.
func (t Token) Describe() string {
	switch t.Type {
	case EOL:
		return "end of input"
	case NEWLINE:
		return "a new line"
	default:
		return strconv.Quote(t.Literal)
	}
}

// IsBlank reports whether t is a WHITESPACE or NUL token.
func (t TokenType) IsBlank() bool {
	switch t {
	case WHITESPACE, NUL:
		return true
	default:
		return false
	}
}

// IsEndOfLine reports whether t is end of input or a newline.
func (t TokenType) IsEndOfLine() bool {
	switch t {
	case EOL, NEWLINE:
		return true
	default:
		return false
	}
}

// IsSeparator reports whether t is '@', ':' or '$'.
func (t TokenType) IsSeparator() bool {
	switch t {
	case AT, COLON, DOLLAR:
		return true
	default:
		return false
	}
}

// IsMisCasedKeyword reports whether the token is a bare word spelling a reserved
// word in the wrong case. The lexer types a keyword by spelling and forgives
// casing only immediately before a ':' (see Lexer.Next), so everywhere else a
// mis-cased keyword arrives as a LITERAL, which the parser rejects wherever a
// clause could start.
func (t Token) IsMisCasedKeyword() bool {
	if t.Type != LITERAL {
		return false
	}
	_, reserved := KeywordsMap[strings.ToLower(t.Literal)]
	return reserved
}
