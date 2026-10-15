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
	"strings"
	"unicode"
)

// Position locates a token in the query. Column counts runes, not bytes, so a
// position points at the right character of a query with non-ASCII text.
type Position struct {
	Column int
}

// Lexer scans one query into tokens on demand. Input holds the query as runes,
// so a multi-byte character is one column; Character is the last rune
// consumed and CurrentPos its 1-based column, which is also the index of the
// next rune to read.
type Lexer struct {
	Input      []rune
	Character  rune
	CurrentPos Position
	NextPos    Position
}

// New returns a Lexer over input.
func New(input string) *Lexer {
	l := Lexer{
		Input: []rune(input),
	}
	l.readCharacter()
	// initialise positions
	l.CurrentPos = Position{
		Column: 0,
	}
	l.NextPos = Position{
		Column: 1,
	}
	return &l
}

// SkipBlank advances past spaces, tabs and carriage returns without emitting a
// token. It stops at a newline, which ends an instruction rather than
// separating words.
func (l *Lexer) SkipBlank() {
	for isBlank(l.peek()) {
		l.readCharacter()
	}
}

func (l *Lexer) readCharacter() {
	if l.CurrentPos.Column >= len(l.Input) {
		l.Character = rune(0)
	} else {
		l.Character = l.Input[l.CurrentPos.Column]
	}
	l.CurrentPos = l.NextPos
	l.NextPos.Column++
}

// isBlank reports whether ch is a space, tab or carriage return. A newline is
// not blank: it ends an instruction, so "recall ferry\nbridge" is two lines
// rather than one two-term recall.
func isBlank(ch rune) bool {
	return ch == rune(' ') || ch == rune('\t') || ch == rune('\r')
}

// isWordCharacter reports whether ch can appear in a bare word: a letter, in any
// script, or a digit. Every other character outside a phrase is syntax or
// special, so a word that needs one — "e-mail", "2026-01-15" — is quoted.
func isWordCharacter(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

// Next scans and returns the next token, and an EOL token on every call once
// the input is exhausted. A phrase whose closing quote is missing comes back
// as an ILLEGAL token carrying the text read, so the parser can report it as
// unterminated.
func (l *Lexer) Next() Token {
	var tok Token

	switch l.peek() {
	case rune(':'):
		l.readCharacter()
		tok = Token{Type: COLON, Literal: string(l.Character)}
	case rune('\''):
		tok = l.scanPhrase()
	case rune('('):
		l.readCharacter()
		tok = Token{Type: LPAREN, Literal: string(l.Character)}
	case rune(')'):
		l.readCharacter()
		tok = Token{Type: RPAREN, Literal: string(l.Character)}
	case rune('$'):
		l.readCharacter()
		tok = Token{Type: DOLLAR, Literal: string(l.Character)}
	case rune('+'):
		l.readCharacter()
		tok = Token{Type: PLUS, Literal: string(l.Character)}
	case rune('-'):
		l.readCharacter()
		tok = Token{Type: MINUS, Literal: string(l.Character)}
	case rune('~'):
		l.readCharacter()
		tok = Token{Type: TILDE, Literal: string(l.Character)}
	case rune('@'):
		l.readCharacter()
		tok = Token{Type: AT, Literal: string(l.Character)}
	case rune('\n'):
		l.readCharacter()
		tok = Token{Type: NEWLINE, Literal: string(l.Character)}
	case rune(' '), rune('\t'), rune('\r'):
		literal := l.scanWhitespace()
		tok = Token{Type: WHITESPACE, Literal: literal}
	case rune(0):
		// peek returns 0 both past the end and at a NUL in the input; only the
		// first is the end. A NUL is its own token, so the parser rejects it
		// instead of dropping the rest of the query.
		if l.CurrentPos.Column < len(l.Input) {
			l.readCharacter()
			tok = Token{Type: NUL, Literal: string(l.Character)}
		} else {
			tok = Token{Type: EOL}
		}
	default:
		if !isWordCharacter(l.peek()) {
			l.readCharacter()
			tok = Token{Type: SPECIAL, Literal: string(l.Character)}
			break
		}
		tokLiteral := l.scanString()
		tokType, reserved := KeywordsMap[tokLiteral]
		if !reserved {
			// A mis-cased keyword lexes as a LITERAL, except immediately
			// before a ':': no production puts a bare word in front of a colon,
			// so "DEPTH:5" can only be the depth clause, and lexing it as one
			// lets a repeated clause be reported as a duplicate rather than as
			// a casing mistake. Away from a ':' the spelling must match, so
			// "RECALL x" is not a command.
			if l.peek() == rune(':') {
				tokType, reserved = KeywordsMap[strings.ToLower(tokLiteral)]
			}
			if !reserved {
				tokType = LITERAL
			}
		}
		tok = Token{Type: tokType, Literal: tokLiteral}
	}
	tok.Pos = l.CurrentPos
	return tok
}

// peek returns the character at the current position, or 0 past the end of
// input.
func (l *Lexer) peek() rune {
	if l.CurrentPos.Column >= len(l.Input) {
		return rune(0)
	}
	return l.Input[l.CurrentPos.Column]
}

func (l *Lexer) scanWhitespace() string {
	var res []rune
	for isBlank(l.peek()) {
		res = append(res, l.peek())
		l.readCharacter()
	}
	return string(res)
}

// scanString scans a bare word: the run of word characters from the current
// position.
func (l *Lexer) scanString() string {
	var res []rune
	for isWordCharacter(l.peek()) {
		res = append(res, l.peek())
		l.readCharacter()
	}
	return string(res)
}

// scanPhrase reads an opaque single-quoted phrase: every character between the
// quotes is data, reserved words and symbols included, so a fact is stored
// verbatim. A doubled single quote is an escaped quote; the first single quote
// that is not doubled closes the phrase.
//
// The opening quote is at the current position. On success a PHRASE token with
// the decoded (unquoted, unescaped) text is returned. If the input ends before
// a closing quote, an ILLEGAL token carrying the partial text is returned so the
// parser can report an unterminated phrase.
//
// End of input is detected by position, not by peek returning 0, because a NUL
// inside a phrase (JSON can carry one as \u0000) is data like any other
// character.
func (l *Lexer) scanPhrase() Token {
	l.readCharacter() // consume the opening quote

	var res []rune
	for {
		if l.CurrentPos.Column >= len(l.Input) {
			return Token{Type: ILLEGAL, Literal: string(res)}
		}
		switch l.peek() {
		case rune('\''):
			l.readCharacter() // consume the quote
			if l.peek() == rune('\'') {
				// doubled quote -> one literal quote, keep scanning
				res = append(res, rune('\''))
				l.readCharacter()
				continue
			}
			// a lone quote closes the phrase
			return Token{Type: PHRASE, Literal: string(res)}
		default:
			res = append(res, l.peek())
			l.readCharacter()
		}
	}
}
