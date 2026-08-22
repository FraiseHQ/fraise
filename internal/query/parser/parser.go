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
		return nil, p.warns, p.errf(p.cur.Pos, "unexpected %s after end of query (one command per instruction)", p.cur.Describe())
	}

	return command, p.warns, nil
}

// cursor: everything that reads or moves the two-token window, and
// nothing that decides what a query means.

// Goes to next token for parser to analyse
func (p *parser[K, P]) next() {
	p.cur = p.peek
	p.peek = p.l.Next()
}

// take consumes the current token whatever its type. Accepting any type is the
// point at a clause's value slot: a keyword, a '-' or end of input reaching the
// clause's own converter is what lets the error name the clause and the text it
// could not read. "expected literal, found \"top\"" is wrong twice — to the
// caller "top" *is* a literal, and the clause that rejected it goes unnamed.
func (p *parser[K, P]) take() lexer.Token {
	tok := p.cur
	p.next()
	return tok
}

// expect consumes the current token if it has the given type, otherwise
// returns an error pointing at it.
func (p *parser[K, P]) expect(t lexer.TokenType) (lexer.Token, error) {
	if p.cur.Type != t {
		return p.cur, p.errf(p.cur.Pos, "expected %v, found %s", t, p.cur.Describe())
	}
	tok := p.cur
	p.next()
	return tok, nil
}

// isAtEnd reports whether the current token ends the command's clause list: end
// of input, or the newline that would start a second instruction. Every loop
// that reads clauses stops here, so the "one command per instruction" rule is
// enforced once, in Parse, rather than by each loop separately.
func (p *parser[K, P]) isAtEnd() bool {
	return p.cur.Type == lexer.EOL || p.cur.Type == lexer.NEWLINE
}

// isValue reports whether the current token can stand in value position:
// the leading term of a recall, or an anchor's value after its ':'. A reserved
// word can, but only when no ':' follows it: keyword-colon is always a clause,
// and that one tie-breaker is what tells "recall topic:billing" — a recall
// seeded by an anchor — apart from "recall topic", a search for the word.
//
// The two positions share this one definition on purpose. They are documented
// together in the query spec, and a second copy of the rule is how they would
// come to disagree about what a value is.
func (p *parser[K, P]) isValue() bool {
	switch {
	case p.cur.Type == lexer.LITERAL, p.cur.Type == lexer.PHRASE:
		return true
	case p.cur.Type.IsKeyword():
		return p.peek.Type != lexer.COLON
	default:
		return false
	}
}

// isDanglingKeyword reports whether the current token is a reserved word with
// nothing after it. No clause can be completed from there, so it is the word a
// caller forgot to quote rather than a clause they abandoned.
func (p *parser[K, P]) isDanglingKeyword() bool {
	return p.cur.Type.IsKeyword() && (p.peek.Type == lexer.EOL || p.peek.Type == lexer.NEWLINE)
}

// errors: every message a caller sees is built here, so the repair
// instructions stay consistent with each other instead of being invented
// again at each rejection site.

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

// errUnexpected builds the error for a token no production accepts here. The
// caller is an agent: it can only repair a query the message tells it how to
// repair, so every shape a caller actually produces gets its own diagnosis,
// and the bare "unexpected" fallback is left for the shapes that have none.
func (p *parser[K, P]) errUnexpected(tok lexer.Token) error {
	switch {
	case tok.Type == lexer.ILLEGAL:
		return p.errf(tok.Pos, "unterminated quoted phrase")
	case tok.Type == lexer.LPAREN, tok.Type == lexer.RPAREN:
		return p.errf(tok.Pos, "grouping is not supported: %s has no meaning in a query — there are no boolean operators to group, and terms are already a union", tok.Describe())
	case tok.Type == lexer.NEWLINE:
		return p.errf(tok.Pos, "unexpected %s: one command per instruction", tok.Describe())
	case tok.Type.IsCommand():
		return p.errf(tok.Pos, "%s starts a second command: one command per instruction", tok.Describe())
	case tok.Type.IsKeyword():
		return p.errKeywordAsClause(tok)
	case tok.IsMisCasedKeyword():
		return p.errf(tok.Pos, "mis-cased keyword %q: keywords are lower case — write %s:<value> if a clause was meant, or quote it ('%s') to search for the word",
			tok.Literal, strings.ToLower(tok.Literal), tok.Literal)
	default:
		return p.errf(tok.Pos, "unexpected %s", tok.Describe())
	}
}

// errKeywordAsClause rejects a reserved word standing where a clause must start
// without the ':' that would make it one. It is the same mistake as a mis-cased
// keyword and gets the same repair instruction, rather than a complaint that a
// colon is missing: "recall ferry top" is a caller who meant the word "top"
// far more often than one who abandoned a top:<n> clause mid-write, and either
// way the fix is a colon or a quote.
func (p *parser[K, P]) errKeywordAsClause(tok lexer.Token) error {
	return p.errf(tok.Pos, "%s is a keyword and starts no clause here: write %s:<value> if a clause was meant, or quote it ('%s') to search for the word",
		tok.Describe(), strings.ToLower(tok.Literal), tok.Literal)
}

// errDuplicate rejects a single-valued clause given twice. Last-wins is the
// worse answer: it runs a differently-scoped query than the one asked with
// nothing in the response to say so, and a repeated clause is an agent
// generation bug the agent can only correct if the message names which clause
// to drop. Anchors are exempt — a repeated topic:/entity: is a list by design.
func (p *parser[K, P]) errDuplicate(tok lexer.Token) error {
	clause := strings.ToLower(tok.Literal)
	return p.errf(tok.Pos, "duplicate %s clause: %s may be given only once — drop one", clause, clause)
}

// errEmpty builds the error for an empty or whitespace-only value, and returns
// nil for any other. Quoting is the only way to write one and it is never what a
// caller meant: an empty fact can never be retrieved, and an empty anchor is an
// identity nobody can name a second time, so both corrupt a graph quietly
// instead of failing where they were made.
func (p *parser[K, P]) errEmpty(role, value string, pos lexer.Position) error {
	if strings.TrimSpace(value) != "" {
		return nil
	}
	return p.errf(pos, "%s must not be empty", role)
}

// productions: the grammar itself, top-down — command, then clauses,
// then the leaves a clause is built from.

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
	if err := p.errEmpty("a remembered fact", phrase.value, phrase.pos); err != nil {
		return nil, err
	}
	r.value = *phrase

	var anchors []AnchorFieldNode

	for !p.isAtEnd() {
		if p.isDanglingKeyword() {
			return nil, p.errKeywordAsClause(p.cur)
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
				return nil, p.errDuplicate(p.cur)
			}
			vec, err := p.parseVecField()
			if err != nil {
				return nil, err
			}
			r.vec = vec
		default:
			return nil, p.errUnexpected(p.cur)
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
	for !p.isAtEnd() {
		if p.isDanglingKeyword() {
			return nil, p.errKeywordAsClause(p.cur)
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
				return nil, p.errDuplicate(p.cur)
			}
			key, t, err := p.parseTimeValue()
			if err != nil {
				return nil, err
			}
			r.until = UntilFieldNode[K]{key: key, value: t}
		case lexer.SINCE:
			if r.since.key.Type == lexer.SINCE {
				return nil, p.errDuplicate(p.cur)
			}
			key, t, err := p.parseTimeValue()
			if err != nil {
				return nil, err
			}
			r.since = SinceFieldNode[K]{key: key, value: t}
		case lexer.DEPTH:
			if r.depth.key.Type == lexer.DEPTH {
				return nil, p.errDuplicate(p.cur)
			}
			key, value, err := p.parseIntField()
			if err != nil {
				return nil, err
			}
			r.depth = DepthFieldNode{key: key, value: value}
		case lexer.TOP:
			if r.top.key.Type == lexer.TOP {
				return nil, p.errDuplicate(p.cur)
			}
			key, value, err := p.parseIntField()
			if err != nil {
				return nil, err
			}
			r.top = TopFieldNode{key: key, value: value}
		case lexer.VEC:
			if r.vec != nil {
				return nil, p.errDuplicate(p.cur)
			}
			vec, err := p.parseVecField()
			if err != nil {
				return nil, err
			}
			r.vec = vec
		default:
			return nil, p.errUnexpected(p.cur)
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
	if !p.isValue() {
		return nil, nil
	}

	tok, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	if err := p.errEmpty("a search term", tok.Literal, tok.Pos); err != nil {
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
		if p.cur.IsMisCasedKeyword() {
			return nil, p.errUnexpected(p.cur)
		}
		if err := p.errEmpty("a search term", p.cur.Literal, p.cur.Pos); err != nil {
			return nil, err
		}
		terms = append(terms, TermNode{token: p.cur, value: strings.ToLower(p.cur.Literal)})
		p.next()
	}

	return terms, nil
}

// parseIntField consumes a depth:/top: clause. One function serves both, as
// parseTimeValue does for since:/until:, so the two ceilings cannot drift apart
// in how they read a value or name it back in an error; the key token says
// which clause was written.
func (p *parser[K, P]) parseIntField() (lexer.Token, int, error) {
	key := p.cur

	p.next()

	if _, err := p.expect(lexer.COLON); err != nil {
		return lexer.Token{}, 0, p.errf(p.cur.Pos, "Expected colon, but found %s", p.cur.Describe())
	}

	tok := p.take()

	// A value too large to hold is reported apart from one that is not a number
	// at all: an agent told only "invalid" retries with another huge number,
	// while one told the value is out of range knows to shrink it. The ceiling
	// on what a *configured* server will accept is the query layer's, not the
	// parser's — this is only about the value fitting at all.
	clause := strings.ToLower(key.Literal)
	i, err := strconv.Atoi(tok.Literal)
	switch {
	case errors.Is(err, strconv.ErrRange):
		return lexer.Token{}, 0, p.errf(tok.Pos, "invalid %s value %s: out of range, expected a non-negative whole number", clause, tok.Describe())
	case err != nil:
		return lexer.Token{}, 0, p.errf(tok.Pos, "invalid %s value %s: expected a non-negative whole number", clause, tok.Describe())
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
		return lexer.Token{}, nil, p.errf(p.cur.Pos, "Expected colon, but found %s", p.cur.Describe())
	}

	tok := p.take()

	t, err := containers.ParseTimeValue[K](tok.Literal)
	if err != nil {
		return lexer.Token{}, nil, p.errf(tok.Pos, "invalid %s value %s: expected a duration like 7d or a date like 2026-01-15", strings.ToLower(key.Literal), tok.Describe())
	}

	return key, t, nil
}

func (p *parser[K, P]) parseGraphSelector() (lexer.Token, uint8, error) {
	key := p.cur

	p.next()

	tok := p.take()

	// Validate the full integer before narrowing to uint8. A blind uint8(i)
	// wraps an out-of-range selector into a valid-looking graph (@256 -> 0,
	// @300 -> 44), so it would silently execute against the wrong graph — a
	// tenant-isolation break, since a graph is a user/session. Reject a
	// non-integer or anything outside the uint8 range here; the handler still
	// enforces the tighter [0, num-graphs) bound on what survives.
	i, err := strconv.Atoi(tok.Literal)

	if err != nil {
		return lexer.Token{}, 0, p.errf(tok.Pos, "invalid graph selector %s: expected a whole number", tok.Describe())
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
		return nil, p.errf(p.cur.Pos, "expected a quoted phrase, but found %s", p.cur.Describe())
	}
	return &PhraseNode{value: tok.Literal, pos: tok.Pos}, nil
}

// parseValue consumes a value, or says why the current token is not one. It is
// the consumer paired with isValue, which holds the rule: in value position
// a keyword is just data (entity:top files under the word "top"), but
// keyword-colon is always a field, so a clause mistyped into value position
// stays an error instead of being swallowed as data. Quoting remains the escape
// hatch for anything the rule cannot express.
func (p *parser[K, P]) parseValue() (lexer.Token, error) {
	if p.isValue() {
		return p.take(), nil
	}
	// A token with a diagnosis of its own keeps it: an unclosed quote and a
	// parenthesis are not "the wrong kind of value", they are mistakes that
	// name themselves, and saying so is worth more here than saying what a
	// value slot wanted.
	switch p.cur.Type {
	case lexer.ILLEGAL, lexer.LPAREN, lexer.RPAREN, lexer.NEWLINE:
		return p.cur, p.errUnexpected(p.cur)
	}
	return p.cur, p.errf(p.cur.Pos, "expected a word or quoted phrase, but found %s", p.cur.Describe())
}

// parseAnchorField consumes a topic:/entity: clause. As in parseTimeValue, the
// ':' is required: "topic food extra" used to shift tokens into the wrong roles
// and return an unfiltered result set rather than a parse error.
func (p *parser[K, P]) parseAnchorField() (lexer.Token, string, error) {
	key := p.cur

	p.next()

	if _, err := p.expect(lexer.COLON); err != nil {
		return lexer.Token{}, "", p.errf(p.cur.Pos, "Expected colon, but found %s", p.cur.Describe())
	}

	// The anchor value is a bare word or a quoted phrase (e.g. topic:'my
	// project'), folded to lower case: an anchor is an identity, not prose —
	// topic:Billing and topic:billing must select the same anchor, and folding
	// at the edge is what stops the graph growing two spellings of one anchor.
	// Only the quoted fact of a remember keeps the case it was written with.
	tok, err := p.parseValue()

	if err != nil {
		return lexer.Token{}, "", err
	}

	if err := p.errEmpty("an anchor value", tok.Literal, tok.Pos); err != nil {
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
		return nil, p.errf(p.cur.Pos, "Expected colon, but found %s", p.cur.Describe())
	}

	if _, err := p.expect(lexer.DOLLAR); err != nil {
		return nil, p.errf(p.cur.Pos, "expected param field operator $, but found %s", p.cur.Describe())
	}

	tok, err = p.expect(lexer.LITERAL)

	if err != nil {
		return nil, err
	}

	r.param = tok

	return &r, nil
}
