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
	"strconv"

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
	case lexer.REMEMBER:
		return (*p).parseRemember()
	default:
		return nil, p.errf(p.l.CurrentPos, "expected a command (recall, remember), found %q", p.cur.Literal)
	}
}

func (p *parser) parseRemember() (*RememberCommandNode, error) {

	r := RememberCommandNode{}

	r.key = p.cur

	p.next()

	if p.cur.Type == lexer.AT {
		selector, err := p.parseGraphSelector()
		if err != nil {
			return nil, p.errf(p.l.CurrentPos, "Error while parsing graph selector %e", err)
		}
		r.selector = *selector
	}
	// Remember only supports one phrase
	_, err := p.expect(lexer.COMMA)

	if err != nil {
		return nil, p.errf(p.l.CurrentPos, "expected comma, but found %q", p.cur.Literal)
	}

	// one phrase or multiple terms

	phrase, err := p.parsePhrase()
	if err != nil {
		return nil, p.errf(p.l.CurrentPos, "Error while parsing phrase %e", err)
	}
	r.value = *phrase

	p.next()

	var anchors []AnchorFieldNode

	for p.cur.Type != lexer.EOL {
		switch p.cur.Type {
		case lexer.ENTITY, lexer.TOPIC:
			anchor, err := p.parseAnchorField()
			if err != nil {
				return nil, p.errf(p.l.CurrentPos, "Error while parsing anchor %e", err)
			}
			anchors = append(anchors, *anchor)
			r.anchors = anchors
			p.next()
		case lexer.VEC:
			vec, err := p.parseVecField()
			if err != nil {
				return nil, p.errf(p.l.CurrentPos, "Error while parsing vector ref field %q", p.cur.Literal)
			}
			r.vec = vec
			p.next()
		default:
			return nil, p.errf(p.l.CurrentPos, "Error while parsing vector ref field %q", p.cur.Literal)
		}
	}

	return &r, nil
}

func (p *parser) parseGraphSelector() (*GraphSelectorNode, error) {
	gsn := &GraphSelectorNode{}

	gsn.key = p.cur

	p.next()

	tok, err := p.expect(lexer.LITERAL)

	if err != nil {
		return nil, p.errf(p.l.CurrentPos, "Expected literal, but found %q", p.cur.Literal)
	}

	i, _ := strconv.Atoi(tok.Literal)

	// NOTE: probably not the safest way to do this (panic if you can't convert or casts anyway?).
	// Maybe just store the graph id as a int and check that value doesn't exceed number of graphs
	/// a bit awkwards but maybe better than having to do this.
	gsn.value = uint8(i)

	return gsn, nil
}

func (p *parser) parsePhrase() (*PhraseNode, error) {
	pn := PhraseNode{}

	for p.cur.Type != lexer.COMMA {
		switch p.cur.Type {
		case lexer.LITERAL:
			pn.tokens = append(pn.tokens, p.cur)
			p.next()
		default:
			return nil, p.errf(p.l.CurrentPos, "Expected literal, but found %q", p.cur.Literal)
		}
	}
	return &pn, nil
}

func (p *parser) parseAnchorField() (*AnchorFieldNode, error) {
	a := AnchorFieldNode{}

	if p.cur.Type == lexer.TOPIC {
		f := TopicFieldNode{}
		f.key = p.cur
		// skip comma
		p.next()
		p.next()
		f.value = p.cur
		a.field = f
	} else {
		f := EntityFieldNode{}
		f.key = p.cur
		// skip comma
		p.next()
		p.next()
		f.value = p.cur
		a.field = f
	}

	return &a, nil
}

func (p *parser) parseVecField() (*VecFieldNode, error) {
	r := VecFieldNode{}

	tok, err := p.expect(lexer.VEC)

	if err != nil {
		return nil, p.errf(p.l.CurrentPos, "Expected vec field, but found %q", tok)
	}

	r.key = tok

	_, err = p.expect(lexer.COLON)

	if err != nil {
		return nil, p.errf(p.l.CurrentPos, "Expected colon, but found %q", tok)
	}

	_, err = p.expect(lexer.DOLLAR)

	if err != nil {
		return nil, p.errf(p.l.CurrentPos, "Expected param field operator $, but found %q", p.cur.Literal)
	}

	tok, err = p.expect(lexer.LITERAL)

	r.param = tok

	if err != nil {
		return nil, p.errf(p.l.CurrentPos, "Expected literal, but found %q", tok)
	}

	return &r, nil
}
