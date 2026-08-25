# Configuration

Fraise reads its configuration from a TOML file and from command-line flags.
Both name the same settings; flags win.

```bash
fraise -config /etc/fraise/fraise.config.toml -log-level error
```

## How a value is resolved

1. Every setting starts at its built-in default.
2. The config file is read (`fraise.config.toml` in the working directory unless
   `-config` says otherwise). A key that maps to no known setting is an error,
   not a typo the server ignores.
3. The command line is applied over the top.
4. Anything still at its zero value falls back to the default.
5. Settings with a fixed vocabulary are validated.

**A missing config file is not an error.** The defaults plus the flags are a
complete configuration, and containers routinely ship without a file — the
server logs that it fell back and starts.

**An unusable value is an error.** The server exits non-zero before it opens a
port, naming the setting and what it accepts:

```text
config: invalid value: db.precision = "floast32" (accepted: float32, float64)
```

Values from that fixed vocabulary are matched without regard to case, so
`-log-level error` and `-log-level ERROR` are the same request. Anything else
would silently substitute a value nobody asked for: the first version of this
server treated `-log-level error` as unrecognised and quietly logged at INFO,
and the only symptom was output the operator was trying to suppress.

## Settings

### `[log]`

| Setting             | Flag                     | Default | Accepts                       |
|---------------------|--------------------------|---------|-------------------------------|
| `level`             | `-log-level`             | `INFO`  | `DEBUG` `INFO` `WARN` `ERROR` |
| `format`            | `-log-format`            | `text`  | `text` `json`                 |
| `disable-timestamp` | `-log-disable-timestamp` | `true`  | boolean                       |

Logs go to stdout; file logging is not supported.

### `[server]`

| Setting               | Flag                   | Default | Meaning                                                |
|-----------------------|------------------------|---------|--------------------------------------------------------|
| `port`                | `-port`                | `9876`  | TCP port the HTTP API listens on                       |
| `read-timeout`        | `-read-timeout`        | `15s`   | max time to read a whole request                       |
| `read-header-timeout` | `-read-header-timeout` | `5s`    | max time to read headers alone (Slowloris)             |
| `write-timeout`       | `-write-timeout`       | `15s`   | max time to write a response                           |
| `idle-timeout`        | `-idle-timeout`        | `60s`   | max idle time on a kept-alive connection               |
| `shutdown-grace`      | `-shutdown-grace`      | `10s`   | how long shutdown waits for in-flight requests         |
| `max-body-bytes`      | `-max-body-bytes`      | `1 MiB` | request body cap, enforced before the body is buffered |

### `[scheduler]`

| Setting           | Flag               | Default                 | Meaning                                       |
|-------------------|--------------------|-------------------------|-----------------------------------------------|
| `workers`         | `-workers`         | `max(2, GOMAXPROCS(0))` | worker goroutines executing streams           |
| `buffer-size`     | `-buffer-size`     | `200`                   | queue depth                                   |
| `enqueue-timeout` | `-enqueue-timeout` | `2s`                    | how long a submit waits for room before a 429 |

See [`concurrency.md`](concurrency.md) for how these three interact.

### `[engine]`

| Setting                   | Flag                       | Default | Meaning                                     |
|---------------------------|----------------------------|---------|---------------------------------------------|
| `allow-unanchored-recall` | `-allow-unanchored-recall` | `false` | permit a recall with no entity/topic anchor |
| `half-life`               | `-half-life`               | `168h`  | time-decay half-life applied to fact scores |
| `cache-capacity`          | `-cache-capacity`          | `1000`  | size of the LRU of optimised query plans    |

A non-positive `half-life` disables decay.

### `[db]`

| Setting                | Flag                    | Default   | Meaning                                                   |
|------------------------|-------------------------|-----------|-----------------------------------------------------------|
| `precision`            | `-precision`            | `float32` | float width of embeddings and scores: `float32` `float64` |
| `num-graphs`           | `-num-graphs`           | `8`       | independent graphs allocated; selectors are `0..n-1`      |
| `default-top`          | `-default-top`          | `10`      | results returned when a recall omits `top:`               |
| `default-depth`        | `-default-depth`        | `0`       | retrieval lane when a recall omits `depth:` (0 = floor)   |
| `max-top`              | `-max-top`              | `1000`    | ceiling on `top:`, rejected at parse time past it         |
| `max-depth`            | `-max-depth`            | `2`       | ceiling on `depth:` (lanes are 0, 1 and 2)                |
| `max-vector-dimension` | `-max-vector-dimension` | `4096`    | ceiling on a bound vector's length                        |
| `seed-size`            | `-seed-size`            | `10`      | minimum candidate budget per source; widened to `top:`    |

`precision` is a compile-time type parameter: both instantiations are built into
the binary and this setting picks which one runs.

The three ceilings exist so one query cannot force unbounded work. They bound
what a *client* may ask for; the defaults above them are what it gets when it
asks for nothing.

### `[db.hashing-function]`

| Setting | Flag                     | Default  | Accepts         |
|---------|--------------------------|----------|-----------------|
| `name`  | `-hashing-function`      | `xxhash` | `xxhash` `t1ha` |
| `seed`  | `-hashing-function-seed` | `0`      | any `uint64`    |

This derives node keys from values. Changing either on a populated store means
the same fact hashes to a different key — since nothing is persisted, that only
matters across a restart with a changed config.

### `[db.search-algorithm]` / `[db.ranking-algorithm]` / `[db.scoring-algorithm]`

| Setting                               | Flag                 | Default  | Accepts                          |
|---------------------------------------|----------------------|----------|----------------------------------|
| `search-algorithm.name`               | `-search-algorithm`  | `excess` | `none` `bfs` `excess`            |
| `ranking-algorithm.name`              | `-ranking-algorithm` | `none`   | `none` `pagerank`                |
| `ranking-algorithm.pagerank-damping`  | `-pagerank-damping`  | `0.85`   | probability of following an edge |
| `ranking-algorithm.pagerank-max-iter` | `-pagerank-max-iter` | `100`    | power-iteration cap              |
| `ranking-algorithm.pagerank-tol`      | `-pagerank-tol`      | `1e-6`   | convergence threshold            |
| `scoring-algorithm.name`              | `-scoring-algorithm` | `excess` | `excess` `rrf`                   |
| `relevance-model.name`                | `-relevance-model`   | `bm25`   | `bm25` `matchcount`              |

`search-algorithm` selects the traversal moving seed evidence through the
graph: `excess` is the shipped excess-transmission strategy, `bfs` remains
available for comparison runs (tree-shaped incidence), and `none` turns the
graph channel off — text/vector search only. `scoring-algorithm` selects the
fold deriving relevance from the pooled evidence; `rrf` remains available for
comparison runs (its dampening constant is fixed at 60, the literature
standard — a comparison baseline is not tunable) but carries no null model,
which is why `excess` is the default (see `docs/design.md`). `relevance-model`
selects how the text index turns a match into a number: `bm25` (raw
idf-weighted mass × query coverage, what the excess methodology consumes) or
`matchcount` (one point per query-term occurrence, the pre-BM25 ranking, for
comparison runs). The pagerank knobs are
read only when `ranking-algorithm` is `pagerank`; `none` applies no global
ranking boost.

### `[db.vector-search]`

| Setting                | Flag                           | Default | Meaning                                             |
|------------------------|--------------------------------|---------|-----------------------------------------------------|
| `projection-dimension` | `-rptree-projection-dimension` | `128`   | dimension each RP-tree projects vectors down to     |
| `number-trees`         | `-rptree-n-trees`              | `16`    | trees in the vector index forest                    |
| `seed`                 | `-rptree-seed`                 | `4`     | seeds the random projections (deterministic builds) |
| `flush-factor`         | `-rptree-flush-factor`         | `2`     | entries per live vector before the forest rebuilds  |
| `leaf-size`            | `-rptree-leaf-size`            | `32`    | points a tree leaf holds before it splits           |
| `overfetch`            | `-rptree-overfetch`            | `32`    | candidates gathered per result before probing stops |

`flush-factor` is what bounds the forest at O(live vectors) under sustained
writes; `/api/v1/stats` reports `forest_entries` so the bound can be watched
rather than assumed.

`overfetch` is the knob to reach for first when vector recall is short. A search
gathers this many candidates per result asked for, scores all of them by true
distance and keeps the best: the projection decides only where to look, and the
factor is how much room the exact measure gets to overrule it. At 1 the result
is whatever the projection routed to; raised far enough it converges on an exact
scan, so it trades query time for recall continuously with no cliff at either
end. It is also the only one of these settings that shapes no index, so what it
costs is query time alone — the others are paid on every write and in resident
memory as well.

`number-trees` and `projection-dimension` shape the index itself. A single
random-projection tree is a weak approximator and the forest is what averages
that away, so recall rises steeply with `number-trees` while query cost and
memory rise linearly. `projection-dimension` widens the set of split directions
a tree can draw on, which matters most for deep trees that would otherwise reuse
the same few; it costs a little at query time and nothing at ingest.

`leaf-size` sets how fine the partition is: bigger leaves mean fewer, coarser
regions and more vectors scanned per probe, smaller leaves the reverse. It is
the least useful of the four to move, because `overfetch` reaches the same
candidate counts without rebuilding the index.

At a fixed latency budget, raising `overfetch` generally beats raising
`number-trees` — it reaches comparable recall without multiplying write cost or
resident memory. Reach for more trees when over-fetch alone has stopped paying.
All four defaults are chosen for recall, not for the fastest possible query.

## Example

```toml
[scheduler]
workers = 4
buffer-size = 128

[server]
port = 9876

[log]
level = "DEBUG"
format = "json"
disable-timestamp = true

[engine]
allow-unanchored-recall = true
half-life = "168h"
cache-capacity = 1024

[db]
precision = "float64"
default-top = 10
default-depth = 2
seed-size = 64

[db.hashing-function]
name = "xxhash"
```
