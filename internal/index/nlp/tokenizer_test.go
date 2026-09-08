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

package nlp_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/FraiseHQ/fraise/internal/index/nlp"
)

// TestWordsKeepsSpellingAndSymbolTerms pins the term boundary the tokenizers
// and stop-word cleaning share: a term is a run of letters, digits and So
// symbols, returned exactly as written. Emoji and marks such as ✓ and © are
// terms in their own right — a fact whose salient content is an emoji used to
// tokenize to nothing and index under no term, so it looked stored but could
// never be found by text. Punctuation and the other symbol classes delimit as
// before, so "c++" still splits to "c" and "$5" to "5", and joiners and
// modifiers delimit, so a multi-rune emoji sequence yields its So symbols.
func TestWordsKeepsSpellingAndSymbolTerms(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"emoji are terms", "🍊 🍋 🍌", []string{"🍊", "🍋", "🍌"}},
		{"symbols keep their spelling alongside words", "Deploy ✓ Acme©", []string{"Deploy", "✓", "Acme©"}},
		{"punctuation delimits", "the, graph; is: temporal!", []string{"the", "graph", "is", "temporal"}},
		{"math, currency and modifier symbols delimit", "c++ costs $5 and 2^3", []string{"c", "costs", "5", "and", "2", "3"}},
		{"joiners and modifiers split a sequence into its symbols", "👨\u200d👩\u200d👧 👍\U0001F3FD ❤\ufe0f", []string{"👨", "👩", "👧", "👍", "❤"}},
		{"delimiters alone yield no terms", " :) -- ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nlp.Words(tt.text); !slices.Equal(got, tt.want) {
				t.Errorf("Words(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// TestStemmingTokenizerReducesInflections pins the stemmer's contract: the
// split and casing are SimpleTokenizer's, and every English inflection lands
// on its Snowball stem, so "running", "runs" and "RUN" become one term.
func TestStemmingTokenizerReducesInflections(t *testing.T) {
	got := nlp.StemmingTokenizer{}.Tokenize("The runner was RUNNING; she runs easily!")
	want := []string{"the", "runner", "was", "run", "she", "run", "easili"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize = %v, want %v", got, want)
	}
}

// TestStemmingTokenizerPassesUnstemmableTermsThrough pins the safety half:
// numbers, non-English words, CJK text and symbols pass through the stemmer
// intact — stemming rewrites, it never drops, so every term the plain split
// would index still exists under some spelling.
func TestStemmingTokenizerPassesUnstemmableTermsThrough(t *testing.T) {
	got := nlp.StemmingTokenizer{}.Tokenize("v2 café 東京 1234 🍊")
	want := []string{"v2", "café", "東京", "1234", "🍊"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize = %v, want %v", got, want)
	}
}
