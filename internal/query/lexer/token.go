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

// Represents tokens returned by the lexer
type Token struct {
	Type    TokenType
	Literal string
	// Pos is where a parse error blaming this token is reported: the 1-based
	// column of the token's *last* character, for every token type (a phrase's
	// closing quote, EOL's end of input). The parser cannot derive this from
	// the lexer's CurrentPos, because its one-token lookahead has already moved
	// CurrentPos past the following token — an error quoting one word would
	// point at the next one.
	Pos Position
}

type TokenType int

const (
	ILLEGAL TokenType = iota
	EOL

	// literal
	LITERAL
	// PHRASE is an opaque single-quoted string, scanned verbatim: reserved
	// words and symbols inside it carry no special meaning, and a doubled
	// quote ('') is an escaped literal quote.
	PHRASE

	// commands
	RECALL
	REMEMBER
	FORGET
	UPDATE

	// anchors
	PLUS
	TILDE
	MINUS

	// punctuation
	AT
	COLON
	LPAREN
	RPAREN
	DOLLAR
	NEWLINE

	// fields
	TOPIC
	ENTITY
	SINCE
	UNTIL
	TOP
	DEPTH

	// param ref
	VEC
)

var TokenMap = map[TokenType]string{
	RECALL:   "recall",
	REMEMBER: "remember",
	FORGET:   "forget",
	UPDATE:   "update",
	COLON:    ":",
	LPAREN:   "(",
	RPAREN:   ")",
	DOLLAR:   "$",
	PHRASE:   "phrase",
	NEWLINE:  "\n",
	PLUS:     "+",
	TILDE:    "~",
	MINUS:    "-",
	TOPIC:    "topic",
	ENTITY:   "entity",
	SINCE:    "since",
	UNTIL:    "until",
	TOP:      "top",
	DEPTH:    "depth",
	LITERAL:  "literal",
	VEC:      "vec",
	EOL:      "eol",
	AT:       "@",
}

var KeywordsMap = map[string]TokenType{
	"recall":   RECALL,
	"remember": REMEMBER,
	"forget":   FORGET,
	"update":   UPDATE,
	"topic":    TOPIC,
	"entity":   ENTITY,
	"since":    SINCE,
	"until":    UNTIL,
	"top":      TOP,
	"depth":    DEPTH,
	"vec":      VEC,
}

// IsKeyword reports whether t is a reserved word — a type the lexer assigns by
// spelling alone. Spelling alone must not make a word syntax: the parser asks
// this in value position (the right-hand side of a field's ':', the leading
// term of a recall) to read a reserved word back as ordinary data, so a stored
// word that happens to be "top" or "entity" needs no quoting there.
func (t TokenType) IsKeyword() bool {
	switch t {
	case RECALL, REMEMBER, FORGET, UPDATE, TOPIC, ENTITY, SINCE, UNTIL, TOP, DEPTH, VEC:
		return true
	default:
		return false
	}
}

// IsCommand reports whether t is one of the verbs a query can open with. A
// query is one instruction, so a command token anywhere but the first position
// is a second command rather than a stray word — the parser asks this to say so
// instead of blaming the token, which is the only form of the message a caller
// can act on.
func (t TokenType) IsCommand() bool {
	switch t {
	case RECALL, REMEMBER, FORGET, UPDATE:
		return true
	default:
		return false
	}
}

func (t TokenType) String() string {
	return TokenMap[t]
}

// Describe renders the token as an error message should name it. End of input
// carries no literal, and quoting it produces `""` — an empty string the caller
// never wrote and cannot act on, which turns a repairable mistake ("recall
// ferry top") into what reads like a parser bug. Naming it here rather than at
// each message keeps every error agreeing on what to call a token, the same way
// Pos keeps them agreeing on where one is.
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

// IsMisCasedKeyword reports whether the token is a bare word spelling a reserved
// word in the wrong case. The lexer types a keyword by spelling, and forgives
// casing only immediately before a ':' (see its keyword lookup), so everywhere
// else a mis-cased keyword arrives as an ordinary literal — and wherever a
// clause could have started, that literal is a mistake rather than a term.
func (t Token) IsMisCasedKeyword() bool {
	if t.Type != LITERAL {
		return false
	}
	_, reserved := KeywordsMap[strings.ToLower(t.Literal)]
	return reserved
}
