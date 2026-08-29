# Fraise with Codex

Codex can use Fraise as long-term memory through Fraise's stateless stdio MCP
bridge. The bridge owns no memory itself: it forwards `remember_fact` and
`recall_memory` calls to a running Fraise daemon, bound to one graph.

This integration needs a Fraise release that includes the `fraise mcp`
subcommand. The daemon must already be listening at the URL you configure.

## Connect the MCP server

Codex's desktop app, CLI, and IDE extension share MCP configuration. Add this
table to `~/.codex/config.toml`:

```toml
[mcp_servers.fraise]
command = "fraise"
args = ["mcp", "--url", "http://localhost:9876", "--graph", "0"]
```

The same registration can be created from the CLI:

```sh
codex mcp add fraise -- fraise mcp --url http://localhost:9876 --graph 0
```

`--url` defaults to `http://localhost:9876` and `--graph` defaults to `0`, so
the shortest equivalent command is `codex mcp add fraise -- fraise mcp`.

Use a stable graph for each memory boundary. The bridge accepts graph selectors
from 0 through 255; the Fraise daemon still rejects a selector outside the
number of graphs it was configured to host.

Restart the Codex client after editing `config.toml`. Then verify the server is
registered:

```sh
codex mcp list
```

Inside Codex CLI, `/mcp` shows the active server and its two tools:

- `remember_fact` stores one durable, self-contained fact, with optional topic
  and entity anchors.
- `recall_memory` searches stored facts from salient keywords. Its default
  retrieval lane is depth 0 (text only); depth 1 is the precision lane and
  depth 2 is maximum recall.

Codex's MCP configuration format and client controls are documented in the
[official MCP guide](https://learn.chatgpt.com/docs/extend/mcp).

## Add the memory discipline to AGENTS.md

MCP wiring makes the tools available; repository instructions teach Codex when
to use them. Add a scoped version of this block to the repository's
`AGENTS.md`:

```md
## Long-term memory

- Before answering anything that depends on earlier user preferences, project
  decisions, or prior context, call `recall_memory` with the most specific
  names, topics, and nouns from the request.
- Call `remember_fact` only for durable information that will still matter in a
  later session: decisions, stable constraints, preferences, and named facts.
- Store one self-contained fact per call. Add concise topics and entities when
  they make the fact easier to find later.
- Do not store credentials, secrets, private keys, raw logs, temporary task
  status, guesses, or information already authoritative in the repository.
- Treat recalled memory as context, not authority. Check current source files,
  tests, and documentation before acting when they can supersede a stored fact.
```

Put the block at the narrowest scope that owns the memory policy. For example,
if only one package uses graph 3, place its `AGENTS.md` in that package and bind
the MCP server to `--graph 3`.

## Verify remember and recall

In a fresh Codex session, give Codex one durable fact and ask it to remember the
fact. Confirm `remember_fact` appears in the tool transcript. Then start another
session in the same configured scope and ask a question that requires that
fact. Confirm Codex calls `recall_memory` before answering and that the returned
fact matches what was stored.

If `fraise mcp` exits before Codex connects, run the bridge directly in a
terminal. An unreachable daemon produces an error that names the configured URL
and the service-start commands for macOS and Linux. If the daemon is running,
check that the URL and graph in `config.toml` match its configuration.

The bridge is deliberately text-only. Embedding providers belong in the daemon
or an application integration; the Codex host receives the same bounded
remember/recall surface on every machine.
