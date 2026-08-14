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

type Position struct {
	Column int
}

type Lexer struct {
	Input      []rune
	Offset     int
	Character  rune
	CurrentPos Position
	NextPos    Position
}

// Returns a new Lexer pointer from a string
func New(input string) *Lexer {
	l := Lexer{
		Input: []rune(input),
	}
	l.readCharacter()
	return &l
}

func (l *Lexer) skipBlank() {
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

// checks if rune is white space
func isBlank(ch rune) bool {
	return ch == rune(' ') || ch == rune('\t') || ch == rune('\r') || ch == rune('\n')
}

func (l *Lexer) Next() Token {
	var tok Token

	l.skipBlank()

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
	case rune(0):
		tok = Token{Type: EOL}
	default:
		tokLiteral := l.scanString()
		tokType, err := KeywordsMap[tokLiteral]
		if !err {
			tokType = LITERAL
		}
		tok = Token{Type: tokType, Literal: tokLiteral}
	}
	tok.Pos = l.CurrentPos
	return tok
}

// peeks current character
func (l *Lexer) peek() rune {
	if l.CurrentPos.Column >= len(l.Input) {
		return rune(0)
	}
	return l.Input[l.CurrentPos.Column]
}

// scans a string
func (l *Lexer) scanString() string {
	var res []rune
f:
	for {
		switch l.peek() {
		case rune(':'), rune('$'), rune('\''), rune('('), rune(')'), rune(' '), rune('\t'), rune('\r'), rune('\n'), rune(0), rune('@'):
			break f
		default:
			res = append(res, l.peek())
		}
		l.readCharacter()
	}
	return string(res)
}

// scanPhrase reads an opaque single-quoted phrase: every character between the
// quotes is taken literally — reserved words and symbols carry no meaning — so
// realistic facts can be stored verbatim. A doubled quote (”) is an escaped
// literal quote; the first single quote that is not doubled closes the phrase.
//
// The opening quote is at the current position. On success a PHRASE token with
// the decoded (unquoted, unescaped) text is returned. If the input ends before
// a closing quote, an ILLEGAL token carrying the partial text is returned so the
// parser can report an unterminated phrase.
//
// End of input is detected by position, not by peek() returning rune(0): JSON
// may legally carry a NUL escape (\u0000) inside free-flowing text, and a
// phrase must swallow it as data like any other character rather than
// misreport the phrase as unterminated.
func (l *Lexer) scanPhrase() Token {
	start := l.CurrentPos
	l.readCharacter() // consume the opening quote

	var res []rune
	for {
		if l.CurrentPos.Column >= len(l.Input) {
			return Token{Type: ILLEGAL, Literal: string(res), Pos: start}
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
			return Token{Type: PHRASE, Literal: string(res), Pos: start}
		default:
			res = append(res, l.peek())
			l.readCharacter()
		}
	}
}
