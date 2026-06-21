.PHONY: help up down sh run test install dev bundle lint build go-test go-build go-image go-up go-down go-smoke go-regression go-test-cover go-test-engines go-lint go-pg-ensure

# Default target
.DEFAULT_GOAL := help

# Show available targets
help:
	@echo "Backend commands:"
	@echo "  make up           - Start application with migrations"
	@echo "  make down         - Stop application"
	@echo "  make sh           - Open shell in application container"
	@echo "  make test ARGS='...' - Run tests (recreates test DB)"
	@echo ""
	@echo "Frontend commands:"
	@echo "  make install      - Install web dependencies"
	@echo "  make dev          - Start web development server"
	@echo "  make bundle       - Bundle web for production"
	@echo "  make lint         - Run web linter"
	@echo ""
	@echo "Production:"
	@echo "  make build        - Build frontend and Docker images for production"
	@echo ""
	@echo "Go backend (drop-in rewrite, in go/):"
	@echo "  make go-smoke      - SMOKE suite: unit + sqlite + lint + coverage gate (no deps)"
	@echo "  make go-regression - REGRESSION suite: smoke + sqlite-vs-pgsql comparison"
	@echo "  make go-test       - Just the fast sqlite tests (CGO off)"
	@echo "  make go-image     - Build the Go backend Docker image"
	@echo "  make go-up        - Start the Go stack (compose, port 8182) side-by-side"
	@echo "  make go-down      - Stop the Go stack"

# Start application
up:
	@if [ ! -d vendor ]; then \
		docker-compose exec -uwww-data app composer install; \
	fi
	docker-compose up -d --build
	docker-compose exec -uwww-data app bin/console doctrine:migrations:migrate -n

# Stop application
down:
	docker-compose down --remove-orphans

# Open shell in application container
sh:
	docker-compose exec -uwww-data app sh

# Run tests
# Usage: make test ARGS='unit'
test:
	docker-compose up -d
	-docker-compose exec -uwww-data app bin/console doctrine:database:drop --force --env=test -vvv
	docker-compose exec -uwww-data app bin/console doctrine:database:create --env=test -vvv
	docker-compose exec -uwww-data app bin/console doctrine:migration:migrate -n --env=test -vvv
	docker-compose exec -uwww-data app bin/console doctrine:fixtures:load --purge-with-truncate -n --env=test -vvv
	-docker-compose exec -uwww-data app php -d register_argc_argv=1 vendor/bin/codecept run $(ARGS) --steps -v

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

# Build for production
build:
	@echo "Setting up frontend environment..."
	rm -f web/.env
	cp web/.env.ce web/.env
	@echo "Building production Docker image..."
	docker buildx build \
		--file deployment/docker/app/Dockerfile \
		--target prod \
		--tag econumo/econumo-ce:local \
		--load \
		.

# --- Go backend (drop-in rewrite) ---

# The Go suite is split into two tiers:
#
#   make go-smoke       SMOKE: unit + sqlite + lint + coverage gate. Fast, zero
#                       external dependencies. Run this constantly / before commit.
#   make go-regression  REGRESSION: everything in smoke PLUS the sqlite-vs-pgsql
#                       engine comparison against a real PostgreSQL. Run before
#                       merging / releasing.
#
# The granular targets below (go-test, go-test-cover, go-test-engines, go-lint)
# remain as the building blocks the two tiers compose.

# ---- SMOKE (unit + sqlite, no dependencies) -------------------------------

# Smoke suite: lint + the fast sqlite-only tests with a coverage gate.
go-smoke: go-lint go-test-cover
	@echo "SMOKE suite passed (unit + sqlite, no external deps)."

# Run the fast Go test suite (CGO off, sqlite-only, no external deps).
go-test:
	cd go && CGO_ENABLED=0 go test ./...

# Coverage threshold for go-test-cover (true cross-package %). Override on the
# command line: make go-test-cover GO_COVER_MIN=70
GO_COVER_MIN ?= 64

# Fast suite WITH a coverage gate: measures true cross-package coverage of all
# internal packages and fails if it drops below GO_COVER_MIN.
go-test-cover:
	cd go && CGO_ENABLED=0 go test ./... -coverpkg=./internal/... -coverprofile=coverage.out
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
go-regression: go-smoke go-pg-ensure go-test-engines
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
		--file deployment/docker/go/Dockerfile \
		--target prod \
		--tag econumo/econumo-go:local \
		--load \
		.

# Start the Go stack side-by-side with the PHP stack (host port 8182).
go-up:
	docker compose -f deployment/docker-compose/docker-compose.go.yml up -d --build

# Stop the Go stack.
go-down:
	docker compose -f deployment/docker-compose/docker-compose.go.yml down --remove-orphans
