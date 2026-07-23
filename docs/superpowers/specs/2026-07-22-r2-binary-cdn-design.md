# Publish release binaries to Cloudflare R2 (CDN)

**Date:** 2026-07-22
**Status:** Approved (design)

## Problem

Release binaries (`econumo-linux-amd64`, `econumo-linux-arm64`, `SHA256SUMS`)
are attached only to GitHub Releases today. We want them additionally mirrored
to a **private** Cloudflare R2 bucket (no public custom domain — objects are
retrieved via the S3 API / signed URLs by credential holders), addressable
by channel:

- per version — `vX.Y.Z`
- `latest`
- `dev`

And `make publish-dev` — which today only builds and pushes the multi-arch
`:dev` Docker image — should also upload the `dev` binaries to the CDN.

## Decisions

| Choice | Value |
|---|---|
| Upload tool | `aws` S3 CLI (S3-compatible R2 API). Preinstalled on GitHub runners; assumed present locally. |
| Key layout | `s3://<bucket>/<project>/<channel>/econumo-linux-{amd64,arm64}` + `SHA256SUMS`, one `SHA256SUMS` per channel prefix. |
| Project | `econumo` — hardcoded default, overridable via `R2_PROJECT`. Namespaces the bucket so it can host multiple projects. |
| Channel | `dev` \| `latest` \| `vX.Y.Z` |
| Bucket visibility | **Private** — no public custom domain; retrieved via the S3 API / signed URLs by credential holders. |
| Bucket | `econumo` — hardcoded default, overridable via `R2_BUCKET` |
| Endpoint | `R2_ENDPOINT` from env/secret (`https://<account_id>.r2.cloudflarestorage.com`) — never committed |
| Credentials | `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` from env/secrets |

## Config surface

The Makefile and the workflow share one interface:

Credential/endpoint names are `ECONUMO_`-prefixed (matching the repo's env
convention) so several projects' R2 endpoints/keys can coexist in one shell
without colliding:

- `ECONUMO_R2_ENDPOINT` — required for any upload; `https://<account_id>.r2.cloudflarestorage.com`.
- `ECONUMO_R2_ACCESS_KEY_ID` / `ECONUMO_R2_SECRET_ACCESS_KEY` — R2 API-token
  credentials. **Optional**: when both are set they are mapped to `AWS_*` for
  the `aws` subprocess only; when unset, aws falls back to its own resolution
  (e.g. the `~/.aws` default profile).
- `R2_BUCKET ?= econumo` — hardcoded default, overridable.
- `R2_PROJECT ?= econumo` — project namespace under the bucket, overridable.
- `CDN_SRC ?= release-out` — directory holding the built binaries + `SHA256SUMS`.

GitHub Actions secrets: `R2_ENDPOINT`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`
(per-repo, so no cross-project collision there), mapped in the upload step's
`env:` to the `ECONUMO_R2_*` names the Makefile reads.

## Makefile changes

### New internal target `cdn-upload`

Parameterized by `CHANNEL` (required) and `SRC` (defaults `release-out`).
Behavior:

1. Fail with a clear message if `CHANNEL` is empty or `ECONUMO_R2_ENDPOINT` is empty.
2. Upload `$(CDN_SRC)/econumo-linux-amd64`, `$(CDN_SRC)/econumo-linux-arm64`, and
   `$(CDN_SRC)/SHA256SUMS` to `s3://$(R2_BUCKET)/$(R2_PROJECT)/$(CHANNEL)/` using
   `aws s3 cp --endpoint-url $(R2_ENDPOINT) --content-type application/octet-stream`.
3. Cache headers: `--cache-control "no-cache"` for `dev`/`latest`,
   `--cache-control "public, max-age=31536000, immutable"` for a `vX.Y.Z` channel.
4. Print the `s3://$(R2_BUCKET)/$(R2_PROJECT)/$(CHANNEL)/` destination.

Exports `AWS_REQUEST_CHECKSUM_CALCULATION=when_required` so the aws CLI v2
default integrity headers that R2 rejects are not sent.

### `publish-dev`

Gains a `release-binaries` prerequisite invoked with `VERSION=dev`, then runs
`cdn-upload CHANNEL=dev`. Net effect: `make publish-dev` pushes the `:dev`
image **and** uploads the `dev/` binaries to the CDN. (Building the binaries
runs the SPA build a second time vs. the buildx image build — acceptable
duplication for a local dev-publish.)

## Release workflow changes (`.github/workflows/publish-release.yml`)

In the existing `build-binaries` job, after the `sha256sum` step, add one
upload step with the three R2 secrets in env (mapped to
`ECONUMO_R2_ENDPOINT`/`ECONUMO_R2_ACCESS_KEY_ID`/`ECONUMO_R2_SECRET_ACCESS_KEY`)
that reuses the Makefile target (DRY — same upload logic as local):

```sh
make cdn-upload CHANNEL="$VERSION"                                  # always
[ "$PUSH_LATEST" = "true" ] && make cdn-upload CHANNEL=latest       # gated
[ "$PUSH_DEV" = "true" ]    && make cdn-upload CHANNEL=dev          # gated
```

This mirrors exactly how the Docker image tags already gate in the `create-tag`
job. The upload reuses the `release-out/` directory already produced in the
job; the same three files go to each selected channel prefix.

## Documentation

- No user-facing docs change: the bucket is private, so `docs/run-without-docker.md`
  keeps pointing at the public GitHub release download.
- No `.env.example` change: `R2_*` are build/publish-time credentials, not
  runtime server config.

## Testing / verification

- No Go or frontend code changes → no unit/integration/parity impact.
- Manual verification of the aws command shape (dry echo of the composed
  commands); a real end-to-end upload requires live R2 credentials held by the
  maintainer and is out of scope for automated tests.

## Out of scope

- Creating the R2 bucket and API token (assumed to exist / provisioned by the
  maintainer). The bucket stays private — no custom domain / public access.
- Updating the `econumo.com/releases/latest.json` update feed (separate system).
- Windows/macOS binaries (only linux amd64/arm64 are built today).
