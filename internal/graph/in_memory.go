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

	mu sync.RWMutex
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
		scorer: scoring.NewExcessScorer[K, P](),
		hasher: hash.NewHasher[K](cfg),
		config: cfg,
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

// Delete removes the node, its index entries and its incident relationships.
// Deleting either endpoint of an edge, or the relationship itself, removes the
// edge as a whole (its node and both adjacency entries), so Nodes, Size and
// AdjacencyMap never disagree about which edges exist.
func (g *InMemoryGraph[K, P]) Delete(node Node[K]) error {
	if node == nil {
		return ErrNilNode
	}
	key := node.Key()
	stored, ok := g.Nodes()[key]
	if !ok {
		return ErrNodeNotFound
	}

	// Deleting an endpoint: its adjacency rows hold each incident edge's key,
	// so walking them finds the relationship nodes to drop.
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

	// Deleting a relationship: it has no rows of its own, only the two entries
	// store wrote into its endpoints' rows. The stored node is inspected rather
	// than the argument, since the key is what identifies the node to remove.
	if r, isEdge := stored.(Relationship[K]); isEdge {
		source := (*r.Source()).Key()
		target := (*r.Target()).Key()
		delete(g.nodeToTargets[source], target)
		delete(g.nodeToSources[target], source)
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
// so identical queries return identical hits) and finally drops hits below the
// db.min-score-ratio cutoff.
func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*Node[K], []P, [][]scoring.Contribution[K, P], P, error) {
	// A. Collection: every observation of every candidate (text and vector
	// seeds, anchor transmission, or the named anchors' members when they
	// seed alone) as Contributions, and the query's background rate.
	candidates, background, err := g.collect(keywords, vector, topics, entities, depth, top)
	if err != nil {
		return nil, nil, nil, 0, err
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

	// D. Rank and truncate to top. Map iteration order is random, so the
	// order must be total for identical queries to return identical hits:
	// score descending, then key ascending. Without the key tie-break,
	// truncation would keep an arbitrary subset of a tied group. TopK costs
	// O(n log top) rather than the O(n log n) of a full sort.

	ranker := containers.NewTopK[K, P](top, comparator.OrderedComparator[K])
	for _, key := range kept {
		ranker.Offer(key, ranked[key])
	}
	rankedKeys, rankedScores := ranker.Drain()

	// E. Score cutoff (db.min-score-ratio)
	rankedKeys, rankedScores = g.scoreCutoff(rankedKeys, rankedScores, scorer, candidates)

	nodes := make([]*Node[K], len(rankedKeys))
	scoresOut := make([]P, len(rankedKeys))
	contributions := make([][]scoring.Contribution[K, P], len(rankedKeys))
	for i, key := range rankedKeys {
		node := g.Nodes()[key]
		nodes[i] = &node
		scoresOut[i] = rankedScores[i]
		contributions[i] = candidates[key]
	}

	logger.Debug("Graph search completed",
		"candidates", len(kept), "returned", len(rankedKeys))
	return nodes, scoresOut, contributions, background, nil
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
// With no keywords and no vector, the named anchors' members are the seeds
// instead (see Graph.Search). No traversal runs, since expanding from every
// member would return most of the graph, and the background is zero.
//
// Topic and entity names are resolved to anchor keys once per query, so
// gatherMembers and findNeighbours agree on which node a name denotes and the
// filter is a key lookup.
func (g *InMemoryGraph[K, P]) collect(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int) (scoring.Candidates[K, P], P, error) {
	topicKeys, entityKeys := g.anchorKeys(topics, entities)
	candidates := make(scoring.Candidates[K, P])
	if len(keywords) == 0 && vector.Empty() {
		g.gatherMembers(topicKeys, entityKeys, candidates)
		return candidates, 0, nil
	}
	seeds, err := g.gatherSeeds(keywords, vector, candidates, top)
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
// Each source is asked for max(seed-size, top) candidates, so the text
// ranking is never cut off before top. The seed keys are returned in
// ascending order: the traversal folds floats, and a fixed order keeps
// identical queries scoring identically. An empty index seeds nothing; a
// vector of the wrong dimension is an error, since it comes from a different
// embedding model than the graph's.
func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[K, P], candidates scoring.Candidates[K, P], top int) ([]K, error) {
	seedK := g.config.DB.SeedSize
	if top > seedK {
		seedK = top
	}

	var textSeeds, vectorSeeds int
	if len(keywords) > 0 {
		// Index errors (empty index) just mean no text seeds.
		if keys, scores, err := g.GetTextIndex().Search(stopwords.CleanContent(strings.Join(keywords, " "), textLanguage), seedK); err == nil {
			textSeeds = len(keys)
			for rank, key := range keys {
				candidates[key] = append(candidates[key], scoring.Contribution[K, P]{Src: scoring.SrcText, Score: scores[rank], Rank: scoring.ClampRank(rank), Count: 1})
			}
		} else {
			logger.Debug("Text index yielded no seeds", "error", err)
		}
	}

	if !vector.Empty() {
		keys, distances, err := g.vectorIndex.Search(vector, seedK)
		switch {
		case errors.Is(err, index.ErrInvalidDimension):
			return nil, err
		case err != nil:
			logger.Debug("Vector index yielded no seeds", "error", err)
		default:
			vectorSeeds = len(keys)
			for rank, key := range keys {
				candidates[key] = append(candidates[key], scoring.Contribution[K, P]{Src: scoring.SrcVector, Score: P(1) / (P(1) + distances[rank]), Rank: scoring.ClampRank(rank), Count: 1})
			}
		}
	}

	seeds := make([]K, 0, len(candidates))
	for key := range candidates {
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
