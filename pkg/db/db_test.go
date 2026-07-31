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

package db_test

import (
	"testing"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/pkg/db"
)

// TestNewDBAllocatesGraphs checks that a new store allocates the default number
// of graph slots.
func TestNewDBAllocatesGraphs(t *testing.T) {
	d, err := db.NewDB[uint64, float32](config.New())
	if err != nil {
		t.Fatalf("NewDB returned error: %v", err)
	}
	if got := d.NumGraphs(); got != int(config.DefaultNumGraph) {
		t.Errorf("NumGraphs() = %d, want %d", got, config.DefaultNumGraph)
	}
}

// TestStartPopulatesGraphs checks that Start replaces the empty slots with real
// graph instances that Select can hand back.
func TestStartPopulatesGraphs(t *testing.T) {
	d, err := db.NewDB[uint64, float32](config.New())
	if err != nil {
		t.Fatalf("NewDB returned error: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	for i := 0; i < d.NumGraphs(); i++ {
		g, err := d.Select(uint8(i))
		if err != nil {
			t.Fatalf("Select(%d) returned error: %v", i, err)
		}
		if g == nil {
			t.Errorf("Select(%d) returned nil graph after Start", i)
		}
	}
}

// TestSelectOutOfRange checks that selecting a graph past the allocated range is
// rejected with an error and a nil graph.
func TestSelectOutOfRange(t *testing.T) {
	d, err := db.NewDB[uint64, float32](config.New())
	if err != nil {
		t.Fatalf("NewDB returned error: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	g, err := d.Select(uint8(d.NumGraphs()))
	if err == nil {
		t.Error("Select past the range returned nil error, want out-of-bounds error")
	}
	if g != nil {
		t.Errorf("Select past the range returned %v, want nil graph", g)
	}
}

// TestStopReinitialises checks that Stop keeps the store usable by leaving the
// default number of (empty) graph slots in place.
func TestStopReinitialises(t *testing.T) {
	d, err := db.NewDB[uint64, float32](config.New())
	if err != nil {
		t.Fatalf("NewDB returned error: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if got := d.NumGraphs(); got != int(config.DefaultNumGraph) {
		t.Errorf("NumGraphs() after Stop = %d, want %d", got, config.DefaultNumGraph)
	}
}
