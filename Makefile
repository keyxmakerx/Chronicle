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
# Overlay that swaps the published GHCR image for a local source build. Kept
# separate so the published tag has exactly one producer — see the file header.
DOCKER_COMP_BUILD := docker-compose.build.yml

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
# `build` depends on `templ`: *_templ.go is generated and gitignored, so a
# clean checkout has none of it. `rm -f` first so a failed build can't leave a
# stale binary that looks fresh. No -ldflags: the Go toolchain stamps
# vcs.revision/vcs.time/vcs.modified automatically when git is on PATH, and
# internal/hostinfo reads those stamps.
.PHONY: build
build: templ ## Build production binary (regenerates templ first)
	rm -f $(BUILD_DIR)/$(APP_NAME)
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)

.PHONY: clean
clean: ## Remove built artifacts
	rm -rf $(BUILD_DIR) tmp/

# --- Code Generation ---
.PHONY: templ
templ: ## Regenerate Templ .go files from .templ sources
	templ generate

.PHONY: foundry-error-catalog
foundry-error-catalog: ## Regenerate the foundry_vtt error-catalog.json from errors.go
	go run ./cmd/foundry-error-catalog

.PHONY: tailwind
tailwind: ## Regenerate Tailwind CSS
	tailwindcss -i static/css/input.css -o static/css/app.css --minify

.PHONY: tailwind-watch
tailwind-watch: ## Watch mode for Tailwind CSS
	tailwindcss -i static/css/input.css -o static/css/app.css --watch

.PHONY: tiptap-bundle
tiptap-bundle: ## Rebuild TipTap editor bundle (table extensions, etc.)
	npx esbuild static/vendor/tiptap-bundle.src.js --bundle --minify --outfile=static/vendor/tiptap-bundle.min.js --format=iife --global-name=__TipTapInternal

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

.PHONY: test-freshdb
test-freshdb: ## Replay core + every plugin migration against a NEVER-migrated schema (requires running DB)
	# Every other integration test assumes `make migrate-up` already ran, so
	# this replays the real bootstrap (core migrations -> plugin migrations)
	# against a throwaway schema. Creates and drops its own scratch schema;
	# never touches the dev database.
	go test ./cmd/server/ -v -run 'TestFreshDatabase_|TestUpgradeDatabase_'
	# Also exercises the plugin-migration pre-flight + resume-after-partial-
	# failure logic against a half-applied migration.
	go test ./internal/database/ -v -run 'TestPluginMigration_'

.PHONY: test-cover
test-cover: ## Run tests with coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

.PHONY: test-js
test-js: ## Run JS runtime tests (sidebar, availability, widgets — node --test)
	node --test test/js/*.test.mjs

# playwright is resolved from NODE_PATH (a global install), not a devDependency,
# so the repo carries no browser-sized dependency tree; CI and local setups
# both put it in the global root. Override NODE_PATH to point elsewhere.
.PHONY: test-e2e
test-e2e: ## Run Playwright browser tests (needs global playwright + chromium)
	NODE_PATH="$${NODE_PATH:-$$(npm root -g)}" node --test test/e2e/*.spec.mjs

# --- Linting & Security ---
# --- Local CI ---
# `make verify` runs the same sequence, in the same order, as the CI "Build &
# Test" job — so a green run here is the strongest local signal a PR will land
# green.
#
# NOT included: golangci-lint (its own CI job; `make lint`), govulncheck
# (`make vuln`), and tools/test-restore-drill.sh (spins real MariaDB
# containers — too heavy for an inner-loop check; CI still runs it).
#
# The diff-scoped guards resolve their base as origin/main and need real git
# history; in a shallow clone they silently report OK. Override with
# DIFF_BASE=<ref>.
.PHONY: verify
verify: ## Run the full local CI sequence (templ → build → vet → guards → go test → js test)
	@echo "==> templ generate";                templ generate
	@echo "==> go build ./...";                go build ./...
	@echo "==> go vet ./...";                  go vet ./...
	@echo "==> guard: no-instance-hostname";   ./tools/check-no-instance-hostname.sh
	@echo "==> guard: plugin-isolation (self-test)"; ./tools/test-plugin-isolation.sh
	@echo "==> guard: plugin-isolation";       ./tools/check-plugin-isolation.sh
	@echo "==> guard: comment-clutter (self-test)"; ./tools/test-comment-clutter.sh
	@echo "==> guard: comment-clutter";        ./tools/check-comment-clutter.sh
	@echo "==> guard: migration-immutability"; ./tools/check-migration-immutability.sh
	@echo "==> guard: v2-motion-discipline";   ./tools/check-v2-motion-discipline.sh
	@echo "==> guard: page-scripts";           ./tools/check-page-scripts.sh
	@echo "==> guard: decision-citations";     ./tools/check-decision-citations.sh
	@echo "==> guard: widget-mounts";          ./tools/check-widget-mounts.sh
	@echo "==> guard: customize-discipline";   ./tools/check-customize-discipline.sh
	@echo "==> guard: templ-packages";         ./tools/check-templ-packages.sh
	@echo "==> go test ./... -short";          go test ./... -short
	@echo "==> make test-js";                  $(MAKE) test-js
	@echo "==> verify: OK"

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: check-scrub
check-scrub: ## Verify operator instance hostnames are absent from source
	./tools/check-no-instance-hostname.sh

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
test-db-up: ## Start a local MariaDB for integration tests WITHOUT Docker (port 13306)
	@./tools/start-test-db.sh

test-db-down: ## Stop the local test MariaDB
	@./tools/start-test-db.sh --stop

test-int-local: ## Run integration tests against the local test MariaDB (starts it if needed)
	@./tools/start-test-db.sh >/dev/null
	@CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./... -count=1

docker-up: ## Start MariaDB + Redis containers
	docker compose -f $(DOCKER_COMP) up -d chronicle-db chronicle-redis

.PHONY: docker-down
docker-down: ## Stop all containers
	docker compose -f $(DOCKER_COMP) down

.PHONY: docker-logs
docker-logs: ## Tail container logs
	docker compose -f $(DOCKER_COMP) logs -f

# Building from source goes through the override file, which tags the result
# `chronicle:local` instead of the GHCR name, so a local build can never be
# mistaken for (or silently replace) the published image. See
# docker-compose.build.yml.
.PHONY: docker-build
docker-build: ## Build the Chronicle Docker image locally (tags chronicle:local)
	docker compose -f $(DOCKER_COMP) -f $(DOCKER_COMP_BUILD) build chronicle

.PHONY: docker-all
docker-all: ## Start full stack from the PUBLISHED image (pulls first)
	docker compose -f $(DOCKER_COMP) up -d

.PHONY: docker-all-local
docker-all-local: ## Start full stack from a LOCAL source build (chronicle:local)
	docker compose -f $(DOCKER_COMP) -f $(DOCKER_COMP_BUILD) up -d --build

# ============================================================================
# Backup & Restore
# ============================================================================
# Operator-facing wrappers around scripts/backup.sh and scripts/restore.sh.
# All targets invoke the script inside the chronicle container, where
# mariadb-client and the data volume are already in place. See
# docs/deployment.md for the full operator runbook.

.PHONY: backup
backup: ## Snapshot DB + media to $$BACKUP_DIR (pass BACKUP_ARGS for flags)
	docker compose -f $(DOCKER_COMP) exec -T chronicle /app/scripts/backup.sh $(BACKUP_ARGS)

.PHONY: backup-check
backup-check: ## Validate backup environment without writing anything
	docker compose -f $(DOCKER_COMP) exec -T chronicle /app/scripts/backup.sh --check

.PHONY: backup-list
backup-list: ## List backup artifacts in the chronicle-data volume
	docker compose -f $(DOCKER_COMP) exec -T chronicle ls -lh /app/data/backups

.PHONY: restore
restore: ## Restore from a manifest (usage: make restore RESTORE_ARGS="--manifest=/app/data/backups/chronicle_manifest_TS.txt")
	docker compose -f $(DOCKER_COMP) exec chronicle /app/scripts/restore.sh $(RESTORE_ARGS)
