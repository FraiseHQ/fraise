# Contributing to Fraise

Thanks for being here. Contributions of any size are welcome: a typo fix, a bug report, a question that reveals the docs are unclear. All of it helps.

**If you're unsure about anything, ask.** Open a [Discussion](https://github.com/FraiseHQ/fraise/discussions) and someone will help you figure it out. You don't need to know the answer before you ask the question, and no question here is too basic.

## Ways to help

You don't have to write code to contribute:

- **Report a bug** — [open an issue](https://github.com/FraiseHQ/fraise/issues/new/choose). Even a partial report is useful; we'll ask if we need more.
- **Improve the docs** — if something confused you, it will confuse the next person. Fixing that is one of the most valuable things you can do.
- **Answer a question** in Discussions.
- **Try a release candidate** and tell us what broke.
- **Share what you built** with it. Seriously — it helps us understand what the project is actually for.

Looking for somewhere to start? Issues labelled [`good first issue`](https://github.com/FraiseHQ/fraise/labels/good%20first%20issue) are scoped small and come with enough context to get going. If one looks interesting but the description is thin, comment and ask — that's not a bother, it's a signal the issue needs a better description.

## How the repo is laid out

Fraise is a monorepo with two released components, each versioned and released on its own clock:

| Component      | Lives in                          | Language      |
| -------------- | --------------------------------- | ------------- |
| Server         | `cmd/`, `internal/`, `pkg/`       | Go (1.26)     |
| Python SDK     | `sdk/python/`                     | Python (≥3.11)|

The single `main` branch is always releasable **for every component**. CI is split per component (`go.yaml`, `python.yaml`) and path-filtered, so a PR only runs the checks for what it touches.

## Getting set up

The toolchain (Go, [`uv`](https://docs.astral.sh/uv/) and pre-commit) and the tasks that build, test and lint the repo live in `mise.toml`. You'll want [mise](https://mise.jdx.dev), [activated in your shell](https://mise.jdx.dev/getting-started.html#activate-mise), and Docker, which the Dockerfile linter and the suites that run the server in a container need.

```bash
git clone https://github.com/FraiseHQ/fraise.git
cd fraise

# Install the toolchain, then the dependencies for both components:
mise install
mise run install

# Confirm it works:
mise run test        # Go server tests
mise run test:all    # everything: Go + Python
```

mise asks you to trust `mise.toml` the first time. uv brings the Python in `.python-version`, and pre-commit the linters it runs, the first time each is needed.

Run the server locally with `mise run dev`. Handy tasks: `mise run lint:all` (every linter), `mise run fmt` (auto-format), `mise run test:e2e` (end-to-end suite via docker compose). Run `mise tasks` to see the rest.

If you'd rather not install anything locally, run the server straight from the published container image:

```bash
docker run --rm -p 9876:9876 ghcr.io/fraisehq/fraise:edge
```

`:edge` tracks the latest commit on `main`; every commit also publishes an immutable tag named after its full commit SHA if you need to pin one. Releases publish `:latest` and semver tags (`:0.3`, `:0.3.0`).

**If setup doesn't work, that's a bug — please report it.** Broken setup instructions are our fault, not yours.

## Making a change

1. **For anything non-trivial, open an issue first.** Not for permission — so nobody spends a weekend on something that's already half-built or heading in a different direction. Small fixes can go straight to a PR.
2. **Create a branch named `type/short-description`** — e.g. `fix/empty-response-body`, `feat/rate-limiting`, `docs/sdk-py-readme`. The type must be one of `feat` `fix` `docs` `style` `refactor` `perf` `test` `build` `ci` `chore` `revert`. CI checks this on every PR.
3. Make your change. Add a test if you're fixing a bug or adding behaviour.
4. Run the checks locally:

   ```bash
   mise run lint:all
   mise run test:all
   ```

   Formatting is handled automatically in CI — don't spend time on style, and don't expect review comments about it.
5. Open the pull request. Describe **what problem it solves**, not just what the code does. If it closes an issue, write `Closes #123` in the description.
6. **The Benchmark gate** measures your change against `main` for retrieval quality and allocations, and comments the results on the pull request. Some regressions are worth what they cost, such as a ranking change that trades one metric for a larger gain in another. To merge one, label the pull request `perf-regression-accepted` and say why in the *Performance regression* section of its description, from the pull request template. The label is what passes the gate; the section is the reason the report shows. See [Accepting a regression](#accepting-a-regression).
7. **Sign the [CLA](https://github.com/FraiseHQ/cla/blob/main/cla.md) on your first PR** — a bot comments with the exact phrase to reply with, and the check clears for all your future contributions.

Draft PRs are welcome. If you want early feedback on an approach before polishing it, open one as a draft and say so.

### PR title & commit conventions

Titles follow [Conventional Commits](https://www.conventionalcommits.org/) — a bot checks the title on every PR:

```text
type(optional-scope): subject
```

- **Subject**: lowercase, no trailing period, 3–60 characters.
- **Type**: `feat` `fix` `perf` `refactor` `build` `ci` `chore` `docs` `test` `deps`, the types `release-please-config.json` gives a changelog section. Branch names also accept `style` and `revert`; PR titles do not.
- **Scope routes the change to a component.** This keeps each component's changelog clean:
  - _no scope_ → server — `feat: add request rate limiting`
  - `(python)` → Python SDK — `fix(python): handle empty response body`
  - `(mcp)` → MCP server — `fix(mcp): name the model in an embedding error`
- **Breaking changes** use a `!` marker: `feat(api)!: drop legacy session cookies`. Pre-1.0, breaking changes are allowed but must be documented (see [RELEASE.md](RELEASE.md)).

A wire-protocol or FQL change that spans the server and the Python SDK lands as **one PR** touching both together.

## Benchmarks

Every change to the server is measured against `main` as it stood the night before, on **retrieval quality**, how well recalls find the facts a question needs, on **allocations**, and on **latency**, how long the work takes. A change that retrieves worse or allocates more fails a check before it merges; a slower one is reported for its reviewer.

### What runs when

| When | Measured | Compared with | Outcome |
| --- | --- | --- | --- |
| Every night | `main`: every benchmark, retrieval text only and seeded with embeddings, HTTP latency at ten and a hundred thousand facts | the night before | saved as the next baseline and appended to the history; a Discord digest, flagged when a gate fails |
| Every pull request | the head: the gates whose packages it changes, retrieval text only | the newest nightly baseline | the **Benchmark gate** check and a comment on the pull request; Discord on a regression |
| The release pull request | the head: every gate, retrieval text only and seeded with embeddings | the newest nightly baseline | the **Benchmark gate** check and a comment on the pull request; Discord always |

A pull request runs only the gates it can affect: a gate's packages are its benchmarks' package and every package of the module that one builds on, so a change to their source runs the gate and any other change skips it, listed as not run in the report. A test file counts only when it defines benchmarks; other tests, the gates in `tests/perf` and what runs them change no number a benchmark measures, so they run nothing. A change to what the benchmarks run or read outside a package's source runs every gate; the gate workflow lists those paths as `--bench-always`: the toolchain in `mise.toml`, the module's dependencies, the LoCoMo sample, the server configuration the benchmarks start with and the embeddings they seed with. The release gate runs every gate whatever changed.

The nightly run is the reference. Its outputs are saved as the `benchmark-baseline` artifact, and every pull request until the next night is measured and compared with them; a pull request measures only its own head. When no baseline exists yet for a benchmark, the benchmark still runs and is reported, and its gate passes, since there is nothing to regress from.

The nightly run also appends one sample per benchmark to the history, one JSON file in the `benchmark-history` artifact that each night downloads, extends and uploads again, latency and retrieval quality each under its own name. It is the data file [github-action-benchmark](https://github.com/benchmark-action/github-action-benchmark) keeps, so its tooling reads it as is, and it comments on the commit when a value moves past its threshold.

### The benchmarks

They are Go benchmarks, beside the code they measure, and run as any other with `go test -bench`.

**Microbenchmarks** are the benchmarks under `internal/`: query commit, graph search in each lane and with a vector seed, both indexes, and key hashing. Graph search runs over ten thousand facts on two topologies. On the first, each fact is filed under one of fifty topics and mentions one of five hundred entities, and each recall names one of the topics. On the second, filed the way a conversation is, every fact sits under one conversation topic, each run of twenty-five under a session topic, and alternate facts mention one of two speakers; each recall names the conversation, a hub that holds every fact.

**Retrieval quality** is `BenchmarkRetrievalQuality` in `pkg/server`. It starts a server, ingests the [LoCoMo](https://github.com/snap-research/locomo) conversations `FRAISE_LOCOMO` names through the HTTP API, one fact per turn exactly as the amb benchmark's raw ingestion does, each said when its session took place, filed under its conversation's topic and its session's topic and mentioning its speaker, and asks each question as a recall of its words scoped to its conversation, ten results deep. LoCoMo names each question's evidence as turns, and a result counts for the turn it is; each question scores:

| Metric | Meaning |
| --- | --- |
| `turn-f1@10` | the harmonic mean of the precision and recall below, over the turns the ten results are |
| `turn-p@1` | whether the first result is an evidence turn |
| `turn-recall@10` | the share of the evidence turns among the ten results |

Each is averaged over all questions and reported per category: single-hop, multi-hop, temporal, open-domain and adversarial. The `text` set stores and asks with text only; the `vectors` set, measured when embeddings are at hand, seeds and stores with them as well, so a run is always compared with a run of the same kind. These numbers are exact: the same server asked the same questions over the same facts ranks them the same way on any machine, so one run is a measurement and any difference is a change in retrieval.

The sample is LoCoMo's conversation `conv-26`, in `tests/perf/data`: 199 questions across all five categories. LoCoMo is licensed CC BY-NC 4.0, not MIT like the rest of the repository; `tests/perf/data/NOTICE.md` gives the attribution.

**HTTP latency** is `BenchmarkHTTP` in `pkg/server`. It fills a fresh server with a deterministic corpus of twelve-word facts, each filed under one of fifty topics and mentioning one of five hundred entities. It then holds it at a constant arrival rate for a fixed time, one write in ten and otherwise recalls of two or three words scoped to one of the topics, spread evenly over the three depths, and reports the p50, p95 and p99 of the responses. Requests arrive on schedule however slowly the server answers, so the percentiles include the queueing a real client would see.

Both start the server as the binary does, on `tests/perf/fraise.config.toml`, and talk to it over HTTP, so what they measure includes everything a client waits for.

### The gates

A gate is a pytest test in `tests/perf`, one file per Go package: `server_test.py` judges `pkg/server`'s benchmarks, `graph_test.py` those of `internal/graph`, and so on. Its marker names the benchmarks and how to run them, and the test checks its limits on the comparison it is handed, failing with the rows that broke them:

```python
@pytest.mark.bench("./internal/graph", "BenchmarkSearch")
def test_allocations_do_not_grow(comparison, fail_regressed):
    grown = [row for row in comparison if row.unit == "allocs/op" and row.change >= 0.05]
    fail_regressed(grown)
```

Each row is one benchmark and unit of [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)'s comparison of the baseline with this run: each side's median, and for a timed unit a Mann-Whitney U test, so a difference counts only at p < 0.05. A unit the benchmark declares exact counts any difference. benchstat names units in base units, so time per operation is `sec/op`.

| Gate | Fails when |
| --- | --- |
| retrieval quality | any metric, overall or in any category, drops by more than 0.001 |
| allocations | allocations per operation grow by 5% or more |
| time | never: a significant slowdown of 10% or more is reported |
| HTTP latency (nightly) | never: a significant rise of 10% or more in a percentile is reported |

Retrieval and allocations are counted, not timed, so they are the same on any machine and their limits sit at the smallest change worth a look. Time is not: the baseline was timed on another runner, and two hosted runners differ by more than the slowdowns worth catching, so a slowdown is a warning in the report for a reviewer to judge, never a failure.

### Reading the report

Every gate run posts one comment on its pull request, updated on every push, whatever the verdict. It opens with the verdict, then a table per gate that failed, of the benchmarks that broke its limit, with how to [accept](#accepting-a-regression) what they caught, the same for the regressions accepted and the slowdowns, then a benchstat table per benchmark set with the baseline, this run and the change; `~` means the difference is within noise. A set with no baseline yet shows this run's numbers alone, and a unit the baseline does not measure yet shows this run's value marked *new*. The raw `go test` output is attached to the run as the `benchmarks` artifact.

### Accepting a regression

Some changes are worth what they cost: a ranking change that trades a category's `turn-p@1` for a large gain elsewhere, or a write path that allocates more to stop walking the graph. To merge one, label the pull request `perf-regression-accepted` and say why in the *Performance regression* section of its description, which the pull request template provides:

```markdown
## Performance regression

Single-hop turn-p@1 drops 0.004 for +0.03 turn-f1@10 on multi-hop; see the report.
```

The label is what accepts it. With it, every gate is expected to fail: its regressions still show in the report, as accepted, with the section as the reason. The section alone changes nothing. The night after it merges, the new numbers become the baseline.

### Running the gates locally

The gates run the same way locally as in CI. Download a nightly run's `benchmark-baseline` artifact to compare with it; without one, the gates measure and report:

```bash
mise run bench --bench-baseline=path/to/benchmark-baseline   # the pull request gates
mise run bench:nightly                                       # every gate, HTTP latency included
```

Results go to `bin/perf/`: this run's output, under the names a baseline uses, and `report.md`, the report CI posts. To run a benchmark on its own:

```bash
FRAISE_LOCOMO=$PWD/tests/perf/data/locomo-conv-26.json \
  go test ./pkg/server -run '^$' -bench BenchmarkRetrievalQuality -benchtime 1x
```

### Embeddings

The nightly run and the release gate seed retrieval with vectors computed by `tools/embed_locomo.py` and cached, keyed by the sample and the script, which names the model. A run that finds none cached computes them first and caches them for the runs after it; `mise run perf:vectors` does the same locally, into `bin/perf/locomo-vectors.json`.

## What happens next

- We aim to respond within a few days. If it's been longer, **bump the thread** — you won't be annoying anyone. Things get missed, and a nudge is a favour to us.
- Review is a conversation, not a verdict. We may ask questions, suggest a different approach, or take a couple of rounds to converge. That's normal and doesn't mean the work was bad.
- If a PR isn't a fit for the project, we'll say so clearly and explain why, rather than leaving it open indefinitely. You deserve a straight answer.
- Merged contributors are credited in the release notes.

## Releases

You don't need to do anything to release your work — maintainers cut releases, and merged work ships in the next one. But it helps to know how it flows. The full runbook is in [RELEASE.md](RELEASE.md); the short version:

- **Each component releases independently**, off its own git tag pointing at a commit on `main`:

  | Component      | Tag example          | Publishes to               |
  | -------------- | -------------------- | -------------------------- |
  | Server         | `v0.1.1`             | GitHub Releases (binaries) |
  | Python SDK     | `python/v1.0.1`      | PyPI (trusted publishing)  |

- A release is a **git tag**, written by release-please when a maintainer merges the release PR. The tag triggers the matching workflow: full test suite → **manual approval** on the `release` environment → publish.
- The **container image** (`ghcr.io/fraisehq/fraise`) publishes on every merge to `main` (`:edge` + an immutable commit-SHA tag) and on server releases (`:latest` and semver tags).
- **Semantic versioning with a `v` prefix.** Pre-1.0, breaking changes bump MINOR and fixes bump PATCH. The FQL query surface is part of the public API, so a breaking grammar change is a breaking release.
- The server and the SDK move on their own clocks, so their version numbers differ. Which SDK version supports which server version is recorded in [COMPATIBILITY.md](COMPATIBILITY.md) — an SDK-only patch releases only that SDK, nothing else.

If you're depending on Fraise while it's pre-1.0, pin an exact version.

## Who can release, and becoming a maintainer

Releases are cut by **maintainers**. For each release one of them acts as the _release captain_ — reviewing and merging the release PR, and approving the `release` environment gate that guards publishing. Any maintainer with write access can be captain; the full checklist lives in [RELEASE.md](RELEASE.md). It's a role, not a mystery, and it's one you can grow into.

There's no exam and no fixed quota. What earns maintainer access is a track record the other maintainers trust: PRs that land cleanly, thoughtful review of other people's work, help in Discussions, and follow-through. When that pattern is there, you'll be invited. Usually triage first, then release rights as trust builds.

**Want to head that way? Say so.** Open a [Discussion](https://github.com/FraiseHQ/fraise/discussions) or mention it on a PR. Knowing someone wants the responsibility makes it easy to hand over the right pieces at the right time — you don't have to wait to be noticed.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). In short: be kind, assume good faith, and remember there's a person on the other end.

To report unacceptable behaviour, contact the maintainers privately, open a confidential report via GitHub, or reach out through the address listed in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Reports are handled discreetly, and you will not be penalised for making one in good faith.

---

Thanks again. Genuinely — maintaining a project is a lot more fun with other people around.
