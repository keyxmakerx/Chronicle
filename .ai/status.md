# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Dark mode, tags, audit log, site settings, public landing page, template editor

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization, visual template editor for
entity type page layouts, public campaign support, dark mode, tags, and audit logging.

## Last Session Summary

### Completed
- **Dark mode toggle:** Full dark mode system with FOUC prevention (inline script
  in `<head>`), localStorage persistence, system preference fallback. Theme toggle
  button in topbar with sun/moon icons. All layout components updated with `dark:`
  variants. `static/js/theme.js` manages toggle/persistence/icon updates.
- **Collapsible admin sidebar:** Admin section in left sidebar uses Alpine.js
  `x-collapse` with localStorage-persisted open/closed state. Chevron rotates
  to indicate state. Non-admin users never see the section (checked via `GetIsAdmin`).
- **Tags widget plugin:** New `internal/widgets/tags/` widget with full
  model/repository/service/handler/routes stack:
  - Campaign-scoped tags with name, slug, hex color
  - Entity-tag many-to-many join (entity_tags table)
  - Diff-based `SetEntityTags` to minimize DB operations
  - Batch loading (`GetEntityTagsBatch`) to avoid N+1 on list views
  - Migration 000009 creates tags + entity_tags tables
- **Audit log plugin:** New `internal/plugins/audit/` plugin:
  - Records user actions (entity CRUD, membership, campaign settings, tags)
  - Activity page for campaign owners with stats cards (entities, words, editors, last edit)
  - Timeline view with color-coded action dots and relative timestamps
  - Per-entity history endpoint for detailed change logs
  - Migration 000010 creates audit_log table
- **Site settings plugin:** New `internal/plugins/settings/` plugin:
  - Global storage limits (max upload, per-user storage, per-campaign storage, file count, rate limit)
  - Per-user storage overrides (NULL = inherit global)
  - Per-campaign storage overrides (NULL = inherit user/global)
  - Override resolution chain: per-campaign > per-user > global
  - Admin settings page with global form, user/campaign override tables, info panel
  - Migration 000011 creates site_settings, user_storage_limits, campaign_storage_limits
- **Public landing page:** Landing page now shows discoverable public campaigns
  in a responsive 3-column card grid with name, description, last updated time.
  Added `ListPublic` method to campaigns repo/service.
- **Enhanced template editor:** Added new block types beyond rows:
  two-column, three-column, tabs, and section/accordion blocks with
  configurable column widths and drag-and-drop support.
- **Plugin wiring:** All new plugins wired in `app/routes.go`:
  - Tags widget registered with campaign-scoped routes
  - Audit plugin registered with campaign-scoped activity/history routes
  - Settings plugin registered on admin group for storage settings
  - Storage Settings link added to admin sidebar
- **AI documentation:** Created `.ai.md` files for audit, settings, and tags plugins.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `internal/templates/layouts/base.templ` -- Theme init script, theme.js inclusion
- `internal/templates/layouts/app.templ` -- Dark mode toggle, collapsible admin, storage settings link, activity log link
- `static/js/theme.js` -- New: theme toggle module with localStorage persistence
- `static/css/input.css` -- Dark mode CSS custom properties
- `tailwind.config.js` -- darkMode: 'class'
- `internal/widgets/tags/` -- New: model, repo, service, handler, routes (6 files)
- `internal/plugins/audit/` -- New: model, repo, service, handler, routes, activity.templ (6 files)
- `internal/plugins/settings/` -- New: model, repo, service, handler, routes, storage_settings.templ (6 files)
- `internal/plugins/admin/routes.go` -- Returns admin group for settings plugin
- `internal/plugins/campaigns/repository.go` -- Added ListPublic method
- `internal/plugins/campaigns/service.go` -- Added ListPublic method
- `internal/templates/pages/landing.templ` -- Rewrote with public campaigns grid
- `internal/app/routes.go` -- Wired tags, audit, settings plugins; updated landing handler
- `static/js/widgets/template_editor.js` -- Added two_column, three_column, tabs, section blocks
- `db/migrations/000009_create_tags.up/down.sql` -- Tags and entity_tags tables
- `db/migrations/000010_create_audit_log.up/down.sql` -- Audit log table
- `db/migrations/000011_create_site_settings.up/down.sql` -- Site settings and override tables

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **Wire audit logging calls** -- Other plugins should call AuditService.Log() on mutations
2. **Wire storage limits** -- Media plugin should use GetEffectiveLimits at upload time
3. **Frontend tag picker widget** -- JS widget for entity profile pages
4. **@mentions** -- Search entities in editor, insert link, parse/render server-side
5. **Password reset** -- Wire auth password reset with SMTP when configured
6. **Entity relations** -- Bi-directional entity linking
7. **Entity type CRUD** -- Let campaign owners add/edit/remove entity types
8. **Regenerate Tailwind CSS** -- Run `make tailwind` to include new dark: and component classes

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
- 2026-02-19: Unified entity type config, color picker, public campaigns
- 2026-02-19: Visual template editor, layout-driven entity pages, admin nav with modules
- 2026-02-20: Fix campaigns 500 error, move admin nav to sidebar
- 2026-02-20: Fix template editor save, drop indicators, admin storage management page
- 2026-02-20: Dark mode toggle, collapsible admin sidebar, theme.js
- 2026-02-20: Tags widget plugin (campaign-scoped entity tagging)
- 2026-02-20: Audit log plugin (campaign activity timeline, stats)
- 2026-02-20: Site settings plugin (editable storage limits, per-user/campaign overrides)
- 2026-02-20: Public landing page with discoverable campaign cards
- 2026-02-20: Enhanced template editor (two-column, three-column, tabs, sections)
