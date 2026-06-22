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
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/RonsenbergVI/fraise/internal/query/parser"
)

type Query[K comparable, P float32 | float64] interface {
	Plan(config *config.ConfigSet) (*Stream[K, P], error)
	GetGraphID() uint8
	hash.Hashable[K, string]
	IsWrite() bool
	SetGraphID(id uint8)
}

type QueryParameters struct {
	Top   int
	Depth int
	Since containers.TimeValue
	Until containers.TimeValue
}

type QueryContext struct {
	GraphID uint8
}

type QueryResult[K comparable, P float32 | float64] struct {
	Count int
	Hits  []Hit[K, P]
}

type Hit[K comparable, P float32 | float64] struct {
	Node  *graph.Node[K]
	Score P
}

func Parse[K comparable, P float32 | float64](q string) (*Query[K, P], error) {

	var qo Query[K, P]
	var qc parser.CommandNode

	cmd, _, err := parser.Parse(q)

	if err != nil {
		return nil, ErrParsingFailed
	}

	// qp := i.Evaluate(cmd)

	switch cmd.(type) {
	case parser.RememberCommandNode:
		qc := cmd.(parser.RememberCommandNode)
		qo = Remember[K, P]{}
	case parser.RecallCommandNode:
		qc := cmd.(parser.RecallCommandNode)
		qp := QueryParameters{
			Top:   qc.Top(),
			Depth: qc.Depth(),
			Since: qc.Since(),
			Until: qc.Until(),
		}
		qo = Recall[K, P]{
			Keywords:   qc.Terms(),
			Entities:   qc.Entities(),
			Topics:     qc.Topics(),
			Vector:     containers.Vector[P]{Data: qc.Vector()},
			Parameters: qp,
		}
	default:
	}

	qo.SetGraphID(qc.Selector())

	return &qo, nil
}
