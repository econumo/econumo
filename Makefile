.PHONY: help web-install web-dev web-bundle web-lint go-build go-run test test-cover go-lint regression test-engines pg-ensure up down publish publish-buildx-ensure

# Default target
.DEFAULT_GOAL := help

# Show available targets
help:
	@echo "Backend (Go):"
	@echo "  make go-build     - Compile the binary to ./econumo (CGO off)"
	@echo "  make go-run       - Run the server locally (go run ./cmd/econumo serve, reads .env)"
	@echo "  make test         - SMOKE suite: unit + sqlite + lint + coverage gate (no deps)"
	@echo "  make regression   - REGRESSION suite: test + sqlite-vs-pgsql comparison"
	@echo "  make go-lint      - build + vet + gofmt check"
	@echo "  make up           - Start the stack (compose, builds from source)"
	@echo "  make down         - Stop the stack"
	@echo "  make publish      - Build + push the multi-arch 'dev' image to $(GHCR_IMAGE)"
	@echo ""
	@echo "Frontend (web/):"
	@echo "  make web-install  - Install web dependencies"
	@echo "  make web-dev      - Start web development server"
	@echo "  make web-bundle   - Bundle web for production"
	@echo "  make web-lint     - Run web linter"

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

# --- Backend (Go) ---

# Compile the self-contained binary to ./econumo (gitignored). CGO off so the
# pure-Go sqlite/pgx drivers are linked in, matching the production build.
go-build:
	CGO_ENABLED=0 go build -o econumo ./cmd/econumo

# Run the server locally without Docker. The listen port and a local sqlite DB are
# passed explicitly (PORT/DATABASE_URL are no longer kept in .env — they are build-
# /run-controlled); ./.env is still auto-loaded for the rest. Migrations run on boot
# and the JWT keypair is generated if missing. Override: make go-run RUN_PORT=9000
RUN_PORT         ?= 8181
RUN_DATABASE_URL ?= sqlite://var/db/db.sqlite
go-run:
	@mkdir -p var/db
	APP_ENV=dev PORT=$(RUN_PORT) DATABASE_URL=$(RUN_DATABASE_URL) go run ./cmd/econumo serve

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
# internal packages and fails if it drops below GO_COVER_MIN.
#
# -count=1 forces every package's tests to actually run. Without it, `go test`
# replays cached results — and a cached package's coverage is printed but NOT
# merged into -coverprofile, so the total silently undercounts whenever the test
# cache is warm (e.g. CI restoring the Go build cache between runs). That made the
# gate nondeterministic: the same commit scored ~66% cold and ~63% warm. Forcing
# a fresh run keeps the merged profile complete and the gate reproducible.
test-cover:
	CGO_ENABLED=0 go test -count=1 ./... -coverpkg=./internal/... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1
	@pct=$$(go tool cover -func=coverage.out | tail -1 | grep -oE '[0-9]+\.[0-9]+' | tail -1); \
		echo "total coverage: $$pct% (min $(GO_COVER_MIN)%)"; \
		awk "BEGIN{exit !($$pct >= $(GO_COVER_MIN))}" || \
		{ echo "FAIL: coverage $$pct% is below the $(GO_COVER_MIN)% gate"; exit 1; }

# Lint gate: build, vet, and gofmt check (fails if any file is unformatted).
go-lint:
	CGO_ENABLED=0 go build ./...
	CGO_ENABLED=0 go vet ./...
	@unformatted=$$(gofmt -l . | grep -v '/gen/' || true); \
		if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi; \
		echo "gofmt: clean"

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

publish: publish-buildx-ensure
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
up:
	docker compose up -d --build

# Stop the stack.
down:
	docker compose down --remove-orphans
