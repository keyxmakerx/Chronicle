# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-03-04 -- Documentation cleanup for fresh start (batch 24 complete).
Branch: `claude/interactive-calendar-sessions-UJaq2`.

## Current Phase
**All major features complete through batch 24.** Ready for next priorities.

### Summary of Recent Work (batches 21-24)
- **Batch 24**: Security hardening (input sanitization, rate limit bounds, fail-closed
  addon middleware) + fog-of-war Chronicle→Foundry sync (polygon drawings).
- **Batch 23**: WebSocket security (origin validation, message type validation), device
  fingerprint race fix, extension `.ai.md` documentation (foundry-module, websocket).
- **Batch 22**: Entity page widget blocks (timeline, map_preview, upcoming_events,
  shop_inventory, text_block).
- **Batch 21**: Calendar sessions modal, RSVP inline controls, recurring session
  auto-generation, calendar sessions fragment endpoint, SMTP verification.

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
1. **Interactive calendar sessions** — The approved plan (`declarative-bouncing-cherny.md`)
   for sessions modal overlay on calendar page is ready for implementation. Key pieces:
   remove old session nav buttons, add sessions modal, create sessions fragment endpoint,
   server-side recurring session auto-generation on completion.
2. **Extension documentation** — `.ai.md` writeups still needed for: syncapi, maps drawing
   subsystem, most JS widgets (editor.js, attributes.js, tag_picker.js, relations.js, notes.js).
3. **API documentation** — OpenAPI spec or handwritten REST v1 reference. Auth guide,
   endpoint reference, rate limiting docs, sync protocol.
4. **Test coverage** — Audit service tests, media service tests, settings service tests.
5. **Timeline Phase 2B** — Event connections, create-from-timeline modal, beautification.

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
