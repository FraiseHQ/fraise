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
	Input     []rune
	Offset    int
	Character rune
	Current   Position
	Next      Position
}

// Returns a new Lexer pointer from a string
func New(input string) *Lexer {
	l := Lexer{
		Input: []rune(input),
	}
	l.readCharacter()
	return &l
}

// func (l *Lexer) NextToken() Token {
// 	var tok Token

// }

func (l *Lexer) skipWhitespace() {
	for isWhitespace(l.Character) {
		l.readCharacter()
	}
}

func (l *Lexer) readCharacter() {
	if l.Current.Column >= len(l.Input) {
		l.Character = rune(0)
	} else {
		l.Character = l.Input[l.Current.Column]
	}
	l.Current = l.Next
	l.Next.Column++
}

// checks if rune is white space
func isWhitespace(ch rune) bool {
	return ch == rune(' ') || ch == rune('\t') || ch == rune('\r')
}

// checks if rune is new line
func isNewline(ch rune) bool {
	return ch == rune('\n')
}

// checks if rune is digit
func isDigit(ch rune) bool {
	return rune('0') <= ch && ch <= rune('9')
}
