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

// point is a test double for trees.TreeNode[int, string, float64] and
// trees.Point[int, float64]. Its Point() returns the receiver itself. That is
// all an RPTree needs to index it.
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

func (p *point) Dim() int               { return len(p.coord) }
func (p *point) Coordinates() []float64 { return p.coord }
func (p *point) Distance(o trees.Point[int, float64]) (float64, error) {
	if o.Dim() != p.Dim() {
		return 0, containers.ErrDimMismatch
	}
	other := o.Coordinates()
	var sum float64
	for d, x := range p.coord {
		diff := x - other[d]
		sum += diff * diff
	}
	return math.Sqrt(sum), nil
}

func randPoint(rng *rand.Rand, key, dim int) *point {
	coord := make([]float64, dim)
	for i := range coord {
		coord[i] = rng.Float64() * 100
	}
	return &point{key: key, value: "v", coord: coord}
}

// distance returns the distance between a and b, and fails the test on an
// error. Every caller passes two points of one dimension.
func distance(t *testing.T, a, b trees.Point[int, float64]) float64 {
	t.Helper()
	d, err := a.Distance(b)
	if err != nil {
		t.Fatalf("Distance = %v, want nil", err)
	}
	return d
}

// nearest runs rt.Nearest and fails the test on an error. Every caller's
// query has the tree's dimension.
func nearest[T any](t *testing.T, rt *trees.RPTree[int, T, float64], p trees.Point[int, float64], k int, keep func(int) bool) []trees.TreeNode[int, T, float64] {
	t.Helper()
	got, err := rt.Nearest(p, k, keep)
	if err != nil {
		t.Fatalf("Nearest = %v, want nil", err)
	}
	return got
}

func bruteForceNearest(t *testing.T, points []*point, q *point, k int) []int {
	type scored struct {
		key int
		d   float64
	}
	scores := make([]scored, len(points))
	for i, p := range points {
		scores[i] = scored{key: p.key, d: distance(t, q, p)}
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
	if got := nearest(t, rt, &point{coord: []float64{0, 0, 0}}, 3, nil); got != nil {
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
// capacity, so the tree never splits. Nearest is then a plain distance scan
// and must match brute force exactly.
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
	want := bruteForceNearest(t, points, query, 5)

	got := nearest(t, rt, query, 5, nil)
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
// at the query and again farther away. Pooling must keep the near copy. The
// far copy must not displace it and then be evicted as the farthest. And the
// collision on a full pool must not cost a slot, so all k come back.
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

	got := nearest(t, rt, &point{coord: []float64{0, 0}}, 2, nil)
	if len(got) != 2 {
		t.Fatalf("Nearest returned %d nodes, want 2", len(got))
	}
	if got[0].Key() != 1 || got[0].Value() != "live" || got[1].Key() != 2 {
		t.Errorf("Nearest() = [%d %s, %d %s], want [1 live, 2 v]",
			got[0].Key(), got[0].Value(), got[1].Key(), got[1].Value())
	}
}

// TestRPTreeNearestReturnsSubsetOfStoredNodes checks the structural contract
// of the approximate Nearest search on a tree that splits. It must return at
// most k nodes, all of them inserted and none twice. They must come in
// non-decreasing true distance to the query.
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
	got := nearest(t, rt, query, k, nil)
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

		d := distance(t, query, p)
		if d < prevDist {
			t.Fatalf("Nearest results not sorted by distance: %v got distance %v after %v", node.Key(), d, prevDist)
		}
		prevDist = d
	}
}

// TestRPTreeNearestFillsKBeyondOneLeaf pins that Nearest fills k past one
// leaf. A single root-to-leaf descent sees one leaf. Without probing, it would
// return at most leafSize nodes however large k is. A caller could not tell
// that apart from an index that holds no more. Here k is several leaves'
// worth, in a tree with far more points than that.
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

	if got := nearest(t, rt, randPoint(rng, -1, dim), k, nil); len(got) != k {
		t.Fatalf("Nearest(k=%d) returned %d nodes out of %d stored, want %d — the search stopped at one leaf", k, len(got), n, k)
	}
}

// TestRPTreeNearestIsExactWhenKCoversTheTree pins the probing walk's
// correctness. Asking for every stored point exhausts the deferred probes. So
// the answer must be the exact brute-force ranking, and a leaf the walk misses
// shows up as a missing point. Unlike a recall threshold, this test cannot
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
	got := nearest(t, rt, query, n, nil)
	if len(got) != n {
		t.Fatalf("Nearest(k=n) returned %d of %d points, want all — probing must exhaust the tree", len(got), n)
	}

	want := make([]*point, n)
	copy(want, all)
	sort.Slice(want, func(i, j int) bool { return distance(t, query, want[i]) < distance(t, query, want[j]) })
	for i, node := range got {
		if d, wantD := distance(t, query, all[node.Key()]), distance(t, query, want[i]); d != wantD {
			t.Fatalf("Nearest rank %d is at distance %v, want %v — the walk missed or repeated a leaf", i, d, wantD)
		}
	}
}

// TestRPTreeNearestKIsACapNotASize pins that k bounds the answer but sizes no
// allocation. k comes from a recall's top:. A k far past the tree, too large
// for any machine to reserve memory for, must still return every stored point.
// If the pool were sized on k, this would panic in makeslice.
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

	if got := nearest(t, rt, randPoint(rng, -1, dim), 1<<50, nil); len(got) != n {
		t.Fatalf("Nearest(k=1<<50) returned %d of %d points, want all", len(got), n)
	}
}

// TestRPTreeOverfetchWidensTheCandidatePool pins what the configured factor
// buys. The projection only decides where to look, and true distance decides
// what comes back. So more candidates can improve the answer or tie, but never
// make it worse. The k-th distance is non-increasing in overfetch. A factor
// large enough to exhaust the tree is exact. So db.vector-search.overfetch
// trades query cost for recall and cannot overshoot into a worse result.
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
		got := nearest(t, rt, query, k, nil)
		if len(got) != k {
			t.Fatalf("Nearest returned %d nodes, want %d", len(got), k)
		}
		return distance(t, query, points[got[k-1].Key()])
	}

	narrow, wide := kth(build(1)), kth(build(n))
	if wide > narrow {
		t.Errorf("over-fetch %d gave a worse k-th distance (%v) than over-fetch 1 (%v) — more candidates must never rank worse", n, wide, narrow)
	}

	// Exhausting the tree leaves nothing for the approximation to miss.
	want := bruteForceNearest(t, points, query, k)
	for i, node := range nearest(t, build(n), query, k, nil) {
		if node.Key() != want[i] {
			t.Errorf("with over-fetch %d, Nearest()[%d].Key() = %d, want %d — a factor past the tree size must be exact", n, i, node.Key(), want[i])
		}
	}
}

// TestRPTreeNearestKeepProbesForAcceptedNodes pins that only the nodes keep
// accepts count against the budget. One node in fifty is accepted, so at
// over-fetch 1 the first leaves hold almost none of them. A tree that filtered
// after gathering k candidates would return short. Nearest must probe on until
// it holds k accepted nodes. Exhausting the tree makes the filtered answer
// exact.
func TestRPTreeNearestKeepProbesForAcceptedNodes(t *testing.T) {
	rng := rand.New(rand.NewSource(37))
	const dim = 4
	const n = 600
	const k = 5

	points := make([]*point, n)
	accepted := make([]*point, 0, n/50)
	for i := range points {
		points[i] = randPoint(rng, i, dim)
		if i%50 == 0 {
			accepted = append(accepted, points[i])
		}
	}
	keep := func(key int) bool { return key%50 == 0 }
	query := randPoint(rng, -1, dim)

	build := func(overfetch int) *trees.RPTree[int, string, float64] {
		rt := trees.NewRPTree[int, string, float64](dim, 4, 3, 8, overfetch)
		for _, p := range points {
			if err := rt.Insert(p); err != nil {
				t.Fatalf("Insert(%d) = %v, want nil", p.Key(), err)
			}
		}
		return rt
	}

	got := nearest(t, build(1), query, k, keep)
	if len(got) != k {
		t.Fatalf("Nearest(k=%d, keep) returned %d nodes, want %d — the budget counted rejected nodes", k, len(got), k)
	}
	for _, node := range got {
		if !keep(node.Key()) {
			t.Errorf("Nearest(keep) returned key %d, which keep rejects", node.Key())
		}
	}

	want := bruteForceNearest(t, accepted, query, k)
	for i, node := range nearest(t, build(n), query, k, keep) {
		if node.Key() != want[i] {
			t.Errorf("with over-fetch %d, Nearest(keep)[%d].Key() = %d, want %d — the nearest accepted nodes", n, i, node.Key(), want[i])
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
			if p.coord[d] < min.coord[d] || p.coord[d] > max.coord[d] {
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

// TestRPTreeDeterministicAcrossRuns pins that the seed alone fixes the tree.
// Two builds over the same points with the same seed must answer every query
// with the same neighbours in the same order. Membership cannot show this,
// since every build holds every point. So the probe is Nearest with a budget
// of one leaf. Its approximate answer is whatever the split structure puts
// beside the query. A different seed must change at least one answer.
// Otherwise the test would also pass for a tree that ignored its seed.
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
			for _, node := range nearest(t, rt, randPoint(rng, -1, 3).Point(), k, nil) {
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

// TestRPTreeNearestRejectsQueryOfWrongDimension pins that Nearest refuses a
// query whose dimension differs from the tree's, as Insert refuses such a
// point. A shorter query would otherwise be read past its last coordinate.
func TestRPTreeNearestRejectsQueryOfWrongDimension(t *testing.T) {
	rt := trees.NewRPTree[int, string, float64](3, 4, 1, 32, 8)
	if err := rt.Insert(&point{key: 1, value: "v", coord: []float64{1, 2, 3}}); err != nil {
		t.Fatalf("Insert = %v, want nil", err)
	}

	cases := map[string]struct{ coord []float64 }{
		"shorter": {[]float64{1, 2}},
		"longer":  {[]float64{1, 2, 3, 4}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := rt.Nearest(&point{coord: tc.coord}, 1, nil)
			if !errors.Is(err, trees.ErrDimensionMismatch) {
				t.Errorf("Nearest(%d-d query) = (%v, %v), want ErrDimensionMismatch", len(tc.coord), got, err)
			}
		})
	}
}

// TestVectorPointDistanceRejectsOtherDimensions pins that two points of
// different dimensions have no distance. That holds against another
// VectorPoint and against any other Point.
func TestVectorPointDistanceRejectsOtherDimensions(t *testing.T) {
	a := trees.NewVectorPoint(1, containers.NewVector[int]([]float64{0, 0, 0}))

	cases := map[string]struct{ other trees.Point[int, float64] }{
		"vector point": {trees.NewVectorPoint(2, containers.NewVector[int]([]float64{3, 4}))},
		"other point":  {&point{coord: []float64{3, 4}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := a.Distance(tc.other); !errors.Is(err, containers.ErrDimMismatch) {
				t.Errorf("Distance(2-d point) = %v, want ErrDimMismatch", err)
			}
		})
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
	if got, want := distance(t, a, b), 5.0; got != want {
		t.Errorf("Distance() = %v, want %v", got, want)
	}
	if got, want := distance(t, b, a), 5.0; got != want {
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

// TestRPTreeWithVectorNodes runs RPTree end to end with VectorNode, the node
// type RPTreeIndex inserts (internal/index/rptree.go).
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
	got := nearest(t, rt, query, 3, nil)
	if len(got) != 3 {
		t.Fatalf("Nearest returned %d nodes, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if distance(t, query, got[i-1].Point()) > distance(t, query, got[i].Point()) {
			t.Errorf("Nearest results not sorted by distance at index %d", i)
		}
	}
}
