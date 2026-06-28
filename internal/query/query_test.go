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

package query_test

import (
	"reflect"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/query"
)

// fmtdump prints values for debugging; kept here so tests stay self-contained.

// TestParseRemember checks that a well-formed remember query is decoded into a
// query.Remember carrying the value, entities, topics and graph selector, and
// that it reports itself as a write.
func TestParseRemember(t *testing.T) {
	q := "remember@1 'anne loves the color orange' topic:color topic:preference entity:anne"

	got, err := query.Parse[string, float32](q)
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
	}

	r, ok := got.(query.Remember[string, float32])
	if !ok {
		t.Fatalf("Parse(%q) returned %T, want query.Remember", q, got)
	}

	if r.Value != "anne loves the color orange" {
		t.Errorf("Value = %q, want %q", r.Value, "anne loves the color orange")
	}
	if want := []string{"anne"}; !reflect.DeepEqual(r.Entities, want) {
		t.Errorf("Entities = %v, want %v", r.Entities, want)
	}
	if want := []string{"color", "preference"}; !reflect.DeepEqual(r.Topics, want) {
		t.Errorf("Topics = %v, want %v", r.Topics, want)
	}
	if got := r.GetGraphID(); got != 1 {
		t.Errorf("GetGraphID() = %d, want 1", got)
	}
	if !r.IsWrite() {
		t.Error("Remember.IsWrite() = false, want true")
	}
}

// TestParseRecall checks that a well-formed recall query is decoded into a
// query.Recall carrying keywords, entities, topics, the top/depth parameters
// and the graph selector, and that it reports itself as a read.
func TestParseRecall(t *testing.T) {
	q := "recall@2 anna bob entity:alice topic:job top:10 depth:5"

	got, err := query.Parse[string, float32](q)
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
	}

	r, ok := got.(query.Recall[string, float32])
	if !ok {
		t.Fatalf("Parse(%q) returned %T, want query.Recall", q, got)
	}

	if want := []string{"anna", "bob"}; !reflect.DeepEqual(r.Keywords, want) {
		t.Errorf("Keywords = %v, want %v", r.Keywords, want)
	}
	if want := []string{"alice"}; !reflect.DeepEqual(r.Entities, want) {
		t.Errorf("Entities = %v, want %v", r.Entities, want)
	}
	if want := []string{"job"}; !reflect.DeepEqual(r.Topics, want) {
		t.Errorf("Topics = %v, want %v", r.Topics, want)
	}
	if r.Parameters.Top != 10 {
		t.Errorf("Parameters.Top = %d, want 10", r.Parameters.Top)
	}
	if r.Parameters.Depth != 5 {
		t.Errorf("Parameters.Depth = %d, want 5", r.Parameters.Depth)
	}
	if got := r.GetGraphID(); got != 2 {
		t.Errorf("GetGraphID() = %d, want 2", got)
	}
	if r.IsWrite() {
		t.Error("Recall.IsWrite() = true, want false")
	}
}

// TestParseRecallDefaultSelector checks that a query without an explicit @N
// selector defaults its graph ID to 0.
func TestParseRecallDefaultSelector(t *testing.T) {
	got, err := query.Parse[string, float32]("recall anna")
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	r, ok := got.(query.Recall[string, float32])
	if !ok {
		t.Fatalf("Parse returned %T, want query.Recall", got)
	}
	if id := r.GetGraphID(); id != 0 {
		t.Errorf("GetGraphID() = %d, want 0", id)
	}
}

// TestParseErrors checks that malformed or empty queries are rejected with
// ErrParsingFailed and a nil query.
func TestParseErrors(t *testing.T) {
	queries := []string{
		"",                 // empty input
		"recall",           // a recall must carry at least one term
		"recall topic:job", // fields may only follow a term
		"forget anna",      // unknown command keyword
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			got, err := query.Parse[string, float32](q)
			if err != query.ErrParsingFailed {
				t.Errorf("Parse(%q) err = %v, want ErrParsingFailed", q, err)
			}
			if got != nil {
				t.Errorf("Parse(%q) = %v, want nil query", q, got)
			}
		})
	}
}
