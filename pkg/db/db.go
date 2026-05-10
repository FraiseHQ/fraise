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

package db

import (
	"sync"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/graph"
	"github.com/RonsenbergVI/fraise/pkg/engine"
)

type DB[K comparable, P float32 | float64] struct {
	mu sync.RWMutex

	buf    []byte
	Config config.ConfigSet
	Graphs []graph.Graph[K, P]

	engine *engine.Engine[K, P]
}

type Stats struct {
}

func (d *DB[K, P]) Init() error {

}

func (d *DB[K, P]) Start() error {

}

func (d *DB[K, P]) Stop() error {
}

// selects with graph to use
func (d *DB[K, P]) Select(index int) error {

}

// Executes Query
func (d *DB[K, P]) Executes(query string) error {

	d.engine.Lock()

	transaction, err := d.engine.Init(query)

	result, err := d.engine.Run(transaction)

	d.engine.Release()
}
