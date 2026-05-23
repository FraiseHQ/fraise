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

package engine

import (
	"github.com/RonsenbergVI/fraise/internal/query"
	"github.com/RonsenbergVI/fraise/pkg/db"
)

// data structure representing a transaction.
// A transaction is the language for the engine to the worker
// Transactions can be read-only or read and write.
type Transaction[K comparable, V any, P float32 | float64] struct {
	DB      *db.DB[K, V, P]
	Write   bool
	Context *WriteContext
	Result  *query.QueryResult[K, P]
}

type WriteContext struct {
}

func (tx *Transaction[K, V, P]) Commit(ctx *WriteContext) error {
	return nil
}

func (tx *Transaction[K, V, P]) Rollback() error {
	return nil
}

func (tx *Transaction[K, V, P]) Get(key K) K {
	return tx.DB.Get(key)
}

func (tx *Transaction[K, V, P]) Set(key K, value V, ctx *WriteContext) error {
	return nil
}

func (tx *Transaction[K, V, P]) Put(key K, value V, ctx *WriteContext) error {
	return nil
}

func (tx *Transaction[K, V, P]) Search(query []V, ctx *WriteContext) []K {
	return nil
}
