# Edge Channel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every push to `main` publishes an `edge` image and R2 channel stamped with the highest release tag, and a release that moves `latest` also moves `edge`.

**Architecture:** One shell helper picks the highest `vX.Y.Z` tag from `git ls-remote` output. A new workflow builds on every push to `main`, stamps that tag as the version, and moves `edge` only if no newer tag appeared during the build. The release workflow adds `edge` next to `latest`.

**Tech Stack:** GitHub Actions, Docker Buildx, ship (R2), POSIX shell, GNU Make.

**Spec:** `docs/superpowers/specs/2026-11-15-edge-channel-design.md`

**Prerequisite (separate repository, not in this plan):** ship accepts any channel name in `publish --channels`, `install --version` and `update --version`, and `dmtrkzntsv/ship@v0` carries it. Until then the R2 step of Task 2 fails; the image step works.

## Global Constraints

- Image tag `edge` on `ghcr.io/econumo/econumo`; R2 channel `edge`; no immutable R2 version and no git tag for an edge build.
- Edge version = highest tag matching `^v[0-9]+\.[0-9]+\.[0-9]+$` by version sort, over ALL tags (not only ancestors of `main`); `dev` when no such tag exists.
- `ECONUMO_COMMIT` / `version.Commit` = the built commit sha.
- A release moves `edge` exactly when it moves `latest` (`push_latest=true`); no new input.
- `dev` workflows are unchanged.
- No app code changes.

## Review Focus

- A tag lands while an edge build runs: the build must not move `edge` (Task 2 re-check, test in Task 1 covers the comparison input).
- The repository has no release tag (a fork): the version is `dev` and the build still publishes (Task 1 empty case, Task 2 fallback).
- Non-release tags (`v1.8.0-rc1`, `v2`, `foo`) exist: they are ignored (Task 1).
- Two-digit components (`v1.10.0` vs `v1.9.3`): version order, not text order (Task 1).
- A hotfix release of an older line (`push_latest=false`): `edge` is untouched (Task 3 check).

---

### Task 1: Highest release tag helper

**Files:**
- Create: `.github/scripts/highest-release-tag.sh`
- Test: `.github/scripts/highest-release-tag_test.sh`

**Interfaces:**
- Produces: `.github/scripts/highest-release-tag.sh` reads lines on stdin, each either a bare tag (`v1.2.3`) or a `git ls-remote --tags --refs` line (`<sha>\trefs/tags/v1.2.3`); prints the highest strict `vX.Y.Z` followed by a newline, or nothing; always exits 0. Callers use `git ls-remote --tags --refs origin | .github/scripts/highest-release-tag.sh`.

- [ ] **Step 1: Write the failing test**

`highest-release-tag_test.sh` (POSIX sh, `set -eu`, exits non-zero on the first failure, prints `ok` at the end) asserts:

```sh
check "$(printf 'v1.9.3\nv1.10.0\nv1.2.0\n' | "$S")" "v1.10.0"
check "$(printf 'v1.8.0-rc1\nv2\nfoo\nv1.7.3\n' | "$S")" "v1.7.3"
check "$(printf 'abc123\trefs/tags/v1.7.3\ndef456\trefs/tags/v1.7.4\n' | "$S")" "v1.7.4"
check "$(printf '' | "$S")" ""
check "$(printf 'v1.8.0-rc1\n' | "$S")" ""
```

where `S` is the script's path resolved next to the test and `check actual expected` prints both and exits 1 on mismatch.

- [ ] **Step 2: Run test to verify it fails**

Run: `sh .github/scripts/highest-release-tag_test.sh`
Expected: FAIL (script missing).

- [ ] **Step 3: Implement `highest-release-tag.sh`**

Strip everything up to `refs/tags/`, keep lines matching `^v[0-9]+\.[0-9]+\.[0-9]+$`, `sort -V`, `tail -n 1`. A `grep` with no match must not fail the script (exit 0 with empty output).

- [ ] **Step 4: Run test and shellcheck**

Run: `sh .github/scripts/highest-release-tag_test.sh && shellcheck .github/scripts/highest-release-tag.sh .github/scripts/highest-release-tag_test.sh`
Expected: `ok`, no shellcheck findings.

- [ ] **Step 5: Commit**

```bash
git add .github/scripts/highest-release-tag.sh .github/scripts/highest-release-tag_test.sh
git commit -m "ci: helper picking the highest release tag"
```

### Task 2: Publish Edge workflow

**Files:**
- Create: `.github/workflows/publish-edge.yml`
- Modify: `Makefile` (the ship comment block, ~lines 300-325: channels are any name, `edge` follows `main`)
- Modify: `CLAUDE.md` ("Publishing" section and the "Deployment" image-tag list: add `:edge`)

**Interfaces:**
- Consumes: Task 1's `highest-release-tag.sh`.
- Produces: image `ghcr.io/econumo/econumo:edge`, R2 channel `edge`.

Shape (mirror `publish-dev.yml` for the build steps; header comment states what edge is and why the version is the highest release tag):

- `on: push: branches: [main]` and `workflow_dispatch` (no inputs). `concurrency: { group: publish-edge, cancel-in-progress: true }`. `env: IMAGE: ghcr.io/econumo/econumo`.
- Job `resolve` (contents: read): checkout `github.sha`; run `sh .github/scripts/highest-release-tag_test.sh`; `V="$(git ls-remote --tags --refs origin | sh .github/scripts/highest-release-tag.sh)"`, `${V:-dev}`; outputs `sha`, `version`; write both to the step summary.
- Job `publish-image` (needs resolve; contents: read, packages: write): Buildx, ghcr login, `docker/build-push-action@v7` with build-args `ECONUMO_VERSION=<version>`, `ECONUMO_COMMIT=<sha>` and `outputs: type=image,name=${{ env.IMAGE }},push-by-digest=true,name-canonical=true,push=true` (no tags). Then step "Move edge": recompute the highest tag (fallback `dev`); if it differs from `needs.resolve.outputs.version`, print a `::notice::` that a newer release moved edge and exit 0; else `docker buildx imagetools create -t "$IMAGE:edge" "$IMAGE@<digest from build-push-action outputs.digest>"`.
- Job `publish-binaries` (needs resolve; contents: read): SPA build, Go build of both linux arches with the ldflags of `publish-dev.yml`, using `<version>` and `<sha>`; `dmtrkzntsv/ship@v0`; step "Publish edge channel to R2" with the R2 secrets env of `publish-dev.yml`: the same re-check as above (skip with notice), else `make ship-dist SHIP_SKIP_BUILD=1` then `make ship-publish CHANNELS=edge` (VERSION stays the Makefile default `dev`, which publishes no immutable version).

- [ ] **Step 1: Write the workflow, Makefile comment and CLAUDE.md lines**

- [ ] **Step 2: Lint**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/publish-edge.yml`
Expected: no output, exit 0.

- [ ] **Step 3: Check the Makefile target accepts the call**

Run: `make -n ship-publish CHANNELS=edge | tail -n 1`
Expected: `ship publish econumo --path "dist/ship"  --channels edge` (no `--version`).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/publish-edge.yml Makefile CLAUDE.md
git commit -m "ci: publish the edge channel on every push to main"
```

### Task 3: Releases move edge

**Files:**
- Modify: `.github/workflows/publish-release.yml` (header comment; "Compute image tags" step ~lines 64-104; "Publish to R2" step ~lines 281-295)
- Modify: `.claude/skills/publish-release/SKILL.md` (dispatch section: `push_latest=true` also moves `edge`, so production gets the release at once)
- Modify: `CLAUDE.md` ("Publishing": a release moves `edge` with `latest`)

**Interfaces:**
- Consumes: nothing from earlier tasks (edge is only a tag/channel name here).
- Produces: a release with `push_latest=true` tags the image `edge` and moves the R2 channel `edge`.

- [ ] **Step 1: In "Compute image tags", when `PUSH_LATEST` is true append `${IMAGE}:edge` after `${IMAGE}:latest`; in "Publish to R2", when `PUSH_LATEST` is true set `CHANNELS="latest,edge"`. Update the header comment and the two docs.**

- [ ] **Step 2: Lint**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/publish-release.yml`
Expected: no output, exit 0.

- [ ] **Step 3: Check the tag and channel logic for both values of push_latest**

Run the "Compute image tags" script body locally with `VERSION=v9.9.9 BRANCH=main PUSH_DEV=false IMAGE=ghcr.io/econumo/econumo GITHUB_OUTPUT=/dev/stdout`, once with `PUSH_LATEST=true` and once with `PUSH_LATEST=false`.
Expected: `true` prints `tags=ghcr.io/econumo/econumo:v9.9.9,ghcr.io/econumo/econumo:latest,ghcr.io/econumo/econumo:edge`; `false` prints `tags=ghcr.io/econumo/econumo:v9.9.9`. Do the same for the R2 channel lines: `latest,edge` vs empty.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/publish-release.yml .claude/skills/publish-release/SKILL.md CLAUDE.md
git commit -m "ci: a release that moves latest also moves edge"
```
