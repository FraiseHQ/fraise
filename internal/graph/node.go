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

package graph

import "time"

type NodeAttributes[V string | ~float32 | ~int | ~bool] struct {
	Value      string
	Timestamp  time.Time
	Attributes []Property[V]
}

type Node[K comparable, V string | ~float32 | ~int | ~bool] interface {
	GetID() K
	GetValue() string
	GetTimestamp() time.Time
	GetAttributes() *NodeAttributes[V]
}

type Entity[K comparable, V string | ~float32 | ~int | ~bool] interface {
	GetAttributes() NodeAttributes[V]
}

type Relationship[K comparable, V string | ~float32 | ~int | ~bool] interface {
	Source() *Entity[K, V]
	Target() *Entity[K, V]
	GetAttributes() NodeAttributes[V]
}
