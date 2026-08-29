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

package scoring

// Source identifies the retrieval stage that produced a Contribution.
// Collection sites record observations on each source's own scale; the
// Scorer is what knows how to combine them, so contributions can only be
// read by knowing where each one came from.
type Source uint8

const (
	// SrcText is the full-text index; Score is the BM25 × coverage mass.
	SrcText Source = iota

	// SrcVector is the vector index; Score is the similarity 1/(1+distance).
	SrcVector

	// SrcGraph is the anchor traversal; Score is the funding anchor's full
	// observed mass M_A — the raw observation, before the scorer subtracts
	// the fair share and applies the hinge.
	SrcGraph
)

// Scorer folds one candidate's contributions into its relevance score;
// higher wins. Search applies it twice — to each seed's own contributions
// before any traversal (fixing the seed masses the traversal aggregates) and
// to every candidate's full list at the end — and one instance is shared by
// every concurrent search on the graph, so implementations must be pure: no
// mutation of the slice, no mutable state, same input same output.
//
// Query-scoped inputs arrive by binding, never by mutation. WithBackground
// returns a scorer bound to one query's background rate — the average mass
// density per unit of anchor degree the traversal observed, the one
// query-global number a null model needs. An unbound scorer folds at
// background zero, which is exactly what seed fusion wants: before the
// traversal has observed anything there is no null to compare against.
// Mutating the shared instance instead would race one query's background
// into another's folds, since reads run concurrently under RLock.
type Scorer[K comparable, P float32 | float64] interface {
	// Score folds contributions at the scorer's bound background rate.
	Score(contributions []Contribution[K, P]) P

	// WithBackground returns a scorer bound to background; a fold with no
	// null model returns itself.
	WithBackground(background P) Scorer[K, P]
}

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

// Contribution records one observation of a candidate by one retrieval
// source. Collection sites record what they saw — who, through which anchor,
// how much mass sat on it — and apply no policy of their own: the hinge, the
// null model and the attenuation all belong to the Scorer, so changing how
// ranking works never touches the collection sites again.
//
// Score is oriented so that bigger is always better: the vector site converts
// the distance its index reports (smaller is nearer) to 1/(1+distance) on the
// way in; a graph observation carries the funding anchor's full observed
// mass. Rank is the candidate's position in the producing source's own result
// list (0 is best) — a graph observation has no list and leaves it zero. Via,
// Degree and Count exist for graph observations: the funding anchor, its
// degree at collection time, and how many seed members funded it; a seed
// contribution's Count is 1.
type Contribution[K comparable, P float32 | float64] struct {
	Src    Source
	Score  P
	Rank   uint16
	Via    K
	Degree uint32
	Count  uint16
}

// Candidates pools every contribution made during one search, keyed by
// candidate node. Append order is deterministic — the sources run in a fixed
// order and seed traversals run in ascending key order — so a Scorer may fold
// a list's floats front to back without run-to-run drift in the low bits.
type Candidates[K comparable, P float32 | float64] map[K][]Contribution[K, P]
