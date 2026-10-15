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

// Package lexer splits an FQL query into the tokens the parser reads.
//
// [Lexer.Next] returns one [Token] per call, typed by a [TokenType], and an
// EOL token once the input is exhausted. A bare word is a keyword only in its
// exact lower-case spelling ([KeywordsMap]). Immediately before a ':', casing
// is forgiven. A single-quoted phrase is scanned verbatim, with a doubled
// quote as an escaped one, so a fact is stored exactly as written.
//
// Every character outside a phrase becomes part of some token. A symbol that
// is not syntax is its own SPECIAL or NUL token. It is not absorbed into a
// word, and it does not end the input. So the parser can reject it by name
// instead of silently searching for it or dropping it. Positions count runes,
// not bytes, so an error points at the right column in non-ASCII text.
package lexer
