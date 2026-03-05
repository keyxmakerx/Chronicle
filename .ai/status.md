# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-03-05 -- Sprint I-2: Timeline Phase 2B complete (batch 28).
Branch: `claude/project-review-planning-Yr4CL`.

## Current Phase
**Sprint I-2 complete (batch 28).** Timeline Phase 2B delivered.
Ready for Sprint I-3 (Calendar Week View).

### Summary of Recent Work (batches 25-28)
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
1. **Timeline Phase 2B** (Sprint I-2) — Event connections, create-from-timeline modal, beautification.
2. **Calendar week view** (Sprint I-3) — 7-day grid with time blocks.
3. **Map UX polish** (Sprint I-4) — Marker clustering + POI icon picker.
4. **Phase J** — Breadcrumbs, editor enhancements, testing, file security.

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`
- Tailwind standalone CLI (`tailwindcss`) is v3; do NOT use `npx @tailwindcss/cli` (v4 syntax)
- Fog-of-war sync is one-way only (Chronicle → Foundry). Foundry → Chronicle not implemented.
- SimpleCalendar events are limited (managed as journal notes, no CRUD hooks).

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
