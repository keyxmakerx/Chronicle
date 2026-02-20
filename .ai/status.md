# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Dark mode fixes, combined storage page, layout editor fixes

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization, visual template editor for
entity type page layouts, public campaign support, dark mode, tags, audit logging,
entity relations, entity type management, @mentions, hover tooltips, and visual polish.

## Last Session Summary

### Completed
- **Dark mode fixes across all admin pages:** Added `dark:` variants to
  dashboard, users, campaigns, storage, and template editor pages. Fixed
  black text on dark backgrounds throughout admin area.

- **Combined Storage + Storage Settings page:** Merged the two separate
  admin pages into a unified `/admin/storage` page with Alpine.js tabs
  (Files | Limits & Overrides). Compact layout with less whitespace,
  inline stats header, horizontal usage breakdown chips. Removed sidebar
  "Storage Settings" link. Old `/admin/storage/settings` route redirects.

- **User/campaign dropdowns for storage overrides:** Replaced raw UUID
  text inputs with `<select>` dropdowns populated from the database.
  Admin users are greyed out (disabled) in the user dropdown. Uses
  Alpine.js + fetch() for dynamic form submission.

- **Template editor dark mode:** All dynamically created elements in
  `template_editor.js` now have dark mode classes (palette, canvas,
  blocks, containers, tabs, sections, sub-blocks, drop zones).

- **Template editor palette drag fix:** Changed palette item
  `effectAllowed` from `'copy'` to `'copyMove'` to resolve browser
  drop rejection when drop zones use `dropEffect = 'move'`. Blocks
  can now be added from the palette again.

- **Alpine.js x-cloak CSS:** Added `[x-cloak] { display: none !important; }`
  to prevent flash-of-unstyled-content on Alpine-managed tab panels.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `internal/plugins/admin/handler.go` -- Added settingsService, StoragePageData struct, combined Storage handler
- `internal/plugins/admin/storage.templ` -- Complete rewrite: combined page with tabs, dark mode, dropdowns
- `internal/plugins/admin/dashboard.templ` -- Dark mode classes throughout
- `internal/plugins/admin/users.templ` -- Dark mode classes throughout
- `internal/plugins/admin/campaigns.templ` -- Dark mode classes throughout
- `internal/plugins/settings/handler.go` -- Redirects to /admin/storage, simplified UpdateStorageSettings
- `internal/plugins/entities/template_editor.templ` -- Dark mode on editor header
- `internal/templates/layouts/app.templ` -- Removed Storage Settings sidebar link, updated Storage link matching
- `internal/app/routes.go` -- Wire settingsService into admin handler
- `static/js/widgets/template_editor.js` -- Dark mode on all elements, palette drag fix (effectAllowed)
- `static/css/input.css` -- Added [x-cloak] CSS rule

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **Password reset** -- Wire auth password reset with SMTP when configured
2. **Entity nesting** -- Parent/child relationships for entity hierarchy
3. **Map viewer** -- Leaflet.js map widget with entity pins
4. **REST API** -- PASETO token auth for external integrations
5. **Relation/tooltip tests** -- Table-driven unit tests for new services
6. **Dark mode audit** -- Remaining pages (entity show, campaign settings, SMTP) may need dark: fixes

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
- 2026-02-20: Wired audit logging into entity, campaign, and tag mutation handlers
- 2026-02-20: Wired storage limit enforcement into media upload handler
- 2026-02-20: Tag picker widget (search, create, assign tags on entity profile pages)
- 2026-02-20: Tag display on entity list cards (batch fetch, colored chips)
- 2026-02-20: @mention system for TipTap editor (search popup, keyboard nav, styled links)
- 2026-02-20: Entity tooltip/popover widget (hover previews with image, type badge, excerpt)
- 2026-02-20: Entity relations widget (bi-directional linking, common types, reverse auto-create)
- 2026-02-20: Entity type CRUD (create, edit, delete, icon/color/fields management)
- 2026-02-20: Visual polish pass (gradient hero, icon cards, refined buttons/cards/inputs)
- 2026-02-20: Dark mode fixes across all admin pages + template editor
- 2026-02-20: Combined Storage + Storage Settings into unified tabbed page
- 2026-02-20: User/campaign dropdowns for storage overrides (admins greyed out)
- 2026-02-20: Fixed template editor palette drag (blocks can be added again)
