# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Phase 1 core infrastructure session

## Current Phase
**Phase 1: Foundation** -- Core infrastructure is built. App compiles and
runs (with MariaDB + Redis available). Next: auth plugin, then campaigns,
then entities.

## Last Session Summary

### Completed
- Initialized Go module (`github.com/keyxmakerx/chronicle`)
- Installed all core deps: Echo v4, Templ, go-sql-driver/mysql, go-redis,
  uuid, golang-migrate, argon2id, validator, bluemonday
- Created `internal/config/config.go` -- ENV loading with typed config struct
- Created `internal/apperror/errors.go` -- domain error types (NotFound,
  BadRequest, Unauthorized, Forbidden, Conflict, Internal, Validation)
- Created `internal/database/mariadb.go` -- connection pool with ping check
- Created `internal/database/redis.go` -- Redis client with ping check
- Created `internal/middleware/` -- logging (slog), recovery (panic handler),
  helpers (IsHTMX, Render)
- Created `internal/app/app.go` -- App struct, middleware setup, custom error
  handler (maps AppError to HTML/JSON responses)
- Created `internal/app/routes.go` -- route aggregation with landing page,
  health check, and commented plugin/module slots
- Created `cmd/server/main.go` -- entry point with graceful shutdown,
  structured logging (text in dev, JSON in prod)
- Created Templ layouts: `base.templ` (HTML shell), `app.templ` (sidebar +
  topbar), `landing.templ`, `error.templ`
- Created migration `000001_create_users` (up + down)
- **Build succeeds:** `go build` and `go vet` pass with zero errors

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Created This Session
- `go.mod`, `go.sum`
- `cmd/server/main.go`
- `internal/config/config.go`
- `internal/apperror/errors.go`
- `internal/database/mariadb.go`, `redis.go`
- `internal/middleware/logging.go`, `recovery.go`, `helpers.go`
- `internal/app/app.go`, `routes.go`
- `internal/templates/layouts/base.templ`, `app.templ`
- `internal/templates/pages/landing.templ`, `error.templ`
- `internal/templates/layouts/*_templ.go` (generated)
- `internal/templates/pages/*_templ.go` (generated)
- `db/migrations/000001_create_users.up.sql`, `.down.sql`

## Active Branch
`claude/setup-ai-project-docs-LhvVz`

## Next Session Should
1. **Auth plugin** -- implement the full auth flow:
   - User model in `internal/plugins/auth/model.go`
   - Repository with Create, FindByEmail, FindByID queries
   - Service with Register, Login, ValidateSession logic
   - Handler with login/register/logout routes + Templ pages
   - Auth middleware for session validation
   - PASETO v4 session token generation
2. **Campaign plugin** -- after auth works:
   - Campaign model, migration 000002
   - CRUD handler/service/repository
   - Campaign list and show Templ pages
3. **Entities plugin** -- after campaigns work:
   - Entity + EntityType models, migration 000003
   - Seed default entity types
   - CRUD + entity profile page

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- HTMX and Alpine.js are loaded from CDN -- should be vendored for self-hosted
- Tailwind CSS output (`static/css/app.css`) doesn't exist yet -- needs
  `tailwindcss` binary to generate it
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone

## Recently Completed Milestones
- 2026-02-19: Project scaffolding and three-tier AI documentation system
- 2026-02-19: Core infrastructure (config, database, middleware, app, server)
