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

	// traversal, ranking and scorer are the pluggable search algorithms (see
	// Traversal, Ranking and Scorer). traversal nil falls back to a built-in
	// breadth-first walk over both edge directions; ranking nil applies no
	// structural boost. scorer folds each candidate's contributions into its
	// relevance score and is never nil — the graph starts with the
	// RRFScorer, because unscored candidates have no rank at all.
	traversal Traversal[K, P]
	ranking   Ranking[K, P]
	scorer    Scorer[K, P]

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

// SetScorer installs the scorer Search folds candidate contributions with.
// nil is ignored rather than stored: unlike its peers, whose nil means "use
// the fallback behaviour", a graph without a scorer cannot rank at all.
func (g *InMemoryGraph[K, P]) SetScorer(s Scorer[K, P]) {
	if s == nil {
		return
	}
	g.scorer = s
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
		scorer: NewRRFScorer[K, P](defaultRRFK),
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

// dropRelationship removes an edge's own node, the counterpart of the store
// call that recorded it. A relationship is never a vertex in the adjacency maps
// and never carries a vector, so idToNodes and the text index are the only
// places it occupies.
func (g *InMemoryGraph[K, P]) dropRelationship(key K) {
	delete(g.idToNodes, key)
	_ = g.textIndex.Delete(key)
}

// Delete removes the node, its incident relationships and its index entries.
// Whichever end of an edge is deleted, the edge leaves as a whole — its node and
// both adjacency entries — because the two halves are one fact about the graph:
// a Mentions left in idToNodes describes an edge that no longer exists (Nodes
// and Stats keep reporting it, and it keeps its text-index entry), while an
// adjacency entry left behind names a relationship node that is no longer
// stored, so Size counts an edge AdjacencyMap cannot resolve.
func (g *InMemoryGraph[K, P]) Delete(node Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	key := node.Key()
	stored, ok := g.idToNodes[key]
	if !ok {
		return ErrNodeNotFound
	}

	// Deleting an endpoint: the adjacency maps hold each edge's own key as their
	// value, so the relationship nodes to prune are exactly what the walk over
	// this node's rows yields.
	for target, edge := range g.nodeToTargets[key] {
		delete(g.nodeToSources[target], key)
		g.dropRelationship(edge)
	}
	for source, edge := range g.nodeToSources[key] {
		delete(g.nodeToTargets[source], key)
		g.dropRelationship(edge)
	}
	delete(g.nodeToTargets, key)
	delete(g.nodeToSources, key)
	delete(g.idToNodes, key)

	// Deleting the edge itself: a relationship is not a vertex, so it owns no
	// rows of its own — the pair of entries store wrote into its endpoints' rows
	// is its whole presence in the graph. The stored node decides this, not the
	// caller's copy, since the key is what the graph was asked to remove.
	if r, isEdge := stored.(Relationship[K]); isEdge {
		source := (*r.Source()).Key()
		target := (*r.Target()).Key()
		delete(g.nodeToTargets[source], target)
		delete(g.nodeToSources[target], source)
	}

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

func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*Node[K], []P, [][]Contribution[P]) {
	// A. Collection: the retrieval stages pool everything they observe about
	// every candidate — text seeds, vector seeds, walk reachability — as
	// Contribution records. No stage computes a final score.
	candidates := g.collect(keywords, vector, topics, entities, depth)

	// B. Scoring: the scorer folds each candidate's contributions into one
	// relevance score, and the installed ranker (if any) boosts the result.
	scores := make(map[K]P, len(candidates))
	keys := make([]K, 0, len(candidates))
	for key, contributions := range candidates {
		scores[key] = g.scorer.Score(contributions)
		keys = append(keys, key)
	}
	g.boost(scores)

	// C. Time filtered (since or until)

	kept, ranked := g.timeFilter(keys, scores, since, until)

	// D. Rank the candidates and truncate to top
	//
	// The candidates arrive in the order the candidate map was iterated in, so
	// the ranking has to be a total order for identical queries to return
	// identical hits: score descending, then fact key. Facts of equal score are
	// ordered by key rather than left as the map presented them, because
	// truncation would otherwise keep an arbitrary subset of a tied group.

	sort.Slice(kept, func(i, j int) bool {
		if ranked[kept[i]] != ranked[kept[j]] {
			return ranked[kept[i]] > ranked[kept[j]]
		}
		return kept[i] < kept[j]
	})

	limit := top
	if limit <= 0 || limit > len(kept) {
		limit = len(kept)
	}

	nodes := make([]*Node[K], limit)
	scoresOut := make([]P, limit)
	contributions := make([][]Contribution[P], limit)
	for i, key := range kept[:limit] {
		node := g.idToNodes[key]
		nodes[i] = &node
		scoresOut[i] = ranked[key]
		contributions[i] = candidates[key]
	}

	logger.Debug("Graph search completed",
		"candidates", len(kept), "returned", limit)
	return nodes, scoresOut, contributions
}

// collect runs the retrieval stages and pools their sightings into one
// candidate map: text and vector seeding, then the walk from every seed, then
// the topic/entity filters. Stages record Contributions and compute no scores
// — ranking policy lives entirely in the Scorer — with one exception: the
// walk stamps each SrcGraph contribution with its seed's fused score, because
// a Contribution names no seed, and that score is the only channel through
// which "reached from a strong seed" can outrank "reached from a weak one".
func (g *InMemoryGraph[K, P]) collect(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int) Candidates[K, P] {
	candidates := make(Candidates[K, P])
	seeds := g.gatherSeeds(keywords, vector, candidates)
	g.findNeighbours(seeds, candidates, topics, entities, depth)
	return candidates
}

// gatherSeeds seeds the candidate pool from the text index (keywords) and the
// vector index (query embedding), appending one Contribution per sighting; a
// key surfaced by both sources holds one from each. Text contributions carry
// the raw match count; vector contributions carry the similarity
// 1/(1+distance), converted here so Contribution.Score is bigger-is-better
// for every source — the index reports distance, where smaller is nearer.
// The seed keys return in ascending key order: scorers fold contribution
// lists as floats, so the walk must append in a deterministic order or the
// low bits of a shared neighbour's score drift between identical queries.
func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[K, P], candidates Candidates[K, P]) []K {
	var textSeeds, vectorSeeds int
	if len(keywords) > 0 {
		// Index errors (empty index) just mean no text seeds.
		if keys, scores, err := g.textIndex.Search(strings.Join(keywords, " "), g.config.DB.SeedSize); err == nil {
			textSeeds = len(keys)
			for rank, key := range keys {
				candidates[key] = append(candidates[key], Contribution[P]{Src: SrcText, Score: scores[rank], Rank: clampRank(rank)})
			}
		} else {
			logger.Debug("Text index yielded no seeds", "error", err)
		}
	}

	if !vector.Empty() {
		if keys, distances, err := g.vectorIndex.Search(vector, g.config.DB.SeedSize); err == nil {
			vectorSeeds = len(keys)
			for rank, key := range keys {
				candidates[key] = append(candidates[key], Contribution[P]{Src: SrcVector, Score: P(1) / (P(1) + distances[rank]), Rank: clampRank(rank)})
			}
		} else {
			logger.Debug("Vector index yielded no seeds", "error", err)
		}
	}

	seeds := make([]K, 0, len(candidates))
	for key := range candidates {
		seeds = append(seeds, key)
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i] < seeds[j] })
	logger.Debug("Gathered search seeds",
		"text", textSeeds, "vector", vectorSeeds, "unique", len(seeds))
	return seeds
}

// findNeighbours expands every seed into its neighbourhood using the graph's
// walker (or the built-in breadth-first walk when none is set), appending a
// SrcGraph contribution for every node a walk reaches: the seed's fused
// score, the node's position in the walk's nearest-first order, and the raw
// hop count. Attenuation is deliberately not applied here — it is a scorer
// parameter. The pooled candidates are then filtered by topics and entities.
func (g *InMemoryGraph[K, P]) findNeighbours(seeds []K, candidates Candidates[K, P], topics []string, entities []string, depth int) {
	// Every seed's fused score is fixed before any walk appends SrcGraph
	// contributions: scoring seeds lazily would fold an earlier seed's graph
	// contribution into a later seed's score, making scores depend on walk
	// order.
	seedScores := make(map[K]P, len(seeds))
	for _, seed := range seeds {
		seedScores[seed] = g.scorer.Score(candidates[seed])
	}

	for _, seed := range seeds {
		walked := g.walk(seed, depth)

		// The walk stores depths in a map, so its discovery order is gone by
		// the time it returns; nearest-first with a key tiebreak reimposes a
		// deterministic order to take ranks from. The seed itself is skipped:
		// its hop-0 entry is already represented by its seeding
		// contributions, and appending it again would double-count the seed
		// under any scorer that sums.
		reached := make([]K, 0, len(walked))
		for key := range walked {
			if key == seed {
				continue
			}
			reached = append(reached, key)
		}
		sort.Slice(reached, func(i, j int) bool {
			if walked[reached[i]] != walked[reached[j]] {
				return walked[reached[i]] < walked[reached[j]]
			}
			return reached[i] < reached[j]
		})

		for rank, key := range reached {
			candidates[key] = append(candidates[key], Contribution[P]{
				Src:   SrcGraph,
				Score: seedScores[seed],
				Rank:  clampRank(rank),
				Hop:   clampHop(walked[key]),
			})
		}
	}

	for key := range candidates {
		if !g.matchesFilter(key, topics) || !g.matchesFilter(key, entities) {
			delete(candidates, key)
		}
	}
}

// boost multiplies each pooled score by the installed ranker's mean-normalised
// global score: an average node is unchanged, central nodes gain, and nodes
// unknown to the ranker (e.g. isolated ones) keep their pooled score. A nil
// ranker boosts nothing.
func (g *InMemoryGraph[K, P]) boost(scores map[K]P) {
	if g.ranking == nil {
		return
	}
	result, err := g.ranking.Run(g)
	r, _ := result.(RankingResult[K, P])
	if err != nil || len(r.Scores) == 0 {
		return
	}
	n := P(len(r.Scores))
	for key := range scores {
		if s, ok := r.Scores[key]; ok {
			scores[key] *= 1 + s*n
		}
	}
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
