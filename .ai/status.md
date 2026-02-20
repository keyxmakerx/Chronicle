# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Entity type CRUD (create, edit, delete entity types)

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization, visual template editor for
entity type page layouts, public campaign support, dark mode, tags, audit logging,
entity relations, and entity type management.

## Last Session Summary

### Completed
- **Entity type CRUD:** Full create/edit/delete workflow for entity types.
  Campaign owners can now add custom entity types, edit existing ones (name,
  plural name, icon, color, custom fields), and delete unused types.
  - New repository methods: `Update`, `Delete`, `SlugExists`, `MaxSortOrder`
    on `EntityTypeRepository`.
  - New service methods: `CreateEntityType`, `UpdateEntityType`, `DeleteEntityType`
    with full validation (name required, slug uniqueness, hex color, entity count
    check before delete).
  - New handler endpoints: `GET /entity-types` (management page), `POST /entity-types`
    (create), `PUT /entity-types/:etid` (update via JSON API), `DELETE /entity-types/:etid`
    (delete with conflict check).
  - New templ template: `entity_types.templ` with management page, icon picker
    (curated Font Awesome icons), color picker, inline edit via Alpine.js toggle,
    and delete confirmation dialog.
  - New JS widget: `entity_type_editor.js` for inline entity type editing with
    field management (add/remove/reorder custom fields, label/type/section editing).
  - Audit logging: Added `entity_type.created`, `entity_type.updated`,
    `entity_type.deleted` action constants.
  - Routes wired under Owner-only permission.
  - "Manage Types" link added to campaign settings page.
  - All existing tests pass, test mocks updated for new interface methods.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `internal/plugins/entities/model.go` -- Added CreateEntityTypeRequest/Input, UpdateEntityTypeRequest/Input DTOs
- `internal/plugins/entities/repository.go` -- Added Update, Delete, SlugExists, MaxSortOrder to EntityTypeRepository
- `internal/plugins/entities/service.go` -- Added CreateEntityType, UpdateEntityType, DeleteEntityType, generateEntityTypeSlug
- `internal/plugins/entities/handler.go` -- Added EntityTypesPage, CreateEntityType, UpdateEntityTypeAPI, DeleteEntityType handlers
- `internal/plugins/entities/routes.go` -- Added GET/POST/PUT/DELETE entity-types routes (Owner only)
- `internal/plugins/entities/entity_types.templ` -- New: entity type management page template
- `internal/plugins/entities/service_test.go` -- Updated mocks with new interface methods
- `internal/plugins/audit/model.go` -- Added ActionEntityTypeCreated/Updated/Deleted constants
- `internal/plugins/campaigns/settings.templ` -- Added "Manage Types" link to entity types section
- `internal/templates/layouts/base.templ` -- Added entity_type_editor.js script tag
- `static/js/widgets/entity_type_editor.js` -- New: inline entity type editor widget

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **Password reset** -- Wire auth password reset with SMTP when configured
2. **Entity nesting** -- Parent/child relationships for entity hierarchy
3. **Regenerate Tailwind CSS** -- Run `make tailwind` to include new dark: and component classes
4. **Map viewer** -- Leaflet.js map widget with entity pins
5. **REST API** -- PASETO token auth for external integrations
6. **Relation tests** -- Table-driven unit tests for relation service

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
- 2026-02-20: @mention system for TipTap editor (search, popup, keyboard nav, styled links)
- 2026-02-20: Entity tooltip/popover widget (hover previews on entity cards, mentions, relations)
- 2026-02-20: Entity relations widget (bi-directional linking, common types, reverse auto-create)
- 2026-02-20: Entity type CRUD (create, edit, delete entity types, icon/color/fields management)
