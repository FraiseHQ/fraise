# Fraise Release Process

This document describes how maintainers cut a release. The process is:
**feature PRs merge to main → release PR (changelog) → captain pushes the tag → automated build, GitHub release, announcement.**

## How changes reach a release

1. Features and fixes land on `main` through normal PRs. The `vX.Y.Z`
   **milestone** is the agreement on scope — issues/PRs assigned to it are
   "in", anything that slips gets bumped to the next milestone.
2. `main` is always releasable: merged means it ships in the next release.
3. A release is a **tag pointing at a commit on main**. The artifacts are
   built from exactly that commit — same tag, same source, reproducible.

## Versioning

- Semantic versioning with a `v` prefix: `vMAJOR.MINOR.PATCH` (e.g. `v0.1.0`).
- Pre-releases use a dot-numbered suffix: `vX.Y.Z-alpha.N`, `vX.Y.Z-beta.N`, or
  `vX.Y.Z-rc.N` (e.g. `v0.1.0-alpha.1`). These are the only suffix forms the
  release workflows accept, and GoReleaser auto-marks them as pre-releases.
- Pre-1.0: breaking changes bump MINOR, fixes bump PATCH.
- The FQL surface is part of the public API — a breaking grammar change is a breaking release.

## Roles

- **Release captain** — the maintainer cutting this release. Owns the checklist below.
- Any maintainer with write access can be release captain.

## Process

Releases are driven by [release-please](https://github.com/googleapis/release-please)
(`release-please-config.json`, `.release-please-manifest.json`). Nobody tags by
hand: the version in the tree and the tag are written by the same commit, so
they cannot disagree.

### 1. The release PR maintains itself

Every push to `main` updates an open PR titled `chore(main): release X.Y.Z`.
release-please derives the next version from the conventional-commit messages
since the last release and writes, in that PR:

- `CHANGELOG.md` — the grouped commit list for the new version.
- `pkg/version/version.go` — the `Version` constant on the
  `x-release-please-version` annotated line. **Never edit this by hand.**
- `.release-please-manifest.json` — the new current version.

Nothing is released while the PR sits open; it just tracks what the next
release would be.

### 2. Merge the release PR

Merging it **is** the release. release-please tags the merge commit `vX.Y.Z`
and creates the GitHub Release with the changelog as its notes. That tag push
then fires the `publish` job in `.github/workflows/go.yaml`, exactly as a
hand-pushed tag used to:

1. **test / cross-compile** — the full suite runs against the tagged commit.
2. **Manual approval** — `publish` pauses on the `release` environment. A
   maintainer clicks **Approve** in the Actions UI. This is the last human gate
   before artifacts exist.
3. **publish** — GoReleaser cross-compiles all targets and *appends* binaries +
   checksums to the release release-please already created, then announces on
   Discord/Slack. It builds no changelog: the notes are release-please's.

Because merging is the trigger, a bad release is fixed forward with another
commit and another release PR — there is no tag to delete and re-push.

> **One-time setup:**
>
> - **Settings → Environments → New environment** named `release`, enable
>   **Required reviewers**, add the maintainers. Move the
>   `DISCORD_WEBHOOK_ID` / `DISCORD_WEBHOOK_TOKEN` secrets into this
>   environment for extra hygiene (only approved runs can read them).
> - **Settings → Actions → General → Allow GitHub Actions to create and
>   approve pull requests**, or release-please cannot open its PR.
> - A `RELEASE_PLEASE_TOKEN` secret holding a PAT. Fine-grained, on this repo:
>   **Contents: Read and write** (branch, commit, tag, release),
>   **Pull requests: Read and write** (open and update the release PR), and
>   **Issues: Read and write** (release-please labels the PR `autorelease:*`,
>   and PR labels go through the Issues API). Metadata: Read-only comes along
>   automatically. A classic PAT needs the single `repo` scope; a GitHub App
>   token works too.
>
>   This is **not optional**: a tag pushed with the default `GITHUB_TOKEN` does
>   not trigger other workflows, so the release would be created with no
>   binaries attached.

### Choosing the version

While `prerelease-type` is `beta`, every release increments the pre-release
counter (`0.1.0-beta.1` → `0.1.0-beta.2`), including breaking changes —
`bump-minor-pre-major` keeps pre-1.0 breaking changes on the same minor rather
than jumping to `1.0.0`.

Moving between phases is the one manual step: release-please will not switch
`alpha` → `beta` → `rc` → stable on its own
([#2447](https://github.com/googleapis/release-please/issues/2447)). Force it
with a footer on any commit:

```text
Release-As: 0.1.0-beta.1
```

> **The footer must survive the squash.** We squash-merge, and when a PR has
> more than one commit GitHub builds the squash message from the commit
> *subjects* — the bodies, and therefore the footer, are dropped, and the
> release PR silently comes out with the wrong version. Put the change in a
> single commit (GitHub then defaults the squash body to that commit's body),
> or paste the `Release-As:` line into the squash body by hand before
> confirming the merge. Verify with
> `git log origin/main -1 --format=%B | grep Release-As` after merging.

The next release PR then targets that version. Update `prerelease-type` in
`release-please-config.json` in the same change so subsequent releases continue
in the new phase.

### 3. Post-release checks

- [ ] GitHub Release page shows all artifacts (6 archives + checksums).
- [ ] `go install github.com/RonsenbergVI/fraise/cmd/fraise@vX.Y.Z` works.
- [ ] Docker image pulled and boots (`docker run … --version` prints the right version).
- [ ] Announcement arrived in the channel.

## Build targets

| OS | Arch |
|---|---|
| linux | amd64, arm64 |
| darwin | amd64, arm64 |
| windows | amd64, arm64 |

Static binaries (`CGO_ENABLED=0`), version injected via ldflags.

## Release notes

Generated by GoReleaser from conventional commits, grouped:

- `feat:` → **Features**
- `fix:` → **Bug fixes**
- `perf:` → **Performance**
- everything else → **Other changes**

The `CHANGELOG.md` section for the version is the human-curated summary; the
GitHub release notes carry the full grouped commit list plus a link to the
changelog.

## Upgrades

Every release that requires user action ships with an **Upgrading** section in
the release notes (and, once non-trivial, a cumulative `UPGRADING.md` in the
repo). It covers:

- **Breaking changes** — what breaks, why, and the migration step. Pre-1.0,
  breaking changes are allowed in MINOR releases but must always be documented
  here; a release with an empty Upgrading section is a drop-in upgrade.
- **FQL changes** — the grammar is a public API. Any change to query syntax or
  semantics (what a query returns) gets its own entry, with before/after
  examples. Agents and fine-tuned models depend on this surface staying stable.
- **Config changes** — renamed/removed flags and config keys, with the old→new
  mapping.
- **Data compatibility** — Fraise is in-memory today, so upgrades are
  stateless (stop old binary, start new). Once persistence lands, this section
  will state whether the on-disk format changed and ship a migration path.
  A release must never silently corrupt or discard stored memory.

**Deprecation policy:** where possible, deprecate one MINOR release before
removal — the deprecated form keeps working but logs a warning, and the
release notes announce the removal target.

## Maintenance

- **Supported version: the latest release only.** Bug reports are triaged
  against the newest version; fixes land on `main` and ship in the next
  release. No backports pre-1.0 — the project is too young to maintain
  parallel versions.
- **Critical bugs & security issues** use the hotfix path below: patch
  release (`vX.Y.Z+1`) as soon as the fix is verified, announced through the
  same channels with a clear severity note.
- **Dependency & toolchain hygiene:** the Go toolchain version is bumped in
  ordinary releases; Fraise's zero-dependency core keeps the update surface
  minimal by design.
- **Issue hygiene:** every accepted issue gets a milestone (or `backlog`).
  Stale issues are closed with a comment, never silently.

This policy grows with the project — once there are production users, the
support window widens (e.g. fixes backported to the previous minor) and this
section is updated to say exactly what's supported for how long.

## Branching & monorepo strategy

One `main`, always releasable **for every component** (server, Python SDK,
TypeScript SDK). There are no per-component branches — CI path-filtering
tests exactly the components a PR touches, and branch protection blocks any
merge that breaks one of them.

### Branches

- Short-lived feature branches off `main`, merged by squash PR:
  `feat/server-rp-forest`, `fix/sdk-py-timeout`, `docs/sdk-ts-readme`.
- No `develop`, no per-SDK branches, no release branches (pre-1.0).

### PR scopes route changes to components

Only two scopes exist — anything unscoped is the server:

- `feat: ...` (no scope) → server
- `fix(python): ...` → Python SDK
- `feat(typescript): ...` → TypeScript SDK

The scope keeps each component's changelog clean: `.goreleaser.yaml`
excludes `(python)`/`(typescript)` scoped commits from server release notes.

### Releases: three tag namespaces, one branch, one workflow

`release.yaml` triggers on all three tag patterns and runs the matching
pipeline (test → manual approval on the `release` environment → publish):

| Component      | Tag example          | Publishes to               |
|----------------|----------------------|----------------------------|
| Server         | `v0.1.1`             | GitHub Releases (binaries) |
| Python SDK     | `python/v1.0.1`      | PyPI (trusted publishing)  |
| TypeScript SDK | `typescript/v0.9.12` | npm (provenance)           |

SDK tagging (after the version-bump PR is merged — the workflow rejects a
tag that doesn't match `pyproject.toml`/`package.json`):

```bash
    git tag python/v1.0.1     && git push origin python/v1.0.1
    git tag typescript/v0.9.12 && git push origin typescript/v0.9.12
```

CI is split per component (`go.yaml`, `python.yaml`, `typescript.yaml`),
each path-filtered to its own files. Branch-protection note: only mark the
**Go** jobs as required checks globally — path-filtered SDK checks would
otherwise show "expected" forever on server-only PRs.

Every tag is a pointer to a commit on `main`; each component versions and
releases on its own clock. Milestones are named like the tags.

### Cross-component (protocol) changes

A wire-protocol or FQL change lands as **one PR** touching the server and
both SDKs together. Release order afterwards: **server first**, then the
SDK tags — SDK integration tests run against a published server release,
and PyPI/npm never ship ahead of the server they target.

### Hotfixes (SDKs)

Branch from the component's tag (`git checkout -b hotfix/python-1.0.2
python/v1.0.1`), fix, PR to main, tag the component's patch release.

## Hotfix path

For a critical fix on the latest release: branch from the tag
(`git checkout -b hotfix/vX.Y.Z+1 vX.Y.Z`), apply the fix, PR into main as
usual, then run the same release workflow with the patch version. No separate
mechanism — one process for everything.

## Secrets required (repo settings → Actions secrets)

- `DISCORD_WEBHOOK_ID` and `DISCORD_WEBHOOK_TOKEN` — the two halves of a Discord
  webhook URL (`https://discord.com/api/webhooks/<ID>/<TOKEN>`). GoReleaser's
  Discord announcer reads these, not the full URL. *(For Slack instead, use
  `SLACK_WEBHOOK_URL` — see the `.goreleaser.yaml` announce section.)*
- `GITHUB_TOKEN` is provided automatically by Actions.
