# Server-backed test suites

Two suites live here. Both drive a real fraise server brought up from
`docker-compose.yaml`, and both run their pytest locally against the published
port, so neither needs a test-runner image of its own.

| Suite | Path | What it tests | Command |
| --- | --- | --- | --- |
| End to end | `tests/e2e/` | the server itself, over raw HTTP | `make test-e2e` |
| Python SDK integration | `tests/integration/python/` | `fraise_sdk` against that server | `make test-integration-py` |

The unit suites are elsewhere and need no server: `make test-py` for the Python
SDK (`sdk/python/src/tests/`), `make test-go` for the server.

## Which suite does a test belong in?

- **Unit** if it needs no server. Collaborators are mocked with
  `unittest.mock`; see `AGENTS.md`.
- **Integration** if it exercises *the SDK's* behaviour against a live server —
  the query strings it generates, the JSON it parses, the exceptions it maps.
  These files mirror the package one-to-one (`fraise_sdk/client.py` →
  `tests/integration/python/client_test.py`).
- **End to end** if it exercises *the server's* behaviour and the SDK is beside
  the point. These files mirror the server's concerns, not a package, and send
  hand-written query strings over `requests`.

## Conventions

These apply to both suites here, and are stated in full in `AGENTS.md`:

- **Every fixture lives in `conftest.py`** — including one only a single file
  asks for. A test module should read as assertions, and there is exactly one
  place to look for how a graph got populated.
- **Never import from a test module.** A test tree is not a package, so
  `from conftest import NO_MATCH` — or an import from a sibling `*_test.py` —
  is banned. The only supported channel out of `conftest.py` is a fixture,
  injected through a test's arguments. That holds for plain values as much as
  for clients: urls, graph ids, seed facts and dimensions are all fixtures,
  backed by a private `_CONSTANT` inside `conftest.py`.
- **Every test has a docstring** saying what is being pinned and why, not what
  the code does.
- **`parametrize` values are written inline**, as literals, at the test that
  uses them — never hoisted into a shared module-level table.
- Test files are named `*_test.py`, never `test_*.py`.

## Graph allocation

Both suites pin their writes to specific graphs so result counts stay
deterministic, including across reruns against a long-lived server — a fact is
keyed by its value, so rewrites are idempotent. Files run in any order, so tests
sharing a graph must not depend on each other's facts.

Each suite keeps its own map in its `conftest.py`; update it when claiming a
graph. The store allocates 8 graphs by default, so valid selectors are 0..7.

## Running against a server you already have

Both suites honour `FRAISE_URL` (default `http://localhost:9876`), so they can
be pointed at any running instance without docker:

```sh
FRAISE_URL=http://localhost:9876 uv run --package fraise-sdk pytest tests/integration/python
FRAISE_URL=http://localhost:9876 uv run --package tests pytest tests/e2e
```

Override the host port docker binds with `FRAISE_E2E_PORT` if 9876 is taken.
