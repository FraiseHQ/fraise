package containers

import "errors"

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
