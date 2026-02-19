# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Auto-migrations, first-user-is-admin, /health alias

## Current Phase
**Phase 1: Foundation** -- Core infrastructure, auth, campaigns, SMTP, admin,
entities plugins, UI layouts, editor widget, and UI polish are built. App
compiles and tests pass. CI/CD pipeline configured. Docker image builds and
pushes to GHCR. Production deployment hardened with DB/Redis retry logic,
real health checks, separate DB env vars, and auto-migrations on startup.
Next: @mentions, password reset, deploy testing on Cosmos Cloud.

## Last Session Summary

### Completed
- **Auto-migrations on startup:** Added `golang-migrate/migrate/v4` as a library
  dependency. New `database.RunMigrations()` runs all pending migrations
  automatically when the app starts. No more manual `make migrate-up` needed
  after deployment. Already-applied migrations are safely skipped.
- **First user is admin:** The first user to register automatically gets
  `is_admin = true`. Subsequent users get `false` as before. Uses existing
  `CountUsers()` to check if the users table is empty.
- **`/health` alias:** Added `/health` alongside `/healthz` so both paths work
  for health probes. Cosmos Cloud's default health check hits `/health`.
- **Dockerfile fix:** Migrations now copied to `/app/db/migrations` (was
  `/app/migrations`) so the path matches local dev (`db/migrations`).
- **Simplified DB config:** (prior commit) `DB_HOST=host:port`, no DB_PORT.
- **Special-char-safe DSN:** (prior commit) `mysql.Config.FormatDSN()`.
- **DB/Redis retry-with-backoff:** (prior commit) Eliminates crash-loop restarts.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `internal/database/migrate.go` -- New file: RunMigrations() using golang-migrate
- `cmd/server/main.go` -- Call RunMigrations() after DB connect
- `internal/plugins/auth/service.go` -- First user gets IsAdmin=true
- `internal/app/routes.go` -- /health alias for /healthz
- `Dockerfile` -- Migrations copy to db/migrations
- `.ai/status.md` -- Updated

## Active Branch
`claude/setup-ai-project-docs-LhvVz`

## Next Session Should
1. **Deploy testing** -- Pull new image on server, `docker compose down -v && up -d`,
   verify app starts cleanly, test `/healthz`, create account, create campaign
2. **@mentions** -- Search entities, insert link, parse/render server-side
3. **Password reset** -- Wire auth password reset with SMTP when configured

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone

## Recently Completed Milestones
- 2026-02-19: Project scaffolding and three-tier AI documentation system
- 2026-02-19: Core infrastructure (config, database, middleware, app, server)
- 2026-02-19: Security middleware (proxy trust, CORS, CSRF, security headers)
- 2026-02-19: Auth plugin (register, login, logout, session management)
- 2026-02-19: Campaigns plugin (CRUD, roles, membership, ownership transfer)
- 2026-02-19: SMTP plugin (encrypted password, STARTTLS/SSL, test connection)
- 2026-02-19: Admin plugin (user management, campaign oversight, SMTP config)
- 2026-02-19: Entities plugin (CRUD, entity types, FULLTEXT search, privacy, dynamic fields)
- 2026-02-19: UI & Layouts (dynamic sidebar, topbar, pagination, flash messages, error pages)
- 2026-02-19: Editor widget (TipTap integration, boot.js auto-mounter, entry API)
- 2026-02-19: Vendor HTMX + Alpine.js, campaign selector dropdown
- 2026-02-19: UI polish (light theme unification, CSS component library, landing page)
- 2026-02-19: Entity service unit tests (30 tests passing)
- 2026-02-19: Dockerfile fixed for production (Go 1.24, pinned Tailwind)
- 2026-02-19: CI/CD pipeline (GitHub Actions: build, test, Docker push to GHCR)
- 2026-02-19: Production deployment hardening (retry logic, real healthcheck, credential sync)
- 2026-02-19: Separate DB env vars for Cosmos Cloud compatibility
- 2026-02-19: Auto-migrations on startup, first-user-is-admin, /health alias
