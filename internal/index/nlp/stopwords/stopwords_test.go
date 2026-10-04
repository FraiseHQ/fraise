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

package stopwords_test

import (
	"testing"

	"golang.org/x/text/language"

	"github.com/FraiseHQ/fraise/internal/index/nlp/stopwords"
)

// TestCleanContentRemovesEnglishStopWords pins the cleaning contract:
// matching is case-insensitive, surviving words keep their spelling and
// order, and words are delimited the tokenizer's way — punctuation splits, a
// symbol such as an emoji is a word — so punctuation never shields a stop
// word from removal and a symbol survives it as a term.
func TestCleanContentRemovesEnglishStopWords(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"stop words drop, the rest keeps its order", "facts live in the graph", "facts live graph"},
		{"matching ignores case, survivors keep theirs", "The Graph IS Temporal", "Graph Temporal"},
		{"punctuation does not shield a stop word", "the, graph; is: temporal!", "graph temporal"},
		{"hyphens delimit words", "state-of-the-art recall", "state art recall"},
		{"symbols are words and survive", "the 🍊 is ripe ✓", "🍊 ripe ✓"},
		{"only stop words leave nothing", "it is what it was", ""},
		{"a negation survives its stop words", "to be or not to be", "not"},
		{"empty content stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stopwords.CleanContent(tt.content, language.English); got != tt.want {
				t.Errorf("CleanContent(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

// TestCleanContentKeepsContentWordsAndNegations pins what the English list
// must never hold. A stop word is a word no stored fact can be found by, so a
// content word on the list (scikit-learn's English list holds "bill", "fire",
// "system" and "name") makes every fact about it unretrievable, and a
// negation on it makes "Ana is not vegetarian" index the same as "Ana is
// vegetarian". The apostrophe pieces stay terms too: the negative heads are
// negations, and the clitic tails are also letters facts name.
func TestCleanContentKeepsContentWordsAndNegations(t *testing.T) {
	for _, word := range []string{
		"bill", "fire", "system", "name", "names", "show", "call", "back", "full", "interest",
		"no", "nor", "not", "never", "nothing", "nobody", "none", "cannot", "without",
		"isn", "don", "won", "t", "s", "d", "ll", "m", "re", "ve",
	} {
		if got := stopwords.CleanContent(word, language.English); got != word {
			t.Errorf("CleanContent(%q) = %q, want it kept: it is not a stop word", word, got)
		}
	}
}

// TestCleanContentDispatchesOnBaseLanguage pins the language contract: every
// English variant shares the one English list, and a language without a list
// returns content verbatim — with no list to consult, dropping nothing is
// the only safe behaviour.
func TestCleanContentDispatchesOnBaseLanguage(t *testing.T) {
	tests := []struct {
		name    string
		tag     language.Tag
		content string
		want    string
	}{
		{"regional variant uses the English list", language.AmericanEnglish, "the graph", "graph"},
		{"unsupported language is untouched", language.French, "the graph", "the graph"},
		{"undetermined language is untouched", language.Und, "the graph", "the graph"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stopwords.CleanContent(tt.content, tt.tag); got != tt.want {
				t.Errorf("CleanContent(%q, %v) = %q, want %q", tt.content, tt.tag, got, tt.want)
			}
		})
	}
}
