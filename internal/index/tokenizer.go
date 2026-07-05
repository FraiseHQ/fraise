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

package index

import (
	"strings"
	"unicode"
)

// Tokenizer splits raw document text into the sequence of normalized terms that
// get indexed and queried. Implementations decide casing, stemming, stop-word
// removal, n-gramming, etc. The same Tokenizer must be used at index and query
// time so terms line up.
type Tokenizer interface {
	Tokenize(text string) []string
}

// SimpleTokenizer lowercases text and splits it on runs of characters that are
// neither letters nor digits. It performs no stemming or stop-word removal.
type SimpleTokenizer struct{}

// compile-time check that SimpleTokenizer is a Tokenizer.
var _ Tokenizer = SimpleTokenizer{}

// Tokenize returns the lowercased alphanumeric terms found in text.
func (SimpleTokenizer) Tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
