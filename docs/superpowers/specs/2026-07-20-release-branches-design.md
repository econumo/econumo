# Release branches (hotfix-capable release process)

**Date:** 2026-07-20
**Status:** Approved (autonomous session; decisions recorded for review)

## Problem

Releases are tagged straight off `main`. Once newer work lands on `main` (e.g.
changes intended for v1.2.0), there is no way to ship a fix for an older
version (e.g. a bug found in v1.1.1) without also shipping everything that came
after it. The `Publish Release` workflow also hard-guards the `latest` image
tag to dispatches from `main`, which makes releasing from any other branch a
second-class path.

## Decision

### Process

1. **A release from `main` auto-creates its `release/vX.Y.Z` branch**: the
   workflow pushes `release/<version>` at the released commit (skipped when it
   already exists there; an existing branch at a *different* commit is an
   error), so every version cut from main leaves a branch behind as the base
   for future fixes. Nothing to prepare for a normal release from `main`.
   A release from any other source branch creates **no** branch — the rule is
   "create a release branch only when releasing from main; never when the
   dispatch already names a release branch".
2. **Hotfix of an older version**: copy the fixed version's release branch to
   the new version's name
   (`git push origin origin/release/vA.B.C:refs/heads/release/vX.Y.Z`; if the
   version predates release branches, from the tag:
   `git push origin 'vA.B.C^{}':refs/heads/release/vX.Y.Z`), land the fixes on
   the copy, then dispatch with `branch=release/vX.Y.Z` and the same
   `version`. The workflow tags the branch head and creates nothing — the old
   branch stays pinned at exactly what its version shipped.
3. **Dispatch the workflow from `main`** in all cases. Dispatching from `main`
   means the workflow *definition* always comes from `main`, so hotfix
   branches cut from old tags never run a stale copy of the pipeline; the
   `branch` input alone selects the code being released.

Release branches are kept after the release; they are the base for future
fixes to that line.

### Pipeline (`.github/workflows/publish-release.yml`)

- New `workflow_dispatch` input **`branch`** (default `main`): the ref the
  `create-tag` job checks out, tags, and the later jobs build. The default
  keeps the pre-existing "release what's on main" flow working unchanged.
- The `create-tag` job pushes `release/<version>` at the checked-out head
  before tagging, but only when the `branch` input is `main` (duplicate-tag
  check first, then branch, then tag — so a failed tag push can be re-run
  safely: an existing branch at the same commit is skipped, at a different
  commit it fails the run). Any other source skips branch creation with a
  log notice.
- The old guard ("`latest` only when dispatched from `main`") is **replaced** —
  under the new model releases come from `release/*` branches, so it would
  block every `latest`. New `push_latest` guard, checked in `create-tag`:
  1. the source branch is `main` or `release/*`, and
  2. the version is the highest `v*` tag in the repo by semver order
     (`sort -V`) — so a hotfix to an old line can never move `latest`.
- New step in `create-github-release`: after changelogithub creates the
  release, set the GitHub **"Latest release" badge** to match `push_latest`
  (`gh release edit --latest` / `--latest=false`). GitHub otherwise badges
  the newest *created* release as latest, which would mislabel an old-line
  hotfix published after a newer version.
- The test workflows (`go-tests.yml`, `web-tests.yml`) ignore pushes to
  `release/**`: those branches are cut at already-tested commits, and hotfix
  changes reach them via pull requests, whose `pull_request` checks still run.
  (Auto-created branches wouldn't trigger CI anyway — `GITHUB_TOKEN` pushes
  never start workflows — but the filter also covers manual pushes.)

### Docs

- `.claude/skills/publish-release/SKILL.md`: branch-creation step, the
  `branch` dispatch flag, a hotfix walkthrough, and dropping the manual
  `--latest` (the workflow now owns the badge).
- `CLAUDE.md` Publishing section: one-line mention of release branches.

## Alternatives considered

- **Use the native `workflow_dispatch` ref as the source branch** (no input;
  `gh workflow run --ref release/vX.Y.Z`). Rejected: a hotfix branch cut from
  an old tag carries the workflow file as of that tag, so old releases would
  run old pipeline code (missing guards/fixes). The explicit input keeps one
  pipeline definition (main's) for all releases.
- **Per-minor-line branches (`release/v1.1`)** holding all patch tags of a
  line. Standard elsewhere and fewer branches, but the user specified
  `release/vX.X.X` (per-version) naming; per-version is also unambiguous about
  what each branch produced. Revisit if branch count becomes a nuisance.
- **Manual release-branch creation before every dispatch** (the first
  iteration of this design). Replaced at the user's request by auto-creation
  inside the workflow for main releases: normal releases need no manual step.
- **Auto-creating the new version's branch on every release, hotfixes
  included** (the second iteration: fixes land on the *fixed* version's
  branch, and the workflow creates `release/<new-version>` at the fix
  commit). Replaced at the user's request by the create-only-from-main rule:
  the hotfix branch is a hand-made copy of the old release branch under the
  new version's name, the workflow only tags it, and old release branches
  never drift past the commit their version shipped.

## Not changing

- Tag creation, image build/push, changelogithub notes, `push_dev`, the
  version-format validation, and the duplicate-tag check all stay as they are.
- The `dev` tag remains publishable from any branch.
