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

package containers

import (
	"errors"
	"fmt"
)

var (
	// ErrDimMismatch is returned by vector operations when the operands have
	// different dimensionalities.
	ErrDimMismatch = errors.New("containers: vector dimensions do not match")
	// ErrPriorityQueueCapacity is returned when a priority queue is created with
	// a capacity that is not strictly positive.
	ErrPriorityQueueCapacity = errors.New("containers: priority queue capacity must be strictly positive")
	// ErrEmptyPriorityQueue is returned when popping or peeking an empty
	// priority queue.
	ErrEmptyPriorityQueue = errors.New("containers: priority queue is empty")
	// ErrInvalidTime is returned by ParseTimeValue when the input is neither a
	// relative duration ("7d") nor a parseable absolute date.
	ErrInvalidTime = errors.New("containers: invalid time value")
)

// DurationRangeError is returned by ParseTimeValue for a relative duration
// longer than a time.Duration can hold — about 292 years, whatever the unit.
// It carries the largest count its unit allows so the message can state the
// range, and it is an ErrInvalidTime: a caller that only asks whether the
// value parsed still gets the right answer.
type DurationRangeError struct {
	Value string // the duration as written, e.g. "106752d"
	Max   int64  // the largest count its unit allows, e.g. 106751
	Unit  byte   // the unit letter, e.g. 'd'
}

func (e *DurationRangeError) Error() string {
	return fmt.Sprintf("containers: duration %q out of range (at most %d%c)", e.Value, e.Max, e.Unit)
}

func (e *DurationRangeError) Unwrap() error { return ErrInvalidTime }
