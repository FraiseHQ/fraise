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
	// Traversal, Ranking and Scorer). traversal nil hears no structure — the
	// graph channel is off and search is text/vector only (db.Start installs
	// the ExcessTraversal per config.SearchAlgorithm); ranking nil applies no
	// structural boost. scorer folds each candidate's contributions into its
	// relevance score and is never nil — the graph starts with the
	// ExcessScorer, because unscored candidates cannot rank at all.
	traversal Traversal[K, P]
	ranking   Ranking[K, P]
	scorer    scoring.Scorer[K, P]

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
func (g *InMemoryGraph[K, P]) SetScorer(s scoring.Scorer[K, P]) {
	if s == nil {
		return
	}
	g.scorer = s
}

func NewGraph[K ~uint64, P float32 | float64](cfg *config.ConfigSet) *InMemoryGraph[K, P] {
	// The tokenizer and relevance model are installed at the index's
	// construction site, before any insert, per their mid-corpus contracts.
	// Stemming makes morphological variants find each other — recall
	// keywords rarely arrive in the fact's exact inflection — and the excess
	// methodology needs BM25's raw retrieval mass; "matchcount", the index's
	// own default, remains selectable for comparison runs.
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

// store records the node and (re)indexes its value in the text index. Only
// facts are indexed. Relationship nodes carry no text at all, and indexing
// them as empty documents inflated the corpus count ~3× and crushed avgdl —
// silently distorting every idf and length norm the text scores are built
// from. Anchors (Topic, NamedEntity) do carry text, and every one of those
// consequences applies to them harder: a one- or two-token name is exactly
// the document BM25's length norm rewards most, so an anchor whose name is
// the query term outranks every real fact and takes the top of the seed list.
// It cannot pay for the slot. An anchor seed transmits nothing — its
// neighbours are facts, so ExcessTraversal finds no anchors from it and
// observes no mass — and timeFilter drops it before it can be a hit. The
// guard is load-bearing for retrieval quality, not an optimization: anchors
// earn their place in retrieval by mediating transmission between facts, not
// by being retrievable themselves.
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

		if err := g.textIndex.Insert(key, stopwords.CleanContent(attrs.Value, language.English)); err != nil {
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

// Neighbours returns the keys adjacent to key in either direction without
// copying the whole edge set: it allocates one slice sized to that node's
// own degree. This is the read a source-rooted traversal needs — the
// per-seed ExcessTraversal uses it instead of AdjacencyMap/PredecessorMap,
// which each deep-copy every edge in the graph on every call.
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

func (g *InMemoryGraph[K, P]) Search(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int, since time.Time, until time.Time) ([]*Node[K], []P, [][]scoring.Contribution[K, P], P) {
	// A. Collection: the retrieval stages pool everything they observe about
	// every candidate — text seeds, vector seeds, anchor transmission, or
	// with anchors alone the anchors' own members — as Contribution records,
	// plus the one query-global observation (the background rate). No stage
	// computes policy.
	candidates, background := g.collect(keywords, vector, topics, entities, depth, top)

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

	// C. Time filtered (since or until)

	kept, ranked := g.timeFilter(keys, scores, since, until)

	// D. Rank the candidates and truncate to top
	//
	// The candidates arrive in the order the candidate map was iterated in, so
	// the ranking has to be a total order for identical queries to return
	// identical hits: score descending, then fact key. Facts of equal score are
	// ordered by key rather than left as the map presented them, because
	// truncation would otherwise keep an arbitrary subset of a tied group.
	// TopK keeps only the top best under that order without sorting every
	// candidate, so a query that returns few results out of many candidates
	// pays O(n log top) instead of O(n log n).

	ranker := containers.NewTopK[K, P](top, comparator.OrderedComparator[K])
	for _, key := range kept {
		ranker.Offer(key, ranked[key])
	}
	rankedKeys, rankedScores := ranker.Drain()

	nodes := make([]*Node[K], len(rankedKeys))
	scoresOut := make([]P, len(rankedKeys))
	contributions := make([][]scoring.Contribution[K, P], len(rankedKeys))
	for i, key := range rankedKeys {
		node := g.idToNodes[key]
		nodes[i] = &node
		scoresOut[i] = rankedScores[i]
		contributions[i] = candidates[key]
	}

	logger.Debug("Graph search completed",
		"candidates", len(kept), "returned", len(rankedKeys))
	return nodes, scoresOut, contributions, background
}

// collect runs the retrieval stages and pools their sightings into one
// candidate map: text and vector seeding, then the installed traversal from
// every seed, then the topic/entity filters. Stages record Contributions and
// compute no policy — the hinge, the null model and the attenuation live
// entirely in the Scorer — and the second return is the background rate, the
// query-global observation that fold needs.
//
// A call with nothing to match — no keywords and no vector — takes the one
// other seeding there is: the named anchors' own members. The anchors are
// seeds there rather than filters on top (the pool is their union by
// construction, a fact filed under several of them holding one sighting per
// anchor), no traversal runs — depth is inert, since every member is already
// in hand and expanding from all of them would return most of the graph —
// and no anchor is observed, so the background is zero: the scorer folds
// each member's own seed mass, and the recency decay ranks the results from
// there.
func (g *InMemoryGraph[K, P]) collect(keywords []string, vector containers.Vector[K, P], topics []string, entities []string, depth int, top int) (scoring.Candidates[K, P], P) {
	candidates := make(scoring.Candidates[K, P])
	if len(keywords) == 0 && vector.Empty() {
		g.gatherMembers(topics, entities, candidates)
		return candidates, 0
	}
	seeds := g.gatherSeeds(keywords, vector, candidates, top)
	background := g.findNeighbours(seeds, candidates, topics, entities, depth)
	return candidates, background
}

// gatherMembers seeds the candidate pool from the named anchors' adjacency
// rows: every fact filed under a named topic or entity, carrying one
// SrcAnchor contribution of unit mass per named anchor it is filed under, so
// a fact under two of them holds twice the seed mass of a fact under one —
// it answers more of the question. Each anchor resolves to the key store
// filed it under, the Topic or NamedEntity hash of its value: a topic and an
// entity of one name are two anchors, and an anchor nothing is filed under
// resolves to an empty row, so an unknown anchor seeds nothing. Anchors are
// visited in query order and a repeated one once, so a candidate's list is
// appended in a fixed order for the scorer's fold. Only facts seed: an
// anchor's row holds facts by construction, and only facts are memories.
func (g *InMemoryGraph[K, P]) gatherMembers(topics []string, entities []string, candidates scoring.Candidates[K, P]) {
	anchors := make([]K, 0, len(topics)+len(entities))
	named := make(map[K]struct{}, len(topics)+len(entities))
	name := func(anchor K) {
		if _, dup := named[anchor]; dup {
			return
		}
		named[anchor] = struct{}{}
		anchors = append(anchors, anchor)
	}
	for _, value := range topics {
		name(Topic[K]{NodeAttributes: NodeAttributes{Value: value}, Hasher: g.hasher}.Key())
	}
	for _, value := range entities {
		name(NamedEntity[K]{NodeAttributes: NodeAttributes{Value: value}, Hasher: g.hasher}.Key())
	}

	var members int
	for _, anchor := range anchors {
		degree := scoring.ClampDegree(len(g.nodeToTargets[anchor]) + len(g.nodeToSources[anchor]))
		for _, member := range g.Neighbours(anchor) {
			if _, isFact := g.idToNodes[member].(Fact[K]); !isFact {
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
// vector index (query embedding), appending one Contribution per sighting; a
// key surfaced by both sources holds one from each. Text contributions carry
// the BM25 × coverage mass; vector contributions carry the similarity
// 1/(1+distance), converted here so Contribution.Score is bigger-is-better
// for every source — the index reports distance, where smaller is nearer.
// The candidate budget is max(seed-size, top): the text list must track the
// requested result size, because a budget capped below top silently flatlines
// every ranking past seed-size ("fair seeding").
// The seed keys return in ascending key order: scorers fold contribution
// lists as floats, so the traversal must observe in a deterministic order or
// the low bits of a shared anchor's mass drift between identical queries.
func (g *InMemoryGraph[K, P]) gatherSeeds(keywords []string, vector containers.Vector[K, P], candidates scoring.Candidates[K, P], top int) []K {
	seedK := g.config.DB.SeedSize
	if top > seedK {
		seedK = top
	}

	var textSeeds, vectorSeeds int
	if len(keywords) > 0 {
		// Index errors (empty index) just mean no text seeds.
		if keys, scores, err := g.textIndex.Search(strings.Join(keywords, " "), seedK); err == nil {
			textSeeds = len(keys)
			for rank, key := range keys {
				candidates[key] = append(candidates[key], scoring.Contribution[K, P]{Src: scoring.SrcText, Score: scores[rank], Rank: scoring.ClampRank(rank), Count: 1})
			}
		} else {
			logger.Debug("Text index yielded no seeds", "error", err)
		}
	}

	if !vector.Empty() {
		if keys, distances, err := g.vectorIndex.Search(vector, seedK); err == nil {
			vectorSeeds = len(keys)
			for rank, key := range keys {
				candidates[key] = append(candidates[key], scoring.Contribution[K, P]{Src: scoring.SrcVector, Score: P(1) / (P(1) + distances[rank]), Rank: scoring.ClampRank(rank), Count: 1})
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

// depthOneAdmission is the depth-1 precision lane's multiplier on an anchor's
// fair share of the background null: at depth 1 an anchor must clear this many
// times its null-expected mass before it transmits, so only strongly
// above-chance anchors pass (higher precision, lower recall). depth >= 2
// admits at the plain fair share (× 1). A methodology constant, not
// configuration — tune it here.
const depthOneAdmission = 2

// findNeighbours runs the installed traversal from every seed — when the
// query names an anchor, the graph's only door — and pools what it observes,
// in two passes. Pass 1 observes: each seed's mass — the
// scorer's fold of the seed's own contributions, fixed before any traversal
// so scores cannot depend on traversal order — is accumulated onto every
// anchor the traversal reaches at depth 1, alongside the anchor's degree and
// its funding-seed count; the background rate is then the total observed mass
// over the total degree of every touched anchor, silent ones included (the
// null model weighs what the traversal saw, not what later speaks). Pass 2
// expands: only anchors whose mass exceeds their size-proportional share of
// the background — the admission prune; anything at or below background
// transmits nothing by Property 5.3, so a fat fair-share hub costs O(1) here,
// not O(degree) — append one SrcGraph contribution per (member, anchor):
// the anchor's full observed mass, its identity, degree and seed count. The
// hinge, the fair-share subtraction and the attenuation stay in the Scorer;
// this layer records observations. depth selects the lane: 0 skips the
// traversal and only seed mass scores (the floor); 1 and 2 both run the
// single anchor-mediated round, depth 1 admitting anchors only above
// depthOneAdmission * fair share (the precision lane) and depth 2 at plain
// fair share (max recall). The pooled candidates are then filtered by
// topics and entities regardless of the lane.
func (g *InMemoryGraph[K, P]) findNeighbours(seeds []K, candidates scoring.Candidates[K, P], topics []string, entities []string, depth int) P {
	// depth 0 completes no transmission: the candidates are the text/vector
	// seeds alone, scored by their own mass (the floor), the anchor expansion
	// skipped entirely — the fast, text-only lane. depth 1 and 2 both run the
	// one anchor-mediated round (the excess scorer); they differ only in pass
	// 2's admission bar (see depthOneAdmission). Neither iterates — a second
	// round re-observes the first round's concentrated mass through sibling
	// anchors and collapses recall (measured), so depth is capped at 2. The
	// round also needs a door into the graph: an anchor the query names. A
	// recall naming no topic or entity is a text and vector search whatever
	// its depth — the parser says so with a warning when a depth clause asked
	// for more — so the traversal is gated on the filters as well as the lane.
	var background P
	if depth >= 1 && g.traversal != nil && len(seeds) > 0 && (len(topics) > 0 || len(entities) > 0) {
		// Every seed's fused mass is fixed before any traversal appends
		// SrcGraph contributions; seed fusion runs the unbound scorer — no
		// traversal has observed anything yet, so there is no null to bind.
		seedScores := make(map[K]P, len(seeds))
		for _, seed := range seeds {
			seedScores[seed] = g.scorer.Score(candidates[seed])
		}

		// Pass 1 (observe): per-anchor mass, seed count, degree, and — once
		// per anchor, from whichever traversal touches it first — its member
		// band, already deduplicated and ascending per the traversal contract.
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
			// An anchor's member band is identical from every seed that
			// touches it, so it is recorded exactly once — on first touch —
			// while its observed mass accumulates across every touching seed.
			newAnchors := make(map[K]struct{})
			for _, vertex := range r.Order {
				// Only anchor vertices observe mass: a traversal may surface
				// other node kinds at depth 1 (BFS follows every edge), and a
				// fact is a candidate, never an observer.
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
				// Full incidence when the traversal carries it (the excess
				// traversal's Parents); a tree-shaped traversal (BFS) yields
				// single-parent incidence through the canonical Parent — a
				// member is then observed through the one anchor that
				// discovered it, which is the honest reading of a tree.
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

		// Pass 2 (expand): anchors above their admitted share. depth 1 is the
		// precision lane — it raises the bar to depthOneAdmission × fair share,
		// so only strongly-above-chance anchors transmit; depth 2 admits at the
		// plain fair share.
		admitRate := background
		if depth == 1 {
			admitRate *= depthOneAdmission
		}
		for _, anchor := range touched {
			if anchorMass[anchor] <= P(degree[anchor])*admitRate {
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
		if !g.matchesFilter(key, topics) || !g.matchesFilter(key, entities) {
			delete(candidates, key)
		}
	}
	return background
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
