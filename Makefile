.PHONY: help up down sh run test install dev bundle lint build go-test go-build go-image go-up go-down

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
	@echo "  make go-test      - Run the Go test suite (CGO off)"
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

# Run the fast Go test suite (CGO off, sqlite-only, no external deps).
go-test:
	cd go && CGO_ENABLED=0 go test ./...

# Coverage threshold for go-test-cover (true cross-package %). Override on the
# command line: make go-test-cover GO_COVER_MIN=70
GO_COVER_MIN ?= 64

# Fast suite WITH a coverage gate: measures true cross-package coverage of all
# internal packages and fails if it drops below GO_COVER_MIN. This is the gate
# that makes refactoring safe — run it before committing.
go-test-cover:
	cd go && CGO_ENABLED=0 go test ./... -coverpkg=./internal/... -coverprofile=coverage.out
	cd go && go tool cover -func=coverage.out | tail -1
	@cd go && pct=$$(go tool cover -func=coverage.out | tail -1 | grep -oE '[0-9]+\.[0-9]+' | tail -1); \
		echo "total coverage: $$pct% (min $(GO_COVER_MIN)%)"; \
		awk "BEGIN{exit !($$pct >= $(GO_COVER_MIN))}" || \
		{ echo "FAIL: coverage $$pct% is below the $(GO_COVER_MIN)% gate"; exit 1; }

# Engine-comparison suite: runs each repo operation against BOTH sqlite and a
# real PostgreSQL and asserts identical results. Requires DATABASE_TEST_PGSQL_URL
# (the pgsql half SKIPS without it; the sqlite half still runs). Build-tagged so
# it never slows the fast suite.
#   make go-test-engines DATABASE_TEST_PGSQL_URL=postgres://econumo:econumo@localhost:5432/econumo_test?sslmode=disable
go-test-engines:
	cd go && CGO_ENABLED=0 go test -tags enginecompare ./...

# Lint gate: build, vet, and gofmt check (fails if any file is unformatted).
go-lint:
	cd go && CGO_ENABLED=0 go build ./...
	cd go && CGO_ENABLED=0 go vet ./...
	@cd go && unformatted=$$(gofmt -l . | grep -v '/gen/' || true); \
		if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi; \
		echo "gofmt: clean"

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
