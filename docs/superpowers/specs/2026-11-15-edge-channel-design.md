# Edge channel: production runs main before a release

## Intent

Production (econumo cloud) gets every change as soon as it lands on `main`, an
early preview of the next release. When the owner is satisfied, they cut a
release on GitHub exactly as today; self-hosters get it, production moves to it,
and users see the "What's new" card. The owner never picks a version for a
preview build.

## Goals

- Every push to `main` publishes an `edge` build: the image
  `ghcr.io/econumo/econumo:edge` and the R2 channel `edge` (ship).
- Production hosts follow the channel (`ship update econumo --version edge`).
- What's new, the self-hosted update notice and new-account version seeding keep
  working with no app code change.

## Non-goals

- No git tag, GitHub release or immutable R2 version for edge builds.
- No change to how releases are cut, versioned or announced.
- How a host picks up a new edge build (running `ship update`, a timer) stays
  host-side and is not part of this work.

## Anti-goals

- Users see the What's new card for a release before it is cut, or lose it
  after it is.
- The owner has to think of, type or guess a version for a preview.

## Approaches considered

1. **Edge reports the latest release tag (picked).** An edge build is stamped
   `VERSION` = the highest release tag, e.g. `v1.8.0` for every edge build
   between the `v1.8.0` and `v1.8.1` releases. Every version check already
   accepts it, so What's new, the update notice and seeding need no change.
   `VERSION_REF` (the commit) still tells builds apart and keys the client
   caches, as it does for `dev` today.
2. **Edge reports `git describe` (`v1.8.0-12-gabc1234`) and the app reads the
   release out of it.** Builds are distinguishable by version, but four version
   checks (Go `IsReleaseVersion`; SPA `SEMVER`/`isNewerVersion`, What's new, the
   update hook) need a new rule. Rejected: more change for distinguishability
   `VERSION_REF` already gives.
3. **Server remembers the last release it ran.** Rejected: a server that never
   runs the exact release build (a push right after the tag), a fresh host or a
   restored database has no record, and it adds persisted state.

Also rejected earlier: stamping a guessed next version (`v1.8.0-beta.N`), and
switching production between `latest` and `edge` by hand each release.

## Design

### 1. ship accepts any channel name (separate repository, lands first)

`ship publish --channels`, `ship install --version` and `ship update --version`
accept any channel name, not only `latest` and `dev`, so a new channel never
needs a ship change. A channel name must not be mistakable for a version (the
builder of ship decides the exact rule, e.g. `^[a-z][a-z0-9-]*$`).

### 2. Publish Edge workflow (`.github/workflows/publish-edge.yml`)

- Triggered on every push to `main` (and `workflow_dispatch` for a manual
  re-run). Concurrency group `publish-edge`, newer run cancels an in-flight one.
- Version: the highest tag matching `v[0-9]*.[0-9]*.[0-9]*` by `sort -V`, the
  same rule the release workflow's `latest` guard uses (a hotfix tag cut from a
  `release/*` branch counts even though it is not an ancestor of `main`).
  Checkout needs the tags (`fetch-depth: 0`).
- Builds the image and the linux binaries exactly like Publish Dev, with
  `ECONUMO_VERSION=<that tag>` and `ECONUMO_COMMIT=<sha>`; pushes the image tag
  `edge` and publishes the R2 channel `edge` only (no immutable version, so R2
  version retention is unaffected).
- Race with a release: just before moving the image tag and the channel, the
  job re-reads the highest tag; if it is higher than the one it stamped, it
  skips publishing (the release has already moved `edge` to a newer-stamped
  build, and the next push builds on it). Otherwise a build that started before
  the tag would overwrite edge with an older version and hide the card.

### 3. Releases move edge

When Publish Release runs with `push_latest=true` (a release from `main` or the
newest `release/*`, above every tag), it also points the image tag `edge` and
the R2 channel `edge` at the release build, so production gets the release
immediately even when nothing is pushed afterwards. A hotfix to an older line
never moves `edge`. No new checkbox.

### 4. `dev` is unchanged

`dev` stays the manual channel for testing any branch (Publish Dev,
`make publish-dev`), reporting version `dev`.

### 5. Docs

CLAUDE.md "Publishing", the Makefile ship comments (channels are no longer
limited to `latest,dev`) and `.claude/skills/publish-release` describe `edge`
and that a release moves it.

## Known trade-offs

- Analytics `$app_version`, the sidebar label and logs read `v1.8.0` for both
  the release build and every edge build after it; only `VERSION_REF` tells
  them apart.
- Before a release, production shows the previous release's number while
  running newer code.
- Migrations: production applies `main`'s migrations before any release
  contains them, so a hotfix migration on a `release/*` branch must be numbered
  above every migration production has applied, not only above the last
  release.

## Left to the builder

- Sharing the build steps between Publish Dev and Publish Edge (reusable
  workflow or duplication).
- The exact ship channel-name rule and its error messages.

## Revision 1 (after the branch review)

Wins over the sections above where they differ.

- **Version = the GitHub release marked Latest**, not the highest tag
  (`.github/scripts/latest-release.sh`; `dev` when there is no release, any
  other failure fails the build). A tag whose release failed, or a release
  published without `push_latest`, never becomes the edge version, so no
  user can dismiss a card for a release that does not exist.
- **Releases dispatch Publish Edge on `main`** as their last job (only with
  `push_latest`) instead of tagging the release build `edge`. Edge is always
  main's head: pointing it at the release commit could take production back
  to older code than it runs (a hotfix of the newest line from `release/*`,
  or a push that landed during the release) and below migrations it applied.
  The dispatch cancels an in-flight edge build; the stand-down re-check stays
  as a guard.
- **Docs-only pushes publish nothing** (`docs/`, `.claude/`, Markdown outside
  the embedded `prompts/`): every edge build changes `VERSION_REF`, and the
  client cache buster is `VERSION+VERSION_REF`, so each one makes every
  client drop and re-download its cached data.
- A hotfix migration on `release/*` must be numbered above every migration
  on `main` (CLAUDE.md, publish-release skill).

## Revision 2 (owner: edge always follows the newest release)

Wins over Revision 1 where they differ.

- **Version = the highest published release** (a plain vX.Y.Z GitHub release,
  no draft, no pre-release), not the Latest badge. A tag whose release failed
  still never counts.
- **Any release of the newest version rebuilds edge**, with or without
  `push_latest`: forgetting the checkbox must not keep production on the
  previous version. An old-line patch (v1.6.2 after v1.7.0) is for
  self-hosters only and never touches edge.
