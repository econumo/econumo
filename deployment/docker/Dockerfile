# Econumo backend: one static Go binary serving the JSON API + SPA on a single port.
# Build context is the repo root:
#   docker build -f deployment/docker/Dockerfile -t ghcr.io/econumo/econumo:latest .
# DATABASE_URL comes from the environment (.env), not baked in. Mount a volume
# under /app/var to persist the sqlite db. Migrations run inside the binary on boot.

# Both build stages run natively on the build host ($BUILDPLATFORM) — the SPA
# output is platform-independent and Go cross-compiles — so a multi-arch build
# never runs the heavy toolchains under QEMU emulation.

# --- 1. Frontend build ---
FROM --platform=$BUILDPLATFORM node:26-alpine AS frontend
WORKDIR /build/web
RUN npm install -g pnpm@11
# pnpm-workspace.yaml carries allowBuilds; without it pnpm 11 fails the
# install on unapproved build scripts.
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# web/src/app/i18n.ts imports ../../../locales/*.json, so locales/ must land
# one level above WORKDIR (/build/locales), mirroring the repo-root layout.
COPY locales/ /build/locales/
# Vite inlines ECONUMO_VERSION (envPrefix in vite.config.ts) as the UI version label.
# Pass --build-arg ECONUMO_VERSION=v1.2.3 to set it; defaults to "dev".
ARG ECONUMO_VERSION=""
RUN ECONUMO_VERSION="${ECONUMO_VERSION:-dev}" pnpm run build

# --- 2. Go build (CGO off -> fully static binary) ---
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS gobuild
WORKDIR /src
# Module files first for layer-cached `go mod download`.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# internal/infra/i18n imports github.com/econumo/econumo/locales, which lives
# at the module root alongside cmd/ and internal/.
COPY locales/ ./locales/
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /app/econumo ./cmd/econumo
# Pre-create runtime dirs owned by the non-root user (distroless can't mkdir on boot).
RUN mkdir -p /out/var/db

# --- 3. Runtime (distroless: no shell, minimal surface) ---
FROM gcr.io/distroless/static-debian12 AS prod
COPY --from=gobuild /app/econumo /app/econumo
COPY --from=frontend /build/web/dist /app/web
# Runtime data dir (sqlite db); mount a volume here to persist.
COPY --from=gobuild --chown=nonroot:nonroot /out/var /app/var

ENV ECONUMO_WEB_DIST=/app/web \
    PORT=80

EXPOSE 80
WORKDIR /app
USER nonroot:nonroot

# `serve` is explicit: the binary prints usage when invoked with no command.
ENTRYPOINT ["/app/econumo", "serve"]
