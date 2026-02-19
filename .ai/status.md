# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Auth plugin implementation session

## Current Phase
**Phase 1: Foundation** -- Core infrastructure + auth plugin are built. App
compiles successfully. Next: campaigns plugin, then entities plugin.

## Last Session Summary

### Completed
- Created `internal/middleware/security.go` -- security headers (CSP, X-Frame-Options,
  X-Content-Type-Options, Referrer-Policy, Permissions-Policy, X-XSS-Protection)
- Created `internal/middleware/proxy.go` -- trusted reverse proxy IP extraction for
  Cosmos Cloud. Custom IPExtractor trusts X-Forwarded-For/X-Real-IP from Docker CIDRs.
- Created `internal/middleware/cors.go` -- CORS middleware with HTMX header support.
  Exposes HX-Redirect/HX-Refresh/HX-Trigger. Caches preflight for 1 hour.
- Created `internal/middleware/csrf.go` -- double-submit cookie CSRF with HTMX
  integration. Cookie NOT HttpOnly so JS can read it for X-CSRF-Token header.
- Updated `internal/app/app.go` -- wired all new middleware + trusted proxy config
- **Auth plugin fully implemented:**
  - `model.go` -- User domain model, RegisterRequest/LoginRequest DTOs,
    RegisterInput/LoginInput service DTOs, Session struct for Redis
  - `repository.go` -- UserRepository interface + MariaDB implementation
    (Create, FindByID, FindByEmail, EmailExists, UpdateLastLogin)
  - `service.go` -- AuthService with Register (argon2id hash, UUID gen),
    Login (password verify, Redis session create), ValidateSession, DestroySession
  - `handler.go` -- thin HTTP handlers for login/register/logout with HTMX support
  - `login.templ` -- login page + form (HTMX partial replacement on error)
  - `register.templ` -- register page + form (HTMX partial replacement on error)
  - `middleware.go` -- RequireAuth middleware, GetSession/GetUserID helpers
  - `routes.go` -- public route registration
- Updated `internal/app/routes.go` -- wired auth plugin with DI, added dashboard
  placeholder route behind RequireAuth middleware
- **Build succeeds:** `go build ./...` passes with zero errors

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Created This Session
- `internal/middleware/security.go`, `proxy.go`, `cors.go`, `csrf.go`
- `internal/plugins/auth/model.go`, `repository.go`, `service.go`
- `internal/plugins/auth/handler.go`, `middleware.go`, `routes.go`
- `internal/plugins/auth/login.templ`, `register.templ`
- `internal/plugins/auth/login_templ.go`, `register_templ.go` (generated)

## Active Branch
`claude/setup-ai-project-docs-LhvVz`

## Next Session Should
1. **Campaign plugin** -- implement CRUD:
   - Campaign model, migration 000002_create_campaigns
   - Repository with Create, FindByID, ListByUser, Update, Delete
   - Service with Create, GetByID, List, Update, Delete + slug gen
   - Handler with campaign list/show/create/edit/delete + Templ pages
   - Campaign Templ pages (list, show, create, edit)
2. **Entities plugin** -- after campaigns work:
   - Entity + EntityType models, migration 000003
   - Seed default entity types (Character, Location, etc.)
   - CRUD + entity profile page
3. **Editor widget** -- TipTap integration after entities

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- HTMX and Alpine.js are loaded from CDN -- should be vendored for self-hosted
- Tailwind CSS output (`static/css/app.css`) doesn't exist yet -- needs
  `tailwindcss` binary to generate it
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Dashboard route currently renders the landing page as a placeholder

## Recently Completed Milestones
- 2026-02-19: Project scaffolding and three-tier AI documentation system
- 2026-02-19: Core infrastructure (config, database, middleware, app, server)
- 2026-02-19: Security middleware (proxy trust, CORS, CSRF, security headers)
- 2026-02-19: Auth plugin (register, login, logout, session management)
