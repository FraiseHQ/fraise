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
