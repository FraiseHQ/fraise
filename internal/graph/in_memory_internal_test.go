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

// Same-package tests for the collection phase: the candidate map is internal
// to Search on purpose — callers see ranked hits, not contribution records —
// so what each stage appended can only be asserted from inside the package.

package graph

import (
	"reflect"
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
)

// collectGraph builds an empty graph with the same vector-search shape the
// exported suite's testConfig uses.
func collectGraph() *InMemoryGraph[uint64, float64] {
	cfg := config.New()
	cfg.DB.VectorSearch.ProjectionDimension = 8
	cfg.DB.VectorSearch.NumberTrees = 4
	cfg.DB.VectorSearch.Seed = 4
	return NewGraph[uint64, float64](cfg)
}

func storeAll(t *testing.T, g *InMemoryGraph[uint64, float64], nodes ...Node[uint64]) {
	t.Helper()
	for _, n := range nodes {
		if err := g.Set(n); err != nil {
			t.Fatalf("Set(%q) = %v, want nil", n.GetValue(), err)
		}
	}
}

// TestCollectPoolsSourcesPerCandidate pins the whole candidate map of a hybrid
// query — the sources each node was sighted by, and every field the sighting
// records. One fact is found by text and by vector (two seed contributions, no
// graph one: nothing walks to it); the entity and the fact behind it are
// reached only by the walk, so each holds a single SrcGraph contribution
// stamped with the seed's fused score (1 text + 1 vector = 2), its rank in
// the walk's nearest-first order, and its raw hop count. Hop 2 arriving with
// Score 2 unreduced is the point of the layer: attenuation is no longer
// applied at collection.
func TestCollectPoolsSourcesPerCandidate(t *testing.T) {
	g := collectGraph()
	now := time.Now()

	seed := Fact[uint64]{NodeAttributes: NodeAttributes{Value: "alpha fact about acme", Timestamp: now}, Hasher: g.hasher}
	linked := Fact[uint64]{NodeAttributes: NodeAttributes{Value: "beta fact about paris", Timestamp: now}, Hasher: g.hasher}
	entity := &NamedEntity[uint64]{NodeAttributes: NodeAttributes{Value: "alice", Timestamp: now}, Hasher: g.hasher}
	storeAll(t, g, seed, linked, entity,
		Mentions[uint64]{Fact: &seed, NamedEntity: entity, NodeAttributes: NodeAttributes{Timestamp: now}, Hasher: g.hasher},
		Mentions[uint64]{Fact: &linked, NamedEntity: entity, NodeAttributes: NodeAttributes{Timestamp: now}, Hasher: g.hasher},
	)
	if err := g.vectorIndex.Insert(seed.Key(), containers.NewVector[uint64]([]float64{1, 0})); err != nil {
		t.Fatalf("vector Insert = %v, want nil", err)
	}

	// The query embedding equals the stored one, so the similarity is exactly
	// 1/(1+0): the seed's fused score is 1 (text, rank 0) + 1 (vector) = 2.
	got := g.collect([]string{"acme"}, containers.NewVector[uint64]([]float64{1, 0}), nil, nil, 2)

	want := Candidates[uint64, float64]{
		seed.Key(): {
			{Src: SrcText, Score: 1, Rank: 0, Hop: 0},
			{Src: SrcVector, Score: 1, Rank: 0, Hop: 0},
		},
		entity.Key(): {
			{Src: SrcGraph, Score: 2, Rank: 0, Hop: 1},
		},
		linked.Key(): {
			{Src: SrcGraph, Score: 2, Rank: 1, Hop: 2},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collect() = %+v, want %+v", got, want)
	}
}

// TestCollectRecordsVectorSimilarityNotDistance pins the conversion at the
// vector collection site — the bug the layer fixes. The index reports the
// Euclidean distance; the site records 1/(1+distance), so the far vector
// arrives as similarity 1/(1+3) = 0.25, not distance 3 and not the constant 1
// a zero-distance fixture cannot tell apart. Before the layer, this quantity
// was discarded and a far nearest-neighbour seeded as strongly as an exact
// match. depth is 0, so no walk runs and the map holds seeds alone.
func TestCollectRecordsVectorSimilarityNotDistance(t *testing.T) {
	g := collectGraph()
	now := time.Now()

	near := Fact[uint64]{NodeAttributes: NodeAttributes{Value: "the near fact", Timestamp: now}, Hasher: g.hasher}
	far := Fact[uint64]{NodeAttributes: NodeAttributes{Value: "the far fact", Timestamp: now}, Hasher: g.hasher}
	storeAll(t, g, near, far)
	if err := g.vectorIndex.Insert(near.Key(), containers.NewVector[uint64]([]float64{1, 0})); err != nil {
		t.Fatalf("vector Insert(near) = %v, want nil", err)
	}
	if err := g.vectorIndex.Insert(far.Key(), containers.NewVector[uint64]([]float64{1, 3})); err != nil {
		t.Fatalf("vector Insert(far) = %v, want nil", err)
	}

	got := g.collect(nil, containers.NewVector[uint64]([]float64{1, 0}), nil, nil, 0)

	want := Candidates[uint64, float64]{
		near.Key(): {{Src: SrcVector, Score: 1, Rank: 0, Hop: 0}},
		far.Key():  {{Src: SrcVector, Score: 0.25, Rank: 1, Hop: 0}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collect() = %+v, want %+v", got, want)
	}
}

// TestCollectAccumulatesASightingPerSource checks accumulation across walks:
// two facts share a keyword and an entity, so each is a text seed and each is
// also reached by the other's walk — one SrcText and one SrcGraph contribution
// apiece — while the shared entity collects one SrcGraph contribution per
// walk. Seed ranks follow the text index's key tiebreak and walks run in
// ascending key order, so the map, including the order within each list, is
// exactly reproducible.
func TestCollectAccumulatesASightingPerSource(t *testing.T) {
	g := collectGraph()
	now := time.Now()

	factA := Fact[uint64]{NodeAttributes: NodeAttributes{Value: "comet over the alps", Timestamp: now}, Hasher: g.hasher}
	factB := Fact[uint64]{NodeAttributes: NodeAttributes{Value: "comet in the archive", Timestamp: now}, Hasher: g.hasher}
	entity := &NamedEntity[uint64]{NodeAttributes: NodeAttributes{Value: "halley", Timestamp: now}, Hasher: g.hasher}
	storeAll(t, g, factA, factB, entity,
		Mentions[uint64]{Fact: &factA, NamedEntity: entity, NodeAttributes: NodeAttributes{Timestamp: now}, Hasher: g.hasher},
		Mentions[uint64]{Fact: &factB, NamedEntity: entity, NodeAttributes: NodeAttributes{Timestamp: now}, Hasher: g.hasher},
	)

	// Both facts match "comet" once, so text rank falls back to key order:
	// the lower key seeds at rank 0 with fused score 1, the higher at rank 1
	// with 1/2.
	first, second := factA, factB
	if second.Key() < first.Key() {
		first, second = second, first
	}

	got := g.collect([]string{"comet"}, containers.Vector[uint64, float64]{}, nil, nil, 2)

	want := Candidates[uint64, float64]{
		first.Key(): {
			{Src: SrcText, Score: 1, Rank: 0, Hop: 0},
			{Src: SrcGraph, Score: 0.5, Rank: 1, Hop: 2}, // second's walk
		},
		second.Key(): {
			{Src: SrcText, Score: 1, Rank: 1, Hop: 0},
			{Src: SrcGraph, Score: 1, Rank: 1, Hop: 2}, // first's walk
		},
		entity.Key(): {
			{Src: SrcGraph, Score: 1, Rank: 0, Hop: 1},   // first's walk
			{Src: SrcGraph, Score: 0.5, Rank: 0, Hop: 1}, // second's walk
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collect() = %+v, want %+v", got, want)
	}
}
