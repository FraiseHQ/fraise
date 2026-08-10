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

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/RonsenbergVI/fraise/internal/comparator"
	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/hash"
	"github.com/RonsenbergVI/fraise/internal/index"
	"github.com/RonsenbergVI/fraise/pkg/logger"
)

// InMemoryGraph is the in-process implementation of Graph. Nodes live in a
// key-addressed map, relationships in a pair of mirrored adjacency maps, and
// two secondary indices serve hybrid search: a BTree full-text index over the
// nodes' values and an RPTree (random projection forest) vector index over
// caller-provided embeddings.
type InMemoryGraph[K ~uint64, P float32 | float64] struct {
	idToNodes     map[K]Node[K] // all nodes
	nodeToSources map[K]map[K]K // incoming: target -> source -> edge
	nodeToTargets map[K]map[K]K // outgoing: source -> target -> edge

	textIndex   *index.BTreeIndex[K, P]
	vectorIndex *index.RPTreeIndex[K, P]

	// traversal and ranking are the pluggable search algorithms (see
	// Traversal and Ranking, mirrored here by the
	// unexported hook interfaces). traversal nil falls back to a built-in
	// breadth-first walk over both edge directions; ranking nil applies no
	// structural boost.
	traversal Traversal[K, P]
	ranking   Ranking[K, P]

	hasher hash.Hasher[K, string]

	config *config.ConfigSet

	mu sync.RWMutex
}

// SetTraversal installs the traversal algorithm Search expands seeds with
// (typically an algorithms.Traversal such as BFS).
func (g *InMemoryGraph[K, P]) SetTraversal(t Traversal[K, P]) {
	g.traversal = t
}

// SetRanking installs the global ranking algorithm Search boosts scores with
// (typically an algorithms.Ranking such as PageRank).
func (g *InMemoryGraph[K, P]) SetRanking(r Ranking[K, P]) {
	g.ranking = r
}

func NewGraph[K ~uint64, P float32 | float64](config *config.ConfigSet) *InMemoryGraph[K, P] {
	g := &InMemoryGraph[K, P]{
		idToNodes:     make(map[K]Node[K]),
		nodeToSources: make(map[K]map[K]K),
		nodeToTargets: make(map[K]map[K]K),
		textIndex:     index.NewBTreeIndex[K, P](comparator.OrderedComparator[K]),
		vectorIndex: index.NewRPTreeIndex[K, P](
			0,
			config.DB.VectorSearch.ProjectionDimension,
			config.DB.VectorSearch.NumberTrees,
			config.DB.VectorSearch.Seed,
			config.DB.VectorSearch.FlushFactor,
			comparator.OrderedComparator[K],
		),
		hasher: hash.NewHasher[K](config),
		config: config,
	}
	return g
}

// write lock
func (g *InMemoryGraph[K, P]) Lock() {
	g.mu.Lock()
}

// read lock
func (g *InMemoryGraph[K, P]) RLock() {
	g.mu.RLock()
}

// write unlock
func (g *InMemoryGraph[K, P]) Unlock() {
	g.mu.Unlock()
}

// read unlock
func (g *InMemoryGraph[K, P]) RUnlock() {
	g.mu.RUnlock()
}

// Get hasher
func (g *InMemoryGraph[K, P]) GetHasher() hash.Hasher[K, string] {
	return g.hasher
}

// Get returns the node stored under key, or nil if absent.
func (g *InMemoryGraph[K, P]) Get(key K) Node[K] {
	node, ok := g.idToNodes[key]
	if !ok {
		return nil
	}
	return node
}

// Set inserts a new node under its own ID, returning ErrNodeAlreadyExists if
// the key is taken.
func (g *InMemoryGraph[K, P]) Set(node Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	n := node
	if _, exists := g.idToNodes[n.Key()]; exists {
		return ErrNodeAlreadyExists
	}
	return g.store(n.Key(), n)
}

// Put stores node under key, replacing whatever was there.
func (g *InMemoryGraph[K, P]) Put(key K, node Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	return g.store(key, node)
}

// store records the node and (re)indexes its value in the text index.
func (g *InMemoryGraph[K, P]) store(key K, node Node[K]) error {
	g.idToNodes[key] = node

	r, ok := node.(Relationship[K])
	if ok {
		// store relationship
		source := (*r.Source()).Key()
		target := (*r.Target()).Key()

		if g.nodeToTargets[source] == nil {
			g.nodeToTargets[source] = make(map[K]K)
		}

		g.nodeToTargets[source][target] = r.Key()

		if g.nodeToSources[target] == nil {
			g.nodeToSources[target] = make(map[K]K)
		}

		g.nodeToSources[target][source] = r.Key()
	}

	if attrs := node.GetAttributes(); attrs != nil {
		if err := g.textIndex.Insert(key, attrs.Value); err != nil {
			logger.Warn("Failed to index node text", "error", err)
			return err
		}
	}
	return nil
}

// Delete removes the node, its incident relationships and its index entries.
func (g *InMemoryGraph[K, P]) Delete(node Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	key := node.Key()
	if _, ok := g.idToNodes[key]; !ok {
		return ErrNodeNotFound
	}

	for target := range g.nodeToTargets[key] {
		delete(g.nodeToSources[target], key)
	}
	for source := range g.nodeToSources[key] {
		delete(g.nodeToTargets[source], key)
	}
	delete(g.nodeToTargets, key)
	delete(g.nodeToSources, key)
	delete(g.idToNodes, key)

	// The node may legitimately be absent from either index.
	_ = g.textIndex.Delete(key)
	_ = g.vectorIndex.Delete(key)
	return nil
}

func (g *InMemoryGraph[K, P]) Nodes() map[K]Node[K] {
	return g.idToNodes
}

func (g *InMemoryGraph[K, P]) AdjacencyMap() map[K]map[K]K {
	return exportEdges(g.nodeToTargets)
}

func (g *InMemoryGraph[K, P]) PredecessorMap() map[K]map[K]K {
	return exportEdges(g.nodeToSources)
}

// exportEdges converts the internal edge maps to the pointer-valued shape the
// Graph interface exposes.
func exportEdges[K comparable](edges map[K]map[K]K) map[K]map[K]K {
	out := make(map[K]map[K]K, len(edges))
	for from, tos := range edges {
		row := make(map[K]K, len(tos))
		for to, rel := range tos {
			row[to] = rel
		}
		out[from] = row
	}
	return out
}

// Order returns the number of entities (vertices) in the graph.
func (g *InMemoryGraph[K, P]) Order() int {
	order := 0
	for _, node := range g.idToNodes {
		if _, ok := node.(Entity[K]); ok {
			order++
		}
	}
	return order
}

// Size returns the number of relationships (edges) in the graph.
func (g *InMemoryGraph[K, P]) Size() int {
	size := 0
	for _, targets := range g.nodeToTargets {
		size += len(targets)
	}
	return size
}

func (g *InMemoryGraph[K, P]) Stats() GraphStats {
	return GraphStats{
		Order:   g.Order(),
		Size:    g.Size(),
		Nodes:   len(g.idToNodes),
		Vectors: g.GetVectorIndex().Count(),
		// Entries - Count is the vector index's compaction debt; the index's
		// automatic Flush keeps it bounded (see rptree flush-factor).
		ForestEntries: g.GetVectorIndex().Entries(),
	}
}

// Returns the graph vector index
func (g *InMemoryGraph[K, P]) GetVectorIndex() index.VectorIndex[K, P] {
	return g.vectorIndex
}

// Returns the graph full text search index
func (g *InMemoryGraph[K, P]) GetTextIndex() index.TextIndex[K, P] {
	return g.textIndex
}

func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*Node[K], []P) {
	// A. Search starts with gathering seeds for the graph search.
	// Seeds are found from
	// 1. Vector search (top K - default = 10)
	// 2. Matching keywords
	seeds, seedScores := g.gatherSeeds(keywords, vector)

	// B. Walking the graph from all seeds and unioning the found facts
	neighbors, scores := g.findNeighbours(seeds, seedScores, topics, entities, depth)

	// C. Time filtered (since or until)

	candidates, ranked := g.timeFilter(neighbors, scores, since, until)

	// D. Rank the candidates and truncate to top
	//
	// The candidates arrive in the order the score map was iterated in, so the
	// ranking has to be a total order for identical queries to return identical
	// hits: score descending, then fact key. Facts of equal score are ordered
	// by key rather than left as the map presented them, because truncation
	// would otherwise keep an arbitrary subset of a tied group.

	sort.Slice(candidates, func(i, j int) bool {
		if ranked[candidates[i]] != ranked[candidates[j]] {
			return ranked[candidates[i]] > ranked[candidates[j]]
		}
		return candidates[i] < candidates[j]
	})

	limit := top
	if limit <= 0 || limit > len(candidates) {
		limit = len(candidates)
	}

	nodes := make([]*Node[K], limit)
	scoresOut := make([]P, limit)
	for i, key := range candidates[:limit] {
		node := g.idToNodes[key]
		nodes[i] = &node
		scoresOut[i] = ranked[key]
	}

	logger.Debug("Graph search completed",
		"seeds", len(seeds), "candidates", len(candidates), "returned", limit)
	return nodes, scoresOut
}

// gatherSeeds pools search seeds from the text index (keywords) and the
// vector index (query embedding). Seeds are scored by source rank, 1/(1+rank),
// and a key surfaced by both sources accumulates both scores.
func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[K, P]) ([]K, map[K]P) {
	scores := make(map[K]P)

	var textSeeds, vectorSeeds int
	if len(keywords) > 0 {
		// Index errors (empty index) just mean no text seeds. Seeds are scored
		// by rank rather than raw index score: the text and vector indices score
		// on different scales (match count vs distance), so rank keeps them
		// comparable when their seeds are pooled here.
		if keys, _, err := g.textIndex.Search(strings.Join(keywords, " "), g.config.DB.SeedSize); err == nil {
			textSeeds = len(keys)
			for rank, key := range keys {
				scores[key] += P(1) / P(1+rank)
			}
		} else {
			logger.Debug("Text index yielded no seeds", "error", err)
		}
	}

	if !vector.Empty() {
		if keys, _, err := g.vectorIndex.Search(vector, g.config.DB.SeedSize); err == nil {
			vectorSeeds = len(keys)
			for rank, key := range keys {
				scores[key] += P(1) / P(1+rank)
			}
		} else {
			logger.Debug("Vector index yielded no seeds", "error", err)
		}
	}

	seeds := make([]K, 0, len(scores))
	for key := range scores {
		seeds = append(seeds, key)
	}
	// Seeds are expanded in key order because findNeighbours pools their
	// contributions by summing floats, which is not associative: walking them
	// in map-iteration order perturbs the low bits of a shared neighbour's
	// score, and two identical queries can then rank it differently.
	sort.Slice(seeds, func(i, j int) bool { return seeds[i] < seeds[j] })
	logger.Debug("Gathered search seeds",
		"text", textSeeds, "vector", vectorSeeds, "unique", len(seeds))
	return seeds, scores
}

// findNeighbours expands every seed into its neighbourhood using the graph's
// walker (or the built-in breadth-first walk when none is set). A node
// discovered h hops from a seed contributes that seed's score attenuated by
// hopAttenuation^h; contributions from multiple seeds accumulate. When a
// ranker is installed its global scores boost the pooled walk scores. The
// pooled nodes are then filtered by topics and entities.
func (g *InMemoryGraph[K, P]) findNeighbours(seeds []K, seedScores map[K]P, topics []string, entities []string, depth int) ([]K, map[K]P) {
	scores := make(map[K]P, len(seedScores))
	for _, seed := range seeds {
		for key, hop := range g.walk(seed, depth) {
			scores[key] += seedScores[seed] * P(math.Pow(g.config.DB.HopAttenuation, float64(hop)))
		}
	}

	if g.ranking != nil {
		result, err := g.ranking.Run(g)
		r, _ := result.(RankingResult[K, P])
		if err == nil && len(r.Scores) > 0 {
			// Boost by the mean-normalised global score: an average node is
			// unchanged, central nodes gain, nodes unknown to the ranker
			// (e.g. isolated ones) keep their walk score.
			n := P(len(r.Scores))
			for key := range scores {
				if r, ok := r.Scores[key]; ok {
					scores[key] *= 1 + r*n
				}
			}
		}
	}

	keys := make([]K, 0, len(scores))
	for key := range scores {
		if !g.matchesFilter(key, topics) || !g.matchesFilter(key, entities) {
			delete(scores, key)
			continue
		}
		keys = append(keys, key)
	}
	return keys, scores
}

// walk expands source up to depth hops, delegating to the installed
// Traversal or falling back to a breadth-first traversal over both edge
// directions.
func (g *InMemoryGraph[K, P]) walk(source K, depth int) map[K]int {
	if g.traversal != nil {

		// Clone per walk so concurrent searches never share the mutable source.
		t := g.traversal.Clone()
		t.SetSource(source)
		result, err := t.Run(g)

		r, _ := result.(TraversalResult[K])

		if err != nil {
			return nil
		}
		depths := make(map[K]int, len(r.Depth))
		for key, hop := range r.Depth {
			if hop <= depth {
				depths[key] = hop
			}
		}
		return depths
	}

	depths := map[K]int{source: 0}
	frontier := []K{source}
	for hop := 1; hop <= depth && len(frontier) > 0; hop++ {
		var next []K
		for _, u := range frontier {
			for _, neighbors := range []map[K]K{g.nodeToTargets[u], g.nodeToSources[u]} {
				for v := range neighbors {
					if _, seen := depths[v]; seen {
						continue
					}
					depths[v] = hop
					next = append(next, v)
				}
			}
		}
		frontier = next
	}
	return depths
}

// matchesFilter reports whether the node passes a value filter: trivially if
// values is empty, otherwise if its own value or any direct neighbour's value
// is one of values (facts are tagged with topics/entities by being linked to
// the Topic/NamedEntity node carrying that value).
func (g *InMemoryGraph[K, P]) matchesFilter(key K, values []string) bool {
	if len(values) == 0 {
		return true
	}

	match := func(k K) bool {
		node, ok := g.idToNodes[k]
		if !ok {
			return false
		}
		attrs := node.GetAttributes()
		if attrs == nil {
			return false
		}
		for _, v := range values {
			if strings.EqualFold(attrs.Value, v) {
				return true
			}
		}
		return false
	}

	if match(key) {
		return true
	}
	for neighbor := range g.nodeToTargets[key] {
		if match(neighbor) {
			return true
		}
	}
	for neighbor := range g.nodeToSources[key] {
		if match(neighbor) {
			return true
		}
	}
	return false
}

// timeFilter drops nodes outside [since, until) — either bound is unbounded
// when zero — and returns the surviving keys with their scores decayed by
// recency: a fact's score is multiplied by 0.5^(age/half-life), so of two
// equally relevant facts the more recent one outranks the older ("recent
// memories outrank older ones"). The half-life comes from Engine.Halflife; a
// non-positive value disables decay. A timestamp in the future decays as age
// zero — recency never boosts a score above its relevance.
func (g *InMemoryGraph[K, P]) timeFilter(keys []K, scores map[K]P, since time.Time, until time.Time) ([]K, map[K]P) {
	now := time.Now()
	halflife := g.config.Engine.Halflife

	kept := make([]K, 0, len(keys))
	for _, key := range keys {
		node, ok := g.idToNodes[key]
		if !ok {
			delete(scores, key)
			continue
		}
		// Only facts are memories: Topic/NamedEntity nodes exist to seed and
		// filter searches (they are walked through and matched against), but
		// they are never returned as hits themselves.
		if _, isFact := node.(Fact[K]); !isFact {
			delete(scores, key)
			continue
		}
		ts := node.GetAttributes().Timestamp
		if !since.IsZero() && ts.Before(since) {
			delete(scores, key)
			continue
		}
		if !until.IsZero() && !ts.Before(until) {
			delete(scores, key)
			continue
		}

		if halflife > 0 {
			if age := now.Sub(ts); age > 0 {
				scores[key] *= P(math.Pow(0.5, age.Seconds()/halflife.Seconds()))
			}
		}
		kept = append(kept, key)
	}
	return kept, scores
}

// Copy returns a deep copy of the graph: nodes, relationships and both
// indices are rebuilt so mutating one graph never affects the other.
func (g *InMemoryGraph[K, P]) Copy() Graph[K, P] {
	out := NewGraph[K, P](g.config)
	for key, node := range g.idToNodes {
		_ = out.store(key, node)
	}
	for key, vector := range g.vectorIndex.Vectors() {
		_ = out.vectorIndex.Insert(key, vector)
	}
	return out
}

// MergeFrom merges the contents of in into this graph: nodes, relationships
// and index entries. On key collision the incoming node wins.
func (g *InMemoryGraph[K, P]) MergeFrom(in Graph[K, P]) {
	other, ok := in.(*InMemoryGraph[K, P])
	if !ok {
		return
	}

	for key, node := range other.idToNodes {
		_ = g.store(key, node)
	}
	for key, vector := range other.vectorIndex.Vectors() {
		_ = g.vectorIndex.Insert(key, vector)
	}
}
