# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Unified entity type config, color picker, public campaigns

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization (drag-to-reorder, hide/show
entity types), layout builder scaffold, and public campaign support.

## Last Session Summary

### Completed
- **Unified entity type config widget:** Combined sidebar config + layout builder
  into a single "Entity Types" section on the campaign settings page. One widget
  handles drag-to-reorder, visibility toggles, color picker, and layout editing.
  Replaces the two separate sidebar_config.js and layout_builder.js widgets.
- **Entity type color picker:** Added native HTML5 color picker to the unified
  widget. Color changes are persisted via PUT `/campaigns/:id/entity-types/:etid/color`
  with hex validation in the service layer.
- **Public campaign support (is_public flag):**
  - Migration 000008: adds `is_public` boolean column to campaigns table.
  - OptionalAuth middleware: loads session if cookie exists but doesn't reject guests.
  - AllowPublicCampaignAccess middleware: lets unauthenticated visitors see public
    campaigns with RolePlayer (read-only). Non-public campaigns redirect to /login.
  - Campaign and entity view routes use public-capable middleware.
  - Topbar shows "Log in / Sign up" for guests instead of user avatar/logout.
  - Sidebar hides "Manage" section and "All Campaigns" for guests.
  - Campaign picker dropdown only renders for authenticated users.
  - Edit form has "Make this campaign public" checkbox.
- **Bug fixes from prior session:**
  - `config.IsDevelopment()` now case-insensitive, also matches "dev"
  - Media upload: clean up thumbnails on disk when DB insert fails

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `static/js/widgets/entity_type_config.js` -- New unified widget
- `internal/plugins/entities/repository.go` -- UpdateColor method
- `internal/plugins/entities/service.go` -- UpdateEntityTypeColor + hex validation
- `internal/plugins/entities/handler.go` -- UpdateEntityTypeColor endpoint
- `internal/plugins/entities/routes.go` -- Color route + public view routes
- `internal/plugins/entities/service_test.go` -- Mock UpdateColor
- `internal/plugins/campaigns/settings.templ` -- Unified entity type config section
- `internal/plugins/campaigns/model.go` -- IsPublic field + request/input DTOs
- `internal/plugins/campaigns/repository.go` -- All queries updated for is_public
- `internal/plugins/campaigns/service.go` -- IsPublic wired into Update method
- `internal/plugins/campaigns/middleware.go` -- AllowPublicCampaignAccess
- `internal/plugins/campaigns/handler.go` -- IsPublic in Update handler
- `internal/plugins/campaigns/routes.go` -- Public view routes
- `internal/plugins/campaigns/form.templ` -- IsPublic checkbox on edit form
- `internal/plugins/auth/middleware.go` -- OptionalAuth middleware
- `internal/templates/layouts/app.templ` -- Guest-friendly topbar + sidebar
- `internal/templates/layouts/data.go` -- IsAuthenticated context helper
- `internal/templates/layouts/base.templ` -- entity_type_config.js script
- `internal/app/routes.go` -- SetIsAuthenticated in LayoutInjector
- `db/migrations/000008_campaign_public.up.sql` -- New migration
- `db/migrations/000008_campaign_public.down.sql` -- Rollback migration

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **Admin notification system** -- Notification bell in topbar, admin alerts
2. **Apply layout_json to entity show page** -- Render entity profiles using the layout config
3. **@mentions** -- Search entities in editor, insert link, parse/render server-side
4. **Password reset** -- Wire auth password reset with SMTP when configured
5. **Entity relations** -- Bi-directional entity linking
6. **Entity type CRUD** -- Let campaign owners add/edit/remove entity types

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
