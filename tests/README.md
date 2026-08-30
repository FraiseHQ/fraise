# Server-backed test suites

Two suites live here, and both drive a real fraise server — but they get it differently. `tests/e2e/` talks HTTP to the container `docker-compose.yaml` brings up; `tests/integration/` builds the binary and runs it itself, as the daemon *and* as the `fraise mcp` stdio bridge, so it needs Go rather than docker.

| Suite | Path | What it tests | Command |
| --- | --- | --- | --- |
| End to end | `tests/e2e/` | the server itself, over raw HTTP | `make test-e2e` |
| Server + MCP bridge | `tests/integration/` | `pkg/mcp` over stdio, against the daemon behind it | `make test-integration` |

The SDK's own tests are not here. They live beside the package in `sdk/python/src/tests/`, one file per module, holding that module's unit tests and its integration tests together — the latter marked `integration`. `make test-py` runs the unit half (`-m "not integration"`, no server); `make test-integration-py` runs the marked half against the compose daemon. `make test-go` covers the server's unit tests.

## Which suite does a test belong in?

- **SDK unit** if it needs no server: it goes in the mirrored file under `sdk/python/src/tests/`, unmarked. Collaborators are mocked with `unittest.mock`; see `AGENTS.md`.
- **SDK integration** if it exercises *the SDK's* behaviour against a live server — the query strings it generates, the JSON it parses, the exceptions it maps. Same mirrored file, under its `# -- integration` banner, carrying `@pytest.mark.integration`.
- **Server + MCP bridge** (`tests/integration/`) if it exercises what an MCP client sees: the handshake, the tool schemas, a `tools/call` round trip. These mirror `pkg/mcp`, not the SDK, and speak JSON-RPC over the bridge's stdin/stdout.
- **End to end** (`tests/e2e/`) if it exercises *the server's* behaviour and neither SDK nor bridge is the point. These mirror the server's concerns and send hand-written query strings over `requests`.

## Conventions

These apply to every suite here, and are stated in full in `AGENTS.md`:

- **Every fixture lives in `conftest.py`** — including one only a single file asks for. A test module should read as assertions, and there is exactly one place to look for how a graph got populated.
- **Never import from a test module.** A test tree is not a package, so `from conftest import NO_MATCH` — or an import from a sibling `*_test.py` — is banned. The only supported channel out of `conftest.py` is a fixture, injected through a test's arguments. That holds for plain values as much as for clients: urls, graph ids, seed facts and dimensions are all fixtures, backed by a private `_CONSTANT` inside `conftest.py`.
- **Every test has a docstring** saying what is being pinned and why, not what the code does.
- **`parametrize` values are written inline**, as literals, at the test that uses them — never hoisted into a shared module-level table.
- Test files are named `*_test.py`, never `test_*.py`.

## Graph allocation

`tests/e2e/` pins its writes to specific graphs so result counts stay deterministic, including across reruns against a long-lived server — a fact is keyed by its value, so rewrites are idempotent. Files run in any order, so tests sharing a graph must not depend on each other's facts. The map lives in that suite's `conftest.py`; update it when claiming a graph. The store allocates 8 graphs by default, so valid selectors are 0..7.

`tests/integration/` needs no such map: it starts a fresh daemon on a free port for the session, so the store begins empty every run.

## Running against a server you already have

`tests/e2e/` and the SDK's integration half honour `FRAISE_URL` (default `http://localhost:9876`), so they can be pointed at any running instance without docker:

```sh
FRAISE_URL=http://localhost:9876 uv run --package tests pytest tests/e2e
FRAISE_URL=http://localhost:9876 uv run --package fraise-sdk pytest sdk/python/src/tests -m integration
```

Override the host port docker binds with `FRAISE_E2E_PORT` if 9876 is taken — and set `FRAISE_URL` to match, since it does not follow automatically.

`tests/integration/` owns its processes and ignores `FRAISE_URL`; point it at a binary you already built with `FRAISE_BIN=/path/to/fraise` (what `make test-integration` does) or let it `go build` one itself.
