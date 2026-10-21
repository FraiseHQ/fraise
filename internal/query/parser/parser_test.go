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
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"

	"github.com/FraiseHQ/fraise/internal/query/lexer"
	"github.com/FraiseHQ/fraise/internal/query/parser"
)

// FuzzRememberPhraseRoundTrip is the server-side guarantee that quoting can
// encode any free text. Take any valid UTF-8 value that is not blank. With its
// apostrophes doubled and quotes around it, it parses back to exactly that
// value. The String() reconstruction parses back to it as well. Ingestion may
// feed a phrase any character JSON can carry. So nothing between the quotes may
// be lost or altered, or end the phrase early. Invalid UTF-8 is skipped,
// because JSON decoding replaces it before a query reaches the parser. A blank
// value is skipped, because an empty fact is rejected.
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

		cmd, _, err := parser.Parse[uint64, float32](q, nil)
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
		cmd2, _, err := parser.Parse[uint64, float32](cmd.String(), nil)
		if err != nil {
			t.Fatalf("re-Parse(String() = %q) = %v", cmd.String(), err)
		}
		if got := cmd2.(*parser.RememberCommandNode[float32]).Value(); got != value {
			t.Fatalf("re-parsed value = %q, want %q", got, value)
		}
	})
}

// FuzzParseNeverPanics feeds the parser arbitrary wire input. Raw queries come
// from any HTTP client, not just the SDK. On garbage the server's only
// acceptable answers are a parsed query or a positioned error. It must never
// panic, and never return both or neither. An error's column must never point
// outside the query, since the column is what an agent repairs the query from.
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
		cmd, _, err := parser.Parse[uint64, float32](raw, nil)
		if err == nil {
			if cmd == nil {
				t.Fatalf("Parse(%q) = nil command and nil error, want one or the other", raw)
			}
			return
		}
		if cmd != nil {
			t.Fatalf("Parse(%q) = %v alongside error %v, want no command on error", raw, cmd, err)
		}
		var perr *parser.Error
		if !errors.As(err, &perr) {
			t.Fatalf("Parse(%q) error %v is %T, want a *parser.Error carrying its position", raw, err, err)
		}
		// A column names the rune the error blames. End of input blames the
		// last rune. Only an empty query has no rune to name.
		if col, end := perr.Pos.Column, utf8.RuneCountInString(raw); col < 0 || col > end {
			t.Fatalf("Parse(%q) error at column %d, want a column in [0, %d]", raw, col, end)
		}
	})
}

// FuzzRoundTripAllCommands pins that every command the parser accepts renders
// back to a query that parses to the same command. String() is how a parsed
// query is shown back to an agent. A rendering that does not reparse hands the
// agent a query it cannot send. A rendering that drifts on a second trip gives
// one query two spellings that hash apart in the plan cache.
func FuzzRoundTripAllCommands(f *testing.F) {
	for _, seed := range []string{
		"recall x",
		"recall@2 x 'a phrase' topic:'y' entity:anne",
		"recall x since:7d until:'2026-01-15' depth:2 top:5",
		"recall x since:'2026-01-15T10:30:00.123456789Z'",
		"recall topic:x vec:$v",
		"remember 'a fact' topic:x entity:anne vec:$v",
		"remember@1 'it''s got an apostrophe' topic:x",
		"recall 'déjà vu 😀 東京'",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		cmd, _, err := parser.Parse[uint64, float32](raw, nil)
		if err != nil {
			t.Skip("only an accepted query has a rendering to read back")
		}
		rendered := cmd.String()
		cmd2, _, err := parser.Parse[uint64, float32](rendered, nil)
		if err != nil {
			t.Fatalf("Parse(%q).String() = %q, which does not reparse: %v", raw, rendered, err)
		}
		if got := cmd2.String(); got != rendered {
			t.Fatalf("Parse(%q) rendering is unstable: %q, then %q", raw, rendered, got)
		}
	})
}

// TestDurationsAreBoundedWithTheRangeInTheMessage pins the duration bound at
// the level an agent sees. A count past what its unit can hold is a parse error
// naming the largest count allowed. It never wraps silently into a future
// bound. The largest count still parses.
func TestDurationsAreBoundedWithTheRangeInTheMessage(t *testing.T) {
	cases := map[string]struct {
		want string // "" when the query must parse
	}{
		"recall x since:106751d":               {""},
		"recall x since:106752d":               {"invalid since value \"106752d\": out of range (at most 106751d)"},
		"recall x until:15251w":                {"invalid until value \"15251w\": out of range (at most 15250w)"},
		"recall x since:99999999999999999999d": {"out of range (at most 106751d)"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Parse(%q) = %v, want it to parse", query, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse(%q) = %v, want an error containing %q", query, err, tc.want)
			}
		})
	}
}

// TestTimeValuesAreQuotedOnlyWhenAbsolute pins the grammar's split between the
// two time forms. A date carries a '-' and is quoted. A duration is a bare
// word. A quoted duration is an error naming the unquoted spelling. So one
// clause has one spelling, and the quote keeps telling a date from a duration.
func TestTimeValuesAreQuotedOnlyWhenAbsolute(t *testing.T) {
	cases := map[string]struct {
		want string // "" when the query must parse
	}{
		"recall x since:7d":                     {""},
		"recall x since:'2026-01-15'":           {""},
		"recall x until:'2026-01-15T10:00:00Z'": {""},
		"recall x since:'7d'":                   {`invalid since value "7d": a duration is not quoted — write since:7d`},
		"recall x until:'3h'":                   {`invalid until value "3h": a duration is not quoted — write until:3h`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Parse(%q) = %v, want it to parse", query, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse(%q) = %v, want an error containing %q", query, err, tc.want)
			}
		})
	}
}

// TestClauseErrorsSurfaceUnmangled pins that a clause helper's positioned error
// reaches the caller as is. It is not re-wrapped. Nor is it garbled by a bad
// format verb into `&{%!e(string=...)}`, which would destroy the message an
// agent needs to correct itself.
func TestClauseErrorsSurfaceUnmangled(t *testing.T) {
	cases := map[string]struct {
		want string // substring of the inner, positioned error
	}{
		"recall x since:soon":  {"invalid since value"},
		"recall x until:later": {"invalid until value"},
		"recall x depth:abc":   {"invalid depth value"},
		"recall x top:abc":     {"invalid top value"},
		"recall x topic:":      {"expected a word or quoted phrase"},
		// vec: on both commands. parseVecField's positioned error must reach
		// the caller unwrapped, like every other clause's.
		"recall x vec:v":           {"expected param field operator $"},
		"remember 'a fact' vec$:v": {"is missing its ':'"}, // a remember, since among a recall's terms a bare vec is a keyword term
		"recall x vec:$":           {"expected literal"},
		"remember 'a fact' vec:v":  {"expected param field operator $"},
		"remember 'a fact' vec:$":  {"expected literal"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a parse error", query)
			}
			got := err.Error()
			if !strings.Contains(got, tc.want) {
				t.Errorf("error %q does not contain the inner message %q", got, tc.want)
			}
			if strings.Contains(got, "%!e") || strings.Contains(got, "Error while parsing") {
				t.Errorf("error %q is re-wrapped/mangled, want the inner error as-is", got)
			}
		})
	}
}

func TestRememberParser(t *testing.T) {
	q := "remember@1 'anne loves the color orange' topic:color topic:preference entity:anne vec:$v"

	qo, _, err := parser.Parse[uint64, float32](q, nil)
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
	}

	if got, want := qo.String(), q; got != want {
		t.Errorf("round-trip mismatch: String() = %q, want %q", got, want)
	}
}

// TestRecallParser checks that valid recall queries parse without error and
// round-trip through String(). Fields are written in the order String() emits
// them, so the reconstruction matches: terms, entities, topics, top, depth,
// since, until. A value that was quoted comes back quoted. Printed bare,
// 'e-mail' or '2026-01-15' would not parse.
func TestRecallParser(t *testing.T) {
	queries := map[string]struct{}{
		"recall anna":             {},
		"recall anna bob charlie": {},
		"recall@2 anna":           {},
		"recall@2 anna bob entity:alice topic:job top:10 depth:5":    {},
		"recall 'e-mail' entity:'o''brien' topic:'machine-learning'": {},
		"recall@2 anna since:7d until:'2026-01-15'":                  {},
		"recall anna since:'2026-01-15T10:00:00Z'":                   {},
	}

	for q := range queries {
		t.Run(q, func(t *testing.T) {
			qo, _, err := parser.Parse[uint64, float32](q, nil)
			if err != nil {
				t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
			}
			if got := qo.String(); got != q {
				t.Errorf("round-trip mismatch: String() = %q, want %q", got, q)
			}
		})
	}
}

// TestRecallParserErrors checks that malformed recall queries are rejected, and
// that the message says what the recall is missing.
func TestRecallParserErrors(t *testing.T) {
	const noSeed = "a recall needs at least one seed: a term, a topic:/entity: anchor, or vec:$<name>"
	cases := map[string]struct {
		want string
	}{
		"recall": {"expected a space after the command, found end of input"},
		// Modifiers scope a search. They cannot start one.
		"recall top:3":         {noSeed},
		"recall since:7d":      {noSeed},
		"recall depth:2 top:5": {noSeed},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestAnchorSeededRecallParses pins that a recall needs a seed, not a *term*.
// An anchor and a vector are seeds too, so "everything about billing" is a
// question the grammar can express. Requiring a term would make callers invent
// a keyword to get past the parser. The search would then be seeded with a word
// they did not mean.
func TestAnchorSeededRecallParses(t *testing.T) {
	queries := map[string]struct{}{
		"recall topic:job":               {},
		"recall entity:alice":            {},
		"recall topic:job entity:alice":  {},
		"recall@2 topic:job":             {},
		"recall topic:job top:5 depth:2": {},
		"recall topic:job since:7d":      {},
		"recall vec:$v":                  {},
	}

	for q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q, nil); err != nil {
				t.Errorf("Parse(%q) = %v, want it to parse", q, err)
			}
		})
	}
}

// TestGraphSelectorRejectsOutOfRange checks that a selector that does not fit
// in a uint8 is rejected at parse time. It is not silently wrapped into a
// valid-looking graph (@256 -> 0, @300 -> 44). Wrapping would route the query
// to the wrong tenant's graph, so this must fail before execution. A
// non-integer selector is rejected too. Both recall and remember are covered.
// The message names the selector and the range, so the caller knows which graph
// id to fix and what it may be.
func TestGraphSelectorRejectsOutOfRange(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"recall@256 secret":                  {"graph selector 256 out of range (0-255)"},
		"recall@300 secret":                  {"graph selector 300 out of range (0-255)"},
		"recall@-1 secret":                   {`invalid graph selector "-": expected a whole number`},
		"recall@abc secret":                  {`invalid graph selector "abc": expected a whole number`},
		"remember@256 'secret plan' topic:x": {"graph selector 256 out of range (0-255)"},
		"remember@300 'secret plan' topic:x": {"graph selector 300 out of range (0-255)"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an out-of-range/parse error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}

	// A selector that fits in a uint8 still parses here. The tighter
	// [0, num-graphs) bound is the handler's job, not the parser's.
	if _, _, err := parser.Parse[uint64, float32]("recall@255 secret", nil); err != nil {
		t.Errorf("Parse(recall@255 …) returned error: %v, want nil", err)
	}
}

// TestRememberPhrase covers the opaque single-quoted phrase. Reserved words and
// symbols (: $ @ ( )) inside it are stored verbatim. A doubled single quote is
// an escaped apostrophe.
func TestRememberPhrase(t *testing.T) {
	cases := map[string]struct {
		query string
		want  string // the fact that should be stored (RememberCommandNode.Value)
	}{
		"colon in phrase":            {"remember 'meeting at 3:30pm with anna' topic:meetings", "meeting at 3:30pm with anna"},
		"reserved word in phrase":    {"remember 'remind me about this topic later' topic:reminders", "remind me about this topic later"},
		"multiple reserved words":    {"remember 'recall the top since until depth vec entity' topic:x", "recall the top since until depth vec entity"},
		"symbols in phrase":          {"remember 'email $bill @ (acme)' topic:contacts", "email $bill @ (acme)"},
		"escaped apostrophe":         {"remember 'alice''s laptop' topic:devices", "alice's laptop"},
		"apostrophe at edges":        {"remember '''quoted''' topic:x", "'quoted'"},
		"interior spacing preserved": {"remember 'a   b' topic:x", "a   b"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query, nil)
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

// TestRememberPhraseRoundTrip checks that a fact with an apostrophe survives
// String() reconstruction, because the inner quote is doubled again. A quoted
// anchor value comes back quoted. The selector is printed only when it was
// written, as a recall's is.
func TestRememberPhraseRoundTrip(t *testing.T) {
	queries := map[string]struct{}{
		"remember 'alice''s laptop' topic:devices":                       {},
		"remember@0 'alice''s laptop' topic:devices":                     {},
		"remember@0 'a fact' topic:'machine-learning' entity:'o''brien'": {},
	}

	for q := range queries {
		t.Run(q, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](q, nil)
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
	cases := map[string]struct {
		want string
	}{
		"remember 'unterminated phrase topic:x": {"unterminated quoted phrase"},
		"remember topic:x":                      {`expected a quoted phrase, but found "topic"`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestFieldRequiresColonSeparator pins that every keyed field rejects a missing
// ':', instead of advancing past whatever token sits in the separator's place.
// Advancing would shift the remaining tokens into other roles, and answer a
// different query with no error. An agent cannot detect that failure, let alone
// correct it. The whole family (topic, entity, since, until, top, depth) is
// listed, so no field can regress alone.
func TestFieldRequiresColonSeparator(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		// The fix names the clause the caller would write, with the word they
		// wrote after the keyword as its value. That is the query they meant.
		"recall zebras topic food":                  {"write topic:food"},
		"recall zebras topic food extra":            {"write topic:food"},
		"recall zebras entity alice":                {"write entity:alice"},
		"recall zebras since 7d":                    {"write since:7d"},
		"recall zebras until 30d":                   {"write until:30d"},
		"recall zebras top 5":                       {"write top:5"},
		"recall zebras depth 2":                     {"write depth:2"},
		"recall zebras topic:food entity alice":     {"write entity:alice"},
		"remember 'zebras eat grass' topic food":    {"write topic:food"},
		"remember 'zebras eat grass' entity zebras": {"write entity:zebras"},
		// The token-shifting shapes. Without the check, the second value would
		// silently win the field and the first would be swallowed.
		"recall zebras since 7d 30d":           {"write since:7d"},
		"recall zebras until 7d 30d":           {"write until:7d"},
		"recall zebras since:7d until 30d 60d": {"write until:30d"},
		// A quoted duration is spelled back bare. The repair since:'7d' would
		// itself be rejected as a quoted duration, sending the caller from one
		// error to the next. A quoted date keeps its quotes, and so does a
		// signed count, which is no duration.
		"recall x topic:y since'7d'":          {`"since" is missing its ':': write since:7d`},
		"recall x topic:y since '7d'":         {`"since" is missing its ':': write since:7d`},
		"recall x since '7d'":                 {`term "since" is also a keyword: write since:7d if a filter was meant, or quote it ('since') to search the word`},
		"recall x topic:y until '3h'":         {`"until" is missing its ':': write until:3h`},
		"recall x topic:y since '2026-01-15'": {`"since" is missing its ':': write since:'2026-01-15'`},
		"recall x topic:y since '+7d'":        {`"since" is missing its ':': write since:'+7d'`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-separator error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name the missing ':' separator: want %q", err, tc.want)
			}
		})
	}
}

// TestParseErrorBlamesTheOffendingToken pins where a parse error points. The
// column is the last character of the token the message quotes.
//
// The lexer's CurrentPos is not where the parser is. The parser's cur and peek
// read one token ahead, so CurrentPos sits at the end of the token after the
// bad one. So every case below keeps a token after the offending one. With the
// bad token last, an error positioned at CurrentPos would pass unnoticed.
//
// The column and the quoted literal are asserted together. Either alone can
// look right while the pair disagrees. And the pair is what an agent or a human
// uses to find the mistake.
func TestParseErrorBlamesTheOffendingToken(t *testing.T) {
	cases := map[string]struct {
		blame string // the token the error must quote and point at
	}{
		// Missing ':', for the whole keyed-field family. Among the terms the
		// blame falls on the keyword, the word to write as a filter or quote.
		"recall zebras topic food extra":   {"topic"},
		"recall zebras entity alice extra": {"entity"},
		"recall zebras since 7d 30d":       {"since"},
		"recall zebras until 7d 30d":       {"until"},
		"recall zebras top 5 depth:2":      {"top"},
		"recall zebras depth 2 top:3":      {"depth"},
		// Unparseable values: the blamed token is the value, not the key.
		"recall zebras since:soon top:3":  {"soon"},
		"recall zebras until:later top:3": {"later"},
		"recall zebras depth:abc top:3":   {"abc"},
		"recall zebras top:abc depth:2":   {"abc"},
		"recall@abc zebras top:3":         {"abc"},
		// A token in a position no clause can start.
		"recall zebras : topic:food": {":"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a parse error", query)
			}

			var perr *parser.Error
			if !errors.As(err, &perr) {
				t.Fatalf("error %v is not a *parser.Error, so it carries no position", err)
			}

			// 1-based column of the offending token's last character.
			want := strings.Index(query, tc.blame) + len(tc.blame)
			if got := perr.Pos.Column; got != want {
				t.Errorf("%q: column %d, want %d (the end of %q)\n  %s\n  %*s\n  %v",
					query, got, want, tc.blame,
					query, got, "^", err)
			}
			if !strings.Contains(perr.Msg, strconv.Quote(tc.blame)) {
				t.Errorf("%q: message %q does not quote the offending token %q",
					query, perr.Msg, tc.blame)
			}
		})
	}
}

// TestQuotedValues checks that quoting also works for anchor values and recall
// terms. So a topic or a search term can itself contain spaces, symbols, or a
// reserved word.
func TestQuotedValues(t *testing.T) {
	t.Run("quoted anchor value", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("remember 'x' topic:'my project'", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RememberCommandNode[float32])
		if diff := cmp.Diff([]string{"my project"}, rc.Topics()); diff != "" {
			t.Errorf("Topics() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("quoted recall term", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("recall 'meeting at 3:30pm' topic:work", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RecallCommandNode[uint64, float32])
		if diff := cmp.Diff([]string{"meeting at 3:30pm"}, rc.Terms()); diff != "" {
			t.Errorf("Terms() mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestBareWordIsLettersAndDigits pins the word rule from the accepting side. A
// bare word is letters, in any script, and digits. Any other character a value
// needs is written inside quotes, where it is data. Rejecting "e-mail" bare is
// only safe while its quoted form reaches the same value.
func TestBareWordIsLettersAndDigits(t *testing.T) {
	cases := map[string]struct {
		terms  []string
		topics []string
	}{
		"recall café 東京 42": {[]string{"café", "東京", "42"}, nil},
		"recall 'e-mail' topic:'machine-learning' since:'2026-01-15'": {[]string{"e-mail"}, []string{"machine-learning"}},
		"recall 'ferry;' 'recall bridge'":                             {[]string{"ferry;", "recall bridge"}, nil},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])
			if diff := cmp.Diff(tc.terms, rc.Terms()); diff != "" {
				t.Errorf("Terms() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.topics, rc.Topics()); diff != "" {
				t.Errorf("Topics() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestKeywordAsValue pins that a reserved word after a field's ':' parses as an
// ordinary word, though the lexer types "top" by spelling alone. An entity an
// LLM extracts can be a single reserved word, such as "top" from "she reached
// the top". Rejecting it would fail ingestion with an error the client cannot
// anticipate. As a recall term a reserved word is written quoted, as in the
// last two cases.
func TestKeywordAsValue(t *testing.T) {
	cases := map[string]struct {
		query    string
		entities []string
		topics   []string
		terms    []string
	}{
		"entity top on remember": {
			query:    "remember 'a neutral test value.' topic:'some-topic' entity:top",
			entities: []string{"top"},
			topics:   []string{"some-topic"},
		},
		"topic top on remember": {
			query:  "remember 'a neutral test value.' topic:'some-topic' topic:top",
			topics: []string{"some-topic", "top"},
		},
		"every field keyword as an anchor value": {
			query:    "remember 'x' entity:recall entity:since entity:until entity:depth entity:vec entity:entity topic:topic",
			entities: []string{"recall", "since", "until", "depth", "vec", "entity"},
			topics:   []string{"topic"},
		},
		"keyword anchors on recall": {
			query:    "recall shelf entity:top topic:top",
			entities: []string{"top"},
			topics:   []string{"top"},
			terms:    []string{"shelf"},
		},
		"keyword as the leading recall term": {
			query: "recall 'top'",
			terms: []string{"top"},
		},
		"keyword value followed by a real top clause": {
			query:    "recall 'top' entity:top top:3",
			entities: []string{"top"},
			terms:    []string{"top"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](tc.query, nil)
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

			if diff := cmp.Diff(tc.entities, entities); diff != "" {
				t.Errorf("Entities() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.topics, topics); diff != "" {
				t.Errorf("Topics() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.terms, terms); diff != "" {
				t.Errorf("Terms() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestKeywordAsValueDisambiguation pins the tie-breaker that keeps the rule
// safe. A keyword directly followed by ':' is always a field, never a value. So
// a clause mistyped into value position is an error, not silently consumed as
// data. That is the failure TestFieldRequiresColonSeparator exists to prevent.
// A bare keyword among a recall's terms is an error too.
func TestKeywordAsValueDisambiguation(t *testing.T) {
	queries := map[string]struct{}{
		"recall top:3":                         {}, // a modifier is not a seed
		"recall shelf top":                     {}, // clause position: a bare keyword is not a term
		"remember 'x' entity:top:3":            {}, // keyword-colon after the anchor's ':' is a field, not a value
		"remember 'x' entity:since:7d topic:x": {},
	}

	for q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q, nil); err == nil {
				t.Errorf("Parse(%q) = nil error, want an error", q)
			}
		})
	}
}

// TestMiscasedKeywordIsRejected pins that a keyword written with any upper case
// is a parse error wherever it could still be something else. Case folding
// applies to data only. Letting it reach a keyword would fold
// "recall x Since 7d" into a three-term search. That is the silent token shift
// the separator tests guard against.
//
// The exception is a keyword glued to a ':', which has its own test below. The
// colon leaves nothing to mistake it for. So there the clause runs, and its
// casing earns a warning rather than an error.
func TestMiscasedKeywordIsRejected(t *testing.T) {
	cases := map[string]struct {
		want string // substring the error must carry
	}{
		// Clause position in the term stream.
		"recall zebras Since 7d":         {"lower case"},
		"recall zebras Since 7d 30d":     {"lower case"},
		"recall zebras Until 2026-01-15": {"lower case"},
		"recall zebras Depth 2":          {"lower case"},
		// Command position. A command is never inferred from a mis-spelling,
		// because the verb decides whether the query writes.
		"Recall zebras":        {"expected a command"},
		"REMEMBER 'x' topic:y": {"expected a command"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a mis-cased-keyword error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestMiscasedKeywordBeforeColonIsTheClause pins the one place casing is
// forgiven. A word glued to a ':' can only be a field, because no production
// puts a bare word in front of a colon. Reading "DEPTH:5" as the depth clause
// lets a repeated clause be caught as a duplicate. Blaming the casing instead
// would report the shallower of the two mistakes. An agent that fixed the
// casing would meet the duplicate only on its next try.
//
// It stays narrow on purpose. Away from a ':' the spelling must still match. So
// "Recall x" is not a command, and "recall x Since 7d" is not a time bound (see
// TestMiscasedKeywordIsRejected).
func TestMiscasedKeywordBeforeColonIsTheClause(t *testing.T) {
	recalls := map[string]struct {
		check func(*testing.T, *parser.RecallCommandNode[uint64, float32])
	}{
		"recall zebras TOP:3": {func(t *testing.T, r *parser.RecallCommandNode[uint64, float32]) {
			if got, want := r.Top(0), 3; got != want {
				t.Errorf("Top() = %d, want %d", got, want)
			}
		}},
		"recall zebras Depth:2": {func(t *testing.T, r *parser.RecallCommandNode[uint64, float32]) {
			if !r.HasDepth() || r.Depth(0) != 2 {
				t.Errorf("Depth() = %d (has %v), want 2", r.Depth(0), r.HasDepth())
			}
		}},
		"recall zebras Topic:food": {func(t *testing.T, r *parser.RecallCommandNode[uint64, float32]) {
			if diff := cmp.Diff([]string{"food"}, r.Topics()); diff != "" {
				t.Errorf("Topics() mismatch (-want +got):\n%s", diff)
			}
		}},
	}

	for query, tc := range recalls {
		t.Run(query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) = %v, want the clause it spells", query, err)
			}
			r, ok := cmd.(*parser.RecallCommandNode[uint64, float32])
			if !ok {
				t.Fatalf("Parse(%q) returned %T, want *RecallCommandNode", query, cmd)
			}
			tc.check(t, r)
		})
	}

	remembers := map[string]struct {
		want []string
		got  func(*parser.RememberCommandNode[float32]) []string
	}{
		"remember 'x' Entity:bob": {[]string{"bob"}, (*parser.RememberCommandNode[float32]).Entities},
		"remember 'x' Topic:food": {[]string{"food"}, (*parser.RememberCommandNode[float32]).Topics},
	}

	for query, tc := range remembers {
		t.Run(query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) = %v, want the clause it spells", query, err)
			}
			r, ok := cmd.(*parser.RememberCommandNode[float32])
			if !ok {
				t.Fatalf("Parse(%q) returned %T, want *RememberCommandNode", query, cmd)
			}
			if diff := cmp.Diff(tc.want, tc.got(r)); diff != "" {
				t.Errorf("anchors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDuplicateModifierIsRejected pins that a single-valued clause given twice
// is an error, not a last-wins overwrite. Last-wins is the failure an agent
// cannot detect. The query runs and the results are real, but they answer a
// differently scoped question than the one asked. Anchors are excluded, because
// a repeated topic: or entity: is a list by design. The last two cases pin that
// the stricter rule does not swallow them.
func TestDuplicateModifierIsRejected(t *testing.T) {
	rejected := map[string]struct{}{
		"recall zebras depth:1 depth:2":             {},
		"recall zebras top:3 top:10":                {},
		"recall zebras since:7d since:30d":          {},
		"recall zebras until:1d until:2d":           {},
		"recall zebras since:7d until:30d since:1d": {},
		"recall zebras vec:$a vec:$b":               {},
		"recall zebras depth:2 DEPTH:5":             {},
		"remember 'x' vec:$a vec:$b":                {},
	}

	for q := range rejected {
		t.Run(q, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a duplicate-clause error", q)
			}
			if !strings.Contains(err.Error(), "duplicate") {
				t.Errorf("error %q does not name the repeat as a duplicate", err)
			}
		})
	}

	accepted := map[string]struct{}{
		"recall zebras topic:food topic:drink":  {},
		"recall zebras entity:alice entity:bob": {},
	}

	for q := range accepted {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q, nil); err != nil {
				t.Errorf("Parse(%q) = %v, want repeated anchors to parse", q, err)
			}
		})
	}
}

// TestMiscasedKeywordStaysDataInValuePosition pins the other side of the casing
// rule. Where a token is clearly data (an anchor value, a quoted phrase), upper
// case is legal and folds, keyword spellings included. An extracted entity
// arrives in whatever case the model emitted, so rejecting these would fail
// ingestion.
func TestMiscasedKeywordStaysDataInValuePosition(t *testing.T) {
	t.Run("quoted term after the first", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("recall zebras 'Since'", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RecallCommandNode[uint64, float32])
		if diff := cmp.Diff([]string{"zebras", "since"}, rc.Terms()); diff != "" {
			t.Errorf("Terms() mismatch (-want +got):\n%s", diff)
		}
	})

	// entity:Top folding to the "top" anchor is pinned in
	// TestValuesFoldToLowerCase.
}

// TestValuesFoldToLowerCase pins the case contract. Terms and anchor values are
// identity, not prose, and fold to lower case on the way in. The quoted fact of
// a remember keeps the spelling it was written with. If the fold regresses, one
// anchor exists under as many nodes as it has capitalisations. Recalls then
// silently miss facts filed under another one.
func TestValuesFoldToLowerCase(t *testing.T) {
	t.Run("anchor values fold, the fact does not", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("remember 'MiXeD Case' topic:Billing entity:Anna", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RememberCommandNode[float32])
		if got, want := rc.Value(), "MiXeD Case"; got != want {
			t.Errorf("Value() = %q, want %q, the fact stored exactly as written", got, want)
		}
		if diff := cmp.Diff([]string{"billing"}, rc.Topics()); diff != "" {
			t.Errorf("Topics() mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{"anna"}, rc.Entities()); diff != "" {
			t.Errorf("Entities() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("quoted anchor values fold too", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("remember 'x' topic:'My Project'", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RememberCommandNode[float32])
		if diff := cmp.Diff([]string{"my project"}, rc.Topics()); diff != "" {
			t.Errorf("Topics() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("recall terms fold, bare and quoted alike", func(t *testing.T) {
		cmd, _, err := parser.Parse[uint64, float32]("recall Anna 'Bob Marley' topic:Music", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RecallCommandNode[uint64, float32])
		if diff := cmp.Diff([]string{"anna", "bob marley"}, rc.Terms()); diff != "" {
			t.Errorf("Terms() mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{"music"}, rc.Topics()); diff != "" {
			t.Errorf("Topics() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("a capitalised keyword folds into the same word", func(t *testing.T) {
		// entity:Top and entity:top must land on one anchor. "Top" is a plain
		// LITERAL to the lexer, while "top" is a keyword. The fold stops that
		// lexing difference from leaking into the graph as two anchors.
		cmd, _, err := parser.Parse[uint64, float32]("remember 'x' entity:Top", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rc := cmd.(*parser.RememberCommandNode[float32])
		if diff := cmp.Diff([]string{"top"}, rc.Entities()); diff != "" {
			t.Errorf("Entities() mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestExplicitTopIsVisibleIncludingZero pins that the *presence* of a top
// clause, not a nonzero value, decides whether the configured default applies.
//
// Unlike depth:0, top:0 is not a valid request, because the documented range is
// 1 to db.max-top. But it must survive parsing as an explicit clause. Read off
// the value, it would collapse into the default and skip the range check. A
// caller asking for zero results would then silently get the default count. The
// rejection lives in query.Parse. This test pins that the parser keeps the
// clause visible for it.
func TestExplicitTopIsVisibleIncludingZero(t *testing.T) {
	cases := map[string]struct {
		explicit bool
		want     int // Top(7), where 7 stands in for the configured default
	}{
		"recall x top:0": {true, 0},
		"recall x top:5": {true, 5},
		"recall x":       {false, 7},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
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
// A depth of 0 is the floor lane: seeds only, no anchor traversal. Read off the
// value, it would collapse into the configured default. A client that turned
// the graph channel off would silently get it back. The test asserts both the
// flag and the resolved value, because Depth's fallback is what a caller
// receives.
func TestExplicitDepthIsHonouredIncludingZero(t *testing.T) {
	cases := map[string]struct {
		explicit bool
		want     int // Depth(7), where 7 stands in for the configured default
	}{
		"recall x depth:0": {true, 0},
		"recall x depth:1": {true, 1},
		"recall x depth:2": {true, 2},
		"recall x":         {false, 7},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
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

// TestAnchorBesideASeedRunsAtDepthOneAtLeast pins the depth floor of an
// anchored search. An anchor beside a term or vector takes part only through
// the excess it transmits over the graph, and depth 0 transmits nothing. So
// such a recall resolves to depth 1 whether its depth:0 is written or is the
// operator's default (0 here). A deeper depth is kept. Anchors alone seed the
// search and keep whatever depth they are given.
func TestAnchorBesideASeedRunsAtDepthOneAtLeast(t *testing.T) {
	cases := map[string]int{
		"recall ferry topic:harbour":         1,
		"recall ferry topic:harbour depth:0": 1,
		"recall vec:$v entity:acme depth:0":  1,
		"recall ferry entity:acme depth:2":   2,
		"recall topic:harbour depth:0":       0,
		"recall topic:harbour":               0,
		"recall ferry depth:0":               0,
	}

	params := map[string][]float32{"v": {1, 0}}
	for query, want := range cases {
		t.Run(query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](query, params)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
			}
			if got := cmd.(*parser.RecallCommandNode[uint64, float32]).Depth(0); got != want {
				t.Errorf("Depth(0) = %d, want %d", got, want)
			}
		})
	}
}

// TestVectorIsTheBoundParameter pins that a command's Vector is the parameter
// its vec: clause names. It is nil when there is no clause or no such
// parameter. Most queries have no vector, so a nil dereference on the absent
// clause would take down any caller that reads the vector. And a clause whose
// vector stayed empty would give a vector search nothing to search with.
func TestVectorIsTheBoundParameter(t *testing.T) {
	params := map[string][]float32{"q": {0.1, 0.2, 0.3}}

	cases := map[string]struct {
		vector func(parser.CommandNode) []float32
		want   []float32
	}{
		"recall x": {func(c parser.CommandNode) []float32 {
			return c.(*parser.RecallCommandNode[uint64, float32]).Vector()
		}, nil},
		"recall x vec:$q": {func(c parser.CommandNode) []float32 {
			return c.(*parser.RecallCommandNode[uint64, float32]).Vector()
		}, []float32{0.1, 0.2, 0.3}},
		"recall x vec:$missing": {func(c parser.CommandNode) []float32 {
			return c.(*parser.RecallCommandNode[uint64, float32]).Vector()
		}, nil},
		"remember 'x'": {func(c parser.CommandNode) []float32 {
			return c.(*parser.RememberCommandNode[float32]).Vector()
		}, nil},
		"remember 'x' vec:$q": {func(c parser.CommandNode) []float32 {
			return c.(*parser.RememberCommandNode[float32]).Vector()
		}, []float32{0.1, 0.2, 0.3}},
		"remember 'x' vec:$missing": {func(c parser.CommandNode) []float32 {
			return c.(*parser.RememberCommandNode[float32]).Vector()
		}, nil},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](query, params)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
			}
			// cmp tells a nil vector from an empty one, which is the
			// distinction this test exists to keep.
			if diff := cmp.Diff(tc.want, tc.vector(cmd)); diff != "" {
				t.Errorf("Vector() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestEmptyDataIsRejected pins that a quoted empty or whitespace-only value is
// refused wherever it can be written (see errEmpty for why). The message names
// which value was empty, so the caller knows which one to fill.
func TestEmptyDataIsRejected(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"remember ''":               {"a remembered fact must not be empty"},
		"remember '' topic:harbour": {"a remembered fact must not be empty"},
		"remember '   '":            {"a remembered fact must not be empty"},
		"recall ''":                 {"a search term must not be empty"},
		"recall '   '":              {"a search term must not be empty"},
		"recall ferry ''":           {"a search term must not be empty"},
		"recall ferry topic:''":     {"an anchor value must not be empty"},
		"recall ferry entity:'   '": {"an anchor value must not be empty"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an empty-value error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestRejectedTokensNameTheirOwnMistake pins that a token no production accepts
// is diagnosed, not merely reported. The caller is an agent, and it can only
// make a repair the message spells out. So each shape a caller produces needs
// its own repair instruction, not a shared "unexpected token".
func TestRejectedTokensNameTheirOwnMistake(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		// Grouping. The tokens lex but no rule accepts them, so the message
		// says the feature is absent rather than blaming the character.
		"recall (zebras)":            {"grouping is not supported"},
		"recall zebras (food)":       {"grouping is not supported"},
		"recall zebras topic:(food)": {"grouping is not supported"},
		"recall zebras )food(":       {"grouping is not supported"},
		// A second command is one instruction too many, not a stray word.
		"remember 'a' remember 'b'": {"one command per instruction"},
		// Among a recall's terms a command word is the word the caller forgot
		// to quote.
		"recall zebras recall food":  {`term "recall" is also a command: quote it ('recall')`},
		"recall zebras remember 'x'": {`term "remember" is also a command: quote it ('remember')`},
		// A prefix has no ':' form either, so its message names only the quote.
		// explain:<value> would itself be rejected.
		"recall ferry explain":       {`term "explain" is also a prefix: quote it ('explain') to search for the word`},
		"recall describe ferry":      {`term "describe" is also a prefix: quote it ('describe') to search for the word`},
		"recall ferry explain:ferry": {`"explain" is a prefix and starts no clause here: quote it ('explain') to search for the word`},
		"remember 'a' describe":      {`"describe" is a prefix and starts no clause here: quote it ('describe') to search for the word`},
		// A keyword with nothing after it can never finish a clause, so it is
		// the word the caller forgot to quote.
		"recall zebras top":   {"write top:<value> if a filter was meant, or quote it ('top')"},
		"recall zebras since": {"write since:<value> if a filter was meant, or quote it ('since')"},
		"remember 'a' topic":  {`"topic" is a keyword and starts no clause here: write topic:<value> if a clause was meant, or quote it ('topic') to search for the word`},
		// Casing still matters where a clause could start.
		"recall zebras Depth 2": {"lower case"},
		// A modifier is a recall clause. The message names the command it was
		// given to and the clauses a remember takes. It does not tell the
		// caller to write since:<value>, which is what they wrote.
		"remember 'a fact' top:3":    {"top: is a recall clause: a remember takes only topic:, entity: and vec:"},
		"remember 'a fact' since:7d": {"since: is a recall clause"},
		"remember 'a fact' depth:1":  {"depth: is a recall clause"},
		// An unclosed quote is reported as one, not as whatever it swallowed.
		"recall 'unclosed phrase": {"unterminated quoted phrase"},
		// In a clause value too. There the swallowed text would otherwise read
		// as the value, until:'2026-01-15 as until:2026-01-15.
		"recall x since:'7d":         {"unterminated quoted phrase"},
		"recall x until:'2026-01-15": {"unterminated quoted phrase"},
		"recall x top:'5":            {"unterminated quoted phrase"},
		"recall x depth:'2":          {"unterminated quoted phrase"},
		// A NUL outside a phrase is rejected, not read as the end of the query
		// with everything after it dropped.
		"recall zebras\x00food": {"NUL character is only allowed inside a quoted phrase"},
		// Outside a phrase a word is letters and digits only. Any other
		// character is rejected for itself. Absorbed into a word, "ferry;"
		// would be a term, and the next recall a search for the word "recall".
		"recall ferry; recall bridge":          {`";" is only allowed inside a quoted phrase`},
		"recall e-mail":                        {`"-" is only allowed inside a quoted phrase`},
		"recall zebras topic:machine-learning": {`"-" is only allowed inside a quoted phrase`},
		"recall zebras vec:$my_vec":            {`"_" is only allowed inside a quoted phrase`},
		"recall zebras since:2026-01-15":       {"a quoted date like '2026-01-15'"},
		// A newline in a value slot is a second instruction starting early.
		"recall zebras topic:\nfood": {"one command per instruction"},
		// A second '@' is named as a second selector.
		"recall@3@5 zebras": {"one selector"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a parse error", query)
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
// was meant") would send the caller from one rejection to the next. No clause
// is spelled with a command. Whether the word is the first term or a later one,
// the message says only what works.
func TestCommandWordsAreNeverOfferedAsClauses(t *testing.T) {
	cases := map[string]struct {
		word string
	}{
		"recall recall":          {"recall"},
		"recall forget":          {"forget"},
		"recall zebras recall":   {"recall"},
		"recall zebras remember": {"remember"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](query, nil)
			got := fmt.Sprint(err, warns)
			if err == nil && len(warns) == 0 {
				t.Fatalf("Parse(%q) was silent, want a warning or an error naming the command word", query)
			}
			if !strings.Contains(got, "quote it ('"+tc.word+"')") {
				t.Errorf("%s does not tell the caller to quote %q", got, tc.word)
			}
			if strings.Contains(got, tc.word+":<value>") {
				t.Errorf("%s offers %s:<value>, which is itself an error", got, tc.word)
			}
		})
	}
}

// TestDanglingKeywordAfterAClauseIsAWordToQuote pins the repair for a keyword
// that ends a recall after its first clause. Past the terms a keyword can only
// start a clause, and with nothing after it none can be finished. So it is
// either a clause whose ':' went missing or a word the caller forgot to quote.
// The message offers both.
func TestDanglingKeywordAfterAClauseIsAWordToQuote(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"recall x topic:y top":   {`"top" is a keyword and starts no clause here: write top:<value> if a clause was meant, or quote it ('top') to search for the word`},
		"recall x topic:y since": {`"since" is a keyword and starts no clause here: write since:<value> if a clause was meant, or quote it ('since') to search for the word`},
		"recall x vec:$v depth":  {`"depth" is a keyword and starts no clause here: write depth:<value> if a clause was meant, or quote it ('depth') to search for the word`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a dangling-keyword error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestDanglingCommandWordStartsASecondCommand pins the message for a command
// word that ends a query after its fact or its first clause. It names the
// second command and the quote, and nothing else. A message offering
// recall:<value> would send the caller from one rejection to the next, because
// that is itself an error.
func TestDanglingCommandWordStartsASecondCommand(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"remember 'a' recall":     {`"recall" starts a second command: one command per instruction — quote it ('recall') to search for the word`},
		"remember 'a' remember":   {`"remember" starts a second command: one command per instruction — quote it ('remember') to search for the word`},
		"recall x topic:y recall": {`"recall" starts a second command: one command per instruction — quote it ('recall') to search for the word`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a second-command error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestBareModifierOnARememberIsRejected pins that since, until, top and depth
// without their ':' after a remember's fact are rejected by name, not as an
// unexpected token. A remember has no terms, so the word cannot be data there.
// It is a clause missing its ':', or a word that belongs inside quotes.
func TestBareModifierOnARememberIsRejected(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"remember 'a' since 7d": {`"since" is a keyword and starts no clause here: write since:<value> if a clause was meant, or quote it ('since') to search for the word`},
		"remember 'a' until 1d": {`"until" is a keyword and starts no clause here: write until:<value> if a clause was meant, or quote it ('until') to search for the word`},
		"remember 'a' top 3":    {`"top" is a keyword and starts no clause here: write top:<value> if a clause was meant, or quote it ('top') to search for the word`},
		"remember 'a' depth 2":  {`"depth" is a keyword and starts no clause here: write depth:<value> if a clause was meant, or quote it ('depth') to search for the word`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a keyword error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestVecWithoutColonIsRejected pins that vec followed by anything but ':' is
// rejected with the clause repair, on both commands. Among a recall's terms a
// bare vec is a keyword term. So these cases put it where only a clause can
// start: after a remember's fact, or after a recall's first clause.
func TestVecWithoutColonIsRejected(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"remember 'a' vec$:v":     {`"vec" is missing its ':': write vec:$<name>`},
		"recall x topic:y vec$:v": {`"vec" is missing its ':': write vec:$<name>`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-separator error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestBareVecRepairNamesTheParameterForm pins the clause repair offered for a
// vec written without its ':'. The vec clause takes a parameter reference, so
// the repair is vec:$<name> wherever the word stands: among the terms,
// dangling, before a value, or mis-cased. Spelling back the next word (vec:3)
// or the generic vec:<value> would only lead to "expected param field
// operator $", one rejection after another.
func TestBareVecRepairNamesTheParameterForm(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"recall x vec 3":         {`term "vec" is also a keyword: write vec:$<name> if a filter was meant, or quote it ('vec') to search the word`},
		"recall x vec$:v":        {`term "vec" is also a keyword: write vec:$<name> if a filter was meant, or quote it ('vec') to search the word`},
		"recall x topic:y vec":   {`"vec" is a keyword and starts no clause here: write vec:$<name> if a clause was meant, or quote it ('vec') to search for the word`},
		"remember 'a' vec":       {`"vec" is a keyword and starts no clause here: write vec:$<name> if a clause was meant, or quote it ('vec') to search for the word`},
		"recall x topic:y vec q": {`"vec" is missing its ':': write vec:$<name>`},
		"remember 'a' vec $q":    {`"vec" is missing its ':': write vec:$<name>`},
		"recall x topic:y Vec":   {`mis-cased keyword "Vec": keywords are lower case — write vec:$<name> if a clause was meant, or quote it ('Vec') to search for the word`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a keyword error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestRememberNeedsASpaceAfterTheCommand pins that a remember's command, with
// or without its graph selector, is followed by a space before the fact. A
// remember with nothing after it says what is missing, rather than reporting a
// phrase that was never started.
func TestRememberNeedsASpaceAfterTheCommand(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"remember":         {"expected a space after the command, found end of input"},
		"remember@2":       {"expected a space after the command, found end of input"},
		"remember'a fact'": {`expected a space after the command, found "a fact"`},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-space error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestNulWhereTheFactShouldStartIsRejected pins that a NUL in the fact's place
// is rejected for itself, as it is everywhere else outside quotes. It is not
// reported as a missing phrase. The caller needs to know the character is the
// problem, not that the fact is absent.
func TestNulWhereTheFactShouldStartIsRejected(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"remember \x00'a'": {"a NUL character is only allowed inside a quoted phrase"},
		"remember@1 \x00":  {"a NUL character is only allowed inside a quoted phrase"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a NUL error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestNewlineEndsTheInstruction pins that a newline separates instructions,
// rather than blending into the whitespace around it. So a second line cannot
// silently join the first: "recall ferry\nbridge" is not a two-term recall.
// Trailing blank lines are not a second instruction and must still parse.
// Otherwise every client that ends its payload with a newline would break.
func TestNewlineEndsTheInstruction(t *testing.T) {
	accepted := map[string]struct{}{
		"recall zebras\n":        {},
		"recall zebras\n\n":      {},
		"recall zebras \n  \n\t": {},
		"remember 'a fact'\n":    {},
	}

	for q := range accepted {
		t.Run("accepted:"+strconv.Quote(q), func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q, nil); err != nil {
				t.Errorf("Parse(%q) = %v, want a trailing newline to be ignored", q, err)
			}
		})
	}

	rejected := map[string]struct{}{
		"recall zebras\nfood":        {},
		"recall zebras\nrecall food": {},
		"remember 'a'\nremember 'b'": {},
	}

	for q := range rejected {
		t.Run("rejected:"+strconv.Quote(q), func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a second-instruction error", q)
			}
			if !strings.Contains(err.Error(), "one command per instruction") {
				t.Errorf("error %q does not say one command per instruction", err)
			}
		})
	}
}

// TestIntegerValueTooLargeIsReportedAsOutOfRange pins that a number too large
// to hold is reported apart from text that is not a number at all. An agent
// told only "invalid" retries with another huge number. One told the value is
// out of range knows to shrink it. Both messages still name the clause that
// rejected it.
func TestIntegerValueTooLargeIsReportedAsOutOfRange(t *testing.T) {
	cases := map[string]struct {
		want []string
	}{
		"recall zebras depth:99999999999999999999": {[]string{"invalid depth value", "out of range"}},
		"recall zebras top:99999999999999999999":   {[]string{"invalid top value", "out of range"}},
		// Not a number at all: named, but not as a range problem.
		"recall zebras depth:abc": {[]string{"expected a non-negative whole number"}},
		"recall zebras top:top":   {[]string{"invalid top value"}},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a value error", query)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}

	if _, _, err := parser.Parse[uint64, float32]("recall zebras depth:abc", nil); err == nil ||
		strings.Contains(err.Error(), "out of range") {
		t.Errorf("depth:abc = %v, want it reported as not-a-number rather than out of range", err)
	}
}

// TestMiscasedClauseWarns pins the mis-cased keyword warning. The query ran,
// and a keyword in it was not lower case.
//
// The colon leaves a mis-cased clause one reading, so it is not an error. An
// error there would report the casing instead of the duplicate it may be hiding
// (see TestMiscasedKeywordBeforeColonIsTheClause). But the language is lower
// case, and accepting `Depth:2` in silence teaches the caller the opposite. So
// the clause runs, and the response says which clause it ran as.
//
// Anchor values are excluded. `entity:Top` is data, folded to the same anchor
// as `entity:top`. An agent never has to remember how it capitalised something.
func TestMiscasedClauseWarns(t *testing.T) {
	cases := map[string]struct {
		query string
		warns bool
	}{
		"mis-cased modifier":                       {"recall zebras TOP:3", true},
		"mis-cased anchor key":                     {"recall zebras Topic:food", true},
		"mis-cased time bound":                     {"recall zebras Since:7d", true},
		"mis-cased on a write":                     {"remember 'a fact' Entity:bob", true},
		"one warning per mis-cased clause":         {"recall zebras Topic:food Depth:2", true},
		"lower case is silent":                     {"recall zebras top:3 topic:food", false},
		"an anchor value is data, not syntax":      {"recall zebras entity:Top", false},
		"a mis-cased value on a write is data too": {"remember 'a fact' topic:Food", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](tc.query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			if got := len(warns) > 0; got != tc.warns {
				t.Fatalf("Parse(%q) warnings = %v, want warned=%v", tc.query, warns, tc.warns)
			}
		})
	}

	// Every mis-cased clause is named. A query with two gets two warnings, so
	// the caller can fix both in one pass.
	_, warns, err := parser.Parse[uint64, float32]("recall zebras Topic:food Depth:2", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 2 {
		t.Fatalf("got %d warnings %v, want one per mis-cased clause", len(warns), warns)
	}
	got := warns[0].String() + " " + warns[1].String()
	for _, want := range []string{`"Topic"`, "topic clause", `"Depth"`, "depth clause", "lower case"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings %q do not mention %q", got, want)
		}
	}
}

// TestDepthWithoutAnchorWarns pins the depth warning. The graph is entered only
// through an anchor the recall names. So a depth above 0 on a recall naming no
// topic or entity has no effect. The query runs, and the response says so.
// Nothing warns in these cases:
//   - The recall names an anchor.
//   - The depth is 0, which asks for nothing the recall cannot do.
//   - The clause is omitted. The depth is then the operator's default, not the
//     caller's choice.
func TestDepthWithoutAnchorWarns(t *testing.T) {
	cases := map[string]struct {
		query string
		warns bool
	}{
		"depth on a term-only recall":         {"recall ferry depth:2", true},
		"depth on a vector-only recall":       {"recall vec:$v depth:1", true},
		"depth beside an anchor alone":        {"recall topic:harbour depth:2", false},
		"depth beside a term and an anchor":   {"recall ferry topic:harbour depth:2", false},
		"depth beside a vector and an anchor": {"recall vec:$v entity:acme depth:1", false},
		"the floor never warns":               {"recall ferry depth:0", false},
		"no depth clause never warns":         {"recall ferry", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](tc.query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.query, err)
			}
			if got := len(warns) > 0; got != tc.warns {
				t.Fatalf("Parse(%q) warnings = %v, want warned=%v", tc.query, warns, tc.warns)
			}
		})
	}
}

// TestDepthWithoutAnchorWarningIsActionable pins the message. It names the
// clause that had no effect and what the graph needs. Like every other warning,
// it is positioned at the clause. So a caller can fix the query from the
// response alone, by adding an anchor or dropping the clause.
func TestDepthWithoutAnchorWarningIsActionable(t *testing.T) {
	q := "recall ferry depth:2"
	_, warns, err := parser.Parse[uint64, float32](q, nil)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", q, err)
	}
	if len(warns) != 1 {
		t.Fatalf("Parse(%q) warnings = %v, want exactly one", q, warns)
	}

	got := warns[0].String()
	for _, want := range []string{
		"depth:2 has no effect",   // the clause that did nothing
		"topic:/entity:",          // what opens the graph
		"names none",              // why this recall could not
		"parse warning at column", // positioned like an error
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warning %q does not contain %q", got, want)
		}
	}
}

// TestDepthZeroBesideAnAnchorWarns pins the other depth warning. An explicit
// depth:0 on a recall naming an anchor beside a term or vector asks for a lane
// in which the anchor means nothing, so the recall runs at depth 1 and the
// response says so, at the clause. Nothing warns in these cases:
//   - The clause is omitted. The operator's default is raised in silence,
//     since the caller chose nothing.
//   - The anchors stand alone. They seed the search at any depth.
//   - The recall names no anchor. depth:0 asks for nothing it cannot do.
func TestDepthZeroBesideAnAnchorWarns(t *testing.T) {
	cases := map[string]bool{
		"recall ferry topic:harbour depth:0": true,
		"recall vec:$v entity:acme depth:0":  true,
		"recall ferry topic:harbour":         false,
		"recall ferry topic:harbour depth:1": false,
		"recall topic:harbour depth:0":       false,
		"recall ferry depth:0":               false,
	}

	for query, want := range cases {
		t.Run(query, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
			}
			if got := len(warns) > 0; got != want {
				t.Fatalf("Parse(%q) warnings = %v, want warned=%v", query, warns, want)
			}
			if !want {
				return
			}
			got := warns[0].String()
			for _, part := range []string{
				"depth:0 ran as depth:1",  // which depth ran
				"topic:/entity: anchor",   // what needs the graph
				"parse warning at column", // positioned like an error
			} {
				if !strings.Contains(got, part) {
					t.Errorf("warning %q does not contain %q", got, part)
				}
			}
		})
	}
}

// TestSelectorWithSpace pins that whitespace between a command and its graph
// selector is a query error that says so. The selector is glued to the verb. A
// message that only blames the "@" leaves the caller to guess the fix.
func TestSelectorWithSpace(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"remember @2 signal":    {"no space allowed between remember and @"},
		"recall @3 'car park'":  {"no space allowed between recall and @"},
		"recall  @5 'names of'": {"no space allowed between recall and @"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a selector-spacing error", query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestWhitespaceSeparatesWords pins that any run of space, tab or carriage
// return separates the words of a query, wherever it falls. A NUL inside quotes
// is data like any other character.
func TestWhitespaceSeparatesWords(t *testing.T) {
	cases := map[string]struct {
		terms []string
	}{
		"recall  zebras":          {[]string{"zebras"}},
		"recall\tzebras":          {[]string{"zebras"}},
		"recall zebras ":          {[]string{"zebras"}},
		"recall 'zebras\x00food'": {[]string{"zebras\x00food"}},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])
			if diff := cmp.Diff(tc.terms, rc.Terms()); diff != "" {
				t.Errorf("Terms() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNoSpaceInsideACommandOrClause pins that the parts of a command or clause
// are glued: the selector to its verb, the value to its ':' and a parameter
// name to its '$'. A space inside one is rejected with a message naming where
// it is not allowed. A bare "unexpected" would leave the caller to guess the
// fix. want is empty for a query that must parse.
func TestNoSpaceInsideACommandOrClause(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"remember@2 'x'":             {""},
		"remember @2 'x'":            {"no space allowed between remember and @"},
		"remember@ 2 'x'":            {"no space allowed between @ and the selector"},
		"recall@3@5 zebras":          {"one selector"},
		"recall zebras topic:food":   {""},
		"recall zebras topic :food":  {"no space allowed before :"},
		"recall zebras topic: food":  {"no space allowed after :"},
		"recall zebras : topic:food": {"stray"},
		"recall zebras vec:$v":       {""},
		"recall zebras vec: $v":      {"no space allowed after :"},
		"recall zebras vec:$ v":      {"no space allowed after $"},
		"recall zebras since:7d until:'2026-01-15' depth:2 top:5": {""},
		"recall zebras since :7d":                                 {"no space allowed before :"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Parse(%q) = %v, want it to parse", query, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want an error containing %q", query, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestReservedWordAsATermNamesBothFixes pins that a reserved word written as a
// bare term is an error, in every position and any casing. The message offers
// both readings. One is the clause, with the next word as its value when there
// is one. The other is the quote that searches the word. The error points at
// the keyword itself, the word the caller has to change.
func TestReservedWordAsATermNamesBothFixes(t *testing.T) {
	cases := map[string]struct {
		column int
		want   []string
	}{
		"recall top":                       {10, []string{"write top:<value>", "quote it ('top')"}},
		"recall Top":                       {10, []string{"write top:<value>", "quote it ('Top')"}},
		"recall zebras top":                {17, []string{"write top:<value>", "quote it ('top')"}},
		"recall zebras topic food":         {19, []string{"write topic:food", "quote it ('topic')"}},
		"recall since 7d":                  {12, []string{"write since:7d", "quote it ('since')"}},
		"recall zebras entity alice extra": {20, []string{"write entity:alice", "quote it ('entity')"}},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			var perr *parser.Error
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%q) = %v, want a positioned parse error", query, err)
			}
			if got := perr.Pos.Column; got != tc.column {
				t.Errorf("Parse(%q) error at column %d, want %d", query, got, tc.column)
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
// to. Quoted, a reserved word in any casing is an ordinary search term, and the
// query runs without a warning.
func TestQuotedReservedWordIsATerm(t *testing.T) {
	cases := map[string]struct {
		terms []string
	}{
		"recall 'top'":               {[]string{"top"}},
		"recall 'Top' 'since'":       {[]string{"top", "since"}},
		"recall zebras 'topic' food": {[]string{"zebras", "topic", "food"}},
		"recall 'since' 7d":          {[]string{"since", "7d"}},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			cmd, warns, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
			}
			if len(warns) != 0 {
				t.Errorf("Parse(%q) warnings = %v, want none", query, warns)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])
			if diff := cmp.Diff(tc.terms, rc.Terms()); diff != "" {
				t.Errorf("Terms() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestKeywordAfterAClauseIsAMissingColon pins the one reading a keyword has
// once a clause has started. Terms come first, so the keyword cannot be a word
// to quote. The message names only the clause the caller meant, with the next
// word as its value.
func TestKeywordAfterAClauseIsAMissingColon(t *testing.T) {
	cases := map[string]struct {
		want string
	}{
		"recall zebras since:7d topic food":               {"write topic:food"},
		"recall zebras top:5 depth 2":                     {"write depth:2"},
		"remember 'zebras eat grass' topic food entity:x": {"write topic:food"},
		"recall x topic:y top 5":                          {"write top:5"},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](query, nil)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want a missing-colon error", query)
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

// TestAnchorValueIsData pins that only a value can appear after a ':'. A
// reserved word, any casing, and a quoted phrase with spaces or an apostrophe
// all name an anchor, folded to lower case. None of them warns.
func TestAnchorValueIsData(t *testing.T) {
	cases := map[string]struct {
		topics   []string
		entities []string
	}{
		"recall x topic:top":            {[]string{"top"}, nil},
		"recall x entity:Top":           {nil, []string{"top"}},
		"recall x topic:'US elections'": {[]string{"us elections"}, nil},
		"recall x topic:'My Project'":   {[]string{"my project"}, nil},
		"recall x entity:'O''Brien'":    {nil, []string{"o'brien"}},
		"recall x topic:'since'":        {[]string{"since"}, nil},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			cmd, warns, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
			}
			if len(warns) != 0 {
				t.Errorf("Parse(%q) warnings = %v, want none", query, warns)
			}
			rc := cmd.(*parser.RecallCommandNode[uint64, float32])
			if diff := cmp.Diff(tc.topics, rc.Topics()); diff != "" {
				t.Errorf("Topics() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.entities, rc.Entities()); diff != "" {
				t.Errorf("Entities() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestStopWordTermWarns pins the stop-word warning. Stored facts are cleaned of
// stop words at index time, so a bare stop word can never match. The query runs
// with one warning per such term, positioned at the term. A phrase or a vector
// beside it is a real seed, so it stays a warning.
func TestStopWordTermWarns(t *testing.T) {
	type warning struct {
		term   string
		column int
	}
	cases := map[string]struct {
		warns []warning
	}{
		"recall the parrot":           {[]warning{{"the", 10}}},
		"recall parrot and the zebra": {[]warning{{"and", 17}, {"the", 21}}},
		"recall the 'parrot'":         {[]warning{{"the", 10}}},
		"recall the vec:$v":           {[]warning{{"the", 10}}},
	}

	for query, tc := range cases {
		t.Run(query, func(t *testing.T) {
			_, got, err := parser.Parse[uint64, float32](query, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", query, err)
			}
			want := make([]parser.Warning, len(tc.warns))
			for i, w := range tc.warns {
				want[i] = parser.Warning{
					Msg: fmt.Sprintf("term %q is a stop word: stored facts never contain it, so it cannot match", w.term),
					Pos: lexer.Position{Column: w.column},
				}
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("Parse(%q) warnings mismatch (-want +got):\n%s", query, diff)
			}
		})
	}
}

// TestStopWordOnlyRecallIsAnError pins that a recall whose every bare term is a
// stop word, with no phrase and no vector, is an error. An empty result would
// look like a miss. Anchors do not rescue it, because the query that was meant
// is the anchor alone.
func TestStopWordOnlyRecallIsAnError(t *testing.T) {
	queries := map[string]struct{}{
		"recall the":             {},
		"recall the and":         {},
		"recall the topic:birds": {},
	}

	for q := range queries {
		t.Run(q, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q, nil)
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

// TestUnambiguousQueriesRunSilently pins where no warning may fire:
//   - A clause written correctly.
//   - A depth beside an anchor it can act through.
//   - A phrase whose content is only stop words.
//   - A recall seeded by an anchor alone.
//
// A warning there is noise the caller learns to ignore.
func TestUnambiguousQueriesRunSilently(t *testing.T) {
	queries := map[string]struct{}{
		"recall x since:7d":                 {},
		"recall parrot topic:birds depth:2": {},
		"recall 'the parrot'":               {},
		"recall 'the'":                      {},
		"recall topic:birds":                {},
	}

	for q := range queries {
		t.Run(q, func(t *testing.T) {
			_, warns, err := parser.Parse[uint64, float32](q, nil)
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
// parses back to the same command. The reconstruction, terms, anchors and fact
// all stay the same. Every shape the tests from TestWhitespaceSeparatesWords on
// accept is here. So is a quoted form of each reserved word, since a bare one
// would not parse.
func TestStringRoundTripsToTheSameCommand(t *testing.T) {
	// command holds what a round trip must not change.
	type command struct {
		Value                   string
		Terms, Topics, Entities []string
	}

	queries := map[string]struct{}{
		"recall zebras":            {},
		"recall  zebras":           {},
		"recall\tzebras":           {},
		"recall zebras ":           {},
		"recall zebras \n  \n\t":   {},
		"recall 'zebras\x00food'":  {},
		"remember@2 'x'":           {},
		"recall zebras topic:food": {},
		"recall zebras vec:$v":     {},
		"recall zebras since:7d until:'2026-01-15' depth:2 top:5": {},
		"recall 'top'":                      {},
		"recall 'Top' 'since'":              {},
		"recall zebras 'topic' food":        {},
		"recall x topic:top":                {},
		"recall x entity:Top":               {},
		"recall x topic:'US elections'":     {},
		"recall x topic:'My Project'":       {},
		"recall x entity:'O''Brien'":        {},
		"recall x topic:'since'":            {},
		"recall x TOP:3":                    {},
		"recall x Topic:food":               {},
		"recall the parrot":                 {},
		"recall parrot and the zebra":       {},
		"recall 'the parrot'":               {},
		"recall 'the'":                      {},
		"recall the 'parrot'":               {},
		"recall the vec:$v":                 {},
		"recall topic:birds":                {},
		"recall 'since' 7d":                 {},
		"recall x since:7d":                 {},
		"recall parrot topic:birds depth:2": {},
		"recall parrot depth:2":             {},
		"recall 'top' topic:'US elections' since:'2026-01-15' top:5": {},
		"remember@2 'a fact' topic:'my project' entity:'O''Brien'":   {},
		"recall 'recall'":   {},
		"recall 'remember'": {},
		"recall 'forget'":   {},
		"recall 'update'":   {},
		"recall 'topic'":    {},
		"recall 'entity'":   {},
		"recall 'since'":    {},
		"recall 'until'":    {},
		"recall 'depth'":    {},
		"recall 'vec'":      {},
		"recall 'explain'":  {},
		"recall 'describe'": {},
	}

	for q := range queries {
		t.Run(q, func(t *testing.T) {
			cmd, _, err := parser.Parse[uint64, float32](q, nil)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", q, err)
			}
			again, _, err := parser.Parse[uint64, float32](cmd.String(), nil)
			if err != nil {
				t.Fatalf("Parse(String() = %q) = %v, want the reconstruction to parse", cmd.String(), err)
			}
			if got, want := again.String(), cmd.String(); got != want {
				t.Errorf("String() after a round trip = %q, want %q", got, want)
			}

			var got, want command
			switch c := cmd.(type) {
			case *parser.RecallCommandNode[uint64, float32]:
				a := again.(*parser.RecallCommandNode[uint64, float32])
				want = command{Terms: c.Terms(), Topics: c.Topics(), Entities: c.Entities()}
				got = command{Terms: a.Terms(), Topics: a.Topics(), Entities: a.Entities()}
			case *parser.RememberCommandNode[float32]:
				a := again.(*parser.RememberCommandNode[float32])
				want = command{Value: c.Value(), Topics: c.Topics(), Entities: c.Entities()}
				got = command{Value: a.Value(), Topics: a.Topics(), Entities: a.Entities()}
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("round trip changed the command: mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestTokenGluedToPhrase pins that a word glued to the closing quote of a
// remember's fact is a parse error.
func TestTokenGluedToPhrase(t *testing.T) {
	queries := map[string]struct{}{
		"remember 'a fact'x":           {},
		"remember 'a fact'zzz topic:x": {},
	}

	for q := range queries {
		t.Run(q, func(t *testing.T) {
			_, _, err := parser.Parse[uint64, float32](q, nil)
			if err == nil {
				t.Fatalf("Expected error: parse error at column 18: unexpected \"x\" but got: %q", err)
			}
		})
	}
}
