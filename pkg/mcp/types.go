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

package mcp

import (
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/FraiseHQ/fraise/internal/containers"
)

// The tool schemas and types mirror the HTTP query API (POST /api/v1/q). The
// bridge forwards its input as the request body and returns the response body
// as its output. pkg/server and internal/query define the wire contract. The
// request is a HandleQueryRequest. A recall answers {"results": QueryResult}
// and a write answers {"status": "ok"}, each with optional warnings.
// Explain-mode fields (background, the answer of each when a bound read,
// per-hit contributions and decay) belong to /api/v1/explain, which the bridge
// does not call. If either wire shape moves, these move with it in the same
// change.

// parametersSchema mirrors HandleQueryRequest.Parameters. Vector placeholders
// (vec:$name) are bound out of band by name, so large vector literals never
// ride inline in the query text.
var parametersSchema = &jsonschema.Schema{
	Type:        "object",
	Description: "Out-of-band vector bindings: one array of numbers per vec:$name placeholder the query references.",
	AdditionalProperties: &jsonschema.Schema{
		Type:  "array",
		Items: &jsonschema.Schema{Type: "number"},
	},
}

// intervalSchema mirrors containers.Interval: a span that holds start and
// excludes end. A single moment has the two equal, and a span that has not
// ended carries no end.
var intervalSchema = &jsonschema.Schema{
	Type: "object",
	Properties: map[string]*jsonschema.Schema{
		"start": {
			Type:        "string",
			Format:      "date-time",
			Description: "Where the span starts.",
		},
		"end": {
			Type:        "string",
			Format:      "date-time",
			Description: "Where the span ends, itself excluded: equal to start for a single moment, absent for a span that has not ended.",
		},
	},
	Required: []string{"start"},
}

// hitSchema mirrors Hit.MarshalJSON: the stored fact, its three times, and its
// score (relevance after recency decay). Only the write time is always there.
// The other two are the ones the writer gave, so a fact given neither carries
// neither. The score is on the scorer's own scale. It is an ordering, not a
// probability, and nothing caps it at 1.0.
var hitSchema = &jsonschema.Schema{
	Type: "object",
	Properties: map[string]*jsonschema.Schema{
		"value": {
			Type:        "string",
			Description: "The remembered fact, exactly as stored.",
		},
		"timestamp": {
			Type:        "string",
			Format:      "date-time",
			Description: "When the fact was written.",
		},
		"said": {
			Type:        "string",
			Format:      "date-time",
			Description: "When the fact was said, as its remember gave it with said:. Absent when it gave none: the fact was then said when it was written.",
		},
		"occurred": {
			Type:        "array",
			Description: "Each span over which what the fact states happened, as its remember gave them with occurred:. Absent when it gave none: the fact then happened when it was said.",
			Items:       intervalSchema,
		},
		"score": {
			Type:        "number",
			Description: "Raw relevance in the scorer's own units: an ordering, not a probability, with no upper bound. When anchors alone seed the search it is one unit per named anchor the fact is filed under, decayed by age.",
		},
	},
	Required: []string{"value", "timestamp", "score"},
}

// warningsSchema mirrors the response's optional warnings key, which holds
// the warnings the server attached: those of the parse, then those raised
// while the query ran. The query still ran, and the results beside them stand.
var warningsSchema = &jsonschema.Schema{
	Type:        "array",
	Description: "Warnings the server attached; the query still ran and the results stand.",
	Items:       &jsonschema.Schema{Type: "string"},
}

var (
	rememberInputSchema = &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"query": {
				Type:        "string",
				Description: "A full FQL remember: the fact in single quotes, anchors as repeatable topic:/entity: pairs, @N selecting the graph, vec:$name binding an optional vector. An anchor value that is not letters and digits only is quoted too: topic:'machine-learning'. Two clauses date the fact. said: is when it was said, given once: the date of the message it comes from. occurred: is when what it states happened, and repeats for something that happened more than once. Each takes a quoted date, month or year (said:'2026-01-15', occurred:'2026-01', occurred:'2025'), and occurred: also a range of them, both ends included ('2026-01-10'..'2026-01-12'), or one left open for what is not over ('2026-01-10'..). A fact with no occurred: happened when it was said, and one with no said: was said when it was written. Example: remember@2 'the barometer fell before the storm' topic:weather entity:harbour said:'2026-01-15' occurred:'2026-01-14'",
			},
			"parameters": parametersSchema,
		},
		Required: []string{"query"},
	}
	recallInputSchema = &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"query": {
				Type:        "string",
				Description: "A full FQL recall: search terms, then optional topic:/entity: anchors (beside terms they lift the facts filed under them through the graph and filter nothing, so the recall runs at depth:1 at least), top:N result cap, depth:0-2 retrieval lane (the graph is searched only beside a topic:/entity:), since:/until: time bounds, vec:$name for an optional vector seed. A bare term or value is letters and digits only; quote anything else: 'e-mail', topic:'machine-learning'. A search term that spells a keyword (when, said, top, since...) is quoted too: 'when'. Example: recall@2 barometer storm topic:weather top:5 depth:1 since:7d. A bound keeps the facts that happened on its side of a time: since: from it on, until: before it. The time is a span back from now (since:7d), a quoted date, month or year (since:'2026-01-15', until:'2026-02', since:'2025'), or a when in parentheses, which is the time another fact holds: until:(when 'the regatta') keeps what happened before the regatta began, and since:(when 'the regatta') what happened once it was over. A when searches as a recall does and reads its best match. It reads when that fact happened unless it names another of its times, as in since:(when:said 'the regatta') or when:written. A when can also be the whole query: when 'the regatta' answers with the time itself and the fact it was read from. It takes no @N: in a bound it reads the graph of its recall, and on its own the default graph. Anchors alone, with no terms and no vec:, seed the search with every fact filed under those topics/entities, scored one unit per named anchor and decayed by age, so newest first under one anchor (depth: has no effect). Example: recall@2 topic:weather top:20",
			},
			"parameters": parametersSchema,
		},
		Required: []string{"query"},
	}
	rememberOutputSchema = &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"status": {
				Type:        "string",
				Description: "The write acknowledgement, the fixed token \"ok\". A write has no result set to return; this key's presence is what distinguishes an accepted write from a recall that matched nothing.",
			},
			"warnings": warningsSchema,
		},
		Required: []string{"status"},
	}
	recallOutputSchema = &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"results": {
				Type:        "object",
				Description: "Ranked recall results, best first; with anchors alone, each hit scores one unit per named anchor it is filed under, decayed by age — newest first under a single anchor.",
				Properties: map[string]*jsonschema.Schema{
					"count": {
						Type:        "integer",
						Description: "Number of hits returned, after the top: cap.",
					},
					"hits": {Type: "array", Items: hitSchema},
					"start": {
						Type:        "string",
						Format:      "date-time",
						Description: "Where the time a when answered with starts. Only a when that matched a fact carries it.",
					},
					"end": {
						Type:        "string",
						Format:      "date-time",
						Description: "Where that time ends, itself excluded: equal to start for a single moment, absent for a time that has not ended.",
					},
				},
				Required: []string{"count", "hits"},
			},
			"warnings": warningsSchema,
		},
		Required: []string{"results"},
	}
)

// Hit is one recalled fact as the query endpoint serializes it
// (Hit.MarshalJSON in internal/query): the stored value, its times and its raw
// score. Said and Occurred are the times its remember gave it, and stay zero
// for a fact given none. The server leaves them off the wire then, and
// omitzero and omitempty leave them off the structured output the same way.
// Scores are float64 whatever the daemon's precision. JSON numbers decode to
// float64, and the bridge never does arithmetic on them.
type Hit struct {
	Value     string                `json:"value"`
	Timestamp time.Time             `json:"timestamp"`
	Said      time.Time             `json:"said,omitzero"`
	Occurred  []containers.Interval `json:"occurred,omitempty"`
	Score     float64               `json:"score"`
}

// Result is the results envelope of a recall response: the hit count and the
// ranked hits, best first. Start and End are the time a when answers with, an
// interval that holds Start and excludes End. A recall leaves both zero, and
// End alone is zero for an answer that has not ended. Only reads carry a
// Result. A write is acknowledged with a status token instead
// (RememberOutput).
type Result struct {
	Count int       `json:"count"`
	Hits  []Hit     `json:"hits"`
	Start time.Time `json:"start,omitzero"`
	End   time.Time `json:"end,omitzero"`
}

// RecallInput is the recall tool's argument payload, forwarded verbatim as the
// /api/v1/q request body (HandleQueryRequest). It holds the raw FQL query and
// any out-of-band vector bindings its vec:$name placeholders reference.
type RecallInput struct {
	Query      string               `json:"query"`
	Parameters map[string][]float64 `json:"parameters,omitempty"`
}

// RecallOutput is the recall tool's result payload, the /api/v1/q response
// body verbatim. It holds the results envelope and any warnings the server
// attached. The query still ran, and the results beside them stand.
type RecallOutput struct {
	Results  Result   `json:"results"`
	Warnings []string `json:"warnings,omitempty"`
}

// RememberInput is the remember tool's argument payload, forwarded verbatim
// as the /api/v1/q request body, exactly as RecallInput is.
type RememberInput struct {
	Query      string               `json:"query"`
	Parameters map[string][]float64 `json:"parameters,omitempty"`
}

// RememberOutput is the remember tool's result payload, the /api/v1/q write
// response verbatim. It holds the acknowledgement token and any parse warnings
// the server attached (the write still committed). It is not a recall's
// envelope on purpose, so a write cannot be mistaken for a recall that
// matched nothing.
type RememberOutput struct {
	Status   string   `json:"status"`
	Warnings []string `json:"warnings,omitempty"`
}
