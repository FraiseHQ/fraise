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

// Black-box tests (package containers_test) for the growable max-priority queue.
// The backing slice and capacity hint are unexported, so the contract is verified
// through the public API only:
//
//   - NewPriorityQueue rejects a zero capacity and otherwise yields an empty queue.
//   - Dequeue always surfaces the highest-Priority item; draining is non-increasing.
//   - Peek observes the max without removing it.
//   - The queue is growable: `capacity` is an initial size hint, not a bound, so
//     enqueuing past it grows the queue rather than evicting anything.
//
// The implementation orders by Item.Priority as a MAX-priority queue (largest
// first), matching the Item docs and the sibling Heap. Flip `less`/`byPriority`
// below if the intended ordering is ever inverted.
package containers_test

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/containers"
)

// ---- helpers ---------------------------------------------------------------

// newPQ builds a queue of the given capacity and fails the test if construction
// errors, so callers can use the queue directly.
func newPQ(t *testing.T, capacity uint) *containers.PriorityQueue[uint32, int] {
	t.Helper()
	pq, err := containers.NewPriorityQueue[uint32, int](capacity)
	if err != nil {
		t.Fatalf("NewPriorityQueue(%d) returned error: %v", capacity, err)
	}
	if pq == nil {
		t.Fatalf("NewPriorityQueue(%d) returned nil queue", capacity)
	}
	return pq
}

// item is a terse constructor for queue entries.
func item(key uint32, pri uint64) containers.Item[uint32, int] {
	return containers.Item[uint32, int]{Key: key, Value: int(key), Priority: pri}
}

// drainPQ dequeues everything and returns the priorities in pop order.
// (assertNonIncreasing is shared with heap_test.go in this package.)
func drainPQ(pq *containers.PriorityQueue[uint32, int]) []uint64 {
	out := make([]uint64, 0, pq.Len())
	for !pq.Empty() {
		it, _ := pq.Dequeue()
		if it == nil {
			break
		}
		out = append(out, it.Priority)
	}
	return out
}

// ---- constructor tests -----------------------------------------------------

func TestNewPriorityQueue_ZeroCapacity(t *testing.T) {
	pq, err := containers.NewPriorityQueue[uint32, int](0)
	if err == nil {
		t.Fatal("NewPriorityQueue(0) = nil error, want ErrPriorityQueueCapacity")
	}
	if pq != nil {
		t.Fatalf("NewPriorityQueue(0) returned non-nil queue %v alongside error", pq)
	}
}

func TestNewPriorityQueue_StartsEmpty(t *testing.T) {
	pq := newPQ(t, 8)

	if pq.Len() != 0 {
		t.Errorf("fresh queue Len() = %d, want 0", pq.Len())
	}
	if !pq.Empty() {
		t.Error("fresh queue Empty() = false, want true")
	}
	if it := pq.Peek(); it != nil {
		t.Errorf("Peek() on empty queue = %v, want nil", it)
	}
	if it, _ := pq.Dequeue(); it != nil {
		t.Errorf("Dequeue() on empty queue = %v, want nil", it)
	}
}

// ---- behaviour tests -------------------------------------------------------

func TestEnqueue_LenAndPeek(t *testing.T) {
	pq := newPQ(t, 8)

	pq.Enqueue(item(1, 102))
	pq.Enqueue(item(2, 13))
	pq.Enqueue(item(3, 1))
	pq.Enqueue(item(4, 1045))

	if pq.Len() != 4 {
		t.Fatalf("Len() = %d after 4 enqueues, want 4", pq.Len())
	}
	if pq.Empty() {
		t.Error("Empty() = true on non-empty queue")
	}
	// The largest priority must surface at the head.
	top := pq.Peek()
	if top == nil || top.Priority != 1045 {
		t.Fatalf("Peek() = %v, want priority 1045", top)
	}
	// Peek must not remove anything.
	if pq.Len() != 4 {
		t.Fatalf("Len() = %d after Peek, want 4", pq.Len())
	}
}

func TestDequeue_OrdersByPriority(t *testing.T) {
	pq := newPQ(t, 16)
	for i, v := range []uint64{102, 13, 1, 1045, 7} {
		pq.Enqueue(item(uint32(i), v))
	}

	got := drainPQ(pq)
	want := []uint64{1045, 102, 13, 7, 1}
	if len(got) != len(want) {
		t.Fatalf("drained %d items, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dequeue order[%d] = %d, want %d (full: %v)", i, got[i], want[i], got)
		}
	}

	if !pq.Empty() || pq.Len() != 0 {
		t.Errorf("queue not empty after draining: Len=%d Empty=%v", pq.Len(), pq.Empty())
	}
	if it, _ := pq.Dequeue(); it != nil {
		t.Errorf("Dequeue() on drained queue = %v, want nil", it)
	}
}

func TestDequeue_PreservesValue(t *testing.T) {
	pq := newPQ(t, 4)
	pq.Enqueue(containers.Item[uint32, int]{Key: 7, Value: 42, Priority: 100})

	it, _ := pq.Dequeue()
	if it == nil {
		t.Fatal("Dequeue() = nil, want the enqueued item")
	}
	if it.Key != 7 || it.Value != 42 || it.Priority != 100 {
		t.Errorf("Dequeue() = %+v, want {Key:7 Value:42 Priority:100}", *it)
	}
}

// ---- growth (unbounded) tests ----------------------------------------------

// TestEnqueue_GrowsPastCapacity checks the growable contract: `capacity` is only
// an initial hint, so enqueuing past it retains every item rather than evicting
// the lowest, and the max ordering is preserved across the growth boundary.
func TestEnqueue_GrowsPastCapacity(t *testing.T) {
	pq := newPQ(t, 3)
	pq.Enqueue(item(1, 10))
	pq.Enqueue(item(2, 20))
	pq.Enqueue(item(3, 30)) // at the capacity hint: {10,20,30}

	pq.Enqueue(item(4, 25)) // past the hint: nothing dropped -> {10,20,25,30}

	if pq.Len() != 4 {
		t.Fatalf("Len() = %d after enqueue past capacity, want 4 (growable)", pq.Len())
	}
	got := drainPQ(pq)
	want := []uint64{30, 25, 20, 10}
	if len(got) != len(want) {
		t.Fatalf("drained %d items, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("order[%d] = %d, want %d (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestEnqueue_KeepsSmallItems is the dual of the old eviction test: a new item
// smaller than everything present is kept, not dropped.
func TestEnqueue_KeepsSmallItems(t *testing.T) {
	pq := newPQ(t, 3)
	pq.Enqueue(item(1, 10))
	pq.Enqueue(item(2, 20))
	pq.Enqueue(item(3, 30))

	pq.Enqueue(item(4, 5)) // smaller than all -> still retained

	if pq.Len() != 4 {
		t.Fatalf("Len() = %d, want 4", pq.Len())
	}
	got := drainPQ(pq)
	want := []uint64{30, 20, 10, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("order[%d] = %d, want %d (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestEnqueue_CapacityIsHintNotBound enqueues well past the initial hint and
// checks that Len tracks every insert while Cap only ever grows to accommodate.
func TestEnqueue_CapacityIsHintNotBound(t *testing.T) {
	const hint = 4
	pq := newPQ(t, hint)
	if pq.Cap() < hint {
		t.Fatalf("fresh queue Cap() = %d, want >= hint %d", pq.Cap(), hint)
	}

	const n = 100
	for i := 0; i < n; i++ {
		pq.Enqueue(item(uint32(i), uint64(i)))
	}

	if pq.Len() != n {
		t.Fatalf("Len() = %d after %d enqueues, want %d (nothing evicted)", pq.Len(), n, n)
	}
	if pq.Cap() < n {
		t.Fatalf("Cap() = %d after growth, want >= %d", pq.Cap(), n)
	}
	if top := pq.Peek(); top == nil || top.Priority != n-1 {
		t.Fatalf("Peek() = %v, want priority %d", top, n-1)
	}
}

// ---- randomized property test ----------------------------------------------

// less reports the max-priority ordering used by the queue. Invert to switch to
// a min-priority queue.
func less(a, b uint64) bool { return a < b }

// byPriority sorts priorities into pop order (descending for a max-queue).
func byPriority(s []uint64) { sort.Slice(s, func(i, j int) bool { return less(s[j], s[i]) }) }

// TestFuzz_AgainstReferenceModel drives a long random sequence of Enqueue and
// Dequeue against a plain slice used as an oracle. The queue is growable, so the
// model simply tracks every live item; after each step Len and Peek must agree
// and Dequeue must surface a true maximum priority.
//
// Keys are drawn from a wide range and the queue is not assumed to dedup by key;
// the model therefore tracks a multiset of priorities.
func TestFuzz_PQAgainstReferenceModel(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const capacity = 32 // initial hint only; the queue grows past it
	pq := newPQ(t, capacity)
	model := make([]uint64, 0, capacity) // multiset of live priorities

	maxPri := func() (uint64, bool) {
		best, ok := uint64(0), false
		for _, p := range model {
			if !ok || less(best, p) {
				best, ok = p, true
			}
		}
		return best, ok
	}
	removeOne := func(target uint64) {
		for i, p := range model {
			if p == target {
				model = append(model[:i], model[i+1:]...)
				return
			}
		}
	}

	const steps = 20000
	for i := 0; i < steps; i++ {
		switch rng.Intn(3) {
		case 0, 1: // Enqueue (weighted so the queue tends to grow)
			pri := uint64(rng.Intn(1000))
			// Unique key per enqueue: the heap dedups by key (keeping the higher
			// priority), but the oracle is a plain priority multiset, so distinct
			// keys keep the two in agreement. Dedup is covered by the heap tests.
			pq.Enqueue(item(uint32(i), pri))
			model = append(model, pri)
		case 2: // Dequeue
			want, ok := maxPri()
			got, _ := pq.Dequeue()
			if ok != (got != nil) {
				t.Fatalf("step %d: Dequeue presence mismatch (model non-empty=%v got=%v)", i, ok, got)
			}
			if got != nil {
				if got.Priority != want {
					t.Fatalf("step %d: Dequeue priority = %d, want max %d", i, got.Priority, want)
				}
				removeOne(got.Priority)
			}
		}

		if pq.Len() != len(model) {
			t.Fatalf("step %d: Len() = %d, model size = %d", i, pq.Len(), len(model))
		}
		if want, ok := maxPri(); ok {
			if p := pq.Peek(); p == nil || p.Priority != want {
				t.Fatalf("step %d: Peek() = %v, want max priority %d", i, p, want)
			}
		} else if p := pq.Peek(); p != nil {
			t.Fatalf("step %d: Peek() = %v, want nil (model empty)", i, p)
		}
	}
}
