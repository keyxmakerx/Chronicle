# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC — thin index, not a session log                        -->
<!-- Purpose: Cross-cutting project state + index of per-plugin .ai.md files. -->
<!-- Update: When release status / branch state / cross-cutting work changes. -->
<!-- ====================================================================== -->

## For humans

### What this file is

A thin index. It documents Chronicle's current high-level state (release version, active phase, cross-cutting items) and points at where per-plugin status lives — each plugin owns its own `.ai.md` per the convention in `cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §0.5 D2=(c)` + `2026-05-23-c-plugin-isolation-audit.md §2.3`.

### What this file is NOT

It is no longer a chronological session-recap log. Pre-2026-05-23 session recaps (51 numbered entries spanning ~135 KB) live in `.ai/archive/status-2026-04-25-pre-shrink.md`. Going forward, per-session deliverables are tracked by the dispatch workflow (`cordinator/decisions/2026-05-19-dispatch-workflow.md`): one report per dispatch in `cordinator/reports/chronicle/`, plus the PR itself.

If you're an AI session looking for "what shipped last week", read the Cordinator working branch (`claude/setup-working-memory-vROh3`) and grep `reports/chronicle/2026-05-*` by date. If you're looking for plugin-specific architecture, footguns, or recent work on plugin X, read `internal/plugins/<X>/.ai.md`.

## For AI sessions

### Current release + branch state

- **Release line:** 0.0.1 (Release Readiness completed 2026-04-25 — backup scripts + mariadb-client in image + deployment runbook)
- **Active phase:** Phase 4 (post-hygiene + post-security + plugin-isolation arc closed; G2/F2 follow-ups + NW-2.3 next). See `cordinator/plans/2026-05-21-master-plan.md` for the phase definition.
- **Coordinator working branch (cordinator artifacts):** `claude/setup-working-memory-vROh3`
- **Recent cross-cutting decisions** (most recent first):
  - chronicle#442 (+ #443 r2) — worldstate render overhaul: cal-almanac-render.css render/chrome CSS split, back+front sky canvases with layered SKY_FX, strip+full-band-sheets GM console. See `internal/plugins/calendar/.ai.md` §"Worldstate render architecture (2026-06)".
  - `cordinator/decisions/2026-05-26-chronicle-production-safety-system.md` — `RunStartupHealthChecks` rubric + docker-unavailable substitute pattern
  - `cordinator/decisions/2026-05-26-ai-export-pipeline-design.md` — AI export pipeline design (future scope, scoping decisions locked)
  - `cordinator/decisions/2026-05-26-draw-steel-spin-up-strategy.md` — Draw Steel game system spin-up strategy (own security audit first)
  - `cordinator/decisions/2026-05-25-plugin-static-assets.md` — per-plugin `embed.FS` static-asset ownership (NW-2.2 Chunk F)
  - `cordinator/decisions/2026-05-23-plugin-registration.md` — lightweight `PluginRegistration` registry (NW-2.2 Chunk A)
  - `cordinator/decisions/2026-05-22-loadDescriptor-fallback.md` — Chronicle/Foundry-Module descriptor wire pin (locked, used by `internal/plugins/foundry_vtt/descriptor_fallback_test.go`)

### Bootstrap reads for a new session

In order:

1. `cordinator/decisions/2026-05-21-core-tenets.md` — binding tenets every session honors
2. `cordinator/decisions/2026-05-19-dispatch-workflow.md` — how dispatches + status reports flow
3. `cordinator/decisions/2026-05-23-decision-routing.md` — backend-vs-frontend question routing
4. This file (you're here) — high-level state + plugin index
5. `.ai/conventions.md` — code patterns
6. `.ai/architecture.md` — three-tier extension model + request flow
7. The relevant plugin's `.ai.md` if your work is plugin-scoped (see index below)

### Plugin .ai.md index (the canonical per-plugin docs)

#### Plugins (22)

| Plugin | `.ai.md` |
|---|---|
| addons | [internal/plugins/addons/.ai.md](../internal/plugins/addons/.ai.md) |
| admin | [internal/plugins/admin/.ai.md](../internal/plugins/admin/.ai.md) |
| armory | [internal/plugins/armory/.ai.md](../internal/plugins/armory/.ai.md) |
| audit | [internal/plugins/audit/.ai.md](../internal/plugins/audit/.ai.md) |
| auth | [internal/plugins/auth/.ai.md](../internal/plugins/auth/.ai.md) |
| backup | [internal/plugins/backup/.ai.md](../internal/plugins/backup/.ai.md) |
| bestiary | [internal/plugins/bestiary/.ai.md](../internal/plugins/bestiary/.ai.md) |
| calendar | [internal/plugins/calendar/.ai.md](../internal/plugins/calendar/.ai.md) |
| campaigns | [internal/plugins/campaigns/.ai.md](../internal/plugins/campaigns/.ai.md) |
| designlab | [internal/plugins/designlab/.ai.md](../internal/plugins/designlab/.ai.md) |
| entities | [internal/plugins/entities/.ai.md](../internal/plugins/entities/.ai.md) |
| foundry_vtt | [internal/plugins/foundry_vtt/.ai.md](../internal/plugins/foundry_vtt/.ai.md) |
| maps | [internal/plugins/maps/.ai.md](../internal/plugins/maps/.ai.md) |
| media | [internal/plugins/media/.ai.md](../internal/plugins/media/.ai.md) |
| npcs | [internal/plugins/npcs/.ai.md](../internal/plugins/npcs/.ai.md) |
| packages | [internal/plugins/packages/.ai.md](../internal/plugins/packages/.ai.md) |
| restore | [internal/plugins/restore/.ai.md](../internal/plugins/restore/.ai.md) |
| sessions | [internal/plugins/sessions/.ai.md](../internal/plugins/sessions/.ai.md) |
| settings | [internal/plugins/settings/.ai.md](../internal/plugins/settings/.ai.md) |
| smtp | [internal/plugins/smtp/.ai.md](../internal/plugins/smtp/.ai.md) |
| syncapi | [internal/plugins/syncapi/.ai.md](../internal/plugins/syncapi/.ai.md) |
| timeline | [internal/plugins/timeline/.ai.md](../internal/plugins/timeline/.ai.md) |

#### Widgets (9)

| Widget | `.ai.md` |
|---|---|
| attributes | [internal/widgets/attributes/.ai.md](../internal/widgets/attributes/.ai.md) |
| editor | [internal/widgets/editor/.ai.md](../internal/widgets/editor/.ai.md) |
| entity_notes | [internal/widgets/entity_notes/.ai.md](../internal/widgets/entity_notes/.ai.md) |
| mentions | [internal/widgets/mentions/.ai.md](../internal/widgets/mentions/.ai.md) |
| notes | [internal/widgets/notes/.ai.md](../internal/widgets/notes/.ai.md) |
| posts | [internal/widgets/posts/.ai.md](../internal/widgets/posts/.ai.md) |
| relations | [internal/widgets/relations/.ai.md](../internal/widgets/relations/.ai.md) |
| tags | [internal/widgets/tags/.ai.md](../internal/widgets/tags/.ai.md) |
| title | [internal/widgets/title/.ai.md](../internal/widgets/title/.ai.md) |

### Cross-cutting state (not plugin-scoped)

#### Closed arc: NW-2.2 plugin-isolation refactor

Per `cordinator/reports/chronicle/2026-05-23-c-plugin-isolation-audit.md` §3 (7 chunks A-G + D2-cleanup):

| Chunk | What | Status |
|---|---|---|
| A | Lightweight `PluginRegistration` registry (`internal/plugins/registry.go`); foundry_vtt + smtp pilots | ✅ shipped PR #334 |
| B | Magic-string consolidation (4 code + 2 templ sites) | ✅ shipped PR #332 |
| C | Cross-plugin import discipline docs (this file's §Cross-plugin imports above) | ✅ shipped PR #333 |
| D | Plugin-specific UI back into owning plugin (4 sub-refactors: banner / dashboard sync block / settings guide / show-banner fragment) | ✅ shipped PR #338 |
| D2-cleanup | Drop unused `fmBanner` + `maps.FoundryPresence` chains exposed by Chunk D; preserves campaigns.FoundryPresenceLookup (live diagnostic) | ✅ shipped PR #342 |
| E | Per-plugin `.ai.md` split + `status.md` shrink via archive-and-thin-index | ✅ shipped PR #335 |
| F | Per-plugin static-asset ownership via `embed.FS` (calendar pilot; other plugins migrate opportunistically) | ✅ shipped PR #336 |
| G | Packages plugin per-row foundry UI fragment; pattern for D. Blocks 2-4 deferred to G2-wave per reshape pattern | ✅ shipped PR #337 |

#### Closed arc: Wave 2 security work (2026-05-22 → 2026-05-26)

| Chunk | What | Status |
|---|---|---|
| 1 + 5 | Password-reset log scrub (`auth.hashEmail`) + `database.SafeIdent` DDL helper | ✅ shipped PR #331 |
| 2 (Phase 2B) | Focused AST middleware-pin for the Foundry public rate-limit invariant (`internal/wire/foundry_public_ratelimit_test.go`) | ✅ shipped PR #339 |
| 3-AMENDED | `syncapi.RequireJSONContentType` middleware + `v1Multipart` sub-group skip pattern (D-C3.1) | ✅ shipped PR #344 |
| 4-AMENDED | `loadDescriptor` fallback snapshot test pinning Chronicle defaults against canonical `chronicle-package.json` from Foundry-Module | ✅ shipped PR #343 |
| 6-AMENDED | Egress HTML sanitization on the 6 `/api/v1/*` GET handlers via `internal/plugins/syncapi/egress_sanitize.go` (D-C6.1); D4=(c) backup/restore lossless preserved | ✅ shipped PR #345 |
| 7 | File-level sanitize-on-write invariant (`internal/sanitize/invariant_test.go` + snapshot) | ✅ shipped PR #340 |
| 8 | `.ai/conventions.md §Security` consolidated reference | ✅ shipped PR #341 |

Wiki All-Pages mobile-layout cosmetic fix (`data-entity-id` on the shared `EntityTableRow` to match the bulk-actions widget's contract): ✅ shipped PR #346.

#### Open work

- **C-SEC-CHUNK-2-PHASE-2C** — full middleware-chain capture for every route via `golang.org/x/tools/go/packages`. Deferred from PR #339's reshape.
- **C-SEC-CHUNK-7-PHASE-2** — method-level sanitize invariant with flow analysis + helper tracing. Deferred from PR #340's reshape.
- **G2-wave** — packages plugin per-row foundry UI Blocks 2-4 (deferred from PR #337).
- **F2-wave** — remaining plugins migrate to per-plugin `embed.FS` static assets (deferred from PR #336; calendar was the pilot).
- **NW-2.3** — move `/foundry-presence` endpoint into the foundry_vtt plugin (currently lives on campaigns; was preserved by D2-cleanup as a parallel structure).
- **Draw Steel spin-up** — pending its own security audit per `cordinator/decisions/2026-05-26-draw-steel-spin-up-strategy.md`.
- **AI Export Pipeline** — scoping locked per `cordinator/decisions/2026-05-26-ai-export-pipeline-design.md`; implementation pending.
- **Plugin Host interface design pass** — tracked in `cordinator/decisions/2026-05-23-plugin-registration.md`. Deferred from Chunk A.
- **C-CAL-WORLDSTATE-EFFECTS-SYSTEM** — synced world-state animation (Almanac sky-band + hourglass) on `/demo/calendar/almanac`, mock-data only. Spec in `docs/design/world-state-effects/`. **Wave 0 + Wave 1 + Wave 2a/2b merged to main; Wave 2c (mood-tint) in review** — this closes the Wave 2 MUST set. Shipped: `worldState` + `setWorldState` pub/sub spine + unified `EFFECTS` registry (PR #388); sun supersession to inline `lorc/sun.svg` + hourglass interior (heightmap sand + day/night + glass/wood materials, PR #389/#390); 10-effect weather/celestial bundle (PR #391); ~28-option moon library w/ vendored Noto/Twemoji + 12 procedurals (PR #394); mood-tint wash (PR #395, merged — Wave 2 MUST closed). **Wave 3 (time-control verb layer) in review** — `timepieceFill`/`atmospherePaused` state + verbs (+1hr/+1day/long-rest/custom/set-time/step-back/atmosphere-pause) tweened on the shared rAF (`CalParticleEngine.addTick`), fill caps ~1/3 → reuse dawn/dusk flip; reusable mechanics in `window.__calTimeControl`. Tests: `test/js/*.test.mjs` (`make test-js`) + Go static guards in `internal/templates/demo/calendar_*_test.go`. Visual verification is the operator's local gate (build env has no headless browser). **After Wave 3:** Waves 4–5 incremental effect long-tail + the production GM Live Control Panel (post-deadline). Queue in `.ai/todo.md §2`.
- **C-TIMELINE-V2-DESIGN-1-TUNER** — the "FM Tuner" timeline showcase on `/demo/timeline/tuner`, mock-data only, page-separated (own `cal-timeline-tuner.{css,js}`), raw SVG + CSS transforms (NO D3). Lead of two candidate timeline designs (Ledger alternate not yet built). Radio-dial etched-metal time axis through the canvas middle with adaptive tick notches (7 zoom levels millennia→days); swim-lanes above/below (entity/category/tier grouping); era gradient bands + watermarks; hover-revealed entity-color-coded connection arcs + show-all toggle; self-contained effect registries with `timelineAxisRender` + `timelineBackdropRender` hooks; §J2 restrained atmospheric backdrop (weather + non-routine celestial always; sun+moons ONLY on special-moon days); §J1 cursor-sync DOM-event protocol (`cal:cursor-change`/`cal:event-create`/`cal:date-jump`, loop-prevented, 50ms drag throttle) — **Almanac amended to emit/listen too** (small `cal-almanac.js` delta + `window.__calCursorSync`). Exempt-OKLCH canvas CSS carries the rendering-canvas exemption marker. Tests: `test/js/tuner.test.mjs` + `test/js/cursor_sync.test.mjs` (+ shared-harness event-bus addition) + Go render/discipline guards in `internal/templates/demo/calendar_timeline_tuner_test.go`. Visual verification is the operator's local gate. **Merged (PR #414).**
- **C-CAL-WORLDSTATE-WIDGETS** — Phase 6 widgetization: graduates the showcase worldState renderers into a production widget + an entity-page block, completing "all three views entity-able" (calendar #411/#413, timeline Tuner #414, worldstate here). New `entity_worldstate` block (`internal/plugins/calendar/entity_worldstate_block.*`) renders the "mini shelf view" (sky band + hourglass-on-shelf) — campaign-level, `Contexts:["template","dashboard"]`, Singleton, friendly empty(Create-calendar CTA)/unavailable states mirroring #413. The `worldState` **provider singleton** (`static/js/widgets/worldstate_provider.js`) is the one source of truth per page: ONE `/calendar/world-state` fetch regardless of widget count (or ZERO when a server seed is embedded), `subscribe`/`onError`/`current`/`push`, shared rAF, reduced-motion, self-destroy on last unsubscribe. The `worldstate` **widget** (`static/js/widgets/worldstate.js`, `Chronicle.register`) drives the SHARED engine (`cal-almanac.js`) via `window.__calSetWorldState` — engine reused, not rebuilt. Rendering canvas reuses the already-exempt `cal-almanac.css` (did NOT duplicate the marked exempt CSS). Tests: `test/js/worldstate_provider.test.mjs` + `worldstate_widget.test.mjs` + Go block tests; widget docs in `static/js/widgets/worldstate.ai.md`. **Merged (PR #415).** Wave-4 per-entity configurable attachment remains OUT of scope (post-deadline widget framework).
- **C-ENTITY-PERMISSIONS-UX** — three entities-plugin presentation changes (one PR): (1) entity card's single **3-state visibility badge** (Everyone `fa-globe` / DM-Only `fa-lock` / Custom `fa-shield-halved`), Scribe+ gated, cards only — `entityVisibilityBadge` in `entity_card.templ`; (2) **inline permission editor** — `permissions.js` gains an opt-in `data-layout="inline"` (the edit-form mount uses it) that renders the widget as a compact summary row expanding in place via the `grid-template-rows 0fr↔1fr` animation (reduced-motion safe), reusing 100% of the grant/load/save path (C-PERMISSIONS-SAVE-FIX intact); slide-in card unchanged for other callers; (3) read-only **Category › Sub-category lineage** in the edit form (`entityTypeLineage`), with `ParentTypeName` now populated via a LEFT JOIN in `entityTypeRepository.FindByID`. Tests: `entity_permissions_ux_test.go` (badge states + player-hidden, inline-layout opt-in, lineage with/without parent) + `test/js/permissions_inline.test.mjs` (inline build/expand/collapse/disclosure + slide-in regression). **Merged (PR #416).** Visual feel of the inline expand is the operator's local gate.
- **C-MAPS-EDITOR-PIN-AND-ICON-PARITY** (operator priority #3) — Chronicle-side of a cross-repo dispatch. **Part A (icon parity):** `internal/plugins/maps/marker_icons.go` is now the canonical source of truth for the 39-icon marker vocabulary; the editor picker renders from `MarkerIconGroups()` and `GET /campaigns/:id/maps/marker-icons` exposes `{default,icons,groups}` as the contract the **Foundry** sync module aligns to (Chronicle authoritative — Option 1 / §A4). **Part B (pin editing in-editor):** double-clicking the map (Scribe+) drops a pin without the separate "Place Marker" toggle (`doubleClickZoom` disabled to avoid the zoom conflict; toggle + marker-list management preserved); shared `MapEditorBody` → applies to the full map page AND the entity-page embed. Tests: `marker_icons_test.go` (catalog integrity/validation/groups, select-from-catalog, inline-create affordances, player-gating, marker-icons API). **FLAGGED:** the Foundry-side translation table (`scripts/map-sync.mjs`) + the create→Foundry→edit→Chronicle round-trip are a **separate Foundry repo/PR** (out of this session's repo scope) — can't be built or round-trip-verified here. **Merged (PR #417).** Inline-pin gesture feel is the operator's local gate.
- **C-AUTH-LOGIN-CSRF-FIX** (login-blocking, HIGH) — fresh logins were failing the double-submit CSRF check with a raw "invalid CSRF token". **Root cause:** `internal/middleware/csrf.go` named the cookie by scheme (`__Host-chronicle_csrf` HTTPS / bare HTTP); behind a TLS-terminating proxy the scheme derived on the POST could differ from the GET that set the cookie, so the lookup missed it and compared the form token against a fresh value → 403. **Fix:** read the cookie under BOTH names (`readExistingCSRF`) + hardened `schemeIsSecure` (parses comma-list `X-Forwarded-Proto`). **Part B:** friendly no-jargon 403 (`middleware.CSRFFriendlyMessage`, flows through `ErrorPage`/HTMX toast) + login auto-recovery (stale-token login POST → `GET /login?expired=1` via HX-Redirect/303 → re-issues token + friendly banner). Tests: `internal/middleware/csrf_test.go` (set→submit, both scheme-flip directions, recovery HTMX+plain, friendly-403, API skip). **(this PR).** ⚠️ Operator confirms a real proxied login post-merge (CI can't reproduce the proxy/scheme condition).
- **C-APPS-CAL-DASH-W1** (E1 Wave 1 of 4) — the Calendars management dashboard as a **dedicated page** (`GET /campaigns/:id/apps/calendar`, Owner), reached from the Extensions hub's per-app "Open dashboard" entry for calendar (now a dedicated-page link via `campaigns.ExtensionDashboardPageURL`; the inline-panel mechanism stays for apps without a dedicated page). **List + detail-pane**: left list via `ListCalendars`; right detail **composes** the existing CRUD (open / settings / setup-wizard / delete / active-switch — no new CRUD) + a **read-only associations panel**. Two new reads, **no migrations**: `EntitiesForCalendar` (entity-ties, joined through `calendar_events`/`calendar_eras` since the link tables carry no calendar_id — corrects the audit) + timeline `ListByCalendar`→`ListTimelinesForCalendar`, exposed to calendar via a **service-interface adapter** (`calendar.TimelineLister`, wired in `app/routes.go` — no repo cross-import, respects plugin isolation). Friendly empty/error states (#413 pattern). Files: `calendar/app_dashboard.{go,templ}`, `entity_ties_repository.go`, `timeline/{repository,service}.go`. Tests: `calendar/app_dashboard_test.go` (+ `EntitiesForCalendar` passthrough), `timeline/list_by_calendar_test.go`, updated hub tests. **Merged (PR #419).**
- **C-APPS-CAL-DASH-W2** (E1 Wave 2 of 4) — live "see in action" embeds in the W1 detail pane, reusing shipped surfaces (no new widgets): LIVE worldstate band (`worldStateBandV2` — the production sky+hourglass the #415 block also wraps) **only when the selected calendar is the campaign's ACTIVE one** (engine-singleton nuance DEFAULT — non-active shows a friendly "set active" note, no widget surgery, no stop-flag needed); the engine-free month grid lazy-loaded via the existing `/calendars/:calId/embed` route (any calendar); per-associated-timeline `timeline-viz` widget mounts (reusing the shipped widget; D3 loaded at page level when timelines exist). **Design call (flagged):** dashboard selection is now a **full navigation** (list rows are plain hrefs, not HTMX detail swaps) because `htmx.config.allowScriptTags=false` means engine/D3 `<script>`s in a swapped fragment won't run, and the engine inits from its seed at load — full-load makes "one live worldstate surface + clean teardown" automatic. No new routes/migrations. Tests: `calendar/app_dashboard_test.go` (active-vs-non-active worldstate branch, grid lazy-load, timeline previews, D3 gating, full-nav rows); reused-widget mount/teardown is covered by the existing `worldstate_*`/boot.js lifecycle. **Merged (PR #420).** Visual feel is the operator's local gate.
- **C-WIDGET-BINDING-P1-SPINE** (the real Wave-4, P1 of 4; supersedes E1 W3/W4 — W1/W2 stand) — generic **host ↔ widget-type ↔ data-instance** binding. New **`widgetbindings` plugin**: a polymorphic **FK-free** `widget_bindings` table (`host_type` ∈ entity/entity_type/dashboard from day one; `widget_type` = registry slug; unique per host+type), a declarative `WidgetType` **Registry** (the dynamic-not-hardcoded answer; namespace guarded in app code, no DB enum), and a `Service` whose **precedence resolver** runs own-binding → entity-type template → **default (= today's behavior)** and returns `{InstanceID, Source}`. **Integrity kit (AND, not OR):** per-plugin delete hook (`OnInstanceDeleted`) + always-on render-time orphan guard (in `Resolve`) + periodic `Sweep`; **campaign scope** is in the repo signature (unscoped read unrepresentable) and checked on **host AND resolved instance** (the leakage vector; MariaDB has no RLS). **Calendar retrofitted** (`calendar_widget_type.go` + `EntityCalendarBlock` takes a resolved `calendarID`; unbound resolves to the campaign default = identical to pre-framework → #411–#420 unchanged). Built-now-dormant: the **entity-type template inheritance** path (unit-tested, surfaced as data in P4). **ADR-038** records why polymorphic-FK-free (the core-before-plugin migration rule makes the integrity-preserving alternatives un-collectable + forces per-type schema churn we forbid). Tests: `widgetbindings/service_test.go` (CRUD, precedence, **directional cascade** type≠>own per Foundry #9818, orphan guard ×3, campaign-scope on bind+resolve, source layer, all-host-types-storable). Sanitize snapshot +1 line (no HTML surface). **Merged (PR #421).**
- **C-WIDGET-BINDING-P2-WORLDSTATE-TIMELINE** (P2 of 4) — retrofits **worldstate** + **timeline** onto the framework as WidgetTypes (no new tables). **Worldstate's instance is a calendar id** (it's a view over a calendar's clock) under a **distinct `"worldstate"` slug** (shares `calendarInstanceBacking` with calendar), so a host can point its hourglass at a different calendar than its calendar embed. **Timeline's instance is a timeline record**; because the entity timeline block is a campaign-level **preview list** (not a single timeline), the timeline type has **no single default** — unbound keeps the list, bound renders the one timeline (via the shipped `timeline-viz` mount). `EntityWorldStateBlock(svc,cc,userID,calendarID)` + `BlockTimeline(cc,timelineID)` take a resolved id; unbound = identical to today. **Delete hooks wired** (P1 left them unconnected): a `BindingCleaner` interface injected into the calendar + timeline services via a type-asserted `SetBindingCleaner` (interfaces unchanged) → `DeleteCalendar` sweeps `calendar`+`worldstate` (both reference calendar ids), `DeleteTimeline` sweeps `timeline`. `InstanceExists` hardened with `errors.As` (the services wrap not-found via `%w` and `apperror.SafeCode` doesn't unwrap → a wrapped 404 must still be sweepable). Two new slugs are append-only registry entries (no schema). Tests: `calendar/widget_type_test.go` + `timeline/widget_type_test.go` (slug, in/cross-campaign + not-found + transient-error guard, default-vs-no-default, delete-hook per type). **Merged (PR #422).** P3 maps + dashboard-as-host · P4 create-or-pick UI.
- **C-WIDGET-BINDING-P3a-MAPS** (P3a of 4) — read-side maps retrofit; maps is the original `entity.map_id` precedent the framework generalizes. New `maps/map_widget_type.go` registers a `"map"` WidgetType (instance = a map id; `InstanceExists` = `GetMap` + campaign-scope, `errors.As` wrapped-404 guard; **no campaign default** — the legacy fallback lives in the closure). The `map_editor` block closure resolves the map id through the framework with a **legacy fallback**: default = today's `entity.map_id`; a `widget_bindings` row (widget_type="map") **wins** when present; **unbound = identical to today** (column drives the embed/picker/empty branches). `DeleteMap` delete-hook wired via the same type-asserted `SetBindingCleaner` seam (interfaces + mocks untouched); the legacy `entity.map_id` is independently SET-NULLed by `fk_entities_map_id` (maps migration 005). **No migration, no schema change** — the `entity.map_id` backfill→bindings + column drop is a **deferred** span-the-layers migration (after P3b). Tests: `maps/map_widget_type_test.go` (slug, in/cross-campaign + not-found + transient guard, no-default, binding-wins, delete-hook). **(this PR).** Deferred: **P3b** dashboard-as-host (unify `DashboardBlockSwitch` onto `BlockRegistry.Render` so `host_type='dashboard'` resolves) · the `entity.map_id` migration · **P4** create-or-pick UI.

### Archive

`.ai/archive/` holds historical docs that have served their purpose:

- `status-2026-04-25-pre-shrink.md` — the 1198-line chronological session log that lived at `.ai/status.md` until 2026-05-23. Pre-Phase-4 session recaps live here.
- `phase-d-plan.md` — Phase D sprint plan (Phase D shipped)
- `security-audit-2026-03-06.md` — the original security audit (superseded by `cordinator/reports/chronicle/2026-05-22-c-security-audit.md`)
- `plan.md` — Draw Steel system implementation plan (work shipped)

### IMPORTANT RULES (mirrored from CLAUDE.md)

Per `cordinator/decisions/2026-05-19-dispatch-workflow.md`:

1. Session-work deliverables → committed PR on the target repo + a Cordinator status report (`reports/chronicle/YYYY-MM-DD-<dispatch>.md` on `claude/setup-working-memory-vROh3`).
2. Plugin-scoped status updates → append to the owning plugin's `.ai.md` "Recent Work" section. Don't bloat this file.
3. Cross-cutting decisions → new file in `cordinator/decisions/` + cite from code.
4. This file's "Cross-cutting state" section gets updated when an arc advances or a release ships.
