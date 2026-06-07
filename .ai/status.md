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
- **C-CAL-WORLDSTATE-WIDGETS** — Phase 6 widgetization: graduates the showcase worldState renderers into a production widget + an entity-page block, completing "all three views entity-able" (calendar #411/#413, timeline Tuner #414, worldstate here). New `entity_worldstate` block (`internal/plugins/calendar/entity_worldstate_block.*`) renders the "mini shelf view" (sky band + hourglass-on-shelf) — campaign-level, `Contexts:["template","dashboard"]`, Singleton, friendly empty(Create-calendar CTA)/unavailable states mirroring #413. The `worldState` **provider singleton** (`static/js/widgets/worldstate_provider.js`) is the one source of truth per page: ONE `/calendar/world-state` fetch regardless of widget count (or ZERO when a server seed is embedded), `subscribe`/`onError`/`current`/`push`, shared rAF, reduced-motion, self-destroy on last unsubscribe. The `worldstate` **widget** (`static/js/widgets/worldstate.js`, `Chronicle.register`) drives the SHARED engine (`cal-almanac.js`) via `window.__calSetWorldState` — engine reused, not rebuilt. Rendering canvas reuses the already-exempt `cal-almanac.css` (did NOT duplicate the marked exempt CSS). Tests: `test/js/worldstate_provider.test.mjs` + `worldstate_widget.test.mjs` + Go block tests; widget docs in `static/js/widgets/worldstate.ai.md`. **(this PR).** Wave-4 per-entity configurable attachment remains OUT of scope (post-deadline widget framework).

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
