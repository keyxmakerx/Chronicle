# ============================================================================
# Chronicle Makefile
# ============================================================================
# All development commands for the Chronicle project.
# Run `make help` for a list of available targets.
# ============================================================================

# --- Variables ---
APP_NAME    := chronicle
BUILD_DIR   := ./bin
MAIN_PKG    := ./cmd/server
MIGRATIONS  := ./db/migrations
DOCKER_COMP := docker-compose.yml

# Database URL for migrations (override via env or .env file)
DATABASE_URL ?= mysql://chronicle:chronicle@tcp(localhost:3306)/chronicle

# --- Help ---
.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# --- Development ---
.PHONY: dev
dev: ## Start dev server with hot reload (air)
	air

.PHONY: run
run: ## Run the server directly (no hot reload)
	go run $(MAIN_PKG)

# --- Build ---
.PHONY: build
build: ## Build production binary
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)

.PHONY: clean
clean: ## Remove built artifacts
	rm -rf $(BUILD_DIR) tmp/

# --- Code Generation ---
.PHONY: templ
templ: ## Regenerate Templ .go files from .templ sources
	templ generate

.PHONY: tailwind
tailwind: ## Regenerate Tailwind CSS
	tailwindcss -i static/css/input.css -o static/css/app.css --minify

.PHONY: tailwind-watch
tailwind-watch: ## Watch mode for Tailwind CSS
	tailwindcss -i static/css/input.css -o static/css/app.css --watch

.PHONY: generate
generate: templ tailwind ## Run all code generation (templ + tailwind)

# --- Testing ---
.PHONY: test
test: ## Run all tests
	go test ./... -v

.PHONY: test-unit
test-unit: ## Run unit tests only (skip integration)
	go test ./... -v -short

.PHONY: test-int
test-int: ## Run integration tests (requires running DB)
	go test ./... -v -run Integration

.PHONY: test-cover
test-cover: ## Run tests with coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# --- Linting & Security ---
.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: security
security: ## Run gosec security scanner
	gosec ./...

.PHONY: vuln
vuln: ## Run govulncheck dependency vulnerability scanner
	govulncheck ./...

# --- Database Migrations ---
.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Rollback last migration
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" down 1

.PHONY: migrate-create
migrate-create: ## Create new migration (usage: make migrate-create NAME=description)
	migrate create -ext sql -dir $(MIGRATIONS) -seq $(NAME)

.PHONY: migrate-status
migrate-status: ## Show current migration version
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" version

.PHONY: seed
seed: ## Seed dev database with sample data (TODO: implement cmd/seed)
	@echo "cmd/seed not yet implemented. Default entity types are seeded automatically when creating a campaign."

# --- Docker ---
.PHONY: docker-up
docker-up: ## Start MariaDB + Redis containers
	docker compose -f $(DOCKER_COMP) up -d chronicle-db chronicle-redis

.PHONY: docker-down
docker-down: ## Stop all containers
	docker compose -f $(DOCKER_COMP) down

.PHONY: docker-logs
docker-logs: ## Tail container logs
	docker compose -f $(DOCKER_COMP) logs -f

.PHONY: docker-build
docker-build: ## Build the Chronicle Docker image
	docker compose -f $(DOCKER_COMP) build chronicle

.PHONY: docker-all
docker-all: ## Start full stack (app + db + redis)
	docker compose -f $(DOCKER_COMP) up -d
