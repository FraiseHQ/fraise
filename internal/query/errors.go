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

package query

import "errors"

var (
	// ErrParsingFailed is returned when a raw query string cannot be parsed into
	// an executable query. It wraps the underlying *parser.Error (with its
	// position), so callers can errors.As it out at the boundary.
	ErrParsingFailed = errors.New("query: parsing error")
	// ErrMissingParameter is returned when a query references a placeholder
	// (e.g. vec:$v) that has no matching entry in the supplied parameters.
	ErrMissingParameter = errors.New("query: missing parameter")
	// ErrLimitExceeded is returned when a query asks for more than a configured
	// ceiling allows (top:, depth:, or the length of a bound vector). It is a
	// client error: the request is rejected rather than clamped, so the caller
	// learns their bound was too high.
	ErrLimitExceeded = errors.New("query: request limit exceeded")
)
