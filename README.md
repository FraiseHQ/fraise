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
  <a href="https://codecov.io/gh/FraiseHQ/fraise" ><img src="https://codecov.io/gh/FraiseHQ/fraise/branch/main/graph/badge.svg?token=Y4T2AA3JBF"/></a>
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

Two verbs. One binary. No infrastructure to run.

<!-- demo GIF goes here -->

## Install

```sh
brew install fraisehq/tap/fraise
brew services start fraise
curl -X POST localhost:9876/api/v1/q -H 'content-type: application/json' \
  -d "{\"query\": \"remember 'the parrot is turquoise' topic:color\"}"
```

Linux packages, Docker, `go install` and signed release binaries are all in
[Get Started](#get-started) below.

> **Fraise is in-memory and ephemeral.** Memories live in the process and are
> gone when it stops — there is no snapshot and no load-on-boot yet.
> Persistence is [issue #171](https://github.com/FraiseHQ/fraise/issues/171)
> and is the next major piece of work. Build agents on it, don't put your only
> copy of anything in it.

## How it compares

Measured on [LoCoMo](https://github.com/snap-research/locomo) — 10 multi-session
conversations, 1,982 questions — with every system ingesting the same
conversations, using the same extraction model and the same embedding model, and
answering the same questions. `k=10`, full data, run 2026-08-30.

| system | retrieval recall | p50 search | memory tokens |
|---|---|---|---|
| **fraise 0.1.0** | **0.893** | **0.176 s** | **2,902,039** |
| everos 1.2.3 | 0.897 | 0.376 s | 10,030,966 |
| letta 0.16.8 | 0.897 | 0.349 s | 226,854 |
| mem0 2.0.18 | 0.886 | 0.576 s | 4,120,078 |
| graphiti 0.29.3 | 0.819 | 0.301 s | 9,866,183 |
| cognee 1.5.3 | 0.790 | 6.273 s | 8,526,019 |

**Fraise matches the best systems on recall, at 2× the speed and a third of the
tokens.** Same evidence found; half the latency; a fraction of the cost.

These come from a standalone multi-system harness — precision, recall and F1
across `k` ∈ {1, 3, 5, 10}, per category, every run tagged and reproducible from
the tag. The harness and the full results are published separately, in October.

## Why Fraise

- **A query language agents can actually write.** FQL has two verbs —
  `remember` and `recall` — and one way to say each thing. Fewer degrees of
  freedom means fewer ways for a model to get it wrong, and fewer tokens spent
  saying it.
- **Hybrid retrieval.** Facts are indexed for full-text, graph, and (optionally)
  vector search. One query, ranked across all three.
- **Temporal by default.** Recent memories outrank older ones, so recall is
  recency-aware without asking for it.
- **The fastest system measured.** 0.176 s p50 on LoCoMo, twice the next best.
  Remember and recall mid-step, while the user waits.
- **No infrastructure.** A single binary. No database to provision, no service
  to stand up beside it.
- **Open source, MIT.**

## Status

**v0.1.0 — the first stable release.** The core loop works end to end, the
install paths are verified on clean machines, and the benchmark row above is
produced from this tag.

Good for building agent memory today. Not yet for long term production use.

## How it works

Fraise stores knowledge as a **temporal memory graph** built from three kinds of
node:

- **facts** — the things you remember, one statement each
- **entities** — who or what a fact mentions
- **topics** — what a fact is about
Edges connect facts to the entities they mention and the topics they're about,
so a query can start from either side. A `recall` finds seed facts by text (and
optionally by vector similarity), expands through shared entities and topics up
to `depth` hops when it names a topic or entity, ranks by relevance and recency,
and returns the best `top` results.

Ranking is not a black box: a fact's score is its own match strength plus what
it receives through anchors carrying *more* mass than their size would predict.
The background rate that "more" is measured against is estimated per query, from
the part of the graph the query touched — so there is no relevance constant to
tune. [`docs/design.md`](./docs/design.md) has the full model.

A single Fraise instance holds several independent memory graphs (8 by default),
addressed with `@N` — one per user, per session, per agent, however you like.

## Get Started

Fraise is a single binary — no database to provision, nothing to configure.
Every route below leaves you with a server listening on `127.0.0.1:9876`.

### With Homebrew

```sh
brew install fraisehq/tap/fraise
brew services start fraise
```

The service survives crashes and restarts on login (`keep_alive`), logs to `$(brew --prefix)/var/log/fraise.log`, and reads its config from `$(brew --prefix)/etc/fraise/fraise.config.toml` — installed with every setting commented at its default, and never overwritten on upgrade.

### With Docker

```sh
docker run -p 127.0.0.1:9876:9876 ghcr.io/fraisehq/fraise:latest
```

Published tags: one per release, `latest` for the newest stable, `edge` for the
tip of `main`, and an immutable full-commit-SHA tag for every merge.

Images are built with SLSA provenance, verifiable without pulling:

```sh
gh attestation verify oci://ghcr.io/fraisehq/fraise:latest --repo FraiseHQ/fraise
```

### With Go

```sh
go install github.com/FraiseHQ/fraise/cmd/server@latest
"$(go env GOPATH)/bin/server"
```

The binary installs as `server`, after its package path — rename it to `fraise`
if that reads better.

Nothing further is needed to trust this: the Go toolchain checks every module
download against the public checksum transparency log, and your own machine
compiles the result.

### Linux packages

`.deb` and `.rpm` packages ship with every release, with a systemd user unit:

```sh
VERSION=0.1.0
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -sSfLO "https://github.com/FraiseHQ/fraise/releases/download/v${VERSION}/fraise_${VERSION}_${ARCH}.deb"
sudo dpkg -i "fraise_${VERSION}_${ARCH}.deb"
systemctl --user enable --now fraise
```

Logs go to the journal (`journalctl --user -u fraise -f`), and the unit reads `~/.config/fraise/fraise.config.toml` when present — a shipped default with every setting commented lives at `/etc/fraise/fraise.config.toml` to copy from. For agents that outlive your login session, let the user manager keep running: `loginctl enable-linger $USER`. A system-level (shared server) variant of the unit is described in [`docs/operations.md`](docs/operations.md).

### From a release binary

```sh
VERSION=0.1.0
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

Releases carry a [cosign](https://docs.sigstore.dev/) signature over
`checksums.txt`, using the same `VERSION` and `BASE` as above:

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

### Run from source

```sh
git clone https://github.com/FraiseHQ/fraise
cd fraise
make dev
```

### Your first memory

```sh
curl -X POST localhost:9876/api/v1/q \
  -H 'content-type: application/json' \
  -d '{"query":"remember \"the parrot is turquoise\" topic:color"}'

curl -X POST localhost:9876/api/v1/q \
  -H 'content-type: application/json' \
  -d '{"query":"recall parrot"}'
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

## Use it from an agent

### MCP

`fraise mcp` is a stdio MCP server — a thin bridge to a running daemon, so any MCP client can remember and recall. It exposes two tools, `recall` and `remember`, and needs no flags when the daemon is on its default address.

Start the daemon first (`brew services start fraise`, or `systemctl --user start fraise` on Linux), then register the bridge with whichever coding agent you use.

#### Claude Code

```sh
claude mcp add fraise -- fraise mcp
```

That registers it for the current project; add `--scope user` to make it available in every project instead.

#### Codex

```sh
codex mcp add fraise -- fraise mcp
```

Or write it into `~/.codex/config.toml` directly:

```toml
[mcp_servers.fraise]
command = "fraise"
args = ["mcp"]
```

#### OpenCode

Add it to `opencode.json` in your project root:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "fraise": {
      "type": "local",
      "command": ["fraise", "mcp"]
    }
  }
}
```

#### Any other client

```json
{
  "mcpServers": {
    "fraise": { "command": "fraise", "args": ["mcp"] }
  }
}
```

The bridge describes itself over MCP: each tool arrives with a description and a full JSON schema for its arguments and results — the FQL shapes, a worked example, what a score means — so a client knows what `recall` and `remember` do and how to call them without being told. What a tool description cannot carry is the policy: the habit of reaching for memory unprompted, and the judgement about what is worth keeping. That belongs in the file your agent already reads — `CLAUDE.md` for Claude Code, `AGENTS.md` for Codex and OpenCode — and two habits are enough: recall before answering anything that leans on earlier decisions or preferences, and remember only facts that will still matter in a later session, one self-contained fact per call with the topics and entities that will make it findable.

### SDKs

**Python** ([`sdk/python`](./sdk/python)) — the only SDK today:

```sh
pip install --pre fraise-sdk
```

```python
from fraise_sdk import FraiseClient

with FraiseClient("http://localhost:9876") as fraise:
    fraise.remember("the parrot is turquoise", topics=["color"])
    for hit in fraise.recall("parrot", top=5):
        print(hit.value, hit.score)
```

The Python SDK is dependency-light and supports vector search when you supply an
embedder. See [its README](./sdk/python) for embeddings and the full API.

**TypeScript** — not available yet
([#179](https://github.com/FraiseHQ/fraise/issues/179)). Until it lands,
TypeScript callers use `fraise mcp` or talk to the HTTP endpoint directly; it is
two verbs over one route, so a client is a short wrapper around `fetch`. See
[the HTTP API](./docs/http-api.md).

### Claude Agent SDK

The Python SDK ships memory tools for the [Claude Agent
SDK](./sdk/python#claude-agent-sdk-tools), exposed as an in-process MCP server
so the agent decides *what* to store and recall:

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

### OpenAI Agents SDK

`memory_tools(client)` returns bound `recall` and `remember` tools:

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

Complete, Docker-runnable agents live in
[`examples/openai-agents`](./examples/openai-agents).

## References

* [Roadmap](https://github.com/orgs/FraiseHQ/projects/1/views/1)
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

## Citing

If you use Fraise in academic work, see [CITATION.cff](./CITATION.cff).

## License

MIT — see [LICENSE](./LICENSE).
