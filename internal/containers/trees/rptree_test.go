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
	"reflect"
	"sort"
	"testing"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/containers/trees"
)

// point is a test double implementing both trees.TreeNode[int, string, float64]
// and trees.Point[int, float64] (Point() returns the receiver itself), which
// is all an RPTree needs to index it.
type point struct {
	key   int
	value string
	coord []float64
}

func (p *point) Key() int      { return p.key }
func (p *point) Value() string { return p.value }

func (p *point) Point() trees.Point[int, float64] {
	if p.coord == nil {
		return nil
	}
	return p
}

func (p *point) Dim() int                 { return len(p.coord) }
func (p *point) GetValue(dim int) float64 { return p.coord[dim] }
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
	rt := trees.NewRPTree[int, string, float64](3, 4, 1, 32, 8)
	if got := rt.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if got := rt.Nearest(&point{coord: []float64{0, 0, 0}}, 3); got != nil {
		t.Errorf("Nearest on empty tree = %v, want nil", got)
	}
}

func TestRPTreeInsertRejectsBadPoints(t *testing.T) {
	rt := trees.NewRPTree[int, string, float64](3, 4, 1, 32, 8)

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
	const n = 20 // well under the leaf size this tree is built with

	rt := trees.NewRPTree[int, string, float64](dim, 4, 7, 32, 8)
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

// TestRPTreeNearestPoolsEachKeyAtItsNearestCopy pins Nearest when a key is
// stored twice, as RPTreeIndex leaves an old copy behind on update. Key 1 sits
// at the query and again far away: pooling must keep the near copy, not let the
// far one displace it and then be evicted as the farthest; and the collision on
// a full pool must not cost a slot, so all k come back.
func TestRPTreeNearestPoolsEachKeyAtItsNearestCopy(t *testing.T) {
	rt := trees.NewRPTree[int, string, float64](2, 2, 0, 100, 1)
	for _, p := range []*point{
		{key: 9, value: "v", coord: []float64{9, 0}},
		{key: 1, value: "stale", coord: []float64{2, 0}},
		{key: 1, value: "live", coord: []float64{0, 0}},
		{key: 2, value: "v", coord: []float64{1, 0}},
		{key: 3, value: "v", coord: []float64{1.5, 0}},
	} {
		if err := rt.Insert(p); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", p.key, err)
		}
	}

	got := rt.Nearest(&point{coord: []float64{0, 0}}, 2)
	if len(got) != 2 {
		t.Fatalf("Nearest returned %d nodes, want 2", len(got))
	}
	if got[0].Key() != 1 || got[0].Value() != "live" || got[1].Key() != 2 {
		t.Errorf("Nearest() = [%d %s, %d %s], want [1 live, 2 v]",
			got[0].Key(), got[0].Value(), got[1].Key(), got[1].Value())
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

	rt := trees.NewRPTree[int, string, float64](dim, 6, 5, 32, 8)
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

// TestRPTreeNearestFillsKBeyondOneLeaf pins that Nearest fills k past one
// leaf: a single root-to-leaf descent sees one leaf, so without probing it
// would return at most leafSize nodes however large k is, which a caller
// cannot tell apart from an index that holds no more. k here is several
// leaves' worth of a tree with far more points than that.
func TestRPTreeNearestFillsKBeyondOneLeaf(t *testing.T) {
	rng := rand.New(rand.NewSource(23))
	const dim = 8
	const n = 1000
	const k = 200

	rt := trees.NewRPTree[int, string, float64](dim, 6, 3, 32, 8)
	for i := 0; i < n; i++ {
		if err := rt.Insert(randPoint(rng, i, dim)); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}

	if got := rt.Nearest(randPoint(rng, -1, dim), k); len(got) != k {
		t.Fatalf("Nearest(k=%d) returned %d nodes out of %d stored, want %d — the search stopped at one leaf", k, len(got), n, k)
	}
}

// TestRPTreeNearestIsExactWhenKCoversTheTree pins the probing walk's
// correctness. Asking for every stored point exhausts the deferred probes, so
// the answer must be the exact brute-force ranking, and any leaf the walk fails
// to reach shows up as a missing point. Unlike a recall threshold, it cannot
// pass by luck.
func TestRPTreeNearestIsExactWhenKCoversTheTree(t *testing.T) {
	rng := rand.New(rand.NewSource(29))
	const dim = 5
	const n = 300

	rt := trees.NewRPTree[int, string, float64](dim, 6, 4, 32, 8)
	all := make([]*point, n)
	for i := range all {
		p := randPoint(rng, i, dim)
		all[i] = p
		if err := rt.Insert(p); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}

	query := randPoint(rng, -1, dim)
	got := rt.Nearest(query, n)
	if len(got) != n {
		t.Fatalf("Nearest(k=n) returned %d of %d points, want all — probing must exhaust the tree", len(got), n)
	}

	want := make([]*point, n)
	copy(want, all)
	sort.Slice(want, func(i, j int) bool { return query.Distance(want[i]) < query.Distance(want[j]) })
	for i, node := range got {
		if d, wantD := query.Distance(all[node.Key()]), query.Distance(want[i]); d != wantD {
			t.Fatalf("Nearest rank %d is at distance %v, want %v — the walk missed or repeated a leaf", i, d, wantD)
		}
	}
}

// TestRPTreeNearestKIsACapNotASize pins that k bounds the answer without
// sizing any allocation: k reaches here from a recall's top:, so a k far past
// the tree, which no machine could reserve memory for, must still return every
// stored point. If the pool were sized on k, this would panic in makeslice.
func TestRPTreeNearestKIsACapNotASize(t *testing.T) {
	rng := rand.New(rand.NewSource(37))
	const dim = 4
	const n = 50

	rt := trees.NewRPTree[int, string, float64](dim, 4, 4, 8, 2)
	for i := range n {
		if err := rt.Insert(randPoint(rng, i, dim)); err != nil {
			t.Fatalf("Insert(%d) = %v, want nil", i, err)
		}
	}

	if got := rt.Nearest(randPoint(rng, -1, dim), 1<<50); len(got) != n {
		t.Fatalf("Nearest(k=1<<50) returned %d of %d points, want all", len(got), n)
	}
}

// TestRPTreeOverfetchWidensTheCandidatePool pins what the configured factor
// buys. The projection only decides where to look and true distance decides
// what comes back, so more candidates can improve the answer or tie but never
// worsen it: the k-th distance is non-increasing in overfetch. A factor large
// enough to exhaust the tree is exact, so db.vector-search.overfetch trades
// query cost for recall and cannot overshoot into a worse result.
func TestRPTreeOverfetchWidensTheCandidatePool(t *testing.T) {
	rng := rand.New(rand.NewSource(31))
	const dim = 6
	const n = 600
	const k = 10

	points := make([]*point, n)
	for i := range points {
		points[i] = randPoint(rng, i, dim)
	}
	query := randPoint(rng, -1, dim)

	build := func(overfetch int) *trees.RPTree[int, string, float64] {
		rt := trees.NewRPTree[int, string, float64](dim, 6, 17, 32, overfetch)
		for _, p := range points {
			if err := rt.Insert(p); err != nil {
				t.Fatalf("Insert(%d) = %v, want nil", p.Key(), err)
			}
		}
		return rt
	}

	kth := func(rt *trees.RPTree[int, string, float64]) float64 {
		got := rt.Nearest(query, k)
		if len(got) != k {
			t.Fatalf("Nearest returned %d nodes, want %d", len(got), k)
		}
		return query.Distance(points[got[k-1].Key()])
	}

	narrow, wide := kth(build(1)), kth(build(n))
	if wide > narrow {
		t.Errorf("over-fetch %d gave a worse k-th distance (%v) than over-fetch 1 (%v) — more candidates must never rank worse", n, wide, narrow)
	}

	// Exhausting the tree leaves nothing for the approximation to miss.
	want := bruteForceNearest(points, query, k)
	for i, node := range build(n).Nearest(query, k) {
		if node.Key() != want[i] {
			t.Errorf("with over-fetch %d, Nearest()[%d].Key() = %d, want %d — a factor past the tree size must be exact", n, i, node.Key(), want[i])
		}
	}
}

func TestRPTreeRangeIsExact(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	const dim = 3
	const n = 400

	rt := trees.NewRPTree[int, string, float64](dim, 5, 2, 32, 8)
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

// TestRPTreeDeterministicAcrossRuns pins that the seed alone fixes the tree:
// two builds over the same points with the same seed answer every query with
// the same neighbours in the same order. Membership cannot show this — every
// build holds every point — so the probe is Nearest with a budget of one leaf,
// whose approximate answer is whatever the split structure puts beside the
// query. A different seed must change at least one answer, or the comparison
// would pass for a tree that ignored its seed as well.
func TestRPTreeDeterministicAcrossRuns(t *testing.T) {
	const k = 3
	answers := func(seed uint64) [][]int {
		rng := rand.New(rand.NewSource(123))
		rt := trees.NewRPTree[int, string, float64](3, 4, seed, 8, 1)
		for i := 0; i < 200; i++ {
			_ = rt.Insert(randPoint(rng, i, 3))
		}
		out := make([][]int, 0, 50)
		for q := 0; q < 50; q++ {
			keys := make([]int, 0, k)
			for _, node := range rt.Nearest(randPoint(rng, -1, 3).Point(), k) {
				keys = append(keys, node.Key())
			}
			out = append(out, keys)
		}
		return out
	}

	a, b := answers(99), answers(99)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("two builds with seed 99 answered differently:\n%v\n%v", a, b)
	}
	if reflect.DeepEqual(a, answers(100)) {
		t.Fatalf("seeds 99 and 100 answered every query alike, want the seed to shape the tree")
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

// TestRPTreeWithVectorNodes exercises RPTree end to end with VectorNode, the
// node type RPTreeIndex inserts (internal/index/rptree.go).
func TestRPTreeWithVectorNodes(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	const dim = 4
	const n = 20 // stays under this tree's leaf size

	rt := trees.NewRPTree[int, containers.Vector[int, float64], float64](dim, 4, 13, 32, 8)

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
