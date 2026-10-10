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
	"errors"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
	"github.com/FraiseHQ/fraise/internal/hash"
	"github.com/FraiseHQ/fraise/internal/index"
	"github.com/FraiseHQ/fraise/internal/index/nlp"
	"github.com/FraiseHQ/fraise/internal/index/nlp/stopwords"
	"github.com/FraiseHQ/fraise/internal/index/relevance"
	"github.com/FraiseHQ/fraise/pkg/logger"
	"golang.org/x/text/language"
)

// InMemoryGraph is the in-process implementation of Graph. Nodes live in a
// key-addressed map, relationships in a pair of mirrored adjacency maps, and
// two secondary indices serve hybrid search: a BTree full-text index over fact
// text and an RPTree (random projection forest) vector index over
// caller-provided embeddings.
type InMemoryGraph[K ~uint64, P float32 | float64] struct {
	idToNodes     map[K]Node[K] // all nodes
	nodeToSources map[K]map[K]K // incoming: target -> source -> edge
	nodeToTargets map[K]map[K]K // outgoing: source -> target -> edge

	textIndex   *index.BTreeIndex[K, P]
	vectorIndex *index.RPTreeIndex[K, P]

	// traversal, ranking and scorer are the pluggable search algorithms,
	// installed by db.Start from configuration. A nil traversal turns the
	// graph channel off (text and vector search only) and a nil ranking
	// applies no boost. scorer is never nil: NewGraph installs the
	// ExcessScorer, since candidates cannot be ranked without one.
	traversal Traversal[K, P]
	ranking   Ranking[K, P]
	scorer    scoring.Scorer[K, P]

	hasher hash.Hasher[K, string]

	config *config.ConfigSet

	// sequences caches, per anchor, its member facts in the order they were
	// written: the view contiguous reads a fact's temporal neighbours from.
	// An anchor's sequence is built on first use, under sequenceMu because
	// searches share the graph's read lock, and dropped by every write that
	// touches the anchor or one of its members, so a sequence is never stale
	// and a graph never searched with a window never builds one.
	sequences  map[K]*sequence[K]
	sequenceMu sync.Mutex

	mu sync.RWMutex
}

// sequence is one anchor's member facts ordered by write time, then key, with
// each member's position so a neighbour lookup is a map read and two slice
// reads rather than a search.
type sequence[K comparable] struct {
	keys []K
	at   map[K]int
}

// SetTraversal installs the traversal Search expands seeds with, such as
// ExcessTraversal or BFS.
func (g *InMemoryGraph[K, P]) SetTraversal(t Traversal[K, P]) {
	g.traversal = t
}

// SetRanking installs the global ranking Search boosts scores with, such as
// PageRank.
func (g *InMemoryGraph[K, P]) SetRanking(r Ranking[K, P]) {
	g.ranking = r
}

// SetScorer installs the scorer Search folds candidate contributions with.
// nil is ignored: a nil traversal or ranking switches a stage off, but a
// graph without a scorer cannot rank at all.
func (g *InMemoryGraph[K, P]) SetScorer(s scoring.Scorer[K, P]) {
	if s == nil {
		return
	}
	g.scorer = s
}

// NewGraph returns an empty graph with its indexes, relevance model and
// hasher configured from cfg and the [scoring.ExcessScorer] installed.
// [InMemoryGraph.SetTraversal] and [InMemoryGraph.SetRanking] install the
// traversal and ranking.
func NewGraph[K ~uint64, P float32 | float64](cfg *config.ConfigSet) *InMemoryGraph[K, P] {
	// The tokenizer and relevance model must be installed before the first
	// insert. Stemming lets a keyword find other inflections of the same
	// word. BM25 is configured by default because the excess scorer needs its
	// raw retrieval mass; "matchcount", the index's own default, stays
	// selectable for comparison runs.
	textIndex := index.NewBTreeIndex[K, P](comparator.OrderedComparator[K])
	textIndex.SetTokenizer(nlp.StemmingTokenizer{})
	if cfg.DB.RelevanceModel.Name == config.RelevanceBM25 {
		textIndex.SetRelevance(relevance.NewBM25[K, P]())
	}

	g := &InMemoryGraph[K, P]{
		idToNodes:     make(map[K]Node[K]),
		nodeToSources: make(map[K]map[K]K),
		nodeToTargets: make(map[K]map[K]K),
		textIndex:     textIndex,
		vectorIndex: index.NewRPTreeIndex[K, P](
			0,
			cfg.DB.VectorSearch.ProjectionDimension,
			cfg.DB.VectorSearch.NumberTrees,
			cfg.DB.VectorSearch.Seed,
			cfg.DB.VectorSearch.FlushFactor,
			cfg.DB.VectorSearch.LeafSize,
			cfg.DB.VectorSearch.Overfetch,
			comparator.OrderedComparator[K],
		),
		scorer:    scoring.NewExcessScorer[K, P](),
		hasher:    hash.NewHasher[K](cfg),
		config:    cfg,
		sequences: make(map[K]*sequence[K]),
	}
	// The window needs the graph's notion of adjacency, which only exists
	// once the graph does, so it is installed last. A non-positive gamma
	// leaves the index on the plain search.
	if cfg.DB.WindowGamma > 0 {
		textIndex.SetWindow(P(cfg.DB.WindowGamma), g.contiguous)
	}
	return g
}

// Lock acquires the graph's write lock.
func (g *InMemoryGraph[K, P]) Lock() {
	g.mu.Lock()
}

// RLock acquires the graph's read lock.
func (g *InMemoryGraph[K, P]) RLock() {
	g.mu.RLock()
}

// Unlock releases the graph's write lock.
func (g *InMemoryGraph[K, P]) Unlock() {
	g.mu.Unlock()
}

// RUnlock releases the graph's read lock.
func (g *InMemoryGraph[K, P]) RUnlock() {
	g.mu.RUnlock()
}

// GetHasher returns the hasher the graph derives node keys with.
func (g *InMemoryGraph[K, P]) GetHasher() hash.Hasher[K, string] {
	return g.hasher
}

// Get returns the node stored under key, or nil if absent.
func (g *InMemoryGraph[K, P]) Get(key K) Node[K] {
	node, ok := g.Nodes()[key]
	if !ok {
		return nil
	}
	return node
}

// Set inserts a new node under its own key, returning ErrNodeAlreadyExists if
// the key is taken.
func (g *InMemoryGraph[K, P]) Set(node Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	n := node
	if _, exists := g.Nodes()[n.Key()]; exists {
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

// textLanguage is the language whose stop words are removed from text on both
// sides of the text index: a fact's value in store and the query keywords in
// gatherSeeds. Both sides must use the same language, or a stop word kept on
// one side leaves a stem that collides with a content word's ("own" with
// "owns"). English is the only language supported for now.
var textLanguage = language.English

// store records the node and indexes a fact's value in the text index. Only
// facts are indexed. Relationships carry no text, and as empty documents they
// would skew the corpus statistics (document count, average length) every
// BM25 score is built from. Anchors (Topic, NamedEntity) are left out because
// a one- or two-word name is the document BM25's length norm favours most: an
// anchor named like the query term would take the top seed slots while
// transmitting nothing (its neighbours are facts, not anchors) and never being
// returned as a hit.
//
// A fact the text index already holds under key, re-asserted through Put, is
// replaced through Update; a new one is added through Insert.
func (g *InMemoryGraph[K, P]) store(key K, node Node[K]) error {
	g.idToNodes[key] = node
	g.dropSequences(key)

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
		// The edge files source under target, so target's sequence has a
		// new member.
		g.dropSequences(source)
	}

	_, isFact := node.(Fact[K])
	if attrs := node.GetAttributes(); isFact && attrs != nil && attrs.Value != "" {

		text := g.GetTextIndex()
		write := text.Insert
		if _, err := text.Retrieve(key); err == nil {
			write = text.Update
		}
		if err := write(key, stopwords.CleanContent(attrs.Value, textLanguage)); err != nil {
			logger.Warn("Failed to index node text", "error", err)
			return err
		}
	}
	return nil
}

// dropRelationship removes a relationship's own node. The caller removes its
// adjacency entries; a relationship is not a vertex and has no vector.
func (g *InMemoryGraph[K, P]) dropRelationship(key K) {
	delete(g.idToNodes, key)
}

// unlink removes the edge source -> target from both adjacency views and drops
// any row it leaves empty. A row's presence is what makes a node a vertex to
// anything that enumerates the views (PageRank collects its vertices from
// them), so a node whose last edge goes must leave both views with it, or it
// is ranked as a dangling vertex instead of left out as isolated.
func (g *InMemoryGraph[K, P]) unlink(source, target K) {
	delete(g.nodeToTargets[source], target)
	if len(g.nodeToTargets[source]) == 0 {
		delete(g.nodeToTargets, source)
	}
	delete(g.nodeToSources[target], source)
	if len(g.nodeToSources[target]) == 0 {
		delete(g.nodeToSources, target)
	}
}

// Delete removes the node, its index entries and its incident relationships.
// Deleting either endpoint of an edge, or the relationship itself, removes the
// edge as a whole (its node and both adjacency entries), so Nodes, Size and
// AdjacencyMap never disagree about which edges exist. A node left with no
// edges has no row in either view, so it is isolated rather than dangling.
func (g *InMemoryGraph[K, P]) Delete(node Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	key := node.Key()
	stored, ok := g.Nodes()[key]
	if !ok {
		return ErrNodeNotFound
	}
	g.dropSequences(key)
	if r, isEdge := stored.(Relationship[K]); isEdge {
		g.dropSequences((*r.Source()).Key())
	}

	// Deleting an endpoint: its adjacency rows hold each incident edge's key,
	// so walking them finds the relationship nodes to drop.
	for target, edge := range g.nodeToTargets[key] {
		g.unlink(key, target)
		g.dropRelationship(edge)
	}
	for source, edge := range g.nodeToSources[key] {
		g.unlink(source, key)
		g.dropRelationship(edge)
	}
	delete(g.idToNodes, key)

	// Deleting a relationship: it has no rows of its own, only the two entries
	// store wrote into its endpoints' rows. The stored node is inspected rather
	// than the argument, since the key is what identifies the node to remove.
	if r, isEdge := stored.(Relationship[K]); isEdge {
		source := (*r.Source()).Key()
		target := (*r.Target()).Key()
		g.unlink(source, target)
	}

	// The node may legitimately be absent from either index.
	_ = g.GetTextIndex().Delete(key)
	_ = g.vectorIndex.Delete(key)
	return nil
}

// Nodes returns the live node map. Every read of the graph's nodes goes
// through it; only store, Delete and dropRelationship write the map itself.
func (g *InMemoryGraph[K, P]) Nodes() map[K]Node[K] {
	return g.idToNodes
}

// AdjacencyMap implements [Graph]: it returns a deep copy of the outgoing
// edges, so a caller holding it cannot corrupt the graph's own rows.
func (g *InMemoryGraph[K, P]) AdjacencyMap() map[K]map[K]K {
	return exportEdges(g.nodeToTargets)
}

// PredecessorMap implements [Graph]: it returns a deep copy of the incoming
// edges, so a caller holding it cannot corrupt the graph's own rows.
func (g *InMemoryGraph[K, P]) PredecessorMap() map[K]map[K]K {
	return exportEdges(g.nodeToSources)
}

// Neighbours returns the keys adjacent to key in either direction, allocating
// a single slice of the node's degree.
func (g *InMemoryGraph[K, P]) Neighbours(key K) []K {
	out := make([]K, 0, len(g.nodeToTargets[key])+len(g.nodeToSources[key]))
	for neighbour := range g.nodeToTargets[key] {
		out = append(out, neighbour)
	}
	for neighbour := range g.nodeToSources[key] {
		out = append(out, neighbour)
	}
	return out
}

// contiguous returns the facts written immediately before and after key under
// each anchor it is filed under, in ascending key order, each once. It is the
// neighbours function the text index's window scores with (see
// [index.BTreeIndex.SetWindow]): two facts adjacent in time under a shared
// anchor are one exchange, a question and its answer, a statement and its
// follow-up, and the window lets the exchange match as a whole. A fact filed
// under no anchor has no neighbours. Searches call it under the graph's read
// lock.
func (g *InMemoryGraph[K, P]) contiguous(key K) []K {
	var out []K
	for anchor := range g.nodeToTargets[key] {
		if !isAnchor[K, P](g, anchor) {
			continue
		}
		seq := g.sequence(anchor)
		i, ok := seq.at[key]
		if !ok {
			continue
		}
		if i > 0 {
			out = append(out, seq.keys[i-1])
		}
		if i+1 < len(seq.keys) {
			out = append(out, seq.keys[i+1])
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// sequence returns anchor's members in write order, building and caching it
// on first use. Members are the facts adjacent to the anchor; ties on the
// timestamp (a batch written in one instant) are broken by key, so the order
// is total and two searches agree on who neighbours whom.
func (g *InMemoryGraph[K, P]) sequence(anchor K) *sequence[K] {
	g.sequenceMu.Lock()
	defer g.sequenceMu.Unlock()
	if seq, ok := g.sequences[anchor]; ok {
		return seq
	}
	seq := &sequence[K]{at: make(map[K]int, len(g.nodeToSources[anchor]))}
	for member := range g.nodeToSources[anchor] {
		if _, isFact := g.Nodes()[member].(Fact[K]); isFact {
			seq.keys = append(seq.keys, member)
		}
	}
	slices.SortFunc(seq.keys, func(a, b K) int {
		if c := g.Nodes()[a].GetTimestamp().Compare(g.Nodes()[b].GetTimestamp()); c != 0 {
			return c
		}
		return comparator.OrderedComparator(a, b)
	})
	for i, member := range seq.keys {
		seq.at[member] = i
	}
	g.sequences[anchor] = seq
	return seq
}

// dropSequences discards the cached sequence of key and of every node
// adjacent to it. A write to a fact moves it in each of its anchors'
// sequences (a re-asserted fact takes a new timestamp), a new edge adds a
// member, a deleted node removes one: all three reach here, under the write
// lock, so a cached sequence never outlives the writes that would change it.
func (g *InMemoryGraph[K, P]) dropSequences(key K) {
	g.sequenceMu.Lock()
	defer g.sequenceMu.Unlock()
	delete(g.sequences, key)
	for neighbour := range g.nodeToTargets[key] {
		delete(g.sequences, neighbour)
	}
	for neighbour := range g.nodeToSources[key] {
		delete(g.sequences, neighbour)
	}
}

// exportEdges returns a deep copy of an edge map, so a caller cannot mutate
// the graph's own rows.
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

// Order returns the number of vertices in the graph: facts, entities and
// topics. Relationships are stored as nodes and satisfy Entity, so they are
// excluded explicitly; they are edges, which Size counts.
func (g *InMemoryGraph[K, P]) Order() int {
	order := 0
	for _, node := range g.Nodes() {
		if _, isEdge := node.(Relationship[K]); isEdge {
			continue
		}
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

// Stats implements [Graph]: it snapshots the vertex, edge, node and vector
// counts, plus the forest entries whose excess over Vectors is the vector
// index's pending compaction.
func (g *InMemoryGraph[K, P]) Stats() GraphStats {
	return GraphStats{
		Order:   g.Order(),
		Size:    g.Size(),
		Nodes:   len(g.Nodes()),
		Vectors: g.GetVectorIndex().Count(),
		// Entries - Count is the vector index's compaction debt; the index's
		// automatic Flush keeps it bounded (see rptree flush-factor).
		ForestEntries: g.GetVectorIndex().Entries(),
	}
}

// GetVectorIndex returns the graph's vector index.
func (g *InMemoryGraph[K, P]) GetVectorIndex() index.VectorIndex[K, P] {
	return g.vectorIndex
}

// GetTextIndex returns the graph's full-text index.
func (g *InMemoryGraph[K, P]) GetTextIndex() index.TextIndex[K, P] {
	return g.textIndex
}

// Search implements [Graph.Search]. It collects candidates, folds each with the
// installed scorer and boosts by the installed ranking, applies the time window
// and recency decay, keeps the top hits (score descending, then key ascending,
// so identical queries return identical hits), drops hits below the
// db.min-score-ratio cutoff and, under db.aggregate, spends the top slots
// across the facets a flat ranking spans rather than down the ranking.
func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) (Result[K, P], error) {
	// Aggregation reads a pool deeper than top, since the facets it spreads
	// the slots over, and the members a group holds, lie below what top
	// slots would keep, and the indexes have to be read that deep for the
	// pool to exist.
	aggregating := g.config.DB.Aggregate.Name != config.AggregateNone && top > 0
	pool := top
	if aggregating && g.config.DB.Aggregate.Pool > pool {
		pool = g.config.DB.Aggregate.Pool
	}

	// A. Collection: every observation of every candidate (text and vector
	// seeds, anchor transmission, or the named anchors' members when they
	// seed alone) as Contributions, and the query's background rate.
	candidates, background, err := g.collect(keywords, vector, topics, entities, depth, top, pool)
	if err != nil {
		return Result[K, P]{}, err
	}

	// B. Scoring: the scorer folds each candidate's contributions into one
	// relevance score given the background, and the installed ranker (if any)
	// boosts the result.
	scores := make(map[K]P, len(candidates))
	keys := make([]K, 0, len(candidates))
	scorer := g.scorer.WithBackground(background)
	for key, contributions := range candidates {
		scores[key] = scorer.Score(contributions)
		keys = append(keys, key)
	}
	g.boost(scores)

	// C. Time window and recency decay.
	kept, ranked := g.timeFilter(keys, scores, since, until)

	// D. Rank and truncate to the pool. Map iteration order is random, so
	// the order must be total for identical queries to return identical
	// hits: score descending, then key ascending. Without the key tie-break,
	// truncation would keep an arbitrary subset of a tied group. TopK costs
	// O(n log pool) rather than the O(n log n) of a full sort.
	ranker := containers.NewTopK[K, P](pool, comparator.OrderedComparator[K])
	for _, key := range kept {
		ranker.Offer(key, ranked[key])
	}
	rankedKeys, rankedScores := ranker.Drain()

	// E. Score cutoff (db.min-score-ratio)
	rankedKeys, rankedScores = g.scoreCutoff(rankedKeys, rankedScores, scorer, candidates)

	// F. Aggregation (db.aggregate), then the answer.
	result := Result[K, P]{Background: background}
	if aggregating {
		result.Hits, result.Spread = g.aggregate(rankedKeys, rankedScores, candidates, keywords, topics, entities, top)
	} else {
		result.Hits = g.hits(rankedKeys, rankedScores, candidates)
	}

	logger.Debug("Graph search completed",
		"candidates", len(kept), "returned", len(result.Hits), "spread", result.Spread)
	return result, nil
}

// hits turns a ranking into fact hits, each carrying the contributions its
// score was folded from.
func (g *InMemoryGraph[K, P]) hits(keys []K, scores []P, candidates scoring.Candidates[K, P]) []Hit[K, P] {
	out := make([]Hit[K, P], len(keys))
	for i, key := range keys {
		node := g.Nodes()[key]
		out[i] = Hit[K, P]{Node: &node, Score: scores[i], Contributions: candidates[key]}
	}
	return out
}

// spreadWindow is how deep into the ranking the aggregation gate looks when
// it measures spread. Twenty is two slots of the default top: enough to tell a
// ranking that peaks on one or two facets from one that is flat across many,
// without reading the whole pool. It is a methodology constant, not
// configuration.
const spreadWindow = 20

// aggregate draws at most top hits from the ranked pool under db.aggregate.
// It first measures the spread: how many distinct facets the first
// spreadWindow candidates scoring at least ratio times the best belong to. A
// facet is the finest anchor a candidate is filed under beyond the anchors the
// query named (those are shared by every candidate and say nothing), or the
// candidate itself when it has none: on a graph filed by session, the spread
// is the number of sessions near the top. Below the configured spread the
// ranking is a specific question, peaked on one context, and is returned as
// it is, truncated to top.
//
// At or above it the question is broad, and ten slots down a ranking that
// leans on one context would hold that context ten times over: the slots are
// spread over the facets instead (see spread), so the answer covers as many
// contexts as the pool offers before it deepens any. Under the group
// algorithm the pool is also folded into group hits (see groups), each placed
// at the first of its members the spread order reaches and standing in for
// all of them. The spread is returned for explain whether or not the slots
// were spread.
func (g *InMemoryGraph[K, P]) aggregate(keys []K, scores []P, candidates scoring.Candidates[K, P], keywords []string, topics []string, entities []string, top int) ([]Hit[K, P], int) {
	cfg := g.config.DB.Aggregate
	topicKeys, entityKeys := g.anchorKeys(topics, entities)
	named := make(map[K]struct{}, len(topicKeys)+len(entityKeys))
	for _, anchor := range slices.Concat(topicKeys, entityKeys) {
		named[anchor] = struct{}{}
	}

	spread := 0
	if len(keys) > 0 {
		facets := make(map[K]struct{}, spreadWindow)
		bar := P(cfg.Ratio) * scores[0]
		for i := 0; i < len(keys) && i < spreadWindow; i++ {
			if scores[i] < bar {
				continue
			}
			facets[g.facet(keys[i], named)] = struct{}{}
		}
		spread = len(facets)
	}
	if spread < cfg.Spread {
		if len(keys) > top {
			keys, scores = keys[:top], scores[:top]
		}
		return g.hits(keys, scores, candidates), spread
	}

	// Groups are folded from the ranking, so their members come best first,
	// and placed along the spread order, so a group stands where the spread
	// would have put its first member.
	var groups []Hit[K, P]
	var grouped map[K]int
	if cfg.Name == config.AggregateGroup && len(keywords) > 0 {
		groups, grouped = g.groups(keys, scores, candidates, keywords)
	}
	keys, scores = g.spread(keys, scores, named)

	hits := make([]Hit[K, P], 0, top)
	placed := make([]bool, len(groups))
	for i, key := range keys {
		if len(hits) >= top {
			break
		}
		if j, ok := grouped[key]; ok {
			if !placed[j] {
				placed[j] = true
				hits = append(hits, groups[j])
			}
			continue
		}
		node := g.Nodes()[key]
		hits = append(hits, Hit[K, P]{Node: &node, Score: scores[i], Contributions: candidates[key]})
	}
	return hits, spread
}

// spread reorders a ranking so that every facet gets its best cap candidates
// before any facet gets more: a walk down the ranking takes a candidate whose
// facet has fewer than cap taken and defers the rest, and the deferred follow
// in rank order. The first candidate is always taken, so the best hit is the
// best hit whether or not the slots were spread; a ranking with fewer facets
// than slots is still filled; a ranking on a single facet comes back as it
// was. The order within a facet is the ranking's, so what is deferred is
// each facet's weaker matches, never its best.
func (g *InMemoryGraph[K, P]) spread(keys []K, scores []P, named map[K]struct{}) ([]K, []P) {
	taken := make(map[K]int)
	spreadKeys := make([]K, 0, len(keys))
	spreadScores := make([]P, 0, len(keys))
	var deferredKeys []K
	var deferredScores []P
	for i, key := range keys {
		facet := g.facet(key, named)
		if taken[facet] >= g.config.DB.Aggregate.Cap {
			deferredKeys = append(deferredKeys, key)
			deferredScores = append(deferredScores, scores[i])
			continue
		}
		taken[facet]++
		spreadKeys = append(spreadKeys, key)
		spreadScores = append(spreadScores, scores[i])
	}
	return append(spreadKeys, deferredKeys...), append(spreadScores, deferredScores...)
}

// groups folds the first pool candidates of a ranking into group hits. The
// candidates are keyed by the rarest query term each holds (see
// [index.BTreeIndex.KeyTerms]): the candidates that matched the query by the
// same rare term are instances of one thing the query asked about, and every
// term with at least min-size of them becomes a group hit, its members best
// first, its node, score and contributions its best member's. The largest
// groups win, up to max-groups, ties broken by term so the fold is
// deterministic. The groups come back with the index of the group each
// grouped key belongs to, so the caller can place a group where it meets a
// member and skip the others.
func (g *InMemoryGraph[K, P]) groups(keys []K, scores []P, candidates scoring.Candidates[K, P], keywords []string) ([]Hit[K, P], map[K]int) {
	cfg := g.config.DB.Aggregate
	pool := keys
	if len(pool) > cfg.Pool {
		pool = pool[:cfg.Pool]
	}
	terms := g.textIndex.KeyTerms(stopwords.CleanContent(strings.Join(keywords, " "), textLanguage), pool)
	members := make(map[string][]int)
	order := make([]string, 0)
	for i, key := range pool {
		term, ok := terms[key]
		if !ok {
			continue
		}
		if _, seen := members[term]; !seen {
			order = append(order, term)
		}
		members[term] = append(members[term], i)
	}
	folded := make([]string, 0, len(order))
	for _, term := range order {
		if len(members[term]) >= cfg.MinSize {
			folded = append(folded, term)
		}
	}
	sort.SliceStable(folded, func(a, b int) bool {
		if len(members[folded[a]]) != len(members[folded[b]]) {
			return len(members[folded[a]]) > len(members[folded[b]])
		}
		return folded[a] < folded[b]
	})
	if len(folded) > cfg.MaxGroups {
		folded = folded[:cfg.MaxGroups]
	}

	groups := make([]Hit[K, P], 0, len(folded))
	grouped := make(map[K]int)
	for j, term := range folded {
		hit := Hit[K, P]{Key: term}
		for _, i := range members[term] {
			node := g.Nodes()[keys[i]]
			hit.Members = append(hit.Members, &node)
			hit.MemberScores = append(hit.MemberScores, scores[i])
			grouped[keys[i]] = j
		}
		hit.Node, hit.Score, hit.Contributions = hit.Members[0], hit.MemberScores[0], candidates[keys[members[term][0]]]
		groups = append(groups, hit)
	}
	return groups, grouped
}

// facet is the finest anchor key is filed under beyond the named ones: the
// adjacent topic or entity with the fewest members, ties broken by key, so a
// session anchor wins over the conversation it sits in and the person it
// mentions. A fact under no other anchor is its own facet.
func (g *InMemoryGraph[K, P]) facet(key K, named map[K]struct{}) K {
	facet, degree := key, -1
	for anchor := range g.nodeToTargets[key] {
		if _, isNamed := named[anchor]; isNamed || !isAnchor[K, P](g, anchor) {
			continue
		}
		d := len(g.nodeToSources[anchor])
		if degree == -1 || d < degree || (d == degree && anchor < facet) {
			facet, degree = anchor, d
		}
	}
	return facet
}

// scoreCutoff applies db.min-score-ratio to a best-first ranking: it keeps
// each hit whose relevance (the scorer's output, before boost and decay) is at
// least MinScoreRatio times the best relevance in the list. Relevance is
// refolded from the contributions of the ranked hits only; the scorer is pure,
// so this reproduces stage B's values in O(top). Every hit is tested, not a
// prefix, because a decay-ordered list is not sorted by relevance. Kept hits
// keep their order and scores, the best hit always survives (the ratio is at
// most 1), and a zero ratio is a no-op.
func (g *InMemoryGraph[K, P]) scoreCutoff(keys []K, scores []P, scorer scoring.Scorer[K, P], candidates scoring.Candidates[K, P]) ([]K, []P) {
	ratio := g.config.DB.MinScoreRatio
	if ratio <= 0 || len(keys) == 0 {
		return keys, scores
	}
	relevance := make([]P, len(keys))
	for i, key := range keys {
		relevance[i] = scorer.Score(candidates[key])
	}
	bar := P(ratio) * slices.Max(relevance)
	keep := 0
	for i := range keys {
		if relevance[i] >= bar {
			keys[keep], scores[keep] = keys[i], scores[i]
			keep++
		}
	}
	return keys[:keep], scores[:keep]
}

// collect runs the retrieval stages and pools their observations into one
// candidate map: text and vector seeding, then the traversal from every seed,
// then the topic and entity filters. It also returns the background rate the
// scorer needs. Stages only record Contributions; the hinge, the null model
// and the attenuation belong to the Scorer. The error is a vector dimension
// mismatch.
//
// top is how many hits the recall asks for and pool how many candidates the
// ranking will consider, top itself or deeper under db.aggregate: each index
// is read pool deep, and the traversal expands from the first max(seed-size,
// top) of each reading, so an aggregation that reads deeper does not
// traverse deeper (see gatherSeeds).
//
// With no keywords and no vector, the named anchors' members are the seeds
// instead (see Graph.Search). No traversal runs, since expanding from every
// member would return most of the graph, and the background is zero.
//
// Topic and entity names are resolved to anchor keys once per query, so
// gatherMembers and findNeighbours agree on which node a name denotes and the
// filter is a key lookup.
func (g *InMemoryGraph[K, P]) collect(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, pool int) (scoring.Candidates[K, P], P, error) {
	topicKeys, entityKeys := g.anchorKeys(topics, entities)
	candidates := make(scoring.Candidates[K, P])
	if len(keywords) == 0 && vector.Empty() {
		g.gatherMembers(topicKeys, entityKeys, candidates)
		return candidates, 0, nil
	}
	seeds, err := g.gatherSeeds(keywords, vector, candidates, top, pool)
	if err != nil {
		return nil, 0, err
	}
	background := g.findNeighbours(seeds, candidates, topicKeys, entityKeys, depth)
	return candidates, background, nil
}

// anchorKeys resolves topic and entity names to the keys of their anchor
// nodes, in query order. A topic and an entity with the same name are
// different anchors. A name nothing is filed under resolves to a key no node
// holds, so it seeds nothing and, as a filter, excludes everything.
func (g *InMemoryGraph[K, P]) anchorKeys(topics []string, entities []string) (topicKeys []K, entityKeys []K) {
	topicKeys = make([]K, 0, len(topics))
	for _, value := range topics {
		topicKeys = append(topicKeys, Topic[K]{NodeAttributes: NodeAttributes{Value: value}, Hasher: g.hasher}.Key())
	}
	entityKeys = make([]K, 0, len(entities))
	for _, value := range entities {
		entityKeys = append(entityKeys, NamedEntity[K]{NodeAttributes: NodeAttributes{Value: value}, Hasher: g.hasher}.Key())
	}
	return topicKeys, entityKeys
}

// gatherMembers seeds the candidate pool with every fact filed under a named
// anchor. Each fact gets one SrcAnchor contribution of unit mass per named
// anchor it is filed under, so a fact under two of them starts with twice the
// mass of a fact under one. Anchors are visited once each in query order
// (topics, then entities), so contributions are appended in a fixed order for
// the scorer's fold.
func (g *InMemoryGraph[K, P]) gatherMembers(topicKeys []K, entityKeys []K, candidates scoring.Candidates[K, P]) {
	anchors := make([]K, 0, len(topicKeys)+len(entityKeys))
	named := make(map[K]struct{}, len(topicKeys)+len(entityKeys))
	for _, anchor := range slices.Concat(topicKeys, entityKeys) {
		if _, dup := named[anchor]; dup {
			continue
		}
		named[anchor] = struct{}{}
		anchors = append(anchors, anchor)
	}

	var members int
	for _, anchor := range anchors {
		degree := scoring.ClampDegree(len(g.nodeToTargets[anchor]) + len(g.nodeToSources[anchor]))
		for _, member := range g.Neighbours(anchor) {
			if _, isFact := g.Nodes()[member].(Fact[K]); !isFact {
				continue
			}
			candidates[member] = append(candidates[member], scoring.Contribution[K, P]{
				Src:    scoring.SrcAnchor,
				Score:  1,
				Via:    anchor,
				Degree: degree,
				Count:  1,
			})
			members++
		}
	}
	logger.Debug("Gathered anchor members", "anchors", len(anchors), "members", members)
}

// gatherSeeds seeds the candidate pool from the text index (keywords) and the
// vector index (embedding), appending one Contribution per hit; a key found by
// both gets one from each. Keywords are cleaned of stop words exactly as store
// cleans a fact's text, so both sides of the index share one vocabulary. Text
// contributions carry the BM25 × coverage mass and vector contributions the
// similarity 1/(1+distance), so Score is bigger-is-better for every source.
//
// Each source is asked for pool candidates, at least max(seed-size, top), so
// the text ranking is never cut off before top, and every candidate read is
// pooled on its own evidence. The seeds, the keys the traversal expands from,
// are the first max(seed-size, top) of each source's reading: what an
// aggregation reads below that stands on its text or vector match alone, so
// reading deeper for it costs index lookups, not traversals. The seed keys
// are returned in ascending order: the traversal folds floats, and a fixed
// order keeps identical queries scoring identically. An empty index seeds
// nothing; a vector of the wrong dimension is an error, since it comes from
// a different embedding model than the graph's.
func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[K, P], candidates scoring.Candidates[K, P], top int, pool int) ([]K, error) {
	seedK := g.config.DB.SeedSize
	if top > seedK {
		seedK = top
	}
	if seedK > pool {
		pool = seedK
	}
	seeded := make(map[K]struct{}, seedK)

	var textSeeds, vectorSeeds int
	if len(keywords) > 0 {
		// Index errors (empty index) just mean no text seeds.
		if keys, scores, err := g.GetTextIndex().Search(stopwords.CleanContent(strings.Join(keywords, " "), textLanguage), pool); err == nil {
			textSeeds = len(keys)
			for rank, key := range keys {
				candidates[key] = append(candidates[key], scoring.Contribution[K, P]{Src: scoring.SrcText, Score: scores[rank], Rank: scoring.ClampRank(rank), Count: 1})
				if rank < seedK {
					seeded[key] = struct{}{}
				}
			}
		} else {
			logger.Debug("Text index yielded no seeds", "error", err)
		}
	}

	if !vector.Empty() {
		keys, distances, err := g.vectorIndex.Search(vector, pool)
		switch {
		case errors.Is(err, index.ErrInvalidDimension):
			return nil, err
		case err != nil:
			logger.Debug("Vector index yielded no seeds", "error", err)
		default:
			vectorSeeds = len(keys)
			for rank, key := range keys {
				candidates[key] = append(candidates[key], scoring.Contribution[K, P]{Src: scoring.SrcVector, Score: P(1) / (P(1) + distances[rank]), Rank: scoring.ClampRank(rank), Count: 1})
				if rank < seedK {
					seeded[key] = struct{}{}
				}
			}
		}
	}

	seeds := make([]K, 0, len(seeded))
	for key := range seeded {
		seeds = append(seeds, key)
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i] < seeds[j] })
	logger.Debug("Gathered search seeds",
		"text", textSeeds, "vector", vectorSeeds, "unique", len(seeds))
	return seeds, nil
}

// depthOneAdmission multiplies an anchor's fair share at depth 1: the anchor
// transmits only when its observed mass exceeds this many times its fair
// share, trading recall for precision. depth 2 admits at the fair share
// itself. It is a methodology constant, not configuration.
const depthOneAdmission = 2

// findNeighbours runs the traversal from every seed, pools what it observes
// into candidates, applies the topic and entity filters, and returns the
// background rate.
//
// Pass 1 observes. Each seed's mass, the scorer's fold of its own
// contributions, is added to every anchor the traversal reaches at depth 1,
// along with the anchor's degree and how many seeds funded it. The background
// rate is the total observed mass over the total degree of every touched
// anchor, silent ones included.
//
// Pass 2 expands. An anchor whose mass exceeds its fair share (degree ×
// background, raised by depthOneAdmission at depth 1) appends one SrcGraph
// contribution per member, carrying its full observed mass, identity, degree
// and seed count. An anchor at or below its fair share transmits nothing (hub
// silence), so pass 2 skips it without visiting its members. The hinge, the
// fair-share subtraction and the attenuation are left to the Scorer.
//
// depth 0 skips both passes, so only seed mass scores. The filters apply in
// every lane.
func (g *InMemoryGraph[K, P]) findNeighbours(seeds []K, candidates scoring.Candidates[K, P], topicKeys []K, entityKeys []K, depth int) P {
	// The traversal runs at depth 1 or 2, and only when the query names a
	// topic or entity: a recall naming neither is a text and vector search
	// whatever its depth, and the parser warns when it asks for more. There
	// is a single round; a second would re-observe the first round's mass
	// through sibling anchors and collapse recall, which is why depth stops
	// at 2.
	var background P
	if depth >= 1 && g.traversal != nil && len(seeds) > 0 && (len(topicKeys) > 0 || len(entityKeys) > 0) {
		// Fix every seed's mass before any SrcGraph contribution is
		// appended, so scores cannot depend on traversal order. The scorer
		// is unbound: nothing has been observed yet, so there is no
		// background.
		seedScores := make(map[K]P, len(seeds))
		for _, seed := range seeds {
			seedScores[seed] = g.scorer.Score(candidates[seed])
		}

		// Pass 1 (observe): each anchor's mass, seed count and degree, and
		// its members, recorded from the first traversal that touches it.
		anchorMass := make(map[K]P)
		anchorSeeds := make(map[K]int)
		degree := make(map[K]int)
		members := make(map[K][]K)
		touched := make([]K, 0)
		for _, seed := range seeds {
			t := g.traversal.Clone()
			t.SetSource(seed)
			result, err := t.Run(g)
			r, ok := result.(TraversalResult[K])
			if err != nil || !ok {
				continue
			}
			// ExcessTraversal reports an anchor's full membership, the seed
			// included, so the members are the same from every seed and are
			// recorded on first touch; the anchor's mass accumulates over
			// every seed that touches it.
			newAnchors := make(map[K]struct{})
			for _, vertex := range r.Order {
				// Only anchors observe mass. BFS follows every edge, so it can
				// reach other kinds of node at depth 1.
				if r.Depth[vertex] != 1 || !isAnchor[K, P](g, vertex) {
					continue
				}
				anchor := vertex
				if _, seen := degree[anchor]; !seen {
					degree[anchor] = len(g.nodeToTargets[anchor]) + len(g.nodeToSources[anchor])
					touched = append(touched, anchor)
					newAnchors[anchor] = struct{}{}
				}
				anchorMass[anchor] += seedScores[seed]
				anchorSeeds[anchor]++
			}
			for _, vertex := range r.Order {
				if r.Depth[vertex] != 2 {
					continue
				}
				// Use the full incidence when the traversal provides it
				// (ExcessTraversal's Parents). A tree-shaped traversal such as
				// BFS has only Parent, so a member is observed through the
				// one anchor that discovered it.
				parents := r.Parents[vertex]
				if len(parents) == 0 {
					if parent, ok := r.Parent[vertex]; ok {
						parents = []K{parent}
					}
				}
				for _, anchor := range parents {
					if _, isNew := newAnchors[anchor]; isNew {
						members[anchor] = append(members[anchor], vertex)
					}
				}
			}
		}
		sort.Slice(touched, func(i, j int) bool { return touched[i] < touched[j] })

		var totalMass P
		var totalDegree int
		for _, anchor := range touched {
			totalMass += anchorMass[anchor]
			totalDegree += degree[anchor]
		}
		if totalDegree > 0 {
			background = totalMass / P(totalDegree)
		}

		// Pass 2 (expand): anchors above their admitted share transmit. The
		// test compares M_A·Σd with d_A·ΣM rather than M_A with d_A·ρ₀,
		// because the division rounds: an anchor holding exactly its share,
		// such as the only anchor a query reaches, must stay silent, and a
		// rounding error could let it clear the bar.
		admission := P(1)
		if depth == 1 {
			admission = depthOneAdmission
		}
		for _, anchor := range touched {
			if anchorMass[anchor]*P(totalDegree) <= P(degree[anchor])*totalMass*admission {
				continue
			}
			for _, member := range members[anchor] {
				candidates[member] = append(candidates[member], scoring.Contribution[K, P]{
					Src:    scoring.SrcGraph,
					Score:  anchorMass[anchor],
					Via:    anchor,
					Degree: scoring.ClampDegree(degree[anchor]),
					Count:  scoring.ClampCount(anchorSeeds[anchor]),
				})
			}
		}
	}

	for key := range candidates {
		if !g.matchesFilter(key, topicKeys) || !g.matchesFilter(key, entityKeys) {
			delete(candidates, key)
		}
	}
	return background
}

// boost multiplies each score by 1 + n·s, where s is the node's score from the
// installed ranking and n the number of nodes it ranked, so n·s is 1 for an
// average node. Scores only grow: a node the ranking did not score (an
// isolated one, say) keeps its score, and a nil ranking boosts nothing.
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

// matchesFilter reports whether key passes an anchor filter: always when
// anchors is empty, otherwise when key is one of the anchors or adjacent to
// one. A fact is filed under a topic or entity by an edge to its anchor node,
// so the test is one adjacency lookup per anchor.
func (g *InMemoryGraph[K, P]) matchesFilter(key K, anchors []K) bool {
	if len(anchors) == 0 {
		return true
	}
	for _, anchor := range anchors {
		if key == anchor {
			return true
		}
		if _, ok := g.nodeToTargets[key][anchor]; ok {
			return true
		}
		if _, ok := g.nodeToSources[key][anchor]; ok {
			return true
		}
	}
	return false
}

// timeFilter keeps the facts inside [since, until), a zero bound being open,
// and decays each kept score by recency: score × 0.5^(age/half-life), so of
// two equally relevant facts the more recent ranks first. The half-life is
// Engine.Halflife; a non-positive value disables decay. A future timestamp
// counts as age zero, so decay never raises a score.
func (g *InMemoryGraph[K, P]) timeFilter(keys []K, scores map[K]P, since time.Time, until time.Time) ([]K, map[K]P) {
	now := time.Now()
	halflife := g.config.Engine.Halflife

	kept := make([]K, 0, len(keys))
	for _, key := range keys {
		node, ok := g.Nodes()[key]
		if !ok {
			delete(scores, key)
			continue
		}
		// Only facts are hits: anchors seed and filter searches but are
		// never returned.
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

// IsEmpty reports whether the graph holds no nodes. Callers ask it under the
// graph lock, so it is O(1) rather than derived from Stats, which walks every
// node.
func (s *InMemoryGraph[K, P]) IsEmpty() bool {
	return len(s.Nodes()) == 0
}
