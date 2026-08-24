# HTTP API

Fraise exposes three endpoints. Everything an agent does goes through one of
them; the query language ([`query-spec.md`](query-spec.md)) carries the rest.

The surface is deliberately small: handlers bind the request, hand the query
string to the parser, refuse what the client got wrong, and wait for the engine.
Nothing that decides *what an answer is* lives here.

## `GET /` — health check

```json
{"status": "ok", "version": "0.1.0-beta.8"}
```

`version` is the SDKs' only handshake: they read it to check the server falls in
the range they support (see `COMPATIBILITY.md`), so the field is part of the
contract rather than a convenience.

## `POST /api/v1/q` — query

The one endpoint that reads and writes. The body carries the query and any
parameters it references:

```json
{
  "query": "recall bird topic:garden vec:$v top:5",
  "parameters": {"v": [0.10, 0.22, 0.87]}
}
```

| Field        | Type                   | Required | Notes                                   |
|--------------|------------------------|----------|-----------------------------------------|
| `query`      | string                 | yes      | one command per request                 |
| `parameters` | object of float arrays | no       | binds `$name` references, e.g. `vec:$v` |

Vectors stay out of the query string on purpose: the query is a cache key and a
log line, and an inline embedding would ruin both.

A successful response carries ranked hits, newest-and-strongest first:

```json
{
  "results": {
    "count": 1,
    "hits": [
      {
        "value": "the parrot is turquoise",
        "timestamp": "2026-08-09T17:40:53.851+01:00",
        "score": 1
      }
    ]
  }
}
```

`count` is how many hits were returned, not how many exist — a recall is capped
by `top:` and by `default-top`. A write returns 200 with an empty result set.

## `POST /api/v1/explain` — explained recall

The same request body and pipeline as `/q`, for recalls only: each hit also
carries `contributions`, the per-source sightings its score was folded from.
Use it to see *why* a fact ranked where it did; use `/q` when the ranking is
all you need — the breakdown costs response tokens, which is why it lives on
its own endpoint instead of every recall.

```json
{
  "results": {
    "count": 1,
    "hits": [
      {
        "value": "the parrot is turquoise",
        "timestamp": "2026-08-09T17:40:53.851+01:00",
        "score": 0.75,
        "contributions": [
          { "source": "text",  "score": 1, "rank": 0, "hop": 0 },
          { "source": "graph", "score": 2, "rank": 1, "hop": 2 }
        ]
      }
    ]
  }
}
```

One contribution records one sighting of the hit by one retrieval source:

| Field    | Meaning                                                            |
|----------|--------------------------------------------------------------------|
| `source` | which stage saw it: `text`, `vector` or `graph`                    |
| `score`  | the source's raw magnitude: match count, similarity, or seed score |
| `rank`   | the hit's position in that source's own result list, 0 first       |
| `hop`    | 0 for a seed; how many hops from its seed for a graph sighting     |

`score` on the hit remains the final fused value (after recency decay and any
ranking boost), so the contributions explain its ingredients rather than
summing to it exactly.

A `remember` on this endpoint is rejected with 400: a write has no ranking to
explain, and explaining must never mutate a graph.

## `GET /api/v1/stats` — per-graph snapshot

One entry per graph, in selector order, computed on demand from the live graphs:

```json
{
  "graphs": [
    {"id": 0, "order": 12, "size": 18, "nodes": 30, "vectors": 4, "forest_entries": 6}
  ]
}
```

| Field            | Meaning                                                                     |
|------------------|-----------------------------------------------------------------------------|
| `order`          | entities (vertices)                                                         |
| `size`           | relationships (edges)                                                       |
| `nodes`          | total stored nodes                                                          |
| `vectors`        | vectors indexed                                                             |
| `forest_entries` | entries in the vector forest: live vectors plus garbage awaiting compaction |

`forest_entries` exists to make an internal invariant observable: it must stay
within `flush-factor × vectors`, so a leak in the vector index shows up here
before it shows up as memory.

## Errors

Every failure returns the same shape, with the status code carrying the category:

```json
{"error": "parse error at column 24: Expected colon, but found \"food\""}
```

| Status | When                                                                                                                                  |
|--------|---------------------------------------------------------------------------------------------------------------------------------------|
| 400    | malformed JSON, a body over `max-body-bytes`, an unparseable query, a missing parameter, a vector whose dimension does not match its graph, a graph selector outside the allocated range |
| 429    | the scheduler queue stayed full past `enqueue-timeout` — back off and retry |
| 503    | the server is shutting down |
| 500    | an internal failure |

Two properties are deliberate:

* **A client error says exactly what is wrong**, including the column a parse
  failed at and the values a field accepts. The caller is usually a model, and
  a model can correct itself from a precise message — that is the whole reason
  the parser refuses to guess.
* **A 500 never carries detail.** The body is a generic message and the specifics
  go to the log, so an internal error cannot leak the shape of the store to a
  client.
