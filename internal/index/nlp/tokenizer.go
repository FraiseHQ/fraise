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

// Tokenizer splits raw text into the normalized terms that are indexed and
// queried. Implementations decide casing, stemming, stop-word removal,
// n-gramming and so on. Index and query time must use the same Tokenizer so
// their terms line up. The index owns one instance and runs it on both sides,
// which makes a stemmed posting findable by a stemmed query. A tokenizer is
// installed before the first insert and never swapped mid-corpus. Postings
// tokenized under the old scheme would be unreachable under the new one.
type Tokenizer interface {
	Tokenize(text string) []string
}

// Words splits text into the spellings of its terms: the maximal runs of term
// characters, in order and exactly as written. It is the one definition of
// where a term ends. The tokenizers and stop-word cleaning share it, so the
// words cleaning removes are exactly the terms the index would otherwise
// carry.
//
// A term character is a letter, a digit, or a symbol in Unicode category So
// (Symbol, other). That category holds emoji, check marks and signs such as
// ©. Without it, a fact whose main content is an emoji would index under no
// term, and text search could not reach it. Every other rune delimits:
// whitespace, punctuation, and the math, currency and modifier symbol classes
// ("+", "$", "^"). Operators and prices split apart as a result. Joiners and
// variation selectors delimit too, so a multi-rune emoji sequence indexes as
// its So components.
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

// Tokenize returns the lowercased terms found in text.
func (SimpleTokenizer) Tokenize(text string) []string {
	return Words(strings.ToLower(text))
}

// StemmingTokenizer splits like SimpleTokenizer, then applies Snowball
// (Porter2) English stemming to each term. "running", "runs" and "run" all
// index and query as "run", so variants of a word find each other. Recall
// keywords rarely arrive in the inflection the fact was written with. A term
// with no English suffix to strip, such as a number, a symbol or CJK text,
// passes through unchanged. Stemming only rewrites and never drops, so every
// term SimpleTokenizer would index still exists under some spelling.
type StemmingTokenizer struct{}

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
