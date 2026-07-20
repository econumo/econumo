---
name: publish-release
description: Publish a new Econumo release end-to-end — pick the version, dispatch the Publish Release GitHub workflow, watch the build, verify the published images, and replace the auto-generated GitHub release notes with a written summary in the house style. Use whenever the user asks to publish, cut, ship, or tag a new version/release (e.g. "publish v1.2.0", "cut a release", "ship what's on main"), or to rewrite/update the release notes of an existing release.
---

# Publish an Econumo release

Releases are cut by the `Publish Release` workflow (`.github/workflows/publish-release.yml`),
never by pushing tags or images locally. The workflow's `branch` input selects the source
branch (`main` by default; a `release/*` branch for hotfixes), and the dispatch itself always
uses `--ref main` so the workflow definition never comes from a stale branch. The workflow
does three things in order: auto-creates a `release/vX.Y.Z` branch at the source branch's
head (the base for future hotfixes to that version) and the annotated git tag on the same
commit, builds and pushes the multi-arch image to `ghcr.io/econumo/econumo`, and creates the
GitHub release with auto-generated notes (changelogithub). Your job is to drive it, verify
each artifact, and then replace the auto-generated notes with notes a human would want to
read.

Publishing is outward-facing and irreversible in practice (the tag and image are public
immediately). Only run the dispatch when the user has explicitly asked for a release, and if
they didn't name the version, propose one and get confirmation first.

## 1. Establish the baseline

```bash
git tag --sort=-creatordate | head -5        # last released version
gh release list --limit 3
git fetch origin main --quiet
git log vLAST..origin/main --oneline          # everything the release will contain
```

Pick the version by semver over that range: any `feat:` → minor bump; fixes/chores only →
patch. Sanity-check that main is what the user intends to release (no surprising or
half-landed work in the log) and that the latest run of the Go tests workflow on main is
green (`gh run list --workflow=go-tests.yml --branch main --limit 1`). Surface anything odd
before dispatching — after dispatch there is no undo.

For a **hotfix of an older version** the version is a patch bump of that line (bug in
v1.1.1 → v1.1.2), regardless of what has since shipped from main.

## 2. Prepare the source branch (hotfixes only)

For a normal release there is nothing to prepare — the workflow auto-creates
`release/vX.Y.Z` at the released commit, so every version leaves a branch behind as the
base for future fixes.

For a hotfix, the fixes must be on the source branch BEFORE dispatching. Land them on the
release branch of the version being fixed (`release/vA.B.C`, auto-created when it was
released) — cherry-pick the fix commits from main onto a working branch and PR it with the
release branch as the base (or push the cherry-picks directly if the user prefers). If that
branch doesn't exist (the version predates auto-created release branches), create it from
the tag first:

```bash
git fetch --tags --quiet
git push origin 'vA.B.C^{}':refs/heads/release/vA.B.C
```

Confirm with `git log origin/release/vA.B.C` that the branch holds exactly the intended
fixes and nothing else from main. Note the test workflows ignore pushes to `release/**` —
only the PR route runs tests on the fixes, which is why it is the default; after a direct
push, run the suites locally before dispatching.

## 3. Dispatch and watch

```bash
# Normal release (source defaults to main):
gh workflow run publish-release.yml --ref main -f version=vX.Y.Z -f push_latest=true

# Hotfix (source = the fixed version's release branch):
gh workflow run publish-release.yml --ref main -f version=vX.Y.Z -f branch=release/vA.B.C
```

- Always dispatch with `--ref main`; the `branch` input (default `main`) is what selects the
  code being released.
- `push_latest=true` is the norm when releasing the newest version. The workflow enforces it:
  `latest` only moves when the source branch is `main` or `release/*` AND the version is
  higher than every existing tag. For a hotfix of an older line, leave it off — the guard
  would reject it anyway. Leave it off for pre-releases/betas too.
- The workflow fails fast if the tag already exists.
- Grab the run id (`gh run list --workflow=publish-release.yml --limit 1`) and watch it in the
  background: `gh run watch <run-id> --exit-status --interval 30` via a background Bash call.
  The image build is the long step (several minutes) — use that time for step 4.

## 4. Write the release notes while the build runs

The auto-generated notes are just a commit list; always replace them. Gather substance first:
the commit subjects give you the map, and `gh pr view <n> --json title,body` on the headline
PRs gives you accurate detail. Write for two audiences at once — self-hosters deciding
whether/how to upgrade, and API-client authors who need to know about wire changes.

House style (see the v1.0.0 release for the reference tone):

```markdown
<one-paragraph summary of the release's themes>

## What's new
- **Feature name** (#PR) — what it does and why a user cares, in plain language.

## Security hardening        <- only if the release contains security fixes
- What was fixed, honestly stated; users deserve to know what was exposed.

## Fixes & improvements
- Grouped, user-visible phrasing (not commit subjects).

## Upgrading
Drop-in note for self-hosters (image pull + restart; migrations run on boot).
Call out anything that affects third-party API clients — the wire contract is
frozen for the bundled SPA, so any change to it (new/removed endpoints, field
format changes) MUST be listed here.

The `ghcr.io/econumo/econumo:latest` image tag points to this build; to pin the
exact version use `ghcr.io/econumo/econumo:vX.Y.Z`.

## Contributors
[@login](https://github.com/login) — ...

**Full changelog**: [vLAST...vX.Y.Z](https://github.com/econumo/econumo/compare/vLAST...vX.Y.Z)
```

Contributors come from the actual range, not memory:

```bash
git fetch --tags --quiet     # the tag was created REMOTELY by the workflow — fetch before ranging over it
git log vLAST..vX.Y.Z --format='%an <%ae>' | sort | uniq -c
git log vLAST..vX.Y.Z | grep -i 'co-authored-by' | sort | uniq -c
```

GitHub no-reply emails (`ID+login@users.noreply.github.com`) embed the numeric user id;
resolve the CURRENT login with `gh api user/<id> -q .login` — old emails may carry a former
handle, and two emails with the same id are one person. Credit Claude co-author trailers as
a "with Claude (...) co-authoring" note rather than a separate contributor entry.

Draft the notes into a scratchpad file so `gh release edit --notes-file` can consume it.

## 5. Verify, then apply the notes

After the workflow succeeds (all three jobs green):

```bash
gh release view vX.Y.Z --json tagName,isDraft,url        # release exists, not draft
gh release edit vX.Y.Z --notes-file <notes.md>           # replace notes
docker buildx imagetools inspect ghcr.io/econumo/econumo:vX.Y.Z | grep Digest
docker buildx imagetools inspect ghcr.io/econumo/econumo:latest  | grep Digest
```

The workflow already aligns the GitHub "Latest" badge with `push_latest` — don't pass
`--latest`/`--latest=false` yourself. When `push_latest` was set, the two digests must
match — that is the proof `latest` actually moved; for a hotfix of an older line, verify
the OPPOSITE: `latest` must still point at the newest version's digest, not the hotfix's.
Report the release URL, the digest check, and a summary of what the notes say; invite the
user to adjust the wording, since the notes are public-facing prose.

## Hotfix example

v1.1.1 is released, main already carries v1.2.0-bound work, and a bug is found in v1.1.1:

```bash
# land the fix on release/v1.1.1 (cherry-pick PR based on it; create the branch
# from the v1.1.1 tag first only if the release predates auto-created branches), then:
gh workflow run publish-release.yml --ref main -f version=v1.1.2 -f branch=release/v1.1.1
```

The workflow tags v1.1.2 on the fix commit and auto-creates `release/v1.1.2` there — the
base for a future v1.1.3. No `push_latest`: `latest` keeps pointing at the newest line, and
the workflow keeps the GitHub "Latest" badge off the hotfix release. Notes follow the
normal flow with the range v1.1.1...v1.1.2.

## Updating notes on an already-published release

Skip steps 1–3. Build the notes exactly as in step 4 (fetch tags first if the tag isn't
local), then `gh release edit <tag> --notes-file <notes.md>`. Don't pass `--latest` unless
the release should also become the latest one.
