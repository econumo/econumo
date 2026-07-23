# Publish release binaries to Cloudflare R2 (CDN)

**Date:** 2026-07-22
**Status:** Approved (design)

## Problem

Release binaries (`econumo-linux-amd64`, `econumo-linux-arm64`, `SHA256SUMS`)
are attached only to GitHub Releases today. We want them additionally mirrored
to a Cloudflare R2 bucket, served publicly from `cdn.econumo.com`, addressable
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
| Key layout | `s3://<bucket>/<channel>/econumo-linux-{amd64,arm64}` + `SHA256SUMS`, one `SHA256SUMS` per channel prefix. |
| Channel | `dev` \| `latest` \| `vX.Y.Z` |
| Public base URL | `https://cdn.econumo.com` → `https://cdn.econumo.com/<channel>/econumo-linux-amd64` |
| Bucket | `econumo` — hardcoded default, overridable via `R2_BUCKET` |
| Endpoint | `R2_ENDPOINT` from env/secret (`https://<account_id>.r2.cloudflarestorage.com`) — never committed |
| Credentials | `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` from env/secrets |

## Config surface

The Makefile and the workflow share one interface:

- `R2_ENDPOINT` — required for any upload; `https://<account_id>.r2.cloudflarestorage.com`.
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` — R2 API-token credentials.
- `R2_BUCKET ?= econumo` — hardcoded default, overridable.
- `CDN_BASE_URL ?= https://cdn.econumo.com` — hardcoded default, used only to print
  human-readable download URLs after upload.

New GitHub Actions secrets: `R2_ENDPOINT`, `R2_ACCESS_KEY_ID`,
`R2_SECRET_ACCESS_KEY` (the latter two mapped to the `AWS_*` env names inside
the upload step).

## Makefile changes

### New internal target `cdn-upload`

Parameterized by `CHANNEL` (required) and `SRC` (defaults `release-out`).
Behavior:

1. Fail with a clear message if `R2_ENDPOINT` is empty.
2. Upload `$(SRC)/econumo-linux-amd64`, `$(SRC)/econumo-linux-arm64`, and
   `$(SRC)/SHA256SUMS` to `s3://$(R2_BUCKET)/$(CHANNEL)/` using
   `aws s3 cp --endpoint-url $(R2_ENDPOINT) --content-type application/octet-stream`.
3. Cache headers: `--cache-control "no-cache"` for `dev`/`latest`,
   `--cache-control "public, max-age=31536000, immutable"` for a `vX.Y.Z` channel.
4. Print the resulting `$(CDN_BASE_URL)/$(CHANNEL)/...` URLs.

Sets `AWS_REQUEST_CHECKSUM_CALCULATION=when_required` (or
`--checksum-algorithm CRC32`) so the aws CLI v2 default integrity headers that
R2 rejects are not sent.

### `publish-dev`

Gains a `release-binaries` prerequisite invoked with `VERSION=dev`, then runs
`cdn-upload CHANNEL=dev`. Net effect: `make publish-dev` pushes the `:dev`
image **and** uploads the `dev/` binaries to the CDN. (Building the binaries
runs the SPA build a second time vs. the buildx image build — acceptable
duplication for a local dev-publish.)

## Release workflow changes (`.github/workflows/publish-release.yml`)

In the existing `build-binaries` job, after the `sha256sum` step, add upload
step(s) that run the aws CLI with the three R2 secrets in env
(`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`/`R2_ENDPOINT`):

- **Always** upload `s3://econumo/${VERSION}/` (immutable cache).
- Upload `latest/` **only when** `push_latest == 'true'` (no-cache).
- Upload `dev/` **only when** `push_dev == 'true'` (no-cache).

This mirrors exactly how the Docker image tags already gate in the `create-tag`
job. The upload reuses the `release-out/` directory already produced in the
job; the same three files go to each selected channel prefix.

## Documentation

- `docs/run-without-docker.md`: mention the CDN mirror
  (`https://cdn.econumo.com/latest/econumo-linux-amd64`) as an alternative to
  the GitHub release download.
- No `.env.example` change: `R2_*` are build/publish-time credentials, not
  runtime server config.

## Testing / verification

- No Go or frontend code changes → no unit/integration/parity impact.
- Manual verification of the aws command shape (dry echo of the composed
  commands); a real end-to-end upload requires live R2 credentials held by the
  maintainer and is out of scope for automated tests.

## Out of scope

- Creating the R2 bucket, API token, and `cdn.econumo.com` custom domain
  (assumed to exist / provisioned by the maintainer).
- Updating the `econumo.com/releases/latest.json` update feed (separate system).
- Windows/macOS binaries (only linux amd64/arm64 are built today).
