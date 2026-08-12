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

package scheduler

import "errors"

var (
	// ErrShutdown is returned when work is submitted to a scheduler that has
	// already been shut down.
	ErrShutdown = errors.New("scheduler: shut down")
	// ErrEnqueueStream is returned when a stream cannot be added to the queue.
	ErrEnqueueStream = errors.New("scheduler: could not add stream to queue")
	// ErrQueueFull is returned when the queue stays saturated past the
	// configured enqueue timeout, so callers can shed load (e.g. answer 429)
	// instead of blocking without bound.
	ErrQueueFull = errors.New("scheduler: queue full")
	// ErrStreamExecution is returned when a stream fails while being executed.
	ErrStreamExecution = errors.New("scheduler: error while executing stream")
	// ErrStreamCommit is returned when a stream fails while being committed.
	// It always wraps the underlying commit error rather than replacing it, so
	// the HTTP boundary can errors.Is the cause out (e.g. a vector-dimension
	// mismatch, a client error) instead of reporting every failed commit as an
	// internal fault.
	ErrStreamCommit = errors.New("scheduler: error while committing stream")
)
