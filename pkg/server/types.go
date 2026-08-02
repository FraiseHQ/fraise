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

package server

// Request is a placeholder for a generic incoming request payload.
type Request struct {
}

// ErrorResponse is the JSON error body returned to clients: a single
// human-readable message under "error". The HTTP status code carries the
// category (4xx client error, 5xx server error), so it is not duplicated in the
// body. Both SDKs read the "error" field, so that key is part of the contract.
type ErrorResponse struct {
	Error string `json:"error"`
}

// HandleQueryRequest is the JSON body expected by the query endpoint. It carries
// the raw query string plus any out-of-band parameters it references. Vector
// placeholders in the query (e.g. vec:$v) are bound by name from Parameters, so
// the parser never has to handle large vector literals inline.
type HandleQueryRequest[P float32 | float64] struct {
	Query      string         `json:"query"`
	Parameters map[string][]P `json:"parameters,omitempty"`
}
