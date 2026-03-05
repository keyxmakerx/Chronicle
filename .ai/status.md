# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-03-05 -- Sprint K-4 (Module Data API & Widget Integration) complete (batch 38).
Branch: `claude/plan-development-phases-G8Pwf`.

## Current Phase
**Phase K: Permissions UI + Module Foundation.** Sprints K-1, K-2, and K-3 delivered. Roadmap organized into 3 interleaved tracks:

1. **Module Framework** — ~~K-3 (manifest/loader)~~, K-4 (data API), L-1 (SDK docs), L-5 (packaging), M-4 (Draw Steel).
2. **Foundry VTT** — K-5 (bug fixes), L-3 (multi-scene + combat), M-1 (character sheet linking).
3. **Permissions + Content** — ~~K-2 (permissions UI)~~, K-6 (relations graph), L-2 (entity posts), L-4 (auto-linking), M-2 (notes rich text), M-3 (group visibility), M-5 (calendar DnD).

Next sprint: **K-5 (Foundry Polish Sprint)**.

### Summary of Recent Work (batches 25-38)
- **Batch 38**: Sprint K-4 Module Data API & Widget Integration — JSONProvider
  (loads JSON data files into memory, implements DataProvider interface). D&D 5e
  module: first concrete Module implementation with 10 SRD spells data, tooltip
  renderer. Factory registry pattern (RegisterFactory/init()) avoids circular imports.
  Generic module HTTP handler (Index/CategoryList/ItemDetail/SearchAPI/TooltipAPI).
  Templ reference pages (category grid, item table, item detail with breadcrumbs).
  Module route registration with dynamic addon middleware (per-module :mod param).
  ModuleSearcher interface added to entity handler (follows existing searcher pattern).
  ModuleSearchAdapter searches enabled modules for campaign. Entity SearchAPI includes
  module results in @mention/quick search. 20 new tests (json_provider). ADR-020.
- **Batch 37**: Sprint K-3 Module Manifest & Loader Framework — Manifest-driven
  module system replacing static hardcoded registry. ModuleManifest JSON spec
  (id, name, version, author, license, api_version, categories with field schemas,
  entity presets, tooltip template). ModuleLoader auto-discovers modules by
  scanning `internal/modules/*/manifest.json`. Sandboxed interfaces: Module,
  DataProvider, TooltipRenderer, ReferenceItem struct. manifest.json files for
  dnd5e, pathfinder2e, drawsteel. Admin modules page updated to show author,
  license, API version. Module slugs added to installedAddons. 13 new tests.
  ADR-019 recorded.
- **Batch 36**: Sprint K-2 Per-Entity Permissions UI — Visibility section on
  entity edit page (Alpine.js) with three modes: Everyone/GM Only/Custom
  Permissions. Custom mode: dynamic permission grant builder with subject type
  (role/user), subject ID picker, permission level (view/edit), auto-save via
  API. Handler: GET/PUT `/entities/:eid/permissions`, GET `/entities/members`.
  Visibility indicators updated across entity cards, table rows, tree view,
  show page details block. Shield icon for custom permissions, lock for private.
  MemberLister dependency wired into entity handler.
- **Batch 35**: Sprint K-1 Per-Entity Permissions Model — Migration 000048
  (`entity_permissions` table, `visibility` ENUM column on entities). Permission
  model types (VisibilityMode, SubjectType, Permission, EntityPermission,
  EffectivePermission, SetPermissionsInput, PermissionGrant). Full
  EntityPermissionRepository (ListByEntity, SetPermissions transactional,
  DeleteByEntity, GetEffectivePermission, UpdateVisibility). visibilityFilter()
  SQL helper handles both legacy is_private and custom permission modes in a
  single WHERE clause. Service: CheckEntityAccess, SetEntityPermissions,
  GetEntityPermissions. All entity list/search/count/children/backlinks queries
  updated with userID parameter across handlers, sync API, export adapters,
  layout injector, campaign dashboard. 13 new unit tests. Pure backend — no UI.
- **Batch 34**: Sprint J-4 File Security — ClamAV antivirus integration for upload
  scanning via clamd TCP protocol (INSTREAM). Fail-open when clamd unavailable.
  ClamAV container in docker-compose (clamav/clamav:1.4). CLAMAV_ADDRESS env var.
  3 unit tests. SVG blocked by MIME allowlist. CDR strips metadata/polyglots.
- **Batch 33**: Sprint J-3 Testing & Infrastructure — Verified HTMX edge cases (CSRF
  propagation, double-init prevention, widget cleanup, form tracking all covered by boot.js).
  Created `.air.toml` for hot reload config. Fixed docker-compose.yml em-dash in error
  message. Added `doc.go` for templates/components package. Verified all Go packages have
  package doc comments.
- **Batch 32**: Sprint J-2 Editor Enhancements — Code block syntax highlighting via
  @tiptap/extension-code-block-lowlight with highlight.js common languages (JS, Python,
  Go, SQL, etc.). Tokyo Night-inspired dark/light syntax theme in input.css. Find/replace
  bar (Ctrl+F find, Ctrl+H replace) with match navigation, replace, replace-all.
  TipTap bundle rebuilt with lowlight extension (~558KB).
- **Batch 31**: Sprint J-1 Breadcrumbs + Navigation — Shared breadcrumb component
  (`components/breadcrumbs.templ`). Added breadcrumbs to: maps list, map detail, timeline
  list, timeline detail, sessions list, session detail, calendar grid header, calendar
  timeline view, calendar week view. Recently Viewed Entities tracker
  (`recent_entities.js`) using localStorage, renders in sidebar drill panel.
- **Batch 30**: Sprint I-4 Map UX Polish — Leaflet.markercluster integration for both
  map widget and full map page (auto-clustering when >5 markers). Expanded POI icon
  picker from 18 to 39 icons organized in 8 groups (General, Settlements, Fortifications,
  Dungeons & Ruins, Nature, Maritime, Sacred & Magic, Resources). Custom cluster icon
  styling. CDN-loaded MarkerCluster CSS + JS.
- **Batch 29**: Sprint I-3 Calendar Week View — 7-column day grid with event cards.
  Repo: `ListEventsForDateRange`. Handler: `ShowWeek`. Template: `WeekPage/WeekFragment`.
  View toggle (Grid/Week/Timeline) added to all 3 calendar views. Navigation: prev/next/today.
  Cross-month and cross-year boundary handling. 5 unit tests. Route: GET /calendar/week.
- **Batch 28**: Sprint I-2 Timeline Phase 2B — Event connections (migration 000047,
  model/repo/service/handler, D3 SVG lines/arrows with arrowhead markers, 4 line styles),
  create-from-timeline (double-click empty space opens modal with date pre-filled),
  visual polish (connection line CSS, event marker hover effects, ruler label improvements).
  3 unit tests. Routes: GET/POST/DELETE /timelines/:tid/connections.
- **Batch 27**: Sprint I-1 Campaign Export/Import — Full JSON export/import for campaigns
  including entity types, entities, tags, relations, calendar (config + events),
  timelines (standalone events), sessions, maps (markers, drawings, layers, tokens, fog),
  addons, media manifest. 6 new files (export.go, import.go, export_service.go,
  export_handler.go, export_adapters.go, import_test.go). Adapter pattern for 7 plugin
  services. Routes: GET /campaigns/:id/export, POST /campaigns/import.
- **Batch 26**: Phase H Release Readiness — error standardization (249 calls), code dedup,
  OpenAPI 3.0.3 spec, extension docs.
- **Batch 25**: Alpha Hardening — CI pipeline, 3 service test suites, IDOR helper,
  input validation, TipTap table support.

### Earlier Batches (summary)
- **Batch 20**: Fixed duplicate migration 000041, mobile nav cleanup, 3 dashboard widgets.
- **Batch 19**: Shop entity type, relation metadata, shop inventory widget, README cleanup.
- **Batch 16-18**: Sessions-Calendar integration, RSVP emails, recurring sessions, Foundry
  VTT bidirectional sync (WebSocket hub, sync mappings, journal sync, map API, calendar sync).
- **Batches 1-15**: Core platform (auth, campaigns, entities, editor, media, calendar, maps,
  timelines, sessions, addons, admin), 294+ tests, security audit (14 fixes), code quality
  sprint (138 lint fixes), mobile responsive, dark mode, extension framework.

---

## Next Session Should
Continue **Phase K** with Sprint K-5 (Foundry Polish Sprint — shop icon fix, fog bidirectional sync, connection status UI, SimpleCalendar hooks). Then K-6 (Relations Graph Visualization). Full reorganized roadmap (Phases K-M, 15 sprints across 3 tracks) in `.ai/todo.md`.

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`
- Tailwind standalone CLI (`tailwindcss`) is v3; do NOT use `npx @tailwindcss/cli` (v4 syntax)
- Fog-of-war sync is one-way only (Chronicle → Foundry). Foundry → Chronicle planned in Sprint K-5.
- SimpleCalendar events are limited (managed as journal notes, no CRUD hooks). Improvement planned in Sprint K-5.
- Foundry shop icon field always returns null. Fix planned in Sprint K-5.

## Completed Phases
- **2026-02-19: Phase 0** — Project scaffolding, AI docs, build config
- **2026-02-19: Phase 1** — Auth, campaigns, SMTP, admin, entities, editor, UI layouts,
  unit tests, Dockerfile, CI/CD, production deployment, auto-migrations
- **2026-02-19 to 2026-02-20: Phase 2** — Media, security audit, sidebar, entity images,
  layout builder, dark mode, tags, relations, attributes, editor, semantic colors
- **2026-02-20: Phase 3** — UI overhaul: Page/Category rename, drill-down sidebar
- **2026-02-20: Phase B** — Extension framework, Sync API, REST API v1
- **2026-02-20: Phase C** — Player notes, terminology standardization
- **2026-02-22 to 2026-02-24: Phase D** — Customization Hub, Dashboard Editor, Page Layouts
- **2026-02-24 to 2026-02-25: Phase E** — Quick Search, Entity Hierarchy, Editor Insert Menu
- **2026-02-25 to 2026-02-28: Phase F** — Calendar & Time (monthly grid, events, settings,
  import/export, timeline view, eras, seasons)
- **2026-02-28 to 2026-03-03: Phase G** — Maps & Geography (Leaflet.js, markers, DM-only),
  Timeline standalone events, D3 visualization (ruler, grid, eras, clustering, minimap)
- **2026-03-03: Alpha Documentation Sprint** — Bug fixes, extension docs, README
- **2026-03-03: Code Quality Sprint** — golangci-lint v2, 138 lint fixes, JS dedup
- **2026-03-04: Bug Fixes & Testing** — Image upload, apiFetch, HTMX fixes, service tests
- **2026-03-04: Foundry VTT Completion** — Sessions-calendar integration, RSVP emails,
  recurring sessions, Foundry sync (WebSocket, maps API, calendar, shop, fog), security
  hardening, dashboard widgets, mobile responsive, extension documentation
- **2026-03-04: Alpha Hardening** — CI pipeline (golangci-lint + govulncheck), 3 service
  test suites (audit/media/settings), generic IDOR helper, input validation, widget docs,
  TipTap table extensions
- **2026-03-04: Phase H Release Readiness** — Error standardization (249 calls, 15+ files),
  code dedup (MemberLister, LIKE escape), OpenAPI 3.0.3 spec (63 endpoints), extension docs
- **2026-03-05: Sprint I-1 Campaign Export/Import** — Full JSON export/import for campaigns
  (entities, calendar, timelines, sessions, maps, addons, media manifest). 7 adapter services.
- **2026-03-05: Phase I Core UX** — Sprint I-2 (timeline connections + create-from-timeline),
  Sprint I-3 (calendar week view), Sprint I-4 (map marker clustering + expanded POI icons).
- **2026-03-05: Sprint J-1** — Breadcrumbs on all non-entity pages, recently viewed entities sidebar widget.
- **2026-03-05: Sprint J-2** — Code syntax highlighting (lowlight + highlight.js), find/replace bar (Ctrl+F/H).
- **2026-03-05: Sprint J-3** — HTMX verification, `.air.toml`, docker-compose fix, package doc comments.
- **2026-03-05: Sprint J-4** — ClamAV antivirus scanning, docker-compose ClamAV container.
- **2026-03-05: ALL PHASES COMPLETE** — H (release readiness), I (core UX), J (polish & infra).
- **2026-03-05: Sprint K-1** — Per-entity permissions model (backend): migration 000048, model types, permission repository, service methods, visibility filter, 13 tests.
- **2026-03-05: Sprint K-2** — Per-entity permissions UI: visibility section on edit page (Everyone/GM Only/Custom), permission grant builder, API endpoints, visibility indicators across all views.
- **2026-03-05: Sprint K-3** — Module manifest & loader framework: ModuleManifest JSON spec, ModuleLoader auto-discovery, sandboxed Module/DataProvider/TooltipRenderer interfaces, manifest.json for 3 modules, admin page updated, installedAddons wired, 13 tests, ADR-019.
- **2026-03-05: Sprint K-4** — Module data API & widget integration: JSONProvider (JSON-file DataProvider), D&D 5e module (10 SRD spells, tooltip renderer), factory registry pattern, generic module HTTP handler (5 endpoints), Templ reference pages, dynamic addon middleware, ModuleSearcher in entity search, 20 new tests, ADR-020.
