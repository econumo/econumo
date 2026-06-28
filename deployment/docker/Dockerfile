# Econumo backend image. The Go binary is ONE static process that serves both
# the JSON API and the SPA on a single port (no nginx/php-fpm/supervisor).
#
# Build context is the REPOSITORY ROOT, so the COPY paths below are root-relative:
#
#   docker build -f deployment/docker/Dockerfile -t ghcr.io/econumo/econumo:latest .
#
# Runtime contract (see docker-compose.yml at the repo root):
#   - DATABASE_URL is REQUIRED and comes from the environment / .env (it is NOT
#     baked into the image); e.g. sqlite:///app/var/db/db.sqlite.
#   - The JWT keypair is generated automatically on first boot if it does not
#     already exist (default /app/var/jwt/{private,public}.pem). NOTHING is
#     baked in at build time — keys are unique per deployment. Mount a volume at
#     /app/var so the keypair, passphrase, and the sqlite db survive restarts.
#     Override paths with JWT_SECRET_KEY/JWT_PUBLIC_KEY to mount your own keys.
#   - Migrations run inside the binary on boot (baseline-aware), so there is no
#     entrypoint script — distroless has no shell and needs none.
#   - econumo-config.js ships with a working default (API_URL:null -> same
#     origin) baked into the SPA build; override it by mounting your own file
#     over /app/spa/econumo-config.js.

# --- 1. Frontend build ---
FROM node:25-alpine AS frontend
WORKDIR /build/web
RUN npm install -g pnpm@10
COPY web/package.json web/pnpm-lock.yaml* ./
RUN pnpm install --shamefully-hoist
COPY web/ ./
# Build the SPA from the single committed env template (web/.env.dist). web/.env
# is gitignored (local dev only) and absent in CI, so we always copy .env.dist ->
# .env for a deterministic build. Pass --build-arg ECONUMO_VERSION=v1.2.3 to set
# the version label shown in the UI; with no arg the template's "dev" is used.
ARG ECONUMO_VERSION=""
RUN cp .env.dist .env && \
    if [ -n "$ECONUMO_VERSION" ]; then \
        sed -i "s/^ECONUMO_EDITION_LABEL=.*/ECONUMO_EDITION_LABEL=\"${ECONUMO_VERSION}\"/" .env; \
    fi
RUN pnpm run build
# -> /build/web/dist/spa (includes the default econumo-config.js)

# --- 2. Go build (pure Go, CGO off -> fully static binary) ---
FROM golang:1.26-alpine AS gobuild
WORKDIR /src
# Module files first for layer-cached `go mod download`.
COPY go.mod go.sum ./
RUN go mod download
# Only the Go source (the module lives at the repo root alongside web/, etc.).
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /app/econumo ./cmd/econumo
# Pre-create the runtime data dirs so they are owned by the non-root user in the
# final image: the sqlite db (var/db) and the auto-generated JWT keypair (var/jwt)
# must be writable on first boot, and distroless cannot mkdir/chown (no shell).
RUN mkdir -p /out/var/db /out/var/jwt

# --- 3. Runtime (distroless: no shell, no package manager, minimal surface) ---
FROM gcr.io/distroless/static-debian12 AS prod
COPY --from=gobuild /app/econumo /app/econumo
# Management commands run via the binary directly: `docker exec <c> /app/econumo user:create …`
# (the binary dispatches a non-flag first argument to the CLI).
COPY --from=frontend /build/web/dist/spa /app/spa
# Runtime data dir, owned by the non-root user: the sqlite db (var/db) and the JWT
# keypair (var/jwt) the server generates on first boot. Mount a volume over
# /app/var to persist both.
COPY --from=gobuild --chown=nonroot:nonroot /out/var /app/var

# Serve the SPA the frontend stage built; the container always listens on :80
# (map it to any host port in compose). DATABASE_URL is intentionally NOT set here
# — it comes from .env (the compose env_file). The JWT keypair defaults to
# /app/var/jwt and is auto-generated on first boot (config defaults), so no key
# paths are baked into ENV here.
ENV ECONUMO_SPA_DIR=/app/spa \
    PORT=80

EXPOSE 80

# WORKDIR sets the /app working layout so the default relative paths resolve here
# — notably the JWT keypair (var/jwt → /app/var/jwt, auto-generated on first boot).
WORKDIR /app

# distroless ships a non-root `nonroot` user (uid 65532); the SQLite db dir +
# jwt keys mounted at runtime must be readable/writable by it.
USER nonroot:nonroot

# `serve` is explicit: the binary prints usage (and does not start a server) when
# invoked with no command, so the server start is always intentional.
ENTRYPOINT ["/app/econumo", "serve"]
