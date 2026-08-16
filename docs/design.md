# Fraise design

This document discusses the components and overall architecture design of Fraise.

## Introduction

Fraise is designed to be a high-performance, in-memory database for AI agents
contextual memory. Agents are given access to a simple DSL that allows them to
store and retrieve memory in real time.

Everything below follows from one constraint: the caller is a model, not a
person. It queries far more often than a human would, it pays for every token it
reads back, and it cannot tell a wrong answer from a right one. That is why the
query language is small, why results are ranked and capped rather than complete,
and why an unparseable query is an error rather than a best guess — see
[`query-spec.md`](query-spec.md).

## Context engineering

A model's context is assembled per call, and memory is only one of the things
competing for it. Fraise's job is not to hold everything an agent has ever seen;
it is to answer "what, of what I know, is worth the tokens for *this* turn".

Two consequences run through the design:

* **Retrieval is ranked and bounded, never exhaustive.** A recall returns the
  top few facts (10 by default, `top:` to change it), because a caller that
  cannot afford to read a thousand rows is not served by being offered them.
* **Recency is a first-class ranking signal.** Contextual memory ages: what an
  agent learned an hour ago usually beats what it learned last quarter. That is
  the "temporal" in temporal memory graph, and it is scored rather than
  filtered — see [Temporality](#temporality).

## Design

Fraise is an in-memory database with a complex query system flexible enough to
store and retrieve contextual information. As mentioned earlier, memory is only
one of many parts of context, however, any long term memory storage can find its
way in the model short term memory. A system that allows to keep track and
update existing memory.

This is done via sending `streams` to the database. A stream contains a single
update to the `temporal memory graph`. A memory graph is made of knowledge:

* `facts`
* `entities`
* `relationships`

and a memory retrieval system. Facts, entities and relationships are also
indexed for fast retrieval via graph, semantic and text representation. Those
representations are gathered in indices for fast query. A federated query engine
manages retrieval and ranking. The memory graph is referred to as temporal
because short term memories are by default deemed more relevant than older ones.

## Memory Graphs

A Fraise database consists of multiple memory graphs (by default 8), selected by
the query itself: `recall@3` and `remember@3` address graph 3, and an absent
selector means graph 0.

A graph is a full isolation boundary — its own nodes, edges, text index and
vector index — and nothing traverses from one to another. That makes a graph the
natural unit for a tenant, a user, or a session: two agents on different graphs
cannot read each other's memories even by accident, and their writes never
contend.

Selectors are `uint8` and bounded twice over: the parser rejects anything that
does not fit the type (so `@256` cannot wrap around to graph 0 and land in
another tenant's memory), and the server then rejects selectors past the
allocated count. Raising `num-graphs` raises the isolation available; it does
not shard a single tenant's data.

## Temporality

Every fact carries the time it was written, and a recall's score decays with
that age on an exponential half-life (`half-life`, one week by default):

```text
score *= 0.5 ^ (age / half-life)
```

So a fact twice the half-life old contributes a quarter of its score. Decay is
ranking, not filtering — an old fact still surfaces when nothing newer matches,
which is what makes it safe to leave on by default. A non-positive half-life
disables it entirely.

Filtering is what `since:` and `until:` are for, and they are absolute
constraints rather than nudges: a fact outside the window does not appear at any
score. Bounds are written as a duration read as "ago" (`since:7d`) or as a date
(`since:2026-01-15`).

## Indices

Each temporal memory graph indexes all data ingested into a full-text search
index and a vector index (when embeddings are provided).

The two are seeds, not answers, and retrieval scores are **BM25 plus
transmitted surplus**. A recall pulls a bounded number of candidates from each
source it can use (at least `seed-size`, widened to `top:` when more results
are asked for), each carrying its raw retrieval mass: BM25 × query coverage
from the text index, `1/(1+distance)` similarity from the vector index. That
mass then flows into the fact–anchor graph: every anchor (topic or entity
node) the query touches observes the total seed mass on its members and
compares it to a **background rate** — the mass a size-proportional smear
across the query's touched anchors would predict for an anchor of its degree.
Only the excess above background is transmitted to the anchor's members,
attenuated per edge (α = 0.5, α² over the two-edge seed→anchor→fact path),
with the member's own mass excluded so a fact never funds its own boost. A
fact's relevance is its own mass plus everything transmitted to it.

One sentence of intuition: *an anchor may only speak when its members matched
better than its size alone predicts — hubs are heard exactly when they are
surprising, and silent when they are merely large.* Four consequences are the
contract, each pinned by the test suite:

* **BM25 floor.** Transmission only ever adds, so anchored search can never
  fall below the plain text index's ranking by scoring alone; with no surplus
  anywhere the two are identical.
* **Hub silence.** An anchor at its fair share of the background transmits
  nothing — a mega-hub cannot flood the tail on size alone, structurally.
* **Earned preemption.** A fact with no match of its own outranks a match
  only through anchors carrying genuine above-background evidence, never
  through mere reachability.
* **No normalization, no knobs.** Scores stay in raw seed units end to end
  (relevance is homogeneous in the mass scale, so normalizing is a provable
  ordering no-op that only breaks the channels' commensurability), and the
  methodology carries zero dataset-tuned constants.

This is why a recall needs at least one term: without a seed there is no mass
to transmit. `depth:` selects which of the two lanes above a recall takes:
`depth:1` — the default — stops at the seed mass, which *is* the BM25 floor,
and skips the anchor traversal entirely, so it is the fast, text-only lane;
`depth:2` runs the one anchor-mediated round described here. Depth counts
edges, and transmission is a two-edge path (fact → anchor → fact), so those
are the only two meaningful values: 2 is the ceiling, and a larger depth is
rejected rather than silently answered as 2. The methodology does not iterate
— a second round re-observes the first round's own concentrated mass through
sibling anchors and collapses recall — so iterated transmission stays
reserved for a future generalization.

The vector index is a forest of random-projection trees (`rptree-n-trees`, each
projecting to `rptree-projection-dimension`). Deleting or overwriting a vector
leaves garbage behind rather than restructuring the trees, so the forest is
rebuilt from the live set once it holds more than `rptree-flush-factor` entries
per live vector — bounding it at O(live vectors) under sustained writes. The
`/api/v1/stats` endpoint exposes `forest_entries` so that bound is observable
rather than assumed.

## Architecture

The database application is made up of the following components:

* database
* server
* engine
* scheduler

A query passes through all four in one direction:

```text
HTTP request
  │
  ▼
server     parse the body, parse the query, reject client errors
  │
  ▼
engine     look up or build a plan (stream), then submit it
  │
  ▼
scheduler  queue the stream; a worker takes it and locks its graph
  │
  ▼
database   apply the stream to graph N: index writes, or seed + walk
  │
  ▼
results marshalled back onto the waiting request
```

Each layer is generic in `[K, P]`: `K` is the node key type (`uint64` in
production) and `P` the float precision of embeddings and scores, chosen at
startup by `db.precision`. Both instantiations are compiled in; `cmd/server`
picks one and the whole stack below follows.

### Engine

Fraise query language is inspired from the simplicity of Redis and borrows some
syntax elements from Lucene.

The engine turns a parsed query into an executable stream. Planning is cached:
a query hashes itself into a key (`internal/hash`), and an LRU of
`cache-capacity` entries holds the plans built for those keys, so a repeated
query — the common case for an agent in a loop — skips optimisation entirely.

The cache key is the whole query, built from lossless material: exact hex
floats, RFC3339Nano timestamps, `|`-delimited `tag=` segments with lists
NUL-delimited. Two queries that would produce different results must never hash
alike, which is why nothing in the key is derived from a lossy `String()`.

### Database

The database owns the graphs and translates a stream into calls on one of them.
It allocates `num-graphs` graphs at startup and never grows: a selector is an
index, not a lookup.

Reads and writes both arrive as streams, and the database does not distinguish
them — `Query.IsWrite()` is what the scheduler uses to decide the lock it takes.
Everything is in memory and nothing is persisted; a restart is an empty
database.

### Server

The HTTP surface is deliberately thin: three endpoints, and no behaviour of its
own beyond validation. Handlers bind the request, hand the query string to the
parser, refuse what the client got wrong, and wait for the stream to finish.
Anything that decides *what an answer is* belongs to the engine or the query
layer.

That thinness is what keeps error handling honest. A parse failure is a 400
carrying the parser's positioned message; a full queue is a 429; a shutdown in
progress is a 503; anything unrecognised is a 500 with a generic body and the
detail in the log, so an internal error never leaks through the wire. See
[`http-api.md`](http-api.md).

### Scheduler

The scheduler is a pool of `workers` goroutines fed by a bounded queue of
`buffer-size` streams. A worker takes a stream, locks the graph it names, runs
it, and unlocks — so writes serialise per graph while reads on other graphs
carry on untouched.

Bounded is the operative word. Submitting to a full queue waits at most
`enqueue-timeout` and then fails, which turns an overloaded server into 429s
instead of an unbounded pile of blocked handler goroutines. Shutdown is the
mirror image: the queue stops accepting, workers drain what is already in it,
and in-flight writes finish rather than being dropped.

The locking rules and the shutdown protocol are covered in
[`concurrency.md`](concurrency.md).

## References

* [`query-spec.md`](query-spec.md) — the query language: grammar, tokens, and
  what each clause means.
* [`concurrency.md`](concurrency.md) — sharding, the worker pool, and the
  per-graph locking strategy.
* [`configuration.md`](configuration.md) — every setting, its accepted values
  and its default.
* [`http-api.md`](http-api.md) — the HTTP surface and its status codes.
