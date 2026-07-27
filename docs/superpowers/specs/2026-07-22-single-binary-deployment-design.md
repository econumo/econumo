# Single-binary release artifacts + bare-metal deployment

**Date:** 2026-07-22
**Status:** Approved

## Goal

Offer a Docker-free distribution channel: every GitHub release ships
self-contained Linux binaries (SPA embedded) that a self-hoster can run on a
single server — binary + SQLite database on one host — under systemd. The
Docker image remains the primary channel and converges on the same embedded-SPA
code path.

## Decisions (settled with the user)

- **Audience:** official release artifacts attached to every GitHub release,
  not a personal-server-only path.
- **Packaging:** true single file via `go:embed` of the built SPA — no tarball,
  no build-tag split. `ECONUMO_WEB_DIST` stays as a disk override.
- **Platforms:** `linux/amd64` + `linux/arm64` only (matches the image's arch
  matrix). No macOS/Windows, no deb/rpm, no install script, no auto-update.
- **Deploy story:** a reference systemd unit shipped in `deployment/` plus a
  documented walkthrough.

## 1. SPA embedding

- New `web/embed.go` (`package web`, module path
  `github.com/econumo/econumo/web`) with `//go:embed all:dist` exposing the
  built SPA as an `embed.FS`. `go:embed` cannot reach outside a package
  directory, and `web/dist` is exactly where `pnpm build` already writes —
  no copy step, no drift.
- Commit a placeholder `web/dist/.gitkeep` (everything else in `dist/` stays
  gitignored) so `go build ./...` / `go test ./...` / CI smoke never require a
  frontend build. The `all:` prefix embeds dot-files, so the pattern always
  matches. An embed containing no `index.html` means "no SPA embedded".
- `internal/web/spa` refactors from `Handler(dir string, overrides …)` to
  serving from an `fs.FS` (`http.FileServerFS`, `fs.Stat`/`fs.ReadFile`).
  Frozen semantics are unchanged: history-mode fallback to `index.html`,
  honest 404 for missing asset-like paths and reserved (`/api`, `/_`) paths,
  cache-control headers, and the `econumo-config.js` `Object.assign` merge of
  server-owned keys.
- Runtime source selection (in `config`/`server` wiring):
  1. `ECONUMO_WEB_DIST` explicitly set → serve that disk directory
     (dev override, separately-hosted SPA — current behavior).
  2. Else, embedded dist contains `index.html` → serve the embedded FS.
  3. Else → the current `web/dist` disk default (source checkout with a built
     SPA but a stale binary).
- Dockerfile: the gobuild stage gains
  `COPY --from=frontend /build/web/dist ./web/dist` before `go build`; the
  runtime stage drops the `/app/web` COPY and the `ECONUMO_WEB_DIST` env. The
  image and the release binary now exercise one code path.

## 2. Binary version identity

- New `internal/version` package: `var Version = "dev"`, set by
  `-ldflags "-X github.com/econumo/econumo/internal/version.Version=vX.Y.Z"`
  in the release job and the Docker build.
- Surfaced in a `version` subcommand (the binary is subcommand-driven; no
  flag form) and as a field on the serve boot log line. It complements (does not replace) the SPA's
  Vite-inlined `ECONUMO_VERSION` label; the workflow sets both from the same
  input.

## 3. Release artifacts (workflow changes)

- `.github/workflows/publish-release.yml` gains a `build-binaries` job
  (`needs: create-tag`, checkout of the tagged ref):
  1. Build the SPA once with `ECONUMO_VERSION=<version>` (`pnpm install
     --frozen-lockfile && pnpm run build`, with `locales/` present as in the
     Dockerfile — a repo checkout already has it in place).
  2. For `GOARCH` in `amd64`, `arm64`:
     `CGO_ENABLED=0 GOOS=linux go build -trimpath
     -ldflags "-s -w -X …/internal/version.Version=<version>"`
     → `econumo-linux-amd64`, `econumo-linux-arm64`.
  3. `sha256sum econumo-linux-* > SHA256SUMS`; upload the three files as a
     workflow artifact.
- `create-github-release` adds `build-binaries` to its `needs`, downloads the
  artifact, and after changelogithub creates the release runs
  `gh release upload "$TAG" econumo-linux-amd64 econumo-linux-arm64 SHA256SUMS`.
- A local `make release-binaries` target mirrors the cross-compile (SPA build +
  both arches + checksums, version from a `VERSION` variable defaulting to
  `dev`) so the artifact shape is verifiable without a workflow run.

## 4. Deployment: systemd unit + docs

- `deployment/systemd/econumo.service` — reference unit:
  - dedicated `econumo` system user/group; binary at `/opt/econumo/econumo`;
    `ExecStart=/opt/econumo/econumo serve`.
  - `EnvironmentFile=/etc/econumo/env` (contents modeled on `.env.example`;
    at minimum `DATABASE_URL=sqlite:///var/lib/econumo/db.sqlite` and a
    non-privileged `PORT`).
  - SQLite data in `/var/lib/econumo`.
  - Hardening: `NoNewPrivileges=yes`, `ProtectSystem=strict`,
    `ProtectHome=yes`, `ReadWritePaths=/var/lib/econumo`,
    `PrivateTmp=yes`; `Restart=on-failure`.
- Docs: a "Run without Docker (single binary)" section in the README (or a
  `docs/` page the README links): download the arch binary from the release,
  verify against `SHA256SUMS`, create user + dirs, write `/etc/econumo/env`,
  install the unit, `systemctl enable --now econumo`, check
  `/opt/econumo/econumo healthcheck` / `/health`. Upgrades: replace the
  binary, restart — migrations run on boot, exactly as in the image. CLI
  management commands run as `sudo -u econumo /opt/econumo/econumo user:create …`.

## 5. Testing

- `spa_test.go` moves to `fs.FS` fixtures (`fstest.MapFS`); all existing
  behavioral assertions carry over.
- New test: a placeholder-only embed (no `index.html`) is treated as absent —
  selection falls through to the disk path.
- New test: version wiring (`internal/version` default + subcommand output).
- apiparity / mcpparity / goldens: untouched — no route, envelope, or wire
  change.
- CI smoke (`make go-test`) stays frontend-free via the `.gitkeep`
  placeholder.
- Artifact verification (manual, pre-first-release): `make release-binaries`,
  boot `econumo-linux-amd64` on a scratch SQLite `.env`, log in, confirm the
  SPA loads from the embed and `version` reports the ldflags value.

## Out of scope

Install script, macOS/Windows binaries, OS packages (deb/rpm), auto-update,
PostgreSQL setup guidance beyond what `.env.example` already covers.
