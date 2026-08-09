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
	"math"
	"strconv"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/query/lexer"
)

type Warning struct {
	Msg string
	Pos lexer.Position
}

// Error is a parse failure at a specific position in the query. It is returned
// (wrapped) by the query layer; recover it with errors.As to get the position.
type Error struct {
	Msg string
	Pos lexer.Position
}

func (e *Error) Error() string {
	return fmt.Sprintf("parse error at column %d: %s", e.Pos.Column, e.Msg)
}

type parser[K comparable, P float32 | float64] struct {
	l     *lexer.Lexer
	cur   lexer.Token
	peek  lexer.Token
	warns []Warning
}

func Parse[K comparable, P float32 | float64](q string) (cmd CommandNode, warns []Warning, err error) {
	p := &parser[K, P]{l: lexer.New(q)}
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
func (p *parser[K, P]) next() {
	p.cur = p.peek
	p.peek = p.l.Next()
}

// expect consumes the current token if it has the given type, otherwise
// returns an error pointing at it.
func (p *parser[K, P]) expect(t lexer.TokenType) (lexer.Token, error) {
	if p.cur.Type != t {
		return p.cur, p.errf(p.l.CurrentPos, "expected %v, found %q", t, p.cur.Literal)
	}
	tok := p.cur
	p.next()
	return tok, nil
}

func (p *parser[K, P]) errf(pos lexer.Position, format string, args ...any) error {
	return &Error{
		Pos: pos,
		Msg: fmt.Sprintf(format, args...),
	}
}

func (p *parser[K, P]) parseQuery() (CommandNode, error) {
	switch p.cur.Type {
	case lexer.REMEMBER:
		return (*p).parseRemember()
	case lexer.RECALL:
		return (*p).parseRecall()
	default:
		return nil, p.errf(p.l.CurrentPos, "expected a command (recall, remember), found %q", p.cur.Literal)
	}
}

func (p *parser[K, P]) parseRemember() (*RememberCommandNode[P], error) {

	r := RememberCommandNode[P]{}

	r.key = p.cur

	p.next()

	if p.cur.Type == lexer.AT {
		key, value, err := p.parseGraphSelector()
		if err != nil {
			return nil, err
		}
		r.selector = GraphSelectorNode{key: key, value: value}
	}

	// Remember carries exactly one quoted phrase (the fact). The lexer returns
	// the whole '...' as a single PHRASE token, so consuming it also consumes
	// the closing quote — no separate delimiter handling here.
	phrase, err := p.parsePhrase()
	if err != nil {
		return nil, err
	}
	r.value = *phrase

	var anchors []AnchorFieldNode

	for p.cur.Type != lexer.EOL {
		switch p.cur.Type {
		case lexer.ENTITY, lexer.TOPIC:
			key, value, err := p.parseAnchorField()
			if err != nil {
				return nil, err
			}
			var field FieldNode[string]
			if key.Type == lexer.TOPIC {
				field = TopicFieldNode{key: key, value: value}
			} else {
				field = EntityFieldNode{key: key, value: value}
			}
			anchors = append(anchors, AnchorFieldNode{field: field})
			r.anchors = anchors
		case lexer.VEC:
			vec, err := p.parseVecField()
			if err != nil {
				return nil, p.errf(p.l.CurrentPos, "Error while parsing vector ref field %q", p.cur.Literal)
			}
			r.vec = vec
		default:
			return nil, p.errf(p.l.CurrentPos, "Encountered unexpected token: %q", p.cur.Literal)
		}
	}

	return &r, nil
}

func (p *parser[K, P]) parseRecall() (*RecallCommandNode[K, P], error) {

	r := RecallCommandNode[K, P]{}

	r.key = p.cur

	p.next()

	if p.cur.Type == lexer.AT {
		key, value, err := p.parseGraphSelector()
		if err != nil {
			// parseGraphSelector already returns a positioned parse error with a
			// clear message; surface it as-is rather than re-wrapping (which lost
			// the column and mangled the message via a bad %e verb).
			return nil, err
		}
		r.selector = GraphSelectorNode{key: key, value: value}
	}

	// parse terms — a recall requires at least one term (a bare word or a quoted
	// phrase); fields (topic:, since:, ...) follow.
	tok, err := p.expectValue()
	if err != nil {
		return nil, err
	}

	r.terms = append(r.terms, TermNode{token: tok, value: tok.Literal})

	for p.cur.Type == lexer.LITERAL || p.cur.Type == lexer.PHRASE {
		r.terms = append(r.terms, TermNode{token: p.cur, value: p.cur.Literal})
		p.next()
	}

	// parse
	for p.cur.Type != lexer.EOL {
		switch p.cur.Type {
		case lexer.ENTITY:
			key, value, err := p.parseAnchorField()
			if err != nil {
				return nil, err
			}
			r.entities = append(r.entities, AnchorFieldNode{field: EntityFieldNode{key: key, value: value}})
		case lexer.TOPIC:
			key, value, err := p.parseAnchorField()
			if err != nil {
				return nil, err
			}
			r.topics = append(r.topics, AnchorFieldNode{field: TopicFieldNode{key: key, value: value}})
		case lexer.UNTIL:
			key, t, err := p.parseTimeValue()
			if err != nil {
				return nil, err
			}
			r.until = UntilFieldNode[K]{key: key, value: t}
		case lexer.SINCE:
			key, t, err := p.parseTimeValue()
			if err != nil {
				return nil, err
			}
			r.since = SinceFieldNode[K]{key: key, value: t}
		case lexer.DEPTH:
			key, value, err := p.parseDepth()
			if err != nil {
				return nil, err
			}
			r.depth = DepthFieldNode{key: key, value: value}
		case lexer.TOP:
			key, value, err := p.parseTop()
			if err != nil {
				return nil, err
			}
			r.top = TopFieldNode{key: key, value: value}
		case lexer.VEC:
			vec, err := p.parseVecField()
			if err != nil {
				return nil, p.errf(p.l.CurrentPos, "Error while parsing vector ref field %q", p.cur.Literal)
			}
			r.vec = vec
		default:
			return nil, p.errf(p.l.CurrentPos, "Encountered unexpected token: %q", p.cur.Literal)
		}
	}
	return &r, nil
}

func (p *parser[K, P]) parseDepth() (lexer.Token, int, error) {
	key := p.cur

	p.next()

	if _, err := p.expect(lexer.COLON); err != nil {
		return lexer.Token{}, 0, p.errf(p.l.CurrentPos, "Expected colon, but found %q", p.cur.Literal)
	}

	tok, err := p.expect(lexer.LITERAL)

	if err != nil {
		return lexer.Token{}, 0, p.errf(p.l.CurrentPos, "Expected literal, but found %q", p.cur.Literal)
	}

	i, err := strconv.Atoi(tok.Literal)
	if err != nil {
		return lexer.Token{}, 0, p.errf(p.l.CurrentPos, "invalid depth value %q: expected a non-negative integer", tok.Literal)
	}

	return key, i, nil
}

func (p *parser[K, P]) parseTop() (lexer.Token, int, error) {
	key := p.cur

	p.next()

	if _, err := p.expect(lexer.COLON); err != nil {
		return lexer.Token{}, 0, p.errf(p.l.CurrentPos, "Expected colon, but found %q", p.cur.Literal)
	}

	tok, err := p.expect(lexer.LITERAL)

	if err != nil {
		return lexer.Token{}, 0, p.errf(p.l.CurrentPos, "Expected literal, but found %q", p.cur.Literal)
	}

	i, err := strconv.Atoi(tok.Literal)
	if err != nil {
		return lexer.Token{}, 0, p.errf(p.l.CurrentPos, "invalid top value %q: expected a non-negative integer", tok.Literal)
	}

	return key, i, nil
}

// parseTimeValue consumes a since:/until: clause. The ':' must be present and
// is checked, never skipped: advancing blindly past the separator shifts every
// following token into the wrong role, so "since 7d 30d" parsed clean and
// bounded the recall at 30d — a query silently answering a different question
// than the one asked, which is worse than an error an agent can correct from.
func (p *parser[K, P]) parseTimeValue() (lexer.Token, containers.TimeValue[K], error) {
	key := p.cur

	p.next()

	if _, err := p.expect(lexer.COLON); err != nil {
		return lexer.Token{}, nil, p.errf(p.l.CurrentPos, "Expected colon, but found %q", p.cur.Literal)
	}

	tok, err := p.expect(lexer.LITERAL)

	if err != nil {
		return lexer.Token{}, nil, p.errf(p.l.CurrentPos, "Expected literal, but found %q", p.cur.Literal)
	}

	t, err := containers.ParseTimeValue[K](tok.Literal)
	if err != nil {
		return lexer.Token{}, nil, p.errf(p.l.CurrentPos, "invalid %s value %q: expected a duration like 7d or a date like 2026-01-15", key.Literal, tok.Literal)
	}

	return key, t, nil
}

func (p *parser[K, P]) parseGraphSelector() (lexer.Token, uint8, error) {
	key := p.cur

	p.next()

	tok, err := p.expect(lexer.LITERAL)

	if err != nil {
		return lexer.Token{}, 0, p.errf(p.l.CurrentPos, "Expected literal, but found %q", p.cur.Literal)
	}

	// Validate the full integer before narrowing to uint8. A blind uint8(i)
	// wraps an out-of-range selector into a valid-looking graph (@256 -> 0,
	// @300 -> 44), so it would silently execute against the wrong graph — a
	// tenant-isolation break, since a graph is a user/session. Reject a
	// non-integer or anything outside the uint8 range here; the handler still
	// enforces the tighter [0, num-graphs) bound on what survives.
	i, err := strconv.Atoi(tok.Literal)
	if err != nil {
		return lexer.Token{}, 0, p.errf(tok.Pos, "invalid graph selector %q: expected a whole number", tok.Literal)
	}
	if i < 0 || i > math.MaxUint8 {
		return lexer.Token{}, 0, p.errf(tok.Pos, "graph selector %d out of range (0-%d)", i, math.MaxUint8)
	}

	return key, uint8(i), nil
}

// parsePhrase consumes a single opaque PHRASE token (a quoted fact). The lexer
// has already stripped the quotes and decoded the ” escape.
func (p *parser[K, P]) parsePhrase() (*PhraseNode, error) {
	// An ILLEGAL token here means the lexer hit end-of-input before the closing
	// quote — report it at the opening quote it recorded.
	if p.cur.Type == lexer.ILLEGAL {
		return nil, p.errf(p.cur.Pos, "unterminated quoted phrase")
	}
	tok, err := p.expect(lexer.PHRASE)
	if err != nil {
		return nil, p.errf(p.l.CurrentPos, "expected a quoted phrase, but found %q", p.cur.Literal)
	}
	return &PhraseNode{value: tok.Literal, pos: tok.Pos}, nil
}

// expectValue consumes a value that may be either a bare word (LITERAL) or a
// quoted phrase (PHRASE) — the two forms a recall term or an anchor value can
// take. Quoting lets a value contain reserved words or symbols verbatim.
func (p *parser[K, P]) expectValue() (lexer.Token, error) {
	if p.cur.Type == lexer.ILLEGAL {
		return p.cur, p.errf(p.cur.Pos, "unterminated quoted phrase")
	}
	if p.cur.Type == lexer.LITERAL || p.cur.Type == lexer.PHRASE {
		tok := p.cur
		p.next()
		return tok, nil
	}
	return p.cur, p.errf(p.l.CurrentPos, "expected a word or quoted phrase, but found %q", p.cur.Literal)
}

// parseAnchorField consumes a topic:/entity: clause. As in parseTimeValue, the
// ':' is required: "topic food extra" used to shift tokens into the wrong roles
// and return an unfiltered result set rather than a parse error.
func (p *parser[K, P]) parseAnchorField() (lexer.Token, string, error) {
	key := p.cur

	p.next()

	if _, err := p.expect(lexer.COLON); err != nil {
		return lexer.Token{}, "", p.errf(p.l.CurrentPos, "Expected colon, but found %q", p.cur.Literal)
	}

	// The anchor value is a bare word or a quoted phrase (e.g. topic:'my project').
	tok, err := p.expectValue()

	if err != nil {
		return lexer.Token{}, "", err
	}

	return key, tok.Literal, nil
}

func (p *parser[K, P]) parseVecField() (*VecFieldNode[P], error) {
	r := VecFieldNode[P]{}

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
