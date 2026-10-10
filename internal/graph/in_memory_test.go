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

package graph_test

import (
	"errors"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/graph"
	"github.com/FraiseHQ/fraise/internal/graph/scoring"
	"github.com/FraiseHQ/fraise/internal/index"
)

// newGraph builds an empty graph carrying the test config.
func newGraph() *graph.InMemoryGraph[uint64, float64] {
	return graph.NewGraph[uint64, float64](testConfig())
}

// mkFact returns a Fact wired to g's hasher; its key is derived from value.
func mkFact[P float32 | float64](g *graph.InMemoryGraph[uint64, P], value string, ts time.Time) graph.Fact[uint64] {
	return graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: value, Timestamp: ts}, Hasher: g.GetHasher()}
}

func mkEntity[P float32 | float64](g *graph.InMemoryGraph[uint64, P], value string, ts time.Time) *graph.NamedEntity[uint64] {
	return &graph.NamedEntity[uint64]{NodeAttributes: graph.NodeAttributes{Value: value, Timestamp: ts}, Hasher: g.GetHasher()}
}

func mkTopic[P float32 | float64](g *graph.InMemoryGraph[uint64, P], value string, ts time.Time) *graph.Topic[uint64] {
	return &graph.Topic[uint64]{NodeAttributes: graph.NodeAttributes{Value: value, Timestamp: ts}, Hasher: g.GetHasher()}
}

func mustSet[P float32 | float64](t *testing.T, g *graph.InMemoryGraph[uint64, P], n graph.Node[uint64]) {
	t.Helper()
	if err := g.Set(n); err != nil {
		t.Fatalf("Set(%q) = %v, want nil", n.GetValue(), err)
	}
}

func values(nodes []*graph.Node[uint64]) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = (*n).GetValue()
	}
	return out
}

func keys(nodes []*graph.Node[uint64]) []uint64 {
	out := make([]uint64, len(nodes))
	for i, n := range nodes {
		out[i] = (*n).Key()
	}
	return out
}

// assertEdgesResolve checks the invariant tying the two halves of an edge
// together: every key the adjacency maps report is a relationship node the graph
// still stores. Both maps are walked because they mirror each other, and a
// deletion that forgets one leaves the graph disagreeing with itself.
func assertEdgesResolve(t *testing.T, g *graph.InMemoryGraph[uint64, float64]) {
	t.Helper()
	for name, edges := range map[string]map[uint64]map[uint64]uint64{
		"AdjacencyMap":   g.AdjacencyMap(),
		"PredecessorMap": g.PredecessorMap(),
	} {
		for from, tos := range edges {
			for to, edge := range tos {
				if g.Get(edge) == nil {
					t.Errorf("%s()[%d][%d] = %d, which is no longer a stored node", name, from, to, edge)
				}
			}
		}
	}
}

func TestInMemoryGraphSetGetPutDelete(t *testing.T) {
	g := newGraph()
	now := time.Now()

	fact := mkFact(g, "alice works at acme", now)
	key := fact.Key()
	mustSet(t, g, fact)

	if err := g.Set(fact); !errors.Is(err, graph.ErrNodeAlreadyExists) {
		t.Errorf("Set on existing key = %v, want ErrNodeAlreadyExists", err)
	}
	if err := g.Set(nil); !errors.Is(err, graph.ErrNilNode) {
		t.Errorf("Set(nil) = %v, want ErrNilNode", err)
	}

	got := g.Get(key)
	if got == nil || got.GetValue() != "alice works at acme" {
		t.Fatalf("Get(key) = %v, want the stored node", got)
	}
	if g.Get(key+1) != nil {
		t.Errorf("Get(missing) != nil, want nil for missing key")
	}

	replacement := mkFact(g, "alice moved to initech", now)
	if err := g.Put(key, replacement); err != nil {
		t.Fatalf("Put = %v, want nil", err)
	}
	if got := g.Get(key); got.GetValue() != "alice moved to initech" {
		t.Errorf("Get(key) after Put = %q, want replaced value", got.GetValue())
	}

	// fact.Key() still equals key, so it addresses whatever Put stored there.
	if err := g.Delete(fact); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if g.Get(key) != nil {
		t.Errorf("Get(key) after Delete != nil, want nil")
	}
	if err := g.Delete(fact); !errors.Is(err, graph.ErrNodeNotFound) {
		t.Errorf("Delete on missing node = %v, want ErrNodeNotFound", err)
	}
}

func TestInMemoryGraphRelationships(t *testing.T) {
	g := newGraph()
	now := time.Now()

	fact := mkFact(g, "alice works at acme", now)
	entity := mkEntity(g, "alice", now)
	mustSet(t, g, fact)
	mustSet(t, g, entity)

	rel := graph.Mentions[uint64]{Fact: &fact, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()}
	mustSet(t, g, rel)

	factKey, entityKey := fact.Key(), entity.Key()

	adj := g.AdjacencyMap()
	if _, ok := adj[factKey][entityKey]; !ok {
		t.Errorf("AdjacencyMap()[fact][entity] missing, want the relationship")
	}
	pred := g.PredecessorMap()
	if _, ok := pred[entityKey][factKey]; !ok {
		t.Errorf("PredecessorMap()[entity][fact] missing, want the relationship")
	}

	if got, want := g.Size(), 1; got != want {
		t.Errorf("Size() = %d, want %d", got, want)
	}
	stats := g.Stats()
	if stats.Size != 1 {
		t.Errorf("Stats().Size = %d, want 1", stats.Size)
	}
	// fact, entity and the relationship are all stored nodes.
	if stats.Nodes != 3 {
		t.Errorf("Stats().Nodes = %d, want 3", stats.Nodes)
	}
	// ...but only the fact and the entity are vertices: the relationship is
	// the edge between them, already counted by Size.
	if stats.Order != 2 || g.Order() != 2 {
		t.Errorf("Stats().Order = %d, Order() = %d, want 2 (the edge is not a vertex)", stats.Order, g.Order())
	}

	// Deleting an endpoint removes the incident edge from both maps.
	if err := g.Delete(entity); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if got := g.Size(); got != 0 {
		t.Errorf("Size() after deleting endpoint = %d, want 0", got)
	}
	if adj := g.AdjacencyMap(); len(adj[factKey]) != 0 {
		t.Errorf("AdjacencyMap()[fact] = %v, want empty after endpoint delete", adj[factKey])
	}
}

// TestInMemoryGraphStoresAFactAndATopicOfTheSameText pins
// `remember 'billing' topic:billing`: the fact, the topic and the IsAbout edge
// between them are three nodes. If the fact and the topic shared a key, Set
// would refuse the topic as an existing node and the edge would run from the
// fact to itself.
func TestInMemoryGraphStoresAFactAndATopicOfTheSameText(t *testing.T) {
	g := newGraph()
	now := time.Now()

	fact := mkFact(g, "billing", now)
	topic := mkTopic(g, "billing", now)
	mustSet(t, g, fact)
	mustSet(t, g, topic)
	mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})

	factKey, topicKey := fact.Key(), topic.Key()
	if factKey == topicKey {
		t.Fatalf("fact and topic of the same text share key %d, want distinct nodes", factKey)
	}
	if got, want := len(g.Nodes()), 3; got != want {
		t.Errorf("len(Nodes()) = %d, want %d (fact, topic and the edge)", got, want)
	}

	adj := g.AdjacencyMap()
	if _, self := adj[factKey][factKey]; self {
		t.Errorf("AdjacencyMap()[fact][fact] exists, want no self-referential edge")
	}
	if _, ok := adj[factKey][topicKey]; !ok {
		t.Errorf("AdjacencyMap()[fact][topic] missing, want the IsAbout edge")
	}
	// The topic must be stored as a topic for anchored recalls to resolve it.
	if got := g.Get(topicKey); got == nil {
		t.Errorf("Get(topic) = nil, want the stored topic node")
	} else if _, isTopic := got.(*graph.Topic[uint64]); !isTopic {
		t.Errorf("Get(topic) = %T, want *graph.Topic", got)
	}
}

// TestInMemoryGraphDeletePrunesIncidentRelationshipNodes checks the whole of
// Delete's contract: the node, its index entries and its incident
// relationships. Unlinking an edge from the adjacency maps is not enough: a
// Mentions node left in idToNodes would describe an edge that no longer
// exists, and Nodes and Stats would keep reporting it.
func TestInMemoryGraphDeletePrunesIncidentRelationshipNodes(t *testing.T) {
	g := newGraph()
	now := time.Now()

	fact := mkFact(g, "alice works at acme", now)
	entity := mkEntity(g, "alice", now)
	topic := mkTopic(g, "work", now)
	mentions := graph.Mentions[uint64]{Fact: &fact, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()}
	about := graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()}
	for _, node := range []graph.Node[uint64]{fact, entity, topic, mentions, about} {
		mustSet(t, g, node)
	}
	if got, want := len(g.Nodes()), 5; got != want {
		t.Fatalf("len(Nodes()) = %d, want %d (three entities and two relationships)", got, want)
	}

	// Deleting the far endpoint of one edge prunes that edge's node and nothing
	// else: the fact's other relationship is not incident to the entity.
	if err := g.Delete(entity); err != nil {
		t.Fatalf("Delete(entity) = %v, want nil", err)
	}
	if g.Get(mentions.Key()) != nil {
		t.Errorf("the Mentions node outlived the entity it pointed at, want it pruned")
	}
	if g.Get(about.Key()) == nil {
		t.Errorf("the IsAbout node was pruned, want only edges incident to the deleted node to go")
	}
	if got, want := len(g.Nodes()), 3; got != want {
		t.Errorf("len(Nodes()) after Delete(entity) = %d, want %d (fact, topic, IsAbout)", got, want)
	}
	if got, want := g.Size(), 1; got != want {
		t.Errorf("Size() after Delete(entity) = %d, want %d (fact -> topic remains)", got, want)
	}
	assertEdgesResolve(t, g)

	// Deleting the fact takes its remaining edge with it.
	if err := g.Delete(fact); err != nil {
		t.Fatalf("Delete(fact) = %v, want nil", err)
	}
	if g.Get(about.Key()) != nil {
		t.Errorf("the IsAbout node outlived its fact, want it pruned")
	}
	if keys, _, err := g.GetTextIndex().Search("acme", 0); err == nil && len(keys) != 0 {
		t.Errorf("text Search(acme) after Delete(fact) = %v, want no hits", keys)
	}
	if got, want := len(g.Nodes()), 1; got != want {
		t.Errorf("len(Nodes()) after Delete(fact) = %d, want %d (the topic alone)", got, want)
	}
	if got := g.Size(); got != 0 {
		t.Errorf("Size() = %d, want 0", got)
	}
	assertEdgesResolve(t, g)
}

func TestInMemoryGraphIndexes(t *testing.T) {
	g := newGraph()
	now := time.Now()
	fact := mkFact(g, "alice works at acme", now)
	key := fact.Key()
	mustSet(t, g, fact)

	// Set must index the node's value in the text index.
	keys, _, err := g.GetTextIndex().Search("acme", 0)
	if err != nil || len(keys) != 1 || keys[0] != key {
		t.Errorf("text Search(acme) = (%v, %v), want ([key], nil)", keys, err)
	}

	// The vector index adopts the dimension of the first inserted embedding.
	if err := g.GetVectorIndex().Insert(key, containers.NewVector[uint64]([]float64{1, 0, 0})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}
	got, _, err := g.GetVectorIndex().Search(containers.NewVector[uint64]([]float64{1, 0, 0}), 1)
	if err != nil || len(got) != 1 || got[0] != key {
		t.Errorf("vector Search = (%v, %v), want ([key], nil)", got, err)
	}

	// Deleting the node clears both indexes.
	if err := g.Delete(fact); err != nil {
		t.Fatalf("Delete = %v, want nil", err)
	}
	if keys, _, err := g.GetTextIndex().Search("acme", 0); err == nil && len(keys) != 0 {
		t.Errorf("text Search(acme) after delete = %v, want no hits", keys)
	}
	if got := g.GetVectorIndex().Count(); got != 0 {
		t.Errorf("vector Count() after delete = %d, want 0", got)
	}
}

func TestInMemoryGraphSearchByKeywords(t *testing.T) {
	g := newGraph()
	now := time.Now()

	fact1 := mkFact(g, "alice works at acme", now)
	fact2 := mkFact(g, "bob plays tennis", now)
	fact3 := mkFact(g, "alice lives in paris", now)
	entity := mkEntity(g, "alice", now)
	mustSet(t, g, fact1)
	mustSet(t, g, fact2)
	mustSet(t, g, fact3)
	mustSet(t, g, entity)
	// fact1 and fact3 share the alice entity; fact2 is unconnected.
	mustSet(t, g, graph.Mentions[uint64]{Fact: &fact1, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	mustSet(t, g, graph.Mentions[uint64]{Fact: &fact3, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})

	nodes, scores, _, _, _ := g.Search([]string{"acme"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alice works at acme" {
		t.Fatalf("Search(acme) = %v, want [alice works at acme]", values(nodes))
	}
	if len(scores) != 1 || scores[0] <= 0 {
		t.Errorf("Search(acme) scores = %v, want one positive score", scores)
	}

	// The shared entity is the only anchor the query touches, so its fair
	// share is all of its observed mass: it holds no surplus, and the
	// entity-linked fact does not ride in on mere reachability. With the
	// entity named and depth 2, the traversal runs and still funds nothing.
	g.SetTraversal(graph.NewExcessTraversal[uint64, float64]())
	nodes, _, _, _, _ = g.Search([]string{"acme"}, containers.Vector[uint64, float64]{}, nil, []string{"alice"}, 2, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alice works at acme" {
		t.Errorf("Search(acme, depth=2) = %v, want only the direct hit — a lone anchor has no surplus to transmit", values(nodes))
	}
}

func TestInMemoryGraphSearchByVector(t *testing.T) {
	g := newGraph()
	now := time.Now()
	fact1 := mkFact(g, "alpha", now)
	fact2 := mkFact(g, "beta", now)
	mustSet(t, g, fact1)
	mustSet(t, g, fact2)

	if err := g.GetVectorIndex().Insert(fact1.Key(), containers.NewVector[uint64]([]float64{1, 0})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}
	if err := g.GetVectorIndex().Insert(fact2.Key(), containers.NewVector[uint64]([]float64{0, 1})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}

	nodes, _, _, _, _ := g.Search(nil, containers.NewVector[uint64]([]float64{0.9, 0.1}), nil, nil, 0, 1, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alpha" {
		t.Errorf("Search(vector near alpha) = %v, want [alpha]", values(nodes))
	}
}

// TestInMemoryGraphSearchRejectsAVectorOfTheWrongDimension pins Search's
// error contract. A vector whose width differs from the one the index was
// built at is a question asked in another embedding model: Search returns
// index.ErrInvalidDimension instead of quietly answering from the text index
// alone. A graph with no vectors yet has no dimension to disagree with, so
// there the vector seeds nothing and the call is not an error.
func TestInMemoryGraphSearchRejectsAVectorOfTheWrongDimension(t *testing.T) {
	g := newGraph()
	fact := mkFact(g, "alpha", time.Now())
	mustSet(t, g, fact)

	narrow := containers.NewVector[uint64]([]float64{1, 0})
	if _, _, _, _, err := g.Search([]string{"alpha"}, narrow, nil, nil, 0, 10, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Search on a graph with no vectors = %v, want nil: there is no dimension to disagree with", err)
	}

	if err := g.GetVectorIndex().Insert(fact.Key(), containers.NewVector[uint64]([]float64{1, 0, 0})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}
	nodes, _, _, _, err := g.Search([]string{"alpha"}, narrow, nil, nil, 0, 10, time.Time{}, time.Time{})
	if !errors.Is(err, index.ErrInvalidDimension) {
		t.Fatalf("Search(2-d vector on a 3-d index) = %v, want ErrInvalidDimension", err)
	}
	if nodes != nil {
		t.Errorf("Search returned hits %v alongside the error, want none: a text-only answer is what the error replaces", values(nodes))
	}
}

func TestInMemoryGraphSearchTopicFilter(t *testing.T) {
	g := newGraph()
	now := time.Now()

	fact1 := mkFact(g, "alice works at acme", now)
	fact2 := mkFact(g, "alice plays tennis", now)
	topic := mkTopic(g, "work", now)
	mustSet(t, g, fact1)
	mustSet(t, g, fact2)
	mustSet(t, g, topic)
	mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact1, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})

	// Both facts match "alice", but only fact1 is tagged with topic "work".
	nodes, _, _, _, _ := g.Search([]string{"alice"}, containers.Vector[uint64, float64]{}, []string{"work"}, nil, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alice works at acme" {
		t.Errorf("Search(alice, topic=work) = %v, want [alice works at acme]", values(nodes))
	}
}

func TestInMemoryGraphSearchTimeFilter(t *testing.T) {
	g := newGraph()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mustSet(t, g, mkFact(g, "alice ancient fact", old))
	mustSet(t, g, mkFact(g, "alice recent fact", recent))

	nodes, _, _, _, _ := g.Search([]string{"alice"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alice recent fact" {
		t.Errorf("Search(alice, since=2025) = %v, want [alice recent fact]", values(nodes))
	}

	nodes, _, _, _, _ = g.Search([]string{"alice"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alice ancient fact" {
		t.Errorf("Search(alice, until=2025) = %v, want [alice ancient fact]", values(nodes))
	}
}

// TestInMemoryGraphSearchRecencyDecayFactor pins the decay formula the README
// promises ("recent memories outrank older ones"): a fact's score is
// multiplied by 0.5^(age/half-life). A lone fact in a one-document corpus has
// a hand-derivable BM25 mass, idf ln(1 + 0.5/1.5) at length norm 1, scaled by
// the coverage the index hands Finalize for a fully matched one-term query.
// Coverage is the matched idf mass over the query's in 1/1024 fixed point,
// here ⌊1024·idf⌋/(⌊1024·idf⌋+1) = 294/295. The decay factor is the score's
// ratio to that mass.
func TestInMemoryGraphSearchRecencyDecayFactor(t *testing.T) {
	halflife := testConfig().Engine.Halflife // default 90d

	cases := []struct {
		name string
		age  time.Duration
		want float64
	}{
		{"fresh fact keeps its relevance", 0, 1.0},
		{"one half-life halves the score", halflife, 0.5},
		{"two half-lives quarter it", 2 * halflife, 0.25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newGraph()
			mustSet(t, g, mkFact(g, "aurora over the fjord", time.Now().Add(-tc.age)))

			_, scores, _, _, _ := g.Search([]string{"aurora"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
			if len(scores) != 1 {
				t.Fatalf("Search returned %d scores, want 1", len(scores))
			}
			// The fact ages a hair between Set and Search, so allow a small
			// tolerance around the exact factor.
			idf := math.Log(1 + 0.5/1.5)
			covered := float64(int(idf * 1024))
			preDecay := idf * covered / (covered + 1)
			if diff := scores[0]/preDecay - tc.want; diff > 1e-3 || diff < -1e-3 {
				t.Errorf("score = %v, want ~%v of the BM25 mass %v (0.5^(age/half-life))", scores[0], tc.want, preDecay)
			}
		})
	}
}

// constScorer is a minimal alternative fold: every candidate scores the same
// constant, whatever the background. It shows that a new fold is a Scorer
// installed with SetScorer, not a change to Search, and like fakeHasher it is
// a real, deterministic implementation of an in-tree interface.
type constScorer struct{ score float64 }

func (s constScorer) Score([]scoring.Contribution[uint64, float64]) float64 { return s.score }

func (s constScorer) WithBackground(float64) scoring.Scorer[uint64, float64] { return s }

// TestNewGraphInstallsStemmingTokenizer pins the tokenizer wiring: recall
// keywords rarely arrive in the fact's exact inflection, so NewGraph installs
// the stemming tokenizer and a query for "blogging" finds the fact written
// with "blogs".
func TestNewGraphInstallsStemmingTokenizer(t *testing.T) {
	g := newGraph()
	mustSet(t, g, mkFact(g, "jules blogs about lighthouses", time.Now()))

	nodes, _, _, _, _ := g.Search([]string{"blogging"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "jules blogs about lighthouses" {
		t.Fatalf("Search(blogging) = %v, want the blogs fact — inflections must unify", values(nodes))
	}
}

// TestNewGraphSelectsConfiguredRelevanceModel pins the relevance-model
// wiring: under "matchcount" a lone single-term match scores exactly 1 (one
// point per query-term occurrence), where the default "bm25" gives the same
// fact its idf mass (pinned by the decay tests). Decay is off so the score is
// the model's alone.
func TestNewGraphSelectsConfiguredRelevanceModel(t *testing.T) {
	cfg := testConfig()
	cfg.Engine.Halflife = 0
	cfg.DB.RelevanceModel.Name = "matchcount"
	g := graph.NewGraph[uint64, float64](cfg)
	mustSet(t, g, mkFact(g, "aurora over the fjord", time.Now()))

	_, scores, _, _, _ := g.Search([]string{"aurora"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(scores) != 1 || scores[0] != 1 {
		t.Fatalf("Search scores under matchcount = %v, want exactly [1]", scores)
	}
}

// TestSetScorerInstallsFold pins the scoring seam: the fold Search derives
// relevance with is the installed Scorer. With decay disabled the constant
// fold's output reaches the caller untouched.
func TestSetScorerInstallsFold(t *testing.T) {
	cfg := testConfig()
	cfg.Engine.Halflife = 0
	g := graph.NewGraph[uint64, float64](cfg)
	g.SetScorer(constScorer{score: 7})
	mustSet(t, g, mkFact(g, "aurora over the fjord", time.Now()))

	_, scores, _, _, _ := g.Search([]string{"aurora"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(scores) != 1 || scores[0] != 7 {
		t.Fatalf("Search scores = %v, want the installed fold's [7]", scores)
	}
}

// TestInMemoryGraphSearchRecencyOrdersTies checks the user-visible promise:
// of two facts matching the same term, the recent one outranks the much older
// one. The old fact is aged ten half-lives, so decay outweighs any difference
// in their text scores and the order cannot depend on how the index breaks
// ties.
func TestInMemoryGraphSearchRecencyOrdersTies(t *testing.T) {
	g := newGraph()
	halflife := testConfig().Engine.Halflife
	mustSet(t, g, mkFact(g, "comet sighted in march", time.Now().Add(-10*halflife)))
	mustSet(t, g, mkFact(g, "comet sighted today", time.Now()))

	nodes, scores, _, _, _ := g.Search([]string{"comet"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 {
		t.Fatalf("Search(comet) returned %d nodes, want 2", len(nodes))
	}
	if (*nodes[0]).GetValue() != "comet sighted today" {
		t.Errorf("Search(comet) ranked %q first, want the recent fact", (*nodes[0]).GetValue())
	}
	if scores[0] <= scores[1] {
		t.Errorf("recent fact score %v not above old fact score %v", scores[0], scores[1])
	}
}

// TestInMemoryGraphSearchRecencyKeepsRelevance pins what the default
// half-life is for: decay multiplies the whole score, so the half-life decides
// whether a month of age outweighs matching the query. A fact matching both
// query terms, written 30 days ago, faces one matching a single term, written
// now; two non-matching facts keep the terms' idf apart. Under the default the
// full match ranks first. Under a one-week half-life it keeps 0.5^(30/7) ≈ 5%
// of its score and the weak match overtakes it: on a graph used for months,
// nothing older than a few weeks would rank.
func TestInMemoryGraphSearchRecencyKeepsRelevance(t *testing.T) {
	cases := []struct {
		name     string
		halflife time.Duration
		want     []string
	}{
		{"the default keeps the full match first", testConfig().Engine.Halflife, []string{"glacier survey from the north ridge", "glacier sighted today"}},
		{"a one-week half-life lets the weak match win", 7 * 24 * time.Hour, []string{"glacier sighted today", "glacier survey from the north ridge"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.Engine.Halflife = tc.halflife
			g := graph.NewGraph[uint64, float64](cfg)
			now := time.Now()
			mustSet(t, g, mkFact(g, "glacier survey from the north ridge", now.Add(-30*24*time.Hour)))
			mustSet(t, g, mkFact(g, "glacier sighted today", now))
			mustSet(t, g, mkFact(g, "harbour cranes at dawn", now))
			mustSet(t, g, mkFact(g, "pilot boat on watch", now))

			nodes, scores, _, _, _ := g.Search([]string{"glacier", "survey"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
			if got := values(nodes); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Search(glacier survey, half-life=%v) = %v (scores %v), want %v", tc.halflife, got, scores, tc.want)
			}
		})
	}
}

// TestInMemoryGraphSearchDecayDisabled checks that a non-positive half-life
// switches decay off: an old fact keeps its full relevance score — its BM25
// mass in a one-document corpus at the 294/295 fixed-point coverage of a
// fully-matched one-term query, untouched by age.
func TestInMemoryGraphSearchDecayDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.Engine.Halflife = 0
	g := graph.NewGraph[uint64, float64](cfg)
	mustSet(t, g, mkFact(g, "glacier survey notes", time.Now().Add(-365*24*time.Hour)))

	_, scores, _, _, _ := g.Search([]string{"glacier"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(scores) != 1 {
		t.Fatalf("Search returned %d scores, want 1", len(scores))
	}
	idf := math.Log(1 + 0.5/1.5)
	covered := float64(int(idf * 1024))
	if want := idf * covered / (covered + 1); scores[0] != want {
		t.Errorf("score with decay disabled = %v, want exactly the BM25 mass %v", scores[0], want)
	}
}

// TestInMemoryGraphSearchOrdersTiesByKey pins the total order Search ranks by:
// score descending, then key ascending. Three facts match the query term once
// each with identical term frequency and length, so their BM25 masses and
// their decay (one shared timestamp) tie to the bit; a fourth matches twice
// and ranks first. Without the key tiebreak the trio comes back in whatever
// order the score map was iterated in, and top truncates that arbitrary
// order. Each case repeats the query because that map order changes between
// calls: a single pass can agree by luck.
func TestInMemoryGraphSearchOrdersTiesByKey(t *testing.T) {
	g := newGraph()
	now := time.Now()

	first := mkFact(g, "acme acme prime", now)
	tiedA := mkFact(g, "acme alpha line", now)
	tiedB := mkFact(g, "acme beta line", now)
	tiedC := mkFact(g, "acme gamma line", now)
	for _, node := range []graph.Node[uint64]{first, tiedA, tiedB, tiedC} {
		mustSet(t, g, node)
	}

	// The premise of the test: the three single-match facts score equally to
	// the bit, so nothing but the key can order them.
	if _, scores, _, _, _ := g.Search([]string{"acme"}, containers.Vector[uint64, float64]{}, nil, nil, 1, 10, time.Time{}, time.Time{}); len(scores) != 4 || scores[1] != scores[2] || scores[2] != scores[3] {
		t.Fatalf("Search(acme) scores = %v, want four hits whose last three tie", scores)
	}

	tied := []uint64{tiedA.Key(), tiedB.Key(), tiedC.Key()}
	sort.Slice(tied, func(i, j int) bool { return tied[i] < tied[j] })

	cases := []struct {
		name string
		top  int
		want []uint64
	}{
		{"tied facts follow the top hit in key order", 10, []uint64{first.Key(), tied[0], tied[1], tied[2]}},
		{"truncation keeps the lowest tied key", 2, []uint64{first.Key(), tied[0]}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 20; i++ {
				nodes, _, _, _, _ := g.Search([]string{"acme"}, containers.Vector[uint64, float64]{}, nil, nil, 1, tc.top, time.Time{}, time.Time{})
				if got := keys(nodes); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("Search(acme, top=%d) keys = %v on call %d, want %v every call (%v)", tc.top, got, i+1, tc.want, values(nodes))
				}
			}
		})
	}
}

func TestInMemoryGraphSearchTopTruncation(t *testing.T) {
	g := newGraph()
	now := time.Now()
	mustSet(t, g, mkFact(g, "shared term one", now))
	mustSet(t, g, mkFact(g, "shared term two", now))
	mustSet(t, g, mkFact(g, "shared term three", now))

	nodes, scores, _, _, _ := g.Search([]string{"shared"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 2, time.Time{}, time.Time{})
	if len(nodes) != 2 || len(scores) != 2 {
		t.Errorf("Search(top=2) returned %d nodes and %d scores, want 2 and 2", len(nodes), len(scores))
	}
	if scores[0] < scores[1] {
		t.Errorf("scores not descending: %v", scores)
	}
}

// TestInMemoryGraphSearchScoreCutoff pins db.min-score-ratio through the
// public surface. The fixture is an anchor-only recall with decay off, so
// every score is exactly the number of named anchors a fact is filed under —
// 3, 2, 1, 1 — and the bar each ratio sets is known to the bit. Off (the
// default) is the first case on purpose: the cutoff must change nothing until
// an operator asks for it.
//
// The table runs at both precisions because the bar is a product computed in
// P and "exactly at the bar" is a rounding question: float32 is what the
// server ships with (config.DefaultPrecision), and a pin that only holds in
// float64 would not be testing the contract an operator gets.
func TestInMemoryGraphSearchScoreCutoff(t *testing.T) {
	t.Run("float32", scoreCutoffAt[float32])
	t.Run("float64", scoreCutoffAt[float64])
}

func scoreCutoffAt[P float32 | float64](t *testing.T) {
	cfg := testConfig()
	cfg.Engine.Halflife = 0
	g := graph.NewGraph[uint64, P](cfg)
	now := time.Now()

	topics := map[string]*graph.Topic[uint64]{}
	for _, name := range []string{"a", "b", "c"} {
		topics[name] = mkTopic(g, name, now)
		mustSet(t, g, topics[name])
	}
	file := func(value string, under ...string) {
		fact := mkFact(g, value, now)
		mustSet(t, g, fact)
		for _, name := range under {
			mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topics[name], NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
		}
	}
	file("three anchors", "a", "b", "c")
	file("two anchors", "a", "b")
	file("one anchor", "a")
	file("another one anchor", "a")

	// The premise of the test: the scores are 3, 2, 1, 1 before any cutoff.
	if _, scores, _, _, _ := g.Search(nil, containers.Vector[uint64, P]{}, []string{"a", "b", "c"}, nil, 0, 10, time.Time{}, time.Time{}); !reflect.DeepEqual(scores, []P{3, 2, 1, 1}) {
		t.Fatalf("Search(topics a b c) scores = %v, want [3 2 1 1]", scores)
	}

	cases := []struct {
		name       string
		ratio      float64
		top        int
		wantScores []P
	}{
		{"off keeps the whole list", 0, 10, []P{3, 2, 1, 1}},
		{"ratio drops the tail under the bar", 0.5, 10, []P{3, 2}},
		{"a hit exactly at the bar is kept", 1.0 / 3, 10, []P{3, 2, 1, 1}},
		{"the cut falls just under a hit on the bar", 2.0 / 3, 10, []P{3, 2}},
		{"the best hit survives any ratio", 1, 10, []P{3}},
		{"top still caps the list", 0.1, 2, []P{3, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg.DB.MinScoreRatio = tc.ratio
			nodes, scores, contributions, _, err := g.Search(nil, containers.Vector[uint64, P]{}, []string{"a", "b", "c"}, nil, 0, tc.top, time.Time{}, time.Time{})
			if err != nil {
				t.Fatalf("Search() error = %v, want nil", err)
			}
			if !reflect.DeepEqual(scores, tc.wantScores) {
				t.Fatalf("Search(ratio=%v, top=%d) scores = %v, want %v (%v)", tc.ratio, tc.top, scores, tc.wantScores, values(nodes))
			}
			if len(nodes) != len(scores) || len(contributions) != len(scores) {
				t.Fatalf("Search() returned %d nodes and %d contribution lists for %d scores, want parallel slices", len(nodes), len(contributions), len(scores))
			}
		})
	}
}

// TestInMemoryGraphSearchScoreCutoffIgnoresDecay pins that the bar is set on
// relevance, not on the decayed score the list is ordered by. Two facts are
// filed under the same two anchors (relevance 2 each), one fresh and one two
// half-lives old (decayed score 0.5), next to a fresh fact under one anchor
// (relevance 1, score 1). The list is ordered by decayed score — fresh pair,
// single, old pair — but the cutoff reads the evidence: at 0.3 the old fact
// clears the bar its 0.5 score would have missed; at 0.6 the single-anchor
// fact is dropped although it outscores the old fact that stays. Measured
// after decay, the documented 0.3 would turn into a recency window and 0.6
// would keep the weaker evidence over the stronger.
func TestInMemoryGraphSearchScoreCutoffIgnoresDecay(t *testing.T) {
	cfg := testConfig()
	g := graph.NewGraph[uint64, float64](cfg)
	now := time.Now()
	old := now.Add(-2 * cfg.Engine.Halflife)

	topics := map[string]*graph.Topic[uint64]{}
	for _, name := range []string{"a", "b"} {
		topics[name] = mkTopic(g, name, now)
		mustSet(t, g, topics[name])
	}
	file := func(value string, ts time.Time, under ...string) {
		fact := mkFact(g, value, ts)
		mustSet(t, g, fact)
		for _, name := range under {
			mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topics[name], NodeAttributes: graph.NodeAttributes{Timestamp: ts}, Hasher: g.GetHasher()})
		}
	}
	file("fresh pair", now, "a", "b")
	file("old pair", old, "a", "b")
	file("fresh single", now, "a")

	cases := []struct {
		name  string
		ratio float64
		want  []string
	}{
		{"off keeps the whole list", 0, []string{"fresh pair", "fresh single", "old pair"}},
		{"the same evidence clears the bar at any age", 0.3, []string{"fresh pair", "fresh single", "old pair"}},
		{"weaker evidence is cut even when it outscores what stays", 0.6, []string{"fresh pair", "old pair"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg.DB.MinScoreRatio = tc.ratio
			nodes, scores, _, _, err := g.Search(nil, containers.Vector[uint64, float64]{}, []string{"a", "b"}, nil, 0, 10, time.Time{}, time.Time{})
			if err != nil {
				t.Fatalf("Search() error = %v, want nil", err)
			}
			if got := values(nodes); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Search(ratio=%v) = %v (scores %v), want %v", tc.ratio, got, scores, tc.want)
			}
			if !sort.SliceIsSorted(scores, func(i, j int) bool { return scores[i] > scores[j] }) {
				t.Errorf("Search(ratio=%v) scores = %v, want the decay order kept", tc.ratio, scores)
			}
		})
	}
}

// noDecayGraph builds a graph whose scores are pure relevance (decay off,
// excess traversal installed), so the floor and determinism pins can assert
// exact equalities.
func noDecayGraph() *graph.InMemoryGraph[uint64, float64] {
	cfg := testConfig()
	cfg.Engine.Halflife = 0
	g := graph.NewGraph[uint64, float64](cfg)
	g.SetTraversal(graph.NewExcessTraversal[uint64, float64]())
	return g
}

// stormGraph is the behavioural transmission fixture (the barometer/storm
// probe): a small "weather" cluster holding most of the query's seed mass
// next to a large "archive" hub holding a little. Querying "barometer storm"
// touches both anchors; only the cluster runs above background.
//
//	weather (degree 3): "the barometer falls before the storm",
//	                    "storm clouds gather at sea",
//	                    "the harbour is calm tonight"      <- no query term
//	archive (degree 8): "a storm of paperwork" + 7 unrelated memos
func stormGraph(t *testing.T, g *graph.InMemoryGraph[uint64, float64]) (calm string, memos []string) {
	t.Helper()
	now := time.Now()
	calm = "the harbour is calm tonight"

	link := func(topic *graph.Topic[uint64], value string) {
		fact := mkFact(g, value, now)
		if err := g.Set(fact); err != nil {
			t.Fatalf("Set(%q) = %v, want nil", value, err)
		}
		mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	}

	weather := mkTopic(g, "weather", now)
	mustSet(t, g, weather)
	link(weather, "the barometer falls before the storm")
	link(weather, "storm clouds gather at sea")
	link(weather, calm)

	archive := mkTopic(g, "archive", now)
	mustSet(t, g, archive)
	link(archive, "a storm of paperwork")
	for i := 0; i < 7; i++ {
		memo := "unrelated archive memo " + string(rune('a'+i))
		memos = append(memos, memo)
		link(archive, memo)
	}
	return calm, memos
}

// TestSearchBM25Floor pins the BM25 floor through the public surface: when the
// query touches a single anchor, its fair share is all of its observed mass and
// it holds no surplus, so the ranking and the scores are exactly the text
// index's. Transmission can add to a text score but never lower it.
func TestSearchBM25Floor(t *testing.T) {
	g := noDecayGraph()
	now := time.Now()
	topic := mkTopic(g, "harbour", now)
	mustSet(t, g, topic)
	for _, value := range []string{
		"the comet streaked past the mast",
		"a comet is drawn in the log",
		"children watched the comet at dawn from the pier deck",
	} {
		fact := mkFact(g, value, now)
		mustSet(t, g, fact)
		mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	}

	nodes, scores, _, _, _ := g.Search([]string{"comet"}, containers.Vector[uint64, float64]{}, []string{"harbour"}, nil, 1, 10, time.Time{}, time.Time{})
	textKeys, textScores, err := g.GetTextIndex().Search("comet", 10)
	if err != nil {
		t.Fatalf("text Search = %v, want nil", err)
	}
	if got := keys(nodes); !reflect.DeepEqual(got, textKeys) {
		t.Errorf("Search ranking %v differs from the text index's %v with no surplus anywhere", got, textKeys)
	}
	if !reflect.DeepEqual(scores, textScores) {
		t.Errorf("Search scores %v differ from the text index's %v — a silent anchor added mass", scores, textScores)
	}
}

// TestSearchScoresNeverBelowTextMass is the floor's other half: where
// transmission does happen, it only adds, so every hit's relevance is at least
// its own text mass.
func TestSearchScoresNeverBelowTextMass(t *testing.T) {
	g := noDecayGraph()
	stormGraph(t, g)

	nodes, scores, _, _, _ := g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"weather", "archive"}, nil, 1, 20, time.Time{}, time.Time{})
	textKeys, textScores, err := g.GetTextIndex().Search("barometer storm", 20)
	if err != nil {
		t.Fatalf("text Search = %v, want nil", err)
	}
	textMass := make(map[uint64]float64, len(textKeys))
	for i, key := range textKeys {
		textMass[key] = textScores[i]
	}
	for i, node := range nodes {
		if m := textMass[(*node).Key()]; scores[i] < m {
			t.Errorf("hit %q scored %v below its own text mass %v", (*node).GetValue(), scores[i], m)
		}
	}
}

// TestSearchHubSilenceAndEarnedPreemption pins hub silence and earned
// preemption through the public surface. The weather cluster concentrates the
// query's mass, so its silent member surfaces on transmitted surplus alone,
// earned by above-background evidence rather than reachability. The archive
// hub also holds a matching fact, but at its size that mass is within its fair
// share, so its seven memos must not ride in behind it.
func TestSearchHubSilenceAndEarnedPreemption(t *testing.T) {
	g := noDecayGraph()
	calm, memos := stormGraph(t, g)

	nodes, _, contributions, background, _ := g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"weather", "archive"}, nil, 2, 20, time.Time{}, time.Time{})
	if background <= 0 {
		t.Fatalf("background = %v, want positive: two anchors are touched", background)
	}
	got := values(nodes)

	found := -1
	for i, value := range got {
		if value == calm {
			found = i
		}
		for _, memo := range memos {
			if value == memo {
				t.Errorf("hub memo %q surfaced — a fair-share hub transmitted", value)
			}
		}
	}
	if found == -1 {
		t.Fatalf("Search = %v, want the cluster's silent member %q funded by surplus", got, calm)
	}
	// The funded member carries exactly one observation: its cluster's mass,
	// through the weather anchor.
	list := contributions[found]
	if len(list) != 1 || list[0].Src != scoring.SrcGraph || list[0].Score <= 0 || list[0].Degree != 3 {
		t.Errorf("silent member's contributions = %+v, want a single weather-cluster observation", list)
	}
}

// marginalGraph builds the case that separates the two transmitting lanes: an
// anchor holding more than its fair share of the query's mass, but less than
// twice it. depth 2 admits it at the plain fair share; depth 1 raises the bar
// to depthOneAdmission x fair share and turns it away.
//
//	harbour (degree 4): 3 "tide" facts + "the lamps are lit at dusk"  <- no query term
//	ledger  (degree 4): 1 "tide" fact  + 3 unrelated entries
//
// With four seeds of comparable mass m, the background is 4m/8 = m/2, so the
// harbour's fair share is 2m against an observed 3m: above 1x, below 2x.
func marginalGraph(t *testing.T, g *graph.InMemoryGraph[uint64, float64]) (silent string) {
	t.Helper()
	now := time.Now()
	silent = "the lamps are lit at dusk"

	link := func(topic *graph.Topic[uint64], value string) {
		fact := mkFact(g, value, now)
		if err := g.Set(fact); err != nil {
			t.Fatalf("Set(%q) = %v, want nil", value, err)
		}
		mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	}

	harbour := mkTopic(g, "harbour", now)
	mustSet(t, g, harbour)
	link(harbour, "the tide turns before dawn")
	link(harbour, "a high tide floods the slipway")
	link(harbour, "the tide leaves the mudflats bare")
	link(harbour, silent)

	ledger := mkTopic(g, "ledger", now)
	mustSet(t, g, ledger)
	link(ledger, "the tide of invoices never stops")
	for _, value := range []string{
		"an entry for the quarterly audit",
		"an entry for the annual return",
		"an entry for the petty cash",
	} {
		link(ledger, value)
	}
	return silent
}

// TestSearchDepthLanes pins the three depth lanes on a cluster whose anchor is
// strongly above chance. depth 0 is the floor: the traversal is skipped, so
// the background is 0, no SrcGraph contribution is recorded, and the silent
// member the other lanes fund is not surfaced. depth 1 and depth 2 both run
// the anchor-mediated round, and the weather cluster clears even the
// precision bar, so both fund it. TestSearchDepthOnePrecisionBar separates the
// two bars.
func TestSearchDepthLanes(t *testing.T) {
	g := noDecayGraph()
	calm, _ := stormGraph(t, g)
	contains := func(vals []string, want string) bool {
		for _, v := range vals {
			if v == want {
				return true
			}
		}
		return false
	}
	search := func(depth int) ([]string, [][]scoring.Contribution[uint64, float64], float64) {
		nodes, _, contribs, bg, _ := g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"weather", "archive"}, nil, depth, 20, time.Time{}, time.Time{})
		return values(nodes), contribs, bg
	}

	// depth 0: floor — traversal skipped, graph channel off.
	vals0, contribs0, bg0 := search(0)
	if bg0 != 0 {
		t.Errorf("depth 0 background = %v, want 0: the floor lane runs no traversal", bg0)
	}
	for _, list := range contribs0 {
		for _, c := range list {
			if c.Src == scoring.SrcGraph {
				t.Errorf("depth 0 recorded a graph contribution %+v: the floor lane funds nothing", c)
			}
		}
	}
	if contains(vals0, calm) {
		t.Errorf("depth 0 surfaced silent member %q: the floor lane transmits no mass", calm)
	}

	// depth 1 and 2: both traverse, and this anchor clears both bars.
	for _, depth := range []int{1, 2} {
		vals, _, bg := search(depth)
		if bg <= 0 {
			t.Errorf("depth %d background = %v, want positive: the lane observes anchors", depth, bg)
		}
		if !contains(vals, calm) {
			t.Errorf("depth %d did not fund silent member %q: a strongly above-chance anchor transmits in both lanes", depth, calm)
		}
	}
}

// TestSearchDepthOnePrecisionBar pins what makes depth 1 a distinct lane
// rather than a synonym for depth 2: an anchor between its fair share and
// twice it is evidence, but not strong evidence. depth 2 (max recall) admits
// it and funds its silent member; depth 1 (precision) turns it away, so the
// same query returns only what the text index matched. With depthOneAdmission
// at 1, this test fails.
func TestSearchDepthOnePrecisionBar(t *testing.T) {
	g := noDecayGraph()
	silent := marginalGraph(t, g)
	contains := func(vals []string, want string) bool {
		for _, v := range vals {
			if v == want {
				return true
			}
		}
		return false
	}
	search := func(depth int) []string {
		nodes, _, _, _, _ := g.Search([]string{"tide"}, containers.Vector[uint64, float64]{}, []string{"harbour", "ledger"}, nil, depth, 20, time.Time{}, time.Time{})
		return values(nodes)
	}

	if got := search(1); contains(got, silent) {
		t.Errorf("depth 1 funded %q: a marginally above-chance anchor must not clear the precision bar; got %v", silent, got)
	}
	if got := search(2); !contains(got, silent) {
		t.Errorf("depth 2 did not fund %q: it clears the plain fair share; got %v", silent, got)
	}
}

// TestSearchFairSeeding pins fair seeding: the text candidate budget tracks the
// requested result size. Fifteen facts match; with seed-size at its default
// of 10, top:15 must still return all fifteen; a budget capped at seed-size
// would cut every ranking off at ten.
func TestSearchFairSeeding(t *testing.T) {
	g := noDecayGraph()
	now := time.Now()
	for i := 0; i < 15; i++ {
		mustSet(t, g, mkFact(g, "lighthouse entry "+string(rune('a'+i)), now))
	}

	nodes, _, _, _, _ := g.Search([]string{"lighthouse"}, containers.Vector[uint64, float64]{}, nil, nil, 1, 15, time.Time{}, time.Time{})
	if len(nodes) != 15 {
		t.Fatalf("Search(top=15) returned %d hits, want all 15 — the seed budget must widen to top", len(nodes))
	}
}

// TestSearchAnchorsDoNotConsumeSeedBudget is fair seeding's other half: the
// seed budget goes to facts. An anchor named like the query term would be the
// strongest text seed, since BM25's length norm favours a one- or two-word
// document, yet it can return nothing: its neighbours are facts, so
// ExcessTraversal finds no anchors from it, and timeFilter drops it before it
// can be a hit. Twelve facts match "billing" and top is twelve, so all twelve
// must come back; if anchors were indexed, the Topic and the NamedEntity would
// take two of the twelve seed slots and the search would return ten.
func TestSearchAnchorsDoNotConsumeSeedBudget(t *testing.T) {
	g := noDecayGraph()
	now := time.Now()

	topic := mkTopic(g, "billing", now)
	mustSet(t, g, topic)
	entity := mkEntity(g, "billing team", now)
	mustSet(t, g, entity)
	for i := 0; i < 12; i++ {
		fact := mkFact(g, "the billing note "+string(rune('a'+i)), now)
		mustSet(t, g, fact)
		mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
		mustSet(t, g, graph.Mentions[uint64]{Fact: &fact, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	}

	seeds, _, err := g.GetTextIndex().Search("billing", 12)
	if err != nil {
		t.Fatalf("text Search(billing) = %v, want nil", err)
	}
	for _, key := range seeds {
		if node := g.Get(key); node.Key() == topic.Key() || node.Key() == entity.Key() {
			t.Errorf("text index seeded the anchor %q, want facts only — it can neither transmit nor be a hit", node.GetValue())
		}
	}

	nodes, _, _, _, _ := g.Search([]string{"billing"}, containers.Vector[uint64, float64]{}, nil, nil, 1, 12, time.Time{}, time.Time{})
	if len(nodes) != 12 {
		t.Fatalf("Search(top=12) returned %d hits, want all 12 — anchors took seed slots no fact could then use", len(nodes))
	}
}

// TestSearchDeterminism pins determinism: with no randomness and no iteration
// to convergence, two identical queries on an identical graph return
// byte-identical rankings and, with decay off, byte-identical scores and
// background.
func TestSearchDeterminism(t *testing.T) {
	g := noDecayGraph()
	stormGraph(t, g)

	firstNodes, firstScores, _, firstBackground, _ := g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"weather", "archive"}, nil, 1, 20, time.Time{}, time.Time{})
	for i := 0; i < 20; i++ {
		nodes, scores, _, background, _ := g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"weather", "archive"}, nil, 1, 20, time.Time{}, time.Time{})
		if !reflect.DeepEqual(keys(nodes), keys(firstNodes)) || !reflect.DeepEqual(scores, firstScores) || background != firstBackground {
			t.Fatalf("call %d returned a different ranking, scores or background: %v %v %v vs %v %v %v",
				i+1, keys(nodes), scores, background, keys(firstNodes), firstScores, firstBackground)
		}
	}
}

// vectorHybridSearchAtPrecision indexes three facts with orthogonal embeddings,
// then runs a full graph.Search whose query sits closest to one of them and
// asserts that fact ranks first. It exercises the precision-sensitive read path
// (vector distance, the similarity seed mass and result assembly) and is
// generic over P, so the same scenario runs at float32 and float64.
func vectorHybridSearchAtPrecision[P float32 | float64](t *testing.T) {
	t.Helper()
	g := graph.NewGraph[uint64, P](testConfig())
	now := time.Now()

	facts := []struct {
		value string
		vec   []P
	}{
		{"alpha fact about foxes", []P{1, 0, 0}},
		{"beta fact about kites", []P{0, 1, 0}},
		{"gamma fact about reefs", []P{0, 0, 1}},
	}
	for _, f := range facts {
		fact := graph.Fact[uint64]{
			NodeAttributes: graph.NodeAttributes{Value: f.value, Timestamp: now},
			Hasher:         g.GetHasher(),
		}
		if err := g.Set(fact); err != nil {
			t.Fatalf("Set(%q) = %v, want nil", f.value, err)
		}
		if err := g.GetVectorIndex().Insert(fact.Key(), containers.NewVector[uint64](f.vec)); err != nil {
			t.Fatalf("vector Insert(%q) = %v, want nil", f.value, err)
		}
	}

	// The query sits nearest the "beta" embedding, so beta must rank first. No
	// keywords: the vector index is the only seed source.
	query := containers.NewVector[uint64]([]P{0.1, 0.9, 0.1})
	nodes, scores, _, _, _ := g.Search(nil, query, nil, nil, 1, 10, time.Time{}, time.Time{})

	if len(nodes) == 0 {
		t.Fatalf("Search returned no results, want the nearest fact")
	}
	if got := (*nodes[0]).GetValue(); got != "beta fact about kites" {
		t.Errorf("nearest result = %q, want the beta fact", got)
	}
	if len(scores) != len(nodes) {
		t.Errorf("got %d nodes but %d scores; slices must be parallel", len(nodes), len(scores))
	}
}

func TestGraphVectorSearch_float64(t *testing.T) { vectorHybridSearchAtPrecision[float64](t) }
func TestGraphVectorSearch_float32(t *testing.T) { vectorHybridSearchAtPrecision[float32](t) }

// Anchor seeding. A Search carrying no keywords and no vector seeds from the
// named anchors' own members, and the ordinary ranking orders what it found.
// The fixture files three facts under a "harbour" topic an hour apart, has a
// "pilot" entity share the middle one and file one of its own, and keeps a
// bystander under a "ledger" topic no search here names. Timestamps are
// explicit so the recency order is pinned by the fixture, not the clock.
const (
	harbourDawn     = "the harbour master logged the dawn tide"      // harbour, newest
	harbourFerry    = "the pilot boat met the ferry off the harbour" // harbour + pilot, an hour old
	harbourCranes   = "the harbour cranes were serviced"             // harbour, two hours old
	pilotWatch      = "the pilot took the night watch"               // pilot, three hours old
	ledgerBystander = "the ledger closed for the quarter"            // ledger, newest
)

// anchorGraph builds the anchor-seeding fixture and returns it with the two
// anchors a search names, so a test can check a sighting's Via against the
// key the store filed the anchor under. The excess traversal is installed as
// db.Start installs it: searchByAnchors asks for depth 2, and the claim that
// anchor seeding runs no traversal is only pinned if there is one to run.
func anchorGraph(t *testing.T) (g *graph.InMemoryGraph[uint64, float64], harbour *graph.Topic[uint64], pilot *graph.NamedEntity[uint64]) {
	t.Helper()
	g = newGraph()
	g.SetTraversal(graph.NewExcessTraversal[uint64, float64]())
	now := time.Now()

	harbour = mkTopic(g, "harbour", now)
	pilot = mkEntity(g, "pilot", now)
	ledger := mkTopic(g, "ledger", now)
	for _, anchor := range []graph.Node[uint64]{harbour, pilot, ledger} {
		mustSet(t, g, anchor)
	}

	file := func(value string, ts time.Time, topic *graph.Topic[uint64], entity *graph.NamedEntity[uint64]) {
		fact := mkFact(g, value, ts)
		mustSet(t, g, fact)
		if topic != nil {
			mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: ts}, Hasher: g.GetHasher()})
		}
		if entity != nil {
			mustSet(t, g, graph.Mentions[uint64]{Fact: &fact, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: ts}, Hasher: g.GetHasher()})
		}
	}
	file(harbourDawn, now, harbour, nil)
	file(harbourFerry, now.Add(-time.Hour), harbour, pilot)
	file(harbourCranes, now.Add(-2*time.Hour), harbour, nil)
	file(pilotWatch, now.Add(-3*time.Hour), nil, pilot)
	file(ledgerBystander, now, ledger, nil)
	return g, harbour, pilot
}

// searchByAnchors runs the anchor-only Search shape — nil keywords, an empty
// vector — with the given anchors, cap and window. It asks for depth 2 on
// purpose: a test that passes through it also proves the lane is inert when
// the anchors seed.
func searchByAnchors(g *graph.InMemoryGraph[uint64, float64], topics, entities []string, top int, since, until time.Time) ([]*graph.Node[uint64], []float64, [][]scoring.Contribution[uint64, float64], float64, error) {
	return g.Search(nil, containers.Vector[uint64, float64]{}, topics, entities, 2, top, since, until)
}

// TestInMemoryGraphSearchSeedsFromAnchorMembersNewestFirst pins the anchor-only
// shape: a Search with nothing to match and a topic named returns every fact
// filed under that topic — and nothing else — newest first. Each hit is
// scored (the unit anchor mass, decayed by age, which is what orders the
// results), carries exactly one anchor sighting naming the topic it was
// found under, and the background is zero because no anchor was observed.
// depth 2 is passed and changes nothing: the pilot's other fact sits two hops
// from a member and would be a transmission candidate from a text seed, but
// anchor seeding runs no traversal.
func TestInMemoryGraphSearchSeedsFromAnchorMembersNewestFirst(t *testing.T) {
	g, harbour, _ := anchorGraph(t)

	nodes, scores, contributions, background, _ := searchByAnchors(g, []string{"harbour"}, nil, 10, time.Time{}, time.Time{})
	if want := []string{harbourDawn, harbourFerry, harbourCranes}; !reflect.DeepEqual(values(nodes), want) {
		t.Fatalf("Search(topic=harbour) = %v, want the topic's members newest first %v", values(nodes), want)
	}
	for i := range scores {
		if scores[i] <= 0 {
			t.Errorf("scores[%d] = %v, want the decayed unit mass, above zero", i, scores[i])
		}
		if i > 0 && scores[i] >= scores[i-1] {
			t.Errorf("scores[%d] = %v is not below scores[%d] = %v: the ranking must decay with age", i, scores[i], i-1, scores[i-1])
		}
		want := []scoring.Contribution[uint64, float64]{{Src: scoring.SrcAnchor, Score: 1, Via: harbour.Key(), Degree: 3, Count: 1}}
		if !reflect.DeepEqual(contributions[i], want) {
			t.Errorf("contributions[%d] = %+v, want one anchor sighting via harbour %+v", i, contributions[i], want)
		}
	}
	if background != 0 {
		t.Errorf("background = %v, want 0: anchor seeding observes no anchor", background)
	}
}

// TestInMemoryGraphSearchAnchorSeedsUnion pins what naming several anchors
// means when they seed: the pool is their union, each fact once (beside a
// term, the same two clauses narrow to facts filed under both). A fact filed
// under both named anchors carries a sighting from each, twice the seed mass,
// which at an hour's age puts it ahead of the fresher singly-filed fact
// (2 × 0.5^(1h/2160h) against 1); the rest follow newest first. The sightings
// arrive in query order, topics then entities, so the fold is byte-identical
// run to run.
func TestInMemoryGraphSearchAnchorSeedsUnion(t *testing.T) {
	g, harbour, pilot := anchorGraph(t)

	nodes, scores, contributions, _, _ := searchByAnchors(g, []string{"harbour"}, []string{"pilot"}, 10, time.Time{}, time.Time{})
	if want := []string{harbourFerry, harbourDawn, harbourCranes, pilotWatch}; !reflect.DeepEqual(values(nodes), want) {
		t.Fatalf("Search(topic=harbour, entity=pilot) = %v, want the union with the doubly-filed fact first %v", values(nodes), want)
	}
	want := []scoring.Contribution[uint64, float64]{
		{Src: scoring.SrcAnchor, Score: 1, Via: harbour.Key(), Degree: 3, Count: 1},
		{Src: scoring.SrcAnchor, Score: 1, Via: pilot.Key(), Degree: 2, Count: 1},
	}
	if !reflect.DeepEqual(contributions[0], want) {
		t.Errorf("contributions of the doubly-filed fact = %+v, want one sighting per anchor in query order %+v", contributions[0], want)
	}
	if scores[0] <= scores[1] {
		t.Errorf("doubly-filed score %v is not above the singly-filed %v", scores[0], scores[1])
	}

	// The same anchors beside a term filter rather than seed: "dawn" is in a
	// harbour fact that mentions no pilot, so the entity clause empties it.
	nodes, _, _, _, _ = g.Search([]string{"dawn"}, containers.Vector[uint64, float64]{}, []string{"harbour"}, []string{"pilot"}, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 0 {
		t.Errorf("Search(dawn, topic=harbour, entity=pilot) = %v, want nothing: beside a term the anchors narrow", values(nodes))
	}
}

// TestInMemoryGraphSearchAnchorSeedsCapWindowAndUnknownAnchor pins the
// modifiers on an anchor-seeded search and its empty cases: top keeps the head
// of the ranking (a non-positive top keeps all of it), since and until bound
// it as always ([since, until), a zero bound open), an anchor nothing is filed
// under seeds nothing, alone or beside ones that do, and identity is by kind:
// a topic's name asked for as an entity names no anchor at all.
func TestInMemoryGraphSearchAnchorSeedsCapWindowAndUnknownAnchor(t *testing.T) {
	g, _, _ := anchorGraph(t)
	cutoff := time.Now().Add(-90 * time.Minute)
	harbourAll := []string{harbourDawn, harbourFerry, harbourCranes}

	cases := []struct {
		name     string
		topics   []string
		entities []string
		top      int
		since    time.Time
		until    time.Time
		want     []string
	}{
		{"top keeps the newest", []string{"harbour"}, nil, 2, time.Time{}, time.Time{}, []string{harbourDawn, harbourFerry}},
		{"a non-positive top returns everything", []string{"harbour"}, nil, 0, time.Time{}, time.Time{}, harbourAll},
		{"since keeps the recent", []string{"harbour"}, nil, 10, cutoff, time.Time{}, []string{harbourDawn, harbourFerry}},
		{"until keeps the old", []string{"harbour"}, nil, 10, time.Time{}, cutoff, []string{harbourCranes}},
		{"an unknown anchor is empty", []string{"nosuch"}, nil, 10, time.Time{}, time.Time{}, []string{}},
		{"an unknown anchor beside a known one adds nothing", []string{"nosuch", "harbour"}, nil, 10, time.Time{}, time.Time{}, harbourAll},
		{"a topic's name is not an entity", nil, []string{"harbour"}, 10, time.Time{}, time.Time{}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes, _, _, _, _ := searchByAnchors(g, tc.topics, tc.entities, tc.top, tc.since, tc.until)
			if got := values(nodes); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Search(topics=%v entities=%v top=%d) = %v, want %v", tc.topics, tc.entities, tc.top, got, tc.want)
			}
		})
	}
}

// TestInMemoryGraphSearchNamesAnAnchorOnce pins that a repeated anchor is one
// anchor: the grammar reads topic:x topic:x as a list, but its members are
// sighted once, not once per mention. Otherwise repeating a clause would
// double their mass and put them ahead of everything filed under the other
// anchors.
func TestInMemoryGraphSearchNamesAnAnchorOnce(t *testing.T) {
	g, harbour, _ := anchorGraph(t)

	_, _, contributions, _, _ := searchByAnchors(g, []string{"harbour", "harbour"}, nil, 10, time.Time{}, time.Time{})
	if len(contributions) != 3 {
		t.Fatalf("Search(topic=harbour, topic=harbour) returned %d hits, want the topic's 3 members", len(contributions))
	}
	want := []scoring.Contribution[uint64, float64]{{Src: scoring.SrcAnchor, Score: 1, Via: harbour.Key(), Degree: 3, Count: 1}}
	for i, c := range contributions {
		if !reflect.DeepEqual(c, want) {
			t.Errorf("contributions[%d] = %+v, want a single sighting %+v", i, c, want)
		}
	}
}

// TestInMemoryGraphSearchAnchorSeedsOrderTiesByKey pins the key tie-break on
// an anchor-seeded search: three facts filed under one topic at the same
// instant carry equal mass and equal decay, so nothing but the key can order
// them. The search must rank them by key every time, as any search does, or
// top would truncate an arbitrary member of the tie; the query is repeated
// because the candidate map's iteration order changes between calls.
func TestInMemoryGraphSearchAnchorSeedsOrderTiesByKey(t *testing.T) {
	g := newGraph()
	now := time.Now()
	topic := mkTopic(g, "tides", now)
	mustSet(t, g, topic)
	tied := make([]uint64, 0, 3)
	for _, value := range []string{"spring tide", "neap tide", "king tide"} {
		fact := mkFact(g, value, now)
		mustSet(t, g, fact)
		mustSet(t, g, graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
		tied = append(tied, fact.Key())
	}
	sort.Slice(tied, func(i, j int) bool { return tied[i] < tied[j] })

	for i := 0; i < 20; i++ {
		nodes, _, _, _, _ := searchByAnchors(g, []string{"tides"}, nil, 2, time.Time{}, time.Time{})
		if got := keys(nodes); !reflect.DeepEqual(got, tied[:2]) {
			t.Fatalf("Search(topic=tides, top=2) keys = %v on call %d, want the two lowest keys %v every call", got, i+1, tied[:2])
		}
	}
}

// TestInMemoryGraphSearchWithATermSeedsFromText pins that a term keeps the
// seeding in the text index: the same anchor beside a keyword filters the
// text matches and lends no sighting, so every contribution is the text
// index's and the score is BM25 mass rather than the unit anchor mass.
func TestInMemoryGraphSearchWithATermSeedsFromText(t *testing.T) {
	g, _, _ := anchorGraph(t)

	nodes, _, contributions, _, _ := g.Search([]string{"cranes"}, containers.Vector[uint64, float64]{}, []string{"harbour"}, nil, 0, 10, time.Time{}, time.Time{})
	if want := []string{harbourCranes}; !reflect.DeepEqual(values(nodes), want) {
		t.Fatalf("Search(cranes, topic=harbour) = %v, want the one text match %v", values(nodes), want)
	}
	for _, c := range contributions[0] {
		if c.Src == scoring.SrcAnchor {
			t.Errorf("a text-seeded search recorded an anchor sighting %+v; anchors seed only on their own", c)
		}
	}
}

// TestInMemoryGraphSearchCleansQueryStopWords pins that the query is cleaned
// of stop words the way a stored fact is, so the two sides of the text index
// see one vocabulary. A stop word left in the query is not merely a term with
// no postings: the tokenizer stems it, and the stem can be a content word's
// ("own" stems to the same term as "owns"), so without the cleaning the stop
// word would surface facts about owning, as the content word rightly does.
// With it, the stop word seeds nothing, and a query that is nothing but stop
// words matches nothing rather than erroring. Results are compared as sets:
// the cases pin what is found, not how equal scores tie.
func TestInMemoryGraphSearchCleansQueryStopWords(t *testing.T) {
	g := newGraph()
	now := time.Now()
	const owns, joined = "Ana owns the bakery", "Caroline joined the team"
	mustSet(t, g, mkFact(g, owns, now))
	mustSet(t, g, mkFact(g, joined, now))

	cases := []struct {
		name     string
		keywords []string
		want     []string
	}{
		{"a stop word is not a search term, whatever it stems to", []string{"Caroline", "own"}, []string{joined}},
		{"the content word sharing the stem does match", []string{"Caroline", "owns"}, []string{owns, joined}},
		{"only stop words match nothing without erroring", []string{"when", "the", "own"}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes, _, _, _, err := g.Search(tc.keywords, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
			if err != nil {
				t.Fatalf("Search(%v) error = %v, want nil", tc.keywords, err)
			}
			got := values(nodes)
			sort.Strings(got)
			sort.Strings(tc.want)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Search(%v) = %v, want %v", tc.keywords, got, tc.want)
			}
		})
	}
}

// TestSearchWithoutAnchorsOpensFromTheSeeds pins that the round opens from the
// anchors the seeds are filed under, so a recall naming no topic or entity
// still enters the graph. The storm query reaches the weather cluster and the
// archive hub through its matches alone: the background is positive, the
// cluster's silent member arrives on a graph contribution, and the hub's memos
// stay out because admission, not a named filter, keeps a fair-share hub
// silent. The same call at depth 0 stays a text search, so the lane opens the
// graph, not a name.
func TestSearchWithoutAnchorsOpensFromTheSeeds(t *testing.T) {
	g := noDecayGraph()
	calm, memos := stormGraph(t, g)

	nodes, _, contributions, background, _ := g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, nil, nil, 2, 20, time.Time{}, time.Time{})
	if background <= 0 {
		t.Fatalf("background = %v without an anchor named, want positive: the seeds' own anchors open the graph", background)
	}
	got := values(nodes)
	found := -1
	for i, value := range got {
		if value == calm {
			found = i
		}
		for _, memo := range memos {
			if value == memo {
				t.Errorf("hub memo %q surfaced without an anchor named — a fair-share hub transmitted", value)
			}
		}
	}
	if found == -1 {
		t.Fatalf("Search(no anchor) = %v, want the silent member %q funded through the anchor its seeds are filed under", got, calm)
	}
	if list := contributions[found]; len(list) != 1 || list[0].Src != scoring.SrcGraph {
		t.Errorf("silent member's contributions = %+v, want a single graph observation", list)
	}

	nodes, _, _, background, _ = g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 20, time.Time{}, time.Time{})
	if background != 0 {
		t.Errorf("depth 0 background = %v without an anchor named, want 0: the floor runs no traversal", background)
	}
	for _, value := range values(nodes) {
		if value == calm {
			t.Errorf("depth 0 surfaced silent member %q: the floor transmits nothing", calm)
		}
	}
}

// TestSearchNamedAnchorsFilterTheRound pins the other half of the door: a
// named anchor narrows what the round returns, it does not decide whether it
// runs. Naming the archive hub alone keeps the weather cluster's facts out —
// the silent member included, although the cluster still transmitted to it —
// and leaves the hub's one matching fact, with its memos still silent: the
// filter removed the cluster from the answer, not the hub's fair share from
// the null.
func TestSearchNamedAnchorsFilterTheRound(t *testing.T) {
	g := noDecayGraph()
	calm, memos := stormGraph(t, g)

	nodes, _, _, background, _ := g.Search([]string{"barometer", "storm"}, containers.Vector[uint64, float64]{}, []string{"archive"}, nil, 2, 20, time.Time{}, time.Time{})
	if background <= 0 {
		t.Fatalf("background = %v with the hub named, want positive: the round ran through the seeds' anchors", background)
	}
	if got, want := values(nodes), []string{"a storm of paperwork"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Search(topic=archive) = %v, want %v: the filter keeps the hub's match and drops %q, and the memos %v stay silent", got, want, calm, memos)
	}
}

// TestInMemoryGraphSearchFilterIsTheAnchorTheDoorOpens pins that a filter
// names the same node the door does (the anchor node whose members seed an
// anchor-only search): topic:work admits facts filed under the Topic "work",
// not facts mentioning a NamedEntity spelled "work". A topic and an entity
// with the same name are two anchors with two keys and two rows, and a filter
// matching on the value alone would confuse them, admitting through a topic
// filter a fact the topic's own row does not hold.
func TestInMemoryGraphSearchFilterIsTheAnchorTheDoorOpens(t *testing.T) {
	g := newGraph()
	now := time.Now()

	fact := mkFact(g, "alice works at acme", now)
	entity := mkEntity(g, "work", now)
	mustSet(t, g, fact)
	mustSet(t, g, entity)
	mustSet(t, g, graph.Mentions[uint64]{Fact: &fact, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})

	nodes, _, _, _, _ := g.Search([]string{"alice"}, containers.Vector[uint64, float64]{}, []string{"work"}, nil, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 0 {
		t.Errorf("Search(alice, topic=work) = %v, want nothing: the fact mentions an entity named work, it is not filed under a topic", values(nodes))
	}
	nodes, _, _, _, _ = g.Search([]string{"alice"}, containers.Vector[uint64, float64]{}, nil, []string{"work"}, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 {
		t.Errorf("Search(alice, entity=work) = %v, want [alice works at acme]", values(nodes))
	}
}

// benchmarkGraph builds a graph of n facts for the Search benchmarks: each
// fact is eight to twelve words from a Zipf-distributed vocabulary, filed
// under one of fifty topics and mentioning one of five hundred entities, with
// a 32-dimensional embedding in the forest. The generator is seeded, so every
// run searches the same graph. It returns the graph and the vocabulary's most
// frequent words, the terms with the longest posting lists.
func benchmarkGraph(b *testing.B, n int) (g *graph.InMemoryGraph[uint64, float64], query []string) {
	b.Helper()
	g = newGraph()
	g.SetTraversal(graph.NewExcessTraversal[uint64, float64]())
	rng := rand.New(rand.NewSource(1))
	zipf := rand.NewZipf(rng, 1.1, 1, 4999)
	now := time.Now()

	topics := make([]*graph.Topic[uint64], 50)
	for i := range topics {
		topics[i] = mkTopic(g, "topic"+strconv.Itoa(i), now)
		if err := g.Set(topics[i]); err != nil {
			b.Fatalf("Set(topic %d) = %v, want nil", i, err)
		}
	}
	entities := make([]*graph.NamedEntity[uint64], 500)
	for i := range entities {
		entities[i] = mkEntity(g, "entity"+strconv.Itoa(i), now)
		if err := g.Set(entities[i]); err != nil {
			b.Fatalf("Set(entity %d) = %v, want nil", i, err)
		}
	}
	for i := range n {
		words := make([]string, 8+rng.Intn(5))
		for w := range words {
			words[w] = "w" + strconv.FormatUint(zipf.Uint64(), 10)
		}
		ts := now.Add(-time.Duration(i) * time.Minute)
		fact := mkFact(g, strings.Join(words, " "), ts)
		if err := g.Set(fact); err != nil {
			b.Fatalf("Set(fact %d) = %v, want nil", i, err)
		}
		topic := topics[rng.Intn(len(topics))]
		if err := g.Set(graph.IsAbout[uint64]{Fact: &fact, Topic: topic, NodeAttributes: graph.NodeAttributes{Timestamp: ts}, Hasher: g.GetHasher()}); err != nil {
			b.Fatalf("Set(isabout %d) = %v, want nil", i, err)
		}
		entity := entities[rng.Intn(len(entities))]
		if err := g.Set(graph.Mentions[uint64]{Fact: &fact, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: ts}, Hasher: g.GetHasher()}); err != nil {
			b.Fatalf("Set(mentions %d) = %v, want nil", i, err)
		}
		vec := make([]float64, 32)
		for d := range vec {
			vec[d] = rng.NormFloat64()
		}
		if err := g.GetVectorIndex().Insert(fact.Key(), containers.NewVector[uint64](vec)); err != nil {
			b.Fatalf("vector Insert(fact %d) = %v, want nil", i, err)
		}
	}
	return g, []string{"w0", "w1", "w2"}
}

// BenchmarkSearch measures the graph's Search over ten thousand facts in each
// lane (depth 0 seeds only; depths 1 and 2 run the anchor-mediated round at
// the precision and recall bars) for a three-term query filtered by one topic,
// and a vector variant seeding from the forest instead of the text index.
func BenchmarkSearch(b *testing.B) {
	g, query := benchmarkGraph(b, 10000)
	rng := rand.New(rand.NewSource(2))
	vec := make([]float64, 32)
	for d := range vec {
		vec[d] = rng.NormFloat64()
	}
	vector := containers.NewVector[uint64](vec)
	none := containers.Vector[uint64, float64]{}

	for depth := range 3 {
		b.Run("depth"+strconv.Itoa(depth), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, _, _, err := g.Search(query, none, []string{"topic7"}, nil, depth, 10, time.Time{}, time.Time{}); err != nil {
					b.Fatalf("Search = %v, want nil", err)
				}
			}
		})
	}
	b.Run("vector", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, _, _, err := g.Search(nil, vector, []string{"topic7"}, nil, 1, 10, time.Time{}, time.Time{}); err != nil {
				b.Fatalf("Search = %v, want nil", err)
			}
		}
	})
}
