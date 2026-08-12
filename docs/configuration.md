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

| Setting           | Flag               | Default | Meaning                                       |
|-------------------|--------------------|---------|-----------------------------------------------|
| `workers`         | `-workers`         | `2`     | worker goroutines executing streams           |
| `buffer-size`     | `-buffer-size`     | `200`   | queue depth                                   |
| `enqueue-timeout` | `-enqueue-timeout` | `2s`    | how long a submit waits for room before a 429 |

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
| `default-depth`        | `-default-depth`        | `2`       | hops walked when a recall omits `depth:`                  |
| `max-top`              | `-max-top`              | `1000`    | ceiling on `top:`, rejected at parse time past it         |
| `max-depth`            | `-max-depth`            | `6`       | ceiling on `depth:`                                       |
| `max-vector-dimension` | `-max-vector-dimension` | `4096`    | ceiling on a bound vector's length                        |
| `seed-size`            | `-seed-size`            | `10`      | seeds pulled from each source (text, vector)              |

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

### `[db.search-algorithm]` / `[db.ranking-algorithm]`

| Setting                               | Flag                 | Default | Accepts                          |
|---------------------------------------|----------------------|---------|----------------------------------|
| `search-algorithm.name`               | `-search-algorithm`  | `none`  | `none` `bfs`                     |
| `ranking-algorithm.name`              | `-ranking-algorithm` | `none`  | `none` `pagerank`                |
| `ranking-algorithm.pagerank-damping`  | `-pagerank-damping`  | `0.85`  | probability of following an edge |
| `ranking-algorithm.pagerank-max-iter` | `-pagerank-max-iter` | `100`   | power-iteration cap              |
| `ranking-algorithm.pagerank-tol`      | `-pagerank-tol`      | `1e-6`  | convergence threshold            |

`none` leaves the graph on its built-in walk and applies no global ranking
boost. The pagerank knobs are read only when `ranking-algorithm` is `pagerank`.

### `[db.vector-search]`

| Setting                | Flag                           | Default | Meaning                                             |
|------------------------|--------------------------------|---------|-----------------------------------------------------|
| `projection-dimension` | `-rptree-projection-dimension` | `8`     | dimension each RP-tree projects vectors down to     |
| `number-trees`         | `-rptree-n-trees`              | `4`     | trees in the vector index forest                    |
| `seed`                 | `-rptree-seed`                 | `4`     | seeds the random projections (deterministic builds) |
| `flush-factor`         | `-rptree-flush-factor`         | `2`     | entries per live vector before the forest rebuilds  |

`flush-factor` is what bounds the forest at O(live vectors) under sustained
writes; `/api/v1/stats` reports `forest_entries` so the bound can be watched
rather than assumed.

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
default-depth = 3
seed-size = 64

[db.hashing-function]
name = "xxhash"
```
