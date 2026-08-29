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

package stopwords

import (
	"strings"
	"unicode"

	"golang.org/x/text/language"
)

// sets maps a base language to its lowercase stop-word set. Keying on the
// base rather than the full tag is what lets regional variants ("en-US",
// "en-GB") share one list: stop words are a property of the language, not
// the locale.
var sets = map[language.Base]map[string]struct{}{
	language.MustParseBase("en"): toSet(English),
}

func toSet(words []string) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		set[word] = struct{}{}
	}
	return set
}

// CleanContent removes tag's stop words from content and returns the words
// that survive, in their original spelling and order, joined by single
// spaces. Matching is case-insensitive, and words are delimited the way the
// tokenizer delimits them — runs of characters that are neither letters nor
// digits — so punctuation never shields a stop word and the terms removed
// here are exactly the terms the index would otherwise have carried.
// Cleaning only removes, never rewrites: every surviving word still reaches
// the index as written. A tag that does not state its language (Base infers
// "en" for language.Und, at low confidence) or states one with no stop-word
// list gets content back verbatim — a stop word is only noise in the language
// that owns it, so removal on a guess would drop meaning, and dropping
// nothing is the only safe answer.
func CleanContent(content string, tag language.Tag) string {
	base, conf := tag.Base()
	if conf < language.Exact {
		return content
	}
	set, ok := sets[base]
	if !ok {
		return content
	}
	words := strings.FieldsFunc(content, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	kept := words[:0]
	for _, word := range words {
		if _, stop := set[strings.ToLower(word)]; !stop {
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
}
