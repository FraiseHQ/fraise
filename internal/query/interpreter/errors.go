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

package interpreter

import (
	"github.com/RonsenbergVI/fraise/internal/query/lexer"
)

// Interpretor errors which are different than database errors, are returned by the evaluation
// and execution planning of a query.
type InterpretorError interface {
	error
	Position() lexer.Position
}

type NameError struct {
	Message string
	Pos     lexer.Position
}

func (e NameError) Position() lexer.Position {
	return e.Pos
}

type ValueError struct {
	Message string
	Pos     lexer.Position
}

func (e ValueError) Position() lexer.Position {
	return e.Pos
}

type TypeError struct {
	Message string
	Pos     lexer.Position
}

func (e TypeError) Position() lexer.Position {
	return e.Pos
}

type SyntaxError struct {
	Message string
	Pos     lexer.Position
}

func (e SyntaxError) Position() lexer.Position {
	return e.Pos
}
