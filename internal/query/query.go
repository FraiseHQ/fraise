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

	// Background is the query's background rate ρ₀, the observed mass per unit
	// of degree over the anchors the traversal touched, attached only in
	// explain mode (omitempty keeps it off a plain response). With each hit's
	// contributions it lets a client recompute the hit's relevance (its score
	// before boost and recency decay):
	// S = m + α²·Σ max(0, (M_A − m − d_A·ρ₀)/d_A).
	Background P `json:"background,omitempty"`
}

type Hit[K comparable, P float32 | float64] struct {
	Node  *graph.Node[K]
	Score P

	// Contributions is the hit's per-source breakdown, set only in explain
	// mode. nil keeps it off the wire, and is unambiguous because a recall hit
	// always has at least one contribution. The entries are already in wire
	// form, anchors resolved to their values, because resolving them needs the
	// graph, which only Commit holds.
	Contributions []HitContribution[P]
}

// HitContribution is the wire form of one contribution, with the source
// serialized by name because clients never see the Go constants. A text or
// vector entry carries its raw mass and list position. A graph entry, one per
// funding anchor, carries the anchor's full observed mass, its value under
// via, its degree and how many seeds funded it. An anchor entry, one per
// named anchor an anchor-seeded fact is filed under, carries a unit mass and
// the anchor's value and degree. With the query's background rate these are
// the scorer's whole input, so a client can recompute the hit's relevance
// from its own payload.
type HitContribution[P float32 | float64] struct {
	Source string `json:"source"`
	Score  P      `json:"score"`
	Rank   uint16 `json:"rank"`
	Via    string `json:"via,omitempty"`
	Degree uint32 `json:"degree,omitempty"`
	Count  uint16 `json:"count"`
}

// MarshalJSON flattens the node into the hit so the response carries only the
// value, timestamp and score, with no nested Node object. The contributions
// appear only when the hit carries them (explain mode), so an ordinary query
// response has no contributions key.
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

// checkVector vets the vector the parser bound to placeholder name. The parser
// leaves it nil when no parameter has that name, which is ErrMissingParameter;
// a vector longer than maxDim is ErrLimitExceeded. Both are client errors
// (400); checking the length here keeps an oversized vector from reaching the
// index.
func checkVector[P float32 | float64](name string, data []P, maxDim int) error {
	if data == nil {
		return fmt.Errorf("%w: $%s", ErrMissingParameter, name)
	}
	if len(data) > maxDim {
		return fmt.Errorf("%w: vector $%s has %d dimensions, max %d", ErrLimitExceeded, name, len(data), maxDim)
	}
	return nil
}

// Parse turns a raw query string into an executable Query. Vector arguments are
// passed out-of-band in params, keyed by the placeholder name used in the query
// (e.g. `vec:$v` binds to params["v"]): the parser binds the vector, and the
// bound vector is checked here against the configured limits.
//
// Warnings flag a reading of a valid query the client may not have meant (see
// parser.Warning). They are returned beside the query, never stored on it,
// because the plan cache substitutes query objects on a hash hit and state on
// the query would leak between requests.
func Parse[K comparable, P float32 | float64](q string, params map[string][]P, c *config.ConfigSet) (Query[K, P], []parser.Warning, error) {
	cmd, warns, err := parser.Parse[K, P](q, params)
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

		// Take the vector the parser bound to the placeholder (if any),
		// rejecting a missing or over-long vector.
		if name, ok := n.VecParam(); ok {
			if err := checkVector(name, n.Vector(), c.DB.MaxVectorDimension); err != nil {
				logger.Warn("Rejecting vector parameter for remember", "parameter", name, "error", err)
				return nil, nil, err
			}
			qo.Vector = containers.NewVector[K](n.Vector())
		}

		logger.Debug("Parsed remember query", "graph", qo.GetGraphID(), "value", qo.Value)

		return qo, warns, nil

	case *parser.RecallCommandNode[K, P]:
		// Enforce the top and depth ranges: an out-of-range value is a client
		// error, rejected rather than clamped. Only an explicit clause is
		// checked; the configured default is the operator's and is trusted,
		// even above the ceiling.
		top := n.Top(c.DB.DefaultTop)
		if n.HasTop() && (top < 1 || top > c.DB.MaxTop) {
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

		// Take the vector the parser bound to the placeholder (if any),
		// rejecting a missing or over-long vector.
		if name, ok := n.VecParam(); ok {
			if err := checkVector(name, n.Vector(), c.DB.MaxVectorDimension); err != nil {
				logger.Warn("Rejecting vector parameter for recall", "parameter", name, "error", err)
				return nil, nil, err
			}
			qo.Vector = containers.NewVector[K](n.Vector())
		}

		logger.Debug("Parsed recall query", "graph", qo.GetGraphID(), "keywords", len(qo.Keywords))
		return qo, warns, nil

	default:
		logger.Debug("Unsupported query command", "query", q)
		return nil, nil, ErrParsingFailed
	}
}
