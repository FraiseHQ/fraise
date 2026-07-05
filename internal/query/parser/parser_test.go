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
	"testing"

	"github.com/RonsenbergVI/fraise/internal/query/parser"
)

func TestRememberParser(t *testing.T) {
	q := "remember@1 'anne loves the color orange' topic:color topic:preference entity:anne vec:$v"

	qo, _, err := parser.Parse[float32](q)

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
			qo, _, err := parser.Parse[float32](q)
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
			if _, _, err := parser.Parse[float32](q); err == nil {
				t.Errorf("Parse(%q) = nil error, want an error", q)
			}
		})
	}
}
