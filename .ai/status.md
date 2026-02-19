# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Security audit: comprehensive vulnerability fixes

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization (drag-to-reorder, hide/show
entity types), and layout builder scaffold (two-column entity profile layout
editor). Comprehensive security audit completed with 14 fixes across 14 files.
All tests pass.

## Last Session Summary

### Completed
- **Full codebase security audit:** Reviewed all 6 plugins, middleware, core
  infrastructure, templates, and JS widgets for vulnerabilities and code quality.
- **14 security fixes applied across 14 files:**
  - CSRF timing attack fix (constant-time comparison)
  - CSRF form field name mismatch (`_csrf` -> `csrf_token` in 2 templates)
  - Deprecated X-XSS-Protection header disabled (set to "0")
  - Health endpoint info leak fixed (generic error messages, server-side logging)
  - SECRET_KEY validation (case-insensitive env check, 32-char minimum in prod)
  - generateUUID error handling (panic on rand failure in 3 locations)
  - Slug generation bounded loop (100 attempts + random fallback in 2 locations)
  - FULLTEXT boolean mode operator stripping in entity search
  - LIKE wildcard injection escaping in entity search
  - Directory traversal prevention for entity image paths
  - Last admin removal protection
  - SMTP header injection prevention (from_address, from_name, subject)
  - SMTP encryption mode and port range validation
  - Media upload body size limit middleware
- **Documentation fixes:** Updated Go version in tech-stack.md, added missing
  API routes to api-routes.md (image upload, entity type layout endpoints).
- **Bug fixes:**
  - `config.IsDevelopment()` now case-insensitive, also matches "dev"
  - Media upload: clean up thumbnails on disk when DB insert fails

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Known Security Items (Deferred -- Larger Changes)
- **Stored XSS via entry_html:** entry_html is not rendered as raw HTML in
  templates (TipTap uses ProseMirror JSON), but adding bluemonday sanitization
  at storage time would provide defense-in-depth. Requires new dependency.
- **CSP unsafe-eval/unsafe-inline:** Required by Alpine.js. Migrating to
  Alpine.js CSP build would allow tightening the Content-Security-Policy.
- **In-memory rate limiter:** Current rate limiter uses sync.Map (not Redis),
  so limits don't persist across restarts or multiple instances. Fine for
  single-instance deployments but should move to Redis for multi-instance.

### Files Modified This Session
- `internal/middleware/csrf.go` -- Constant-time CSRF comparison
- `internal/middleware/security.go` -- X-XSS-Protection set to "0"
- `internal/app/routes.go` -- Health endpoint info leak fix, media route maxSize param
- `internal/config/config.go` -- Case-insensitive env check, 32-char SECRET_KEY minimum
- `internal/plugins/entities/service.go` -- generateUUID panic, slug bounds, path traversal
- `internal/plugins/entities/repository.go` -- FULLTEXT operator stripping, LIKE escaping
- `internal/plugins/campaigns/service.go` -- generateUUID panic, slug bounds
- `internal/plugins/admin/handler.go` -- Last admin removal protection
- `internal/plugins/auth/repository.go` -- CountAdmins interface + implementation
- `internal/plugins/smtp/service.go` -- Header injection, validation hardening
- `internal/plugins/smtp/settings.templ` -- CSRF field name fix
- `internal/plugins/admin/campaigns.templ` -- CSRF field name fix
- `internal/plugins/media/service.go` -- generateUUID panic fix
- `internal/plugins/media/routes.go` -- Body size limit middleware
- `.ai/tech-stack.md` -- Go version updated to 1.24+
- `.ai/api-routes.md` -- Added missing image + layout API routes

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **@mentions** -- Search entities in editor, insert link, parse/render server-side
2. **Password reset** -- Wire auth password reset with SMTP when configured
3. **Entity relations** -- Bi-directional entity linking
4. **Apply layout_json to entity show page** -- Render entity profiles using the layout config
5. **Entity type CRUD** -- Let campaign owners add/edit/remove entity types

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`

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
- 2026-02-19: Auto-migrations on startup, first-user-is-admin, /health alias
- 2026-02-19: Media plugin (upload, thumbnails, magic byte validation, rate limiting)
- 2026-02-19: Security hardening (IDOR fixes, HSTS, rate limiting on auth)
- 2026-02-19: Dynamic sidebar with entity types from DB + count badges
- 2026-02-19: Entity image upload pipeline + UI quality upgrade
- 2026-02-19: Sidebar customization (drag-to-reorder, hide/show entity types)
- 2026-02-19: Layout builder scaffold (two-column entity profile layout editor)
- 2026-02-19: Comprehensive security audit (14 vulnerability fixes across 14 files)
