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

import "errors"

var (
	// ErrEmptyIndex is returned by Search when the index holds no entries. A
	// search of a non-empty index that matches nothing returns no keys and no
	// error, so callers can tell the two situations apart.
	ErrEmptyIndex = errors.New("index: is empty")
	// ErrIndexNotFound is returned by Retrieve, Update and Delete when the key
	// is not indexed.
	ErrIndexNotFound = errors.New("index: not found")
	// ErrInvalidDimension is returned when a vector's dimensionality does not
	// match the index it is being used with.
	ErrInvalidDimension = errors.New("index: invalid vector dimension")
)
