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
	// ErrEmptyIndex is returned by a search when nothing has been indexed yet.
	// It is kept distinct from ErrIndexNotFound ("indexed, but no match") so
	// callers can tell the two situations apart.
	ErrEmptyIndex = errors.New("index: is empty")
	// ErrIndexNotFound is returned by a lookup (e.g. Retrieve) when the key or
	// point is not present in a non-empty index.
	ErrIndexNotFound = errors.New("index: not found")
	// ErrInvalidDimension is returned when a vector's dimensionality does not
	// match the index it is being used with.
	ErrInvalidDimension = errors.New("index: invalid vector dimension")
	// ErrFailedToCreateIndex is returned when an index cannot be constructed.
	ErrFailedToCreateIndex = errors.New("index: failed to create index")
	// ErrFailedToLoadIndex is returned when an existing index cannot be loaded.
	ErrFailedToLoadIndex = errors.New("index: failed to load index")
)
