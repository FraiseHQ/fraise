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

package nlp

import (
	"strings"
	"unicode"

	"github.com/blevesearch/snowballstem"
	"github.com/blevesearch/snowballstem/english"
)

// Tokenizer splits raw document text into the sequence of normalized terms that
// get indexed and queried. Implementations decide casing, stemming, stop-word
// removal, n-gramming, etc. The same Tokenizer must be used at index and query
// time so terms line up — the index owns one instance and runs it on both
// sides, which is what makes a stemmed posting findable by a stemmed query.
// Like the Relevance model, a tokenizer is installed before the first insert
// and never swapped mid-corpus: postings tokenized under the old scheme would
// be unreachable under the new one.
type Tokenizer interface {
	Tokenize(text string) []string
}

// Words splits text into the spellings of its terms: the maximal runs of term
// characters, in order, with their case and everything else intact. It is the
// one definition of where a term ends, shared by the tokenizers and by
// stop-word cleaning, so the words cleaning removes are exactly the terms the
// index would otherwise have carried — a second copy of the boundary would let
// the two drift and lose terms between them. A term character is a letter, a
// digit, or a symbol in Unicode category So (Symbol, other), the class that
// holds emoji, check marks and marks such as ©. Keeping So is what makes a
// fact whose salient content is an emoji findable by it: dropped at the
// boundary, such a fact indexes under no term at all and is unreachable by
// text search while looking stored. Every other rune delimits — whitespace,
// punctuation, and the math, currency and modifier symbol classes ("+", "$",
// "^"), which stay delimiters so operators and prices split the way they
// always have. Joiners and variation selectors delimit too, so a multi-rune
// emoji sequence indexes as its So components.
func Words(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !isTermRune(r)
	})
}

func isTermRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.So, r)
}

// SimpleTokenizer lowercases text and splits it into its Words. It performs no
// stemming or stop-word removal.
type SimpleTokenizer struct{}

// compile-time check that SimpleTokenizer is a Tokenizer.
var _ Tokenizer = SimpleTokenizer{}

// Tokenize returns the lowercased terms found in text.
func (SimpleTokenizer) Tokenize(text string) []string {
	return Words(strings.ToLower(text))
}

// StemmingTokenizer is SimpleTokenizer's split with Snowball (Porter2)
// English stemming over each term: "running", "runs" and "run" all index and
// query as "run", so morphological variants of a word find each other —
// natural-language recall keywords rarely arrive in the exact inflection the
// fact was written with. Terms the stemmer does not recognise (numbers,
// symbols, non-English words, CJK text) pass through unchanged: stemming
// only ever rewrites, never drops, so every term SimpleTokenizer would index
// still exists under some spelling.
type StemmingTokenizer struct{}

// compile-time check that StemmingTokenizer is a Tokenizer.
var _ Tokenizer = StemmingTokenizer{}

// Tokenize returns the lowercased terms found in text, each reduced to its
// Snowball English stem.
func (StemmingTokenizer) Tokenize(text string) []string {
	tokens := SimpleTokenizer{}.Tokenize(text)
	for i, token := range tokens {
		env := snowballstem.NewEnv(token)
		english.Stem(env)
		tokens[i] = env.Current()
	}
	return tokens
}
