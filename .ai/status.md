# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Production deployment fixes and test readiness session

## Current Phase
**Phase 1: Foundation** -- Core infrastructure, auth, campaigns, SMTP, admin,
entities plugins, UI layouts, editor widget, and UI polish are built. App
compiles and tests pass. CI/CD pipeline configured. Docker image builds and
pushes to GHCR. Production deployment hardened with DB/Redis retry logic,
real health checks, and credential-synced docker-compose.
Next: @mentions, password reset, deploy testing on Cosmos Cloud.

## Last Session Summary

### Completed
- **DB/Redis retry-with-backoff:** `NewMariaDB()` and `NewRedis()` now retry
  up to 10 times with exponential backoff (1s→30s cap). Eliminates crash-loop
  restarts during Docker Compose cold-starts.
- **Real `/healthz` endpoint:** Pings both MariaDB and Redis. Returns 503 with
  error details when infrastructure is down. Docker/Cosmos can now detect actual
  unhealthy state.
- **docker-compose.yml hardened:** Removed all `${VAR:-default}` shell
  interpolation. All env values hardcoded directly. Password in `DATABASE_URL`
  and `MYSQL_PASSWORD` confirmed to match (`chronicle`). Set `ENV=production`.
- **`.golangci.yml` created:** Linter config with standard linters enabled,
  generated `_templ.go` files excluded.
- **Test readiness audit passed:** 30 unit tests passing, CI pipeline verified,
  Docker multi-stage build working, all packages compile clean.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Created This Session
- `.golangci.yml` -- Linter configuration

### Files Modified This Session
- `internal/database/mariadb.go` -- Retry-with-backoff for DB connection
- `internal/database/redis.go` -- Retry-with-backoff for Redis connection
- `internal/app/routes.go` -- Real health check (DB + Redis ping)
- `docker-compose.yml` -- Hardcoded env values, credential sync
- `.ai/status.md` -- Updated for deployment fixes session

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
- SMTP migration assumes smtp_settings table exists before first admin access
  (migration must be applied)

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
