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
	"time"
	"unicode/utf8"

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
	// nested counts the parentheses the parser is inside. A query written in
	// parentheses ends at its ')', and opens no parentheses of its own.
	nested int
}

// Parse parses q into its command. Vectors travel out of band in params, keyed
// by placeholder name, so vec:$v reads params["v"]. The command's Vector is
// then the vector the query names. A placeholder with no entry leaves the
// vector nil. Rejecting that is the query layer's call, beside its other
// limits.
func Parse[K comparable, P float32 | float64](q string, params map[string][]P) (cmd CommandNode, warns []Warning, err error) {
	// A query is UTF-8 text. An invalid byte ranges as U+FFFD, the character
	// JSON decoding puts in place of invalid UTF-8 and of a lone surrogate, so
	// rejecting it catches all three on every path. A fact stored with one
	// would hold text its client never wrote, and could not be recalled by the
	// text it was written with.
	col := 0
	for _, r := range q {
		col++
		if r == utf8.RuneError {
			return nil, nil, &Error{Pos: lexer.Position{Column: col}, Msg: "not UTF-8 text: an invalid byte, a lone surrogate, or U+FFFD in place of one"}
		}
	}

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

// ends reports whether tok ends the command being read. That is end of input,
// the newline that would start a second instruction, or, for a query written
// in parentheses, the ')' that closes it. Outside parentheses a ')' ends
// nothing, and is diagnosed as the stray token it is.
func (p *parser[K, P]) ends(tok lexer.Token) bool {
	return tok.Type.IsEndOfLine() || (p.nested > 0 && tok.Type == lexer.RPAREN)
}

// isAtEnd reports whether the current token ends the command's clause list
// (see ends). Every loop that reads clauses stops here. So Parse enforces the
// "one command per instruction" rule once, rather than each loop separately.
func (p *parser[K, P]) isAtEnd() bool {
	return p.ends(p.cur)
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
	return p.cur.Type.IsKeyword() && p.ends(p.peek)
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
		return p.errf(tok.Pos, "grouping is not supported: %s only encloses a when in a time bound, as in until:(when <terms>) — there are no boolean operators to group, and terms are already a union", tok.Describe())
	case tok.Type == lexer.RANGE:
		return p.errf(tok.Pos, "%s only joins the two ends of a time range, as in since:'2023-05-07'..'2023-05-09': anywhere else it is only allowed inside a quoted phrase", tok.Describe())
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
// Neither has said or occurred, which are clauses of a remember and filter
// nothing on a recall. Their messages name only the quote.
func (p *parser[K, P]) errKeywordAsTerm(tok lexer.Token, value string) error {
	if tok.Type.IsCommand() {
		return p.errf(tok.Pos, "term %q is also a command: quote it ('%s') to search for the word", tok.Literal, tok.Literal)
	}
	if tok.Type.IsPrefix() {
		return p.errf(tok.Pos, "term %q is also a prefix: quote it ('%s') to search for the word", tok.Literal, tok.Literal)
	}
	if tok.Type == lexer.SAID || tok.Type == lexer.OCCURRED {
		return p.errf(tok.Pos, "term %q is also a keyword: quote it ('%s') to search for the word", tok.Literal, tok.Literal)
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
//   - A quoted duration after a time clause (since, until, said or occurred)
//     loses its quotes. since:'7d' is rejected as a quoted duration, so
//     "since '7d'" is offered since:7d, the one spelling a duration has.
func (p *parser[K, P]) clauseValue(key, tok lexer.Token) string {
	kind := lexer.KeywordsMap[strings.ToLower(key.Literal)]
	switch {
	case kind == lexer.VEC:
		return p.clausePlaceholder(key)
	case tok.Type == lexer.LITERAL:
		return tok.Literal
	case tok.Type == lexer.PHRASE && (kind == lexer.SINCE || kind == lexer.UNTIL || kind == lexer.SAID || kind == lexer.OCCURRED):
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
	return p.errf(tok.Pos, "%s: is a recall clause: a remember takes only topic:, entity:, vec:, said: and occurred:", strings.ToLower(tok.Literal))
}

// errRememberClauseOnRead rejects said or occurred where a clause of a recall
// or a when would start. Like its counterpart on a remember, the mistake is the
// command, not the spelling. The clause gives a fact its time when the fact is
// written. So the message says where the clause belongs, and names the clauses
// that bound a search in time. It holds with or without the ':', and the
// message never offers said:<value>. That repair is this same rejection, one
// step later.
//
// With nothing after it the word can finish no clause on any command. It is
// then the word the caller forgot to quote, and the message names the quote.
func (p *parser[K, P]) errRememberClauseOnRead(tok lexer.Token) error {
	if p.isDanglingKeyword() {
		return p.errf(tok.Pos, "%s is a keyword and starts no clause here: quote it ('%s') to search for the word", tok.Describe(), tok.Literal)
	}
	return p.errf(tok.Pos, "%s: is a remember clause: it gives a fact its time, and a search is bounded with since: and until:", strings.ToLower(tok.Literal))
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
// anchor's value is data, and entity:Top is the same anchor as entity:top. A
// when glued to its clock has one reading for the same reason, and its warning
// says which command ran.
func (p *parser[K, P]) warnMisCasedKeyword(tok lexer.Token) {
	lower := strings.ToLower(tok.Literal)
	if !tok.Type.IsKeyword() || tok.Literal == lower {
		return
	}
	role := "clause"
	if tok.Type.IsCommand() {
		role = "command"
	}
	p.warns = append(p.warns, Warning{
		Msg: fmt.Sprintf("keyword %q is not lower case: it ran as the %s %s, but keywords are syntax and are written lower case",
			tok.Literal, lower, role),
		Pos: tok.Pos,
	})
}

// warnDepthWithoutGraph flags a depth above 0 on a recall that names no anchor.
// The graph is entered only through a topic: or entity: anchor. Such a recall
// runs on the text and vector indices alone, so the depth has no effect. The
// query still runs. The warning says the clause did nothing, and what would
// give it an effect. Neither depth:0 nor an omitted depth warns. A depth of 0
// asks for nothing the recall cannot do. An omitted depth is the operator's
// default, not the caller's choice. cmd is the command the body belongs to,
// which the warning names.
func (p *parser[K, P]) warnDepthWithoutGraph(cmd lexer.Token, r *RecallBodyNode[K, P]) {
	if !r.HasDepth() || r.depth.value == 0 || len(r.topics) > 0 || len(r.entities) > 0 {
		return
	}
	p.warns = append(p.warns, Warning{
		Msg: fmt.Sprintf("%s has no effect: the graph is searched only through a topic:/entity: anchor and this %s names none, so it runs on the text and vector indices alone", r.depth.String(), strings.ToLower(cmd.Literal)),
		Pos: r.depth.key.Pos,
	})
}

// warnAnchorsWithoutDepth flags depth:0 on a recall that names an anchor beside
// a term or vector. Such an anchor takes part only through the excess it
// transmits over the graph, which depth 0 does not search, so the recall runs
// at depth 1 instead (see [RecallBodyNode.Depth]). The query still runs. The
// warning says which depth ran. An omitted depth does not warn: the operator's
// default is raised in silence, since the caller chose nothing.
func (p *parser[K, P]) warnAnchorsWithoutDepth(r *RecallBodyNode[K, P]) {
	if !r.HasDepth() || r.depth.value != 0 || !r.anchoredSearch() {
		return
	}
	p.warns = append(p.warns, Warning{
		Msg: fmt.Sprintf("%s ran as depth:1: a topic:/entity: anchor beside a term or vector contributes through the graph, which depth:0 does not search", r.depth.String()),
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
func (p *parser[K, P]) warnStopWords(r *RecallBodyNode[K, P]) error {
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

// parseQuery reads one command. It serves the top of an instruction and the
// inside of a time bound's parentheses alike, so a query reads the same
// wherever it stands. What a position accepts is checked by its caller, on the
// command returned.
func (p *parser[K, P]) parseQuery() (CommandNode, error) {
	switch p.cur.Type {
	case lexer.REMEMBER:
		return (*p).parseRemember()
	case lexer.RECALL:
		return (*p).parseRecall()
	case lexer.WHEN:
		return (*p).parseWhen()
	default:
		return nil, p.errf(p.cur.Pos, "expected a command (recall, remember, when), found %s", p.cur.Describe())
	}
}

func (p *parser[K, P]) parseRemember() (*RememberCommandNode[K, P], error) {

	r := RememberCommandNode[K, P]{}

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
		case lexer.SAID:
			if r.said.key.Type == lexer.SAID {
				return nil, p.errDuplicate(p.cur)
			}
			key, value, err := p.parseTimeValue(r.key)
			if err != nil {
				return nil, err
			}
			// A fact is said at one moment. A range would leave the fact dated
			// by its first day alone, dropping the rest of what was written.
			if value.dots.Type == lexer.RANGE {
				return nil, p.errf(value.dots.Pos, "said: is one moment, not a range: write the day the fact was said, as in said:'2023-05-08', and give a span to occurred:")
			}
			r.said = SaidFieldNode[K]{key: key, value: value}
		case lexer.OCCURRED:
			key, value, err := p.parseTimeValue(r.key)
			if err != nil {
				return nil, err
			}
			r.occurred = append(r.occurred, OccurredFieldNode[K]{key: key, value: value})
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

	body, err := p.parseRecallBody(r.key)
	if err != nil {
		return nil, err
	}
	r.RecallBodyNode = body

	return &r, nil
}

// parseWhen reads a when: the verb, the clock glued to it, then the body of a
// recall. A when takes no graph selector. Inside a time bound it reads the
// graph of the recall that holds it, so a selector there could only disagree
// with the recall's. On its own it reads the default graph.
func (p *parser[K, P]) parseWhen() (*WhenCommandNode[K, P], error) {

	w := WhenCommandNode[K, P]{}

	w.key = p.cur
	p.warnMisCasedKeyword(w.key)
	p.next()

	clock, err := p.parseClock()
	if err != nil {
		return nil, err
	}
	w.clock = clock

	if at := p.afterBlank(); at.Type == lexer.AT {
		return nil, p.errf(at.Pos, "a when takes no graph selector: in a time bound it reads the graph of the recall that holds it, and on its own the default graph")
	}

	if _, err := p.expect(lexer.WHITESPACE); err != nil {
		return nil, p.errf(p.cur.Pos, "expected a space after the command, found %s", p.cur.Describe())
	}

	body, err := p.parseRecallBody(w.key)
	if err != nil {
		return nil, err
	}
	w.RecallBodyNode = body

	return &w, nil
}

// parseClock reads the clock glued to a when, as in when:said. A when with no
// ':' names no clock and gets the zero node. String prints nothing for it, and
// it reads as happened. A clock is one of three bare lower-case words. A
// quoted one is rejected rather than read, so a clock has one spelling.
func (p *parser[K, P]) parseClock() (ClockNode, error) {
	if p.cur.Type == lexer.WHITESPACE && p.peek.Type == lexer.COLON {
		return ClockNode{}, p.errf(p.cur.Pos, "no space allowed before :")
	}
	if p.cur.Type != lexer.COLON {
		return ClockNode{}, nil
	}

	key := p.take()
	if p.cur.Type == lexer.WHITESPACE {
		return ClockNode{}, p.errf(p.cur.Pos, "no space allowed after :")
	}

	tok := p.take()
	if tok.Type == lexer.ILLEGAL {
		return ClockNode{}, p.errUnexpected(tok)
	}

	clock, err := containers.ParseClock(tok.Literal)
	switch {
	case err != nil:
		return ClockNode{}, p.errf(tok.Pos, "invalid clock %s: a when reads said, happened or written, as in when:said", tok.Describe())
	case tok.Type == lexer.PHRASE:
		return ClockNode{}, p.errf(tok.Pos, "invalid clock %s: a clock is not quoted — write when:%s", tok.Describe(), tok.Literal)
	}

	return ClockNode{key: key, value: clock}, nil
}

// parseRecallBody reads what a recall searches by: its terms, then its clauses.
// A recall and a when share it, so the two cannot drift apart in what a search
// may say. cmd is the command the body belongs to. Its messages name it, and it
// decides whether a time bound may hold a query (see parseTimeValue).
func (p *parser[K, P]) parseRecallBody(cmd lexer.Token) (RecallBodyNode[K, P], error) {

	r := RecallBodyNode[K, P]{}

	terms, err := p.parseTerms()
	if err != nil {
		return RecallBodyNode[K, P]{}, err
	}
	r.terms = terms

	// Clauses follow the terms. A modifier is single-valued, so a repeat is
	// rejected. An anchor is a list, so a repeat adds to it.
	for !p.isAtEnd() {
		// said and occurred start no clause of a search, however they are
		// written. They are refused before the repairs below can offer one.
		if p.cur.Type == lexer.SAID || p.cur.Type == lexer.OCCURRED {
			return RecallBodyNode[K, P]{}, p.errRememberClauseOnRead(p.cur)
		}
		if p.isDanglingKeyword() {
			return RecallBodyNode[K, P]{}, p.errKeywordAsClause(p.cur)
		}
		p.warnMisCasedKeyword(p.cur)
		switch p.cur.Type {
		case lexer.ENTITY:
			key, tok, value, err := p.parseAnchorField()
			if err != nil {
				return RecallBodyNode[K, P]{}, err
			}
			r.entities = append(r.entities, AnchorFieldNode{field: EntityFieldNode{key: key, token: tok, value: value}})
		case lexer.TOPIC:
			key, tok, value, err := p.parseAnchorField()
			if err != nil {
				return RecallBodyNode[K, P]{}, err
			}
			r.topics = append(r.topics, AnchorFieldNode{field: TopicFieldNode{key: key, token: tok, value: value}})
		case lexer.UNTIL:
			if r.until.key.Type == lexer.UNTIL {
				return RecallBodyNode[K, P]{}, p.errDuplicate(p.cur)
			}
			key, value, err := p.parseTimeValue(cmd)
			if err != nil {
				return RecallBodyNode[K, P]{}, err
			}
			r.until = UntilFieldNode[K]{key: key, value: value}
		case lexer.SINCE:
			if r.since.key.Type == lexer.SINCE {
				return RecallBodyNode[K, P]{}, p.errDuplicate(p.cur)
			}
			key, value, err := p.parseTimeValue(cmd)
			if err != nil {
				return RecallBodyNode[K, P]{}, err
			}
			r.since = SinceFieldNode[K]{key: key, value: value}
		case lexer.DEPTH:
			if r.depth.key.Type == lexer.DEPTH {
				return RecallBodyNode[K, P]{}, p.errDuplicate(p.cur)
			}
			key, value, err := p.parseIntField()
			if err != nil {
				return RecallBodyNode[K, P]{}, err
			}
			r.depth = DepthFieldNode{key: key, value: value}
		case lexer.TOP:
			if r.top.key.Type == lexer.TOP {
				return RecallBodyNode[K, P]{}, p.errDuplicate(p.cur)
			}
			key, value, err := p.parseIntField()
			if err != nil {
				return RecallBodyNode[K, P]{}, err
			}
			r.top = TopFieldNode{key: key, value: value}
		case lexer.VEC:
			if r.vec != nil {
				return RecallBodyNode[K, P]{}, p.errDuplicate(p.cur)
			}
			vec, err := p.parseVecField()
			if err != nil {
				return RecallBodyNode[K, P]{}, err
			}
			r.vec = vec
		case lexer.WHITESPACE:
			p.next()
		default:
			return RecallBodyNode[K, P]{}, p.errUnexpected(p.cur)
		}
	}

	// A recall needs a seed to start the search from: a term, an anchor or a
	// vector. Anchors seed the search, so "everything about billing" is a
	// well-formed question. A query of modifiers alone scopes a search that
	// never starts. It would return an empty result as though it had asked
	// something.
	if !r.hasSeed() {
		return RecallBodyNode[K, P]{}, p.errf(cmd.Pos, "a %s needs at least one seed: a term, a topic:/entity: anchor, or vec:$<name>", strings.ToLower(cmd.Literal))
	}

	if err := p.warnStopWords(&r); err != nil {
		return RecallBodyNode[K, P]{}, err
	}

	p.warnDepthWithoutGraph(cmd, &r)
	p.warnAnchorsWithoutDepth(&r)

	return r, nil
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
		switch {
		case p.peek.Type == lexer.COLON:
			return p.errf(p.cur.Pos, "no space allowed before :")
		case p.ends(p.peek):
			return p.errKeywordAsClause(key)
		default:
			return p.errMissingColon(key, p.clauseValue(key, p.peek))
		}
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		if p.ends(p.cur) {
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
// parseTimeValue does for the time clauses. So the two cannot drift apart in
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

// parseTimeValue consumes a clause whose value is a time: since: and until: on
// a recall or a when, said: and occurred: on a remember. One function serves
// all four, so they cannot drift apart in how they read a time or name it back
// in an error. The returned key says which clause was written. A duration too
// long to hold is reported with the largest count its unit allows, and a time
// a remember gives outside the years a hit can carry with those years.
//
// cmd is the command the clause belongs to. Only a recall's bound may be a
// query in parentheses. A when holds no when, and a remember is given its
// times written out. Both are refused at the '(' before anything inside it is
// read. So however deep a query stacks its parentheses, the parser descends
// one level and no further.
func (p *parser[K, P]) parseTimeValue(cmd lexer.Token) (lexer.Token, TimeValueNode[K], error) {
	key := p.cur

	p.next()

	if err := p.parseSeparator(key); err != nil {
		return lexer.Token{}, TimeValueNode[K]{}, err
	}

	clause := strings.ToLower(key.Literal)

	if p.cur.Type == lexer.LPAREN {
		switch cmd.Type {
		case lexer.WHEN:
			return lexer.Token{}, TimeValueNode[K]{}, p.errf(p.cur.Pos, "a when holds no when: write %s: as a duration or a quoted date, as in %s:'2023-05-07'", clause, clause)
		case lexer.REMEMBER:
			return lexer.Token{}, TimeValueNode[K]{}, p.errf(p.cur.Pos, "%s: takes a time written out, as in %s:'2023-05-07': a when only bounds a recall, in since: and until:", clause, clause)
		}
		subquery, err := p.parseSubquery(key)
		if err != nil {
			return lexer.Token{}, TimeValueNode[K]{}, err
		}
		return key, TimeValueNode[K]{subquery: subquery}, nil
	}

	// A when written without its parentheses would otherwise be reported as a
	// malformed date, which names no repair.
	if p.cur.Type == lexer.WHEN && cmd.Type == lexer.RECALL {
		return lexer.Token{}, TimeValueNode[K]{}, p.errf(p.cur.Pos, "invalid %s value %s: a when is written in parentheses — write %s:(when <terms>)", clause, p.cur.Describe(), clause)
	}

	tok := p.take()
	if tok.Type == lexer.ILLEGAL {
		return lexer.Token{}, TimeValueNode[K]{}, p.errUnexpected(tok)
	}

	t, err := containers.ParseTimeValue[K](tok.Literal)
	var rangeErr *containers.DurationRangeError
	switch {
	case errors.As(err, &rangeErr):
		return lexer.Token{}, TimeValueNode[K]{}, p.errf(tok.Pos, "invalid %s value %s: out of range (at most %d%c)", clause, tok.Describe(), rangeErr.Max, rangeErr.Unit)
	case err != nil:
		return lexer.Token{}, TimeValueNode[K]{}, p.errf(tok.Pos, "invalid %s value %s: expected a duration like 7d or a quoted date like '2026-01-15'", clause, tok.Describe())
	}

	// The quote is how the grammar tells a date from a duration, so each has
	// one spelling. A quoted duration is rejected rather than read, because
	// accepting since:'7d' would give one clause two spellings. A bare date is
	// rejected too. Only a year can be written bare at all, since a longer date
	// carries a '-', and since:2023 beside since:'2023-05' would be a second
	// spelling of its own.
	_, relative := t.(containers.RelativeTime[K])
	switch {
	case relative && tok.Type == lexer.PHRASE:
		return lexer.Token{}, TimeValueNode[K]{}, p.errf(tok.Pos, "invalid %s value %s: a duration is not quoted — write %s:%s", clause, tok.Describe(), clause, tok.Literal)
	case !relative && tok.Type != lexer.PHRASE:
		return lexer.Token{}, TimeValueNode[K]{}, p.errf(tok.Pos, "invalid %s value %s: expected a duration like 7d or a quoted date like '2026-01-15'", clause, tok.Describe())
	}

	value, err := p.parseTimeRange(key, TimeValueNode[K]{token: tok, value: t})
	if err != nil {
		return lexer.Token{}, TimeValueNode[K]{}, err
	}

	// A remember's times are kept on its fact and written back on every hit as
	// RFC 3339 text. That form has no year after 9999, and its year 0000 is one
	// not every client reads. The first instant of the year 0001 is the zero
	// time, which reads as no time: a fact said then would read as never dated,
	// and a when answering with it as unanswered. So a time given to a fact
	// starts after the year 0001 does and ends before the year 9999 does, as it
	// is written and in UTC, where a reader comparing two times puts them. The
	// last day, month and year of 9999 each end on the first instant of the year
	// after, and one fact holding such a time would leave every response that
	// carries it unwritable. A bound is never written back, and keeps every date
	// a literal can spell. A duration counts back from the write by 292 years at
	// most, and stays inside those years too.
	if cmd.Type == lexer.REMEMBER && !relative {
		span := value.value.Resolve(time.Time{})
		early := span.Start.Year() < 1 || span.End.Year() < 1 || !span.Start.After(time.Time{})
		late := span.Start.UTC().Year() > 9999 || span.End.UTC().Year() > 9999
		if early || late {
			return lexer.Token{}, TimeValueNode[K]{}, p.errf(tok.Pos, "invalid %s value: out of range (a fact's time starts after the year 0001 does and ends before the year 9999 does)", clause)
		}
	}

	return key, value, nil
}

// parseTimeRange reads what may follow a time value's first end: the '..' of a
// range, then the range's second end. from is the value read so far, and comes
// back unchanged when no '..' follows it. A range joins quoted dates, with
// nothing between them and the dots. Its second end may be left out, for a
// span that began and is not over. A duration is one instant, so it is never
// an end of a range.
func (p *parser[K, P]) parseTimeRange(key lexer.Token, from TimeValueNode[K]) (TimeValueNode[K], error) {
	if p.cur.Type == lexer.WHITESPACE && p.peek.Type == lexer.RANGE {
		return TimeValueNode[K]{}, p.errf(p.cur.Pos, "no space allowed before ..")
	}
	if p.cur.Type != lexer.RANGE {
		return from, nil
	}

	clause := strings.ToLower(key.Literal)
	const shape = "a range joins two quoted dates, as in '2023-05-07'..'2023-05-09'"

	if from.token.Type != lexer.PHRASE {
		return TimeValueNode[K]{}, p.errf(p.cur.Pos, "invalid %s value: a duration cannot start a range — %s", clause, shape)
	}

	r := from
	r.dots = p.take()

	switch {
	case p.cur.Type == lexer.ILLEGAL:
		return TimeValueNode[K]{}, p.errUnexpected(p.cur)
	case p.cur.Type == lexer.WHITESPACE && (p.peek.Type == lexer.PHRASE || p.peek.Type == lexer.ILLEGAL):
		return TimeValueNode[K]{}, p.errf(p.cur.Pos, "no space allowed after ..")
	case p.cur.Type.IsBlank(), p.ends(p.cur):
		r.value = containers.RangeTime[K]{From: from.value}
		return r, nil
	case p.cur.Type != lexer.PHRASE:
		return TimeValueNode[K]{}, p.errf(p.cur.Pos, "invalid %s value %s: the end of a range is a quoted date — %s", clause, p.cur.Describe(), shape)
	}

	r.to = p.take()
	to, err := containers.ParseTimeValue[K](r.to.Literal)
	if _, relative := to.(containers.RelativeTime[K]); err != nil || relative {
		return TimeValueNode[K]{}, p.errf(r.to.Pos, "invalid %s value %s: the end of a range is a quoted date — %s", clause, r.to.Describe(), shape)
	}

	// Both ends are fixed on the calendar, so the range is the same whenever it
	// is resolved. Its second end has to reach where its first begins: what the
	// second names must share an instant with everything from the first's start
	// on. '2024'..'2023' does not, though 2023 ends on the instant 2024 begins.
	// A range written that way round holds nothing, and every comparison
	// against it would come out empty without saying why.
	begun := containers.Interval{Start: from.value.Resolve(time.Time{}).Start}
	if !begun.Overlaps(to.Resolve(time.Time{})) {
		return TimeValueNode[K]{}, p.errf(r.to.Pos, "invalid %s value: the range ends before it starts — write the earlier date first", clause)
	}
	r.value = containers.RangeTime[K]{From: from.value, To: to}

	return r, nil
}

// parseSubquery consumes a query written in parentheses as the value of key's
// time bound. What stands between the parentheses is a whole command, read by
// parseQuery, the production that reads an instruction. So a query has one
// grammar wherever it stands. The limits of the position are checks on the
// command that came back: only a when answers with a time.
//
// Parentheses do not nest, and that one limit is checked before descending. A
// query's size is bounded only by the request body, and a parser that read
// each '(' before judging it would recurse once per parenthesis.
func (p *parser[K, P]) parseSubquery(key lexer.Token) (SubqueryNode, error) {
	clause := strings.ToLower(key.Literal)

	lparen := p.take()
	if p.nested > 0 {
		return SubqueryNode{}, p.errf(lparen.Pos, "parentheses do not nest: a query in a time bound holds no query of its own")
	}
	if p.cur.Type == lexer.WHITESPACE {
		return SubqueryNode{}, p.errf(p.cur.Pos, "no space allowed after (")
	}
	// Anything but a command gets its repair here, where the clause is known.
	// parseQuery would only say that a command was expected.
	if !p.cur.Type.IsCommand() {
		return SubqueryNode{}, p.errf(p.cur.Pos, "expected a when after %s, found %s: a time bound's parentheses hold the when whose answer is the bound — write %s:(when <terms>)", lparen.Describe(), p.cur.Describe(), clause)
	}

	cmd := p.cur

	p.nested++
	query, err := p.parseQuery()
	p.nested--

	if err != nil {
		return SubqueryNode{}, err
	}
	if _, ok := query.(*WhenCommandNode[K, P]); !ok {
		return SubqueryNode{}, p.errf(cmd.Pos, "%s cannot stand in a time bound: only a when answers with a time — write %s:(when <terms>)", cmd.Describe(), clause)
	}

	rparen, err := p.expect(lexer.RPAREN)
	if err != nil {
		return SubqueryNode{}, p.errf(p.cur.Pos, "expected \")\" to close the when, found %s", p.cur.Describe())
	}

	return SubqueryNode{lparen: lparen, query: query, rparen: rparen}, nil
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

	if !p.cur.Type.IsBlank() && !p.ends(p.cur) {
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
		lexer.SPECIAL, lexer.PLUS, lexer.TILDE, lexer.MINUS, lexer.RANGE:
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
