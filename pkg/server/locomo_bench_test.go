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

package server_test

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/FraiseHQ/fraise/internal/query/lexer"
)

// locomoCategories names LoCoMo's question categories by the number the
// dataset files them under; the sub-benchmarks are named after them.
var locomoCategories = []string{1: "multi-hop", 2: "temporal", 3: "open-domain", 4: "single-hop", 5: "adversarial"}

// locomoConversation is one LoCoMo conversation in the dataset's own shape:
// session_N holds the turns of session N, from 1, and session_N_date_time
// when it took place.
type locomoConversation struct {
	SampleID     string                     `json:"sample_id"`
	Conversation map[string]json.RawMessage `json:"conversation"`
	QA           []locomoQuestion           `json:"qa"`
}

type locomoTurn struct {
	Speaker     string `json:"speaker"`
	Text        string `json:"text"`
	BlipCaption string `json:"blip_caption"`
}

// locomoQuestion is a question with the dia_ids ("D8:6", session 8 turn 6)
// of the turns that answer it, and its category.
type locomoQuestion struct {
	Question string   `json:"question"`
	Evidence []string `json:"evidence"`
	Category int      `json:"category"`
}

// question is one LoCoMo question ready to send: its recall, the vector bound
// to it, the sessions its evidence lies in, and the session each value of its
// conversation was said in.
type question struct {
	category int
	fql      string
	vec      []float64
	evidence map[string]bool
	sessions map[string]string
}

// evidenceSession reads a session number out of a dia_id ("D8:6"). A handful
// of evidence strings join several ids ("D8:6; D9:17") or are malformed
// ("D:11:26"), so every well-formed id in a string counts and nothing else.
var evidenceSession = regexp.MustCompile(`D(\d+):\d+`)

// plainWord is the bare-word alphabet of the grammar and of the amb adapter,
// which builds a recall from the question's runs of letters and digits.
var plainWord = regexp.MustCompile(`[A-Za-z0-9]+`)

// anchorBreak is what the amb adapter folds to a hyphen in an anchor name.
var anchorBreak = regexp.MustCompile(`[^A-Za-z0-9]+`)

// fqlValue writes an anchor or a term bare when it is one plain word and
// quoted otherwise. A term that spells a reserved word is quoted as well:
// bare, it is syntax.
func fqlValue(v string) string {
	_, reserved := lexer.KeywordsMap[strings.ToLower(v)]
	if !reserved && plainWord.FindString(v) == v {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// withVec appends the vector clause when the run binds one.
func withVec(fql string, vec []float64) string {
	if vec == nil {
		return fql
	}
	return fql + " vec:$v"
}

// loadLoCoMo reads the conversations in the LoCoMo file at path, in the
// dataset's own format.
func loadLoCoMo(b *testing.B, path string) []locomoConversation {
	b.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read LoCoMo: %v", err)
	}
	var convs []locomoConversation
	if err := json.Unmarshal(raw, &convs); err != nil {
		b.Fatalf("decode LoCoMo: %v", err)
	}
	return convs
}

// loadVectors reads the embeddings FRAISE_LOCOMO_VECTORS names, one vector per
// embedded text, or returns nil when it is unset and the run is text only.
func loadVectors(b *testing.B) map[string][]float64 {
	b.Helper()
	path := os.Getenv("FRAISE_LOCOMO_VECTORS")
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read vectors: %v", err)
	}
	var vectors map[string][]float64
	if err := json.Unmarshal(raw, &vectors); err != nil {
		b.Fatalf("decode vectors: %v", err)
	}
	return vectors
}

// vectorOf returns the cached embedding of text. A text missing from a cache
// that exists means the cache was built from other values than the ones
// stored, and every vector score measured with it would be wrong, so the run
// stops rather than fall back to text only for some facts.
func vectorOf(b *testing.B, vectors map[string][]float64, text string) []float64 {
	b.Helper()
	if vectors == nil {
		return nil
	}
	v, ok := vectors[text]
	if !ok {
		b.Fatalf("FRAISE_LOCOMO_VECTORS has no vector for %q: rebuild it with make perf-vectors", text)
	}
	return v
}

// ingest stores conv the way the amb adapter's raw path does: one fact per
// turn, in order, valued "[session N @ <date>] <speaker>: <text>" with any
// shared image's caption appended, filed under the conversation and session
// topics and the speaker's entity, all in graph 0. It returns the session each
// stored value belongs to, which is how a hit is traced back to evidence.
func ingest(b *testing.B, url string, conv locomoConversation, vectors map[string][]float64) map[string]string {
	b.Helper()
	sessions := map[string]string{}
	for i := 1; ; i++ {
		n := strconv.Itoa(i)
		raw, ok := conv.Conversation["session_"+n]
		if !ok {
			return sessions
		}
		var turns []locomoTurn
		if err := json.Unmarshal(raw, &turns); err != nil {
			b.Fatalf("%s session %s: %v", conv.SampleID, n, err)
		}
		var date string
		if err := json.Unmarshal(conv.Conversation["session_"+n+"_date_time"], &date); err != nil {
			b.Fatalf("%s session %s date: %v", conv.SampleID, n, err)
		}

		for _, t := range turns {
			text := t.Text
			if t.BlipCaption != "" {
				text += " [shared image: " + t.BlipCaption + "]"
			}
			// The adapter swaps the ASCII apostrophe for a typographic one
			// before storing, so the stored value, and the text its vector was
			// computed from, carry the swap.
			value := strings.ReplaceAll(fmt.Sprintf("[session %s @ %s] %s: %s", n, date, t.Speaker, text), "'", "’")
			sessions[value] = n

			vec := vectorOf(b, vectors, value)
			speaker := strings.Trim(anchorBreak.ReplaceAllString(t.Speaker, "-"), "-")
			fql := fmt.Sprintf("remember@0 %s topic:%s topic:%s entity:%s",
				fqlValue(value), fqlValue("conv-"+conv.SampleID), fqlValue("session-"+n), fqlValue(speaker))
			query(b, url, withVec(fql, vec), vec)
		}
	}
}

// questions turns conv's QA into recalls the way the amb adapter does: the
// question's plain words as terms, scoped to the conversation's topic, ten
// results at the server's default depth, and the question's own embedding as
// the vector seed. A question whose evidence names no session cannot be
// scored and is left out.
func questions(b *testing.B, conv locomoConversation, sessions map[string]string, vectors map[string][]float64) []question {
	b.Helper()
	out := make([]question, 0, len(conv.QA))
	for _, qa := range conv.QA {
		evidence := map[string]bool{}
		for _, e := range qa.Evidence {
			for _, m := range evidenceSession.FindAllStringSubmatch(e, -1) {
				evidence[m[1]] = true
			}
		}
		if len(evidence) == 0 {
			continue
		}

		terms := plainWord.FindAllString(qa.Question, -1)
		for i, t := range terms {
			terms[i] = fqlValue(t)
		}
		vec := vectorOf(b, vectors, qa.Question)
		fql := fmt.Sprintf("recall@0 %s topic:%s top:10", strings.Join(terms, " "), fqlValue("conv-"+conv.SampleID))
		out = append(out, question{category: qa.Category, fql: withVec(fql, vec), vec: vec, evidence: evidence, sessions: sessions})
	}
	return out
}

// scores are the session-level retrieval metrics of one question: a hit
// counts for the session its fact was said in, and a session retrieved twice
// counts once, as the amb adapter scores them.
type scores struct {
	f1, p1, recall float64
}

func score(q question, hits []string) scores {
	if len(hits) == 0 {
		return scores{}
	}
	retrieved := map[string]bool{}
	for _, h := range hits {
		retrieved[q.sessions[h]] = true
	}
	found := 0
	for s := range retrieved {
		if q.evidence[s] {
			found++
		}
	}
	precision := float64(found) / float64(len(retrieved))
	recall := float64(found) / float64(len(q.evidence))
	var s scores
	if precision+recall > 0 {
		s.f1 = 2 * precision * recall / (precision + recall)
	}
	if q.evidence[q.sessions[hits[0]]] {
		s.p1 = 1
	}
	s.recall = recall
	return s
}

// BenchmarkRetrievalQuality ingests the LoCoMo conversations FRAISE_LOCOMO
// names into a fresh server and reports how well its recalls find the
// sessions each question's evidence lies in: f1@10, p@1 and recall@10,
// macro-averaged over all questions and over each category's. Ingestion and
// questions follow the amb adapter's raw path, so the numbers are comparable
// with a LoCoMo run of the same server through amb.
//
// The data is not part of this package: tests/perf/data holds the sample the
// benchmark gates run on, and the harness passes its path. Without one the
// benchmark is skipped. The text set stores and asks with text only; the
// vectors set, run when FRAISE_LOCOMO_VECTORS names an embeddings file, seeds
// and stores with those vectors as well. Each gets its own server, and a run
// with embeddings measures both, so text is always compared with text and
// vectors with vectors whichever runs measured them. Run it with -benchtime
// 1x: each sub-benchmark asks its questions once per iteration, and one is
// the measurement.
//
// The metrics are exact: the same server given the same facts and questions
// ranks them the same way, so a single run is a measurement and any change is
// a change in retrieval. The Unit lines say so to whatever reads the output,
// which otherwise waits for enough runs to test a difference that has no
// noise.
func BenchmarkRetrievalQuality(b *testing.B) {
	path := os.Getenv("FRAISE_LOCOMO")
	if path == "" {
		b.Skip("FRAISE_LOCOMO is unset: point it at a LoCoMo file, such as tests/perf/data/locomo-conv-26.json")
	}
	convs := loadLoCoMo(b, path)
	for _, unit := range []string{"f1@10", "p@1", "recall@10"} {
		fmt.Printf("Unit %s better=higher assume=exact\n", unit)
	}

	sets := map[string]map[string][]float64{"text": nil}
	if vectors := loadVectors(b); vectors != nil {
		sets["vectors"] = vectors
	}
	for _, set := range []string{"text", "vectors"} {
		vectors, ok := sets[set]
		if !ok {
			continue
		}
		b.Run(set, func(b *testing.B) {
			url := startServer(b)
			var qs []question
			for _, conv := range convs {
				qs = append(qs, questions(b, conv, ingest(b, url, conv, vectors), vectors)...)
			}

			b.Run("all", func(b *testing.B) { measure(b, url, qs) })
			for c, name := range locomoCategories {
				var in []question
				for _, q := range qs {
					if q.category == c {
						in = append(in, q)
					}
				}
				if len(in) > 0 {
					b.Run(name, func(b *testing.B) { measure(b, url, in) })
				}
			}
		})
	}
}

// measure asks qs once per iteration and reports their mean scores.
func measure(b *testing.B, url string, qs []question) {
	var total scores
	for b.Loop() {
		total = scores{}
		for _, q := range qs {
			s := score(q, query(b, url, q.fql, q.vec))
			total.f1 += s.f1
			total.p1 += s.p1
			total.recall += s.recall
		}
	}
	n := float64(len(qs))
	b.ReportMetric(total.f1/n, "f1@10")
	b.ReportMetric(total.p1/n, "p@1")
	b.ReportMetric(total.recall/n, "recall@10")
}
