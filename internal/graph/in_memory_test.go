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
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
	"github.com/RonsenbergVI/fraise/internal/index"
)

// newGraph builds an empty graph carrying the test config.
func newGraph() *graph.InMemoryGraph[uint64, float64] {
	return graph.NewGraph[uint64, float64](testConfig())
}

// mkFact returns a Fact wired to g's hasher; its key is derived from value.
func mkFact(g *graph.InMemoryGraph[uint64, float64], value string, ts time.Time) graph.Fact[uint64] {
	return graph.Fact[uint64]{NodeAttributes: graph.NodeAttributes{Value: value, Timestamp: ts}, Hasher: g.GetHasher()}
}

func mkEntity(g *graph.InMemoryGraph[uint64, float64], value string, ts time.Time) *graph.NamedEntity[uint64] {
	return &graph.NamedEntity[uint64]{NodeAttributes: graph.NodeAttributes{Value: value, Timestamp: ts}, Hasher: g.GetHasher()}
}

func mkTopic(g *graph.InMemoryGraph[uint64, float64], value string, ts time.Time) *graph.Topic[uint64] {
	return &graph.Topic[uint64]{NodeAttributes: graph.NodeAttributes{Value: value, Timestamp: ts}, Hasher: g.GetHasher()}
}

func mustSet(t *testing.T, g *graph.InMemoryGraph[uint64, float64], n graph.Node[uint64]) {
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

// TestInMemoryGraphStoresAFactAndATopicOfTheSameText is the ticket repro:
// `remember 'billing' topic:billing`. With one content-hash namespace the fact
// and the topic keyed alike, so Set refused the topic as an existing node and
// the IsAbout edge ran from the fact to itself — one node where three belong.
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
	// The topic node has to be reachable as a topic for anchored recalls to
	// resolve it, which is what the collision took away.
	if got := g.Get(topicKey); got == nil {
		t.Errorf("Get(topic) = nil, want the stored topic node")
	} else if _, isTopic := got.(*graph.Topic[uint64]); !isTopic {
		t.Errorf("Get(topic) = %T, want *graph.Topic", got)
	}
}

// TestInMemoryGraphDeletePrunesIncidentRelationshipNodes checks the whole of
// Delete's contract — the node "and, by extension, its index entries and
// incident relationships". Unlinking an edge from the adjacency maps is not
// enough: a Mentions node left in idToNodes describes an edge that no longer
// exists, and Nodes, Stats and the text index all keep reporting it.
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

	// Deleting the fact takes its remaining edge with it, index entry included.
	if err := g.Delete(fact); err != nil {
		t.Fatalf("Delete(fact) = %v, want nil", err)
	}
	if g.Get(about.Key()) != nil {
		t.Errorf("the IsAbout node outlived its fact, want it pruned")
	}
	if _, err := g.GetTextIndex().Retrieve(about.Key()); !errors.Is(err, index.ErrIndexNotFound) {
		t.Errorf("text index Retrieve(IsAbout) after pruning = %v, want ErrIndexNotFound", err)
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
	if _, err := g.GetVectorIndex().Retrieve(key); err == nil {
		t.Errorf("vector Retrieve(key) after delete succeeded, want error")
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

	nodes, scores, _ := g.Search([]string{"acme"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alice works at acme" {
		t.Fatalf("Search(acme) = %v, want [alice works at acme]", values(nodes))
	}
	if len(scores) != 1 || scores[0] <= 0 {
		t.Errorf("Search(acme) scores = %v, want one positive score", scores)
	}

	// With depth 2 the walk crosses the shared entity into the related fact,
	// which joins at an attenuated score. Only facts are hits, so the entity
	// node itself never appears in the results.
	nodes, scores, _ = g.Search([]string{"acme"}, containers.Vector[uint64, float64]{}, nil, nil, 2, 10, time.Time{}, time.Time{})
	if len(nodes) != 2 {
		t.Fatalf("Search(acme, depth=2) = %v, want 2 nodes", values(nodes))
	}
	if (*nodes[0]).GetValue() != "alice works at acme" {
		t.Errorf("Search(acme, depth=2) best hit = %q, want the direct hit", (*nodes[0]).GetValue())
	}
	if (*nodes[1]).GetValue() != "alice lives in paris" {
		t.Errorf("Search(acme, depth=2) second hit = %q, want the entity-linked fact", (*nodes[1]).GetValue())
	}
	if scores[1] >= scores[0] {
		t.Errorf("neighbour score %v not attenuated below seed score %v", scores[1], scores[0])
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

	nodes, _, _ := g.Search(nil, containers.NewVector[uint64]([]float64{0.9, 0.1}), nil, nil, 0, 1, time.Time{}, time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alpha" {
		t.Errorf("Search(vector near alpha) = %v, want [alpha]", values(nodes))
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
	nodes, _, _ := g.Search([]string{"alice"}, containers.Vector[uint64, float64]{}, []string{"work"}, nil, 0, 10, time.Time{}, time.Time{})
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

	nodes, _, _ := g.Search([]string{"alice"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alice recent fact" {
		t.Errorf("Search(alice, since=2025) = %v, want [alice recent fact]", values(nodes))
	}

	nodes, _, _ = g.Search([]string{"alice"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(nodes) != 1 || (*nodes[0]).GetValue() != "alice ancient fact" {
		t.Errorf("Search(alice, until=2025) = %v, want [alice ancient fact]", values(nodes))
	}
}

// TestInMemoryGraphSearchRecencyDecayFactor pins the decay formula the README
// promises ("recent memories outrank older ones"): a fact's score is
// multiplied by 0.5^(age/half-life). A single fact is a rank-0 text seed, so
// its pre-decay score is exactly 1/60 — one sighting under the RRF fold
// Σ 1/(60+rank) — and the decay factor is observable as the ratio to it.
func TestInMemoryGraphSearchRecencyDecayFactor(t *testing.T) {
	halflife := testConfig().Engine.Halflife // default 7d

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

			_, scores, _ := g.Search([]string{"aurora"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
			if len(scores) != 1 {
				t.Fatalf("Search returned %d scores, want 1", len(scores))
			}
			// The fact ages a hair between Set and Search, so allow a small
			// tolerance around the exact factor.
			if diff := scores[0]*60 - tc.want; diff > 1e-3 || diff < -1e-3 {
				t.Errorf("score = %v, want ~%v/60 (0.5^(age/half-life) of the lone rank-0 sighting)", scores[0], tc.want)
			}
		})
	}
}

// TestInMemoryGraphSearchRecencyOrdersTies checks the user-visible promise:
// of two facts matching the same term, the recent one outranks the much older
// one. The old fact is aged ten half-lives so decay dominates whatever seed
// ranks the text index assigns — the ordering cannot depend on index
// tie-breaking.
func TestInMemoryGraphSearchRecencyOrdersTies(t *testing.T) {
	g := newGraph()
	halflife := testConfig().Engine.Halflife
	mustSet(t, g, mkFact(g, "comet sighted in march", time.Now().Add(-10*halflife)))
	mustSet(t, g, mkFact(g, "comet sighted today", time.Now()))

	nodes, scores, _ := g.Search([]string{"comet"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
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

// TestInMemoryGraphSearchDecayDisabled checks that a non-positive half-life
// switches decay off: an old fact keeps its full relevance score — exactly
// the 1/60 its lone rank-0 sighting fuses to, untouched by age.
func TestInMemoryGraphSearchDecayDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.Engine.Halflife = 0
	g := graph.NewGraph[uint64, float64](cfg)
	mustSet(t, g, mkFact(g, "glacier survey notes", time.Now().Add(-365*24*time.Hour)))

	_, scores, _ := g.Search([]string{"glacier"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 10, time.Time{}, time.Time{})
	if len(scores) != 1 {
		t.Fatalf("Search returned %d scores, want 1", len(scores))
	}
	if scores[0] != 1.0/60 {
		t.Errorf("score with decay disabled = %v, want exactly 1/60", scores[0])
	}
}

// TestInMemoryGraphSearchOrdersTiesByKey pins the total order Search ranks by:
// score descending, then fact key. Under rank fusion equal ranks fuse to equal
// scores wherever they occur: the two components here are disjoint, so each
// seed's walk reaches its own leaf at walk rank 1, and the rank-1 text seed
// lands on the same 1/(60+1) — a three-way tie to the bit that relevance and
// decay cannot separate. Without the key tiebreak the trio comes back in
// whatever order the score map was iterated in, and top truncates that
// arbitrary order. Each case repeats the query because that map order changes
// between calls: a single pass can agree by luck.
func TestInMemoryGraphSearchOrdersTiesByKey(t *testing.T) {
	g := newGraph()
	now := time.Now()

	seedA := mkFact(g, "acme builds rockets", now)
	seedB := mkFact(g, "acme ships engines", now)
	leafA := mkFact(g, "the assembly line hums", now)
	leafB := mkFact(g, "the test stand fires", now)
	entityA := mkEntity(g, "orion", now)
	entityB := mkEntity(g, "vulcan", now)
	for _, node := range []graph.Node[uint64]{seedA, seedB, leafA, leafB, entityA, entityB} {
		mustSet(t, g, node)
	}
	// Two disjoint components: each seed reaches its own leaf two hops away
	// through its own entity, and nothing links the components.
	mustSet(t, g, graph.Mentions[uint64]{Fact: &seedA, NamedEntity: entityA, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	mustSet(t, g, graph.Mentions[uint64]{Fact: &leafA, NamedEntity: entityA, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	mustSet(t, g, graph.Mentions[uint64]{Fact: &seedB, NamedEntity: entityB, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	mustSet(t, g, graph.Mentions[uint64]{Fact: &leafB, NamedEntity: entityB, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})

	// Both seeds match "acme" once, so the text index ranks them by key: the
	// lower-key seed is rank 0 and scores 1/60 alone at the top; the other
	// seed and both leaves tie at 1/61.
	first, second := seedA, seedB
	if second.Key() < first.Key() {
		first, second = second, first
	}

	// The premise of the test: the rank-1 seed and the two leaves score
	// equally to the bit, so nothing but the key can order them.
	if _, scores, _ := g.Search([]string{"acme"}, containers.Vector[uint64, float64]{}, nil, nil, 2, 10, time.Time{}, time.Time{}); len(scores) != 4 || scores[1] != scores[2] || scores[2] != scores[3] {
		t.Fatalf("Search(acme, depth=2) scores = %v, want four hits whose last three tie", scores)
	}

	tied := []uint64{second.Key(), leafA.Key(), leafB.Key()}
	sort.Slice(tied, func(i, j int) bool { return tied[i] < tied[j] })

	cases := []struct {
		name string
		top  int
		want []uint64
	}{
		{"tied facts follow the top seed in key order", 10, []uint64{first.Key(), tied[0], tied[1], tied[2]}},
		{"truncation keeps the lowest tied key", 2, []uint64{first.Key(), tied[0]}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 20; i++ {
				nodes, _, _ := g.Search([]string{"acme"}, containers.Vector[uint64, float64]{}, nil, nil, 2, tc.top, time.Time{}, time.Time{})
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

	nodes, scores, _ := g.Search([]string{"shared"}, containers.Vector[uint64, float64]{}, nil, nil, 0, 2, time.Time{}, time.Time{})
	if len(nodes) != 2 || len(scores) != 2 {
		t.Errorf("Search(top=2) returned %d nodes and %d scores, want 2 and 2", len(nodes), len(scores))
	}
	if scores[0] < scores[1] {
		t.Errorf("scores not descending: %v", scores)
	}
}

func TestInMemoryGraphCopyIsIndependent(t *testing.T) {
	g := newGraph()
	now := time.Now()
	fact := mkFact(g, "alice works at acme", now)
	entity := mkEntity(g, "alice", now)
	mustSet(t, g, fact)
	mustSet(t, g, entity)
	mustSet(t, g, graph.Mentions[uint64]{Fact: &fact, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: g.GetHasher()})
	if err := g.GetVectorIndex().Insert(fact.Key(), containers.NewVector[uint64]([]float64{1, 2})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}

	clone := g.Copy()
	if clone.Stats() != g.Stats() {
		t.Fatalf("Copy() stats = %+v, want %+v", clone.Stats(), g.Stats())
	}

	// Mutating the copy must not touch the original.
	if err := clone.Delete(fact); err != nil {
		t.Fatalf("Delete on copy = %v, want nil", err)
	}
	if g.Get(fact.Key()) == nil {
		t.Errorf("deleting from the copy removed the fact from the original")
	}
	if got := g.Size(); got != 1 {
		t.Errorf("original Size() = %d after mutating copy, want 1", got)
	}
	if _, err := g.GetVectorIndex().Retrieve(fact.Key()); err != nil {
		t.Errorf("original vector entry lost after mutating copy: %v", err)
	}
}

func TestInMemoryGraphMergeFrom(t *testing.T) {
	now := time.Now()

	a := newGraph()
	mustSet(t, a, mkFact(a, "alice works at acme", now))

	b := newGraph()
	fact2 := mkFact(b, "bob plays tennis", now)
	entity := mkEntity(b, "bob", now)
	mustSet(t, b, fact2)
	mustSet(t, b, entity)
	mustSet(t, b, graph.Mentions[uint64]{Fact: &fact2, NamedEntity: entity, NodeAttributes: graph.NodeAttributes{Timestamp: now}, Hasher: b.GetHasher()})
	if err := b.GetVectorIndex().Insert(fact2.Key(), containers.NewVector[uint64]([]float64{3, 4})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}

	a.MergeFrom(b)

	// a's own fact plus b's fact, entity and relationship node.
	if got := a.Stats(); got.Nodes != 4 || got.Size != 1 {
		t.Errorf("Stats() after merge = %+v, want {Nodes:4 Size:1}", got)
	}
	if a.Get(fact2.Key()) == nil || a.Get(entity.Key()) == nil {
		t.Errorf("merged nodes missing: Get(fact2)=%v Get(entity)=%v", a.Get(fact2.Key()), a.Get(entity.Key()))
	}
	if keys, _, err := a.GetTextIndex().Search("tennis", 0); err != nil || len(keys) != 1 || keys[0] != fact2.Key() {
		t.Errorf("text Search(tennis) after merge = (%v, %v), want ([fact2], nil)", keys, err)
	}
	if _, err := a.GetVectorIndex().Retrieve(fact2.Key()); err != nil {
		t.Errorf("vector Retrieve(fact2) after merge = %v, want nil", err)
	}
}

// vectorHybridSearchAtPrecision indexes three facts with orthogonal embeddings,
// then runs a full graph.Search whose query sits closest to one of them and
// asserts that fact ranks first. It exercises the whole precision-sensitive read
// path — vector distance, rank-based seed scoring, hop attenuation and result
// assembly — and is generic over P so the identical scenario runs for both
// float32 and float64, proving the results match at either precision.
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
	nodes, scores, _ := g.Search(nil, query, nil, nil, 1, 10, time.Time{}, time.Time{})

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

// TestMergeFromForestStaysBounded replays the production write cycle — Stage
// copies the graph, the write commits one vector to the copy, MergeFrom folds
// the copy back — and checks the live vector forest stays O(live vectors).
// Regression test for the quadratic-bloat bug where MergeFrom re-inserted every
// staged vector on each write, growing the forest to ~W^2/2 entries after W
// writes (~900x bloat at 300 writes).
func TestMergeFromForestStaysBounded(t *testing.T) {
	g := newGraph()

	const writes = 300
	const dim = 8
	for w := 0; w < writes; w++ {
		stg := g.Copy() // what Stream.Stage does for a write

		vec := make([]float64, dim)
		vec[w%dim] = float64(w + 1)
		if err := stg.GetVectorIndex().Insert(uint64(w+1), containers.NewVector[uint64](vec)); err != nil {
			t.Fatalf("staging insert (write %d) = %v, want nil", w, err)
		}

		g.MergeFrom(stg) // what scheduler.execute does on success
	}

	idx, ok := g.GetVectorIndex().(*index.RPTreeIndex[uint64, float64])
	if !ok {
		t.Fatalf("vector index is %T, want *index.RPTreeIndex", g.GetVectorIndex())
	}
	if got := idx.Count(); got != writes {
		t.Fatalf("Count() = %d, want %d", got, writes)
	}
	// Bound: idempotent inserts + auto-flush keep the forest within 2x live.
	if got, bound := idx.Entries(), 2*writes; got > bound {
		t.Errorf("Entries() after %d write cycles = %d, want <= %d (was ~%d before the fix)",
			writes, got, bound, writes*writes/2)
	}
}
