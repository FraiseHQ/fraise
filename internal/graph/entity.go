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

type Fact[K comparable] struct {
	ID K
	EntityAttributes[K]
	Properties EntityProperties
}

func (f Fact[K]) GetID() K {
	return f.ID
}

func (f Fact[K]) GetValue() string {
	return f.Value
}

func (f Fact[K]) GetTimestamp() time.Time {
	return f.Timestamp
}

type NamedEntity[K comparable] struct {
	ID K
	EntityAttributes[K]
	Properties EntityProperties
}

func (n NamedEntity[K]) GetID() K {
	return n.ID
}

func (n NamedEntity[K]) GetValue() string {
	return n.Value
}

func (n NamedEntity[K]) GetTimestamp() time.Time {
	return n.Timestamp
}

type Topic[K comparable] struct {
	ID K
	EntityAttributes[K]
	Properties EntityProperties
}

func (t Topic[K]) GetID() K {
	return t.ID
}

func (t Topic[K]) GetValue() string {
	return t.Value
}

func (t Topic[K]) GetTimestamp() time.Time {
	return t.Timestamp
}
