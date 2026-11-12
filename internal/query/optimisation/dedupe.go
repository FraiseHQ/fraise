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

package optimisation

import "github.com/FraiseHQ/fraise/internal/query"

// Dedupe drops repeated keywords, entities and topics from a [query.Recall]. It
// keeps the first occurrence of each, in order, so the search sees each term
// and anchor once. A [query.When] searches through a recall, and a recall may
// hold a query in each of its time bounds. Dedupe reaches both.
type Dedupe[K comparable, P float32 | float64] struct{}

// Optimise implements [Optimisation]. It returns a copy of a recall with its
// lists deduplicated, and leaves the recall as parsed. A when comes back as a
// copy over its deduplicated recall. It returns any other query untouched.
func (d *Dedupe[K, P]) Optimise(q query.Query[K, P]) query.Query[K, P] {
	switch v := q.(type) {
	case *query.Recall[K, P]:
		out := d.recall(*v)
		return &out
	case *query.When[K, P]:
		out := *v
		out.Recall = d.recall(v.Recall)
		return &out
	default:
		return q
	}
}

// recall returns r with its lists deduplicated. r is the stage's own copy. A
// query nested in a time bound is replaced by its optimised copy, so the
// output shares no query with the recall as parsed, and selecting a graph for
// one leaves the other where it was.
func (d *Dedupe[K, P]) recall(r query.Recall[K, P]) query.Recall[K, P] {
	r.Keywords = dedupeStrings(r.Keywords)
	r.Entities = dedupeStrings(r.Entities)
	r.Topics = dedupeStrings(r.Topics)
	if nested := r.Parameters.Since.Query; nested != nil {
		r.Parameters.Since.Query = d.Optimise(nested)
	}
	if nested := r.Parameters.Until.Query; nested != nil {
		r.Parameters.Until.Query = d.Optimise(nested)
	}
	return r
}

func dedupeStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
