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

import "errors"

// Sentinel errors returned by the server when a component fails to start or stop.
var (
	// ErrUnableToStartDatabase is returned when the database fails to start.
	ErrUnableToStartDatabase = errors.New("server: error while attempting to start database")
	// ErrUnableToStartEngine is returned when the query engine fails to start.
	ErrUnableToStartEngine = errors.New("server: error while attempting to start engine")
	// ErrUnableToServe is returned when the HTTP server fails for a reason other
	// than a clean shutdown (e.g. the port is already in use).
	ErrUnableToServe = errors.New("server: error while serving HTTP")
	// ErrUnableToStopDatabase is returned when the database fails to stop.
	ErrUnableToStopDatabase = errors.New("server: error while attempting to stop database")
	// ErrUnableToStopEngine is returned when the query engine fails to stop.
	ErrUnableToStopEngine = errors.New("server: error while attempting to stop engine")
	// ErrUnableToStartScheduler is returned when the scheduler fails to start.
	ErrUnableToStartScheduler = errors.New("server: error while attempting to start scheduler")
)
