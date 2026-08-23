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

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/internal/query/parser"
	"github.com/FraiseHQ/fraise/pkg/logger"
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

	// Background is the query's background rate ρ₀ — the average seed mass
	// per unit of anchor degree the traversal observed — attached only in
	// explain mode. Together with each hit's contribution breakdown it lets a
	// client recompute every score: S = m + α²·Σ max(0, M_A − m − d_A·ρ₀).
	// omitempty doubles as the mode switch: a plain query never carries it.
	Background P `json:"background,omitempty"`
}

type Hit[K comparable, P float32 | float64] struct {
	Node  *graph.Node[K]
	Score P

	// Contributions is the hit's per-source breakdown, populated only when
	// the stream ran in explain mode; nil otherwise. nil doubles as the
	// serialization switch — a recall hit always has at least one
	// contribution, so nil unambiguously means "not asked for", never
	// "asked for and empty". The entries are wire-shaped (anchor identities
	// already resolved to their values) because resolution needs the graph,
	// which only the commit site holds.
	Contributions []HitContribution[P]
}

// HitContribution is the wire form of one contribution: the source is
// serialized by name, not by its Go constant, because the payload documents
// ranking to clients that never see the enum. A text or vector entry carries
// its raw mass and list position; a graph entry — one per funding anchor —
// carries the anchor's full observed mass, the anchor's value under via, its
// degree, and how many seeds funded it. With the query-level background rate,
// these are exactly the inputs of the scoring fold, so a client can recompute
// the hit's score from its own payload.
type HitContribution[P float32 | float64] struct {
	Source string `json:"source"`
	Score  P      `json:"score"`
	Rank   uint16 `json:"rank"`
	Via    string `json:"via,omitempty"`
	Degree uint32 `json:"degree,omitempty"`
	Count  uint16 `json:"count"`
}

// MarshalJSON flattens the node into the hit so the response carries only the
// value, timestamp and score, with no nested Node object. The contribution
// breakdown appears only when the hit carries one (explain mode), keeping the
// ordinary query response byte-compatible with what it was before explain
// existed.
func (h Hit[K, P]) MarshalJSON() ([]byte, error) {
	node := *h.Node

	return json.Marshal(struct {
		Value         string               `json:"value"`
		Timestamp     time.Time            `json:"timestamp"`
		Score         P                    `json:"score"`
		Contributions []HitContribution[P] `json:"contributions,omitempty"`
	}{
		Value:         node.GetValue(),
		Timestamp:     node.GetTimestamp(),
		Score:         h.Score,
		Contributions: h.Contributions,
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
//
// Warnings accompany a query that parsed and will run: they flag a reading the
// client may not have meant (see parser.Warning) and must travel to the client
// alongside the results, never attached to the query itself — the plan cache
// substitutes query objects on a hash hit, so state on the query would leak
// between requests.
func Parse[K comparable, P float32 | float64](q string, params map[string][]P, c *config.ConfigSet) (Query[K, P], []parser.Warning, error) {
	cmd, warns, err := parser.Parse[K, P](q)
	if err != nil {
		logger.Debug("Query parsing failed", "query", q, "error", err)
		return nil, nil, fmt.Errorf("%w: %w", ErrParsingFailed, err)
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
				return nil, nil, err
			}
			qo.Vector = containers.NewVector[K](data)
		}

		logger.Debug("Parsed remember query", "graph", qo.GetGraphID(), "value", qo.Value)

		return qo, warns, nil

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
			return nil, nil, fmt.Errorf("%w: top:%d out of range (1-%d)", ErrLimitExceeded, top, c.DB.MaxTop)
		}
		depth := n.Depth(c.DB.DefaultDepth)
		if n.HasDepth() && depth > c.DB.MaxDepth {
			logger.Warn("Rejecting recall over depth ceiling", "depth", depth, "max", c.DB.MaxDepth)
			return nil, nil, fmt.Errorf("%w: depth:%d out of range (0-%d)", ErrLimitExceeded, depth, c.DB.MaxDepth)
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
				return nil, nil, err
			}
			qo.Vector = containers.NewVector[K](data)
		}

		logger.Debug("Parsed recall query", "graph", qo.GetGraphID(), "keywords", len(qo.Keywords))
		return qo, warns, nil

	default:
		logger.Debug("Unsupported query command", "query", q)
		return nil, nil, ErrParsingFailed
	}
}
