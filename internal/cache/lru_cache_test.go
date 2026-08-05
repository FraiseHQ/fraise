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

package cache_test

import (
	"fmt"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/cache"
)

func TestLRUCache_GetMiss(t *testing.T) {
	c, _ := cache.NewLRUCache[string, int](3)
	if _, ok := c.Get("nope"); ok {
		t.Fatal("expected miss")
	}
}

func TestLRUCache_PutGet(t *testing.T) {
	c, _ := cache.NewLRUCache[string, int](3)
	c.Put("a", 1)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Fatalf("got %v, %v; want 1, true", v, ok)
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	c, _ := cache.NewLRUCache[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // should evict "a"

	if _, ok := c.Get("a"); ok {
		t.Fatal("a should have been evicted")
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Fatalf("b: got %v, %v; want 2, true", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Fatalf("c: got %v, %v; want 3, true", v, ok)
	}
}

func TestLRUCache_GetMovesToFront(t *testing.T) {
	c, _ := cache.NewLRUCache[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a")    // touch "a" so it's most recent
	c.Put("c", 3) // should evict "b", not "a"

	if _, ok := c.Get("a"); !ok {
		t.Fatal("a should still be present")
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted")
	}
}

func TestLRUCache_Update(t *testing.T) {
	c, _ := cache.NewLRUCache[string, int](2)
	c.Put("a", 1)
	c.Put("a", 2)
	if v, ok := c.Get("a"); !ok || v != 2 {
		t.Fatalf("got %v, %v; want 2, true", v, ok)
	}
	if c.Len() != 1 {
		t.Fatalf("got len %d; want 1", c.Len())
	}
}

// TestLRUCache_ResizeShrink is the regression guard for the no-op-shrink bug:
// Resize(smaller) must evict immediately, dropping least-recently-used entries
// first, not defer eviction to later Puts.
func TestLRUCache_ResizeShrink(t *testing.T) {
	c, _ := cache.NewLRUCache[int, int](10)
	for i := 0; i < 10; i++ {
		c.Put(i, i) // 0 is now least-recently-used, 9 most-recently-used
	}

	evicted, err := c.Resize(3)
	if err != nil {
		t.Fatalf("Resize returned error: %v", err)
	}
	if evicted != 7 {
		t.Errorf("Resize evicted %d, want 7", evicted)
	}
	if got := c.Len(); got != 3 {
		t.Fatalf("Len() after Resize(3) = %d, want 3", got)
	}
	if got := c.Capacity(); got != 3 {
		t.Errorf("Capacity() after Resize(3) = %d, want 3", got)
	}

	// The 3 most-recently-used survive; the 7 oldest are gone.
	for i := 0; i < 7; i++ {
		if _, ok := c.Get(i); ok {
			t.Errorf("key %d should have been evicted", i)
		}
	}
	for i := 7; i < 10; i++ {
		if _, ok := c.Get(i); !ok {
			t.Errorf("key %d should have survived", i)
		}
	}
}

// TestLRUCache_ResizeGrow checks the grow path keeps every entry and lets the
// cache hold more before evicting.
func TestLRUCache_ResizeGrow(t *testing.T) {
	c, _ := cache.NewLRUCache[int, int](2)
	c.Put(1, 1)
	c.Put(2, 2)

	if evicted, err := c.Resize(4); err != nil || evicted != 0 {
		t.Fatalf("Resize(4) = (%d, %v), want (0, nil)", evicted, err)
	}
	c.Put(3, 3)
	c.Put(4, 4) // still within the grown capacity, nothing evicted
	if got := c.Len(); got != 4 {
		t.Fatalf("Len() = %d, want 4", got)
	}
	for i := 1; i <= 4; i++ {
		if _, ok := c.Get(i); !ok {
			t.Errorf("key %d should be present after growing", i)
		}
	}
}

// TestLRUCache_ResizeInvalid checks a non-positive capacity is rejected and the
// cache is left unchanged.
func TestLRUCache_ResizeInvalid(t *testing.T) {
	c, _ := cache.NewLRUCache[int, int](2)
	c.Put(1, 1)
	if _, err := c.Resize(0); err == nil {
		t.Error("Resize(0) = nil error, want ErrCacheCapacity")
	}
	if got := c.Capacity(); got != 2 {
		t.Errorf("Capacity() after rejected Resize = %d, want 2", got)
	}
}

func TestLRUCache_Concurrent(t *testing.T) {
	c, _ := cache.NewLRUCache[string, int](100)
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 1000; j++ {
				c.Put(fmt.Sprintf("k%d-%d", id, j), j)
				c.Get(fmt.Sprintf("k%d-%d", id, j))
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// No assertion — just confirms no race or panic under concurrency.
	// Run with: go test -race ./internal/cache/
}
