<p align="center">
  <img height="300px" style="height:300px;" src="assets/logo.png">
</p>

# Fraise

<p align="center">
  <a href="https://github.com/RonsenbergVI/fraise/actions/workflows/go.yaml"><img src="https://github.com/RonsenbergVI/fraise/actions/workflows/go.yaml/badge.svg" alt="CI"></a>
  <a href="https://codecov.io/gh/RonsenbergVI/fraise"><img src="https://codecov.io/gh/RonsenbergVI/fraise/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="https://github.com/RonsenbergVI/fraise/releases/latest"><img src="https://img.shields.io/github/v/release/RonsenbergVI/fraise?sort=semver" alt="Release"></a>
  <a href="https://pkg.go.dev/github.com/RonsenbergVI/fraise"><img src="https://pkg.go.dev/badge/github.com/RonsenbergVI/fraise.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/RonsenbergVI/fraise"><img src="https://goreportcard.com/badge/github.com/RonsenbergVI/fraise" alt="Go Report Card"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
  <a href="https://discord.gg/eHDFwnwHq"><img src="https://img.shields.io/discord/1523303330326253759?logo=discord&logoColor=white&label=discord&color=5865F2" alt="Discord"></a>
</p>

**Fraise is a memory database for AI agents.** One they query directly, in a
language built for tokens, not humans.

```text
remember "acme moved to annual billing" topic:billing entity:acme

recall billing entity:acme since:30d top:5
```

Two verbs. Sub-millisecond recall. No infrastructure to run.

<!-- demo GIF goes here -->

## Why Fraise

- **A query language agents can actually write.** FQL has two verbs — `remember`
  and `recall` — and one way to say each thing. Fewer degrees of freedom means
  fewer ways for a model to get it wrong, and fewer tokens spent saying it.
- **Hybrid retrieval.** Facts are indexed for full-text, graph, and (optionally)
  vector search. One query, ranked across all three.
- **Temporal by default.** Recent memories outrank older ones, so recall is
  recency-aware without asking for it.
- **Fast enough to sit inside a turn.** Recall in tens of microseconds, writes in
  low milliseconds — remember mid-step, while the user waits.
- **No infrastructure.** A single binary. No database to provision, no service to
  stand up beside it.
- **Open source, MIT.**

## Status

Fraise is early and pre-`v0.1.0`. It runs, and the core loop works end to end —
but the API and the query language may still change between minor versions.

| | |
|---|---|
| Storage, `remember` / `recall`, graph traversal | working |
| Full-text search, ranking, `top` / `depth` / time filters | working |
| Python + TypeScript SDKs | in progress |
| Vector / semantic search | in progress |
| BM25 ranking, ANN index | planned for `v0.2.0` |
| Persistence | not yet — Fraise is in-memory only |

Not production-ready. Good for experimenting with agent memory today.

## How it works

Fraise stores knowledge as a **temporal memory graph** built from three kinds of
node:

- **facts** — the things you remember, one statement each
- **entities** — who or what a fact mentions
- **topics** — what a fact is about

Edges connect facts to the entities they mention and the topics they're about, so
a query can start from either side. A `recall` finds seed facts by text (and
optionally by vector similarity), expands through shared entities and topics up to
`depth` hops, ranks by relevance and recency, and returns the best `top` results.

A single Fraise instance holds several independent memory graphs (8 by default),
addressed with `@N` — one per user, per session, per agent, however you like.

## The query language

```text
remember "acme moved to annual billing" topic:billing entity:acme
remember@2 "she prefers morning meetings" topic:scheduling entity:anna

recall billing                        # free-text search
recall topic:billing entity:acme      # anchored to a topic and an entity
recall billing since:30d top:5        # last 30 days, best 5
recall@2 +topic:auth -entity:okta     # require auth, exclude okta
recall onboarding depth:3             # follow the graph 3 hops out
```

| | |
|---|---|
| `topic:` `entity:` | anchor the search to a topic or an entity |
| `+` `-` `~` | require / exclude / loosen an anchor |
| `since:` `until:` | time window (`7d`, `2h`, or `2026-01-15`) |
| `top:` | how many results (default 10) |
| `depth:` | how far to walk the graph (default 2) |
| `@N` | which memory graph |

Full grammar: [query language spec](./docs/query-spec.md).

## Get Started

### Run with Docker

```sh
docker run -p 9876:9876 ghcr.io/ronsenbergvi/fraise:latest
```

### Run from source

```sh
git clone https://github.com/RonsenbergVI/fraise
cd fraise
make run
```

### Your first memory

```sh
curl -X POST localhost:9876/api/v1/q \
  -H 'content-type: application/json' \
  -d '{"query": "remember \"the parrot is turquoise\" topic:color"}'

curl -X POST localhost:9876/api/v1/q \
  -H 'content-type: application/json' \
  -d '{"query": "recall parrot"}'
```

```json
{
  "results": {
    "count": 1,
    "hits": [
      { "value": "the parrot is turquoise", "timestamp": "...", "score": 1 }
    ]
  }
}
```

### SDKs

```sh
pip install fraise-sdk        # Python
npm install fraise-sdk        # TypeScript
```

### Integrate with Claude Agents

_Coming soon._

### Integrate with OpenAI Agents

_Coming soon._

## References

* [Roadmap](https://github.com/users/RonsenbergVI/projects/2)
* [Database design](./docs/design.md)
* [Query language spec](./docs/query-spec.md)
* [Release process](./RELEASE.md)
* [Issues](https://github.com/RonsenbergVI/fraise/issues)

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](./CONTRIBUTING.md) for how to
build, test, and submit changes.

## Code of Conduct

This project follows the [Contributor Covenant](./CODE_OF_CONDUCT.md).

## Community

Questions, ideas, or building something with Fraise? Join the
[Discord](https://discord.gg/eHDFwnwHq). Bugs and feature requests belong in
[issues](https://github.com/RonsenbergVI/fraise/issues) so they don't get lost.

## License

MIT — see [LICENSE](./LICENSE).
