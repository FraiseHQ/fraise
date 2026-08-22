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
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/query/lexer"
)

// Warning is a parse-time observation about a query that ran anyway: the
// query is valid, but it is close enough to a different, also-valid query
// that a typo would change its meaning with no error to correct from. It
// rides the return path, never the query object — the plan cache substitutes
// query objects on a hash hit, so anything attached there would leak between
// requests.
type Warning struct {
	Msg string
	Pos lexer.Position
}

// String renders the warning as clients receive it, mirroring Error's
// "parse error at column N" shape so a position reads the same either way.
func (w Warning) String() string {
	return fmt.Sprintf("parse warning at column %d: %s", w.Pos.Column, w.Msg)
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

	// A query is one instruction, and a newline ends it. Blank lines after the
	// command are not a second instruction, so they are skipped; text on the
	// next line is, and saying so is the whole point of lexing the newline —
	// folded into whitespace it made "recall ferry\nbridge" a two-term recall.
	for p.cur.Type == lexer.NEWLINE {
		p.next()
	}

	if p.cur.Type != lexer.EOL {
		return nil, p.warns, p.errf(p.cur.Pos, "unexpected %s after end of query (one command per instruction)", describe(p.cur))
	}

	return command, p.warns, nil
}

// atEnd reports whether the current token ends the command's clause list: end
// of input, or the newline that would start a second instruction. Every loop
// that reads clauses stops here, so the "one command per instruction" rule is
// enforced once, in Parse, rather than by each loop separately.
func (p *parser[K, P]) atEnd() bool {
	return p.cur.Type == lexer.EOL || p.cur.Type == lexer.NEWLINE
}

// describe renders a token as an error message should name it. End of input has
// no literal and %q prints it as `""` — an empty string the caller never wrote
// and cannot act on, which turns a repairable mistake ("recall ferry top") into
// what reads like a parser bug.
func describe(tok lexer.Token) string {
	switch tok.Type {
	case lexer.EOL:
		return "end of input"
	case lexer.NEWLINE:
		return "a new line"
	default:
		return strconv.Quote(tok.Literal)
	}
}

// misCased reports whether tok is a bare word spelling a keyword in the wrong
// case. The lexer hands those back as literals everywhere a ':' does not follow
// (see its keyword lookup), so recognising one is the parser's job wherever a
// clause could have started.
func misCased(tok lexer.Token) bool {
	if tok.Type != lexer.LITERAL {
		return false
	}
	_, reserved := lexer.KeywordsMap[strings.ToLower(tok.Literal)]
	return reserved
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
		return p.cur, p.errf(p.cur.Pos, "expected %v, found %s", t, describe(p.cur))
	}
	tok := p.cur
	p.next()
	return tok, nil
}

// unexpected builds the error for a token no production accepts at this point.
// The caller is an agent: it can only repair a query the message tells it how
// to repair, so every shape a caller actually produces gets its own diagnosis
// and the bare "unexpected" fallback is left for the shapes that have none.
func (p *parser[K, P]) unexpected(tok lexer.Token) error {
	switch {
	case tok.Type == lexer.ILLEGAL:
		return p.errf(tok.Pos, "unterminated quoted phrase")
	case tok.Type == lexer.LPAREN, tok.Type == lexer.RPAREN:
		return p.errf(tok.Pos, "grouping is not supported: %s has no meaning in a query — there are no boolean operators to group, and terms are already a union", describe(tok))
	case tok.Type == lexer.NEWLINE:
		return p.errf(tok.Pos, "unexpected %s: one command per instruction", describe(tok))
	case tok.Type.IsCommand():
		return p.errf(tok.Pos, "%s starts a second command: one command per instruction", describe(tok))
	case tok.Type.IsKeyword():
		return p.keywordAsClause(tok)
	case misCased(tok):
		return p.errf(tok.Pos, "mis-cased keyword %q: keywords are lower case — write %s:<value> if a clause was meant, or quote it ('%s') to search for the word",
			tok.Literal, strings.ToLower(tok.Literal), tok.Literal)
	default:
		return p.errf(tok.Pos, "unexpected %s", describe(tok))
	}
}

// keywordAsClause rejects a reserved word standing where a clause must start
// without the ':' that would make it one. It is the same mistake as a mis-cased
// keyword and gets the same repair instruction, rather than a complaint that a
// colon is missing: "recall ferry top" is a caller who meant the word "top"
// far more often than one who abandoned a top:<n> clause mid-write, and either
// way the fix is a colon or a quote.
func (p *parser[K, P]) keywordAsClause(tok lexer.Token) error {
	return p.errf(tok.Pos, "%s is a keyword and starts no clause here: write %s:<value> if a clause was meant, or quote it ('%s') to search for the word",
		describe(tok), strings.ToLower(tok.Literal), tok.Literal)
}

// duplicate rejects a single-valued clause given twice. Last-wins is the worse
// answer: it runs a differently-scoped query than the one asked with nothing in
// the response to say so, and a repeated clause is an agent generation bug the
// agent can only correct if the message names which clause to drop. Anchors are
// exempt — a repeated topic:/entity: is a list by design.
func (p *parser[K, P]) duplicate(tok lexer.Token) error {
	clause := strings.ToLower(tok.Literal)
	return p.errf(tok.Pos, "duplicate %s clause: %s may be given only once — drop one", clause, clause)
}

// requireNonBlank rejects an empty or whitespace-only value. Quoting is the only
// way to write one and it is never what a caller meant: an empty fact can never
// be retrieved, and an empty anchor is an identity nobody can name a second
// time, so both corrupt a graph quietly instead of failing where they were made.
func (p *parser[K, P]) requireNonBlank(role, value string, pos lexer.Position) error {
	if strings.TrimSpace(value) != "" {
		return nil
	}
	return p.errf(pos, "%s must not be empty", role)
}

// danglingKeyword reports whether the current token is a reserved word with
// nothing after it. No clause can be completed from there, so it is the word a
// caller forgot to quote rather than a clause they abandoned.
func (p *parser[K, P]) danglingKeyword() bool {
	return p.cur.Type.IsKeyword() && (p.peek.Type == lexer.EOL || p.peek.Type == lexer.NEWLINE)
}

// clauseValue consumes whatever token sits in a clause's value slot. It accepts
// any type on purpose: a keyword, a '-' or end of input reaching the clause's
// own converter is what lets the error name the clause and the text it could
// not read. "expected literal, found \"top\"" is wrong twice — to the caller
// "top" *is* a literal, and the clause that rejected it goes unnamed.
func (p *parser[K, P]) clauseValue() lexer.Token {
	tok := p.cur
	p.next()
	return tok
}

// parseInteger reads a clause's integer value, naming the clause in any error.
// A value too large to hold is reported apart from one that is not a number at
// all: an agent told only "invalid" retries with another huge number, while one
// told the value is out of range knows to shrink it.
func (p *parser[K, P]) parseInteger(tok lexer.Token, what string) (int, error) {
	i, err := strconv.Atoi(tok.Literal)
	switch {
	case errors.Is(err, strconv.ErrRange):
		return 0, p.errf(tok.Pos, "invalid %s %s: out of range, expected a non-negative whole number", what, describe(tok))
	case err != nil:
		return 0, p.errf(tok.Pos, "invalid %s %s: expected a non-negative whole number", what, describe(tok))
	}
	return i, nil
}

// errf builds a positioned parse error. pos must be the Pos of the token the
// message blames — never p.l.CurrentPos, which sits a token further right
// because cur/peek read ahead: an error quoting "food" would then point at the
// end of whatever followed "food", sending a reader to the wrong word. Blaming
// the token itself lands the column on its last character.
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
		return nil, p.errf(p.cur.Pos, "expected a command (recall, remember), found %q", p.cur.Literal)
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
	if err := p.requireNonBlank("a remembered fact", phrase.value, phrase.pos); err != nil {
		return nil, err
	}
	r.value = *phrase

	var anchors []AnchorFieldNode

	for !p.atEnd() {
		if p.danglingKeyword() {
			return nil, p.keywordAsClause(p.cur)
		}
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
		case lexer.VEC:
			if r.vec != nil {
				return nil, p.duplicate(p.cur)
			}
			vec, err := p.parseVecField()
			if err != nil {
				return nil, err
			}
			r.vec = vec
		default:
			return nil, p.unexpected(p.cur)
		}
	}

	r.anchors = anchors

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

	terms, err := p.parseTerms()
	if err != nil {
		return nil, err
	}
	r.terms = terms

	// Clauses follow the terms. Each modifier is single-valued and each anchor
	// is a list, so a repeat means opposite things for the two and only the
	// modifiers reject it.
	for !p.atEnd() {
		if p.danglingKeyword() {
			return nil, p.keywordAsClause(p.cur)
		}
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
			if r.until.key.Type == lexer.UNTIL {
				return nil, p.duplicate(p.cur)
			}
			key, t, err := p.parseTimeValue()
			if err != nil {
				return nil, err
			}
			r.until = UntilFieldNode[K]{key: key, value: t}
		case lexer.SINCE:
			if r.since.key.Type == lexer.SINCE {
				return nil, p.duplicate(p.cur)
			}
			key, t, err := p.parseTimeValue()
			if err != nil {
				return nil, err
			}
			r.since = SinceFieldNode[K]{key: key, value: t}
		case lexer.DEPTH:
			if r.depth.key.Type == lexer.DEPTH {
				return nil, p.duplicate(p.cur)
			}
			key, value, err := p.parseIntField()
			if err != nil {
				return nil, err
			}
			r.depth = DepthFieldNode{key: key, value: value}
		case lexer.TOP:
			if r.top.key.Type == lexer.TOP {
				return nil, p.duplicate(p.cur)
			}
			key, value, err := p.parseIntField()
			if err != nil {
				return nil, err
			}
			r.top = TopFieldNode{key: key, value: value}
		case lexer.VEC:
			if r.vec != nil {
				return nil, p.duplicate(p.cur)
			}
			vec, err := p.parseVecField()
			if err != nil {
				return nil, err
			}
			r.vec = vec
		default:
			return nil, p.unexpected(p.cur)
		}
	}

	// A recall has to be seeded by something the search can start from. Terms,
	// anchors and a vector all qualify — an anchor is a seed, not a filter, so
	// "everything about billing" is a well-formed question — but a query made
	// only of modifiers scopes a search that was never started, and would
	// otherwise return an empty result set as though it had asked something.
	if len(r.terms) == 0 && len(r.topics) == 0 && len(r.entities) == 0 && r.vec == nil {
		return nil, p.errf(r.key.Pos, "a recall needs at least one seed: a term, a topic:/entity: anchor, or vec:$<name>")
	}

	return &r, nil
}

// parseTerms reads a recall's leading term list, which may be empty. Terms are
// folded to lower case: query data is matched without regard to case
// everywhere, and folding at the edge keeps every downstream spelling of the
// query (index lookup, plan-cache key) agreeing on one form.
//
// Only the first term may spell a reserved word. That position is the one place
// no clause can begin, which is what makes a bare keyword data there — and also
// what makes a mistyped clause slip through as a search, so it warns. From the
// second term on a keyword starts a clause, in any casing: folding "Since" into
// a term there would let "recall x Since 7d" read as three terms — the silent
// token-shift parseTimeValue guards against, back through the casing door.
func (p *parser[K, P]) parseTerms() ([]LiteralFieldNode, error) {
	if !p.startsTerm() {
		return nil, nil
	}

	tok, err := p.expectValue()
	if err != nil {
		return nil, err
	}
	if err := p.requireNonBlank("a search term", tok.Literal, tok.Pos); err != nil {
		return nil, err
	}

	// The ambiguity cannot be resolved here — "recall since 7d" is one ':' from
	// "recall since:7d", and the wrong reading silently answers a
	// differently-scoped question — so it is surfaced: the query runs as the
	// term search and carries a warning naming both readings. A quoted phrase
	// never warns; quoting is the deliberate form.
	if _, reserved := lexer.KeywordsMap[strings.ToLower(tok.Literal)]; reserved && tok.Type != lexer.PHRASE {
		p.warns = append(p.warns, Warning{
			Msg: fmt.Sprintf("term %q is also a keyword: write %s:<value> if a clause was meant, or quote it ('%s') to search for the word",
				tok.Literal, strings.ToLower(tok.Literal), tok.Literal),
			Pos: tok.Pos,
		})
	}

	terms := []LiteralFieldNode{TermNode{token: tok, value: strings.ToLower(tok.Literal)}}

	for p.cur.Type == lexer.LITERAL || p.cur.Type == lexer.PHRASE {
		if misCased(p.cur) {
			return nil, p.unexpected(p.cur)
		}
		if err := p.requireNonBlank("a search term", p.cur.Literal, p.cur.Pos); err != nil {
			return nil, err
		}
		terms = append(terms, TermNode{token: p.cur, value: strings.ToLower(p.cur.Literal)})
		p.next()
	}

	return terms, nil
}

// startsTerm reports whether the current token opens the term list. A reserved
// word can, but only when no ':' follows it: keyword-colon is always a clause,
// and that one tie-breaker is what tells "recall topic:billing" — a recall
// seeded by an anchor — apart from "recall topic", a search for the word.
func (p *parser[K, P]) startsTerm() bool {
	switch {
	case p.cur.Type == lexer.LITERAL, p.cur.Type == lexer.PHRASE, p.cur.Type == lexer.ILLEGAL:
		return true
	case p.cur.Type.IsKeyword():
		return p.peek.Type != lexer.COLON
	default:
		return false
	}
}

// parseIntField consumes a depth:/top: clause. One function serves both, as
// parseTimeValue does for since:/until:, so the two ceilings cannot drift apart
// in how they read a value or name it back in an error; the key token says
// which clause was written.
func (p *parser[K, P]) parseIntField() (lexer.Token, int, error) {
	key := p.cur

	p.next()

	if _, err := p.expect(lexer.COLON); err != nil {
		return lexer.Token{}, 0, p.errf(p.cur.Pos, "Expected colon, but found %s", describe(p.cur))
	}

	tok := p.clauseValue()

	i, err := p.parseInteger(tok, strings.ToLower(key.Literal)+" value")
	if err != nil {
		return lexer.Token{}, 0, err
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
		return lexer.Token{}, nil, p.errf(p.cur.Pos, "Expected colon, but found %s", describe(p.cur))
	}

	tok := p.clauseValue()

	t, err := containers.ParseTimeValue[K](tok.Literal)
	if err != nil {
		return lexer.Token{}, nil, p.errf(tok.Pos, "invalid %s value %s: expected a duration like 7d or a date like 2026-01-15", strings.ToLower(key.Literal), describe(tok))
	}

	return key, t, nil
}

func (p *parser[K, P]) parseGraphSelector() (lexer.Token, uint8, error) {
	key := p.cur

	p.next()

	tok := p.clauseValue()

	// Validate the full integer before narrowing to uint8. A blind uint8(i)
	// wraps an out-of-range selector into a valid-looking graph (@256 -> 0,
	// @300 -> 44), so it would silently execute against the wrong graph — a
	// tenant-isolation break, since a graph is a user/session. Reject a
	// non-integer or anything outside the uint8 range here; the handler still
	// enforces the tighter [0, num-graphs) bound on what survives.
	i, err := p.parseInteger(tok, "graph selector")

	if err != nil {
		return lexer.Token{}, 0, err
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
		return nil, p.errf(p.cur.Pos, "expected a quoted phrase, but found %s", describe(p.cur))
	}
	return &PhraseNode{value: tok.Literal, pos: tok.Pos}, nil
}

// expectValue consumes a value that may be either a bare word (LITERAL) or a
// quoted phrase (PHRASE) — the two forms a recall term or an anchor value can
// take. A reserved word also counts as a bare word here unless a ':' follows
// it: in value position a keyword is just data (entity:top files under the
// word "top"), but keyword-colon is always a field, so a clause mistyped into
// value position stays an error instead of being swallowed as data. Quoting
// remains the escape hatch for anything this rule cannot express.
func (p *parser[K, P]) expectValue() (lexer.Token, error) {
	if p.cur.Type == lexer.LITERAL || p.cur.Type == lexer.PHRASE ||
		(p.cur.Type.IsKeyword() && p.peek.Type != lexer.COLON) {
		tok := p.cur
		p.next()
		return tok, nil
	}
	// A token with a diagnosis of its own keeps it: an unclosed quote and a
	// parenthesis are not "the wrong kind of value", they are mistakes that
	// name themselves, and saying so is worth more here than saying what a
	// value slot wanted.
	switch p.cur.Type {
	case lexer.ILLEGAL, lexer.LPAREN, lexer.RPAREN, lexer.NEWLINE:
		return p.cur, p.unexpected(p.cur)
	}
	return p.cur, p.errf(p.cur.Pos, "expected a word or quoted phrase, but found %s", describe(p.cur))
}

// parseAnchorField consumes a topic:/entity: clause. As in parseTimeValue, the
// ':' is required: "topic food extra" used to shift tokens into the wrong roles
// and return an unfiltered result set rather than a parse error.
func (p *parser[K, P]) parseAnchorField() (lexer.Token, string, error) {
	key := p.cur

	p.next()

	if _, err := p.expect(lexer.COLON); err != nil {
		return lexer.Token{}, "", p.errf(p.cur.Pos, "Expected colon, but found %s", describe(p.cur))
	}

	// The anchor value is a bare word or a quoted phrase (e.g. topic:'my
	// project'), folded to lower case: an anchor is an identity, not prose —
	// topic:Billing and topic:billing must select the same anchor, and folding
	// at the edge is what stops the graph growing two spellings of one anchor.
	// Only the quoted fact of a remember keeps the case it was written with.
	tok, err := p.expectValue()

	if err != nil {
		return lexer.Token{}, "", err
	}

	if err := p.requireNonBlank("an anchor value", tok.Literal, tok.Pos); err != nil {
		return lexer.Token{}, "", err
	}

	return key, strings.ToLower(tok.Literal), nil
}

// parseVecField consumes a vec:$name clause. Its errors are returned to the
// caller unchanged: every other clause surfaces its own positioned message, and
// re-wrapping this one lost both the position and the only detail a caller
// could act on ("the $ is missing", "the name is missing").
func (p *parser[K, P]) parseVecField() (*VecFieldNode[P], error) {
	r := VecFieldNode[P]{}

	tok, err := p.expect(lexer.VEC)

	if err != nil {
		return nil, err
	}

	r.key = tok

	if _, err := p.expect(lexer.COLON); err != nil {
		return nil, p.errf(p.cur.Pos, "Expected colon, but found %s", describe(p.cur))
	}

	if _, err := p.expect(lexer.DOLLAR); err != nil {
		return nil, p.errf(p.cur.Pos, "expected param field operator $, but found %s", describe(p.cur))
	}

	tok, err = p.expect(lexer.LITERAL)

	if err != nil {
		return nil, err
	}

	r.param = tok

	return &r, nil
}
