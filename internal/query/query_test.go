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

// These tests exercise Parse, which turns a query string into the concrete
// *Remember/*Recall command it dispatches to. One known quirk is documented
// inline: RememberCommandNode.Entities()/Topics() are currently stubbed to
// return empty slices, so a parsed Remember never carries entities or topics.

// TestParseRemember checks that a remember query is dispatched to a
// *query.Remember carrying the phrase value and the graph selector,
// and reporting as a write.
func TestParseRemember(t *testing.T) {
	q := "remember@1 'anne loves the color orange' topic:color topic:preference entity:anne"

	got, err := query.Parse[string, float32](q)
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
	}

	r, ok := got.(*query.Remember[string, float32])
	if !ok {
		t.Fatalf("Parse(%q) returned %T, want *query.Remember", q, got)
	}

	// The phrase value is the raw text: the quotes are query syntax, not data.
	if want := "anne loves the color orange"; r.Value != want {
		t.Errorf("Value = %q, want %q", r.Value, want)
	}
	// Remember entities/topics are stubbed empty (see file-level note).
	if len(r.Entities) != 0 {
		t.Errorf("Entities = %v, want empty", r.Entities)
	}
	if len(r.Topics) != 0 {
		t.Errorf("Topics = %v, want empty", r.Topics)
	}
	if id := r.GetGraphID(); id != 1 {
		t.Errorf("GetGraphID() = %d, want 1", id)
	}
	if !r.IsWrite() {
		t.Error("Remember.IsWrite() = false, want true")
	}
}

// TestParseRecall checks that a recall query is dispatched to a *query.Recall
// carrying its terms, entities, topics and graph selector, and reporting as a
// read.
func TestParseRecall(t *testing.T) {
	q := "recall@2 anna bob entity:alice topic:job"

	got, err := query.Parse[string, float32](q)
	if err != nil {
		t.Fatalf("Parse(%q) returned unexpected error: %v", q, err)
	}

	r, ok := got.(*query.Recall[string, float32])
	if !ok {
		t.Fatalf("Parse(%q) returned %T, want *query.Recall", q, got)
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
	if id := r.GetGraphID(); id != 2 {
		t.Errorf("GetGraphID() = %d, want 2", id)
	}
	if r.IsWrite() {
		t.Error("Recall.IsWrite() = true, want false")
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
