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

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/index"
)

const (
	// defaultSeedSize is how many seeds each source (text, vector) contributes
	// to a search.
	defaultSeedSize = 10

	// rpProjDim, rpNumTrees and rpSeed configure the vector index forest. The
	// embedding dimension itself is learned from the first inserted vector.
	rpProjDim  = 8
	rpNumTrees = 4
	rpSeed     = 1

	// hopAttenuation is the per-hop score decay applied while walking the
	// graph away from a seed.
	hopAttenuation = 0.5
)

// InMemoryGraph is the in-process implementation of Graph. Nodes live in a
// key-addressed map, relationships in a pair of mirrored adjacency maps, and
// two secondary indices serve hybrid search: a BTree full-text index over the
// nodes' values and an RPTree (random projection forest) vector index over
// caller-provided embeddings.
type InMemoryGraph[K comparable, P float32 | float64] struct {
	idToNodes     map[K]Node[K]
	nodeToSources map[K]map[K]Relationship[K] // incoming: target -> source -> edge
	nodeToTargets map[K]map[K]Relationship[K] // outgoing: source -> target -> edge

	textIndex   *index.BTreeIndex[K, P]
	vectorIndex *index.RPTreeIndex[K, P]

	// traversal and ranking are the pluggable search algorithms (see
	// algorithms.Traversal and algorithms.Ranking, mirrored here by the
	// unexported hook interfaces). traversal nil falls back to a built-in
	// breadth-first walk over both edge directions; ranking nil applies no
	// structural boost.
	traversal Traversal[K, P]
	ranking   Ranking[K, P]

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

func NewGraph[K comparable, P float32 | float64]() *InMemoryGraph[K, P] {
	g := &InMemoryGraph[K, P]{
		idToNodes:     make(map[K]Node[K]),
		nodeToSources: make(map[K]map[K]Relationship[K]),
		nodeToTargets: make(map[K]map[K]Relationship[K]),
		textIndex:     index.NewBTreeIndex[K, P](),
		vectorIndex:   index.NewRPTreeIndex[K, P](0, rpProjDim, rpNumTrees, rpSeed),
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

// Get returns the node stored under key, or nil if absent.
func (g *InMemoryGraph[K, P]) Get(key K) *Node[K] {
	node, ok := g.idToNodes[key]
	if !ok {
		return nil
	}
	return &node
}

// Set inserts a new node under its own ID, returning ErrNodeAlreadyExists if
// the key is taken.
func (g *InMemoryGraph[K, P]) Set(node *Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	n := *node
	if _, exists := g.idToNodes[n.GetID()]; exists {
		return ErrNodeAlreadyExists
	}
	return g.store(n.GetID(), n)
}

// Put stores node under key, replacing whatever was there.
func (g *InMemoryGraph[K, P]) Put(key K, node *Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	return g.store(key, *node)
}

// store records the node and (re)indexes its value in the text index.
func (g *InMemoryGraph[K, P]) store(key K, node Node[K]) error {
	g.idToNodes[key] = node
	if attrs := node.GetAttributes(); attrs != nil {
		return g.textIndex.Insert(key, attrs.Value)
	}
	return nil
}

// Delete removes the node, its incident relationships and its index entries.
func (g *InMemoryGraph[K, P]) Delete(node *Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	key := (*node).GetID()
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

// AddRelationship records the directed edge source -> target. Both endpoints
// must already be stored in the graph.
func (g *InMemoryGraph[K, P]) AddRelationship(source, target K, rel Relationship[K]) error {
	if _, ok := g.idToNodes[source]; !ok {
		return ErrNodeNotFound
	}
	if _, ok := g.idToNodes[target]; !ok {
		return ErrNodeNotFound
	}

	if g.nodeToTargets[source] == nil {
		g.nodeToTargets[source] = make(map[K]Relationship[K])
	}
	g.nodeToTargets[source][target] = rel

	if g.nodeToSources[target] == nil {
		g.nodeToSources[target] = make(map[K]Relationship[K])
	}
	g.nodeToSources[target][source] = rel
	return nil
}

func (g *InMemoryGraph[K, P]) AdjacencyMap() map[K]map[K]*Relationship[K] {
	return exportEdges(g.nodeToTargets)
}

func (g *InMemoryGraph[K, P]) PredecessorMap() map[K]map[K]*Relationship[K] {
	return exportEdges(g.nodeToSources)
}

// exportEdges converts the internal edge maps to the pointer-valued shape the
// Graph interface exposes.
func exportEdges[K comparable](edges map[K]map[K]Relationship[K]) map[K]map[K]*Relationship[K] {
	out := make(map[K]map[K]*Relationship[K], len(edges))
	for from, tos := range edges {
		row := make(map[K]*Relationship[K], len(tos))
		for to, rel := range tos {
			row[to] = &rel
		}
		out[from] = row
	}
	return out
}

// Copy returns a deep copy of the graph: nodes, relationships and both
// indices are rebuilt so mutating one graph never affects the other.
func (g *InMemoryGraph[K, P]) Copy() Graph[K, P] {
	out := NewGraph[K, P]()
	for key, node := range g.idToNodes {
		_ = out.store(key, node)
	}
	for source, targets := range g.nodeToTargets {
		for target, rel := range targets {
			_ = out.AddRelationship(source, target, rel)
		}
	}
	for key, vector := range g.vectorIndex.Vectors() {
		_ = out.vectorIndex.Insert(key, vector)
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
		Order: g.Order(),
		Size:  g.Size(),
		Nodes: len(g.idToNodes),
	}
}

// Entities returns every stored node that is an Entity (has a value).
func (g *InMemoryGraph[K, P]) Entities() []*Entity[K] {
	entities := make([]*Entity[K], 0, len(g.idToNodes))
	for _, node := range g.idToNodes {
		if entity, ok := node.(Entity[K]); ok {
			entities = append(entities, &entity)
		}
	}
	return entities
}

// Relationships returns every relationship (edge) currently in the graph.
func (g *InMemoryGraph[K, P]) Relationships() []*Relationship[K] {
	rels := make([]*Relationship[K], 0, g.Size())
	for _, targets := range g.nodeToTargets {
		for _, rel := range targets {
			rels = append(rels, &rel)
		}
	}
	return rels
}

// Returns the graph vector index
func (g *InMemoryGraph[K, P]) GetVectorIndex() index.VectorIndex[K, P] {
	return g.vectorIndex
}

// Returns the graph full text search index
func (g *InMemoryGraph[K, P]) GetTextIndex() index.TextIndex[K] {
	return g.textIndex
}

func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*Node[K], []P) {
	// A. Search starts with gathering seeds for the graph search.
	// Seeds are found from
	// 1. Vector search (top K - default = 10)
	// 2. Matching keywords
	seeds, seedScores := g.gatherSeeds(keywords, vector)

	// B. Walking the graph from all seeds and unioning the found facts
	neighbors, scores := g.findNeighbours(seeds, seedScores, topics, entities, depth)

	// C. Time filtered (since or until)

	filtered, ranked := g.timeFilter(neighbors, scores, since, until)

	// D. Truncate search results
	//
	// filtered and ranked are parallel slices; sort an index permutation
	// by score so both stay aligned, then truncate to top.

	order := make([]int, len(filtered))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		return ranked[order[i]] > ranked[order[j]]
	})

	limit := top
	if limit <= 0 || limit > len(order) {
		limit = len(order)
	}

	nodes := make([]*Node[K], limit)
	scoresOut := make([]P, limit)
	for i := 0; i < limit; i++ {
		nodes[i] = filtered[order[i]]
		scoresOut[i] = ranked[order[i]]
	}

	return nodes, scoresOut
}

// gatherSeeds pools search seeds from the text index (keywords) and the
// vector index (query embedding). Seeds are scored by source rank, 1/(1+rank),
// and a key surfaced by both sources accumulates both scores.
func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[P]) ([]K, map[K]P) {
	scores := make(map[K]P)

	if len(keywords) > 0 {
		// Index errors (empty index) just mean no text seeds.
		if keys, err := g.textIndex.Search(strings.Join(keywords, " ")); err == nil {
			for rank, key := range keys {
				scores[key] += P(1) / P(1+rank)
			}
		}
	}

	if vector.Dim() > 0 {
		if keys, err := g.vectorIndex.Search(vector, defaultSeedSize); err == nil {
			for rank, key := range keys {
				scores[key] += P(1) / P(1+rank)
			}
		}
	}

	seeds := make([]K, 0, len(scores))
	for key := range scores {
		seeds = append(seeds, key)
	}
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
			scores[key] += seedScores[seed] * P(math.Pow(hopAttenuation, float64(hop)))
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

		result, err := g.traversal.Run(g)

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
			for _, neighbors := range []map[K]Relationship[K]{g.nodeToTargets[u], g.nodeToSources[u]} {
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
// when zero — and materialises the survivors alongside their scores.
func (g *InMemoryGraph[K, P]) timeFilter(keys []K, scores map[K]P, since time.Time, until time.Time) ([]*Node[K], []P) {
	nodes := make([]*Node[K], 0, len(keys))
	ranked := make([]P, 0, len(keys))
	for _, key := range keys {
		node, ok := g.idToNodes[key]
		if !ok {
			continue
		}
		ts := node.GetTimestamp()
		if !since.IsZero() && ts.Before(since) {
			continue
		}
		if !until.IsZero() && !ts.Before(until) {
			continue
		}
		nodes = append(nodes, &node)
		ranked = append(ranked, scores[key])
	}
	return nodes, ranked
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
	for source, targets := range other.nodeToTargets {
		for target, rel := range targets {
			_ = g.AddRelationship(source, target, rel)
		}
	}
	for key, vector := range other.vectorIndex.Vectors() {
		_ = g.vectorIndex.Insert(key, vector)
	}
}
