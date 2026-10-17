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

// Source identifies the retrieval stage that produced a Contribution. Each
// source records observations on its own scale, and only the Scorer knows how
// to combine them. So a contribution can be read only by knowing where it came
// from.
type Source uint8

const (
	// SrcText is the full-text index. Score is the BM25 × coverage mass.
	SrcText Source = iota

	// SrcVector is the vector index. Score is the similarity 1/(1+distance),
	// scaled by the query's vector weight.
	SrcVector

	// SrcGraph is the anchor traversal. Score is the funding anchor's full
	// observed mass M_A. It is the raw observation, before the scorer subtracts
	// the fair share and applies the hinge.
	SrcGraph

	// SrcAnchor is anchor seeding. A Search that names anchors and nothing to
	// match seeds from the anchors' own members (see graph.Graph). Score is a
	// unit mass, one sighting per named anchor the fact is filed under. A fact
	// under several named anchors carries more seed mass than a fact under one.
	// It answers more of the question.
	SrcAnchor
)

// Scorer folds one candidate's contributions into its relevance score. Higher
// wins. Search first folds each seed's own contributions, before any traversal,
// to fix the seed masses the traversal aggregates. Then it folds every
// candidate's full list. Every concurrent search on the graph shares one
// instance. So implementations must be pure: no mutation of the slice, no
// mutable state, same input same output.
//
// Query-scoped inputs arrive by binding, never by mutation. WithBackground
// returns a scorer bound to one query's background rate, the observed mass per
// unit of anchor degree. It is the one query-global number a null model needs.
// An unbound scorer folds at background zero. That is what seed fusion needs,
// since nothing has been observed before the traversal runs. Mutating the
// shared instance instead would race one query's background into another's
// folds, since reads run concurrently under RLock.
type Scorer[K comparable, P float32 | float64] interface {
	// Score folds contributions at the scorer's bound background rate.
	Score(contributions []Contribution[K, P]) P

	// WithBackground returns a scorer bound to background. A fold with no null
	// model returns itself.
	WithBackground(background P) Scorer[K, P]
}

// String names the source for logs and the explain payload. Explain serialises
// contributions for clients, which never see the Go constants.
func (s Source) String() string {
	switch s {
	case SrcText:
		return "text"
	case SrcVector:
		return "vector"
	case SrcGraph:
		return "graph"
	case SrcAnchor:
		return "anchor"
	default:
		return "unknown"
	}
}

// Contribution records one observation of a candidate by one retrieval source.
// Collection sites record what they saw: who, through which anchor, and how
// much mass sat on it. They apply no policy of their own. The hinge, the null
// model and the attenuation belong to the Scorer.
//
// Score is oriented so that bigger is always better. So the vector site turns
// the distance its index reports, where smaller is nearer, into a similarity.
// Each Src constant says what Score holds for its source. Rank is the
// candidate's position in its source's own result list, and 0 is best. A graph
// observation or an anchor sighting has no list and leaves Rank zero. Via,
// Degree and Count describe the anchor of a graph or anchor observation. Via is
// the funding or filing anchor, Degree its degree at collection time, and Count
// how many seeds funded it. A seed contribution's Count is 1.
type Contribution[K comparable, P float32 | float64] struct {
	Src    Source
	Score  P
	Rank   uint16
	Via    K
	Degree uint32
	Count  uint16
}

// Candidates pools every contribution made during one search, keyed by
// candidate node. Append order is deterministic. The sources run in a fixed
// order, and seed traversals run in ascending key order. So a Scorer may fold a
// list's floats front to back without run-to-run drift in the low bits.
type Candidates[K comparable, P float32 | float64] map[K][]Contribution[K, P]
