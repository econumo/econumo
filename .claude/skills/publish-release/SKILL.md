---
name: publish-release
description: Publish a new Econumo release end-to-end — pick the version, dispatch the Publish Release GitHub workflow, watch the build, verify the published images, and replace the auto-generated GitHub release notes with a written summary in the house style. Use whenever the user asks to publish, cut, ship, or tag a new version/release (e.g. "publish v1.2.0", "cut a release", "ship what's on main"), or to rewrite/update the release notes of an existing release.
---

# Publish an Econumo release

Releases are cut by the `Publish Release` workflow (`.github/workflows/publish-release.yml`),
never by pushing tags or images locally. The workflow does three things in order: creates the
annotated git tag, builds and pushes the multi-arch image to `ghcr.io/econumo/econumo`, and
creates the GitHub release with auto-generated notes (changelogithub). Your job is to drive it,
verify each artifact, and then replace the auto-generated notes with notes a human would want
to read.

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

## 2. Dispatch and watch

```bash
gh workflow run publish-release.yml --ref main -f version=vX.Y.Z -f push_latest=true
```

- `push_latest=true` is the norm for a real release from main — the workflow itself guards
  `latest` to the main branch. Leave it off for pre-releases/betas.
- The workflow fails fast if the tag already exists.
- Grab the run id (`gh run list --workflow=publish-release.yml --limit 1`) and watch it in the
  background: `gh run watch <run-id> --exit-status --interval 30` via a background Bash call.
  The image build is the long step (several minutes) — use that time for step 3.

## 3. Write the release notes while the build runs

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

## 4. Verify, then apply the notes

After the workflow succeeds (all three jobs green):

```bash
gh release view vX.Y.Z --json tagName,isDraft,url        # release exists, not draft
gh release edit vX.Y.Z --notes-file <notes.md> --latest  # replace notes, mark as latest
docker buildx imagetools inspect ghcr.io/econumo/econumo:vX.Y.Z | grep Digest
docker buildx imagetools inspect ghcr.io/econumo/econumo:latest  | grep Digest
```

The two digests must match — that is the proof `latest` actually moved. Report the release
URL, the digest check, and a summary of what the notes say; invite the user to adjust the
wording, since the notes are public-facing prose.

## Updating notes on an already-published release

Skip steps 1–2. Build the notes exactly as in step 3 (fetch tags first if the tag isn't
local), then `gh release edit <tag> --notes-file <notes.md>`. Don't pass `--latest` unless
the release should also become the latest one.
