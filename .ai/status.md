# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Tag cards, @mentions, entity types, relations, tooltips, visual polish

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization, visual template editor for
entity type page layouts, public campaign support, dark mode, tags, audit logging,
entity relations, entity type management, @mentions, hover tooltips, and visual polish.

## Last Session Summary

### Completed
- **Tag display on entity list cards:** Entity cards in grid view now show
  colored tag chips. Uses `EntityTagFetcher` interface + adapter pattern to
  batch-fetch tags via `GetEntityTagsBatch`. Graceful degradation if fetch fails.

- **@mentions in TipTap editor:** New `editor_mention.js` extension. Type `@`
  in the editor to search entities via the existing search API (with Accept:
  application/json content negotiation). Shows a dropdown popup with keyboard
  navigation (arrows, Enter, Escape). Inserts styled mention links with
  `data-mention-id` attributes. Debounced search, AbortController for clean
  cancellation, dark mode support, XSS protection.

- **Entity type CRUD:** Campaign owners can create, edit, and delete entity
  types. New service methods with validation (name, slug uniqueness, hex color).
  New templ template with icon picker and color picker. New `entity_type_editor.js`
  widget for inline editing with field management. Audit logged.

- **Entity relations widget:** Bi-directional entity linking. Migration 000012
  creates `entity_relations` table. New `internal/widgets/relations/` package
  (model, repo, service, handler, routes). Auto-creates reverse relations.
  Common types: allied with, enemy of, parent/child, member/contains, owns.
  New `relations.js` frontend widget with grouped display and search to add.

- **Entity hover tooltip widget:** `entity_tooltip.js` shows preview popovers
  on hover over elements with `data-entity-preview` attribute. Debounced (300ms),
  client-side cache (LRU, 100 entries), smart positioning, dark mode, accessible.
  New `PreviewAPI` endpoint returns name, type info, image, excerpt (150 chars).
  Entity cards automatically have `data-entity-preview` attribute.

- **Visual polish pass (Kanka-inspired):**
  - Landing page: dark gradient hero with radial accent glow, feature highlight
    cards (Characters, Locations, Self-Hosted), proper footer
  - Auth pages: branded logo mark, dark mode support, refined typography
  - Campaign cards: accent gradient bar, book icon, public badge, lift-on-hover
  - Campaign dashboard: icon cards (blue Members, indigo Entities, purple Types),
    role badge pill, dark mode throughout
  - CSS: rounded-xl cards with transitions, refined buttons with accent shadows,
    ghost button variant, stat-card component, mention link styles, improved
    inputs with accent ring focus

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `internal/plugins/entities/model.go` -- EntityTagInfo, EntityTagFetcher, Entity type CRUD DTOs
- `internal/plugins/entities/handler.go` -- TagFetcher, PreviewAPI, EntityType CRUD, SearchAPI JSON support
- `internal/plugins/entities/entity_card.templ` -- Tag chips, data-entity-preview attribute
- `internal/plugins/entities/show.templ` -- blockRelations component, data-campaign-id on editor
- `internal/plugins/entities/routes.go` -- PreviewAPI route, entity type CRUD routes
- `internal/plugins/entities/service.go` -- CreateEntityType, UpdateEntityType, DeleteEntityType
- `internal/plugins/entities/repository.go` -- Update, Delete, SlugExists, MaxSortOrder
- `internal/plugins/entities/entity_types.templ` -- New: entity type management page
- `internal/plugins/entities/service_test.go` -- Updated mocks
- `internal/plugins/entities/search_results.templ` -- data-entity-preview on search results
- `internal/plugins/audit/model.go` -- ActionEntityTypeCreated/Updated/Deleted
- `internal/plugins/campaigns/campaign_card.templ` -- Accent bar, icon, public badge, dark mode
- `internal/plugins/campaigns/show.templ` -- Icon cards, entity types link, role badge, dark mode
- `internal/plugins/campaigns/settings.templ` -- "Manage Types" link
- `internal/plugins/auth/login.templ` -- Branded logo, dark mode, refined styling
- `internal/plugins/auth/register.templ` -- Branded logo, dark mode, refined styling
- `internal/templates/layouts/base.templ` -- Script tags for new widgets
- `internal/templates/pages/landing.templ` -- Gradient hero, feature cards, footer
- `internal/app/routes.go` -- entityTagFetcherAdapter, relations widget wiring
- `static/css/input.css` -- Refined buttons, cards, inputs, stat-card, mention styles
- `static/js/widgets/editor.js` -- Mention extension integration
- `static/js/widgets/editor_mention.js` -- New: @mention extension
- `static/js/widgets/entity_tooltip.js` -- New: hover tooltip widget
- `static/js/widgets/entity_type_editor.js` -- New: entity type inline editor
- `static/js/widgets/relations.js` -- New: entity relations widget
- `static/js/widgets/template_editor.js` -- Added relations block type
- `internal/widgets/relations/` -- New: full relations widget (model, repo, service, handler, routes)
- `db/migrations/000012_*` -- New: entity_relations table

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **Password reset** -- Wire auth password reset with SMTP when configured
2. **Regenerate Tailwind CSS** -- Run `make tailwind` to include new classes
3. **Entity nesting** -- Parent/child relationships for entity hierarchy
4. **Map viewer** -- Leaflet.js map widget with entity pins
5. **REST API** -- PASETO token auth for external integrations
6. **Relation/tooltip tests** -- Table-driven unit tests for new services

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
