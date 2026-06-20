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

// Internal test (package optimisation) so dedupeStrings can be exercised
// directly in addition to Dedupe.Optimise.
package optimisation_test

import (
	"reflect"
	"testing"

	"github.com/RonsenbergVI/fraise/internal/query"
	"github.com/RonsenbergVI/fraise/internal/query/optimisation"
)

var _ optimisation.Optimisation[string, float32] = (*optimisation.Dedupe[string, float32])(nil)

// --- Dedupe.Optimise --------------------------------------------------------

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
	// Dedupe works in place and returns the same query value.
	if recall != in {
		t.Errorf("Optimise returned a different pointer; want the input deduped in place")
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

// A query that is not a *Recall must be returned untouched.
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

// --- Pipeline (integration) -------------------------------------------------

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
