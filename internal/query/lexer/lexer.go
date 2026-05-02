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

import "fmt"

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
		CurrentPos: Position{
			Column: 0,
		},
		NextPos: Position{
			Column: 1,
		},
	}
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
	fmt.Println(string(l.Input[l.CurrentPos.Column]))
	l.CurrentPos = l.NextPos
	l.NextPos.Column++
}

// checks if rune is white space
func isBlank(ch rune) bool {
	return ch == rune(' ') || ch == rune('\t') || ch == rune('\r')
}

func (l *Lexer) Next() Token {
	var tok Token

	l.skipBlank()

	switch l.peek() {
	case rune(':'):
		l.readCharacter()
		tok = Token{Type: COLON, Literal: string(l.Character)}
	case rune('\''):
		l.readCharacter()
		tok = Token{Type: COMMA, Literal: string(l.Character)}
	case rune('('):
		l.readCharacter()
		tok = Token{Type: LPAREN, Literal: string(l.Character)}
	case rune(')'):
		l.readCharacter()
		tok = Token{Type: RPAREN, Literal: string(l.Character)}
	case rune('$'):
		l.readCharacter()
		tok = Token{Type: DOLLAR, Literal: string(l.Character)}
	case rune(0):
		tok = Token{Type: EOL}
	default:
		tokLiteral := l.scanString()
		tokType, err := KeywordsMap[tokLiteral]
		if !err {
			tokType = WORD
		}
		tok = Token{Type: tokType, Literal: tokLiteral}
	}
	return tok
}

// peeks current character
func (l *Lexer) peek() rune {
	if l.CurrentPos.Column >= len(l.Input) {
		return rune(0)
	}
	return l.Input[l.CurrentPos.Column]
}

func (l *Lexer) scanString() string {
	var res []rune
f:
	for {
		switch l.peek() {
		case rune(':'), rune('$'), rune('\''), rune('('), rune(')'), rune(' '), rune('\t'), rune('\r'):
			break f
		case rune(0):
			return ""
		default:
			res = append(res, l.peek())
		}
		l.readCharacter()
	}
	return string(res)
}
