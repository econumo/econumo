# Econumo backend: one static Go binary serving the JSON API + SPA on a single port.
# Build context is the repo root:
#   docker build -f deployment/docker/Dockerfile -t ghcr.io/econumo/econumo:latest .
# DATABASE_URL comes from the environment (.env), not baked in. The JWT keypair is
# auto-generated on first boot under /app/var; mount a volume there to persist it
# and the sqlite db. Migrations run inside the binary on boot.

# --- 1. Frontend build ---
FROM node:25-alpine AS frontend
WORKDIR /build/web
RUN npm install -g pnpm@10
COPY web/package.json web/pnpm-lock.yaml* ./
RUN pnpm install --shamefully-hoist
COPY web/ ./
# web/.env is gitignored, so copy the committed template for a deterministic build.
# Pass --build-arg ECONUMO_VERSION=v1.2.3 to set the UI version label.
ARG ECONUMO_VERSION=""
RUN cp .env.dist .env && \
    if [ -n "$ECONUMO_VERSION" ]; then \
        sed -i "s/^ECONUMO_VERSION=.*/ECONUMO_VERSION=\"${ECONUMO_VERSION}\"/" .env; \
    fi
RUN pnpm run build

# --- 2. Go build (CGO off -> fully static binary) ---
FROM golang:1.26-alpine AS gobuild
WORKDIR /src
# Module files first for layer-cached `go mod download`.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /app/econumo ./cmd/econumo
# Pre-create runtime dirs owned by the non-root user (distroless can't mkdir on boot).
RUN mkdir -p /out/var/db /out/var/jwt

# --- 3. Runtime (distroless: no shell, minimal surface) ---
FROM gcr.io/distroless/static-debian12 AS prod
COPY --from=gobuild /app/econumo /app/econumo
COPY --from=frontend /build/web/dist/spa /app/spa
# Runtime data dir (sqlite db + auto-generated JWT keypair); mount a volume here to persist.
COPY --from=gobuild --chown=nonroot:nonroot /out/var /app/var

ENV ECONUMO_WEB_DIST=/app/spa \
    PORT=80

EXPOSE 80
WORKDIR /app
USER nonroot:nonroot

# `serve` is explicit: the binary prints usage when invoked with no command.
ENTRYPOINT ["/app/econumo", "serve"]
