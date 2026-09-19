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
)

// The tool schemas and types mirror the HTTP query API (POST /api/v1/q)
// exactly: the bridge forwards its input as the request body and returns the
// response body as its output, so what a model sees here is the same wire
// contract pkg/server and internal/query define — HandleQueryRequest on the
// way in, {"results": QueryResult} plus optional warnings on the way out for a
// recall, {"status": "ok"} for a write.
// Explain-mode fields (background, per-hit contributions) belong to
// /api/v1/explain, which the bridge does not call, so they do not appear
// here. If either wire shape moves, these move in the same change.

// parametersSchema mirrors HandleQueryRequest.Parameters: vector
// placeholders (vec:$name) are bound out of band by name, so large vector
// literals never ride inline in the query text.
var parametersSchema = &jsonschema.Schema{
	Type:        "object",
	Description: "Out-of-band vector bindings: one array of numbers per vec:$name placeholder the query references.",
	AdditionalProperties: &jsonschema.Schema{
		Type:  "array",
		Items: &jsonschema.Schema{Type: "number"},
	},
}

// hitSchema mirrors Hit.MarshalJSON: the stored fact, when it was written,
// and its raw relevance. The score is on the scorer's own scale — an
// ordering, not a probability — and nothing caps it at 1.0.
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
		"score": {
			Type:        "number",
			Description: "Raw relevance in the scorer's own units: an ordering, not a probability, with no upper bound. When anchors alone seed the search it is one unit per named anchor the fact is filed under, decayed by age.",
		},
	},
	Required: []string{"value", "timestamp", "score"},
}

// warningsSchema mirrors the response's optional warnings key: parse
// warnings the server attached. The query still ran and the results beside
// them stand.
var warningsSchema = &jsonschema.Schema{
	Type:        "array",
	Description: "Parse warnings the server attached; the query still ran and the results stand.",
	Items:       &jsonschema.Schema{Type: "string"},
}

var (
	rememberInputSchema = &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"query": {
				Type:        "string",
				Description: "A full FQL remember: the fact in single quotes, anchors as repeatable topic:/entity: pairs, @N selecting the graph, vec:$name binding an optional vector. Example: remember@2 'the barometer falls before the storm' topic:weather entity:harbour",
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
				Description: "A full FQL recall: bare search terms, then optional topic:/entity: filters, top:N result cap, depth:0-2 retrieval lane (the graph is searched only beside a topic:/entity:), since:/until: time bounds, vec:$name for an optional vector seed. Example: recall@2 barometer storm topic:weather top:5 depth:1 since:7d. Anchors alone, with no terms and no vec:, seed the search with every fact filed under those topics/entities, scored one unit per named anchor and decayed by age, so newest first under one anchor (depth: has no effect). Example: recall@2 topic:weather top:20",
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
				},
				Required: []string{"count", "hits"},
			},
			"warnings": warningsSchema,
		},
		Required: []string{"results"},
	}
)

// Hit is one recalled fact as the query endpoint serializes it
// (Hit.MarshalJSON in internal/query): the stored value, its write time, and
// its raw score. Scores are float64 regardless of the daemon's precision —
// that is what JSON numbers decode to, and the bridge never does arithmetic
// on them.
type Hit struct {
	Value     string    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	Score     float64   `json:"score"`
}

// Result is the results envelope of a recall response: the hit count and the
// ranked hits, best first. Only reads carry one — a write is acknowledged with
// a status token instead (RememberOutput).
type Result struct {
	Count int   `json:"count"`
	Hits  []Hit `json:"hits"`
}

// RecallInput is the recall tool's argument payload, forwarded verbatim as
// the /api/v1/q request body (HandleQueryRequest): the raw FQL query plus
// any out-of-band vector bindings its vec:$name placeholders reference.
type RecallInput struct {
	Query      string               `json:"query"`
	Parameters map[string][]float64 `json:"parameters,omitempty"`
}

// RecallOutput is the recall tool's result payload, the /api/v1/q response
// body verbatim: the results envelope plus any parse warnings the server
// attached — the query still ran and the results beside them stand.
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
// response verbatim: the acknowledgement token, and any parse warnings the
// server attached — the write committed, but the parse read close to a
// different query. It is deliberately not a recall's envelope: a write and a
// recall that matched nothing used to be the same bytes, which made a fact
// that never landed indistinguishable from one that did.
type RememberOutput struct {
	Status   string   `json:"status"`
	Warnings []string `json:"warnings,omitempty"`
}
