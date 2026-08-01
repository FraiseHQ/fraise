package containers

import "errors"

var (
	ErrDimMismatch           = errors.New("vector dimensions do not match")
	ErrPriorityQueueCapacity = errors.New("priority queue capacity must be strictly positive")
	ErrEmptyPriorityQueue    = errors.New("priority queue is empty")
)
