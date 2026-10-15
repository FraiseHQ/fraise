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

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/index/nlp/stopwords"
	"github.com/FraiseHQ/fraise/internal/query/lexer"
	"golang.org/x/text/language"
)

// Warning is a parse-time note about a query that runs anyway. The query has
// one reading, but part of it breaks a spelling rule or has nothing to act on.
// Warnings are returned beside the query, never stored on it. The plan cache
// substitutes query objects on a hash hit, so state on the query would leak
// between requests.
type Warning struct {
	Msg string
	Pos lexer.Position
}

// String renders the warning as clients receive it. It mirrors Error's
// "parse error at column N" shape, so a position reads the same in both.
func (w Warning) String() string {
	return fmt.Sprintf("parse warning at column %d: %s", w.Pos.Column, w.Msg)
}

// Error is a parse failure at a position in the query. The query layer returns
// it wrapped. Recover it with errors.As to get the position.
type Error struct {
	Msg string
	Pos lexer.Position
}

func (e *Error) Error() string {
	return fmt.Sprintf("parse error at column %d: %s", e.Pos.Column, e.Msg)
}

type parser[K comparable, P float32 | float64] struct {
	l      *lexer.Lexer
	cur    lexer.Token
	peek   lexer.Token
	warns  []Warning
	params map[string][]P
}

// Parse parses q into its command. Vectors travel out of band in params, keyed
// by placeholder name, so vec:$v reads params["v"]. The command's Vector is
// then the vector the query names. A placeholder with no entry leaves the
// vector nil. Rejecting that is the query layer's call, beside its other
// limits.
func Parse[K comparable, P float32 | float64](q string, params map[string][]P) (cmd CommandNode, warns []Warning, err error) {
	p := &parser[K, P]{l: lexer.New(q), params: params}
	// prime cur and peek
	p.next()
	p.next()

	command, err := p.parseQuery()

	if err != nil {
		return nil, p.warns, err
	}

	// A query is one instruction, and a newline ends it. Trailing blank lines,
	// and the blanks on them, are skipped. Any other text after the command is
	// a second instruction, and is rejected.
	for p.cur.Type == lexer.NEWLINE || p.cur.Type == lexer.WHITESPACE {
		p.next()
	}

	if p.cur.Type != lexer.EOL {
		return nil, p.warns, p.errf(p.cur.Pos, "unexpected %s after end of query (one command per instruction)", p.cur.Describe())
	}

	return command, p.warns, nil
}

// cursor: everything that reads or moves the two-token window, and
// nothing that decides what a query means.

// next advances the two-token window by one token.
func (p *parser[K, P]) next() {
	p.cur = p.peek
	p.peek = p.l.Next()
}

// take consumes the current token, whatever its type. Clause value slots use it
// so any token there reaches the clause's own converter: a keyword, a '-', or
// end of input. The converter's error names the clause and the text it could
// not read. Using expect(LITERAL) instead would reject "top" as not a literal,
// without naming the clause, though to the caller "top" is one. A slot rejects
// one token before converting: an unclosed quote. The lexer hands it over with
// the text read so far. A converter would accept that text as the value the
// quote never finished, reading since:'7d as since:7d.
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

// afterBlank returns the token a reader meets next, past any blank. That is the
// current token, or the one after it when the current one is whitespace. The
// lexer folds a run of blanks into one token, so one step is enough. It only
// looks, and the window does not move.
func (p *parser[K, P]) afterBlank() lexer.Token {
	if p.cur.Type == lexer.WHITESPACE {
		return p.peek
	}
	return p.cur
}

// isAtEnd reports whether the current token ends the command's clause list.
// That is end of input, or the newline that would start a second instruction.
// Every loop that reads clauses stops here. So Parse enforces the
// "one command per instruction" rule once, rather than each loop separately.
func (p *parser[K, P]) isAtEnd() bool {
	return p.cur.Type == lexer.EOL || p.cur.Type == lexer.NEWLINE
}

// isValue reports whether the current token can stand in value position: a
// recall term, or an anchor's value after its ':'. A reserved word can, but
// only when no ':' follows it, because keyword-colon is always a clause. That
// tie-breaker tells "recall topic:billing" apart from "recall topic". The first
// is a recall seeded by an anchor. The second writes a reserved word as a term,
// which parseTerms rejects.
//
// The two positions share this one definition on purpose. A second copy of the
// rule is how they would come to disagree about what a value is.
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
// nothing after it. No clause can be completed from there. So it is a word the
// caller forgot to quote, not a clause they abandoned.
func (p *parser[K, P]) isDanglingKeyword() bool {
	return p.cur.Type.IsKeyword() && (p.peek.Type == lexer.EOL || p.peek.Type == lexer.NEWLINE)
}

// errors and warnings: diagnosis builders, kept together so the repair
// instructions they give stay consistent with each other.

// errf builds a positioned parse error. pos must be the Pos of the token the
// message blames, which is the token's last character. The lexer's CurrentPos
// would point a token further right, because cur and peek read ahead.
func (p *parser[K, P]) errf(pos lexer.Position, format string, args ...any) error {
	return &Error{
		Pos: pos,
		Msg: fmt.Sprintf(format, args...),
	}
}

// errUnexpected builds the error for a token no production accepts here.
// Callers are agents, and they can only make a repair the message spells out.
// So each shape callers produce gets its own diagnosis. The bare "unexpected"
// message is the fallback for the rest.
func (p *parser[K, P]) errUnexpected(tok lexer.Token) error {
	switch {
	case tok.Type == lexer.ILLEGAL:
		return p.errf(tok.Pos, "unterminated quoted phrase")
	case tok.Type == lexer.LPAREN, tok.Type == lexer.RPAREN:
		return p.errf(tok.Pos, "grouping is not supported: %s has no meaning in a query — there are no boolean operators to group, and terms are already a union", tok.Describe())
	case tok.Type == lexer.NUL:
		return p.errf(tok.Pos, "a NUL character is only allowed inside a quoted phrase")
	case tok.Type == lexer.SPECIAL, tok.Type == lexer.PLUS, tok.Type == lexer.TILDE, tok.Type == lexer.MINUS:
		return p.errf(tok.Pos, "%s is only allowed inside a quoted phrase", tok.Describe())
	case tok.Type == lexer.NEWLINE:
		return p.errf(tok.Pos, "unexpected %s: one command per instruction", tok.Describe())
	case tok.Type == lexer.COLON:
		return p.errf(tok.Pos, "stray %s: a ':' only joins a clause keyword to its value", tok.Describe())
	case tok.Type.IsCommand():
		return p.errf(tok.Pos, "%s starts a second command: one command per instruction", tok.Describe())
	case tok.Type.IsKeyword():
		return p.errKeywordAsClause(tok)
	case tok.IsMisCasedKeyword():
		return p.errf(tok.Pos, "mis-cased keyword %q: keywords are lower case — write %s:%s if a clause was meant, or quote it ('%s') to search for the word",
			tok.Literal, strings.ToLower(tok.Literal), p.clausePlaceholder(tok), tok.Literal)
	default:
		return p.errf(tok.Pos, "unexpected %s", tok.Describe())
	}
}

// errKeywordAsClause rejects a reserved word where a clause must start, without
// the ':' that would make it one, as in "recall x topic:y top". It gets the
// same repair as a mis-cased keyword: add the colon or quote the word. It does
// not just complain that the colon is missing. Such a word is more often one
// the caller meant to search for than a clause abandoned mid-write.
//
// A command word or a prefix gets no clause to suggest. recall:<value> and
// explain:<value> are themselves errors. Suggesting them would send the caller
// from one rejection to the next.
func (p *parser[K, P]) errKeywordAsClause(tok lexer.Token) error {
	if tok.Type.IsCommand() {
		return p.errf(tok.Pos, "%s starts a second command: one command per instruction — quote it ('%s') to search for the word",
			tok.Describe(), tok.Literal)
	}
	if tok.Type.IsPrefix() {
		return p.errf(tok.Pos, "%s is a prefix and starts no clause here: quote it ('%s') to search for the word",
			tok.Describe(), tok.Literal)
	}
	return p.errf(tok.Pos, "%s is a keyword and starts no clause here: write %s:%s if a clause was meant, or quote it ('%s') to search for the word",
		tok.Describe(), strings.ToLower(tok.Literal), p.clausePlaceholder(tok), tok.Literal)
}

// errKeywordAsTerm rejects a reserved word among a recall's terms. The word is
// one ':' from a filter, as in "recall x since 7d" against "recall x since:7d".
// Either reading, if guessed, answers a differently scoped question, and
// nothing in the response says so. So the message names both repairs and leaves
// the choice to the caller. The filter repair is spelled with value, the word
// the caller wrote after the keyword (see clauseValue). That makes it the query
// they meant, not a template to fill. A command word or a prefix has no filter
// reading, because recall:<value> and explain:<value> are themselves errors.
// Its message names only the quote.
func (p *parser[K, P]) errKeywordAsTerm(tok lexer.Token, value string) error {
	if tok.Type.IsCommand() {
		return p.errf(tok.Pos, "term %q is also a command: quote it ('%s') to search for the word", tok.Literal, tok.Literal)
	}
	if tok.Type.IsPrefix() {
		return p.errf(tok.Pos, "term %q is also a prefix: quote it ('%s') to search for the word", tok.Literal, tok.Literal)
	}
	return p.errf(tok.Pos, "term %q is also a keyword: write %s:%s if a filter was meant, or quote it ('%s') to search the word",
		tok.Literal, strings.ToLower(tok.Literal), value, tok.Literal)
}

// errMissingColon rejects a clause keyword written with a value but without the
// ':' between them. Past the terms a keyword can only start a clause. So the
// message names that clause, spelled with the value written after it. It offers
// no quote, because a quoted word there is itself an error.
func (p *parser[K, P]) errMissingColon(key lexer.Token, value string) error {
	return p.errf(key.Pos, "%s is missing its ':': write %s:%s", key.Describe(), strings.ToLower(key.Literal), value)
}

// clauseValue spells tok as a repair should write it after key's ':'. A word
// stays as written, and a phrase gets its quotes back. Any other token becomes
// key's placeholder (see clausePlaceholder). A caller who wrote "since 7d" is
// offered since:7d. Two cases differ, so the repair is never itself rejected:
//   - vec always gets its placeholder. It takes a parameter reference, never a
//     value written inline, so vec:3 for "vec 3" would be rejected too.
//   - A quoted duration after since or until loses its quotes. since:'7d' is
//     rejected as a quoted duration, so "since '7d'" is offered since:7d, the
//     one spelling a duration has.
func (p *parser[K, P]) clauseValue(key, tok lexer.Token) string {
	kind := lexer.KeywordsMap[strings.ToLower(key.Literal)]
	switch {
	case kind == lexer.VEC:
		return p.clausePlaceholder(key)
	case tok.Type == lexer.LITERAL:
		return tok.Literal
	case tok.Type == lexer.PHRASE && (kind == lexer.SINCE || kind == lexer.UNTIL):
		if t, err := containers.ParseTimeValue[K](tok.Literal); err == nil {
			if _, relative := t.(containers.RelativeTime[K]); relative {
				return tok.Literal
			}
		}
		return quote(tok.Literal)
	case tok.Type == lexer.PHRASE:
		return quote(tok.Literal)
	default:
		return p.clausePlaceholder(key)
	}
}

// clausePlaceholder is what a repair writes after key's ':' when it has no
// value to spell back. That is $<name> for vec, whose value is a parameter
// reference, and <value> for every other clause. Offering vec:<value> would
// lead the caller to write an inline value, which vec rejects. The key may be
// mis-cased, so it is matched by spelling rather than by type.
func (p *parser[K, P]) clausePlaceholder(key lexer.Token) string {
	if lexer.KeywordsMap[strings.ToLower(key.Literal)] == lexer.VEC {
		return "$<name>"
	}
	return "<value>"
}

// errRecallClauseOnWrite rejects a well-formed recall clause on a remember. The
// keyword-as-clause repair ("write since:<value>") would repeat what the caller
// wrote. The mistake is the command, so the message lists the clauses a
// remember takes.
func (p *parser[K, P]) errRecallClauseOnWrite(tok lexer.Token) error {
	return p.errf(tok.Pos, "%s: is a recall clause: a remember takes only topic:, entity: and vec:", strings.ToLower(tok.Literal))
}

// errDuplicate rejects a single-valued clause given twice. Letting the last one
// win would run a differently scoped query than the one asked, without saying
// so. The message names the clause, so the caller knows which one to drop.
// Anchors are exempt, because a repeated topic: or entity: is a list.
func (p *parser[K, P]) errDuplicate(tok lexer.Token) error {
	clause := strings.ToLower(tok.Literal)
	return p.errf(tok.Pos, "duplicate %s clause: %s may be given only once — drop one", clause, clause)
}

// warnMisCasedKeyword flags a clause whose keyword is not written in lower
// case. The ':' leaves the clause one reading, so it runs. But accepting the
// spelling in silence would never teach the caller that keywords are lower
// case. The warning says which clause ran. Only the clause key is checked. An
// anchor's value is data, and entity:Top is the same anchor as entity:top.
func (p *parser[K, P]) warnMisCasedKeyword(tok lexer.Token) {
	lower := strings.ToLower(tok.Literal)
	if !tok.Type.IsKeyword() || tok.Literal == lower {
		return
	}
	p.warns = append(p.warns, Warning{
		Msg: fmt.Sprintf("keyword %q is not lower case: it ran as the %s clause, but keywords are syntax and are written lower case",
			tok.Literal, lower),
		Pos: tok.Pos,
	})
}

// warnDepthWithoutGraph flags a depth above 0 on a recall that names no anchor.
// The graph is entered only through a topic: or entity: anchor. Such a recall
// runs on the text and vector indices alone, so the depth has no effect. The
// query still runs. The warning says the clause did nothing, and what would
// give it an effect. Neither depth:0 nor an omitted depth warns. A depth of 0
// asks for nothing the recall cannot do. An omitted depth is the operator's
// default, not the caller's choice.
func (p *parser[K, P]) warnDepthWithoutGraph(r *RecallCommandNode[K, P]) {
	if !r.HasDepth() || r.depth.value == 0 || len(r.topics) > 0 || len(r.entities) > 0 {
		return
	}
	p.warns = append(p.warns, Warning{
		Msg: fmt.Sprintf("%s has no effect: the graph is searched only through a topic:/entity: anchor and this recall names none, so it runs on the text and vector indices alone", r.depth.String()),
		Pos: r.depth.key.Pos,
	})
}

// warnStopWords flags each bare term that is an English stop word, and rejects
// a recall left with nothing to search. Facts lose their stop words on the way
// into the index, through CleanContent with the same English tag. So a bare
// stop word can never match, and it warns at the term. If every bare term is a
// stop word, and the recall has no phrase and no vector, nothing can match at
// all. That is an error, not an empty result that reads like a miss. Named
// anchors do not change it, because the query that was meant is the anchor
// alone. A phrase is a seed whatever its words.
func (p *parser[K, P]) warnStopWords(r *RecallCommandNode[K, P]) error {
	var stops []lexer.Token
	seeded := r.vec != nil
	for _, t := range r.terms {
		term, ok := t.(TermNode)
		if !ok || term.token.Type != lexer.LITERAL || stopwords.CleanContent(term.token.Literal, language.English) != "" {
			seeded = true
			continue
		}
		stops = append(stops, term.token)
	}
	if len(stops) == 0 {
		return nil
	}
	const fix = "so nothing can match; give a term that is not a stop word, a phrase, or a vector"
	switch {
	case !seeded && len(stops) == 1:
		return p.errf(stops[0].Pos, "term %q is a stop word and the only search term: stored facts never contain it, %s", stops[0].Literal, fix)
	case !seeded:
		return p.errf(stops[0].Pos, "term %q is a stop word, and so is every other search term: stored facts never contain them, %s", stops[0].Literal, fix)
	}
	for _, tok := range stops {
		p.warns = append(p.warns, Warning{
			Msg: fmt.Sprintf("term %q is a stop word: stored facts never contain it, so it cannot match", tok.Literal),
			Pos: tok.Pos,
		})
	}
	return nil
}

// errEmpty builds the error for an empty or whitespace-only value, and returns
// nil for any other. Only quoting can write one, and a caller never means it.
// An empty fact can never be retrieved. An empty anchor is an identity nobody
// can name a second time. Both would corrupt a graph quietly, instead of
// failing where they were made.
func (p *parser[K, P]) errEmpty(role, value string, pos lexer.Position) error {
	if strings.TrimSpace(value) != "" {
		return nil
	}
	return p.errf(pos, "%s must not be empty", role)
}

// productions: the grammar itself, from the command down to the leaves a clause
// is built from.

func (p *parser[K, P]) parseQuery() (CommandNode, error) {
	switch p.cur.Type {
	case lexer.REMEMBER:
		return (*p).parseRemember()
	case lexer.RECALL:
		return (*p).parseRecall()
	default:
		return nil, p.errf(p.cur.Pos, "expected a command (recall, remember), found %s", p.cur.Describe())
	}
}

func (p *parser[K, P]) parseRemember() (*RememberCommandNode[P], error) {

	r := RememberCommandNode[P]{}

	r.key = p.cur
	p.next()

	selector, err := p.parseSelector(r.key)
	if err != nil {
		return nil, err
	}
	r.selector = selector

	// A remember carries exactly one quoted phrase, the fact. The lexer returns
	// the whole '...' as one PHRASE token, so consuming it also consumes the
	// closing quote. There is no separate delimiter to handle here.
	phrase, err := p.parsePhrase()

	if err != nil {
		return nil, err
	}
	r.value = *phrase

	var anchors []AnchorFieldNode

	for !p.isAtEnd() {
		if p.isDanglingKeyword() {
			return nil, p.errKeywordAsClause(p.cur)
		}
		p.warnMisCasedKeyword(p.cur)
		switch p.cur.Type {
		case lexer.ENTITY, lexer.TOPIC:
			key, tok, value, err := p.parseAnchorField()
			if err != nil {
				return nil, err
			}
			var field FieldNode[string]
			if key.Type == lexer.TOPIC {
				field = TopicFieldNode{key: key, token: tok, value: value}
			} else {
				field = EntityFieldNode{key: key, token: tok, value: value}
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
		case lexer.SINCE, lexer.UNTIL, lexer.TOP, lexer.DEPTH:
			if p.peek.Type == lexer.COLON {
				return nil, p.errRecallClauseOnWrite(p.cur)
			}
			return nil, p.errUnexpected(p.cur)
		case lexer.WHITESPACE:
			p.next()
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

	selector, err := p.parseSelector(r.key)
	if err != nil {
		return nil, err
	}
	r.selector = selector

	terms, err := p.parseTerms()
	if err != nil {
		return nil, err
	}
	r.terms = terms

	// Clauses follow the terms. A modifier is single-valued, so a repeat is
	// rejected. An anchor is a list, so a repeat adds to it.
	for !p.isAtEnd() {
		if p.isDanglingKeyword() {
			return nil, p.errKeywordAsClause(p.cur)
		}
		p.warnMisCasedKeyword(p.cur)
		switch p.cur.Type {
		case lexer.ENTITY:
			key, tok, value, err := p.parseAnchorField()
			if err != nil {
				return nil, err
			}
			r.entities = append(r.entities, AnchorFieldNode{field: EntityFieldNode{key: key, token: tok, value: value}})
		case lexer.TOPIC:
			key, tok, value, err := p.parseAnchorField()
			if err != nil {
				return nil, err
			}
			r.topics = append(r.topics, AnchorFieldNode{field: TopicFieldNode{key: key, token: tok, value: value}})
		case lexer.UNTIL:
			if r.until.key.Type == lexer.UNTIL {
				return nil, p.errDuplicate(p.cur)
			}
			key, tok, t, err := p.parseTimeValue()
			if err != nil {
				return nil, err
			}
			r.until = UntilFieldNode[K]{key: key, token: tok, value: t}
		case lexer.SINCE:
			if r.since.key.Type == lexer.SINCE {
				return nil, p.errDuplicate(p.cur)
			}
			key, tok, t, err := p.parseTimeValue()
			if err != nil {
				return nil, err
			}
			r.since = SinceFieldNode[K]{key: key, token: tok, value: t}
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
		case lexer.WHITESPACE:
			p.next()
		default:
			return nil, p.errUnexpected(p.cur)
		}
	}

	// A recall needs a seed to start the search from: a term, an anchor or a
	// vector. Anchors seed the search, so "everything about billing" is a
	// well-formed question. A query of modifiers alone scopes a search that
	// never starts. It would return an empty result as though it had asked
	// something.
	if len(r.terms) == 0 && len(r.topics) == 0 && len(r.entities) == 0 && r.vec == nil {
		return nil, p.errf(r.key.Pos, "a recall needs at least one seed: a term, a topic:/entity: anchor, or vec:$<name>")
	}

	if err := p.warnStopWords(&r); err != nil {
		return nil, err
	}

	p.warnDepthWithoutGraph(&r)

	return &r, nil
}

// parseSelector reads what follows a command verb: an optional graph selector
// glued to it, then the space that ends the command. A space before the '@'
// ("recall @3") is reported as a space. A second selector ("recall@3@5") is
// reported as a second selector. Neither surfaces as a token the next
// production cannot use. A command with no selector gets the zero node. String
// prints nothing for it, and the handler reads it as the default graph, 0.
func (p *parser[K, P]) parseSelector(cmd lexer.Token) (GraphSelectorNode, error) {
	var selector GraphSelectorNode
	if p.cur.Type == lexer.AT {
		key, value, err := p.parseGraphSelector()
		if err != nil {
			return GraphSelectorNode{}, err
		}
		selector = GraphSelectorNode{key: key, value: value}
		if p.cur.Type == lexer.AT {
			return GraphSelectorNode{}, p.errf(p.cur.Pos, "unexpected %s: a command takes one selector", p.cur.Describe())
		}
	}

	if _, err := p.expect(lexer.WHITESPACE); err != nil {
		return GraphSelectorNode{}, p.errf(p.cur.Pos, "expected a space after the command, found %s", p.cur.Describe())
	}
	if p.cur.Type == lexer.AT {
		return GraphSelectorNode{}, p.errf(p.cur.Pos, "no space allowed between %s and @", cmd.Literal)
	}
	return selector, nil
}

// parseTerms reads a recall's leading term list, which may be empty. Terms are
// folded to lower case, because query data is matched without regard to case
// everywhere. Folding at the edge keeps every downstream spelling of the query
// on one form, the index lookup and the plan-cache key included.
//
// A reserved word is never a bare term. Followed by ':', it starts a clause and
// ends the terms (isValue owns that rule). Otherwise errKeywordAsTerm rejects
// it. "recall x since 7d" is one ':' from "recall x since:7d", and neither
// reading can be guessed safely. A ':' after a blank is reported as a space
// inside the clause. Quoting is how a caller searches for the word. A mis-cased
// keyword is rejected too. Read as a term, "Since" would turn
// "recall x Since 7d" into a three-term search.
func (p *parser[K, P]) parseTerms() ([]LiteralFieldNode, error) {
	var terms []LiteralFieldNode
	for !p.isAtEnd() {
		switch {
		case p.cur.Type == lexer.WHITESPACE:
			p.next()
		case p.cur.IsMisCasedKeyword():
			return nil, p.errUnexpected(p.cur)
		case !p.isValue():
			// Terms end at a clause, or at a token the clause loop diagnoses.
			return terms, nil
		case p.cur.Type.IsKeyword():
			key := p.take()
			if p.cur.Type == lexer.WHITESPACE && p.peek.Type == lexer.COLON {
				return nil, p.errf(p.cur.Pos, "no space allowed before :")
			}
			return nil, p.errKeywordAsTerm(key, p.clauseValue(key, p.afterBlank()))
		default:
			tok := p.take()
			if err := p.errEmpty("a search term", tok.Literal, tok.Pos); err != nil {
				return nil, err
			}
			terms = append(terms, TermNode{token: tok, value: strings.ToLower(tok.Literal)})
		}
	}
	return terms, nil
}

// parseSeparator consumes the ':' that joins a clause keyword to its value. key
// has just been taken. The ':' is required and checked, never skipped. Skipping
// it would shift every later token into another role, and run a different query
// instead of failing. A blank after the keyword gets its diagnosis from the
// token after it:
//   - Before a ':', it is a space inside the clause.
//   - Before a value, it is a missing ':' (errMissingColon).
//   - Before the end, it is a dangling keyword (errKeywordAsClause).
//
// Any other token glued to the keyword in place of the ':' is diagnosed the
// same way, as a missing ':' or a dangling keyword. A blank after the ':' is a
// space inside the clause too.
func (p *parser[K, P]) parseSeparator(key lexer.Token) error {
	if p.cur.Type == lexer.WHITESPACE {
		switch p.peek.Type {
		case lexer.COLON:
			return p.errf(p.cur.Pos, "no space allowed before :")
		case lexer.EOL, lexer.NEWLINE:
			return p.errKeywordAsClause(key)
		default:
			return p.errMissingColon(key, p.clauseValue(key, p.peek))
		}
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		if p.cur.Type.IsEndOfLine() {
			return p.errKeywordAsClause(key)
		}
		return p.errMissingColon(key, p.clauseValue(key, p.cur))
	}
	if p.cur.Type == lexer.WHITESPACE {
		return p.errf(p.cur.Pos, "no space allowed after :")
	}
	return nil
}

// parseIntField consumes a depth: or top: clause. One function serves both, as
// parseTimeValue does for since: and until:. So the two cannot drift apart in
// how they read a value or name it back in an error. The returned key says
// which clause was written.
func (p *parser[K, P]) parseIntField() (lexer.Token, int, error) {
	key := p.cur

	p.next()

	if err := p.parseSeparator(key); err != nil {
		return lexer.Token{}, 0, err
	}

	tok := p.take()
	if tok.Type == lexer.ILLEGAL {
		return lexer.Token{}, 0, p.errUnexpected(tok)
	}

	// A value too large to hold is reported apart from one that is not a number
	// at all. An agent told only "invalid" retries with another huge number.
	// One told the value is out of range knows to shrink it. The query layer
	// checks the configured ceilings (db.max-top, db.max-depth). Here the value
	// only has to fit in an int.
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

// parseTimeValue consumes a since: or until: clause. A duration too long to
// hold is reported with the largest count its unit allows.
func (p *parser[K, P]) parseTimeValue() (lexer.Token, lexer.Token, containers.TimeValue[K], error) {
	key := p.cur

	p.next()

	if err := p.parseSeparator(key); err != nil {
		return lexer.Token{}, lexer.Token{}, nil, err
	}

	tok := p.take()
	if tok.Type == lexer.ILLEGAL {
		return lexer.Token{}, lexer.Token{}, nil, p.errUnexpected(tok)
	}

	t, err := containers.ParseTimeValue[K](tok.Literal)
	var rangeErr *containers.DurationRangeError
	switch {
	case errors.As(err, &rangeErr):
		return lexer.Token{}, lexer.Token{}, nil, p.errf(tok.Pos, "invalid %s value %s: out of range (at most %d%c)", strings.ToLower(key.Literal), tok.Describe(), rangeErr.Max, rangeErr.Unit)
	case err != nil:
		return lexer.Token{}, lexer.Token{}, nil, p.errf(tok.Pos, "invalid %s value %s: expected a duration like 7d or a quoted date like '2026-01-15'", strings.ToLower(key.Literal), tok.Describe())
	}

	// Only an absolute time is quoted, because it carries a '-'. A quoted
	// duration is rejected rather than read. Accepting since:'7d' would give
	// one clause two spellings, and the quote is how the grammar tells a date
	// from a duration.
	if _, relative := t.(containers.RelativeTime[K]); relative && tok.Type == lexer.PHRASE {
		clause := strings.ToLower(key.Literal)
		return lexer.Token{}, lexer.Token{}, nil, p.errf(tok.Pos, "invalid %s value %s: a duration is not quoted — write %s:%s", clause, tok.Describe(), clause, tok.Literal)
	}

	return key, tok, t, nil
}

func (p *parser[K, P]) parseGraphSelector() (lexer.Token, uint8, error) {

	key := p.cur

	p.next()

	// The number is glued to its '@', as the '@' is to the verb. A space before
	// a number is reported as a space, since that number is the selector the
	// caller meant. Before anything else there is no selector at all, and the
	// whole-number check below says so.
	if p.cur.Type == lexer.WHITESPACE && p.peek.Type == lexer.LITERAL {
		if _, err := strconv.Atoi(p.peek.Literal); err == nil {
			return lexer.Token{}, 0, p.errf(p.cur.Pos, "no space allowed between @ and the selector")
		}
	}

	tok := p.take()

	// Validate the full integer before narrowing it to uint8. A plain uint8(i)
	// would wrap an out-of-range selector into a valid-looking graph
	// (@256 -> 0, @300 -> 44). The query would then run silently against
	// another tenant's graph. The handler enforces the tighter [0, num-graphs)
	// bound on what passes here.
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
// has already stripped the quotes and decoded each doubled single quote.
func (p *parser[K, P]) parsePhrase() (*PhraseNode, error) {
	// An ILLEGAL token here means the input ended before the closing quote.
	if p.cur.Type == lexer.ILLEGAL {
		return nil, p.errf(p.cur.Pos, "unterminated quoted phrase")
	}
	// A NUL where the fact should start is rejected for itself, not reported
	// as a missing phrase.
	if p.cur.Type == lexer.NUL {
		return nil, p.errUnexpected(p.cur)
	}

	tok, err := p.expect(lexer.PHRASE)

	if err != nil {
		return nil, p.errf(p.cur.Pos, "expected a quoted phrase, but found %s", p.cur.Describe())
	}

	if err := p.errEmpty("a remembered fact", tok.Literal, tok.Pos); err != nil {
		return nil, err
	}

	if !p.cur.Type.IsBlank() && !p.cur.Type.IsEndOfLine() {
		return nil, p.errf(p.cur.Pos, "expected a whitespace, found %s", p.cur.Describe())
	}

	return &PhraseNode{value: tok.Literal, pos: tok.Pos}, nil
}

// parseValue consumes an anchor's value, or says why the current token is not
// one. The rule lives in isValue. A keyword is data here: entity:top files
// under the word "top". But keyword-colon is always a field. So a clause
// mistyped into value position stays an error, instead of being swallowed as
// data. Quoting covers anything the rule cannot express.
func (p *parser[K, P]) parseValue() (lexer.Token, error) {
	if p.isValue() {
		return p.take(), nil
	}
	// A token with a diagnosis of its own keeps it: an unclosed quote, a
	// parenthesis, a special character. It is not reported as the wrong kind of
	// value.
	switch p.cur.Type {
	case lexer.ILLEGAL, lexer.LPAREN, lexer.RPAREN, lexer.NEWLINE, lexer.NUL,
		lexer.SPECIAL, lexer.PLUS, lexer.TILDE, lexer.MINUS:
		return p.cur, p.errUnexpected(p.cur)
	}
	return p.cur, p.errf(p.cur.Pos, "expected a word or quoted phrase, but found %s", p.cur.Describe())
}

// parseAnchorField consumes a topic: or entity: clause.
func (p *parser[K, P]) parseAnchorField() (lexer.Token, lexer.Token, string, error) {
	key := p.cur

	p.next()

	if err := p.parseSeparator(key); err != nil {
		return lexer.Token{}, lexer.Token{}, "", err
	}

	// The anchor value is a bare word or a quoted phrase, such as
	// topic:'my project'. It is folded to lower case, because an anchor is an
	// identity, not prose. So topic:Billing and topic:billing name the same
	// anchor rather than grow two. Only the quoted fact of a remember keeps the
	// case it was written with.
	tok, err := p.parseValue()

	if err != nil {
		return lexer.Token{}, lexer.Token{}, "", err
	}

	if err := p.errEmpty("an anchor value", tok.Literal, tok.Pos); err != nil {
		return lexer.Token{}, lexer.Token{}, "", err
	}

	return key, tok, strings.ToLower(tok.Literal), nil
}

// parseVecField consumes a vec:$name clause. Its errors are returned unwrapped,
// like every clause's. So the response keeps their position and the detail a
// client can act on: the '$' or the name is missing.
func (p *parser[K, P]) parseVecField() (*VecFieldNode[P], error) {
	r := VecFieldNode[P]{}

	r.key = p.take()

	if err := p.parseSeparator(r.key); err != nil {
		return nil, err
	}

	if _, err := p.expect(lexer.DOLLAR); err != nil {
		return nil, p.errf(p.cur.Pos, "expected param field operator $, but found %s", p.cur.Describe())
	}
	if p.cur.Type == lexer.WHITESPACE {
		return nil, p.errf(p.cur.Pos, "no space allowed after $")
	}

	tok, err := p.expect(lexer.LITERAL)

	if err != nil {
		return nil, err
	}

	r.param = tok
	r.value = p.params[tok.Literal]

	return &r, nil
}
