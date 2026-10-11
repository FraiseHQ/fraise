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

	// gamma and neighbours are the window a search scores with (see
	// SetWindow): a document borrows gamma of each neighbour's term counts
	// and length. gamma 0 or a nil neighbours is the plain search.
	gamma      P
	neighbours func(K) []K
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

// SetWindow installs the window a search scores with. A document then
// borrows gamma of each of its neighbours' term counts and length, so a
// question answered across two adjacent facts, or a reply whose question is
// the fact before it, is scored as the exchange it is rather than as two
// fragments. neighbours reports the documents adjacent to a key; the index
// does not know what adjacency means, only the graph does (the facts written
// just before and after under the same anchor), so the graph installs it.
// gamma is a share in [0, 1]: 0, or a nil neighbours, is the plain search.
func (idx *BTreeIndex[K, P]) SetWindow(gamma P, neighbours func(K) []K) {
	idx.gamma = gamma
	idx.neighbours = neighbours
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
// With a window installed (SetWindow), a document's term frequency and
// length for each term are its own plus gamma times its neighbours', and a
// document that holds none of a term but sits beside one that does is a
// match too, so the candidates are the matching documents and their
// neighbours. Coverage counts a borrowed term as matched: the exchange
// covers the query, not the fragment.
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

	// A window search keeps its documents in its own slots, so only the
	// plain search needs the maps.
	var scores, matched map[K]P
	var w *window[K, P]
	if idx.gamma > 0 && idx.neighbours != nil {
		w = &window[K, P]{index: idx, at: make(map[K]int, maxPosting), docs: make([]windowDoc[K, P], 0, maxPosting), lent: make([]int, 0, 2*maxPosting), touched: make([]int, 0, maxPosting)}
	} else {
		scores = make(map[K]P, maxPosting)
		matched = make(map[K]P, maxPosting)
	}
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
		if w != nil {
			w.accumulate(list, weight, prepared)
			continue
		}
		for _, p := range list.entries {
			if p.tf == 0 {
				continue
			}
			scores[p.key] += idx.relevance.Increment(weight, p.key, p.tf, prepared)
			matched[p.key] += weight
		}
	}
	top := containers.NewTopK[K, P](k, idx.compare)
	if w != nil {
		w.offer(top, totalW)
	}
	for key, score := range scores {
		top.Offer(key, idx.relevance.Finalize(score, int(matched[key]*1024), int(totalW*1024)+1))
	}
	keys, ranked := top.Drain()

	out := make([]P, len(ranked))
	for i, score := range ranked {
		out[i] = P(score)
	}
	logger.Debug("Text search matched documents", "matches", len(keys), "k", k)
	return keys, out, nil
}

// window is one search's view of the installed window. Every document it
// meets, a posting's or a neighbour it lends to, gets one slot holding all
// the query needs of it: its neighbours' slots and window length, resolved
// once since both are fixed for the query, the frequency the current term
// lends it, and its running score and matched mass. A term then costs one
// map lookup per posting and per neighbour it lends to, and the documents
// are finalized and ranked straight from their slots when the query is done.
type window[K comparable, P float32 | float64] struct {
	index *BTreeIndex[K, P]
	at    map[K]int
	docs  []windowDoc[K, P]

	// lent holds every slot's neighbour slots end to end, each slot's run at
	// lend in windowDoc, so resolving a document allocates nothing of its own.
	lent []int

	// term numbers the posting lists accumulate has read, and touched the
	// slots the current one reached.
	term    int
	touched []int
}

// windowDoc is one document's slot in a window search. tf is the current
// term's frequency, valid while seen is the current term; its neighbours'
// slots (lent[from:to]) and its length are resolved on first need
// (resolved, measured).
type windowDoc[K comparable, P float32 | float64] struct {
	key      K
	from, to int
	length   P
	tf       P
	score    P
	matched  P
	seen     int
	resolved bool
	measured bool
}

// slot returns key's slot, opening one the first time the search meets it.
func (w *window[K, P]) slot(key K) int {
	if i, ok := w.at[key]; ok {
		return i
	}
	i := len(w.docs)
	w.docs = append(w.docs, windowDoc[K, P]{key: key})
	w.at[key] = i
	return i
}

// adjacent returns the slots of slot i's neighbours, asking the graph the
// first time.
func (w *window[K, P]) adjacent(i int) []int {
	if !w.docs[i].resolved {
		from := len(w.lent)
		for _, n := range w.index.neighbours(w.docs[i].key) {
			w.lent = append(w.lent, w.slot(n))
		}
		w.docs[i].from, w.docs[i].to, w.docs[i].resolved = from, len(w.lent), true
	}
	return w.lent[w.docs[i].from:w.docs[i].to]
}

// length returns slot i's window length: its own plus gamma of each
// neighbour's.
func (w *window[K, P]) length(i int) P {
	if !w.docs[i].measured {
		l := w.index.relevance.Length(w.docs[i].key)
		for _, n := range w.adjacent(i) {
			l += w.index.gamma * w.index.relevance.Length(w.docs[n].key)
		}
		w.docs[i].length, w.docs[i].measured = l, true
	}
	return w.docs[i].length
}

// lendTo adds tf to slot i's frequency for the current term.
func (w *window[K, P]) lendTo(i int, tf P) {
	d := &w.docs[i]
	if d.seen != w.term {
		d.seen, d.tf = w.term, 0
		w.touched = append(w.touched, i)
	}
	d.tf += tf
}

// accumulate scores one term's posting list under the window. The term's
// frequency per document is its own count plus gamma of each neighbour's: a
// posting lends to its neighbours, so a document is accumulated from up to
// three postings before it is scored, which is why the saturation runs after
// the list rather than per posting.
func (w *window[K, P]) accumulate(list *postingList[K], weight P, prepared P) {
	idx := w.index
	w.term++
	w.touched = w.touched[:0]
	for _, p := range list.entries {
		if p.tf == 0 {
			continue
		}
		i := w.slot(p.key)
		w.lendTo(i, P(p.tf))
		for _, n := range w.adjacent(i) {
			w.lendTo(n, idx.gamma*P(p.tf))
		}
	}
	for _, i := range w.touched {
		// length may open slots, growing docs, so it runs before d is taken.
		length := w.length(i)
		d := &w.docs[i]
		d.score += idx.relevance.Gain(weight, d.tf, length, prepared)
		d.matched += weight
	}
}

// offer finalizes every document a term reached into top, as the plain
// search does from its maps; totalW is the query's whole idf mass.
func (w *window[K, P]) offer(top *containers.TopK[K, P], totalW P) {
	for i := range w.docs {
		if d := &w.docs[i]; d.seen > 0 {
			top.Offer(d.key, w.index.relevance.Finalize(d.score, int(d.matched*1024), int(totalW*1024)+1))
		}
	}
}

// KeyTerms reports, for each of keys, the query term it holds that the
// relevance model weighs highest: the rarest of the query's terms the
// document matched, the one that most defines why it matched. A key holding
// none of the terms, or not indexed, is absent from the result. The window
// is not consulted: a document is keyed by what it holds itself.
//
// Search's grouping is built on this: candidates that matched the query by
// the same rare term are instances of one thing the query asked about.
func (idx *BTreeIndex[K, P]) KeyTerms(query string, keys []K) map[K]string {
	terms := idx.relevance.Terms(idx.tokenizer.Tokenize(query))
	type weighted struct {
		list   *postingList[K]
		weight P
	}
	lists := make([]weighted, 0, len(terms))
	probe := &postingList[K]{}
	for _, term := range terms {
		probe.term = term
		if list, ok := idx.tree.Find(probe); ok && list.live > 0 {
			lists = append(lists, weighted{list, idx.relevance.Weight(list.live, len(idx.documents))})
		}
	}
	// Rarest first; a tie keeps query order, so the result is deterministic.
	sort.SliceStable(lists, func(i, j int) bool { return lists[i].weight > lists[j].weight })

	out := make(map[K]string, len(keys))
	for _, key := range keys {
		for _, w := range lists {
			if i, found := w.list.find(key, idx.compare); found && w.list.entries[i].tf > 0 {
				out[key] = w.list.term
				break
			}
		}
	}
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
