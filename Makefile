.PHONY: help install dev bundle lint go-test go-build go-image go-up go-down go-test-fast go-regression go-test-cover go-test-engines go-lint go-pg-ensure publish publish-buildx-ensure

# Default target
.DEFAULT_GOAL := help

# Show available targets
help:
	@echo "Frontend (web/):"
	@echo "  make install      - Install web dependencies"
	@echo "  make dev          - Start web development server"
	@echo "  make bundle       - Bundle web for production"
	@echo "  make lint         - Run web linter"
	@echo ""
	@echo "Go backend (go/):"
	@echo "  make go-test       - SMOKE suite: unit + sqlite + lint + coverage gate (no deps)"
	@echo "  make go-regression - REGRESSION suite: go-test + sqlite-vs-pgsql comparison"
	@echo "  make go-test-fast  - Just the fast sqlite tests, no lint/coverage (CGO off)"
	@echo "  make go-image     - Build the Go backend Docker image"
	@echo "  make go-up        - Start the Go stack (compose)"
	@echo "  make go-down      - Stop the Go stack"
	@echo "  make publish      - Build + push the multi-arch 'dev' image to $(GHCR_IMAGE)"

# Install web dependencies
install:
	@echo "Installing web dependencies..."
	cd web && pnpm install

# Start web development server
dev:
	@echo "Starting web development server..."
	cd web && npm run dev

# Bundle web for production
bundle:
	@echo "Bundling web for production..."
	cd web && npm run build

# Run web linter
lint:
	@echo "Running web linter..."
	cd web && npm run lint

# --- Go backend ---

# The Go suite is split into two tiers:
#
#   make go-test        SMOKE: unit + sqlite + lint + coverage gate. Fast, zero
#                       external dependencies. Run this constantly / before commit.
#   make go-regression  REGRESSION: everything in go-test PLUS the sqlite-vs-pgsql
#                       engine comparison against a real PostgreSQL. Run before
#                       merging / releasing.
#
# The granular targets below (go-test-fast, go-test-cover, go-test-engines,
# go-lint) remain as the building blocks the two tiers compose.

# ---- SMOKE (unit + sqlite, no dependencies) -------------------------------

# Smoke suite: lint + the sqlite-only tests with a coverage gate. The everyday
# command; no external dependencies.
go-test: go-lint go-test-cover
	@echo "SMOKE suite passed (unit + sqlite, no external deps)."

# Just the fast sqlite-only tests, no lint/coverage (CGO off). A building block.
go-test-fast:
	cd go && CGO_ENABLED=0 go test ./...

# Coverage threshold for go-test-cover (true cross-package %). Override on the
# command line: make go-test-cover GO_COVER_MIN=70
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
go-test-cover:
	cd go && CGO_ENABLED=0 go test -count=1 ./... -coverpkg=./internal/... -coverprofile=coverage.out
	cd go && go tool cover -func=coverage.out | tail -1
	@cd go && pct=$$(go tool cover -func=coverage.out | tail -1 | grep -oE '[0-9]+\.[0-9]+' | tail -1); \
		echo "total coverage: $$pct% (min $(GO_COVER_MIN)%)"; \
		awk "BEGIN{exit !($$pct >= $(GO_COVER_MIN))}" || \
		{ echo "FAIL: coverage $$pct% is below the $(GO_COVER_MIN)% gate"; exit 1; }

# Lint gate: build, vet, and gofmt check (fails if any file is unformatted).
go-lint:
	cd go && CGO_ENABLED=0 go build ./...
	cd go && CGO_ENABLED=0 go vet ./...
	@cd go && unformatted=$$(gofmt -l . | grep -v '/gen/' || true); \
		if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi; \
		echo "gofmt: clean"

# ---- REGRESSION (smoke + sqlite-vs-pgsql comparison) ----------------------

# Where the regression suite finds PostgreSQL. If DATABASE_TEST_PGSQL_URL is set
# in the environment it is used as-is; otherwise go-regression auto-provisions a
# throwaway DB in the compose `postgres` service at this URL.
DATABASE_TEST_PGSQL_URL ?= postgres://econumo:econumo@localhost:5432/econumo_test?sslmode=disable

# Regression suite: the full smoke suite + the engine-comparison suite against a
# real PostgreSQL. If no Postgres is reachable it auto-creates a throwaway test
# DB in the compose `postgres` service (start it with `make go-up` or
# `docker compose up -d postgres` first).
go-regression: go-test go-pg-ensure go-test-engines
	@echo "REGRESSION suite passed (smoke + sqlite-vs-pgsql comparison)."

# Ensure the throwaway test database exists in the compose postgres service.
# No-op if it already exists; harmless if you point DATABASE_TEST_PGSQL_URL at
# an external Postgres (the CREATE just runs against the compose one).
go-pg-ensure:
	@docker compose exec -T postgres psql -U econumo -d econumo -tAc \
		"SELECT 1 FROM pg_database WHERE datname='econumo_test'" 2>/dev/null | grep -q 1 \
		|| docker compose exec -T postgres psql -U econumo -d econumo -c "CREATE DATABASE econumo_test" \
		|| echo "NOTE: could not auto-create econumo_test (set DATABASE_TEST_PGSQL_URL to an existing DB)"

# Engine-comparison suite: runs each repo operation against BOTH sqlite and the
# PostgreSQL at DATABASE_TEST_PGSQL_URL and asserts identical results. The pgsql
# half SKIPS if the URL is empty/unreachable; the sqlite half still runs.
go-test-engines:
	cd go && CGO_ENABLED=0 DATABASE_TEST_PGSQL_URL='$(DATABASE_TEST_PGSQL_URL)' \
		go test -tags enginecompare ./...

# Build the Go backend Docker image (context is the repo root).
go-image:
	@echo "Building Go backend Docker image..."
	docker buildx build \
		--file deployment/docker/Dockerfile \
		--target prod \
		--tag econumo/econumo-go:local \
		--load \
		.

# --- Publishing (GitHub Container Registry only) ---------------------------
# `make publish` builds the multi-arch Go image locally and pushes the "dev"
# tag to ghcr.io/econumo/econumo. Releases ("latest" + vX.Y.Z) are published by
# the GitHub release workflow, NOT here. Requires `docker login ghcr.io` first.
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

# Build + run the Go stack locally from source (host port 8182).
go-up:
	docker compose -f deployment/docker-compose/docker-compose.go.yml up -d --build

# Stop the Go stack.
go-down:
	docker compose -f deployment/docker-compose/docker-compose.go.yml down --remove-orphans
