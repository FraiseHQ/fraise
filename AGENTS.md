# AGENTS.md

Fraise is an in-memory temporal-memory-graph database for AI agents (Go server,
Python/TypeScript SDKs). Architecture background lives in `docs/design.md`,
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

### Go

- Tests: same-package when they need unexported fields; table-driven; the
  established pair for anything hash-like is (1) an exact-format pin test and
  (2) a "distinguishes" contract test proving variants don't collide. Reuse
  `fakeHasher`-style fakes; keep them per-package, tiny, deterministic.
- `gofmt` is mandatory; `make lint-go` runs golangci-lint.

### Python

- Test files are named `*_test.py` (not `test_*.py`), and the test tree mirrors
  the package it covers, so a module's tests sit at the matching path:

  ```text
  sdk/python/src/fraise_sdk/query.py                      → sdk/python/src/tests/query_test.py
  sdk/python/src/fraise_sdk/providers/openai.py           → sdk/python/src/tests/providers/openai_test.py
  sdk/python/src/fraise_sdk/integrations/openai_agents.py → sdk/python/src/tests/integrations/openai_agents_test.py
  ```

  The point is that the tests for a given module are findable from its path
  alone, without grepping. `tests/e2e/` is deliberately exempt: it tests the
  running server over HTTP, so it mirrors no package.
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

Nothing settled yet — the SDK (#45) has not landed. When it does, this section
should answer the same questions the Python one does: test file naming, where
tests live relative to the source, and the module/barrel-file rule. `biome`
handles formatting and linting (`make lint-ts`).

## Commands

- `make test` / `make test-go` — Go unit tests; `make coverage-go` for coverage.
- `make test-py` / `make test-ts` — SDK unit tests.
- `make test-e2e` — end-to-end suite; runs pytest locally against the fraise
  image brought up as a daemon via `docker-compose.yaml`.
- `make test-integration-py` / `make test-integration-ts` — SDK integration
  tests, same daemon, driven by a locally-run pytest.
- `make lint`, `make fmt`, `make build` — quality and build entry points.

When a change alters a contract (an interface method, hash material, a wire
format), update every implementor and every exact-format test in the same
change — the suite pins these on purpose, and a failing pin is information,
not an obstacle to sed away.

## Pull requests

Follow `CONTRIBUTING.md` for PR instructions: branch naming, commit message
format, and the review process.
