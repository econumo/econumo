.PHONY: help web-install web-dev web-bundle web-lint web-react-install web-react-dev web-react-test web-react-lint web-react-bundle go-build go-run test test-cover go-lint regression test-engines pg-ensure up down publish publish-buildx-ensure swagger swagger-check

# Default target
.DEFAULT_GOAL := help

# Show available targets
help:
	@echo "Backend (Go):"
	@echo "  make go-build     - Compile the binary to ./econumo (CGO off)"
	@echo "  make go-run       - Run the server locally (go run ./cmd/econumo serve, reads .env)"
	@echo "  make test         - SMOKE suite: unit + sqlite + lint + coverage gate (no deps)"
	@echo "  make regression   - REGRESSION suite: test + sqlite-vs-pgsql comparison"
	@echo "  make go-lint      - build + vet + gofmt + OpenAPI-docs-fresh check"
	@echo "  make swagger      - Regenerate the OpenAPI docs (internal/ui/apidoc/docs)"
	@echo "  make up           - Start the stack (compose, builds from source)"
	@echo "  make down         - Stop the stack"
	@echo "  make publish      - Build + push the multi-arch 'dev' image to $(GHCR_IMAGE)"
	@echo ""
	@echo "Frontend (web/):"
	@echo "  make web-install  - Install web dependencies"
	@echo "  make web-dev      - Start web development server"
	@echo "  make web-bundle   - Bundle web for production"
	@echo "  make web-lint     - Run web linter"
	@echo ""
	@echo "Frontend rewrite (web-react/):"
	@echo "  make web-react-install - Install web-react dependencies"
	@echo "  make web-react-dev     - Start web-react development server"
	@echo "  make web-react-test    - Run web-react tests"
	@echo "  make web-react-lint    - Run web-react linter"
	@echo "  make web-react-bundle  - Bundle web-react for production"

# --- Frontend (web/) ---

web-install:
	@echo "Installing web dependencies..."
	cd web && pnpm install

web-dev:
	@echo "Starting web development server..."
	cd web && npm run dev

web-bundle:
	@echo "Bundling web for production..."
	cd web && npm run build

web-lint:
	@echo "Running web linter..."
	cd web && npm run lint

# --- Frontend rewrite (web-react/) ---

web-react-install:
	@echo "Installing web-react dependencies..."
	cd web-react && pnpm install

web-react-dev:
	@echo "Starting web-react development server..."
	cd web-react && pnpm dev

web-react-test:
	@echo "Running web-react tests..."
	cd web-react && pnpm test

web-react-lint:
	@echo "Running web-react linter..."
	cd web-react && pnpm lint

web-react-bundle:
	@echo "Bundling web-react for production..."
	cd web-react && pnpm build

# --- Backend (Go) ---

# Compile the self-contained binary to ./econumo (gitignored). CGO off so the
# pure-Go sqlite/pgx drivers are linked in, matching the production build.
# Depends on `swagger` so the embedded OpenAPI docs are always regenerated from
# the current handler annotations before the binary is built.
go-build: swagger
	CGO_ENABLED=0 go build -o econumo ./cmd/econumo

# Run the server locally without Docker. All configuration (PORT, DATABASE_URL, …)
# comes from ./.env, which the binary auto-loads — copy .env.example to .env first.
# Migrations run on boot and the JWT keypair is generated if missing.
# Regenerates the OpenAPI docs first (see go-build).
go-run: swagger
	go run ./cmd/econumo serve

# The Go suite is split into two tiers:
#
#   make test        SMOKE: unit + sqlite + lint + coverage gate. Fast, zero
#                    external dependencies. Run this constantly / before commit.
#   make regression  REGRESSION: everything in test PLUS the sqlite-vs-pgsql
#                    engine comparison against a real PostgreSQL. Run before
#                    merging / releasing.
#
# The granular targets below (test-cover, test-engines, go-lint) remain as the
# building blocks the two tiers compose.

# ---- SMOKE (unit + sqlite, no dependencies) -------------------------------

# Smoke suite: lint + the sqlite-only tests with a coverage gate. The everyday
# command; no external dependencies.
test: go-lint test-cover
	@echo "SMOKE suite passed (unit + sqlite, no external deps)."

# Coverage threshold for test-cover (true cross-package %). Override on the
# command line: make test-cover GO_COVER_MIN=70
GO_COVER_MIN ?= 64

# Fast suite WITH a coverage gate: measures true cross-package coverage of all
# internal + pkg packages and fails if it drops below GO_COVER_MIN.
#
# -count=1 forces every package's tests to actually run. Without it, `go test`
# replays cached results — and a cached package's coverage is printed but NOT
# merged into -coverprofile, so the total silently undercounts whenever the test
# cache is warm (e.g. CI restoring the Go build cache between runs). That made the
# gate nondeterministic: the same commit scored ~66% cold and ~63% warm. Forcing
# a fresh run keeps the merged profile complete and the gate reproducible.
test-cover:
	CGO_ENABLED=0 go test -count=1 ./... -coverpkg=./internal/...,./pkg/... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1
	@pct=$$(go tool cover -func=coverage.out | tail -1 | grep -oE '[0-9]+\.[0-9]+' | tail -1); \
		echo "total coverage: $$pct% (min $(GO_COVER_MIN)%)"; \
		awk "BEGIN{exit !($$pct >= $(GO_COVER_MIN))}" || \
		{ echo "FAIL: coverage $$pct% is below the $(GO_COVER_MIN)% gate"; exit 1; }

# Lint gate: build, vet, gofmt check (fails if any file is unformatted), and an
# OpenAPI-docs-fresh check (fails if the committed docs are stale vs the current
# annotations — keeps the published spec honest; run `make swagger` to fix).
go-lint: swagger-check
	CGO_ENABLED=0 go build ./...
	CGO_ENABLED=0 go vet ./...
	@unformatted=$$(gofmt -l . | grep -v '/gen/' || true); \
		if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi; \
		echo "gofmt: clean"

# ---- OpenAPI docs (swaggo/swag) -------------------------------------------

# swag is pinned to the version in go.mod so generation is reproducible (the
# go:generate directive in internal/ui/apidoc/doc.go uses @latest for ad-hoc
# runs; the build pipeline uses this pinned version). `go run <pkg>@<ver>` needs
# the module cache (network on first use).
SWAG_VERSION := $(shell go list -m -f '{{.Version}}' github.com/swaggo/swag 2>/dev/null || echo v1.16.6)
SWAG_INIT     = go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) init -g doc.go -d .,../handler,../../app --parseInternal --parseDependency

# Regenerate the committed OpenAPI docs from the handler/DTO annotations. This is
# a prerequisite of go-build / go-run / publish / up so a built artifact never
# embeds stale docs.
swagger:
	@echo "Regenerating OpenAPI docs (swag $(SWAG_VERSION))..."
	cd internal/ui/apidoc && $(SWAG_INIT) -o ./docs

# Fail if the committed docs differ from a fresh generation (stale annotations
# not regenerated + committed). Generates into a temp dir and diffs, so it never
# mutates the working tree. Wired into go-lint (and thus `make test` / CI).
swagger-check:
	@tmp=$$(mktemp -d); \
		( cd internal/ui/apidoc && $(SWAG_INIT) -o "$$tmp" >/dev/null 2>&1 ) || \
			{ echo "FAIL: could not run swag (need network/module cache for swag $(SWAG_VERSION))"; rm -rf "$$tmp"; exit 1; }; \
		if ! diff -q "$$tmp/swagger.json" internal/ui/apidoc/docs/swagger.json >/dev/null 2>&1; then \
			echo "FAIL: OpenAPI docs are stale — run 'make swagger' and commit the result"; rm -rf "$$tmp"; exit 1; fi; \
		rm -rf "$$tmp"; echo "swagger: docs up to date"

# ---- REGRESSION (smoke + sqlite-vs-pgsql comparison) ----------------------

# Where the regression suite finds PostgreSQL. If DATABASE_TEST_PGSQL_URL is set
# in the environment it is used as-is; otherwise regression auto-provisions a
# throwaway DB in the compose `postgres` service at this URL.
DATABASE_TEST_PGSQL_URL ?= postgres://econumo:econumo@localhost:5432/econumo_test?sslmode=disable

# Regression suite: the full smoke suite + the engine-comparison suite against a
# real PostgreSQL. If no Postgres is reachable it auto-creates a throwaway test
# DB in the compose `postgres` service (start it with `make up` or
# `docker compose up -d postgres` first).
regression: test pg-ensure test-engines
	@echo "REGRESSION suite passed (smoke + sqlite-vs-pgsql comparison)."

# Ensure the throwaway test database exists in the compose postgres service.
# No-op if it already exists; harmless if you point DATABASE_TEST_PGSQL_URL at
# an external Postgres (the CREATE just runs against the compose one).
pg-ensure:
	@docker compose exec -T postgres psql -U econumo -d econumo -tAc \
		"SELECT 1 FROM pg_database WHERE datname='econumo_test'" 2>/dev/null | grep -q 1 \
		|| docker compose exec -T postgres psql -U econumo -d econumo -c "CREATE DATABASE econumo_test" \
		|| echo "NOTE: could not auto-create econumo_test (set DATABASE_TEST_PGSQL_URL to an existing DB)"

# Engine-comparison suite: runs each repo operation against BOTH sqlite and the
# PostgreSQL at DATABASE_TEST_PGSQL_URL and asserts identical results. The pgsql
# half SKIPS if the URL is empty/unreachable; the sqlite half still runs.
test-engines:
	CGO_ENABLED=0 DATABASE_TEST_PGSQL_URL='$(DATABASE_TEST_PGSQL_URL)' \
		go test -tags enginecompare ./...

# --- Publishing (GitHub Container Registry only) ---------------------------
# `make publish` builds the multi-arch image locally and pushes the "dev" tag to
# ghcr.io/econumo/econumo. Releases ("latest" + vX.Y.Z) are published by the
# GitHub release workflow, NOT here. Requires `docker login ghcr.io` first.
# Override any of these on the command line, e.g. `make publish PUBLISH_TAG=foo`.
GHCR_IMAGE        ?= ghcr.io/econumo/econumo
PUBLISH_TAG       ?= dev
PUBLISH_PLATFORMS ?= linux/amd64,linux/arm64
BUILDX_BUILDER    ?= econumo-mb
ECONUMO_VERSION   ?= dev

# Ensure a docker-container buildx builder exists (multi-arch push needs it).
publish-buildx-ensure:
	@docker buildx inspect $(BUILDX_BUILDER) >/dev/null 2>&1 || \
		docker buildx create --name $(BUILDX_BUILDER) --driver docker-container --bootstrap

# Regenerates the OpenAPI docs (swagger) first so the image built from source
# embeds the current spec.
publish: swagger publish-buildx-ensure
	@echo "Publishing $(GHCR_IMAGE):$(PUBLISH_TAG) ($(PUBLISH_PLATFORMS))..."
	docker buildx build \
		--builder $(BUILDX_BUILDER) \
		--platform $(PUBLISH_PLATFORMS) \
		--file deployment/docker/Dockerfile \
		--target prod \
		--build-arg ECONUMO_VERSION=$(ECONUMO_VERSION) \
		--tag $(GHCR_IMAGE):$(PUBLISH_TAG) \
		--push \
		.

# Start the stack locally, building the image from source (host port 8181).
# Regenerates the OpenAPI docs first so the built image embeds the current spec.
up: swagger
	docker compose up -d --build

# Stop the stack.
down:
	docker compose down --remove-orphans
