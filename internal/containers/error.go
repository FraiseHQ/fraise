package containers

import "errors"

var (
	DimMismatchError         = errors.New("Vector dimensions do not match.")
	ErrPriorityQueueCapacity = errors.New("Priority queue capacity must be strictly positive.")
)
