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

import "math"

// Source identifies the retrieval stage that produced a Contribution. Scorers
// dispatch on it: each source reports Score on its own scale (match count,
// similarity, a seed's fused score), so contributions can only be combined by
// knowing where each one came from.
type Source uint8

const (
	// SrcText is the full-text index; Score is the raw match count.
	SrcText Source = iota

	// SrcVector is the vector index; Score is the similarity 1/(1+distance).
	SrcVector

	// SrcGraph is the traversal; Score is the fused seed score of the walk
	// that reached the node.
	SrcGraph
)

// String names the source for logs and for the explain payload, which
// serializes contributions for clients that never see the Go constants.
func (s Source) String() string {
	switch s {
	case SrcText:
		return "text"
	case SrcVector:
		return "vector"
	case SrcGraph:
		return "graph"
	default:
		return "unknown"
	}
}

// Contribution records one sighting of a candidate by one retrieval source.
// Collection sites record what they know as they produce results and apply no
// weighting of their own — rank discounts, hop attenuation and magnitude
// scaling all belong to the Scorer — so changing how ranking works never
// touches the collection sites again.
//
// Score is oriented so that bigger is always better: the vector site converts
// the distance its index reports (smaller is nearer) to 1/(1+distance) on the
// way in. Rank is the candidate's position in the producing source's own
// result list (0 is best), recorded at collection because each source knows
// its ordering as it emits results. Hop is 0 for seeds and the hop count for
// graph-reached candidates.
type Contribution[P float32 | float64] struct {
	Src   Source
	Score P
	Rank  uint16
	Hop   uint8
}

// Candidates pools every contribution made during one search, keyed by
// candidate node. Append order is deterministic — the sources run in a fixed
// order and seed walks run in ascending key order — so a Scorer may fold a
// list's floats front to back without run-to-run drift in the low bits.
type Candidates[K comparable, P float32 | float64] map[K][]Contribution[P]

// Scorer folds one candidate's contributions into its relevance score; higher
// wins. Search applies it twice — to each seed's contributions before any
// walk (the walk stamps that fused score on the SrcGraph contributions it
// appends) and to every candidate's full list at the end — so implementations
// must be pure: no mutation of the slice, same input, same output.
type Scorer[K comparable, P float32 | float64] interface {
	Score([]Contribution[P]) P
}

// clampRank bounds a source position to Contribution.Rank's range: a result
// list longer than the field would otherwise wrap, ranking overflow positions
// as if they were the best.
func clampRank(rank int) uint16 {
	if rank > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(rank)
}

// clampHop bounds a walk depth to Contribution.Hop's range, for the same
// reason as clampRank: a wrapped hop would rank a far node as if adjacent.
func clampHop(hop int) uint8 {
	if hop > math.MaxUint8 {
		return math.MaxUint8
	}
	return uint8(hop)
}
