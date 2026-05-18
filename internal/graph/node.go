package graph

import "time"

type Node[K comparable] interface {
	GetID() K
	GetValue() string
	GetTimestamp() time.Time
	GetProperties() *Properties
}
