# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Semantic color system, dark mode fixes, editor toggle, attributes widget, notifications, modules page

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization, visual template editor for
entity type page layouts, public campaign support, dark mode, tags, audit logging,
entity relations, entity type management, @mentions, hover tooltips, visual polish,
semantic color system, toast notifications, and admin modules page.

## Last Session Summary

### Completed
- **Semantic color system:** Created CSS custom properties for all colors
  (`--color-text-primary`, `--color-bg-secondary`, etc.) with light/dark values
  on `:root` / `.dark`. Wired into Tailwind as utility classes (`text-fg`,
  `bg-surface`, `border-edge`, etc.) so components auto-switch without `dark:`.

- **Dark mode fix across ALL templ files:** Three background agents fixed 15+
  templ files (campaigns, entities, error pages, pagination, SMTP settings) to
  use semantic tokens or proper `dark:` variants. Eliminated black-on-black text.

- **CSS component migration to semantic tokens:** Converted `.btn-secondary`,
  `.btn-ghost`, `.card`, `.input`, `.link-muted`, `.badge-gray`, `.empty-state__*`,
  `.table-header`, `.table-row`, `.chronicle-editor`, `.section-header`,
  `.stat-card__*` to use semantic color tokens. No more per-component `dark:`.

- **Editor view/edit mode toggle:** TipTap editor now starts in read-only view
  mode. Header bar with Edit/Done button. `enterEditMode()` shows toolbar,
  enables editing, starts autosave. `exitEditMode()` saves and returns to view.

- **Entity list page redesign:** Changed from vertical sidebar filter to
  horizontal tab navigation with search bar, stats subtitle, and plus icon
  on create button. Better responsive layout.

- **Attributes widget (full-stack):** New `attributes.js` widget with inline
  edit UI for all field types (text, number, url, textarea, select, checkbox).
  Added `GetFieldsAPI` / `UpdateFieldsAPI` handlers, `UpdateFields` repo/service
  methods, and API routes.

- **Descriptor rename:** Renamed "Subtype Label" to "Descriptor" with clearer
  help text explaining the purpose (e.g., "City" for a Location).

- **Template editor block preview overlay:** Right-click context menu and eye
  icon button on blocks. Shows overlay with rendered mockup for all block types
  (title, image, entry, attributes, details, tags, relations, divider).

- **Toast notification system:** Created `notifications.js` with
  `Chronicle.notify(message, type, opts)` API. Success/error/info/warning types
  with slide-in animation. HTMX integration for auto error toasts on
  `htmx:responseError` and `htmx:sendError`. Wired into `base.templ`.

- **Admin modules management page:** Module registry (`internal/modules/registry.go`)
  with D&D 5e, Pathfinder 2e, Draw Steel entries. Modules page with card grid UI,
  status badges, content categories. Dashboard stat card and sidebar link added.

- **Admin dashboard semantic tokens:** Migrated dashboard from hardcoded gray
  classes to `text-fg`, `text-fg-secondary`, `text-fg-muted` semantic tokens.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `static/css/input.css` -- Semantic color variables, component token migration, editor/tab styles
- `tailwind.config.js` -- Semantic token color definitions (fg, surface, page, edge)
- `static/js/widgets/editor.js` -- View/edit mode toggle, header bar
- `static/js/widgets/attributes.js` -- New widget (inline edit for entity custom fields)
- `static/js/widgets/template_editor.js` -- Block preview overlay, context menu
- `static/js/notifications.js` -- New toast notification system
- `internal/modules/registry.go` -- New module registry
- `internal/plugins/admin/modules.templ` -- New admin modules page template
- `internal/plugins/admin/dashboard.templ` -- Modules card + semantic tokens
- `internal/plugins/admin/handler.go` -- Modules handler method
- `internal/plugins/admin/routes.go` -- Modules route
- `internal/plugins/entities/index.templ` -- Horizontal tabs, search, stats redesign
- `internal/plugins/entities/form.templ` -- Descriptor rename
- `internal/plugins/entities/show.templ` -- Attributes widget mount point
- `internal/plugins/entities/handler.go` -- GetFieldsAPI, UpdateFieldsAPI
- `internal/plugins/entities/repository.go` -- UpdateFields
- `internal/plugins/entities/service.go` -- UpdateFields
- `internal/plugins/entities/service_test.go` -- Mock UpdateFields
- `internal/plugins/entities/routes.go` -- Fields API routes
- `internal/templates/layouts/base.templ` -- notifications.js + attributes.js script tags
- `internal/templates/layouts/app.templ` -- Modules sidebar link
- 15+ templ files fixed for dark mode by background agents

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **Password reset** -- Wire auth password reset with SMTP when configured
2. **Entity nesting** -- Parent/child relationships for entity hierarchy
3. **Map viewer** -- Leaflet.js map widget with entity pins
4. **REST API** -- PASETO token auth for external integrations
5. **Relation/tooltip tests** -- Table-driven unit tests for new services
6. **Module implementation** -- Start with D&D 5e SRD data loading and reference pages

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`
- Tailwind standalone CLI (`tailwindcss`) is v3; do NOT use `npx @tailwindcss/cli` (v4 syntax)

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
- 2026-02-20: Semantic color system with CSS custom properties + Tailwind tokens
- 2026-02-20: Dark mode fix across all templ files (15+ files, 3 parallel agents)
- 2026-02-20: CSS component migration to semantic tokens (no more per-component dark:)
- 2026-02-20: Editor view/edit mode toggle (read-only default, Edit/Done button)
- 2026-02-20: Entity list page redesign (horizontal tabs, search, stats)
- 2026-02-20: Attributes widget (full-stack: JS widget + Go API + repo/service)
- 2026-02-20: Descriptor rename (Subtype → Descriptor with help text)
- 2026-02-20: Template editor block preview overlay (right-click + eye icon)
- 2026-02-20: Toast notification system (Chronicle.notify API + HTMX integration)
- 2026-02-20: Admin modules management page (registry, cards, dashboard, sidebar)
