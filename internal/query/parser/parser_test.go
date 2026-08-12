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
		"recall",           // a recall must carry at least one term
		"recall topic:job", // fields may only follow a term
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			if _, _, err := parser.Parse[uint64, float32](q); err == nil {
				t.Errorf("Parse(%q) = nil error, want an error", q)
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
