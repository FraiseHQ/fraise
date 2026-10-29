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
	"math"
	"slices"
	"sort"

	"github.com/FraiseHQ/fraise/internal/comparator"
	"github.com/FraiseHQ/fraise/internal/containers"
	"github.com/FraiseHQ/fraise/internal/containers/trees"
	"github.com/FraiseHQ/fraise/internal/index/nlp"
	"github.com/FraiseHQ/fraise/internal/index/relevance"
	"github.com/FraiseHQ/fraise/pkg/logger"
)

// BTreeIndex is a full-text index that implements TextIndex. Its term
// dictionary is an ordered BTree from the containers/trees package. The tree
// holds one posting list per indexed term, ordered by term, and every term
// lookup goes through it. Scoring belongs to the installed Relevance model.
//
// A posting list is a slice of (key, number, tf) entries sorted by key, not a
// map. The search loop only reads it, and a slice streams through the cache
// where a map hops. Removing a document leaves tombstones in its lists. They
// are garbage until Flush compacts them, as in the vector forest. The index
// compacts itself once it holds more dead documents than live ones.
type BTreeIndex[K comparable, P float32 | float64] struct {
	tree      *trees.BTree[K, *postingList[K], P] // term dictionary, ordered by term (P is unused)
	documents map[K]string                        // key -> raw document text
	dead      int                                 // documents removed or replaced since the last compaction
	tokenizer nlp.Tokenizer
	relevance relevance.Relevance[K, P]
	compare   comparator.Comparator[K] // document key ordering

	// gamma and neighbours are the window a search scores with (see
	// SetWindow): a document borrows gamma of each neighbour's term counts
	// and length. gamma 0 or a nil neighbours is the plain search.
	gamma      P
	neighbours func(K) []K

	// ids numbers the documents densely, in the order they were first
	// indexed, and keys maps a number back to its key. A search keeps its
	// per-document state in slices indexed by number, so a posting costs an
	// index rather than a hash. Flush renumbers the live documents, so the
	// numbers stay as dense as the documents.
	ids  map[K]int32
	keys []K
}

// posting is one document's sighting of a term: its key, its number (see
// BTreeIndex.ids) and how often the term occurs in it. A tf of zero is a
// tombstone for a document that no longer holds the term. It stays in place
// until the next compaction, so a removal costs a lookup, not a shift of every
// entry after it.
type posting[K comparable] struct {
	key K
	doc int32
	tf  int32
}

// postingList holds one term's postings, sorted by key. Deletes and updates
// leave tombstones in entries until Flush, so live counts the entries still
// in use. The relevance model weighs a term by that count, its document
// frequency. The tree orders lists by term, and a lookup probes it with a
// list that holds only the term.
type postingList[K comparable] struct {
	term    string
	entries []posting[K]
	live    int
}

// NewBTreeIndex returns an empty BTreeIndex. Its term dictionary sorts
// posting lists lexicographically by term. compare orders document keys, and
// Search uses it to break ties between equal scores. The index starts with
// the SimpleTokenizer and the MatchCount relevance model, until SetTokenizer
// and SetRelevance replace them.
func NewBTreeIndex[K comparable, P float32 | float64](compare comparator.Comparator[K]) *BTreeIndex[K, P] {
	return &BTreeIndex[K, P]{
		tree: trees.NewBTree[K, *postingList[K], P](32, func(a, b *postingList[K]) int {
			return comparator.OrderedComparator(a.term, b.term)
		}),
		documents: make(map[K]string),
		ids:       make(map[K]int32),
		tokenizer: nlp.SimpleTokenizer{},
		relevance: relevance.MatchCount[K, P]{},
		compare:   compare,
	}
}

// SetTokenizer installs the tokenizer. Call it before the first Insert and
// never after, as [nlp.Tokenizer] explains. A nil tokenizer is ignored,
// because an index without one cannot index.
func (idx *BTreeIndex[K, P]) SetTokenizer(t nlp.Tokenizer) {
	if t == nil {
		return
	}
	idx.tokenizer = t
}

// SetRelevance installs the relevance model. Call it before the first Insert
// and never after, as [relevance.Relevance] requires. A nil model is ignored,
// because an index without one cannot rank.
func (idx *BTreeIndex[K, P]) SetRelevance(r relevance.Relevance[K, P]) {
	if r == nil {
		return
	}
	idx.relevance = r
}

// Relevance returns the installed relevance model: MatchCount until
// SetRelevance replaces it.
func (idx *BTreeIndex[K, P]) Relevance() relevance.Relevance[K, P] {
	return idx.relevance
}

// SetWindow installs the window a search scores with. A document then
// borrows gamma of each of its neighbours' term counts and length. So a
// question answered across two adjacent facts, or a reply whose question is
// the fact before it, is scored as the exchange it is rather than as two
// fragments. neighbours reports the documents adjacent to a key. The index
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
// stored document. The tombstones stay until the next Flush.
func (idx *BTreeIndex[K, P]) Delete(key K) error {
	value, err := idx.Retrieve(key)
	if err != nil {
		return err
	}
	idx.removePostings(key, value)
	delete(idx.documents, key)
	return idx.maybeFlush()
}

// Search tokenizes query and ranks the matching documents with the installed
// Relevance model, once per keep: each reading holds the k best documents its
// keep accepts. The index runs the loop over terms and postings. The model
// supplies every number: how query tokens become terms, what a term is worth,
// what a document gains per match, and how match breadth folds into the final
// relevance.
//
// A keep narrows which documents compete, not what a term is worth: term
// weights still come from the whole index. The readings share one pass over
// the postings. Each keep is asked about a document once, the first time the
// pass meets it, and a document's window is resolved once for every reading.
// So a second reading costs a share of the first, not a second pass.
//
// With a window installed (SetWindow), a document's term frequency and length
// for each term are its own plus gamma times its neighbours'. A document that
// holds none of a term but sits beside one that does is a match too, so the
// candidates are the matching documents and their neighbours. Coverage
// counts a borrowed term as matched: the exchange covers the query, not the
// fragment. A document a keep rejects neither lends nor borrows in that keep's
// reading: its postings are skipped and it is no one's neighbour, so a
// filtered window search scores what a filtered plain search sees.
func (idx *BTreeIndex[K, P]) Search(query string, k int, keeps ...func(K) bool) ([][]K, [][]P, error) {
	if len(idx.documents) == 0 {
		return nil, nil, ErrEmptyIndex
	}
	if len(keeps) == 0 {
		keeps = []func(K) bool{nil}
	}

	terms := idx.relevance.Terms(idx.tokenizer.Tokenize(query))

	// Presize the slots to the query's largest live posting list. Its length
	// is a lower bound on how many documents the query matches.
	var maxPosting int
	probe := &postingList[K]{}
	for _, term := range terms {
		probe.term = term
		if list, ok := idx.tree.Find(probe); ok && list.live > maxPosting {
			maxPosting = list.live
		}
	}

	p := idx.newPass(keeps, maxPosting)
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
		p.accumulate(list, weight, prepared)
	}

	keys := make([][]K, len(p.readings))
	scores := make([][]P, len(p.readings))
	for i := range p.readings {
		keys[i], scores[i] = p.rank(&p.readings[i], k, totalW)
	}
	logger.Debug("Text search matched documents", "documents", len(p.slots), "readings", len(p.readings), "k", k)
	return keys, scores, nil
}

// pass is one search's walk over the query's posting lists, shared by its
// readings. Every document the walk meets gets one slot, found through its
// number rather than by hashing its key. A slot holds what the readings
// share: the key and, under a window, the slots of the document's neighbours
// and its own length, each resolved once for the query. A term then costs one
// slice index per posting and per neighbour it lends to, and the documents are
// finalized and ranked straight from their slots when the query is done.
type pass[K comparable, P float32 | float64] struct {
	index *BTreeIndex[K, P]

	// at maps a document number to its slot plus one, and holds 0 while the
	// document has no slot. A neighbour the index does not hold has no
	// number, so unnumbered maps its key to its slot instead.
	at         []int32
	unnumbered map[K]int32
	slots      []passSlot[K, P]

	// lent holds every slot's neighbour slots end to end, each slot's run at
	// from:to in its passSlot, so resolving a document allocates nothing of
	// its own.
	lent []int32

	// term numbers the posting lists accumulate has read.
	term int32

	readings []reading[K, P]
}

// passSlot is what the readings share about one document: its key, the run of
// its neighbour slots in lent (valid once resolved) and its own length
// (valid once measured).
type passSlot[K comparable, P float32 | float64] struct {
	key      K
	own      P
	from, to int32
	resolved bool
	measured bool
}

// reading is one keep's share of a pass: its verdict on each slot, and the
// frequency, length, score and matched mass it accumulates for the slots it
// accepts. Its docs run parallel to the pass's slots.
type reading[K comparable, P float32 | float64] struct {
	keep    func(K) bool
	docs    []readingDoc[P]
	touched []int32 // slots the current term reached
}

// readingDoc is one slot's state in one reading. verdict is the keep's answer,
// 0 until it is asked. tf is the current term's frequency, valid while seen is
// the current term. length is the window length, valid once measured.
type readingDoc[P float32 | float64] struct {
	verdict  int8
	measured bool
	seen     int32
	tf       P
	length   P
	score    P
	matched  P
}

// newPass returns an empty pass with one reading per keep, its slots presized
// to capacity.
func (idx *BTreeIndex[K, P]) newPass(keeps []func(K) bool, capacity int) *pass[K, P] {
	p := &pass[K, P]{
		index:    idx,
		at:       make([]int32, len(idx.keys)),
		slots:    make([]passSlot[K, P], 0, capacity),
		readings: make([]reading[K, P], len(keeps)),
	}
	if idx.gamma > 0 && idx.neighbours != nil {
		p.lent = make([]int32, 0, 2*capacity)
	}
	for i, keep := range keeps {
		p.readings[i] = reading[K, P]{keep: keep, docs: make([]readingDoc[P], 0, capacity), touched: make([]int32, 0, capacity)}
	}
	return p
}

// slot returns the slot of the document numbered doc, opening one the first
// time the pass meets it.
func (p *pass[K, P]) slot(doc int32, key K) int32 {
	if s := p.at[doc]; s > 0 {
		return s - 1
	}
	s := p.open(key)
	p.at[doc] = s + 1
	return s
}

// neighbourSlot returns the slot of the neighbour under key. The window may
// name a document the index does not hold, such as a fact with no text. It
// still borrows from its neighbours, so it gets a slot of its own.
func (p *pass[K, P]) neighbourSlot(key K) int32 {
	if doc, ok := p.index.ids[key]; ok {
		return p.slot(doc, key)
	}
	if s, ok := p.unnumbered[key]; ok {
		return s
	}
	if p.unnumbered == nil {
		p.unnumbered = make(map[K]int32)
	}
	s := p.open(key)
	p.unnumbered[key] = s
	return s
}

// open appends a slot for key, with its state in every reading.
func (p *pass[K, P]) open(key K) int32 {
	s := int32(len(p.slots))
	p.slots = append(p.slots, passSlot[K, P]{key: key})
	for i := range p.readings {
		p.readings[i].docs = append(p.readings[i].docs, readingDoc[P]{})
	}
	return s
}

// accepts reports whether r's keep accepts slot s. The keep is asked once
// per slot: the graph's state cannot change under the read lock a search runs
// in, so its answer cannot either.
func (p *pass[K, P]) accepts(r *reading[K, P], s int32) bool {
	d := &r.docs[s]
	if d.verdict == 0 {
		d.verdict = -1
		if r.keep == nil || r.keep(p.slots[s].key) {
			d.verdict = 1
		}
	}
	return d.verdict > 0
}

// adjacent returns the slots of slot s's neighbours, asking the window the
// first time. Each reading skips the neighbours its keep rejects.
func (p *pass[K, P]) adjacent(s int32) []int32 {
	if !p.slots[s].resolved {
		from := int32(len(p.lent))
		for _, n := range p.index.neighbours(p.slots[s].key) {
			p.lent = append(p.lent, p.neighbourSlot(n))
		}
		p.slots[s].from, p.slots[s].to, p.slots[s].resolved = from, int32(len(p.lent)), true
	}
	return p.lent[p.slots[s].from:p.slots[s].to]
}

// own returns slot s's own length, as the relevance model recorded it.
func (p *pass[K, P]) own(s int32) P {
	if !p.slots[s].measured {
		p.slots[s].own, p.slots[s].measured = p.index.relevance.Length(p.slots[s].key), true
	}
	return p.slots[s].own
}

// length returns slot s's window length in r: its own plus gamma of each
// neighbour's that r's keep accepts.
func (p *pass[K, P]) length(r *reading[K, P], s int32) P {
	if !r.docs[s].measured {
		l := p.own(s)
		for _, n := range p.adjacent(s) {
			if p.accepts(r, n) {
				l += p.index.gamma * p.own(n)
			}
		}
		r.docs[s].length, r.docs[s].measured = l, true
	}
	return r.docs[s].length
}

// lendTo adds tf to slot s's frequency for the current term in r.
func (p *pass[K, P]) lendTo(r *reading[K, P], s int32, tf P) {
	d := &r.docs[s]
	if d.seen != p.term {
		d.seen, d.tf = p.term, 0
		r.touched = append(r.touched, s)
	}
	d.tf += tf
}

// accumulate scores one term's posting list into every reading. Without a
// window, each posting a reading accepts adds its gain and the term's weight.
// Under a window, the term's frequency per document is its own count plus
// gamma of each neighbour's: a posting lends to its neighbours, so a document
// is accumulated from up to three postings before it is scored, which is why
// the saturation runs after the list rather than per posting.
func (p *pass[K, P]) accumulate(list *postingList[K], weight P, prepared P) {
	idx := p.index
	p.term++
	if idx.gamma <= 0 || idx.neighbours == nil {
		for _, posting := range list.entries {
			if posting.tf == 0 {
				continue
			}
			s := p.slot(posting.doc, posting.key)
			var gain P
			gained := false
			for i := range p.readings {
				r := &p.readings[i]
				if !p.accepts(r, s) {
					continue
				}
				if !gained {
					gain, gained = idx.relevance.Increment(weight, posting.key, int(posting.tf), prepared), true
				}
				d := &r.docs[s]
				d.seen = p.term
				d.score += gain
				d.matched += weight
			}
		}
		return
	}

	for i := range p.readings {
		p.readings[i].touched = p.readings[i].touched[:0]
	}
	for _, posting := range list.entries {
		if posting.tf == 0 {
			continue
		}
		s := p.slot(posting.doc, posting.key)
		tf := P(posting.tf)
		for i := range p.readings {
			r := &p.readings[i]
			if !p.accepts(r, s) {
				continue
			}
			p.lendTo(r, s, tf)
			for _, n := range p.adjacent(s) {
				if p.accepts(r, n) {
					p.lendTo(r, n, idx.gamma*tf)
				}
			}
		}
	}
	for i := range p.readings {
		r := &p.readings[i]
		for _, s := range r.touched {
			// length may open slots, growing docs, so it runs before d is taken.
			length := p.length(r, s)
			d := &r.docs[s]
			d.score += idx.relevance.Gain(weight, d.tf, length, prepared)
			d.matched += weight
		}
	}
}

// rank finalizes every document r reached into its k best, as the plain and
// the window search alike fold score and matched mass. totalW is the query's
// whole idf mass.
func (p *pass[K, P]) rank(r *reading[K, P], k int, totalW P) ([]K, []P) {
	top := containers.NewTopK[K, P](k, p.index.compare)
	total := int(totalW*1024) + 1
	for s := range r.docs {
		if d := &r.docs[s]; d.seen > 0 {
			top.Offer(p.slots[s].key, p.index.relevance.Finalize(d.score, int(d.matched*1024), total))
		}
	}
	return top.Drain()
}

// Information reports, in bits, how much information query carries and how
// much of it each of keys holds, held[i] for keys[i]. A term's information is its surprisal in the index,
// −log₂(df/N). N is the number of live documents, and df the number that
// hold the term. It is the information in the event that a random document
// holds the term. A rare term carries many bits, and a term every document
// holds carries none.
//
// The query carries the sum over its distinct terms that some live document
// holds. key holds the sum over the terms it holds itself. The difference is
// the information in the terms a key lacks. Summing assumes terms occur
// independently. A term no live document holds has no estimate and is left
// out of both sums. A key that is not indexed holds 0. The query is tokenized
// and each term's postings found once, however many keys are asked about, so
// a whole reading costs one pass over the query's terms.
func (idx *BTreeIndex[K, P]) Information(query string, keys []K) (total P, held []P) {
	held = make([]P, len(keys))
	n := float64(len(idx.documents))
	probe := &postingList[K]{}
	seen := make(map[string]struct{})
	for _, term := range idx.tokenizer.Tokenize(query) {
		if _, dup := seen[term]; dup {
			continue
		}
		seen[term] = struct{}{}
		probe.term = term
		list, ok := idx.tree.Find(probe)
		if !ok || list.live == 0 {
			continue
		}
		bits := P(-math.Log2(float64(list.live) / n))
		total += bits
		for k, key := range keys {
			if i, found := list.find(key, idx.compare); found && list.entries[i].tf > 0 {
				held[k] += bits
			}
		}
	}
	return total, held
}

// Count reports the number of indexed documents.
func (idx *BTreeIndex[K, P]) Count() int {
	return len(idx.documents)
}

// Entries counts the live documents plus those removed or replaced since the
// last compaction, whose tombstones are still in the lists. The automatic
// Flush keeps it at most twice Count.
func (idx *BTreeIndex[K, P]) Entries() int {
	return idx.Count() + idx.dead
}

// Flush drops the tombstones from every posting list. It also removes from
// the dictionary every term that no live document holds, and numbers the live
// documents afresh, in the order they hold, so the numbers stay as dense as
// the documents.
func (idx *BTreeIndex[K, P]) Flush() error {
	ids := make(map[K]int32, len(idx.documents))
	keys := make([]K, 0, len(idx.documents))
	for _, key := range idx.keys {
		if _, live := idx.documents[key]; live {
			ids[key] = int32(len(keys))
			keys = append(keys, key)
		}
	}
	for _, list := range idx.tree.Values() {
		if list.live == 0 {
			idx.tree.Delete(list)
			continue
		}
		list.entries = slices.DeleteFunc(list.entries, func(p posting[K]) bool { return p.tf == 0 })
		for i := range list.entries {
			list.entries[i].doc = ids[list.entries[i].key]
		}
	}
	idx.ids, idx.keys = ids, keys
	idx.dead = 0
	return nil
}

// maybeFlush compacts the postings once the index holds more dead documents
// than live ones. It runs after every mutation, which keeps the lists O(live
// documents) at amortised-constant compaction cost.
func (idx *BTreeIndex[K, P]) maybeFlush() error {
	if idx.dead <= len(idx.documents) {
		return nil
	}
	logger.Debug("Text index compaction", "live", len(idx.documents), "dead", idx.dead)
	return idx.Flush()
}

// index stores value under key. It first retires any previous document under
// key: its postings become tombstones and the relevance model forgets it. A
// term the new text shares with the old one revives its tombstone in place.
func (idx *BTreeIndex[K, P]) index(key K, value string) error {
	if old, err := idx.Retrieve(key); err == nil {
		idx.removePostings(key, old)
	}

	doc, numbered := idx.ids[key]
	if !numbered {
		doc = int32(len(idx.keys))
		idx.ids[key] = doc
		idx.keys = append(idx.keys, key)
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
			list.entries = slices.Insert(list.entries, i, posting[K]{key: key, doc: doc, tf: 1})
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
// and retires the document from the relevance model. Delete and a re-index
// under the same key both retire through here. So the model sees exactly one
// Removed per Indexed, and each removal counts one dead document for Flush to
// reclaim.
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

// find returns where key sits in the sorted entries and whether it is there.
// When it is absent, the position is where an insert keeps the list sorted.
func (l *postingList[K]) find(key K, compare comparator.Comparator[K]) (int, bool) {
	i := sort.Search(len(l.entries), func(i int) bool { return compare(l.entries[i].key, key) >= 0 })
	return i, i < len(l.entries) && l.entries[i].key == key
}
