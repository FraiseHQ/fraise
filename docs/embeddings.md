# Embeddings

Semantic search is optional in Fraise, and the server never computes a vector.
It stores what it is given, indexes it, and searches it. Deciding what text
means is the caller's job.

That split is deliberate. Embedding models change often, cost money, and differ
per deployment; a database that picked one would be wrong for most callers and
would tie a memory store's release cycle to a model vendor's.

## The path a vector takes

```text
text ──► embedder (SDK, your process) ──► [0.10, 0.22, ...]
                                              │
                                              ▼
                                   parameters: {"v": [...]}
                                              │
    query: remember 'the parrot is turquoise' topic:birds vec:$v
                                              │
                                              ▼
                             graph N: fact node + vector index entry
```

A vector reaches the server as a request **parameter**, bound to a `vec:$v`
reference in the query — never inline in the query text. The query string is a
cache key and a log line; a few thousand floats spliced into it would ruin both
and put the lexer to work for nothing. See [`http-api.md`](http-api.md) for the
request shape.

`vec:` is valid on both commands:

* on `remember`, the vector is the fact's embedding, stored and indexed with it
* on `recall`, it is a semantic seed — the vector index returns its nearest
  neighbours, which then seed the graph walk

## Producing the vector

The Python SDK ships embedders, or takes any callable:

```python
from fraise_sdk import FraiseClient
from fraise_sdk.providers import OpenAIEmbedder

client = FraiseClient("http://localhost:9876", embedder=OpenAIEmbedder())
client.remember("the parrot is turquoise", topics=["birds"])   # encoded for you
client.recall("colourful birds")                               # encoded for you
```

* `OpenAIEmbedder` — `text-embedding-3-small` by default, and its `dimensions`
  parameter can be set to match a graph's existing dimension.
* `HuggingFaceEmbedder` — `sentence-transformers/all-MiniLM-L6-v2` by default.
  Sentence-level models only: a model emitting one vector per token has nothing
  to store.
* Anything else — an `Embedder` subclass, or a bare `callable(text) -> Sequence[float]`.

A client without an embedder still works; it simply writes and recalls without
vectors, on text and graph structure alone. Per call, `embed=True` forces
encoding and `embed=False` skips it, and an explicit `vector=` always wins.

Both vendor SDKs are optional extras, imported inside the function that needs
them, so `import fraise_sdk` stays cheap for callers who use neither.

## Dimension is per graph, and fixed by the first write

The first vector inserted into a graph establishes that graph's dimensionality.
Every later vector in that graph must match it exactly; one that does not is
rejected rather than padded, truncated, or silently stored.

This is the rule that most often surprises. Practically:

* **One model per graph.** Switching embedding models means a new graph, not a
  migration — nothing is persisted, so "migrating" is re-remembering the facts.
* **The dimension is not configured**, it is observed. The server logs it when
  it is established, and `/api/v1/stats` reports how many vectors a graph holds.

A mismatch is a client error on both commands, answered with a **400** naming
the dimension the graph expects and the one the request carried:

* **`remember`** rejects the write. The fact is not stored, with or without its
  vector — a half-written fact indexed only by text would be worse than none.
* **`recall`** rejects the query rather than dropping the vector. A recall that
  quietly fell back to keyword search would return plausible results with no
  semantic input at all, and nothing in the response would say so.

That symmetry is the point: a dimension mismatch means the caller is using a
different embedding model than the graph was built with, and there is no useful
answer to give it.

`max-vector-dimension` (4096 by default) caps what any single request may bind,
rejecting an oversized vector at parse time — before any index work — so one
request cannot decide how much memory an insert or a search costs.

## Precision

`db.precision` chooses the float width for the whole store, `float32` (default)
or `float64`. It is a compile-time type parameter: both instantiations are built
into the binary and the setting picks which runs.

`float32` halves the memory of every vector and score at a precision that
embedding models comfortably exceed in noise. Choose `float64` only with a
reason to.

## How vectors are searched

Each graph indexes its vectors in a forest of random-projection trees:
`rptree-n-trees` trees, each projecting onto `rptree-projection-dimension`
random directions. A search descends every tree and unions the candidates —
approximate, cheap, and good enough to seed a graph walk that will rank the
results anyway.

Deletes and overwrites leave garbage behind rather than restructuring the trees,
so the forest is rebuilt from the live set once it exceeds
`rptree-flush-factor` entries per live vector. That bounds it at O(live vectors)
under sustained writes; `forest_entries` in `/api/v1/stats` is how you watch it.

`rptree-seed` fixes the random projections, so an identical sequence of writes
produces an identical index — which is what makes vector behaviour reproducible
between runs.

## Vectors are a seed, not an answer

A recall still needs at least one term. The vector index contributes
`seed-size` candidates, the text index contributes its own, and the graph walk
expands from both — attenuating by `hop-attenuation` per hop, then decaying by
age. Semantic similarity is one signal feeding a ranking, never the ranking
itself.
