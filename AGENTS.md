# AGENTS.md

Fraise is an in-memory temporal-memory-graph database for AI agents (Go server,
Python SDK). Architecture background lives in `docs/design.md`,
`docs/concurrency.md`, and `docs/query-spec.md`. This file is about *how to add
code* here.

## Scope discipline

Do exactly what was asked — nothing more. If the request is a text or analysis
task (e.g. "write a PR description"), produce only that; do not edit code,
config, or files as a side effect. When the right move is unclear, or you're
tempted to go beyond the literal request, stop and ask instead of doing
something off-task.

## The rule: no glue code

New behavior must become an organic part of the codebase, expressed through the
concepts and interfaces that already exist — not bridged to them.

Concretely, when an addition doesn't fit an existing contract:

- **Do not** introduce adapter/bridge types, identity implementations, wrapper
  interfaces, or free-floating helper functions to make the mismatch go away.
- **Do** reshape the type so it genuinely satisfies the existing contract —
  usually by threading the required type parameters through, even when that
  ripples across many files. A wide mechanical refactor that conforms to the
  existing pattern is preferred over a small bolt-on that doesn't.
- Method sets follow the pattern of their peers. If existing types implement a
  contract a certain way, a new participant implements it the same way, with
  the same signature shape.

### Worked example (the canonical one)

Query types hash themselves for the engine's plan cache via the contract in
`internal/hash/hash.go`:

```go
type Hashable[K comparable, T any] interface {
    Hash(h Hasher[K, T]) K
}
```

`Recall[K, P]` and `Remember[K, P]` implement it by building delimited key
material and hashing once through the provided hasher. When their components
(`containers.Vector`, `containers.TimeValue`) also needed to contribute to the
cache key, the *wrong* answers were: a package-level serialization helper, a
`Hash() string` method outside the hasher contract, or an identity
`Hasher[string, string]` to bridge signatures. All of those are glue.

The right answer — what's in the tree now — was to give `Vector[K, P]` and
`TimeValue[K]` the type parameter and the same method shape as their peers:

```go
func (v Vector[K, P]) Hash(h hash.Hasher[K, string]) string
```

so composites pass their own hasher straight down (`r.Vector.Hash(h)`). That
meant threading `K` through the parser AST (`Parse[K, P]`,
`RecallCommandNode[K, P]`, `SinceFieldNode[K]`), graph, index, and trees — about
twenty files of mechanical updates. That cost is accepted; a kaleidoscope of
special cases is not.

## Existing concepts — extend these, don't invent parallels

- `internal/hash` — `Hasher[K, T]` / `Hashable[K, T]`: the only hashing
  contract. Anything that needs a cache key participates in it.
- `internal/containers` — value types (`Vector[K, P]`, `TimeValue[K]`,
  heaps, trees). Generic over the key/precision parameters of their users.
- `internal/query` — `Query[K, P]` interface; `Recall`/`Remember` composites;
  hash material is `|`-delimited `tag=` segments, lists NUL-delimited,
  lossless (exact hex floats, RFC3339Nano — never a lossy `String()`).
- `internal/query/parser` — lexer → AST → command nodes, generic in `[K, P]`.
  New query fields get an AST field node, a parser case, and a segment in the
  relevant query's `Hash`.
- `internal/graph`, `internal/index` — `Graph[K, P]`, `SearchIndex[K, V, P]`
  interfaces; in-memory implementations behind them.
- `pkg/server` → `pkg/engine` → `pkg/db`: HTTP handlers stay thin; behavior
  belongs in the engine/query layers. K is `~uint64` in production; tests may
  instantiate any comparable K (query tests use `string` with `fakeHasher`).

Generics discipline: `K comparable` (key), `P float32 | float64` (precision)
are the two axes everything is parameterized on. A new type that touches keys
or vectors takes them explicitly; no interface{}/any escape hatches.

## Conventions

These apply to every component:

- Every source file starts with the MIT license header (copy from a neighbor).
- Doc comments state the contract and the *why*, not the mechanics — e.g.
  `Hash` comments explain what must not collide and what breaks if it does.
- Formatting and linting are non-negotiable, whatever the language: `make lint`
  is the gate and it must be clean before a PR.
- Markdown prose is never hard-wrapped: one paragraph is one source line, however long — the same for a list item and for a blockquote line. Editors and renderers do the wrapping; hard breaks mid-paragraph make the file read as compacted and ragged, and turn every later edit into a rewrap diff. Code blocks, tables and the GitHub issue templates keep their own line structure. This applies to README.md, everything under docs/, and every other prose .md in the tree.

### Go

- Tests: same-package when they need unexported fields; table-driven; the
  established pair for anything hash-like is (1) an exact-format pin test and
  (2) a "distinguishes" contract test proving variants don't collide. Reuse
  `fakeHasher`-style fakes; keep them per-package, tiny, deterministic.
- **No compile-time interface checks** — never add
  `var _ SomeInterface = (*SomeType)(nil)`. In this project every implementor
  is assigned to its interface at a real wiring site (`NewGraph`, `db.Start`),
  and that assignment is the check: the build fails there if the method set
  drifts. A type assigned to its interface nowhere would be dead code, which
  is assumed never to exist here — delete the type, don't guard it.
- `gofmt` is mandatory; `make lint-go` runs golangci-lint.

### Python

- Test files are named `*_test.py` (not `test_*.py`), and the SDK suite mirrors the package one-to-one: a module maps to exactly one test file under the same relative path, and that file holds the module's unit tests and its integration tests together.

  ```text
  sdk/python/src/fraise_sdk/    → sdk/python/src/tests/

  client.py                     → client_test.py
  query.py                      → query_test.py
  providers/openai.py           → providers/openai_test.py
  integrations/openai_agents.py → integrations/openai_agents_test.py
  ```

  **A test file is named after the module under test, never after the scenario
  it exercises.** `connect_test.py`, `lifecycle_test.py`, `compatibility_test.py`,
  `vectors_test.py` are all wrong names for tests of `client.py`: each describes
  a concern, and a concern is a *section inside* the mirrored file, not a file of
  its own. Mirror the source module's own banner comments
  (`# -- lifecycle ---`, `# -- embedding ---`) to keep those sections findable.

  Integration tests live in the same mirrored file as the module's unit tests, under its `# -- integration` banner, and every one carries `@pytest.mark.integration`: `-m "not integration"` is the unit run and touches nothing live (`make test-py`), `-m integration` drives the daemon `make test-integration-py` brings up. The live fixtures they use sit in the same `conftest.py` as the mocked ones, under its "live server" banner — a fixture only instantiates when a test asks, so the unit run never waits on a health check.

  The point is that the tests for a given module are findable from its path
  alone, without grepping. Three consequences are intended, not accidents:

  - The same basename can recur across suite roots (an SDK test file and a
    server-suite file sharing a name). `--import-mode=importlib` in the root
    `pyproject.toml` is what stops pytest tripping over that. Never rename a
    file to dodge the collision.
  - A module with nothing to test at one level simply has no tests at that
    level: `providers/base.py` resolves embedders without touching the
    network, so it has unit tests and no `integration`-marked ones. A missing
    section is fine; a misnamed file is not.
  - `conftest.py` is fixture machinery and mirrors nothing.

  `tests/e2e/` and `tests/integration/` are exempt **from this naming rule
  only**: each drives the released binary and mirrors what it drives —
  `tests/e2e/` the HTTP server, `tests/integration/` the MCP bridge
  (`pkg/mcp`) together with the daemon behind it, both roles of the one
  binary. Every other rule in this section — fixtures, imports, docstrings,
  `parametrize`, mocking — applies to them in full.
- **Every fixture lives in `conftest.py`. This holds for every pytest suite in
  the repo — the SDK unit suite, the integration suite and `tests/e2e/`
  alike** — including a fixture that a single test file asks for, and including
  the seed data it is built from. A test module is assertions; a
  `@pytest.fixture` in one is setup hiding among them, and it splits "how did
  this graph get populated?" across as many files as there are suites. A
  `@pytest.fixture` outside a `conftest.py` is always wrong, however local it
  looks: "only this file uses it" is the reason it drifts, not an exemption.
- **Never import from a test module — a test tree is not a package.**
  `from conftest import NO_MATCH` is banned, and so is importing from a
  sibling `*_test.py`. Whether it resolves at all depends on how pytest put the
  directory on `sys.path`, and it re-introduces the hidden coupling that moving
  fixtures to `conftest.py` just removed. The only supported channel out of
  `conftest.py` is a fixture, injected through a test's arguments.

  This applies to plain values too, not just objects that need setup. Urls,
  graph ids, seed facts, dimensions and helper callables are all fixtures:

  ```python
  # conftest.py — the value is private; the fixture is the interface
  _NO_MATCH = "zzznomatchzzz"


  @pytest.fixture(scope="session")
  def no_match():
      """A keyword no fact in any graph contains."""
      return _NO_MATCH


  # client_test.py
  def test_recall_without_a_match_is_empty(client, round_trip_graph, no_match):
      """A keyword no fact contains yields an empty, falsey result."""
      assert client.recall(no_match, graph=round_trip_graph).count == 0
  ```

  A test that reads a long argument list is telling you what it depends on,
  which is the point.
- **Every test has a docstring.** One line is enough when the name already says
  it; say more when the test pins something non-obvious — why this graph, why
  this exact count, what breaks if the assertion flips. Same rule as everywhere
  else in the tree: state the contract and the *why*, not the mechanics. A test
  that documents a known defect says so in its docstring, with a `NOTE:` and
  what should replace it once the defect is fixed.
- **`parametrize` values are written inline, at the test that uses them.** A
  literal list at the decorator, never a module-level `CASES` dict fed in as
  `CASES.values()` with `ids=CASES.keys()` — that reads as indirection and you
  have to scroll to find out what a case even is. Skip `pytest.param` unless a
  case genuinely needs a marker; auto-generated ids are a fine price for seeing
  the values at the point of use. If two tests want the same case list, they
  are usually one parametrized test.

  ```python
  @pytest.mark.parametrize(
      "kwargs",
      [
          {"keywords": ["barometer"]},
          {"keywords": ["barometer"], "topics": ["weather"]},
          {"keywords": ["barometer"], "top": 3, "depth": 2},
      ],
  )
  def test_every_recall_the_builder_emits_parses(kwargs, client):
      """Every keyword-seeded shape build_recall can produce is accepted."""
  ```

- **No hand-rolled fakes — mock with `unittest.mock`, and only with
  `AsyncMock`, `MagicMock` and `patch`.** That import line is the whole
  toolbox:

  ```python
  from unittest.mock import AsyncMock, MagicMock, patch
  ```

  (import only the names a file actually uses — `ruff` fails on the rest).
  Nothing else from the module: no `create_autospec`, no `Mock`, no
  `mock_open`, no `sentinel`. And a test never defines a `_FakeClient` /
  `_FakeSession` / `_FakeResponse` class to impersonate a collaborator — a
  hand-written stand-in silently keeps passing after the API it imitates has
  changed shape, which is exactly the failure the test existed to catch.
  `MagicMock()` for a collaborator, `AsyncMock()` when the unit awaits it,
  `patch` when the unit constructs its collaborator itself rather than taking
  it as an argument (`patch("fraise_sdk.client.requests.Session")` keeps the
  client's own construction path under test).
- **A unit test mocks every call that leaves the unit.** Anything that reaches
  an external system — the HTTP session and the response it returns, a vendor
  SDK client (`openai`, `huggingface_hub`, `claude_agent_sdk`), the fraise
  server itself — is a `MagicMock`, so unit tests need no network and no
  daemon. Isolation is the goal, not purity: mocking a *deterministic*
  collaborator is fine and often right, because it pins what the unit under
  test asked for instead of testing the collaborator a second time. Mocking an
  embedder to return `[5.0]` is a better test of `recall_tool` than letting a
  real one compute it. In-tree value objects (`Hit`, `RecallResult`) are cheap
  and stable, so just build them.
- **Assert on the mock, not on a bookkeeping structure.**
  `assert_called_once_with`, `call_args.kwargs`, `assert_not_called`,
  `await_args` — never a bespoke `record` dict or `calls` list. Since the mocks
  accept any signature, the call assertion is what pins the contract: check the
  arguments, not just that something was called. (The Go `fakeHasher`
  convention above is unaffected — it is a real, deterministic implementation
  of an in-tree interface, not a stand-in for a collaborator.)
- **No code in `__init__.py` — imports are the only exception.** A package's
  `__init__.py` holds its docstring, re-export imports and `__all__`, nothing
  else. Classes, functions, constants and type aliases live in a named module
  (`providers/base.py`, not `providers/__init__.py`), which the package then
  re-exports. Code in `__init__.py` has no module to mirror under the test
  layout above, forces submodules into import cycles with their own package,
  and hides itself from anyone reading the tree.
- Docstrings follow the **Google** style — a one-line summary, then `Args:` /
  `Returns:` / `Raises:` sections, as in
  `sdk/python/src/fraise_sdk/integrations/openai_agents.py`. Not NumPy, not
  reST field lists. Enforced by `convention = "google"` in `pyproject.toml`.
- Optional integrations import their vendor SDK inside the function that needs
  it, never at module scope, so `import fraise_sdk` stays free of heavy extras.
- `ruff` formats and lints (`make lint-py`); `ty` type-checks.

### TypeScript

There is no TypeScript SDK, and nothing in the tree anticipates one: no `sdk/typescript/`, no `biome.json`, no `make *-ts` targets.
Don't add scaffolding for it ahead of the code. When the SDK does land, this
section should answer the same questions the Python one does — test file naming,
where tests live relative to the source, the module/barrel-file rule — and
`biome` is the intended formatter and linter.

## Commands

- `make test` / `make test-go` — Go unit tests; `make coverage-go` for coverage.
- `make test-py` — Python SDK unit tests.
- `make test-e2e` — end-to-end suite; runs pytest locally against the fraise
  image brought up as a daemon via `docker-compose.yaml`.
- `make test-integration-py` — Python SDK integration
  tests, same daemon, driven by a locally-run pytest.
- `make lint`, `make fmt`, `make build` — quality and build entry points.

When a change alters a contract (an interface method, hash material, a wire
format), update every implementor and every exact-format test in the same
change — the suite pins these on purpose, and a failing pin is information,
not an obstacle to sed away.

## Pull requests

Follow `CONTRIBUTING.md` for PR instructions: branch naming, commit message
format, and the review process.
