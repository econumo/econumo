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

# Run the Go test suite (CGO off, matching the build).
go-test:
	cd go && CGO_ENABLED=0 go test ./...

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
