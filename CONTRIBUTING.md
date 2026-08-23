# Contributing to Fraise

Thanks for being here. Contributions of any size are welcome: a typo fix, a bug
report, a question that reveals the docs are unclear. All of it helps.

**If you're unsure about anything, ask.** Open a
[Discussion](https://github.com/RonsenbergVI/fraise/discussions) and someone will
help you figure it out. You don't need to know the answer before you ask the
question, and no question here is too basic.

## Ways to help

You don't have to write code to contribute:

- **Report a bug** — [open an issue](https://github.com/RonsenbergVI/fraise/issues/new/choose).
  Even a partial report is useful; we'll ask if we need more.
- **Improve the docs** — if something confused you, it will confuse the next
  person. Fixing that is one of the most valuable things you can do.
- **Answer a question** in Discussions.
- **Try a release candidate** and tell us what broke.
- **Share what you built** with it. Seriously — it helps us understand what the
  project is actually for.

Looking for somewhere to start? Issues labelled
[`good first issue`](https://github.com/RonsenbergVI/fraise/labels/good%20first%20issue)
are scoped small and come with enough context to get going. If one looks
interesting but the description is thin, comment and ask — that's not a bother,
it's a signal the issue needs a better description.

## How the repo is laid out

Fraise is a monorepo with three released components, each versioned and released
on its own clock:

| Component      | Lives in                          | Language      |
| -------------- | --------------------------------- | ------------- |
| Server         | `cmd/`, `internal/`, `pkg/`       | Go (1.25)     |
| Python SDK     | `sdk/python/`                     | Python (≥3.12)|
| TypeScript SDK | `sdk/typescript/`                 | TypeScript    |

The single `main` branch is always releasable **for every component**. CI is
split per component (`go.yaml`, `python.yaml`, `typescript.yaml`) and
path-filtered, so a PR only runs the checks for what it touches.

## Getting set up

You'll want Go 1.25+, plus [`uv`](https://docs.astral.sh/uv/) (Python) and
[`pnpm`](https://pnpm.io/) (TypeScript) if you're working on the SDKs.

```bash
git clone https://github.com/RonsenbergVI/fraise.git
cd fraise

# Install dependencies for all three components:
make install

# Confirm it works:
make test        # Go server tests
make test-all    # everything: Go + Python + TypeScript
```

Run the server locally with `make dev`. Handy targets: `make lint` (all
components), `make fmt` (auto-format), `make test-e2e` (end-to-end suite via
docker compose). Run `make help` to see the rest.

If you'd rather not install anything locally, run the server straight from the
published container image:

```bash
docker run --rm -p 9876:9876 ghcr.io/RonsenbergVI/fraise:edge
```

`:edge` tracks the latest commit on `main`; every commit also publishes an
immutable tag named after its full commit SHA if you need to pin one.
Releases publish `:latest` and semver tags (`:0.1`, `:0.1.0`).

**If setup doesn't work, that's a bug — please report it.** Broken setup
instructions are our fault, not yours.

## Making a change

1. **For anything non-trivial, open an issue first.** Not for permission — so
   nobody spends a weekend on something that's already half-built or heading in
   a different direction. Small fixes can go straight to a PR.
2. **Create a branch named `type/short-description`** — e.g.
   `fix/empty-response-body`, `feat/rate-limiting`, `docs/sdk-py-readme`. The
   type must be one of `feat` `fix` `docs` `style` `refactor` `perf` `test`
   `build` `ci` `chore` `revert`. CI checks this on every PR.
3. Make your change. Add a test if you're fixing a bug or adding behaviour.
4. Run the checks locally:

   ```bash
   make lint
   make test-all
   ```

   Formatting is handled automatically in CI — don't spend time on style, and
   don't expect review comments about it.
5. Open the pull request. Describe **what problem it solves**, not just what the
   code does. If it closes an issue, write `Closes #123` in the description.
6. **Sign the [CLA](CLA.md) on your first PR** — a bot comments with the exact
   phrase to reply with, and the check clears for all your future
   contributions.

Draft PRs are welcome. If you want early feedback on an approach before
polishing it, open one as a draft and say so.

### PR title & commit conventions

Titles follow [Conventional Commits](https://www.conventionalcommits.org/) —
a bot checks the title on every PR:

```text
type(optional-scope): subject
```

- **Subject**: lowercase, no trailing period, 3–60 characters.
- **Type**: `feat` `fix` `docs` `style` `refactor` `perf` `test` `build` `ci`
  `chore` `revert` (also `deps`, `upgrade`, `release`).
- **Scope routes the change to a component.** This keeps each component's
  changelog clean:
  - _no scope_ → server — `feat: add request rate limiting`
  - `(python)` → Python SDK — `fix(python): handle empty response body`
  - `(typescript)` → TypeScript SDK — `feat(typescript): stream recall results`
- **Breaking changes** use a `!` marker: `feat(api)!: drop legacy session cookies`.
  Pre-1.0, breaking changes are allowed but must be documented (see
  [RELEASE.md](RELEASE.md)).

A wire-protocol or FQL change that spans the server and both SDKs lands as **one
PR** touching all three together.

## What happens next

- We aim to respond within a few days. If it's been longer, **bump the
  thread** — you won't be annoying anyone. Things get missed, and a nudge is a
  favour to us.
- Review is a conversation, not a verdict. We may ask questions, suggest a
  different approach, or take a couple of rounds to converge. That's normal and
  doesn't mean the work was bad.
- If a PR isn't a fit for the project, we'll say so clearly and explain why,
  rather than leaving it open indefinitely. You deserve a straight answer.
- Merged contributors are credited in the release notes.

## Releases

You don't need to do anything to release your work — maintainers cut releases,
and merged work ships in the next one. But it helps to know how it flows.
The full runbook is in [RELEASE.md](RELEASE.md); the short version:

- **Each component releases independently**, off its own git tag pointing at a
  commit on `main`:

  | Component      | Tag example          | Publishes to               |
  | -------------- | -------------------- | -------------------------- |
  | Server         | `v0.1.1`             | GitHub Releases (binaries) |
  | Python SDK     | `python/v1.0.1`      | PyPI (trusted publishing)  |
  | TypeScript SDK | `typescript/v0.9.12` | npm (with provenance)      |

- A release is a **git tag**. Pushing the tag triggers the matching workflow:
  full test suite → **manual approval** on the `release` environment → publish.
- The **container image** (`ghcr.io/RonsenbergVI/fraise`) publishes on every
  merge to `main` (`:edge` + an immutable commit-SHA tag) and on server
  releases (`:latest` and semver tags).
- **Semantic versioning with a `v` prefix.** Pre-1.0, breaking changes bump
  MINOR and fixes bump PATCH. The FQL query surface is part of the public API,
  so a breaking grammar change is a breaking release.
- The server and SDKs move on their own clocks, so their version numbers
  differ. Which SDK version supports which server version is recorded in
  [COMPATIBILITY.md](COMPATIBILITY.md) — an SDK-only patch releases only that
  SDK, nothing else.

If you're depending on Fraise while it's pre-1.0, pin an exact version.

## Who can release, and becoming a maintainer

Releases are cut by **maintainers**. For each release one of them acts as the
_release captain_ — reviewing the release PR, pushing the tag, and approving the
`release` environment gate that guards publishing. Any maintainer with write
access can be captain; the full checklist lives in [RELEASE.md](RELEASE.md).
It's a role, not a mystery, and it's one you can grow into.

There's no exam and no fixed quota. What earns maintainer access is a track
record the other maintainers trust: PRs that land cleanly, thoughtful review of
other people's work, help in Discussions, and follow-through. When that pattern
is there, you'll be invited. Usually triage first, then release rights as trust
builds.

**Want to head that way? Say so.** Open a
[Discussion](https://github.com/RonsenbergVI/fraise/discussions) or mention it on
a PR. Knowing someone wants the responsibility makes it easy to hand over the
right pieces at the right time — you don't have to wait to be noticed.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). In short:
be kind, assume good faith, and remember there's a person on the other end.

To report unacceptable behaviour, contact the maintainers privately, open a
confidential report via GitHub, or reach out through the address listed in
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Reports are handled discreetly, and
you will not be penalised for making one in good faith.

---

Thanks again. Genuinely — maintaining a project is a lot more fun with other
people around.
