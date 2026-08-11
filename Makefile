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
# WHY `build` depends on `templ`: *_templ.go is generated and gitignored, so a
# clean checkout contains none of it. Measured on a fresh clone of this repo,
# the old recipe exited 2 with "no required module provides package
# github.com/keyxmakerx/chronicle/internal/templates/pages" — and left a
# previously-built ./bin/chronicle byte-identical on disk. A failed build that
# leaves a plausible artifact behind is worse than one that leaves none: every
# later check that asks "is there a binary?" instead of "did the build
# succeed?" answers yes, and you ship the old one. That is the same class of
# mistake as trusting an image label over the running process.
#
# `rm -f` first so a failure cannot masquerade as a fresh binary. Nothing here
# passes -ldflags: the Go toolchain stamps vcs.revision/vcs.time/vcs.modified
# into the binary automatically whenever `git` is on PATH and the main package
# sits in a checkout (verified with `go version -m` on the output of this
# target), and internal/hostinfo reads those stamps. A -X version variable
# would be a SECOND source of the same fact, free to disagree with the first.
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
foundry-error-catalog: ## Regenerate the foundry_vtt error-catalog.json from errors.go (C-FMC-DRIFT-GUARD)
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
	# C-SWEEP-R4 (data/fvtt-fresh-db-rename): nothing in the suite ever
	# migrated an empty database. tools/restore-drill.sh loads a dump of an
	# ALREADY-migrated one and every other integration test assumes
	# `make migrate-up` already ran — so foundry_vtt's migration 001 failed on
	# its first statement on every new self-hosted install and no test noticed.
	# These two replay the real bootstrap (core migrations → the foundry_vtt
	# pre-check + reconciler → RunPluginMigrations over registeredPlugins())
	# against a throwaway schema: one from zero, one from the pre-consolidation
	# shape. They create and drop their own scratch schema, so this never
	# touches the dev database.
	go test ./cmd/server/ -v -run 'TestFreshDatabase_|TestUpgradeDatabase_'
	# The plugin-migration damage-control layer (pre-flight applicability +
	# resume-after-partial-failure) is a claim about what the SERVER does with
	# a half-applied migration, so it is measured on one too.
	go test ./internal/database/ -v -run 'TestPluginMigration_'

.PHONY: test-probes
test-probes: ## Drive the real-browser probes (the only tests that see the RENDERED result)
	# C-SWEEP-R4 (guards/probes-never-run-in-ci). The browser probes skip under
	# `-short` — the mode BOTH `make test-unit` and `make verify` and CI's
	# "Build & Test" job run — and skip again with no Chromium, and a `go test`
	# SKIP hides inside an `ok` package line. So none of them had ever executed
	# in CI or in verify, and a machine with no browser produced a green run
	# indistinguishable from one that measured everything.
	#
	# The guard runs them WITHOUT -short and then requires a PASS from each by
	# name, so once a machine CAN drive a browser, not driving it is an error.
	# With no browser it says so loudly, naming every probe that did not run.
	./tools/check-browser-probes.sh

.PHONY: test-cover
test-cover: ## Run tests with coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

.PHONY: test-js
test-js: ## Run JS runtime tests (cal-almanac world-state spine, node --test)
	node --test test/js/*.test.mjs

# --- Linting & Security ---
# --- Local CI ---
# `make verify` runs the same sequence, in the same order, as the CI "Build &
# Test" job — so a green run here is the strongest local signal a PR will land
# green. Added by C-CALV4-FOUNDATION-P0 because five parallel calendar-v4 chats
# each needed the sequence and were each reconstructing it by hand from ci.yml.
#
# NOT included: golangci-lint (its own CI job; `make lint`), govulncheck
# (`make vuln`), and tools/test-restore-drill.sh (spins real MariaDB
# containers — too heavy for an inner-loop check; CI still runs it).
#
# The browser probes ARE included, at the end. `go test ./... -short` above
# cannot see them — that is the whole point of C-SWEEP-R4's
# guards/probes-never-run-in-ci — so verify would otherwise report a green
# sequence in which nothing had ever looked at a rendered page. On a machine
# with no Chromium the step names every probe it could not run and moves on;
# it is fatal only in CI, which sets BROWSER_PROBES_REQUIRED=1.
#
# The three diff-scoped guards resolve their base as origin/main and need real
# git history; in a shallow clone they silently report OK. Override with
# DIFF_BASE=<ref>.
.PHONY: verify
verify: ## Run the full local CI sequence (templ → build → vet → guards → go test → js test)
	@echo "==> templ generate";                templ generate
	@echo "==> go build ./...";                go build ./...
	@echo "==> go vet ./...";                  go vet ./...
	@echo "==> guard: no-instance-hostname";   ./tools/check-no-instance-hostname.sh
	@echo "==> guard: plugin-isolation (self-test)"; ./tools/test-plugin-isolation.sh
	@echo "==> guard: plugin-isolation";       ./tools/check-plugin-isolation.sh
	@echo "==> guard: migration-immutability"; ./tools/check-migration-immutability.sh
	@echo "==> guard: v2-motion-discipline";   ./tools/check-v2-motion-discipline.sh
	@echo "==> guard: calendar-v4 B1-B4";      ./tools/check-calendar-v4-lints.sh
	@echo "==> guard: decision-citations";     ./tools/check-decision-citations.sh
	@echo "==> guard: widget-mounts";          ./tools/check-widget-mounts.sh
	@echo "==> go test ./... -short";          go test ./... -short
	@echo "==> make test-js";                  $(MAKE) test-js
	@echo "==> browser probes";                $(MAKE) test-probes
	@echo "==> verify: OK"

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: check-scrub
check-scrub: ## Verify operator instance hostnames are absent from source (C-SCRUB-INSTANCE-URLS)
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
# `chronicle:local` instead of the GHCR name. WHY: while the base file declared
# both `image:` and `build:`, this target tagged a local build with
# ghcr.io/<org>/chronicle:latest — so `docker image inspect` on that tag could
# be answering about somebody's afternoon build rather than the published one,
# and `up -d` would then silently keep using it. That ambiguity cost an hour on
# 2026-08-11. See docker-compose.build.yml.
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
