# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Phase 2: Sidebar customization + Layout builder scaffold

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization (drag-to-reorder, hide/show
entity types), and layout builder scaffold (two-column entity profile layout
editor). All tests pass.

## Last Session Summary

### Completed
- **Sidebar customization (migration 000006):** Added `sidebar_config` JSON
  column to campaigns table. Campaign owners can reorder and hide/show entity
  types in the sidebar via a drag-to-reorder widget on the settings page.
  SidebarConfig model with EntityTypeOrder and HiddenTypeIDs. LayoutInjector
  applies the config to sort/filter sidebar entity types. Full API
  (GET/PUT /campaigns/:id/sidebar-config).
- **Layout builder scaffold (migration 000007):** Added `layout_json` JSON
  column to entity_types table. EntityTypeLayout model with Sections
  (key/label/type/column). Full API (GET/PUT /campaigns/:id/entity-types/:etid/layout).
  Layout builder JS widget renders two-column drag-and-drop editor for
  customizing entity profile page sections. Accordion UI on settings page
  shows one layout builder per entity type.
- **sidebar_config.js widget:** Drag-to-reorder entity type list with
  visibility toggles (eye icon). Auto-saves on every change.
- **layout_builder.js widget:** Two-column (left/right) section editor
  with drag-and-drop between columns. Generates default sections from
  field definitions. Auto-saves on every change.
- **Campaign settings page enhanced:** Now has three sections: Sidebar
  customization, Entity Layouts (accordion of layout builders), and
  the existing Transfer Ownership + Danger Zone.
- **EntityTypeLister adapter:** Clean interface adapter in app/routes.go
  to pass entity type data to campaign settings page without circular imports.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `db/migrations/000006_sidebar_config.up.sql` -- New: sidebar_config column
- `db/migrations/000006_sidebar_config.down.sql` -- New: rollback
- `db/migrations/000007_entity_type_layout.up.sql` -- New: layout_json column
- `db/migrations/000007_entity_type_layout.down.sql` -- New: rollback
- `internal/plugins/campaigns/model.go` -- SidebarConfig, ParseSidebarConfig, BackdropPath, new fields
- `internal/plugins/campaigns/repository.go` -- All queries updated for new columns + UpdateSidebarConfig
- `internal/plugins/campaigns/service.go` -- UpdateSidebarConfig, GetSidebarConfig methods
- `internal/plugins/campaigns/handler.go` -- EntityTypeLister interface, sidebar config API handlers, Settings handler updated
- `internal/plugins/campaigns/routes.go` -- Sidebar config API routes
- `internal/plugins/campaigns/settings.templ` -- Sidebar config widget + layout builder accordion
- `internal/plugins/entities/model.go` -- EntityTypeLayout, LayoutSection types, Layout field on EntityType
- `internal/plugins/entities/repository.go` -- layout_json in all queries + UpdateLayout
- `internal/plugins/entities/service.go` -- UpdateEntityTypeLayout method
- `internal/plugins/entities/handler.go` -- GetEntityTypeLayout, UpdateEntityTypeLayout handlers
- `internal/plugins/entities/routes.go` -- Layout API routes
- `internal/plugins/entities/service_test.go` -- UpdateLayout in mock
- `internal/templates/layouts/data.go` -- SortSidebarTypes function, SortOrder field
- `internal/templates/layouts/app.templ` -- (Unchanged, sidebar rendering already dynamic)
- `internal/templates/layouts/base.templ` -- sidebar_config.js + layout_builder.js script tags
- `internal/app/routes.go` -- entityTypeListerAdapter, SetEntityLister wiring, SortSidebarTypes call
- `static/js/widgets/sidebar_config.js` -- New: drag-to-reorder sidebar config widget
- `static/js/widgets/layout_builder.js` -- New: two-column layout builder widget

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
