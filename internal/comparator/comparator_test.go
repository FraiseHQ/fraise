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

package comparator_test

import (
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/comparator"
)

func TestTimeComparator(t *testing.T) {
	now := time.Now()
	before := now.Add(-time.Hour)
	after := now.Add(time.Hour)

	cases := []struct {
		name string
		a, b time.Time
		want int
	}{
		{"equal", now, now, 0},
		{"before", before, now, -1},
		{"after", after, now, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := comparator.TimeComparator(c.a, c.b); got != c.want {
				t.Errorf("TimeComparator(%v, %v) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestOrderedComparatorInt(t *testing.T) {
	cases := []struct {
		name string
		a, b int
		want int
	}{
		{"equal", 5, 5, 0},
		{"less", 3, 5, -1},
		{"greater", 8, 5, 1},
		{"negative", -3, -1, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := comparator.OrderedComparator(c.a, c.b); got != c.want {
				t.Errorf("OrderedComparator(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestOrderedComparatorString(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "abc", "abc", 0},
		{"less", "abc", "abd", -1},
		{"greater", "abd", "abc", 1},
		{"prefix", "ab", "abc", -1},
		{"empty", "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := comparator.OrderedComparator(c.a, c.b); got != c.want {
				t.Errorf("OrderedComparator(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}
