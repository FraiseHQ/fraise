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

// Represents tokens returned by the lexer
type Token struct {
	Type    TokenType
	Literal string
}

type TokenType int

const (
	ILLEGAL TokenType = iota
	EOL

	// literal
	LITERAL

	// commands
	RECALL
	REMEMBER
	FORGET
	UPDATE

	// boolean operations
	AND
	OR

	// punctuation
	COLON
	COMMA
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
	OR:       "or",
	AND:      "and",
	COLON:    ":",
	LPAREN:   "(",
	RPAREN:   ")",
	DOLLAR:   "$",
	COMMA:    "'",
	NEWLINE:  "\n",
	TOPIC:    "topic",
	ENTITY:   "entity",
	SINCE:    "since",
	UNTIL:    "until",
	TOP:      "top",
	DEPTH:    "depth",
	LITERAL:  "literal",
	VEC:      "vec",
	EOL:      "eol",
}

var KeywordsMap = map[string]TokenType{
	"recall":   RECALL,
	"remember": REMEMBER,
	"forget":   FORGET,
	"update":   UPDATE,
	"or":       OR,
	"and":      AND,
	"topic":    TOPIC,
	"entity":   ENTITY,
	"since":    SINCE,
	"until":    UNTIL,
	"top":      TOP,
	"depth":    DEPTH,
	"vec":      VEC,
}

func (t TokenType) String() string {
	return TokenMap[t]
}
