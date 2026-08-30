# Operations

How to run Fraise, what to watch, and how it behaves when things go wrong. [`configuration.md`](configuration.md) is the reference for every setting; this is the runbook.

## The one thing to know first

**Fraise is in-memory and persists nothing.** A restart is an empty database — no snapshot, no write-ahead log, no recovery. Everything below follows from that:

* Treat a Fraise instance as a cache of memory an agent can rebuild, not as a system of record. If the facts matter beyond a process lifetime, something else has to own them.
* A rolling deploy is data loss, not a rollover. Two instances behind a load balancer do not share memory; a client that wrote to one and recalls from the other gets nothing.
* Capacity is memory. There is no spill to disk — the store grows until the process is killed for it.

## Running it

The image runs the binary directly as an unprivileged user and exposes 9876:

```bash
docker run --rm -p 9876:9876 \
  -v "$PWD/fraise.config.toml:/etc/fraise/fraise.config.toml:ro" \
  fraise -config /etc/fraise/fraise.config.toml
```

Or with compose, as the test suites do:

```yaml
services:
  fraise:
    build:
      context: .
      dockerfile: Dockerfile.fraise
    ports:
      - "9876:9876"
    volumes:
      - ./fraise.config.toml:/etc/fraise/fraise.config.toml:ro
    command: ["-config", "/etc/fraise/fraise.config.toml"]
```

A config file is optional — the defaults plus flags are a complete configuration. Mount one when you want to pin settings; use flags for the one or two you override per environment.

## Startup

The server exits non-zero, before binding a port, when a setting names something it cannot honour:

```text
config: invalid value: log.level = "verbose" (accepted: DEBUG, INFO, WARN, ERROR)
```

That is deliberate: exit 0 would look like a clean shutdown to a supervisor's restart policy, and starting anyway would run with a value nobody asked for. A *missing* config file is not fatal — it is logged and the defaults stand.

So a crash loop right after a deploy is worth reading the first log line for: if it names a setting, the config is wrong, and no amount of restarting will fix it.

## Health and readiness

`GET /` returns 200 with a status and the running version once the server is listening. It is a liveness and readiness check in one — there is nothing to warm up and nothing to recover, so a server that answers is a server that works.

```yaml
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://localhost:9876/"]
  interval: 10s
  timeout: 2s
  retries: 3
```

The `version` field is the SDKs' compatibility handshake; see `COMPATIBILITY.md`.

## What to watch

`GET /api/v1/stats` reports one entry per graph. See [`http-api.md`](http-api.md) for the full shape.

| Signal                                         | Why it matters                                                                                                                  |
|------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------|
| `nodes`, `order`, `size` growing without bound | Nothing evicts. Memory is the only limit, and this is the leading indicator.                                                    |
| `forest_entries` > `flush-factor × vectors`    | The vector index is not compacting. It should self-correct; if it does not, memory grows with the garbage rather than the data. |
| a graph far larger than its peers              | Partitioning is uneven — one tenant or session is carrying the store, and its writes serialise against each other.              |
| 429s in the access log                         | The scheduler queue is saturating. See below.                                                                                   |

There are no metrics endpoints yet; the log and `/stats` are the whole observability surface. Run with `format = "json"` if you ship logs anywhere that parses them.

## Load and backpressure

A 429 means the queue stayed full for `enqueue-timeout` and the request was shed. That is working as intended — the alternative is unbounded blocked goroutines — but sustained 429s mean the three scheduler settings are mismatched to the load:

* `workers` bounds how much executes at once. Raise it when workers are idle waiting on graph locks, not when they are busy.
* `buffer-size` bounds how much waits. Raising it absorbs bursts; it does not add throughput, and a deep queue only delays the 429 while raising latency.
* `enqueue-timeout` bounds how long a client waits before being told to back off. Keep it well under the caller's own timeout, or you will shed work the client has already given up on.

Writes to the *same* graph serialise. If one graph is hot, more workers will not help — spread the load across graphs, which is what they are for.

## Shutdown

SIGINT or SIGTERM starts a graceful shutdown: the HTTP server stops accepting, in-flight requests get up to `shutdown-grace` to finish, the scheduler refuses new work, and workers drain what they already accepted before exiting. Work that `Submit` accepted is work that runs — an acknowledged write is never dropped by shutdown.

`docker stop` sends SIGTERM, so the default path is the graceful one. Give the container a stop timeout longer than `shutdown-grace`, or the runtime's SIGKILL will cut the drain short.

Since nothing is persisted, a clean shutdown protects in-flight requests, not the data — the data is gone either way.

## Sizing

* **Memory** is the constraint. Facts, their index entries, and their vectors all live in the heap; a vector costs `dimension × 4` bytes at `float32` (`× 8` at `float64`) plus its entry in every RP-tree.
* **`num-graphs`** costs almost nothing empty. Allocate for the isolation you want, not the load you have.
* **`cache-capacity`** is a count of query plans, not bytes. The default 1000 is generous for agents that repeat a handful of query shapes.
