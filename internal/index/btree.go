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

package index

import (
	"slices"
	"sort"
	"strings"

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/containers/trees"
	"github.com/FraiseHQ/fraise/internal/index/nlp"
	"github.com/FraiseHQ/fraise/internal/index/relevance"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// BTreeIndex is a full-text index backed by an ordered BTree from the
// containers/trees package. The tree is the term dictionary: it holds one
// posting list per indexed term (document key -> term frequency), ordered by
// term, and every term lookup goes through it. The installed Relevance model
// owns every scoring number and whatever corpus statistics it needs; the
// index owns tokenization, postings and the total-order ranking. It
// implements TextIndex.
//
// A posting list is a slice of (key, tf) pairs sorted by key, not a map: the
// search loop only reads it, and a contiguous slice streams through the
// cache where a map hops. Removing a document tombstones its entries in place
// (the tf drops to zero) rather than shifting every list it appears in. The
// tombstones are garbage until Flush compacts them, as in the vector forest,
// and the index compacts itself once it holds more dead documents than live
// ones.
type BTreeIndex[K comparable, P float32 | float64] struct {
	tree      *trees.BTree[K, *postingList[K], P] // term dictionary, ordered by term (P is unused)
	documents map[K]string                        // key -> raw document text
	dead      int                                 // documents removed since the last compaction
	tokenizer nlp.Tokenizer
	relevance relevance.Relevance[K, P]
	compare   comparator.Comparator[K] // document key ordering
}

// posting is one document's sighting of a term: its key and how often the
// term occurs in it. A tf of zero is a tombstone — a document that no longer
// holds the term, kept in place until the next compaction so that removing
// it costs a lookup, not a shift of every entry after it.
type posting[K comparable] struct {
	key K
	tf  int
}

// postingList is one term's postings, sorted by key, with the count of live
// entries kept beside them: the relevance model weighs a term by its document
// frequency, which is the live count, not the slice length once tombstones
// accumulate. term is the list's place in the dictionary: the tree orders
// lists by it, and a lookup probes the tree with a list carrying only the term.
type postingList[K comparable] struct {
	term    string
	entries []posting[K]
	live    int
}

// NewBTreeIndex returns an empty BTreeIndex whose term dictionary is an ordered
// BTree of posting lists, sorted lexicographically by term. compare orders
// document keys, the
// tiebreak Search ranks equally-scoring documents by. The index starts with
// the SimpleTokenizer and the MatchCount relevance model, until SetTokenizer
// and SetRelevance replace them.
func NewBTreeIndex[K comparable, P float32 | float64](compare comparator.Comparator[K]) *BTreeIndex[K, P] {
	return &BTreeIndex[K, P]{
		tree: trees.NewBTree[K, *postingList[K], P](32, func(a, b *postingList[K]) int {
			return comparator.OrderedComparator(a.term, b.term)
		}),
		documents: make(map[K]string),
		tokenizer: nlp.SimpleTokenizer{},
		relevance: relevance.MatchCount[K, P]{},
		compare:   compare,
	}
}

// SetTokenizer installs the tokenizer, mirroring SetRelevance. It must be
// called before the first Insert and never after: postings tokenized under
// the old scheme would be unreachable under the new one. nil is ignored
// rather than stored: an index without a tokenizer cannot index.
func (idx *BTreeIndex[K, P]) SetTokenizer(t nlp.Tokenizer) {
	if t == nil {
		return
	}
	idx.tokenizer = t
}

// SetRelevance installs the relevance model. It must be called before the
// first Insert and never after: the model keeps its own corpus statistics
// through the lifecycle hooks, so one installed mid-corpus would score against
// statistics missing every document already indexed. nil is ignored rather
// than stored: an index without a relevance model cannot rank.
func (idx *BTreeIndex[K, P]) SetRelevance(r relevance.Relevance[K, P]) {
	if r == nil {
		return
	}
	idx.relevance = r
}

// Insert tokenizes value, adds key to the posting list of each of its terms
// and stores the raw document for Retrieve. If key is already indexed, its
// previous document is replaced.
func (idx *BTreeIndex[K, P]) Insert(key K, value string) error {
	return idx.index(key, value)
}

// Retrieve returns the raw document stored under key.
func (idx *BTreeIndex[K, P]) Retrieve(key K) (string, error) {
	value, ok := idx.documents[key]
	if !ok {
		return "", ErrIndexNotFound
	}
	return value, nil
}

// Update replaces the document stored under key, re-indexing its terms. It
// returns ErrIndexNotFound if key is not indexed.
func (idx *BTreeIndex[K, P]) Update(key K, value string) error {
	if _, err := idx.Retrieve(key); err != nil {
		return err
	}
	return idx.index(key, value)
}

// Delete tombstones key in every posting list it appears in and drops the
// stored document. The tombstones are garbage until the next Flush; the
// automatic compaction reclaims them once the dead outnumber the live.
func (idx *BTreeIndex[K, P]) Delete(key K) error {
	value, err := idx.Retrieve(key)
	if err != nil {
		return err
	}
	idx.removePostings(key, value)
	delete(idx.documents, key)
	return idx.maybeFlush()
}

// Search tokenizes query and returns the matching document keys ranked by
// the installed Relevance model, best first, with the scores in a parallel
// slice. The index runs the loop over terms and postings; the model supplies
// every number: how the query tokens become terms, what a term is worth, what
// a document gains per match, and how match breadth folds into the final
// relevance.
//
// Documents of equal score are ordered by key, the total order SearchIndex
// promises. k bounds the number of results; k <= 0 returns every match.
func (idx *BTreeIndex[K, P]) Search(query string, k int) ([]K, []P, error) {
	if len(idx.documents) == 0 {
		return nil, nil, ErrEmptyIndex
	}

	terms := idx.relevance.Terms(idx.tokenizer.Tokenize(query))

	// scores and matched are presized to the query's largest live posting
	// list, a lower bound on how many documents the query matches.
	var maxPosting int
	probe := &postingList[K]{}
	for _, term := range terms {
		probe.term = term
		if list, ok := idx.tree.Find(probe); ok && list.live > maxPosting {
			maxPosting = list.live
		}
	}

	scores := make(map[K]P, maxPosting)
	matched := make(map[K]P, maxPosting)
	prepared := idx.relevance.Prepare()

	var totalW P
	for _, term := range terms {
		probe.term = term
		list, ok := idx.tree.Find(probe)
		if !ok || list.live == 0 {
			continue
		}
		weight := idx.relevance.Weight(list.live, len(idx.documents))
		totalW += weight
		for _, p := range list.entries {
			if p.tf == 0 {
				continue
			}
			scores[p.key] += idx.relevance.Increment(weight, p.key, p.tf, prepared)
			matched[p.key] += weight
		}
	}
	for key := range scores {
		scores[key] = idx.relevance.Finalize(scores[key], int(matched[key]*1024), int(totalW*1024)+1)
	}

	top := containers.NewTopK[K, P](k, idx.compare)

	for key, score := range scores {
		top.Offer(key, score)
	}
	keys, ranked := top.Drain()

	out := make([]P, len(ranked))
	for i, score := range ranked {
		out[i] = P(score)
	}
	logger.Debug("Text search matched documents", "matches", len(keys), "k", k)
	return keys, out, nil
}

// Expand returns up to n spellings to add to query from the documents under
// keys — the feedback set of a pseudo-relevance round — rarest first: by
// document frequency ascending, then by spelling, so the choice is a total
// order and two identical rounds expand identically. Rarity is read off the
// postings rather than the installed Relevance model's term weight: under
// BM25 the two orders agree, since idf falls as df rises, but a model that
// prices every term alike (MatchCount) would tie them all and hand the
// budget to the alphabetically first words. A spelling is a word as it appears in the document
// rather than the stem the postings hold, because the expanded query goes
// back through Search and so through the tokenizer: re-stemming a stem is not
// guaranteed to land on itself, and a term returned as its stem could then
// miss the very posting it was chosen from.
//
// Two kinds of term are left out. One already in the query adds no reach
// and the query carries it anyway. One whose posting holds no document
// outside the feedback set can surface no new candidate — the rarest terms
// of all are exactly the ones confined to a single document, so without
// this rule the budget would go to spellings that only re-find the seeds. A
// key with no document is skipped, not an error: the feedback set is whatever
// the first pass ranked, and a document retired between the passes is not
// the caller's fault.
func (idx *BTreeIndex[K, P]) Expand(query string, keys []K, n int) []string {
	if n <= 0 || len(keys) == 0 {
		return nil
	}

	asked := make(map[string]struct{})
	for _, term := range idx.tokenizer.Tokenize(query) {
		asked[term] = struct{}{}
	}
	feedback := make(map[K]struct{}, len(keys))
	for _, key := range keys {
		feedback[key] = struct{}{}
	}

	type candidate struct {
		spelling string
		df       int
	}
	candidates := make([]candidate, 0)
	seen := make(map[string]struct{})
	probe := &postingList[K]{}
	for _, key := range keys {
		document, ok := idx.documents[key]
		if !ok {
			continue
		}
		for _, word := range nlp.Words(document) {
			// A word is one run of term characters, so it tokenizes to a
			// single term — the same one the document's indexing produced
			// for it, since tokenization is per word.
			terms := idx.tokenizer.Tokenize(word)
			if len(terms) != 1 {
				continue
			}
			term := terms[0]
			if _, dup := seen[term]; dup {
				continue
			}
			seen[term] = struct{}{}
			if _, inQuery := asked[term]; inQuery {
				continue
			}
			probe.term = term
			list, ok := idx.tree.Find(probe)
			if !ok || list.live == 0 {
				continue
			}
			reaches := false
			for _, p := range list.entries {
				if p.tf == 0 {
					continue
				}
				if _, confined := feedback[p.key]; !confined {
					reaches = true
					break
				}
			}
			if !reaches {
				continue
			}
			candidates = append(candidates, candidate{
				spelling: strings.ToLower(word),
				df:       list.live,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].df != candidates[j].df {
			return candidates[i].df < candidates[j].df
		}
		return candidates[i].spelling < candidates[j].spelling
	})
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.spelling
	}
	logger.Debug("Text index expanded a query", "feedback", len(keys), "terms", len(out))
	return out
}

// Count reports the number of indexed documents.
func (idx *BTreeIndex[K, P]) Count() int {
	return len(idx.documents)
}

// Entries reports the documents the postings physically hold: the live ones
// plus those removed or replaced since the last compaction, whose tombstones
// still sit in the lists. The automatic Flush keeps it at most twice Count.
func (idx *BTreeIndex[K, P]) Entries() int {
	return idx.Count() + idx.dead
}

// Flush compacts every posting list, dropping the tombstones left by deletes
// and updates, and retires from the dictionary any term no live document
// holds any more.
func (idx *BTreeIndex[K, P]) Flush() error {
	for _, list := range idx.tree.Values() {
		if list.live == 0 {
			idx.tree.Delete(list)
			continue
		}
		list.entries = slices.DeleteFunc(list.entries, func(p posting[K]) bool { return p.tf == 0 })
	}
	idx.dead = 0
	return nil
}

// maybeFlush compacts the postings once the index holds more dead documents
// than live ones. Called after every mutation, it keeps the lists O(live
// documents) with amortised-constant compaction cost.
func (idx *BTreeIndex[K, P]) maybeFlush() error {
	if idx.dead <= len(idx.documents) {
		return nil
	}
	logger.Debug("Text index compaction", "live", len(idx.documents), "dead", idx.dead)
	return idx.Flush()
}

// index tokenizes value, tombstones any postings left over from a previous
// document stored under key (retiring it from the relevance model's
// statistics), then records the new term frequencies, document text, and the
// document's statistics with the model. A term the new text shares with the
// old one revives its tombstone in place.
func (idx *BTreeIndex[K, P]) index(key K, value string) error {
	if old, err := idx.Retrieve(key); err == nil {
		idx.removePostings(key, old)
	}

	tokens := idx.tokenizer.Tokenize(value)
	probe := &postingList[K]{}
	for _, term := range tokens {
		probe.term = term
		list, ok := idx.tree.Find(probe)
		if !ok {
			list = &postingList[K]{term: term}
			if err := idx.tree.Insert(list); err != nil {
				return err
			}
		}
		i, found := list.find(key, idx.compare)
		switch {
		case !found:
			list.entries = slices.Insert(list.entries, i, posting[K]{key: key, tf: 1})
			list.live++
		case list.entries[i].tf == 0:
			list.entries[i].tf = 1
			list.live++
		default:
			list.entries[i].tf++
		}
	}

	idx.documents[key] = value
	idx.relevance.Indexed(key, tokens)
	return idx.maybeFlush()
}

// removePostings tombstones key in the posting list of every term in value
// and retires the document from the relevance model's statistics. Both
// callers, Delete and a re-index under the same key, retire through here, so
// the model sees exactly one Removed per Indexed, and each leaves one dead
// document's worth of tombstones for Flush to reclaim.
func (idx *BTreeIndex[K, P]) removePostings(key K, value string) {
	tokens := idx.tokenizer.Tokenize(value)
	probe := &postingList[K]{}
	for _, term := range tokens {
		probe.term = term
		list, ok := idx.tree.Find(probe)
		if !ok {
			continue
		}
		if i, found := list.find(key, idx.compare); found && list.entries[i].tf != 0 {
			list.entries[i].tf = 0
			list.live--
		}
	}
	idx.dead++
	idx.relevance.Removed(key, tokens)
}

// find locates key in the sorted entries: the index it sits at and whether it
// is there at all. When it is not, the index is where it belongs, so an
// insert there keeps the list sorted.
func (l *postingList[K]) find(key K, compare comparator.Comparator[K]) (int, bool) {
	i := sort.Search(len(l.entries), func(i int) bool { return compare(l.entries[i].key, key) >= 0 })
	return i, i < len(l.entries) && l.entries[i].key == key
}
