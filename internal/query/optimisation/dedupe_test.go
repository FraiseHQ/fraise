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

// External tests: dedupeStrings is unexported, so it is exercised through
// Dedupe.Optimise.
package optimisation_test

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/query"
	"github.com/FraiseHQ/fraise/internal/query/optimisation"
)

func TestDedupeOptimiseRecall(t *testing.T) {
	in := &query.Recall[string, float32]{
		Keywords: []string{"a", "b", "a", "c", "b"},
		Entities: []string{"e1", "e1", "e2"},
		Topics:   []string{"t1", "t2", "t1"},
	}
	d := &optimisation.Dedupe[string, float32]{}

	out := d.Optimise(in)

	recall, ok := out.(*query.Recall[string, float32])
	if !ok {
		t.Fatalf("Optimise returned %T, want *query.Recall", out)
	}
	// Dedupe returns a copy and leaves the recall as parsed. The engine caches
	// the parsed recall beside the copy and confirms every later hit against
	// it, so a rewrite in place would make "a a" match only "a".
	if recall == in {
		t.Errorf("Optimise returned its input; want a deduplicated copy")
	}
	if want := []string{"a", "b", "a", "c", "b"}; !reflect.DeepEqual(in.Keywords, want) {
		t.Errorf("input Keywords = %v, want %v (untouched)", in.Keywords, want)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(recall.Keywords, want) {
		t.Errorf("Keywords = %v, want %v", recall.Keywords, want)
	}
	if want := []string{"e1", "e2"}; !reflect.DeepEqual(recall.Entities, want) {
		t.Errorf("Entities = %v, want %v", recall.Entities, want)
	}
	if want := []string{"t1", "t2"}; !reflect.DeepEqual(recall.Topics, want) {
		t.Errorf("Topics = %v, want %v", recall.Topics, want)
	}
}

func TestDedupeOptimiseRecallNoDuplicates(t *testing.T) {
	in := &query.Recall[string, float32]{
		Keywords: []string{"a", "b", "c"},
		Entities: []string{"e1"},
		Topics:   nil,
	}
	d := &optimisation.Dedupe[string, float32]{}

	recall := d.Optimise(in).(*query.Recall[string, float32])

	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(recall.Keywords, want) {
		t.Errorf("Keywords = %v, want %v (unchanged)", recall.Keywords, want)
	}
	if want := []string{"e1"}; !reflect.DeepEqual(recall.Entities, want) {
		t.Errorf("Entities = %v, want %v (unchanged)", recall.Entities, want)
	}
	if recall.Topics != nil {
		t.Errorf("Topics = %v, want nil (unchanged)", recall.Topics)
	}
}

// A when searches through its recall, so its lists are deduplicated as a
// recall's are. It comes back as a copy reading the same clock, and the when
// as parsed is left alone: the engine confirms every later cache hit against
// it.
func TestDedupeOptimiseWhen(t *testing.T) {
	type lists struct {
		Keywords, Entities, Topics []string
	}
	in := &query.When[string, float32]{
		Clock: containers.Said,
		Recall: query.Recall[string, float32]{
			Keywords: []string{"a", "b", "a", "c", "b"},
			Entities: []string{"e1", "e1", "e2"},
			Topics:   []string{"t1", "t2", "t1"},
		},
	}
	d := &optimisation.Dedupe[string, float32]{}

	out := d.Optimise(in)

	when, ok := out.(*query.When[string, float32])
	if !ok {
		t.Fatalf("Optimise returned %T, want *query.When", out)
	}
	if when == in {
		t.Errorf("Optimise returned its input; want a deduplicated copy")
	}
	if when.Clock != containers.Said {
		t.Errorf("Clock = %v, want %v (unchanged)", when.Clock, containers.Said)
	}
	want := lists{
		Keywords: []string{"a", "b", "c"},
		Entities: []string{"e1", "e2"},
		Topics:   []string{"t1", "t2"},
	}
	got := lists{Keywords: when.Recall.Keywords, Entities: when.Recall.Entities, Topics: when.Recall.Topics}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Optimise(when) mismatch (-want +got):\n%s", diff)
	}
	parsed := lists{
		Keywords: []string{"a", "b", "a", "c", "b"},
		Entities: []string{"e1", "e1", "e2"},
		Topics:   []string{"t1", "t2", "t1"},
	}
	kept := lists{Keywords: in.Recall.Keywords, Entities: in.Recall.Entities, Topics: in.Recall.Topics}
	if diff := cmp.Diff(parsed, kept); diff != "" {
		t.Errorf("Optimise rewrote the when as parsed (-want +got):\n%s", diff)
	}
}

// A query nested in a time bound runs a search of its own, so Dedupe reaches
// it, in either bound. The recall it returns holds a deduplicated copy of the
// nested query and shares no query with the recall as parsed. A shared one would be rewritten under the cache
// entry that confirms hits against it, and selecting a graph for the planned
// query would move the parsed one with it.
func TestDedupeOptimiseReachesNestedQueries(t *testing.T) {
	nested := func() *query.When[string, float32] {
		return &query.When[string, float32]{
			Clock:  containers.Said,
			Recall: query.Recall[string, float32]{Keywords: []string{"race", "race", "lap"}},
		}
	}
	tests := map[string]struct {
		hang func(r *query.Recall[string, float32], q query.Query[string, float32])
		read func(r *query.Recall[string, float32]) query.Query[string, float32]
	}{
		"since": {
			hang: func(r *query.Recall[string, float32], q query.Query[string, float32]) { r.Parameters.Since.Query = q },
			read: func(r *query.Recall[string, float32]) query.Query[string, float32] { return r.Parameters.Since.Query },
		},
		"until": {
			hang: func(r *query.Recall[string, float32], q query.Query[string, float32]) { r.Parameters.Until.Query = q },
			read: func(r *query.Recall[string, float32]) query.Query[string, float32] { return r.Parameters.Until.Query },
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			parsed := nested()
			in := &query.Recall[string, float32]{Keywords: []string{"x", "x"}}
			tc.hang(in, parsed)
			d := &optimisation.Dedupe[string, float32]{}

			out := d.Optimise(in).(*query.Recall[string, float32])

			when, ok := tc.read(out).(*query.When[string, float32])
			if !ok {
				t.Fatalf("Optimise left a %T in the bound, want *query.When", tc.read(out))
			}
			if when == parsed {
				t.Errorf("Optimise kept the parsed query in the bound; want a deduplicated copy")
			}
			if diff := cmp.Diff([]string{"race", "lap"}, when.Recall.Keywords); diff != "" {
				t.Errorf("nested Keywords mismatch (-want +got):\n%s", diff)
			}
			if when.Clock != containers.Said {
				t.Errorf("nested Clock = %v, want %v (unchanged)", when.Clock, containers.Said)
			}
			if diff := cmp.Diff([]string{"race", "race", "lap"}, parsed.Recall.Keywords); diff != "" {
				t.Errorf("Optimise rewrote the nested query as parsed (-want +got):\n%s", diff)
			}
			if tc.read(in) != query.Query[string, float32](parsed) {
				t.Errorf("Optimise replaced the query in the bound of the recall as parsed")
			}

			out.SetGraphID(3)

			if got := parsed.GetGraphID(); got != 0 {
				t.Errorf("selecting graph 3 for the optimised recall moved the parsed nested query to graph %d", got)
			}
			if got := when.GetGraphID(); got != 3 {
				t.Errorf("nested GetGraphID() = %d after SetGraphID(3) on its recall, want 3", got)
			}
		})
	}
}

// NewPipeline's Dedupe stage reaches a when as it does a recall, so the
// default pipeline deduplicates a when statement end to end.
func TestPipelineDeduplicatesWhen(t *testing.T) {
	p := optimisation.NewPipeline[string, float32]()
	in := &query.When[string, float32]{Recall: query.Recall[string, float32]{Keywords: []string{"a", "a", "b"}}}

	when := p.Optimise(in).(*query.When[string, float32])

	if diff := cmp.Diff([]string{"a", "b"}, when.Recall.Keywords); diff != "" {
		t.Errorf("Keywords mismatch (-want +got):\n%s", diff)
	}
}

// A query that is neither a *Recall nor a *When must be returned untouched.
func TestDedupeOptimisePassthrough(t *testing.T) {
	in := &query.Remember[string, float32]{Value: "hello"}
	d := &optimisation.Dedupe[string, float32]{}

	out := d.Optimise(in)

	got, ok := out.(*query.Remember[string, float32])
	if !ok {
		t.Fatalf("Optimise returned %T, want *query.Remember", out)
	}
	if got != in {
		t.Errorf("Optimise replaced a non-Recall query; want the same value returned")
	}
}

// NewPipeline wires in a Dedupe stage, so the default pipeline deduplicates
// Recall queries end to end.
func TestPipelineDeduplicatesRecall(t *testing.T) {
	p := optimisation.NewPipeline[string, float32]()
	in := &query.Recall[string, float32]{Keywords: []string{"a", "a", "b"}}

	recall := p.Optimise(in).(*query.Recall[string, float32])

	if want := []string{"a", "b"}; !reflect.DeepEqual(recall.Keywords, want) {
		t.Errorf("Keywords = %v, want %v", recall.Keywords, want)
	}
}
