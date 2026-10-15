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

// Package parser turns one FQL instruction into a typed command node.
//
// [Parse] reads the lexer's tokens through a two-token window. It returns a
// [CommandNode], either a [RecallCommandNode] or a [RememberCommandNode]. Their
// clauses are [FieldNode] values. Nodes keep their source tokens, so String
// prints a query that parses back to the same node.
//
// This package owns only the grammar. The query layer reads commands through
// their accessors and enforces the configured limits. Vectors never appear in
// the query text. A vec:$name clause looks its vector up in the params map
// given to Parse.
//
// A rejected query gets an [*Error]. It carries the column it blames and a
// message naming the repair. Callers are agents, and they can only make a
// repair the message spells out. A query that runs can still have a flaw, such
// as a stop word, a depth with no anchor, or a mis-cased keyword. Parse then
// returns a [Warning] beside the command. Warnings are never stored on the
// command, because the plan cache hands the same query objects to other
// requests.
package parser
