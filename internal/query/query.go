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
	"encoding/json"
	"fmt"
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/RonsenbergVI/fraise/internal/query/parser"
	"github.com/RonsenbergVI/fraise/pkg/logger"
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
	Count int         `json:"count"`
	Hits  []Hit[K, P] `json:"hits"`
}

type Hit[K comparable, P float32 | float64] struct {
	Node  *graph.Node[K]
	Score P
}

// MarshalJSON flattens the node into the hit so the response carries only the
// value, timestamp and score, with no nested Node object.
func (h Hit[K, P]) MarshalJSON() ([]byte, error) {
	node := *h.Node
	return json.Marshal(struct {
		Value     string    `json:"value"`
		Timestamp time.Time `json:"timestamp"`
		Score     P         `json:"score"`
	}{
		Value:     node.GetValue(),
		Timestamp: node.GetTimestamp(),
		Score:     h.Score,
	})
}

func bindVector[P float32 | float64](params map[string][]P, name string) ([]P, error) {
	data, provided := params[name]
	if !provided {
		return nil, fmt.Errorf("%w: $%s", ErrMissingParameter, name)
	}
	return data, nil
}

// Parse turns a raw query string into an executable Query. Vector arguments are
// passed out-of-band in params, keyed by the placeholder name used in the query
// (e.g. `vec:$v` binds to params["v"]). This keeps the parser lightweight: it
// only records the placeholder, and the real vector is injected here.
func Parse[K comparable, P float32 | float64](q string, params map[string][]P, c *config.ConfigSet) (Query[K, P], error) {
	cmd, _, err := parser.Parse[P](q)
	if err != nil {
		logger.Debug("Query parsing failed", "query", q, "error", err)
		return nil, fmt.Errorf("%w: %w", ErrParsingFailed, err)
	}

	// qp := i.Evaluate(cmd)

	switch n := cmd.(type) {
	case *parser.RememberCommandNode[P]:
		qo := &Remember[K, P]{
			Value:    n.Value(),
			Entities: n.Entities(),
			Topics:   n.Topics(),
		}
		qo.SetGraphID(n.Selector())

		// Bind the vector placeholder (if any) from the request parameters.
		if name, ok := n.VecParam(); ok {
			data, err := bindVector(params, name)
			if err != nil {
				logger.Warn("Missing vector parameter for remember", "parameter", name)
				return nil, fmt.Errorf("%w: $%s", ErrMissingParameter, name)
			}
			qo.Vector = containers.NewVector(data)
		}

		logger.Debug("Parsed remember query", "graph", qo.GetGraphID(), "value", qo.Value)
		return qo, nil

	case *parser.RecallCommandNode[P]:
		qo := &Recall[K, P]{
			Keywords: n.Terms(),
			Entities: n.Entities(),
			Topics:   n.Topics(),
			Parameters: QueryParameters{
				Top: n.Top(c.DB.DefaultTop), Depth: n.Depth(c.DB.DefaultDepth), Since: n.Since(), Until: n.Until(),
			},
		}
		qo.SetGraphID(n.Selector())

		// Bind the vector placeholder (if any) from the request parameters.
		if name, ok := n.VecParam(); ok {
			data, err := bindVector(params, name)
			if err != nil {
				logger.Warn("Missing vector parameter for recall", "parameter", name)
				return nil, fmt.Errorf("%w: $%s", ErrMissingParameter, name)
			}
			qo.Vector = containers.NewVector(data)
		}

		logger.Debug("Parsed recall query", "graph", qo.GetGraphID(), "keywords", len(qo.Keywords))
		return qo, nil

	default:
		logger.Debug("Unsupported query command", "query", q)
		return nil, ErrParsingFailed
	}
}
