<p align="center">
  <img height="300px" style="height:300px;" src="assets/logo.png">
</p>

# Fraise

<p align="center">
  <a href="https://fraisehq.github.io/fraise">Docs</a>
  ·
  <a href="https://discord.gg/eHDFwnwHq">Discord</a>
  ·
  <a href="./docs/query-spec.md">Query language</a>
  ·
  <a href="https://github.com/FraiseHQ/fraise/issues">Issues</a>
</p>

<p align="center">
  <a href="https://github.com/FraiseHQ/fraise/actions/workflows/go.yaml"><img src="https://github.com/FraiseHQ/fraise/actions/workflows/go.yaml/badge.svg" alt="CI"></a>
  <a href="https://github.com/FraiseHQ/fraise/actions/workflows/python.yaml"><img src="https://github.com/FraiseHQ/fraise/actions/workflows/python.yaml/badge.svg" alt="Python SDK"></a>
  <a href="https://codecov.io/gh/FraiseHQ/fraise"><img src="https://codecov.io/gh/FraiseHQ/fraise/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/FraiseHQ/fraise"><img src="https://api.scorecard.dev/projects/github.com/FraiseHQ/fraise/badge" alt="OpenSSF Scorecard"></a>
  <a href="https://github.com/FraiseHQ/fraise/releases/latest"><img src="https://img.shields.io/github/v/release/FraiseHQ/fraise?sort=semver" alt="Release"></a>
  <a href="https://pkg.go.dev/github.com/FraiseHQ/fraise"><img src="https://pkg.go.dev/badge/github.com/FraiseHQ/fraise.svg" alt="Go Reference"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
  <a href="https://discord.gg/eHDFwnwHq"><img src="https://img.shields.io/discord/1523303330326253759?logo=discord&logoColor=white&label=discord&color=5865F2" alt="Discord"></a>
</p>

**Fraise is a memory database for AI agents.** One they query directly, in a
language built for tokens, not humans.

```text
remember 'acme moved to annual billing' topic:billing entity:acme

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

## Get Started

Fraise is a single binary — no database to provision, nothing to configure. Every
route below leaves you with a server listening on `127.0.0.1:9876`.

> Fraise is pre-`v0.1.0`, so every published version is a pre-release. Two
> consequences for the commands below: Docker's `:latest` tag and GitHub's
> `/releases/latest/` URL don't resolve yet, so each one pins a version.

### With Go

```sh
go install github.com/FraiseHQ/fraise/cmd/server@latest
"$(go env GOPATH)/bin/server"
```

`@latest` resolves to the highest pre-release (`v0.1.0-beta.2` today); pin with
`@v0.1.0-beta.2` to be explicit. The binary installs as `server`, after its
package path — rename it to `fraise` if that reads better.

Nothing further is needed to trust this: the Go toolchain checks every module
download against the public checksum transparency log, and your own machine
compiles the result.

### From a release binary

```sh
VERSION=0.1.0-beta.2
OS=$(uname -s | tr '[:upper:]' '[:lower:]')                # linux | darwin
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')  # amd64 | arm64
ASSET="fraise_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/FraiseHQ/fraise/releases/download/v${VERSION}"

curl -sSfLO "${BASE}/${ASSET}"
tar xzf "$ASSET"
./fraise
```

Windows builds ship as `.zip` under the same naming scheme.

### Verify a release

Releases after `v0.1.0-beta.2` carry a [cosign](https://docs.sigstore.dev/)
signature over `checksums.txt`, using the same `VERSION` and `BASE` as above:

```sh
curl -sSfLO "${BASE}/checksums.txt"
curl -sSfLO "${BASE}/checksums.txt.sigstore.json"

# 1. the bundle proves checksums.txt came from this repo's release workflow
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/FraiseHQ/fraise/.github/workflows/go.yaml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. checksums.txt proves your archive is the one it covers
sha256sum --ignore-missing -c checksums.txt   # macOS: shasum -a 256 --ignore-missing -c
```

### With Docker

```sh
docker run -p 127.0.0.1:9876:9876 ghcr.io/fraisehq/fraise:0.1.0-beta.2
```

Published tags: one per release (`0.1.0-beta.2`), `edge` for the tip of `main`,
and an immutable full-commit-SHA tag for every merge. `latest` starts appearing
at the first stable release.

Images are built with SLSA provenance, verifiable without pulling:

```sh
gh attestation verify oci://ghcr.io/fraisehq/fraise:0.1.0-beta.2 \
  --repo FraiseHQ/fraise
```

### Run from source

```sh
git clone https://github.com/FraiseHQ/fraise
cd fraise
make run
```

### Your first memory

```sh
curl -X POST localhost:9876/api/v1/q \
  -H 'content-type: application/json' \
  -d '{"query": "remember 'the parrot is turquoise' topic:color"}'

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

Prefer to talk to Fraise from your own code? Official clients wrap the query
endpoint behind two verbs — `remember` and `recall` — with optional vector
embeddings and agent-framework tools.

**Python** ([`sdk/python`](./sdk/python)):

```sh
pip install fraise-sdk
```

```python
from fraise_sdk import FraiseClient

with FraiseClient("http://localhost:9876") as fraise:
    fraise.remember("the parrot is turquoise", topics=["color"])
    for hit in fraise.recall("parrot", top=5):
        print(hit.value, hit.score)
```

**TypeScript** ([`sdk/typescript`](./sdk/typescript)):

```sh
npm install fraise-sdk
```

```ts
import { FraiseClient } from "fraise-sdk";

const fraise = new FraiseClient({ baseUrl: "http://localhost:9876" });
await fraise.remember("the parrot is turquoise", { topics: ["color"] });
const result = await fraise.recall(["parrot"], { top: 5 });
for (const hit of result.hits) console.log(hit.value, hit.score);
```

Both are dependency-light and support vector search when you supply an embedder.
See each SDK's README for embeddings and the full API.

### Integrate with Claude Agents

The Python SDK ships memory tools for the [Claude Agent
SDK](./sdk/python#claude-agent-sdk-tools), exposed as an in-process MCP server so
the agent decides *what* to store and recall:

```python
from claude_agent_sdk import ClaudeAgentOptions
from fraise_sdk import FraiseClient
from fraise_sdk.integrations.claude_agents import memory_server, allowed_tools

fraise = FraiseClient("http://localhost:9876")
options = ClaudeAgentOptions(
    system_prompt="Remember durable facts the user shares, and recall them when relevant.",
    mcp_servers={"fraise_memory": memory_server(fraise)},
    allowed_tools=allowed_tools(),
)
```

A complete, Docker-runnable agent lives in
[`examples/claude-agent-sdk`](./examples/claude-agent-sdk).

### Integrate with OpenAI Agents

Both SDKs ship tools for the [OpenAI Agents
SDK](./sdk/python#openai-agents-tools). `memory_tools(client)` returns bound
`recall` and `remember` tools:

```python
from agents import Agent, Runner
from fraise_sdk import FraiseClient
from fraise_sdk.integrations.openai_agents import memory_tools

fraise = FraiseClient("http://localhost:9876")
agent = Agent(
    name="Assistant",
    instructions="Remember durable facts the user shares, and recall them when relevant.",
    tools=memory_tools(fraise),
)

result = Runner.run_sync(agent, "My favourite colour is orange.")
print(result.final_output)
```

The TypeScript equivalent is `memoryTools(client)` from
`fraise-sdk/integrations/openai-agents`. Complete, Docker-runnable agents live in
[`examples/openai-agents`](./examples/openai-agents).

## References

* [Roadmap](https://github.com/users/RonsenbergVI/projects/2)
* [Database design](./docs/design.md)
* [Query language spec](./docs/query-spec.md)
* [Release process](./RELEASE.md)
* [Issues](https://github.com/FraiseHQ/fraise/issues)

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](./CONTRIBUTING.md) for how to
build, test, and submit changes.

## Code of Conduct

This project follows the [Contributor Covenant](./CODE_OF_CONDUCT.md).

## Community

Questions, ideas, or building something with Fraise? Join the
[Discord](https://discord.gg/eHDFwnwHq). Bugs and feature requests belong in
[issues](https://github.com/FraiseHQ/fraise/issues) so they don't get lost.

## License

MIT — see [LICENSE](./LICENSE).
