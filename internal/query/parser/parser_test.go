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

package parser_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/RonsenbergVI/fraise/internal/query/parser"
)

// FuzzRememberPhraseRoundTrip is the server-side guarantee that quoting is a
// total encoding for free-flowing text: for any UTF-8 value, escaping
// apostrophes by doubling and wrapping in quotes parses back to exactly that
// value, and the String() reconstruction re-parses to it as well. Ingestion
// may feed a phrase any JSON-transportable character — nothing between the
// quotes may be lost, altered, or end the phrase early. (Invalid UTF-8 is
// skipped: JSON decoding has already replaced it before a query reaches the
// parser, so it cannot arrive here.)
func FuzzRememberPhraseRoundTrip(f *testing.F) {
	for _, seed := range []string{
		"plain words",
		"it's got an apostrophe",
		"''",
		"'",
		"trailing quote '",
		"line one\nline two",
		"a\r\n\tb",
		"déjà vu 😀 東京",
		`C:\temp\new "quoted"`,
		"a\x00b",
		"remember recall topic:x vec:$v @3 (since)",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if !utf8.ValidString(value) {
			t.Skip("JSON transport never delivers invalid UTF-8")
		}
		quoted := "'" + strings.ReplaceAll(value, "'", "''") + "'"
		q := "remember " + quoted + " topic:x"

		cmd, _, err := parser.Parse[uint64, float32](q)
		if err != nil {
			t.Fatalf("Parse(%q) = %v, want the escaped phrase to parse", q, err)
		}
		rc, ok := cmd.(*parser.RememberCommandNode[float32])
		if !ok {
			t.Fatalf("Parse returned %T, want *RememberCommandNode", cmd)
		}
		if got := rc.Value(); got != value {
			t.Fatalf("stored value = %q, want %q", got, value)
		}

		// The reconstruction must survive a second trip: String() re-escapes.
		cmd2, _, err := parser.Parse[uint64, float32](cmd.String())
		if err != nil {
			t.Fatalf("re-Parse(String() = %q) = %v", cmd.String(), err)
		}
		if got := cmd2.(*parser.RememberCommandNode[float32]).Value(); got != value {
			t.Fatalf("re-parsed value = %q, want %q", got, value)
		}
	})
}

// FuzzParseNeverPanics feeds the parser arbitrary wire input: raw queries come
// from any HTTP client, not just the SDK, so on garbage the server's only
// acceptable answers are a parsed query or a positioned error — never a panic.
func FuzzParseNeverPanics(f *testing.F) {
	for _, seed := range []string{
		"",
		"recall",
		"recall x",
		"remember 'a' topic:b",
		"recall@0 'q' top:3 vec:$v",
		"'",
		"''",
		"recall '",
		"@@@:::$$$",
		"remember remember remember",
		"recall x topic:'y' since:7d until:2026-01-15 depth:2 top:5",
		"recall x \x00 y",
		"recall 'a\x00b'",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		_, _, _ = parser.Parse[uint64, float32](raw) // must return, not panic
	})
}

// TestClauseErrorsSurfaceUnmangled pins that a clause helper's positioned
// error reaches the caller as-is. The call sites used to re-wrap with a bad
// %e verb, turning a clean "invalid since value ..." into
// `&{%!e(string=...)}` in the 400 body — the message an agent needs to
// self-correct was garbled at the last step.
func TestClauseErrorsSurfaceUnmangled(t *testing.T) {
	cases := []struct {
		query string
		want  string // substring of the inner, positioned error
	}{
		{"recall x since:soon", "invalid since value"},
		{"recall x until:later", "invalid until value"},
		{"recall x depth:abc", "invalid depth value"},
		{"recall x top:abc", "invalid top value"},
		{"recall x topic:", "expected a word or quoted phrase"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a parse error", tc.query)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.want) {
				t.Errorf("error %q does not contain the inner message %q", msg, tc.want)
			}
			if strings.Contains(msg, "%!e") || strings.Contains(msg, "Error while parsing") {
				t.Errorf("error %q is re-wrapped/mangled, want the inner error as-is", msg)
			}
		})
	}
}

func TestRememberParser(t *testing.T) {
	q := "remember@1 'anne loves the color orange' topic:color topic:preference entity:anne vec:$v"

	qo, _, err := parser.Parse[uint64, float32](q)

	if err != nil {
		t.Error("Expected no error while parsing this query.")
	}

	if qo.String() != q {
		t.Error("Reconstructed string query should equal original query.")
	}
}

// TestRecallParser checks that valid recall queries parse without error and
// round-trip through String(). Fields are written in the order String() emits
// them (terms, entities, topics, top, depth) so the reconstruction matches.
//
// since/until are intentionally omitted: the parser handles them, but a
// containers.TimeValue cannot currently render back to its source text
// (RelativeTime.String recurses, AbsoluteTime.String uses RFC822), so a
// time field can't round-trip yet.
func TestRecallParser(t *testing.T) {
	queries := []string{
		"recall anna",
		"recall anna bob charlie",
		"recall@2 anna",
		"recall@2 anna bob entity:alice topic:job top:10 depth:5",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			qo, _, err := parser.Parse[uint64, float32](q)
			if err != nil {
				t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
			}
			if qo.String() != q {
				t.Errorf("round-trip mismatch: String() = %q, want %q", qo.String(), q)
			}
		})
	}
}

// TestRecallParserErrors checks that malformed recall queries are rejected.
func TestRecallParserErrors(t *testing.T) {
	queries := []string{
		"recall",          // no seed at all
		"recall top:3",    // modifiers scope a search; they cannot start one
		"recall since:7d", // ditto for a time bound
		"recall depth:2 top:5",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an error", q)
			}
			if !strings.Contains(err.Error(), "at least one seed") {
				t.Errorf("error %q does not say what the recall is missing", err)
			}
		})
	}
}

// TestAnchorSeededRecallParses pins that a recall needs a seed, not a *term*:
// an anchor and a vector are seeds too, so "everything about billing" is a
// question the grammar can express. Requiring a text term made it unreachable —
// callers reached for a nonsense keyword to get past the parser, which seeded
// the search with a word they did not mean.
func TestAnchorSeededRecallParses(t *testing.T) {
	queries := []string{
		"recall topic:job",
		"recall entity:alice",
		"recall topic:job entity:alice",
		"recall@2 topic:job",
		"recall topic:job top:5 depth:2",
		"recall topic:job since:7d",
		"recall vec:$v",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q); err != nil {
				t.Errorf("Parse(%q) = %v, want it to parse", q, err)
			}
		})
	}
}

// TestGraphSelectorRejectsOutOfRange checks that a selector which does not fit
// in a uint8 is rejected at parse time rather than silently wrapped into a
// valid-looking graph (@256 -> 0, @300 -> 44). Wrapping would route the query to
// the wrong tenant's graph, so this must fail before execution. A non-integer
// selector is likewise rejected. Applies to both recall and remember.
func TestGraphSelectorRejectsOutOfRange(t *testing.T) {
	queries := []string{
		"recall@256 secret",
		"recall@300 secret",
		"recall@-1 secret",
		"recall@abc secret",
		"remember@256 'secret plan' topic:x",
		"remember@300 'secret plan' topic:x",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q); err == nil {
				t.Errorf("Parse(%q) = nil error, want an out-of-range/parse error", q)
			}
		})
	}

	// A selector that fits in a uint8 still parses here; the tighter
	// [0, num-graphs) bound is the handler's job, not the parser's.
	if _, _, err := parser.Parse[uint64, float32]("recall@255 secret"); err != nil {
		t.Errorf("Parse(recall@255 …) returned error: %v, want nil", err)
	}
}

// TestRememberPhrase covers the opaque single-quoted phrase: reserved words and
// symbols (: ' $ @ ( )) inside it are stored verbatim, and a doubled quote (”)
// is an escaped apostrophe. These are the cases from the phrase-storage bug
// report — each one used to fail to parse.
func TestRememberPhrase(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string // the fact that should be stored (RememberCommandNode.Value)
	}{
		{"colon in phrase", "remember 'meeting at 3:30pm with anna' topic:meetings", "meeting at 3:30pm with anna"},
		{"reserved word in phrase", "remember 'remind me about this topic later' topic:reminders", "remind me about this topic later"},
		{"multiple reserved words", "remember 'recall the top since until depth vec entity' topic:x", "recall the top since until depth vec entity"},
		{"symbols in phrase", "remember 'email $bill @ (acme)' topic:contacts", "email $bill @ (acme)"},
		{"escaped apostrophe", "remember 'alice''s laptop' topic:devices", "alice's laptop"},
		{"apostrophe at edges", "remember '''quoted''' topic:x", "'quoted'"},
		{"interior spacing preserved", "remember 'a   b' topic:x", "a   b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			rc, ok := cmd.(*parser.RememberCommandNode[float32])
			if !ok {
				t.Fatalf("Parse(%q) returned %T, want *RememberCommandNode", tc.query, cmd)
			}
			if got := rc.Value(); got != tc.want {
				t.Errorf("stored fact = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRememberPhraseRoundTrip checks that a fact containing an apostrophe
// survives String() reconstruction (the inner quote is re-escaped to ”).
func TestRememberPhraseRoundTrip(t *testing.T) {
	// String() always renders the graph selector (@0 by default), so include it.
	q := "remember@0 'alice''s laptop' topic:devices"
	cmd, _, err := parser.Parse[uint64, float32](q)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", q, err)
	}
	if got := cmd.String(); got != q {
		t.Errorf("String() = %q, want %q", got, q)
	}
}

// TestRememberPhraseErrors checks phrases that must be rejected.
func TestRememberPhraseErrors(t *testing.T) {
	queries := []string{
		"remember 'unterminated phrase topic:x", // no closing quote
		"remember topic:x",                      // missing the quoted fact
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q); err == nil {
				t.Errorf("Parse(%q) = nil error, want an error", q)
			}
		})
	}
}

// TestFieldRequiresColonSeparator pins that every keyed field rejects a missing
// ':' instead of advancing past whatever token sits in the separator's place.
// The two unchecked advances this replaces did not merely accept a typo: they
// shifted the remaining tokens one role to the left, so "since 7d 30d" parsed
// clean and bounded the recall at 30d, and "topic food extra" returned results
// filtered by nothing. A wrong answer with no error is the failure an agent
// cannot detect, let alone correct from; the whole family (topic, entity,
// since, until, top, depth) is listed here so no field can regress alone.
func TestFieldRequiresColonSeparator(t *testing.T) {
	queries := []string{
		"recall zebras topic food",
		"recall zebras topic food extra",
		"recall zebras entity alice",
		"recall zebras since 7d",
		"recall zebras until 2026-01-15",
		"recall zebras top 5",
		"recall zebras depth 2",
		"recall zebras topic:food entity alice",
		"remember 'zebras eat grass' topic food",
		"remember 'zebras eat grass' entity zebras",
		// The token-shifting shapes: each one used to parse without error, with
		// the second value silently winning the field and the first swallowed.
		"recall zebras since 7d 30d",
		"recall zebras until 7d 30d",
		"recall zebras since:7d until 30d 60d",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-separator error", q)
			}
			if !strings.Contains(err.Error(), "Expected colon") {
				t.Errorf("error %q does not name the missing ':' separator", err)
			}
		})
	}
}

// TestParseErrorBlamesTheOffendingToken pins where a parse error points: the
// column is the last character of the token the message quotes.
//
// Errors used to be positioned at the lexer's CurrentPos, which is not where
// the parser is — cur/peek read one token ahead, so CurrentPos sits at the end
// of the token *after* the bad one. "recall zebras topic food extra" reported
// column 30, the end of "extra", while the message quoted "food" (ending at
// 24). Every case below therefore keeps a token after the offending one; with
// the bad token last, a CurrentPos regression passes unnoticed, which is how
// this survived.
//
// The column and the quoted literal are asserted together on purpose: either
// alone can look right while the pair contradicts each other, and it is the
// pair an agent (or a human squinting at a caret) uses to find the mistake.
func TestParseErrorBlamesTheOffendingToken(t *testing.T) {
	cases := []struct {
		query string
		blame string // the token the error must quote and point at
	}{
		// Missing ':' — the whole keyed-field family.
		{"recall zebras topic food extra", "food"},
		{"recall zebras entity alice extra", "alice"},
		{"recall zebras since 7d 30d", "7d"},
		{"recall zebras until 7d 30d", "7d"},
		{"recall zebras top 5 depth:2", "5"},
		{"recall zebras depth 2 top:3", "2"},
		{"remember 'zebras eat grass' topic food entity:x", "food"},
		// Unparseable values: the blamed token is the value, not the key.
		{"recall zebras since:soon top:3", "soon"},
		{"recall zebras until:later top:3", "later"},
		{"recall zebras depth:abc top:3", "abc"},
		{"recall zebras top:abc depth:2", "abc"},
		{"recall@abc zebras top:3", "abc"},
		// A token in a position no clause can start.
		{"recall zebras : topic:food", ":"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a parse error", tc.query)
			}

			var perr *parser.Error
			if !errors.As(err, &perr) {
				t.Fatalf("error %v is not a *parser.Error, so it carries no position", err)
			}

			// 1-based column of the offending token's last character.
			want := strings.Index(tc.query, tc.blame) + len(tc.blame)
			if perr.Pos.Column != want {
				t.Errorf("%q: column %d, want %d (the end of %q)\n  %s\n  %*s\n  %v",
					tc.query, perr.Pos.Column, want, tc.blame,
					tc.query, perr.Pos.Column, "^", err)
			}
			if !strings.Contains(perr.Msg, strconv.Quote(tc.blame)) {
				t.Errorf("%q: message %q does not quote the offending token %q",
					tc.query, perr.Msg, tc.blame)
			}
		})
	}
}

// TestQuotedValues checks that quoting also works for anchor values and recall
// terms, so a topic or a search term can itself contain spaces, symbols, or a
// reserved word.
func TestQuotedValues(t *testing.T) {
	t.Run("quoted anchor value", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("remember 'x' topic:'my project'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RememberCommandNode[float32])
		got := rc.Topics()
		if len(got) != 1 || got[0] != "my project" {
			t.Errorf("Topics() = %v, want [\"my project\"]", got)
		}
	})

	t.Run("quoted recall term", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("recall 'meeting at 3:30pm' topic:work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RecallCommandNode[uint64, float32])
		terms := rc.Terms()
		if len(terms) != 1 || terms[0] != "meeting at 3:30pm" {
			t.Errorf("Terms() = %v, want [\"meeting at 3:30pm\"]", terms)
		}
	})
}

// TestKeywordAsValue pins that a reserved word in value position parses as an
// ordinary word. The lexer types "top" by spelling alone, so entity:top used
// to be a 400 — and a single-word entity an LLM extracts (e.g. "top" from
// "she reached the top") is only a matter of corpus size, killing ingestion
// with an error the client cannot anticipate. A keyword is syntax only where
// a clause can start; on the right of a field's ':' or as the leading recall
// term, it is data.
func TestKeywordAsValue(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		entities []string
		topics   []string
		terms    []string
	}{
		{
			name:     "entity top on remember",
			query:    "remember 'a neutral test value.' topic:some-topic entity:top",
			entities: []string{"top"},
			topics:   []string{"some-topic"},
		},
		{
			name:   "topic top on remember",
			query:  "remember 'a neutral test value.' topic:some-topic topic:top",
			topics: []string{"some-topic", "top"},
		},
		{
			name:     "every field keyword as an anchor value",
			query:    "remember 'x' entity:recall entity:since entity:until entity:depth entity:vec entity:entity topic:topic",
			entities: []string{"recall", "since", "until", "depth", "vec", "entity"},
			topics:   []string{"topic"},
		},
		{
			name:     "keyword anchors on recall",
			query:    "recall shelf entity:top topic:top",
			entities: []string{"top"},
			topics:   []string{"top"},
			terms:    []string{"shelf"},
		},
		{
			name:  "keyword as the leading recall term",
			query: "recall top",
			terms: []string{"top"},
		},
		{
			name:     "keyword value followed by a real top clause",
			query:    "recall top entity:top top:3",
			entities: []string{"top"},
			terms:    []string{"top"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}

			var entities, topics, terms []string
			switch n := cmd.(type) {
			case *parser.RememberCommandNode[float32]:
				entities, topics = n.Entities(), n.Topics()
			case *parser.RecallCommandNode[uint64, float32]:
				entities, topics, terms = n.Entities(), n.Topics(), n.Terms()
			default:
				t.Fatalf("Parse(%q) returned %T", tc.query, cmd)
			}

			if got, want := entities, tc.entities; !slices.Equal(got, want) {
				t.Errorf("Entities() = %v, want %v", got, want)
			}
			if got, want := topics, tc.topics; !slices.Equal(got, want) {
				t.Errorf("Topics() = %v, want %v", got, want)
			}
			if got, want := terms, tc.terms; !slices.Equal(got, want) {
				t.Errorf("Terms() = %v, want %v", got, want)
			}
		})
	}
}

// TestKeywordAsValueDisambiguation pins the tie-breaker that keeps the rule
// safe: a keyword immediately followed by ':' is always a field, never a
// value, so a clause mistyped into value position is an error rather than
// silently consumed as data (the failure mode TestFieldRequiresColonSeparator
// exists to prevent). Where a clause can start — after the first recall term —
// a bare keyword still reads as a clause and still errors without its ':'.
func TestKeywordAsValueDisambiguation(t *testing.T) {
	queries := []string{
		"recall top:3",                         // a modifier is not a seed
		"recall shelf top",                     // clause position: a bare keyword is not a term
		"remember 'x' entity:top:3",            // keyword-colon after the anchor's ':' is a field, not a value
		"remember 'x' entity:since:7d topic:x", // ditto for a time field
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q); err == nil {
				t.Errorf("Parse(%q) = nil error, want an error", q)
			}
		})
	}
}

// TestMiscasedKeywordIsRejected pins that a keyword written with any upper
// case is a parse error wherever it could still be something else. Case folding
// applies to data only; letting it reach a keyword would fold "recall x Since
// 7d" into a three-term search — the silent token-shift the separator tests
// guard against, re-entered through the casing door.
//
// The exception is a keyword glued to a ':', which has its own test below: the
// colon leaves nothing for it to be mistaken for, so there casing is forgiven
// rather than reported.
func TestMiscasedKeywordIsRejected(t *testing.T) {
	cases := []struct {
		query string
		want  string // substring the error must carry
	}{
		// Clause position in the term stream: used to fold into terms silently.
		{"recall zebras Since 7d", "lower case"},
		{"recall zebras Since 7d 30d", "lower case"},
		{"recall zebras Until 2026-01-15", "lower case"},
		{"recall zebras Depth 2", "lower case"},
		// Command position: a command is never inferred from a mis-spelling,
		// because the verb decides whether the query writes.
		{"Recall zebras", "expected a command"},
		{"REMEMBER 'x' topic:y", "expected a command"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a mis-cased-keyword error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestMiscasedKeywordBeforeColonIsTheClause pins the one place casing is
// forgiven: a word glued to a ':' can only be a field, because no production
// puts a bare word in front of a colon. Reading "DEPTH:5" as the depth clause
// is what lets a repeated clause be caught as a duplicate — blaming the casing
// instead would report the shallower of the two mistakes, and an agent that
// fixes the casing would then get a silently rescoped query.
//
// It stays narrow on purpose: away from a ':' the spelling must still match, so
// "Recall x" is not a command and "recall x Since 7d" is not a time bound (see
// TestMiscasedKeywordIsRejected).
func TestMiscasedKeywordBeforeColonIsTheClause(t *testing.T) {
	recalls := []struct {
		query string
		check func(*testing.T, *parser.RecallCommandNode[uint64, float32])
	}{
		{"recall zebras TOP:3", func(t *testing.T, r *parser.RecallCommandNode[uint64, float32]) {
			if got := r.Top(0); got != 3 {
				t.Errorf("Top() = %d, want 3", got)
			}
		}},
		{"recall zebras Depth:2", func(t *testing.T, r *parser.RecallCommandNode[uint64, float32]) {
			if !r.HasDepth() || r.Depth(0) != 2 {
				t.Errorf("Depth() = %d (has %v), want 2", r.Depth(0), r.HasDepth())
			}
		}},
		{"recall zebras Topic:food", func(t *testing.T, r *parser.RecallCommandNode[uint64, float32]) {
			if got := r.Topics(); !slices.Equal(got, []string{"food"}) {
				t.Errorf("Topics() = %v, want [food]", got)
			}
		}},
	}

	for _, tc := range recalls {
		t.Run(tc.query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) = %v, want the clause it spells", tc.query, err)
			}
			r, ok := cmd.(*parser.RecallCommandNode[uint64, float32])
			if !ok {
				t.Fatalf("Parse(%q) returned %T, want *RecallCommandNode", tc.query, cmd)
			}
			tc.check(t, r)
		})
	}

	remembers := []struct {
		query string
		want  []string
		got   func(*parser.RememberCommandNode[float32]) []string
	}{
		{"remember 'x' Entity:bob", []string{"bob"}, (*parser.RememberCommandNode[float32]).Entities},
		{"remember 'x' Topic:food", []string{"food"}, (*parser.RememberCommandNode[float32]).Topics},
	}

	for _, tc := range remembers {
		t.Run(tc.query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) = %v, want the clause it spells", tc.query, err)
			}
			r, ok := cmd.(*parser.RememberCommandNode[float32])
			if !ok {
				t.Fatalf("Parse(%q) returned %T, want *RememberCommandNode", tc.query, cmd)
			}
			if got := tc.got(r); !slices.Equal(got, tc.want) {
				t.Errorf("anchors = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDuplicateModifierIsRejected pins that a single-valued clause given twice
// is an error, not a last-wins overwrite. Last-wins is the failure an agent
// cannot detect: the query runs, the results are real, and they answer a
// differently-scoped question than the one asked. Anchors are excluded — a
// repeated topic:/entity: is a list by design, and the last two cases pin that
// the stricter rule did not swallow them.
func TestDuplicateModifierIsRejected(t *testing.T) {
	rejected := []string{
		"recall zebras depth:1 depth:2",
		"recall zebras top:3 top:10",
		"recall zebras since:7d since:30d",
		"recall zebras until:1d until:2d",
		"recall zebras since:7d until:30d since:1d",
		"recall zebras vec:$a vec:$b",
		"recall zebras depth:2 DEPTH:5",
		"remember 'x' vec:$a vec:$b",
	}

	for _, q := range rejected {
		t.Run(q, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a duplicate-clause error", q)
			}
			if !strings.Contains(err.Error(), "duplicate") {
				t.Errorf("error %q does not name the repeat as a duplicate", err)
			}
		})
	}

	accepted := []string{
		"recall zebras topic:food topic:drink",
		"recall zebras entity:alice entity:bob",
	}

	for _, q := range accepted {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q); err != nil {
				t.Errorf("Parse(%q) = %v, want repeated anchors to parse", q, err)
			}
		})
	}
}

// TestLeadingKeywordTermWarns pins the warning that covers the grammar's one
// surviving ambiguity: a leading term that spells a keyword is legal data,
// but it is also one ':' away from a clause — "recall since 7d" is a valid
// two-term search and a near-miss of "recall since:7d". Erroring would take
// back the LLM-extraction fix (recall top must work); staying silent would
// let the typo answer a differently-scoped question with no signal. So the
// query runs and the response says what else it could have meant.
func TestLeadingKeywordTermWarns(t *testing.T) {
	cases := []struct {
		name  string
		query string
		warns bool
	}{
		{"bare keyword before a value-looking term", "recall since 7d", true},
		{"bare keyword alone", "recall top", true},
		{"mis-cased keyword spelling", "recall Top", true},
		{"quoted keyword is deliberate, no warning", "recall 'since' 7d", false},
		{"ordinary word", "recall zebras", false},
		{"keyword used as a clause", "recall zebras since:7d", false},
		{"keyword as an anchor value is unambiguous", "recall zebras entity:top", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			if got := len(warns) > 0; got != tc.warns {
				t.Fatalf("Parse(%q) warnings = %v, want warned=%v", tc.query, warns, tc.warns)
			}
		})
	}
}

// TestLeadingKeywordTermWarningIsActionable pins the warning's content: it
// must name both readings and both remedies, positioned like a parse error,
// because the whole point is that an agent (or a human) can resolve the
// ambiguity from the message alone.
func TestLeadingKeywordTermWarningIsActionable(t *testing.T) {
	q := "recall since 7d"
	_, warns, err := parser.Parse[uint64, float32](q)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", q, err)
	}
	if len(warns) != 1 {
		t.Fatalf("Parse(%q) warnings = %v, want exactly one", q, warns)
	}

	msg := warns[0].String()
	for _, want := range []string{
		"since:<value>",              // the clause reading, with its syntax
		"('since')",                  // the term reading, with the quoting escape
		"parse warning at column 12", // positioned at the term's last character, like an error
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("warning %q does not contain %q", msg, want)
		}
	}
}

// TestMiscasedKeywordStaysDataInValuePosition pins the other side of the
// casing rule: where a token is unambiguously data — the leading recall term,
// an anchor value, a quoted phrase — upper case is legal and folds, keyword
// spellings included. Rejecting these would take the LLM-extraction fix back:
// an extracted entity arrives in whatever case the model emitted.
func TestMiscasedKeywordStaysDataInValuePosition(t *testing.T) {
	t.Run("leading recall term", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("recall Top")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RecallCommandNode[uint64, float32])
		if got := rc.Terms(); !slices.Equal(got, []string{"top"}) {
			t.Errorf("Terms() = %v, want [top]", got)
		}
	})

	t.Run("quoted term after the first", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("recall zebras 'Since'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RecallCommandNode[uint64, float32])
		if got := rc.Terms(); !slices.Equal(got, []string{"zebras", "since"}) {
			t.Errorf("Terms() = %v, want [zebras, since]", got)
		}
	})

	// entity:Top folding to the "top" anchor is pinned in
	// TestValuesFoldToLowerCase.
}

// TestValuesFoldToLowerCase pins the case contract: terms and anchor values
// are identity, not prose, and fold to lower case on the way in — while the
// quoted fact of a remember keeps the spelling it was written with. If the
// fold regresses, the same anchor exists under as many nodes as it has
// capitalisations and recalls silently miss facts filed under another one.
func TestValuesFoldToLowerCase(t *testing.T) {
	t.Run("anchor values fold, the fact does not", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("remember 'MiXeD Case' topic:Billing entity:Anna")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RememberCommandNode[float32])
		if got := rc.Value(); got != "MiXeD Case" {
			t.Errorf("Value() = %q, want the fact stored exactly as written", got)
		}
		if got := rc.Topics(); !slices.Equal(got, []string{"billing"}) {
			t.Errorf("Topics() = %v, want [billing]", got)
		}
		if got := rc.Entities(); !slices.Equal(got, []string{"anna"}) {
			t.Errorf("Entities() = %v, want [anna]", got)
		}
	})

	t.Run("quoted anchor values fold too", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("remember 'x' topic:'My Project'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RememberCommandNode[float32])
		if got := rc.Topics(); !slices.Equal(got, []string{"my project"}) {
			t.Errorf("Topics() = %v, want [my project]", got)
		}
	})

	t.Run("recall terms fold, bare and quoted alike", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("recall Anna 'Bob Marley' topic:Music")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RecallCommandNode[uint64, float32])
		if got := rc.Terms(); !slices.Equal(got, []string{"anna", "bob marley"}) {
			t.Errorf("Terms() = %v, want [anna, bob marley]", got)
		}
		if got := rc.Topics(); !slices.Equal(got, []string{"music"}) {
			t.Errorf("Topics() = %v, want [music]", got)
		}
	})

	t.Run("a capitalised keyword folds into the same word", func(t *testing.T) {
		// entity:Top and entity:top must land on one anchor: "Top" is a plain
		// LITERAL to the lexer while "top" is a keyword, and the fold is what
		// stops that lexing difference leaking into the graph as two anchors.
		cmd, _, err := parser.Parse[uint64, float32]("remember 'x' entity:Top")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RememberCommandNode[float32])
		if got := rc.Entities(); !slices.Equal(got, []string{"top"}) {
			t.Errorf("Entities() = %v, want [top]", got)
		}
	})
}

// TestExplicitDepthIsHonouredIncludingZero pins that the *presence* of a depth
// clause, not a nonzero value, decides whether the configured default applies.
//
// depth:0 is the floor lane — text and vector seeds only, no anchor traversal.
// Reading presence off the value collapsed it into the default, so a client
// that explicitly turned the graph channel off silently got it back, and
// wherever the default is 2 that is the full excess round: the query answered
// was not the query asked. Both the flag and the resolved value are asserted,
// because Depth's fallback is what a caller actually receives.
func TestExplicitDepthIsHonouredIncludingZero(t *testing.T) {
	cases := []struct {
		query    string
		explicit bool
		want     int // Depth(7) — 7 stands in for the configured default
	}{
		{"recall x depth:0", true, 0},
		{"recall x depth:1", true, 1},
		{"recall x depth:2", true, 2},
		{"recall x", false, 7},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])

			if got := rc.HasDepth(); got != tc.explicit {
				t.Errorf("HasDepth() = %v, want %v", got, tc.explicit)
			}
			if got := rc.Depth(7); got != tc.want {
				t.Errorf("Depth(7) = %d, want %d", got, tc.want)
			}
		})
	}
}
