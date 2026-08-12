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

type QueryParameters[K comparable] struct {
	Top   int
	Depth int
	Since containers.TimeValue[K]
	Until containers.TimeValue[K]
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

	// Contributions is the hit's per-source score breakdown, populated only
	// when the stream ran in explain mode; nil otherwise. nil doubles as the
	// serialization switch — a recall hit always has at least one
	// contribution, so nil unambiguously means "not asked for", never
	// "asked for and empty".
	Contributions []graph.Contribution[P]
}

// hitContribution is the wire shape of one contribution: the source is
// serialized by name, not by its Go constant, because the payload documents
// ranking to clients that never see the enum.
type hitContribution[P float32 | float64] struct {
	Source string `json:"source"`
	Score  P      `json:"score"`
	Rank   uint16 `json:"rank"`
	Hop    uint8  `json:"hop"`
}

// MarshalJSON flattens the node into the hit so the response carries only the
// value, timestamp and score, with no nested Node object. The contribution
// breakdown appears only when the hit carries one (explain mode), keeping the
// ordinary query response byte-compatible with what it was before explain
// existed.
func (h Hit[K, P]) MarshalJSON() ([]byte, error) {
	node := *h.Node

	var contributions []hitContribution[P]
	for _, c := range h.Contributions {
		contributions = append(contributions, hitContribution[P]{
			Source: c.Src.String(),
			Score:  c.Score,
			Rank:   c.Rank,
			Hop:    c.Hop,
		})
	}

	return json.Marshal(struct {
		Value         string               `json:"value"`
		Timestamp     time.Time            `json:"timestamp"`
		Score         P                    `json:"score"`
		Contributions []hitContribution[P] `json:"contributions,omitempty"`
	}{
		Value:         node.GetValue(),
		Timestamp:     node.GetTimestamp(),
		Score:         h.Score,
		Contributions: contributions,
	})
}

// bindVector resolves a vector placeholder to its data, enforcing both that the
// parameter was supplied and that its dimension is within maxDim. A missing
// parameter is ErrMissingParameter; an over-long vector is ErrLimitExceeded —
// both are client errors surfaced as 400, and both are bounded here so a huge
// vector never reaches the index.
func bindVector[P float32 | float64](params map[string][]P, name string, maxDim int) ([]P, error) {
	data, provided := params[name]
	if !provided {
		return nil, fmt.Errorf("%w: $%s", ErrMissingParameter, name)
	}
	if len(data) > maxDim {
		return nil, fmt.Errorf("%w: vector $%s has %d dimensions, max %d", ErrLimitExceeded, name, len(data), maxDim)
	}
	return data, nil
}

// Parse turns a raw query string into an executable Query. Vector arguments are
// passed out-of-band in params, keyed by the placeholder name used in the query
// (e.g. `vec:$v` binds to params["v"]). This keeps the parser lightweight: it
// only records the placeholder, and the real vector is injected here.
func Parse[K comparable, P float32 | float64](q string, params map[string][]P, c *config.ConfigSet) (Query[K, P], error) {
	cmd, _, err := parser.Parse[K, P](q)
	if err != nil {
		logger.Debug("Query parsing failed", "query", q, "error", err)
		return nil, fmt.Errorf("%w: %w", ErrParsingFailed, err)
	}

	switch n := cmd.(type) {
	case *parser.RememberCommandNode[P]:
		qo := &Remember[K, P]{
			Value:    n.Value(),
			Entities: n.Entities(),
			Topics:   n.Topics(),
		}
		qo.SetGraphID(n.Selector())

		// Bind the vector placeholder (if any) from the request parameters,
		// rejecting a missing or over-long vector.
		if name, ok := n.VecParam(); ok {
			data, err := bindVector(params, name, c.DB.MaxVectorDimension)
			if err != nil {
				logger.Warn("Rejecting vector parameter for remember", "parameter", name, "error", err)
				return nil, err
			}
			qo.Vector = containers.NewVector[K](data)
		}

		logger.Debug("Parsed remember query", "graph", qo.GetGraphID(), "value", qo.Value)

		return qo, nil

	case *parser.RecallCommandNode[K, P]:
		// Enforce the top/depth ceilings before building the query: a
		// client-supplied result count or walk depth over its ceiling is a
		// client error, rejected here rather than clamped. Only an explicit
		// clause is checked — the configured default is operator-set and trusted,
		// so an unspecified top/depth is never rejected even if the default
		// itself exceeds the ceiling.
		top := n.Top(c.DB.DefaultTop)
		if n.HasTop() && top > c.DB.MaxTop {
			logger.Warn("Rejecting recall over top ceiling", "top", top, "max", c.DB.MaxTop)
			return nil, fmt.Errorf("%w: top:%d exceeds max %d", ErrLimitExceeded, top, c.DB.MaxTop)
		}
		depth := n.Depth(c.DB.DefaultDepth)
		if n.HasDepth() && depth > c.DB.MaxDepth {
			logger.Warn("Rejecting recall over depth ceiling", "depth", depth, "max", c.DB.MaxDepth)
			return nil, fmt.Errorf("%w: depth:%d exceeds max %d", ErrLimitExceeded, depth, c.DB.MaxDepth)
		}

		qo := &Recall[K, P]{
			Keywords: n.Terms(),
			Entities: n.Entities(),
			Topics:   n.Topics(),
			Parameters: QueryParameters[K]{
				Top: top, Depth: depth, Since: n.Since(), Until: n.Until(),
			},
		}
		qo.SetGraphID(n.Selector())

		// Bind the vector placeholder (if any) from the request parameters,
		// rejecting a missing or over-long vector.
		if name, ok := n.VecParam(); ok {
			data, err := bindVector(params, name, c.DB.MaxVectorDimension)
			if err != nil {
				logger.Warn("Rejecting vector parameter for recall", "parameter", name, "error", err)
				return nil, err
			}
			qo.Vector = containers.NewVector[K](data)
		}

		logger.Debug("Parsed recall query", "graph", qo.GetGraphID(), "keywords", len(qo.Keywords))
		return qo, nil

	default:
		logger.Debug("Unsupported query command", "query", q)
		return nil, ErrParsingFailed
	}
}
