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

type Fact[K comparable, V string | ~float32 | ~int | ~bool] struct {
	ID K
	NodeAttributes[V]
}

func (f Fact[K, V]) GetID() K {
	return f.ID
}

func (f Fact[K, V]) GetValue() string {
	return f.Value
}

func (f Fact[K, V]) GetTimestamp() time.Time {
	return f.Timestamp
}

type NamedEntity[K comparable, V string | ~float32 | ~int | ~bool] struct {
	ID K
	NodeAttributes[V]
}

func (n NamedEntity[K, V]) GetID() K {
	return n.ID
}

func (n NamedEntity[K, V]) GetValue() string {
	return n.Value
}

func (n NamedEntity[K, V]) GetTimestamp() time.Time {
	return n.Timestamp
}

type Topic[K comparable, V string | ~float32 | ~int | ~bool] struct {
	ID K
	NodeAttributes[V]
}

func (t Topic[K, V]) GetID() K {
	return t.ID
}

func (t Topic[K, V]) GetValue() string {
	return t.Value
}

func (t Topic[K, V]) GetTimestamp() time.Time {
	return t.Timestamp
}
