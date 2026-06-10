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
	"fmt"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/graph"
)

// the db hols the logic of translating low level calls to the memory Graphs
// from and to the transaction object (that the server directly serialises to the client)
type DB[K comparable, P float32 | float64] struct {
	Config *config.ConfigSet
	Graphs []graph.Graph[K, P]

	stats *Stats
}

type Stats struct {
	Memory int
}

func NewDB[K comparable, P float32 | float64](cfg *config.ConfigSet) (*DB[K, P], error) {
	d := &DB[K, P]{
		Config: cfg,
		Graphs: make([]graph.Graph[K, P], config.DefaultNumGraph),
	}
	for i := range d.Graphs {
		d.Graphs[i] = graph.NewGraph[K, P]()
	}
	return d, nil
}

func (d *DB[K, P]) Start() error {
	return nil
}

func (d *DB[K, P]) Stop() error {
	return nil
}

func (d *DB[K, P]) Stats() Stats {
	return *d.stats
}

func (d *DB[K, P]) Select(index uint8) (graph.Graph[K, P], error) {
	if index < 0 || int(index) >= len(d.Graphs) {
		return nil, fmt.Errorf("index %d out of bounds for slice of length %d", index, len(d.Graphs))
	}
	return d.Graphs[index], nil
}
