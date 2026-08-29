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
// order, and any run of non-alphanumeric characters delimits a word — the
// tokenizer's split — so punctuation never shields a stop word from removal.
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
		{"only stop words leave nothing", "to be or not to be", ""},
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
