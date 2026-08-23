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

package trees_test

import (
	"errors"
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/containers/trees"
	"github.com/FraiseHQ/fraise/internal/hash"
)

// point is a test double implementing both trees.TreeNode[int, string, float64]
// and trees.Point[int, float64] (Point() returns the receiver itself), which
// is all an RPTree needs to index it.
type point struct {
	key   int
	value string
	coord []float64
}

func (p *point) Key() int                          { return p.key }
func (p *point) Value() string                     { return p.value }
func (p *point) Hash(hash.Hasher[int, string]) int { return p.key }

func (p *point) Point() trees.Point[int, float64] {
	if p.coord == nil {
		return nil
	}
	return p
}

func (p *point) Dim() int                 { return len(p.coord) }
func (p *point) GetValue(dim int) float64 { return p.coord[dim] }
func (p *point) PlaneDistance(val float64, dim int) float64 {
	return math.Abs(p.coord[dim] - val)
}
func (p *point) Distance(o trees.Point[int, float64]) float64 {
	var sum float64
	for d := 0; d < p.Dim(); d++ {
		diff := p.GetValue(d) - o.GetValue(d)
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

func randPoint(rng *rand.Rand, key, dim int) *point {
	coord := make([]float64, dim)
	for i := range coord {
		coord[i] = rng.Float64() * 100
	}
	return &point{key: key, value: "v", coord: coord}
}

func bruteForceNearest(points []*point, q *point, k int) []int {
	type scored struct {
		key int
		d   float64
	}
	scores := make([]scored, len(points))
	for i, p := range points {
		scores[i] = scored{key: p.key, d: q.Distance(p)}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].d < scores[j].d })
	if len(scores) > k {
		scores = scores[:k]
	}
	out := make([]int, len(scores))
	for i, s := range scores {
		out[i] = s.key
	}
	return out
}

func TestRPTreeEmpty(t *testing.T) {
	rt := trees.NewRPTree[int, string, float64](3, 4, 1)
	if got := rt.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if got := rt.Nearest(&point{coord: []float64{0, 0, 0}}, 3); got != nil {
		t.Errorf("Nearest on empty tree = %v, want nil", got)
	}
	if got := rt.Nodes(); len(got) != 0 {
		t.Errorf("Nodes() = %v, want empty", got)
	}
}

func TestRPTreeInsertRejectsBadPoints(t *testing.T) {
	rt := trees.NewRPTree[int, string, float64](3, 4, 1)

	if err := rt.Insert(&point{key: 1, coord: []float64{1, 2}}); !errors.Is(err, trees.ErrDimensionMismatch) {
		t.Errorf("Insert with wrong dimension = %v, want ErrDimensionMismatch", err)
	}
	if err := rt.Insert(&point{key: 2}); !errors.Is(err, trees.ErrMissingPoint) {
		t.Errorf("Insert with nil Point = %v, want ErrMissingPoint", err)
	}
	if got := rt.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0 after rejected inserts", got)
	}
}

// TestRPTreeNearestExactWhenSingleLeaf keeps the dataset under the leaf
// capacity so the tree never splits; in that regime Nearest is a plain
// distance scan and must match brute force exactly.
func TestRPTreeNearestExactWhenSingleLeaf(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const dim = 5
	const n = 20 // well under the default leaf size of 32

	rt := trees.NewRPTree[int, string, float64](dim, 4, 7)
	points := make([]*point, n)
	for i := range points {
		points[i] = randPoint(rng, i, dim)
		if err := rt.Insert(points[i]); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}

	query := randPoint(rng, -1, dim)
	want := bruteForceNearest(points, query, 5)

	got := rt.Nearest(query, 5)
	if len(got) != len(want) {
		t.Fatalf("Nearest returned %d nodes, want %d", len(got), len(want))
	}
	for i, node := range got {
		if node.Key() != want[i] {
			t.Errorf("Nearest()[%d].Key() = %d, want %d", i, node.Key(), want[i])
		}
	}
}

// TestRPTreeNodesCoverEveryInsert forces splits (n well past the leaf
// capacity) and checks every inserted node still shows up exactly once
// somewhere in the tree.
func TestRPTreeNodesCoverEveryInsert(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	const dim = 4
	const n = 500

	rt := trees.NewRPTree[int, string, float64](dim, 6, 3)
	points := make([]*point, n)
	for i := range points {
		points[i] = randPoint(rng, i, dim)
		if err := rt.Insert(points[i]); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}
	if got := rt.Len(); got != n {
		t.Fatalf("Len() = %d, want %d", got, n)
	}

	seen := make(map[int]bool, n)
	for _, node := range rt.Nodes() {
		if seen[node.Key()] {
			t.Fatalf("key %d appeared more than once in Nodes()", node.Key())
		}
		seen[node.Key()] = true
	}
	if len(seen) != n {
		t.Fatalf("Nodes() covered %d distinct keys, want %d", len(seen), n)
	}
}

// TestRPTreeNearestReturnsSubsetOfStoredNodes checks the structural contract
// of the (approximate) Nearest search on a tree that does split: it must
// return no more than k nodes, all of them genuinely inserted, with no
// duplicates, ordered by non-decreasing true distance to the query.
func TestRPTreeNearestReturnsSubsetOfStoredNodes(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	const dim = 4
	const n = 500
	const k = 10

	rt := trees.NewRPTree[int, string, float64](dim, 6, 5)
	all := make(map[int]*point, n)
	for i := 0; i < n; i++ {
		p := randPoint(rng, i, dim)
		all[i] = p
		if err := rt.Insert(p); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}

	query := randPoint(rng, -1, dim)
	got := rt.Nearest(query, k)
	if len(got) > k {
		t.Fatalf("Nearest returned %d nodes, want at most %d", len(got), k)
	}

	seen := make(map[int]bool, len(got))
	prevDist := -1.0
	for _, node := range got {
		p, ok := all[node.Key()]
		if !ok {
			t.Fatalf("Nearest returned key %d that was never inserted", node.Key())
		}
		if seen[node.Key()] {
			t.Fatalf("Nearest returned key %d more than once", node.Key())
		}
		seen[node.Key()] = true

		d := query.Distance(p)
		if d < prevDist {
			t.Fatalf("Nearest results not sorted by distance: %v got distance %v after %v", node.Key(), d, prevDist)
		}
		prevDist = d
	}
}

func TestRPTreeRangeIsExact(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	const dim = 3
	const n = 400

	rt := trees.NewRPTree[int, string, float64](dim, 5, 2)
	points := make([]*point, n)
	for i := range points {
		points[i] = randPoint(rng, i, dim)
		if err := rt.Insert(points[i]); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}

	min := &point{coord: []float64{20, 20, 20}}
	max := &point{coord: []float64{60, 60, 60}}

	want := make(map[int]bool)
	for _, p := range points {
		inBox := true
		for d := 0; d < dim; d++ {
			if p.GetValue(d) < min.GetValue(d) || p.GetValue(d) > max.GetValue(d) {
				inBox = false
				break
			}
		}
		if inBox {
			want[p.key] = true
		}
	}

	got := rt.Range(min, max)
	if len(got) != len(want) {
		t.Fatalf("Range returned %d nodes, want %d", len(got), len(want))
	}
	for _, node := range got {
		if !want[node.Key()] {
			t.Errorf("Range returned key %d, which is outside the box", node.Key())
		}
	}
}

func TestRPTreeDeterministicAcrossRuns(t *testing.T) {
	build := func() []int {
		rng := rand.New(rand.NewSource(123))
		rt := trees.NewRPTree[int, string, float64](3, 4, 99)
		for i := 0; i < 200; i++ {
			_ = rt.Insert(randPoint(rng, i, 3))
		}
		keys := make([]int, 0, 200)
		for _, node := range rt.Nodes() {
			keys = append(keys, node.Key())
		}
		sort.Ints(keys)
		return keys
	}

	a, b := build(), build()
	if len(a) != len(b) {
		t.Fatalf("got %d and %d keys across two runs with the same seed", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("runs diverged at index %d: %d vs %d", i, a[i], b[i])
		}
	}
}

func TestVectorPointGeometry(t *testing.T) {
	a := trees.NewVectorPoint(1, containers.NewVector[int]([]float64{0, 0, 0}))
	b := trees.NewVectorPoint(2, containers.NewVector[int]([]float64{3, 4, 0}))

	if got, want := a.Dim(), 3; got != want {
		t.Errorf("Dim() = %d, want %d", got, want)
	}
	if got, want := a.Key(), 1; got != want {
		t.Errorf("Key() = %d, want %d", got, want)
	}
	if got, want := a.Distance(b), 5.0; got != want {
		t.Errorf("Distance() = %v, want %v", got, want)
	}
	if got, want := b.Distance(a), 5.0; got != want {
		t.Errorf("Distance() (symmetric) = %v, want %v", got, want)
	}
	if got, want := a.PlaneDistance(3, 0), 3.0; got != want {
		t.Errorf("PlaneDistance(3, 0) = %v, want %v", got, want)
	}
	if got, want := a.PlaneDistance(-3, 0), 3.0; got != want {
		t.Errorf("PlaneDistance(-3, 0) = %v, want %v", got, want)
	}
}

func TestVectorPointHash(t *testing.T) {
	hasher := hash.XxHash[uint64]{}
	a := trees.NewVectorPoint(uint64(1), containers.NewVector[uint64]([]float64{1, 2, 3}))
	b := trees.NewVectorPoint(uint64(2), containers.NewVector[uint64]([]float64{1, 2, 3}))
	c := trees.NewVectorPoint(uint64(3), containers.NewVector[uint64]([]float64{4, 5, 6}))

	if got, want := a.Hash(hasher), b.Hash(hasher); got != want {
		t.Errorf("points with equal coordinates hashed differently: %#x vs %#x", got, want)
	}
	if got := a.Hash(hasher); got == c.Hash(hasher) {
		t.Errorf("points with different coordinates hashed the same: %#x", got)
	}
}

func TestVectorNode(t *testing.T) {
	vec := containers.NewVector[int]([]float64{1, 2, 3})
	n := trees.NewVectorNode(42, vec)

	if got, want := n.Key(), 42; got != want {
		t.Errorf("Key() = %d, want %d", got, want)
	}
	if got, want := n.Value(), vec; got.Dim() != want.Dim() {
		t.Errorf("Value() = %v, want %v", got, want)
	}

	p := n.Point()
	if p == nil {
		t.Fatalf("Point() = nil, want a VectorPoint")
	}
	if got, want := p.Key(), 42; got != want {
		t.Errorf("Point().Key() = %d, want %d", got, want)
	}
	if got, want := p.Dim(), 3; got != want {
		t.Errorf("Point().Dim() = %d, want %d", got, want)
	}
}

// TestRPTreeWithVectorNodes exercises RPTree end-to-end using VectorNode, the
// same adapter RPTreeIndex will build in internal/index/rptree.go.
func TestRPTreeWithVectorNodes(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	const dim = 4
	const n = 20 // stays under the default leaf size

	rt := trees.NewRPTree[int, containers.Vector[int, float64], float64](dim, 4, 13)

	type sample struct {
		key   int
		coord []float64
	}
	samples := make([]sample, n)
	for i := range samples {
		coord := make([]float64, dim)
		for d := range coord {
			coord[d] = rng.Float64() * 100
		}
		samples[i] = sample{key: i, coord: coord}
		node := trees.NewVectorNode(i, containers.NewVector[int](coord))
		if err := rt.Insert(node); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}
	if got := rt.Len(); got != n {
		t.Fatalf("Len() = %d, want %d", got, n)
	}

	query := trees.NewVectorNode(-1, containers.NewVector[int]([]float64{50, 50, 50, 50})).Point()
	got := rt.Nearest(query, 3)
	if len(got) != 3 {
		t.Fatalf("Nearest returned %d nodes, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if query.Distance(got[i-1].Point()) > query.Distance(got[i].Point()) {
			t.Errorf("Nearest results not sorted by distance at index %d", i)
		}
	}
}
