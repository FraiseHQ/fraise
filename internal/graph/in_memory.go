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
// map keyed by node key. Relationships are also recorded in a pair of
// mirrored adjacency maps. Two secondary indexes serve hybrid search: a BTree
// full-text index over fact text, and an RPTree (random projection forest)
// vector index over caller-provided embeddings.
type InMemoryGraph[K ~uint64, P float32 | float64] struct {
	idToNodes     map[K]Node[K] // all nodes
	nodeToSources map[K]map[K]K // incoming: target -> source -> edge
	nodeToTargets map[K]map[K]K // outgoing: source -> target -> edge

	textIndex   *index.BTreeIndex[K, P]
	vectorIndex *index.RPTreeIndex[K, P]

	// traversal and scorer are the pluggable search algorithms. db.Start
	// installs them from configuration. A nil traversal turns the graph
	// channel off, leaving text and vector search. scorer is never nil.
	// NewGraph installs the ExcessScorer, since candidates cannot be ranked
	// without one.
	traversal Traversal[K, P]
	scorer    scoring.Scorer[K, P]

	hasher hash.Hasher[K, string]

	config *config.ConfigSet

	// sequences caches, per anchor, its member facts in the order they were
	// written. contiguous reads a fact's temporal neighbours from it. An
	// anchor's sequence is built on first use, under sequenceMu because
	// searches share the graph's read lock. Every write that touches the
	// anchor or one of its members drops it. So a sequence is never stale, and
	// a graph never searched with a window never builds one.
	sequences map[K]*sequence[K]

	// contiguity caches, per fact, what contiguous returned for it. It is a
	// pure function of the cached sequences of the fact's anchors, so it is
	// dropped with any of them. A window search asks for the neighbours of
	// every matched fact, and between writes the answer does not change.
	contiguity map[K][]K

	sequenceMu sync.Mutex

	mu sync.RWMutex
}

// sequence is one anchor's member facts ordered by write time, then key. It
// holds each member's position, so a neighbour lookup is a map read and two
// slice reads rather than a search.
type sequence[K comparable] struct {
	keys []K
	at   map[K]int
}

// SetTraversal installs the traversal Search expands seeds with, such as
// ExcessTraversal or BFS.
func (g *InMemoryGraph[K, P]) SetTraversal(t Traversal[K, P]) {
	g.traversal = t
}

// SetScorer installs the scorer Search folds candidate contributions with. A
// nil scorer is ignored. A nil traversal only switches the graph channel off,
// but a graph without a scorer cannot rank at all.
func (g *InMemoryGraph[K, P]) SetScorer(s scoring.Scorer[K, P]) {
	if s == nil {
		return
	}
	g.scorer = s
}

// NewGraph returns an empty graph. Its indexes, relevance model and hasher are
// configured from cfg, and the [scoring.ExcessScorer] is installed.
// [InMemoryGraph.SetTraversal] installs the traversal.
func NewGraph[K ~uint64, P float32 | float64](cfg *config.ConfigSet) *InMemoryGraph[K, P] {
	// The tokenizer and relevance model must be installed before the first
	// insert. Stemming lets a keyword find other inflections of the same
	// word. BM25 is the configured default because the excess scorer needs
	// its raw retrieval mass. "matchcount", the index's own default, stays
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
		scorer:     scoring.NewExcessScorer[K, P](),
		hasher:     hash.NewHasher[K](cfg),
		config:     cfg,
		sequences:  make(map[K]*sequence[K]),
		contiguity: make(map[K][]K),
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

// Set inserts a new node under its own key. It returns ErrNodeAlreadyExists
// if the key is taken.
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

// textLanguage is the language whose stop words are removed on both sides of
// the text index. store removes them from a fact's value, and gatherSeeds from
// the query keywords. Both sides must use the same language. Otherwise a stop
// word kept on one side leaves a stem that collides with a content word's
// ("own" with "owns"). Only English is supported.
var textLanguage = language.English

// store records the node and indexes a fact's value in the text index. Only
// facts are indexed. Relationships carry no text, and as empty documents they
// would skew the corpus statistics that every BM25 score is built from
// (document count, average length). Anchors (Topic, NamedEntity) are left out
// because BM25's length norm favours their one- or two-word names most. An
// anchor named like the query term would take the top seed slots. Yet it
// transmits nothing, since its neighbours are facts, not anchors, and it is
// never returned as a hit.
//
// A fact the text index already holds under key, re-asserted through Put, is
// replaced through Update. A new fact is added through Insert.
func (g *InMemoryGraph[K, P]) store(key K, node Node[K]) error {
	g.idToNodes[key] = node
	g.dropSequences(key)

	r, ok := node.(Relationship[K])
	if ok {
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
		// The edge files source under target, so target's sequence has a new
		// member. The drop walks source's rows, so it runs once they hold the
		// edge.
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
// adjacency entries. A relationship is not a vertex and has no vector.
func (g *InMemoryGraph[K, P]) dropRelationship(key K) {
	delete(g.idToNodes, key)
}

// unlink removes the edge source -> target from both adjacency views and drops
// any row it leaves empty. Code that enumerates the views treats a node with a
// row as a vertex. So a node that loses its last edge must leave both views.
// Otherwise it is counted as a dangling vertex instead of left out as
// isolated.
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
// whole edge: its node and both adjacency entries. So Nodes, Size and AdjacencyMap always agree on which edges exist.
// A node left with no edges has no row in either view, so it is isolated
// rather than dangling.
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

	// When the node is an endpoint, its adjacency rows hold each incident
	// edge's key. Walking them finds the relationship nodes to drop.
	for target, edge := range g.nodeToTargets[key] {
		g.unlink(key, target)
		g.dropRelationship(edge)
	}
	for source, edge := range g.nodeToSources[key] {
		g.unlink(source, key)
		g.dropRelationship(edge)
	}
	delete(g.idToNodes, key)

	// A relationship has no rows of its own. It has only the two entries
	// store wrote into its endpoints' rows. The stored node is inspected, not
	// the argument, because the key identifies the node to remove.
	if r, isEdge := stored.(Relationship[K]); isEdge {
		source := (*r.Source()).Key()
		target := (*r.Target()).Key()
		g.unlink(source, target)
	}

	// The node may be absent from either index, which is not an error.
	_ = g.GetTextIndex().Delete(key)
	_ = g.vectorIndex.Delete(key)
	return nil
}

// Nodes returns the live node map. Every read of the graph's nodes goes
// through it. Only store, Delete and dropRelationship write the map itself.
func (g *InMemoryGraph[K, P]) Nodes() map[K]Node[K] {
	return g.idToNodes
}

// AdjacencyMap implements [Graph]. It returns a deep copy of the outgoing
// edges, so a caller holding it cannot corrupt the graph's own rows.
func (g *InMemoryGraph[K, P]) AdjacencyMap() map[K]map[K]K {
	return exportEdges(g.nodeToTargets)
}

// PredecessorMap implements [Graph]. It returns a deep copy of the incoming
// edges, so a caller holding it cannot corrupt the graph's own rows.
func (g *InMemoryGraph[K, P]) PredecessorMap() map[K]map[K]K {
	return exportEdges(g.nodeToSources)
}

// Neighbours returns the keys adjacent to key in either direction. It
// allocates one slice, sized to the node's degree.
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

// exportEdges returns a deep copy of an edge map.
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
// topics. Relationships are stored as nodes and satisfy Entity, so Order skips
// them explicitly. They are edges, which Size counts.
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

// Stats implements [Graph].
func (g *InMemoryGraph[K, P]) Stats() GraphStats {
	return GraphStats{
		Order:   g.Order(),
		Size:    g.Size(),
		Nodes:   len(g.Nodes()),
		Vectors: g.GetVectorIndex().Count(),
		// Entries minus Count is the vector index's compaction debt. The
		// index's automatic Flush keeps it bounded (see rptree flush-factor).
		ForestEntries: g.GetVectorIndex().Entries(),
		Relevance:     g.textIndex.Relevance().Name(),
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

// contiguous returns the facts written immediately before and after key under
// each anchor it is filed under, in ascending key order, each once. It is the
// neighbours function the text index's window scores with (see
// [index.BTreeIndex.SetWindow]): two facts adjacent in time under a shared
// anchor are one exchange, a question and its answer, a statement and its
// follow-up, and the window lets the exchange match as a whole. A fact filed
// under no anchor has no neighbours. Searches call it under the graph's read
// lock, once per matched fact, so the answer is cached until a write drops it
// (see dropSequences) and a repeated lookup allocates nothing.
func (g *InMemoryGraph[K, P]) contiguous(key K) []K {
	g.sequenceMu.Lock()
	defer g.sequenceMu.Unlock()
	if out, ok := g.contiguity[key]; ok {
		return out
	}
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
	out = slices.Compact(out)
	g.contiguity[key] = out
	return out
}

// sequence returns anchor's members in write order, building and caching it
// on first use. Members are the facts adjacent to the anchor; ties on the
// timestamp (a batch written in one instant) are broken by key, so the order
// is total and two searches agree on who neighbours whom. The caller holds
// sequenceMu.
func (g *InMemoryGraph[K, P]) sequence(anchor K) *sequence[K] {
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
// A dropped sequence takes its members' cached contiguity with it, and key's
// own, so every cached neighbour list is read off sequences still cached.
func (g *InMemoryGraph[K, P]) dropSequences(key K) {
	g.sequenceMu.Lock()
	defer g.sequenceMu.Unlock()
	delete(g.contiguity, key)
	g.dropSequence(key)
	for neighbour := range g.nodeToTargets[key] {
		g.dropSequence(neighbour)
	}
	for neighbour := range g.nodeToSources[key] {
		g.dropSequence(neighbour)
	}
}

// dropSequence discards anchor's cached sequence and the cached contiguity of
// each of its members. The caller holds sequenceMu.
func (g *InMemoryGraph[K, P]) dropSequence(anchor K) {
	seq, ok := g.sequences[anchor]
	if !ok {
		return
	}
	for _, member := range seq.keys {
		delete(g.contiguity, member)
	}
	delete(g.sequences, anchor)
}

// Search implements [Graph.Search]. It collects candidates and folds each
// with the installed scorer. It applies the time window and recency decay,
// then keeps the top hits. Hits rank by score descending, then key ascending,
// so identical queries return identical hits. Hits below the
// db.min-score-ratio cutoff are dropped.
func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) (Result[K, P], error) {
	// A. Collection gathers every observation of every candidate as
	// Contributions, plus the query's background rate. Observations come from
	// text and vector seeds, anchor transmission, or the named anchors'
	// members when they seed alone.
	candidates, background, err := g.collect(keywords, vector, topics, entities, depth, top, since, until)
	if err != nil {
		return Result[K, P]{}, err
	}

	// B. Scoring folds each candidate's contributions into one relevance
	// score, given the background.
	scores := make(map[K]P, len(candidates))
	keys := make([]K, 0, len(candidates))
	scorer := g.scorer.WithBackground(background)
	for key, contributions := range candidates {
		scores[key] = scorer.Score(contributions)
		keys = append(keys, key)
	}

	// C. Time window and recency decay. The seeds are already inside the
	// window. Facts reached by the traversal and anchor members are not.
	kept, ranked := g.timeFilter(keys, scores, since, until)

	// D. Rank and truncate to top. Map iteration order is random, so
	// identical queries return identical hits only under a total order:
	// score descending, then key ascending. Without the key tie-break,
	// truncation would keep an arbitrary subset of a tied group. TopK costs
	// O(n log top) rather than the O(n log n) of a full sort, and a
	// non-positive top keeps every match.
	ranker := containers.NewTopK[K, P](top, comparator.OrderedComparator[K])
	for _, key := range kept {
		ranker.Offer(key, ranked[key])
	}
	rankedKeys, rankedScores := ranker.Drain()

	// E. Score cutoff (db.min-score-ratio), then the answer.
	rankedKeys, rankedScores = g.scoreCutoff(rankedKeys, rankedScores, scorer, candidates)
	result := Result[K, P]{Background: background, Hits: g.hits(rankedKeys, rankedScores, candidates)}

	logger.Debug("Graph search completed",
		"candidates", len(kept), "returned", len(result.Hits))
	return result, nil
}

// scoreCutoff applies db.min-score-ratio to a best-first ranking: it keeps
// each hit whose relevance (the scorer's output, before decay) is at
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

// hits turns a ranking into fact hits, each carrying the contributions its
// score was folded from.
func (g *InMemoryGraph[K, P]) hits(keys []K, scores []P, candidates scoring.Candidates[K, P]) []Hit[K, P] {
	out := make([]Hit[K, P], len(keys))
	for i, key := range keys {
		out[i] = Hit[K, P]{Node: g.Nodes()[key], Score: scores[i], Contributions: candidates[key]}
	}
	return out
}

// collect runs the retrieval stages and pools their observations into one
// candidate map. The stages are text and vector seeding, then the anchor
// round over the seeds (see findNeighbours). collect also returns the
// background rate the scorer needs.
// Stages only record Contributions. The hinge and the null model belong to
// the Scorer. The only error is a vector dimension mismatch.
//
// The [since, until) window restricts what the seeding searches, not what it
// keeps. Each index gets a keep that rejects facts outside it, so the seeds
// are the best matches among the facts the query can return. Named anchors
// filter nothing: a match filed elsewhere is still a candidate, and the
// anchors contribute through the excess they transmit (see findNeighbours).
// Each index is read twice when anchors are named, once over the window and
// once over the facts filed under a named anchor, and the readings are pooled
// (see gatherSeeds). A fact that ranks below the seed budget across the whole
// graph but first under its anchor is still seeded, and funds the anchor's
// transmission. On a graph shared by many topics, a single global reading
// would leave the named anchor unfunded while matching facts sit under it.
//
// top is how many hits the recall asks for. Each index is read max(seed-size,
// top) deep (see gatherSeeds).
//
// With no keywords and no vector, the named anchors' members are the seeds
// instead (see Graph.Search). No traversal runs, since expanding from every
// member would return most of the graph. The background is zero.
//
// Topic and entity names are resolved to anchor keys once per query. So
// gatherMembers, the anchored reading and findNeighbours agree on which node
// a name denotes, and membership is a key lookup.
func (g *InMemoryGraph[K, P]) collect(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) (scoring.Candidates[K, P], P, error) {
	topicKeys, entityKeys := g.anchorKeys(topics, entities)
	candidates := make(scoring.Candidates[K, P])
	if len(keywords) == 0 && vector.Empty() {
		g.gatherMembers(topicKeys, entityKeys, candidates)
		return candidates, 0, nil
	}
	// An unbounded window hands the indexes a nil keep, so a recall naming no
	// anchor pays nothing per posting.
	var window func(K) bool
	if !since.IsZero() || !until.IsZero() {
		window = func(key K) bool {
			node, ok := g.Nodes()[key]
			return ok && within(node.GetTimestamp(), since, until)
		}
	}
	keeps := []func(K) bool{window}
	if named := slices.Concat(topicKeys, entityKeys); len(named) > 0 {
		keeps = append(keeps, func(key K) bool {
			node, ok := g.Nodes()[key]
			return ok && within(node.GetTimestamp(), since, until) && g.filedUnder(key, named)
		})
	}
	seeds, err := g.gatherSeeds(keywords, vector, keeps, candidates, top)
	if err != nil {
		return nil, 0, err
	}
	background := g.findNeighbours(seeds, candidates, topicKeys, entityKeys, depth)
	return candidates, background, nil
}

// anchorKeys resolves topic and entity names to the keys of their anchor
// nodes, in query order. A topic and an entity with the same name are
// different anchors. A name nothing is filed under resolves to a key no node
// holds. It seeds nothing and transmits nothing.
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
// anchor it is filed under. So a fact under two of them starts with twice the
// mass of a fact under one. Anchors are visited once each, in query order
// (topics, then entities). Contributions are thus appended in a fixed order
// for the scorer's fold.
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
// vector index (embedding). It appends one Contribution per hit, so a key
// found by both gets one from each. Keywords are cleaned of stop words exactly
// as store cleans a fact's text, so both sides of the index share one
// vocabulary. Text contributions carry the BM25 × coverage mass. Vector
// contributions carry the similarity 1/(1+distance). So Score is
// bigger-is-better for every source.
//
// Each index is searched once, for one reading per keep, and the readings are
// pooled into one ranking per source (see union). A key read under several
// keeps is sighted once per source.
//
// The vector similarity is weighted by the information the text did not find.
// The weight is I_q / I_hit = 1 + (I_q − I_hit) / I_hit. I_q is the
// information in the query's words, and I_hit is the part of it the best text
// hit holds (see [index.BTreeIndex.Information]). So the embedding is credited
// with the bits the text missed per bit it found. The best hit is the first
// text hit of the pooled ranking. The embedding competes with the text hits
// the recall can return. A full match outside the window says nothing about
// what the text found for this recall.
//
// If the best hit holds all the query's words, the weight is 1 and the text
// leads. A hit holding half the query's information doubles the weight. A hit
// holding a quarter quadruples it, and the embedding, which does not need the
// words, carries the ranking. The weight is a ratio of information, so it
// reads the same for a long question as for a short one. It has no constant
// to tune. With no text hit the recall keeps, the weight is 1. The weight is
// one number per query, applied to every vector sighting. So the vector
// ranking keeps its order, and only its mass against the text's moves.
//
// Each source is asked for max(seed-size, top) candidates among the keys each
// keep accepts, so the text ranking is never cut off before top. Every
// candidate read is a seed, a key the traversal expands from. The seed keys
// are returned in ascending order. The
// traversal folds floats, and a fixed order keeps identical queries scoring
// identically. An empty index seeds nothing. A vector of the wrong dimension
// is an error, since it comes from a different embedding model than the
// graph's.
func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[K, P], keeps []func(K) bool, candidates scoring.Candidates[K, P], top int) ([]K, error) {
	seedK := g.config.DB.SeedSize
	if top > seedK {
		seedK = top
	}
	// No more keys can seed than the indexes hold, so the set is sized on
	// them, not on top. A large db.max-top then costs nothing per recall.
	seeded := make(map[K]struct{}, min(seedK, g.textIndex.Count()+g.vectorIndex.Count()))

	var textSeeds, vectorSeeds int
	weight := P(1)
	if len(keywords) > 0 {
		query := stopwords.CleanContent(strings.Join(keywords, " "), textLanguage)
		// An index error (empty index) means no text seeds.
		readings, masses, err := g.GetTextIndex().Search(query, seedK, keeps...)
		if err != nil {
			logger.Debug("Text index yielded no seeds", "error", err)
		}
		keys, scores := g.union(readings, masses, seeded)
		textSeeds = len(keys)
		if !vector.Empty() && len(keys) > 0 {
			if total, held := g.textIndex.Information(query, keys[0]); held > 0 {
				weight = total / held
			}
		}
		// The reading's sightings share one array, so a reading allocates
		// once. A candidate's first sighting is a capacity-one
		// window onto it. A later append then copies out instead of
		// overwriting the next candidate's sighting.
		sightings := make([]scoring.Contribution[K, P], len(keys))
		for rank, key := range keys {
			sightings[rank] = scoring.Contribution[K, P]{Src: scoring.SrcText, Score: scores[rank], Rank: scoring.ClampRank(rank), Count: 1}
			if candidates[key] == nil {
				candidates[key] = sightings[rank : rank+1 : rank+1]
			} else {
				candidates[key] = append(candidates[key], sightings[rank])
			}
		}
	}

	if !vector.Empty() {
		readings, nearness, err := g.vectorIndex.Search(vector, seedK, keeps...)
		if errors.Is(err, index.ErrInvalidDimension) {
			return nil, err
		}
		if err != nil {
			logger.Debug("Vector index yielded no seeds", "error", err)
		}
		// union ranks bigger first, and a distance ranks smaller first.
		// Negating it keeps the distance exact, so the similarity below folds
		// the same bits as a lone reading's.
		for _, distances := range nearness {
			for i := range distances {
				distances[i] = -distances[i]
			}
		}
		keys, negated := g.union(readings, nearness, seeded)
		vectorSeeds = len(keys)
		// One array for the reading, as for the text index's.
		sightings := make([]scoring.Contribution[K, P], len(keys))
		for rank, key := range keys {
			sightings[rank] = scoring.Contribution[K, P]{Src: scoring.SrcVector, Score: weight / (P(1) - negated[rank]), Rank: scoring.ClampRank(rank), Count: 1}
			if candidates[key] == nil {
				candidates[key] = sightings[rank : rank+1 : rank+1]
			} else {
				candidates[key] = append(candidates[key], sightings[rank])
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

// union pools one source's readings, one per keep, into a single ranking. It
// orders the pool as every index reading is ordered, score descending then
// key ascending, so a candidate's Rank means the same whichever reading found
// it. A key read under several keeps appears once. Every key of every
// reading is marked in seeded, so a reading under a named anchor seeds as
// deep as the reading over the window. A lone reading is returned as is.
func (g *InMemoryGraph[K, P]) union(readings [][]K, scores [][]P, seeded map[K]struct{}) ([]K, []P) {
	for _, keys := range readings {
		for _, key := range keys {
			seeded[key] = struct{}{}
		}
	}
	switch len(readings) {
	case 0:
		return nil, nil
	case 1:
		return readings[0], scores[0]
	}
	pooled := containers.NewTopK[K, P](0, comparator.OrderedComparator[K])
	offered := make(map[K]struct{}, len(readings[0]))
	for i, keys := range readings {
		for j, key := range keys {
			if _, dup := offered[key]; dup {
				continue
			}
			offered[key] = struct{}{}
			pooled.Offer(key, scores[i][j])
		}
	}
	return pooled.Drain()
}

// depthOneAdmission multiplies an anchor's fair share at depth 1. The anchor
// transmits only when its observed mass exceeds this many times its fair
// share. This trades recall for precision. depth 2 admits at the fair share
// itself. It is a methodology constant, not configuration.
const depthOneAdmission = 2

// findNeighbours walks the traversal twice and pools what it observes into
// candidates. It returns the background rate.
//
// Pass 1 observes. One walk from the seeds reaches the anchors they are filed
// under. Each seed's mass is the scorer's fold of its own contributions, and
// an anchor's mass is the sum over the seeds it was reached from. Each anchor
// also records its degree and how many seeds funded it. The background rate is
// the total observed mass over the total degree of every touched anchor,
// silent ones included.
//
// Pass 2 expands. An anchor transmits when its mass exceeds its fair share:
// degree × background, raised by depthOneAdmission at depth 1. One walk from
// the transmitting anchors reaches their members. Each member gets one
// SrcGraph contribution per anchor it was reached from, carrying the anchor's
// full observed mass, identity, degree and seed count. An anchor at or below
// its fair share transmits nothing (hub silence), and pass 2 never walks it.
// So a recall pays once for the members of each anchor that transmits, and
// nothing for a hub its seeds reach that stays silent. The hinge and the
// fair-share subtraction are left to the Scorer.
//
// A named anchor is how the query's topics and entities take part. It filters
// nothing. It transmits whenever a seed reaches it, whatever its share, and
// its contributions are marked Named, so the Scorer lifts its members by the
// mass the other seeds found there rather than by its surplus over chance.
// It still counts toward the background like any touched anchor.
//
// depth 0 skips both passes, so only seed mass scores and the named anchors
// contribute nothing.
func (g *InMemoryGraph[K, P]) findNeighbours(seeds []K, candidates scoring.Candidates[K, P], topicKeys []K, entityKeys []K, depth int) P {
	// The traversal runs at depth 1 or 2, and only when the query names a
	// topic or entity. A recall naming neither is a text and vector search
	// whatever its depth, and the parser warns when it sets a depth above 0.
	// There is a single round. A second round would re-observe the first
	// round's mass through sibling anchors and collapse recall, which is why
	// depth stops at 2.
	var background P
	if depth >= 1 && g.traversal != nil && len(seeds) > 0 && (len(topicKeys) > 0 || len(entityKeys) > 0) {
		// Fix every seed's mass before any SrcGraph contribution is
		// appended, so scores cannot depend on traversal order. The scorer
		// is unbound because nothing has been observed yet, so there is no
		// background.
		seedScores := make(map[K]P, len(seeds))
		for _, seed := range seeds {
			seedScores[seed] = g.scorer.Score(candidates[seed])
		}

		// Pass 1 (observe): each anchor's mass, seed count and degree. The
		// seeds funding an anchor come in seed order, so its mass is folded
		// in a fixed order.
		observed, ok := g.walk(seeds)
		if !ok {
			return background
		}
		anchorMass := make(map[K]P)
		anchorSeeds := make(map[K]int)
		degree := make(map[K]int)
		touched := make([]K, 0)
		for _, anchor := range observed.Order {
			// Only anchors observe mass. BFS follows every edge, so it can
			// reach other kinds of node at depth 1.
			if observed.Depth[anchor] != 1 || !isAnchor[K, P](g, anchor) {
				continue
			}
			degree[anchor] = len(g.nodeToTargets[anchor]) + len(g.nodeToSources[anchor])
			touched = append(touched, anchor)
			// ExcessTraversal reports every seed the anchor was reached from.
			// A tree-shaped traversal such as BFS reports only the seed that
			// discovered it, which alone funds it.
			for _, seed := range observed.reachedFrom(anchor) {
				anchorMass[anchor] += seedScores[seed]
				anchorSeeds[anchor]++
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
		// because the division rounds. An anchor holding exactly its share
		// must stay silent, such as the only anchor a query reaches. A
		// rounding error could let it clear the bar.
		admission := P(1)
		if depth == 1 {
			admission = depthOneAdmission
		}
		named := make(map[K]struct{}, len(topicKeys)+len(entityKeys))
		for _, anchor := range slices.Concat(topicKeys, entityKeys) {
			named[anchor] = struct{}{}
		}
		transmitting := make([]K, 0, len(touched))
		for _, anchor := range touched {
			_, isNamed := named[anchor]
			if isNamed || anchorMass[anchor]*P(totalDegree) > P(degree[anchor])*totalMass*admission {
				transmitting = append(transmitting, anchor)
			}
		}
		if len(transmitting) == 0 {
			return background
		}
		// The anchors a member was reached from come in ascending key order,
		// so its contributions are appended in a fixed order. A tree-shaped
		// traversal observes a member through the one anchor that discovered
		// it.
		expanded, ok := g.walk(transmitting)
		if !ok {
			return background
		}
		for _, member := range expanded.Order {
			if expanded.Depth[member] != 1 {
				continue
			}
			for _, anchor := range expanded.reachedFrom(member) {
				_, isNamed := named[anchor]
				candidates[member] = append(candidates[member], scoring.Contribution[K, P]{
					Src:    scoring.SrcGraph,
					Score:  anchorMass[anchor],
					Via:    anchor,
					Degree: scoring.ClampDegree(degree[anchor]),
					Count:  scoring.ClampCount(anchorSeeds[anchor]),
					Named:  isNamed,
				})
			}
		}
	}
	return background
}

// walk runs a clone of the installed traversal from sources. It reports false
// when the walk fails, which it does only when the graph holds none of the
// sources.
func (g *InMemoryGraph[K, P]) walk(sources []K) (TraversalResult[K], bool) {
	t := g.traversal.Clone()
	t.SetSources(sources)
	result, err := t.Run(g)
	if err != nil {
		return TraversalResult[K]{}, false
	}
	r, ok := result.(TraversalResult[K])
	return r, ok
}

// filedUnder reports whether key is one of the anchors or filed under one. A
// fact is filed under a topic or entity by an edge to its anchor node, so the
// test is one adjacency lookup per anchor.
func (g *InMemoryGraph[K, P]) filedUnder(key K, anchors []K) bool {
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

// timeFilter keeps the facts inside [since, until), where a zero bound is
// open. It decays each kept score by recency: score × 0.5^(age/half-life). So
// of two equally relevant facts, the more recent ranks first. The half-life is
// Engine.Halflife, and a non-positive value disables decay. A future timestamp
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
		// Only facts are hits. Anchors seed and transmit but are never
		// returned.
		if _, isFact := node.(Fact[K]); !isFact {
			delete(scores, key)
			continue
		}
		ts := node.GetTimestamp()
		if !within(ts, since, until) {
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

// within reports whether ts lies in [since, until). A zero bound is open.
// timeFilter and the seeding both use it, so they agree on the window. The
// indexes seed only facts the window keeps. The vector weight reads only text
// hits the recall can return.
func within(ts time.Time, since time.Time, until time.Time) bool {
	return (since.IsZero() || !ts.Before(since)) && (until.IsZero() || ts.Before(until))
}

// IsEmpty reports whether the graph holds no nodes. Callers ask it under the
// graph lock, so it is O(1) rather than derived from Stats, which walks every
// node.
func (g *InMemoryGraph[K, P]) IsEmpty() bool {
	return len(g.Nodes()) == 0
}
