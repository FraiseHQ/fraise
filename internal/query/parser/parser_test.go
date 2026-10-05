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
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/FraiseHQ/fraise/internal/query/parser"
)

// FuzzRememberPhraseRoundTrip is the server-side guarantee that quoting is a
// total encoding for free-flowing text: for any valid UTF-8 value that is not
// blank, escaping apostrophes by doubling and wrapping in quotes parses back to
// exactly that value, and the String() reconstruction re-parses to it as well.
// Ingestion may feed a phrase any JSON-transportable character, so nothing
// between the quotes may be lost, altered, or end the phrase early. Invalid
// UTF-8 is skipped because JSON decoding replaces it before a query reaches
// the parser, and a blank value because an empty fact is rejected.
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
		if strings.TrimSpace(value) == "" {
			t.Skip("a whitespace-only fact is empty, and empty facts are rejected")
		}
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
		"recall x topic:'y' since:7d until:'2026-01-15' depth:2 top:5",
		"recall x \x00 y",
		"recall 'a\x00b'",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		_, _, _ = parser.Parse[uint64, float32](raw) // must return, not panic
	})
}

// TestDurationsAreBoundedWithTheRangeInTheMessage pins the duration bound at
// the level an agent sees: a count past what its unit can hold is a parse
// error naming the largest one allowed, never a silent wrap into a future
// bound, and the largest count still parses.
func TestDurationsAreBoundedWithTheRangeInTheMessage(t *testing.T) {
	cases := []struct {
		query string
		want  string // "" when the query must parse
	}{
		{"recall x since:106751d", ""},
		{"recall x since:106752d", "invalid since value \"106752d\": out of range (at most 106751d)"},
		{"recall x until:15251w", "invalid until value \"15251w\": out of range (at most 15250w)"},
		{"recall x since:99999999999999999999d", "out of range (at most 106751d)"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Parse(%q) = %v, want it to parse", tc.query, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse(%q) = %v, want an error containing %q", tc.query, err, tc.want)
			}
		})
	}
}

// TestClauseErrorsSurfaceUnmangled pins that a clause helper's positioned
// error reaches the caller as-is: not re-wrapped, and not garbled by a bad
// format verb into `&{%!e(string=...)}`, which would destroy the message an
// agent needs to correct itself.
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
		// vec: on both commands: parseVecField's positioned error must reach
		// the caller unwrapped, like every other clause's.
		{"recall x vec:v", "expected param field operator $"},
		{"remember 'a fact' vec$:v", "Expected colon"}, // a remember: among a recall's terms a bare vec is a keyword term
		{"recall x vec:$", "expected literal"},
		{"remember 'a fact' vec:v", "expected param field operator $"},
		{"remember 'a fact' vec:$", "expected literal"},
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
// them (terms, entities, topics, top, depth, since, until) so the
// reconstruction matches. A value that was quoted comes back quoted: printed
// bare, 'e-mail' or '2026-01-15' would not parse.
func TestRecallParser(t *testing.T) {
	queries := []string{
		"recall anna",
		"recall anna bob charlie",
		"recall@2 anna",
		"recall@2 anna bob entity:alice topic:job top:10 depth:5",
		"recall 'e-mail' entity:'o''brien' topic:'machine-learning'",
		"recall@2 anna since:7d until:'2026-01-15'",
		"recall anna since:'2026-01-15T10:00:00Z'",
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

// TestRecallParserErrors checks that malformed recall queries are rejected, and
// that the message says what the recall is missing.
func TestRecallParserErrors(t *testing.T) {
	const noSeed = "a recall needs at least one seed: a term, a topic:/entity: anchor, or vec:$<name>"
	cases := []struct {
		query string
		want  string
	}{
		{"recall", "expected a space after the command, found end of input"},
		// Modifiers scope a search; they cannot start one.
		{"recall top:3", noSeed},
		{"recall since:7d", noSeed}, // ditto for a time bound
		{"recall depth:2 top:5", noSeed},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestAnchorSeededRecallParses pins that a recall needs a seed, not a *term*:
// an anchor and a vector are seeds too, so "everything about billing" is a
// question the grammar can express. Requiring a term would make callers invent
// a keyword to get past the parser, seeding the search with a word they did
// not mean.
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
// selector is likewise rejected. Applies to both recall and remember. The
// message names the selector and the range, so the caller knows which graph
// id to fix and what it may be.
func TestGraphSelectorRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"recall@256 secret", "graph selector 256 out of range (0-255)"},
		{"recall@300 secret", "graph selector 300 out of range (0-255)"},
		{"recall@-1 secret", `invalid graph selector "-": expected a whole number`},
		{"recall@abc secret", `invalid graph selector "abc": expected a whole number`},
		{"remember@256 'secret plan' topic:x", "graph selector 256 out of range (0-255)"},
		{"remember@300 'secret plan' topic:x", "graph selector 300 out of range (0-255)"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an out-of-range/parse error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
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
// symbols (: $ @ ( )) inside it are stored verbatim, and a doubled single quote
// is an escaped apostrophe.
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
// survives String() reconstruction (the inner quote is doubled again), and
// that a quoted anchor value comes back quoted.
func TestRememberPhraseRoundTrip(t *testing.T) {
	// String() always renders the graph selector (@0 by default), so include it.
	queries := []string{
		"remember@0 'alice''s laptop' topic:devices",
		"remember@0 'a fact' topic:'machine-learning' entity:'o''brien'",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](q)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", q, err)
			}
			if got := cmd.String(); got != q {
				t.Errorf("String() = %q, want %q", got, q)
			}
		})
	}
}

// TestRememberPhraseErrors checks phrases that must be rejected, and that the
// message names what is wrong with the fact.
func TestRememberPhraseErrors(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember 'unterminated phrase topic:x", "unterminated quoted phrase"}, // no closing quote
		{"remember topic:x", `expected a quoted phrase, but found "topic"`},     // missing the quoted fact
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestFieldRequiresColonSeparator pins that every keyed field rejects a missing
// ':' instead of advancing past whatever token sits in the separator's place.
// Advancing would shift the remaining tokens into other roles and answer a
// different query with no error, the failure an agent cannot detect, let alone
// correct from. The whole family (topic, entity, since, until, top, depth) is
// listed so no field can regress alone.
func TestFieldRequiresColonSeparator(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		// The fix names the clause the caller would write, with the word they
		// wrote after the keyword as its value: that is the query they meant.
		{"recall zebras topic food", "write topic:food"},
		{"recall zebras topic food extra", "write topic:food"},
		{"recall zebras entity alice", "write entity:alice"},
		{"recall zebras since 7d", "write since:7d"},
		{"recall zebras until 30d", "write until:30d"},
		{"recall zebras top 5", "write top:5"},
		{"recall zebras depth 2", "write depth:2"},
		{"recall zebras topic:food entity alice", "write entity:alice"},
		{"remember 'zebras eat grass' topic food", "write topic:food"},
		{"remember 'zebras eat grass' entity zebras", "write entity:zebras"},
		// The token-shifting shapes: without the check, the second value would
		// silently win the field and the first would be swallowed.
		{"recall zebras since 7d 30d", "write since:7d"},
		{"recall zebras until 7d 30d", "write until:7d"},
		{"recall zebras since:7d until 30d 60d", "write until:30d"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-separator error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name the missing ':' separator: want %q", err, tc.want)
			}
		})
	}
}

// TestParseErrorBlamesTheOffendingToken pins where a parse error points: the
// column is the last character of the token the message quotes.
//
// The lexer's CurrentPos is not where the parser is: cur and peek read one
// token ahead, so CurrentPos sits at the end of the token after the bad one.
// Every case below therefore keeps a token after the offending one; with the
// bad token last, an error positioned at CurrentPos would pass unnoticed.
//
// The column and the quoted literal are asserted together: either alone can
// look right while the pair disagrees, and it is the pair an agent or a human
// uses to find the mistake.
func TestParseErrorBlamesTheOffendingToken(t *testing.T) {
	cases := []struct {
		query string
		blame string // the token the error must quote and point at
	}{
		// Missing ':' — the whole keyed-field family. Among the terms the
		// keyword itself is blamed: it is the word to write as a filter or quote.
		{"recall zebras topic food extra", "topic"},
		{"recall zebras entity alice extra", "entity"},
		{"recall zebras since 7d 30d", "since"},
		{"recall zebras until 7d 30d", "until"},
		{"recall zebras top 5 depth:2", "top"},
		{"recall zebras depth 2 top:3", "depth"},
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

// TestBareWordIsLettersAndDigits pins the word rule from the accepting side: a
// bare word is letters, in any script, and digits, and any other character a
// value needs is written inside quotes, where it is data. Rejecting "e-mail"
// bare is only safe while its quoted form reaches the same value.
func TestBareWordIsLettersAndDigits(t *testing.T) {
	cases := []struct {
		query  string
		terms  []string
		topics []string
	}{
		{"recall café 東京 42", []string{"café", "東京", "42"}, nil},
		{"recall 'e-mail' topic:'machine-learning' since:'2026-01-15'", []string{"e-mail"}, []string{"machine-learning"}},
		{"recall 'ferry;' 'recall bridge'", []string{"ferry;", "recall bridge"}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])
			if got := rc.Terms(); !slices.Equal(got, tc.terms) {
				t.Errorf("Terms() = %q, want %q", got, tc.terms)
			}
			if got := rc.Topics(); !slices.Equal(got, tc.topics) {
				t.Errorf("Topics() = %q, want %q", got, tc.topics)
			}
		})
	}
}

// TestKeywordAsValue pins that a reserved word after a field's ':' parses as an
// ordinary word, though the lexer types "top" by spelling alone. An entity an
// LLM extracts can be a single reserved word ("top" from "she reached the
// top"), and rejecting it would fail ingestion with an error the client cannot
// anticipate. As a recall term a reserved word is written quoted, as in the
// last two cases.
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
			query:    "remember 'a neutral test value.' topic:'some-topic' entity:top",
			entities: []string{"top"},
			topics:   []string{"some-topic"},
		},
		{
			name:   "topic top on remember",
			query:  "remember 'a neutral test value.' topic:'some-topic' topic:top",
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
			query: "recall 'top'",
			terms: []string{"top"},
		},
		{
			name:     "keyword value followed by a real top clause",
			query:    "recall 'top' entity:top top:3",
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
// exists to prevent). A bare keyword among a recall's terms is an error too.
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
// 7d" into a three-term search, the silent token shift the separator tests
// guard against.
//
// The exception is a keyword glued to a ':', which has its own test below: the
// colon leaves nothing for it to be mistaken for, so there the clause runs and
// its casing earns a warning rather than an error.
func TestMiscasedKeywordIsRejected(t *testing.T) {
	cases := []struct {
		query string
		want  string // substring the error must carry
	}{
		// Clause position in the term stream.
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
// is what lets a repeated clause be caught as a duplicate; blaming the casing
// instead would report the shallower of the two mistakes, and an agent that
// fixed the casing would meet the duplicate only on its next try.
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

// TestMiscasedKeywordStaysDataInValuePosition pins the other side of the
// casing rule: where a token is unambiguously data (an anchor value, a quoted
// phrase), upper case is legal and folds, keyword spellings included. An
// extracted entity arrives in whatever case the model emitted, so rejecting
// these would fail ingestion.
func TestMiscasedKeywordStaysDataInValuePosition(t *testing.T) {
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

// TestExplicitTopIsVisibleIncludingZero pins that the *presence* of a top
// clause, not a nonzero value, decides whether the configured default applies.
//
// Unlike depth:0, top:0 is not a valid request (the documented range is 1 to
// db.max-top), but it must survive parsing as an explicit clause: read off the
// value, it would collapse into the default and skip the range check, and a
// caller asking for zero results would silently get the default count. The
// rejection lives in query.Parse; what is pinned here is that the parser keeps
// the clause visible for it.
func TestExplicitTopIsVisibleIncludingZero(t *testing.T) {
	cases := []struct {
		query    string
		explicit bool
		want     int // Top(7) — 7 stands in for the configured default
	}{
		{"recall x top:0", true, 0},
		{"recall x top:5", true, 5},
		{"recall x", false, 7},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])

			if got := rc.HasTop(); got != tc.explicit {
				t.Errorf("HasTop() = %v, want %v", got, tc.explicit)
			}
			if got := rc.Top(7); got != tc.want {
				t.Errorf("Top(7) = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestExplicitDepthIsHonouredIncludingZero pins that the *presence* of a depth
// clause, not a nonzero value, decides whether the configured default applies.
//
// depth:0 is the floor lane: seeds only, no anchor traversal. Read off the
// value, it would collapse into the configured default, and a client that
// turned the graph channel off would silently get it back. Both the flag and
// the resolved value are asserted, because Depth's fallback is what a caller
// actually receives.
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

// TestEmptyDataIsRejected pins that a quoted empty or whitespace-only value is
// refused wherever it can be written (see errEmpty for why), and that the
// message names which value was empty, so the caller knows which one to fill.
func TestEmptyDataIsRejected(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember ''", "a remembered fact must not be empty"},
		{"remember '' topic:harbour", "a remembered fact must not be empty"},
		{"remember '   '", "a remembered fact must not be empty"},
		{"recall ''", "a search term must not be empty"},
		{"recall '   '", "a search term must not be empty"},
		{"recall ferry ''", "a search term must not be empty"}, // blank second term, past the leading position
		{"recall ferry topic:''", "an anchor value must not be empty"},
		{"recall ferry entity:'   '", "an anchor value must not be empty"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an empty-value error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestRejectedTokensNameTheirOwnMistake pins that a token no production accepts
// is diagnosed, not merely reported. The caller is an agent: it can only repair
// a query the message tells it how to repair, so each shape a caller actually
// produces has to arrive with its own repair instruction rather than a shared
// "unexpected token".
func TestRejectedTokensNameTheirOwnMistake(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		// Grouping: the tokens lex but no rule accepts them, so the message
		// says the feature is absent rather than blaming the character.
		{"recall (zebras)", "grouping is not supported"},
		{"recall zebras (food)", "grouping is not supported"},
		{"recall zebras topic:(food)", "grouping is not supported"},
		{"recall zebras )food(", "grouping is not supported"},
		// A second command is one instruction too many, not a stray word.
		{"remember 'a' remember 'b'", "one command per instruction"},
		// Among a recall's terms a command word is the word the caller forgot
		// to quote.
		{"recall zebras recall food", `term "recall" is also a command: quote it ('recall')`},
		{"recall zebras remember 'x'", `term "remember" is also a command: quote it ('remember')`},
		// A prefix has no ':' form either, so its message names only the
		// quote: explain:<value> would itself be rejected.
		{"recall ferry explain", `term "explain" is also a prefix: quote it ('explain') to search for the word`},
		{"recall describe ferry", `term "describe" is also a prefix: quote it ('describe') to search for the word`},
		{"recall ferry explain:ferry", `"explain" is a prefix and starts no clause here: quote it ('explain') to search for the word`},
		{"remember 'a' describe", `"describe" is a prefix and starts no clause here: quote it ('describe') to search for the word`},
		// A keyword with nothing after it can never finish a clause, so it is
		// the word the caller forgot to quote.
		{"recall zebras top", "write top:<value> if a filter was meant, or quote it ('top')"},
		{"recall zebras since", "write since:<value> if a filter was meant, or quote it ('since')"},
		{"remember 'a' topic", `"topic" is a keyword and starts no clause here: write topic:<value> if a clause was meant, or quote it ('topic') to search for the word`},
		// Casing still matters where a clause could start.
		{"recall zebras Depth 2", "lower case"},
		// A modifier is a recall clause: the message names the command it was
		// given to and the clauses a remember takes, rather than telling the
		// caller to write since:<value>, which is what they wrote.
		{"remember 'a fact' top:3", "top: is a recall clause: a remember takes only topic:, entity: and vec:"},
		{"remember 'a fact' since:7d", "since: is a recall clause"},
		{"remember 'a fact' depth:1", "depth: is a recall clause"},
		// An unclosed quote is reported as one, not as whatever it swallowed.
		{"recall 'unclosed phrase", "unterminated quoted phrase"},
		// A NUL outside a phrase is rejected, not read as the end of the query
		// with everything after it dropped.
		{"recall zebras\x00food", "NUL character is only allowed inside a quoted phrase"},
		// Outside a phrase a word is letters and digits only. Any other
		// character is rejected for itself: absorbed into a word, "ferry;" would
		// be a term and the recall after it a search for the word "recall".
		{"recall ferry; recall bridge", `";" is only allowed inside a quoted phrase`},
		{"recall e-mail", `"-" is only allowed inside a quoted phrase`},
		{"recall zebras topic:machine-learning", `"-" is only allowed inside a quoted phrase`},
		{"recall zebras vec:$my_vec", `"_" is only allowed inside a quoted phrase`},
		{"recall zebras since:2026-01-15", "a quoted date like '2026-01-15'"},
		// A newline in a value slot is a second instruction starting early.
		{"recall zebras topic:\nfood", "one command per instruction"},
		// A second '@' is named as a second selector.
		{"recall@3@5 zebras", "one selector"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a parse error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
			if strings.Contains(err.Error(), `found ""`) {
				t.Errorf("error %q reports end of input as an empty literal", err)
			}
		})
	}
}

// TestCommandWordsAreNeverOfferedAsClauses pins the repair for a command word
// in a recall: quote it. The keyword advice ("write recall:<value> if a clause
// was meant") would send the caller from one rejection to the next, since no
// clause is spelled with a command. Whether the word is the first term or a
// later one, the message says only what works.
func TestCommandWordsAreNeverOfferedAsClauses(t *testing.T) {
	cases := []struct {
		query, word string
	}{
		{"recall recall", "recall"},
		{"recall forget", "forget"},
		{"recall zebras recall", "recall"},
		{"recall zebras remember", "remember"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](tc.query)
			msg := fmt.Sprint(err, warns)
			if err == nil && len(warns) == 0 {
				t.Fatalf("Parse(%q) was silent, want a warning or an error naming the command word", tc.query)
			}
			if !strings.Contains(msg, "quote it ('"+tc.word+"')") {
				t.Errorf("%s does not tell the caller to quote %q", msg, tc.word)
			}
			if strings.Contains(msg, tc.word+":<value>") {
				t.Errorf("%s offers %s:<value>, which is itself an error", msg, tc.word)
			}
		})
	}
}

// TestDanglingKeywordAfterAClauseIsAWordToQuote pins the repair for a keyword
// that ends a recall after its first clause. Past the terms a keyword can only
// start a clause, and with nothing after it none can be finished, so it is
// either a clause whose ':' went missing or a word the caller forgot to quote —
// and the message offers both.
func TestDanglingKeywordAfterAClauseIsAWordToQuote(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"recall x topic:y top", `"top" is a keyword and starts no clause here: write top:<value> if a clause was meant, or quote it ('top') to search for the word`},
		{"recall x topic:y since", `"since" is a keyword and starts no clause here: write since:<value> if a clause was meant, or quote it ('since') to search for the word`},
		{"recall x vec:$v depth", `"depth" is a keyword and starts no clause here: write depth:<value> if a clause was meant, or quote it ('depth') to search for the word`},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a dangling-keyword error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestDanglingCommandWordStartsASecondCommand pins the message for a command
// word that ends a query after its fact or its first clause: it names the
// second command and the quote, and nothing else. recall:<value> is itself an
// error, so a message offering it would send the caller from one rejection to
// the next.
func TestDanglingCommandWordStartsASecondCommand(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember 'a' recall", `"recall" starts a second command: one command per instruction — quote it ('recall') to search for the word`},
		{"remember 'a' remember", `"remember" starts a second command: one command per instruction — quote it ('remember') to search for the word`},
		{"recall x topic:y recall", `"recall" starts a second command: one command per instruction — quote it ('recall') to search for the word`},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a second-command error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestBareModifierOnARememberIsRejected pins that since, until, top and depth
// written without their ':' after a remember's fact are rejected by name, not
// as an unexpected token. A remember has no terms, so the word cannot be data
// there: it is a clause missing its ':' or a word that belongs inside quotes.
func TestBareModifierOnARememberIsRejected(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember 'a' since 7d", `"since" is a keyword and starts no clause here: write since:<value> if a clause was meant, or quote it ('since') to search for the word`},
		{"remember 'a' until 1d", `"until" is a keyword and starts no clause here: write until:<value> if a clause was meant, or quote it ('until') to search for the word`},
		{"remember 'a' top 3", `"top" is a keyword and starts no clause here: write top:<value> if a clause was meant, or quote it ('top') to search for the word`},
		{"remember 'a' depth 2", `"depth" is a keyword and starts no clause here: write depth:<value> if a clause was meant, or quote it ('depth') to search for the word`},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a keyword error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestVecWithoutColonIsRejected pins that vec followed by anything but ':' is
// rejected with the token it found, on both commands. Among a recall's terms a
// bare vec is a keyword term, so these cases put it where only a clause can
// start: after a remember's fact, or after a recall's first clause.
func TestVecWithoutColonIsRejected(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember 'a' vec$:v", `Expected colon, but found "$"`},
		{"recall x topic:y vec$:v", `Expected colon, but found "$"`},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-separator error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestRememberNeedsASpaceAfterTheCommand pins that a remember's command, with
// or without its graph selector, is followed by a space before the fact, and
// that a remember with nothing after it says what is missing rather than
// reporting a phrase that was never started.
func TestRememberNeedsASpaceAfterTheCommand(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember", "expected a space after the command, found end of input"},
		{"remember@2", "expected a space after the command, found end of input"},
		{"remember'a fact'", `expected a space after the command, found "a fact"`},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-space error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestNulWhereTheFactShouldStartIsRejected pins that a NUL in the fact's place
// is rejected for itself, as it is everywhere else outside quotes, rather than
// reported as a missing phrase: the caller needs to know the character is the
// problem, not that the fact is absent.
func TestNulWhereTheFactShouldStartIsRejected(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember \x00'a'", "a NUL character is only allowed inside a quoted phrase"},
		{"remember@1 \x00", "a NUL character is only allowed inside a quoted phrase"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a NUL error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestNewlineEndsTheInstruction pins that a newline separates instructions
// rather than blending into the whitespace around it, so a second line cannot
// silently join the first ("recall ferry\nbridge" is not a two-term recall).
// Trailing blank lines are not a second instruction and must still parse, or
// every client that ends its payload with a newline would break.
func TestNewlineEndsTheInstruction(t *testing.T) {
	accepted := []string{
		"recall zebras\n",
		"recall zebras\n\n",
		"recall zebras \n  \n\t",
		"remember 'a fact'\n",
	}

	for _, q := range accepted {
		t.Run("accepted:"+strconv.Quote(q), func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q); err != nil {
				t.Errorf("Parse(%q) = %v, want a trailing newline to be ignored", q, err)
			}
		})
	}

	rejected := []string{
		"recall zebras\nfood",
		"recall zebras\nrecall food",
		"remember 'a'\nremember 'b'",
	}

	for _, q := range rejected {
		t.Run("rejected:"+strconv.Quote(q), func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a second-instruction error", q)
			}
			if !strings.Contains(err.Error(), "one command per instruction") {
				t.Errorf("error %q does not say one command per instruction", err)
			}
		})
	}
}

// TestIntegerValueTooLargeIsReportedAsOutOfRange pins that a number too large to
// hold is reported apart from text that is not a number at all. An agent told
// only "invalid" retries with another huge number; one told the value is out of
// range knows to shrink it. Both messages still name the clause that rejected it.
func TestIntegerValueTooLargeIsReportedAsOutOfRange(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"recall zebras depth:99999999999999999999", "invalid depth value"},
		{"recall zebras depth:99999999999999999999", "out of range"},
		{"recall zebras top:99999999999999999999", "invalid top value"},
		{"recall zebras top:99999999999999999999", "out of range"},
		// Not a number at all: named, but not as a range problem.
		{"recall zebras depth:abc", "expected a non-negative whole number"},
		{"recall zebras top:top", "invalid top value"},
	}

	for _, tc := range cases {
		t.Run(tc.query+"/"+tc.want, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a value error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}

	if _, _, err := parser.Parse[uint64, float32]("recall zebras depth:abc"); err == nil ||
		strings.Contains(err.Error(), "out of range") {
		t.Errorf("depth:abc = %v, want it reported as not-a-number rather than out of range", err)
	}
}

// TestMiscasedClauseWarns pins the mis-cased keyword warning: the query ran,
// and a keyword in it was not lower case.
//
// The colon leaves a mis-cased clause one reading, so it is not an error (see
// TestMiscasedKeywordBeforeColonIsTheClause: erroring there would report the
// casing instead of the duplicate it may be hiding). But the language is lower
// case, and accepting `Depth:2` in silence teaches the caller the opposite, so
// the clause runs and the response says which clause it ran as.
//
// Anchor values are excluded: `entity:Top` is data, folded to the same anchor
// as `entity:top`, and an agent never has to remember how it capitalised
// something.
func TestMiscasedClauseWarns(t *testing.T) {
	cases := []struct {
		name  string
		query string
		warns bool
	}{
		{"mis-cased modifier", "recall zebras TOP:3", true},
		{"mis-cased anchor key", "recall zebras Topic:food", true},
		{"mis-cased time bound", "recall zebras Since:7d", true},
		{"mis-cased on a write", "remember 'a fact' Entity:bob", true},
		{"one warning per mis-cased clause", "recall zebras Topic:food Depth:2", true},
		{"lower case is silent", "recall zebras top:3 topic:food", false},
		{"an anchor value is data, not syntax", "recall zebras entity:Top", false},
		{"a mis-cased value on a write is data too", "remember 'a fact' topic:Food", false},
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

	// Every mis-cased clause is named, so a query with two gets two warnings
	// and the caller can fix both in one pass.
	_, warns, err := parser.Parse[uint64, float32]("recall zebras Topic:food Depth:2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 2 {
		t.Fatalf("got %d warnings %v, want one per mis-cased clause", len(warns), warns)
	}
	joined := warns[0].String() + " " + warns[1].String()
	for _, want := range []string{`"Topic"`, "topic clause", `"Depth"`, "depth clause", "lower case"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings %q do not mention %q", joined, want)
		}
	}
}

// TestDepthWithoutAnchorWarns pins the depth warning: the graph is entered
// only through an anchor the recall names, so a depth above 0 on a recall
// naming no topic or entity has no effect. The query runs and the response
// says so. Nothing warns when the recall names an anchor, when the depth is 0
// (it asks for nothing the recall cannot do), or when the clause is omitted
// (the operator's default, not the caller's choice).
func TestDepthWithoutAnchorWarns(t *testing.T) {
	cases := []struct {
		name  string
		query string
		warns bool
	}{
		{"depth on a term-only recall", "recall ferry depth:2", true},
		{"depth on a vector-only recall", "recall vec:$v depth:1", true},
		{"depth beside an anchor alone", "recall topic:harbour depth:2", false},
		{"depth beside a term and an anchor", "recall ferry topic:harbour depth:2", false},
		{"depth beside a vector and an anchor", "recall vec:$v entity:acme depth:1", false},
		{"the floor never warns", "recall ferry depth:0", false},
		{"no depth clause never warns", "recall ferry", false},
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

// TestDepthWithoutAnchorWarningIsActionable pins the message: it names the
// clause that had no effect and what the graph needs, positioned at the
// clause like every other warning, so a caller can fix the query from the
// response alone — add an anchor, or drop the clause.
func TestDepthWithoutAnchorWarningIsActionable(t *testing.T) {
	q := "recall ferry depth:2"
	_, warns, err := parser.Parse[uint64, float32](q)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", q, err)
	}
	if len(warns) != 1 {
		t.Fatalf("Parse(%q) warnings = %v, want exactly one", q, warns)
	}

	msg := warns[0].String()
	for _, want := range []string{
		"depth:2 has no effect",   // the clause that did nothing
		"topic:/entity:",          // what opens the graph
		"names none",              // why this recall could not
		"parse warning at column", // positioned like an error
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("warning %q does not contain %q", msg, want)
		}
	}
}

// TestSelectorWithSpace pins that whitespace between a command and its graph
// selector is a query error that says so: the selector is glued to the verb,
// and a message that only blames the "@" leaves the caller to guess the fix.
func TestSelectorWithSpace(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember @2 signal", "no space allowed between remember and @"},
		{"recall @3 'car park'", "no space allowed between recall and @"},
		{"recall  @5 'names of'", "no space allowed between recall and @"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a selector-spacing error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestWhitespaceSeparatesWords pins that any run of space, tab or carriage
// return separates the words of a query, wherever it falls, and that a NUL
// inside quotes is data like any other character.
func TestWhitespaceSeparatesWords(t *testing.T) {
	cases := []struct {
		query string
		terms []string
	}{
		{"recall  zebras", []string{"zebras"}},
		{"recall\tzebras", []string{"zebras"}},
		{"recall zebras ", []string{"zebras"}},
		{"recall 'zebras\x00food'", []string{"zebras\x00food"}},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])
			if got := rc.Terms(); !slices.Equal(got, tc.terms) {
				t.Errorf("Terms() = %q, want %q", got, tc.terms)
			}
		})
	}
}

// TestNoSpaceInsideACommandOrClause pins that the parts of a command or clause
// are glued: the selector to its verb, the value to its ':' and a parameter
// name to its '$'. A space inside one is rejected with a message naming where
// it is not allowed, since "unexpected" leaves the caller to guess the fix.
// want is empty for a query that must parse.
func TestNoSpaceInsideACommandOrClause(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"remember@2 'x'", ""},
		{"remember @2 'x'", "no space allowed between remember and @"},
		{"remember@ 2 'x'", "no space allowed between @ and the selector"},
		{"recall@3@5 zebras", "one selector"},
		{"recall zebras topic:food", ""},
		{"recall zebras topic :food", "no space allowed before :"},
		{"recall zebras topic: food", "no space allowed after :"},
		{"recall zebras : topic:food", "stray"},
		{"recall zebras vec:$v", ""},
		{"recall zebras vec: $v", "no space allowed after :"},
		{"recall zebras vec:$ v", "no space allowed after $"},
		{"recall zebras since:7d until:'2026-01-15' depth:2 top:5", ""},
		{"recall zebras since :7d", "no space allowed before :"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Parse(%q) = %v, want it to parse", tc.query, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an error containing %q", tc.query, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestReservedWordAsATermNamesBothFixes pins that a reserved word written as a
// bare term is an error in every position and any casing, and that the message
// offers both readings: the clause, with the next word as its value when there
// is one, and the quote that searches the word. The error points at the
// keyword itself, the word the caller has to change.
func TestReservedWordAsATermNamesBothFixes(t *testing.T) {
	cases := []struct {
		query  string
		column int
		want   []string
	}{
		{"recall top", 10, []string{"write top:<value>", "quote it ('top')"}},
		{"recall Top", 10, []string{"write top:<value>", "quote it ('Top')"}},
		{"recall zebras top", 17, []string{"write top:<value>", "quote it ('top')"}},
		{"recall zebras topic food", 19, []string{"write topic:food", "quote it ('topic')"}},
		{"recall since 7d", 12, []string{"write since:7d", "quote it ('since')"}},
		{"recall zebras entity alice extra", 20, []string{"write entity:alice", "quote it ('entity')"}},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			var perr *parser.Error
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%q) = %v, want a positioned parse error", tc.query, err)
			}
			if perr.Pos.Column != tc.column {
				t.Errorf("Parse(%q) error at column %d, want %d", tc.query, perr.Pos.Column, tc.column)
			}
			for _, want := range tc.want {
				if !strings.Contains(perr.Msg, want) {
					t.Errorf("error %q does not contain %q", perr.Msg, want)
				}
			}
		})
	}
}

// TestQuotedReservedWordIsATerm pins the escape the reserved-word error points
// to: quoted, a reserved word in any casing is an ordinary search term, and
// the query runs without a warning.
func TestQuotedReservedWordIsATerm(t *testing.T) {
	cases := []struct {
		query string
		terms []string
	}{
		{"recall 'top'", []string{"top"}},
		{"recall 'Top' 'since'", []string{"top", "since"}},
		{"recall zebras 'topic' food", []string{"zebras", "topic", "food"}},
		{"recall 'since' 7d", []string{"since", "7d"}},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			cmd, warns, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			if len(warns) != 0 {
				t.Errorf("Parse(%q) warnings = %v, want none", tc.query, warns)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])
			if got := rc.Terms(); !slices.Equal(got, tc.terms) {
				t.Errorf("Terms() = %q, want %q", got, tc.terms)
			}
		})
	}
}

// TestKeywordAfterAClauseIsAMissingColon pins the one reading a keyword has
// once a clause has started: terms come first, so it cannot be a word to
// quote, and the message names only the clause the caller meant, with the
// next word as its value.
func TestKeywordAfterAClauseIsAMissingColon(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"recall zebras since:7d topic food", "write topic:food"},
		{"recall zebras top:5 depth 2", "write depth:2"},
		{"remember 'zebras eat grass' topic food entity:x", "write topic:food"},
		{"recall x topic:y top 5", "write top:5"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](tc.query)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-colon error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "quote it") {
				t.Errorf("error %q suggests a quote, but no term can stand after a clause", err)
			}
		})
	}
}

// TestAnchorValueIsData pins that after a ':' only a value can appear: a
// reserved word, any casing and a quoted phrase with spaces or an apostrophe
// all name an anchor, folded to lower case, and none of them warns.
func TestAnchorValueIsData(t *testing.T) {
	cases := []struct {
		query    string
		topics   []string
		entities []string
	}{
		{"recall x topic:top", []string{"top"}, nil},
		{"recall x entity:Top", nil, []string{"top"}},
		{"recall x topic:'US elections'", []string{"us elections"}, nil},
		{"recall x topic:'My Project'", []string{"my project"}, nil},
		{"recall x entity:'O''Brien'", nil, []string{"o'brien"}},
		{"recall x topic:'since'", []string{"since"}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			cmd, warns, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			if len(warns) != 0 {
				t.Errorf("Parse(%q) warnings = %v, want none", tc.query, warns)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])
			if got := rc.Topics(); !slices.Equal(got, tc.topics) {
				t.Errorf("Topics() = %q, want %q", got, tc.topics)
			}
			if got := rc.Entities(); !slices.Equal(got, tc.entities) {
				t.Errorf("Entities() = %q, want %q", got, tc.entities)
			}
		})
	}
}

// TestStopWordTermWarns pins the stop-word warning: stored facts are cleaned
// of stop words at index time, so a bare stop word can never match, and the
// query runs with one warning per such term, positioned at the term. A phrase
// or a vector beside it is a real seed, so it stays a warning.
func TestStopWordTermWarns(t *testing.T) {
	type warning struct {
		term   string
		column int
	}
	cases := []struct {
		query string
		warns []warning
	}{
		{"recall the parrot", []warning{{"the", 10}}},
		{"recall parrot and the zebra", []warning{{"and", 17}, {"the", 21}}},
		{"recall the 'parrot'", []warning{{"the", 10}}},
		{"recall the vec:$v", []warning{{"the", 10}}},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			if len(warns) != len(tc.warns) {
				t.Fatalf("Parse(%q) warnings = %v, want %d", tc.query, warns, len(tc.warns))
			}
			for i, want := range tc.warns {
				msg := fmt.Sprintf("term %q is a stop word: stored facts never contain it, so it cannot match", want.term)
				if warns[i].Msg != msg {
					t.Errorf("warning %d = %q, want %q", i, warns[i].Msg, msg)
				}
				if warns[i].Pos.Column != want.column {
					t.Errorf("warning %d at column %d, want %d", i, warns[i].Pos.Column, want.column)
				}
			}
		})
	}
}

// TestStopWordOnlyRecallIsAnError pins that a recall whose every bare term is
// a stop word, with no phrase and no vector, is an error rather than an empty
// result that looks like a miss. Anchors do not rescue it: the query that was
// meant is the anchor alone.
func TestStopWordOnlyRecallIsAnError(t *testing.T) {
	for _, q := range []string{
		"recall the",
		"recall the and",
		"recall the topic:birds",
	} {
		t.Run(q, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a stop-word-only error", q)
			}
			for _, want := range []string{
				`term "the" is a stop word`,
				"so nothing can match; give a term that is not a stop word, a phrase, or a vector",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

// TestUnambiguousQueriesRunSilently pins where no warning may fire: a clause
// written correctly, depth beside an anchor it can act through, a phrase whose
// content is only stop words, and a recall seeded by an anchor alone. A
// warning there is noise the caller learns to ignore.
func TestUnambiguousQueriesRunSilently(t *testing.T) {
	for _, q := range []string{
		"recall x since:7d",
		"recall parrot topic:birds depth:2",
		"recall 'the parrot'",
		"recall 'the'",
		"recall topic:birds",
	} {
		t.Run(q, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](q)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", q, err)
			}
			if len(warns) != 0 {
				t.Errorf("Parse(%q) warnings = %v, want none", q, warns)
			}
		})
	}
}

// TestStringRoundTripsToTheSameCommand pins that String() prints a query that
// parses back to the same command: same reconstruction, same terms, anchors
// and fact. Every shape the tests from TestWhitespaceSeparatesWords on accept
// is here, plus a quoted form of each reserved word, since a bare one would
// not parse.
func TestStringRoundTripsToTheSameCommand(t *testing.T) {
	for _, q := range []string{
		"recall zebras",
		"recall  zebras",
		"recall\tzebras",
		"recall zebras ",
		"recall zebras \n  \n\t",
		"recall 'zebras\x00food'",
		"remember@2 'x'",
		"recall zebras topic:food",
		"recall zebras vec:$v",
		"recall zebras since:7d until:'2026-01-15' depth:2 top:5",
		"recall 'top'",
		"recall 'Top' 'since'",
		"recall zebras 'topic' food",
		"recall x topic:top",
		"recall x entity:Top",
		"recall x topic:'US elections'",
		"recall x topic:'My Project'",
		"recall x entity:'O''Brien'",
		"recall x topic:'since'",
		"recall x TOP:3",
		"recall x Topic:food",
		"recall the parrot",
		"recall parrot and the zebra",
		"recall 'the parrot'",
		"recall 'the'",
		"recall the 'parrot'",
		"recall the vec:$v",
		"recall topic:birds",
		"recall 'since' 7d",
		"recall x since:7d",
		"recall parrot topic:birds depth:2",
		"recall parrot depth:2",
		"recall 'top' topic:'US elections' since:'2026-01-15' top:5",
		"remember@2 'a fact' topic:'my project' entity:'O''Brien'",
		"recall 'recall'",
		"recall 'remember'",
		"recall 'forget'",
		"recall 'update'",
		"recall 'topic'",
		"recall 'entity'",
		"recall 'since'",
		"recall 'until'",
		"recall 'top'",
		"recall 'depth'",
		"recall 'vec'",
	} {
		t.Run(q, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](q)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", q, err)
			}
			again, _, err := parser.Parse[uint64, float32](cmd.String())
			if err != nil {
				t.Fatalf("Parse(String() = %q) = %v, want the reconstruction to parse", cmd.String(), err)
			}
			if again.String() != cmd.String() {
				t.Errorf("String() after a round trip = %q, want %q", again.String(), cmd.String())
			}
			switch c := cmd.(type) {
			case *parser.RecallCommandNode[uint64, float32]:
				a := again.(*parser.RecallCommandNode[uint64, float32])
				if !slices.Equal(a.Terms(), c.Terms()) || !slices.Equal(a.Topics(), c.Topics()) || !slices.Equal(a.Entities(), c.Entities()) {
					t.Errorf("round trip changed the recall: terms %q topics %q entities %q, want %q %q %q",
						a.Terms(), a.Topics(), a.Entities(), c.Terms(), c.Topics(), c.Entities())
				}
			case *parser.RememberCommandNode[float32]:
				a := again.(*parser.RememberCommandNode[float32])
				if a.Value() != c.Value() || !slices.Equal(a.Topics(), c.Topics()) || !slices.Equal(a.Entities(), c.Entities()) {
					t.Errorf("round trip changed the remember: value %q topics %q entities %q, want %q %q %q",
						a.Value(), a.Topics(), a.Entities(), c.Value(), c.Topics(), c.Entities())
				}
			}
		})
	}
}

// This is a bug previously introduced when regorganising the query parser
// adding the test as it's an unwanted behaviour and failing to raising an error
// should be flagged.
func TestTokenGluedToPhrase(t *testing.T) {
	for _, q := range []string{
		"remember 'a fact'x",
		"remember 'a fact'zzz topic:x",
	} {
		t.Run(q, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q)
			if err == nil {
				t.Fatalf("Expected error: parse error at column 18: unexpected \"x\" but got: %q", err)
			}
		})
	}
}
