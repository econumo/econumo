# Econumo backend image. The Go binary is ONE static process that serves both
# the JSON API and the SPA on a single port (no nginx/php-fpm/supervisor).
#
# Build context is the REPOSITORY ROOT, so the COPY paths below are root-relative:
#
#   docker build -f deployment/docker/Dockerfile -t econumo/econumo-go:latest .
#
# Runtime contract (see deployment/docker-compose/docker-compose.go.yml):
#   - DATABASE_URL is REQUIRED  (sqlite:///var/www/var/db/db.sqlite by default).
#   - The JWT keypair is generated automatically on first boot if it does not
#     already exist (default /var/www/var/jwt/{private,public}.pem). NOTHING is
#     baked in at build time — keys are unique per deployment. Mount a volume at
#     /var/www/var/jwt so they (and an auto-generated passphrase, unless you set
#     JWT_PASSPHRASE) survive restarts. Override paths with JWT_SECRET_KEY/
#     JWT_PUBLIC_KEY if you prefer to mount your own keys.
#   - Migrations run inside the binary on boot (baseline-aware), so there is no
#     entrypoint script — distroless has no shell and needs none.
#   - econumo-config.js ships with a working default (API_URL:null -> same
#     origin) baked into the SPA build; override it by mounting your own file
#     over /app/web/spa/econumo-config.js.

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
COPY go/go.mod go/go.sum ./
RUN go mod download
COPY go/ ./
# Build to /econumo (the same absolute path it lives at in the final image) so the
# bin/console symlink below is NOT dangling in this stage — BuildKit must be able
# to resolve a symlink's target to COPY it across stages.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /econumo ./cmd/econumo
# Backward-compatible `bin/console` entry: a symlink to the single binary. The
# binary dispatches a non-flag first argument to the CLI (the ported `app:*`
# management commands), so `bin/console app:create-user …` works exactly like
# `econumo app:create-user …`. Made here (alpine has a shell); the distroless
# stage cannot RUN. The target /econumo exists in this stage AND in the final image.
RUN mkdir -p /out/bin && ln -s /econumo /out/bin/console
# Pre-create the runtime data dirs so they are owned by the non-root user in the
# final image: the sqlite db (var/db) and the auto-generated JWT keypair (var/jwt)
# must be writable on first boot, and distroless cannot mkdir/chown (no shell).
RUN mkdir -p /out/var/db /out/var/jwt

# --- 3. Runtime (distroless: no shell, no package manager, minimal surface) ---
FROM gcr.io/distroless/static-debian12 AS prod
COPY --from=gobuild /econumo /econumo
# bin/console -> /econumo symlink, resolved relative to WORKDIR below so
# `docker exec <c> bin/console app:…` works (distroless has no shell to expand a
# PATH lookup). Copy the DIRECTORY, not the link itself: BuildKit dereferences a
# directly-copied symlink (duplicating the 26MB binary) but preserves symlinks
# inside a copied directory. `docker exec <c> /econumo app:…` also works directly.
COPY --from=gobuild /out/bin /var/www/bin
COPY --from=frontend /build/web/dist/spa /app/web/spa
# Runtime data dirs, owned by the non-root user, for the sqlite db and the JWT
# keypair the server generates on first boot. Mount volumes over these to persist.
COPY --from=gobuild --chown=nonroot:nonroot /out/var /var/www/var

# Serve the SPA the frontend stage built; listen on :80 (the compose maps 8181:80).
# Sensible self-hosted defaults; override via env. The JWT keypair defaults to
# /var/www/var/jwt and is auto-generated on first boot (config defaults), so no
# key paths are baked into ENV here.
ENV ECONUMO_SPA_DIR=/app/web/spa \
    PORT=80 \
    APP_ENV=prod \
    DATABASE_URL=sqlite:///var/www/var/db/db.sqlite

EXPOSE 80

# WORKDIR so a relative `bin/console` (the symlink at /var/www/bin/console)
# resolves under `docker exec` (the /var/www working layout).
WORKDIR /var/www

# distroless ships a non-root `nonroot` user (uid 65532); the SQLite db dir +
# jwt keys mounted at runtime must be readable/writable by it.
USER nonroot:nonroot

# `serve` is explicit: the binary prints usage (and does not start a server) when
# invoked with no command, so the server start is always intentional.
ENTRYPOINT ["/econumo", "serve"]
