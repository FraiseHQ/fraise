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

package parser

import (
	"fmt"

	"github.com/RonsenbergVI/fraise/internal/query/lexer"
)

type Warning struct {
	Msg string
	Pos lexer.Position
}

type Error struct {
	Msg string
	Pos lexer.Position
}

func (e *Error) Error() string {
	return fmt.Sprintf("parse error at %d: %s", e.Pos, e.Msg)
}

type parser struct {
	l     *lexer.Lexer
	cur   lexer.Token
	peek  lexer.Token
	warns []Warning
}

func Parse(q string) (cmd CommandNode, warns []Warning, err error) {
	p := &parser{l: lexer.New(q)}
	// prime cur and peek
	p.next()
	p.next()

	command, err := p.parseQuery()

	if err != nil {
		return nil, p.warns, err
	}

	if p.cur.Type != lexer.EOL {
		return nil, p.warns, p.errf(p.l.CurrentPos, "unexpected %q after end of query (one command per instruction)", p.cur.Literal)
	}

	return command, p.warns, nil
}

// Goes to next token for parser to analyse
func (p *parser) next() {
	p.cur = p.peek
	p.peek = p.l.Next()
}

// expect consumes the current token if it has the given type, otherwise
// returns an error pointing at it.
func (p *parser) expect(t lexer.TokenType) (lexer.Token, error) {
	if p.cur.Type != t {
		return p.cur, p.errf(p.l.CurrentPos, "expected %v, found %q", t, p.cur.Literal)
	}
	tok := p.cur
	p.next()
	return tok, nil
}

func (p *parser) errf(pos lexer.Position, format string, args ...any) error {
	return &Error{
		Pos: pos,
		Msg: fmt.Sprintf(format, args...),
	}
}

func (p *parser) warnf(pos lexer.Position, format string, args ...any) {
	p.warns = append(p.warns, Warning{
		Pos: pos,
		Msg: fmt.Sprintf(format, args...),
	})
}

func (p *parser) parseQuery() (CommandNode, error) {
	switch p.cur.Type {
	case lexer.RECALL:
		return p.parseRecall()
	case lexer.REMEMBER:
		return p.parseRemember()
	default:
		return nil, p.errf(p.l.CurrentPos, "expected a command (recall, remember), found %q", p.cur.Literal)
	}
}

func (p *parser) parseRecall() (*RecallCommandNode, error) {
}

func (p *parser) parseRemember() (*RememberCommandNode, error) {
}

func (p *parser) parseGraphSelector() (*GraphSelectorNode, error) {
}

func (p *parser) parseTerm() TermNode {
}

func (p *parser) isFieldStart() bool {}

func (p *parser) parseFieldClause() (FieldNode, error) {
}

func (p *parser) parseTopicField() (*TopicField, error)   { panic("TODO") }
func (p *parser) parseEntityField() (*EntityField, error) { panic("TODO") }
func (p *parser) parseTimeField() (FieldNode, error)      { panic("TODO") } // since/until + parseTimeValue
func (p *parser) parseIntField() (FieldNode, error)       { panic("TODO") } // depth/top
func (p *parser) parseVecField() (*VectorField, error)    { panic("TODO") }
func (p *parser) parseGroupField() (*GroupField, error)   { panic("TODO") }
