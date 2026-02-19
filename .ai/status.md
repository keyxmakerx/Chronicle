# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Visual template editor, layout-driven entity pages, admin nav

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization, visual template editor for
entity type page layouts, and public campaign support.

## Last Session Summary

### Completed
- **Visual template editor:** Full drag-and-drop page template editor for
  entity types. Campaign owners can design entity profile page layouts at
  `/campaigns/:id/entity-types/:etid/template`.
  - New row/column/block grid system (12-column CSS grid).
  - Six block types: title, image, entry, attributes, details, divider.
  - Row layout presets: full-width, 50/50, 67/33, 33/67, 3-column, 4-column.
  - Drag from palette to add blocks, drag within canvas to reorder.
  - Auto-save via PUT endpoint, Ctrl+S keyboard shortcut.
  - Backward-compatible ParseLayoutJSON() handles old section format.
- **Layout-driven entity show page:** Entity profile pages now render
  dynamically from the entity type's layout_json instead of a hardcoded
  two-column layout. Block components: blockTitle, blockImage, blockEntry,
  blockAttributes, blockDetails, blockDivider. Falls back to default layout
  if no layout is configured.
- **Admin panel navigation with modules section:** All admin pages now share
  a consistent sidebar navigation (`AdminLayout` component) with:
  - Administration links: Dashboard, Users, Campaigns, SMTP Settings
  - Modules section: placeholder for game system content packs (D&D 5e, etc.)
  - "Back to Campaigns" link
- **Public campaign support** (from previous session, finalized):
  - Edit form checkbox, IsPublic wired into service Update
  - Guest-friendly topbar, sidebar, and campaign picker
- **Cleanup:** Removed stale layout_builder.js, simplified entity_type_config.js
  to sidebar ordering + visibility + color only (no inline layout editing).
- **Tailwind safelist:** Added col-span-1 through col-span-12 to tailwind
  config safelist for dynamic grid column rendering.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `internal/plugins/entities/model.go` -- New TemplateRow/Column/Block types, DefaultLayout(), ParseLayoutJSON()
- `internal/plugins/entities/repository.go` -- ParseLayoutJSON in all scan locations
- `internal/plugins/entities/service.go` -- Row/column/block validation for layout updates
- `internal/plugins/entities/handler.go` -- TemplateEditor handler
- `internal/plugins/entities/routes.go` -- Template editor route
- `internal/plugins/entities/show.templ` -- Dynamic layout rendering with block components
- `internal/plugins/entities/template_editor.templ` -- New template editor page
- `static/js/widgets/template_editor.js` -- New drag-and-drop editor widget
- `static/js/widgets/entity_type_config.js` -- Simplified (removed inline layout editing)
- `internal/plugins/admin/nav.templ` -- New shared admin nav component
- `internal/plugins/admin/dashboard.templ` -- Uses AdminLayout
- `internal/plugins/admin/users.templ` -- Uses AdminLayout
- `internal/plugins/admin/campaigns.templ` -- Uses AdminLayout
- `internal/templates/layouts/base.templ` -- Added template_editor.js script tag
- `tailwind.config.js` -- Added col-span safelist for dynamic grid
- `internal/plugins/campaigns/service.go` -- IsPublic in Update
- `internal/plugins/campaigns/form.templ` -- IsPublic checkbox
- `internal/templates/layouts/data.go` -- IsAuthenticated context helper
- `internal/templates/layouts/app.templ` -- Guest-friendly topbar/sidebar
- `internal/app/routes.go` -- SetIsAuthenticated in LayoutInjector
- Deleted: `static/js/widgets/layout_builder.js` (replaced by template_editor.js)

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **@mentions** -- Search entities in editor, insert link, parse/render server-side
2. **Password reset** -- Wire auth password reset with SMTP when configured
3. **Entity relations** -- Bi-directional entity linking
4. **Entity type CRUD** -- Let campaign owners add/edit/remove entity types
5. **Game system modules** -- Implement module registry, D&D 5e module, admin module settings
6. **Regenerate Tailwind CSS** -- Run `make tailwind` to include new safelist classes

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
