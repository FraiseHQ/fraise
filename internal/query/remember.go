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

package query

import (
	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/hash"
)

type Remember[K comparable, P float32 | float64] struct {
	Value    string
	Entities []string
	Topics   []string

	context QueryContext
}

func (q Remember[K, P]) Plan(config *config.ConfigSet) (*Stream[K, P], error) {
	return nil, nil
}

func (r Remember[K, P]) GetGraphID() uint8 {
	return r.context.GraphID
}

func (r *Remember[K, P]) SetGraphID(id uint8) {
	r.context.GraphID = id
}

func (r Remember[K, P]) Hash(h hash.Hasher[K, string]) K {
	return h.Hash(r.Value)
}

func (r Remember[K, P]) IsWrite() bool {
	return true
}
