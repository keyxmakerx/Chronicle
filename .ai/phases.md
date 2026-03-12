# Chronicle Phase & Sprint Plan

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: High-level roadmap organizing remaining work into themed phases  -->
<!--          with focused sprints. Cross-references todo.md and ADRs.        -->
<!-- Update: When phases are completed or priorities shift.                    -->
<!-- ====================================================================== -->

## Completed Phases

| Phase | Theme | Completed |
|-------|-------|-----------|
| 0 | Project Scaffolding | 2026-02-19 |
| 1 | Foundation (Auth, Campaigns, Entities, Editor) | 2026-02-19 |
| 2 | Media & UI (Security, Dark Mode, Widgets) | 2026-02-20 |
| 3 | UI Overhaul (Terminology, Drill-Down Sidebar) | 2026-02-20 |
| B | Extensions & API (Addons, Sync API, REST v1) | 2026-02-20 |
| C | Notes & Terminology | 2026-02-20 |
| D | Campaign Customization Hub | 2026-02-24 |
| E | Core UX & Discovery (Search, Hierarchy, Shortcuts) | 2026-02-25 |
| F (calendar) | Calendar & Time | 2026-02-28 |
| G | Maps & Geography + Timeline | 2026-03-03 |
| H | Release Readiness (Error types, Dedup, OpenAPI) | 2026-03-04 |
| I | Core UX (Export/Import, Timeline, Calendar, Maps) | 2026-03-05 |
| J | Polish & Infrastructure | 2026-03-05 |
| K | Permissions & Competitive Gap Closers | 2026-03-05 |
| L | Content Depth & Editor Power | 2026-03-05 |
| M0 | Data Integrity & Export Completeness | 2026-03-05 |
| M1 | Quick Wins Sprint | 2026-03-06 |
| M2 | JS Code Quality | 2026-03-06 |
| M3 | Test Coverage | 2026-03-06 |
| P | Extension System — Content Extensions (Layer 1) | 2026-03-07 |
| Q | Extension System — Widget Extensions (Layer 2) | 2026-03-07 |
| R | Extension System — Logic Extensions / WASM (Layer 3) | 2026-03-07 |
| S | Data Integrity & Admin Tooling (ADRs 024-026) | 2026-03-08 |
| T (partial) | Game System Modules — D&D 5e + PF2e (T-0 through T-2) | 2026-03-08 |
| V | Obsidian-Style Notes & Discovery (V-1 through V-5) | 2026-03-11 |
| F (foundry) | Foundry Sync — Journals, Maps, Calendar, Actors (F-1 through F-4) | 2026-03-12 |
| W (partial) | Nav Reorg Mode (W-0) + Visual Customization (W-0.5, in progress) | 2026-03-12 |

---

## Upcoming Phases — Priority Order

### Phase 1: Foundry Completion & QoL ← START HERE

_Finish Foundry VTT sync so it's fully useful end-to-end. Generic system adapter
is the key unlock — without it, only D&D 5e and PF2e get character sync. QoL
sprints fill gaps that make the overall experience feel incomplete._

#### Sprint F-4.5: Generic System Adapter & Dynamic Matching

Remove hardcoded `SYSTEM_MAP` and adapter switch in Foundry module. Make ANY game
system (including custom-uploaded ones) work with character sync automatically.

**Deliverables:**
1. Add `foundry_system_id` field to system manifest schema — custom systems declare
   which Foundry system they're compatible with (e.g., `"foundry_system_id": "dnd5e"`)
2. Add `foundry_path` and `foundry_writable` annotations on character preset field
   definitions (e.g., `"foundry_path": "system.abilities.str.value"`)
3. New `GET /api/v1/systems/:id/character-fields` endpoint exposing field schema
   with Foundry path annotations to the Foundry module
4. New `generic-adapter.mjs` that reads field definitions from API and auto-generates
   `toChronicleFields()` / `fromChronicleFields()` mappings at runtime
5. `_detectSystem()` queries API and matches by `foundry_system_id` instead of static
   JS map. dnd5e/pf2e adapters remain as overrides for edge cases
6. Fields without `foundry_path` are read-only (pushed to Chronicle, not written back)

**Result:** Any custom-uploaded game system with a character preset and foundry_path
annotations gets automatic bidirectional character sync.

**Key files:** `internal/systems/manifest.go`, `foundry-module/scripts/sync-manager.mjs`,
`foundry-module/scripts/generic-adapter.mjs` (new), `internal/plugins/syncapi/api_handler.go`

#### Sprint F-QoL: Foundry Sync Diagnostics & Error Handling

Make the Foundry integration robust and debuggable for real-world use.

**Deliverables:**
1. **Validation report UI** — When uploading a custom system ZIP, show a detailed
   validation report (field count, category count, character preset detection,
   Foundry compatibility check) instead of just success/error
2. **System preview** — Preview what a system provides before enabling (categories,
   presets, field count, Foundry sync compatibility)
3. **Foundry sync health dashboard** — Expand existing sync dashboard with:
   connection uptime, last sync timestamps per entity type, error log with
   actionable messages, retry buttons for failed syncs
4. **Graceful error recovery** — When Foundry disconnects mid-sync, queue pending
   changes and retry on reconnect. Show queued change count in dashboard
5. **Field mapping debug view** — In Foundry module dashboard, show which Chronicle
   fields map to which Foundry paths, highlighting unmapped fields

**Key files:** `internal/systems/campaign_systems.go`, `internal/systems/custom_system.templ`,
`foundry-module/scripts/sync-manager.mjs`, Foundry dashboard templates

#### Sprint F-5: NPC Viewer / Hall (Plugin + Widget + Foundry Sync)

Full NPC plugin at `internal/plugins/npcs/` with its own widget. Campaign route
`/campaigns/:id/npcs` — gallery/grid of revealed NPCs (non-private character
entities). Portrait, name, description, location, faction tags. Filters by
location/organization/relation. "Reveal" = DM toggles visibility.

**Widget:** `npc_gallery` layout block type for entity pages and dashboards.
Mounts a filterable NPC card grid. Registered in block registry.

**Foundry Sync:** NPC gallery on Chronicle stays in sync with Foundry. When a
DM reveals/hides an NPC in Foundry (ownership change), the corresponding
Chronicle entity visibility updates automatically and vice versa. The Foundry
module's actor-sync detects NPC-type actors and pushes visibility changes.

**Key files:** `internal/plugins/npcs/` (new), `static/js/widgets/npc_gallery.js`
(new), `foundry-module/scripts/actor-sync.mjs`, entity templates

---

### Phase 2: System Modularity & Owner Experience

_Validate the full pipeline: owner uploads a custom game system → enables it →
gets reference data, tooltips, entity presets, Foundry sync, character sheets.
Ensure the system framework is truly modular and self-service._

#### Sprint X-1: System Upload UX & Validation

Polish the custom system upload experience so a non-technical owner can succeed.

**Deliverables:**
1. **Step-by-step upload wizard** — Replace bare file input with guided flow:
   (a) upload ZIP, (b) show parsed manifest summary (name, categories, presets,
   field count, Foundry compatibility), (c) confirm and install
2. **Manifest documentation page** — In-app help page or expandable section
   explaining manifest.json schema with annotated example. Link from upload UI
3. **Validation error detail** — On failure, show which specific check failed
   with a fix suggestion (e.g., "Missing `api_version` in manifest.json —
   add `\"api_version\": \"1\"` to the root object")
4. **Sample system download** — "Download sample system ZIP" button that provides
   a minimal working example (3-category, 1 preset, 5 reference items)

**Key files:** `internal/systems/campaign_systems.go`, `internal/systems/custom_system.templ`,
sample ZIP fixture

#### Sprint X-2: System-Provided Entity Presets & Auto-Setup

When a system with entity presets is enabled, automatically offer to create matching
entity types in the campaign.

**Deliverables:**
1. **Preset auto-detection on enable** — When owner enables a system addon, if
   the system has `entity_presets`, show a modal: "This system provides Character
   and Creature presets. Create matching entity types?"
2. **One-click entity type creation** — Each preset creates an entity type with
   the preset's fields, icon, and color pre-configured. Owner can skip or customize
3. **Preset sync on system update** — If owner re-uploads a system with changed
   presets, show diff and offer to update entity types (add new fields, don't
   remove existing ones to avoid data loss)
4. **Preset indicator on entity types** — Badge showing "From: D&D 5e" on entity
   types created from system presets. Link to system reference pages

**Key files:** `internal/plugins/addons/service.go`, `internal/plugins/entities/`,
`internal/systems/manifest.go`

#### Sprint X-3: System-Provided Widgets & Layout Blocks

Allow game systems to contribute UI widgets that appear in the template editor
palette and mount on entity pages.

**Deliverables:**
1. **System widget manifest entry** — New `widgets` array in manifest with widget
   name, JS file path, mount target, and config schema
2. **Widget JS bundling in system ZIP** — Allow `.js` files in system ZIPs (scoped
   to widget registration via `Chronicle.register()` pattern)
3. **Layout block registration** — System widgets appear as available block types
   in the template editor when the system is enabled
4. **Example: Stat Block widget** — D&D 5e system provides a `stat-block` widget
   that renders a formatted stat block from entity fields. Serves as reference
   implementation

**Key files:** `internal/systems/manifest.go`, `static/js/boot.js` (widget discovery),
`internal/widgets/dashboard_editor/`, system ZIP loader

#### Sprint X-4: System Debugging & Diagnostics

Give system authors and campaign owners tools to diagnose issues.

**Deliverables:**
1. **System status page** — `/campaigns/:id/systems/status` showing:
   enabled system, loaded categories, item counts per category, entity presets
   detected, Foundry compatibility status, any load warnings
2. **Reference data browser** — Browsable list of all reference items from the
   system (like the D&D 5e reference pages but auto-generated for any system)
3. **Tooltip preview** — Test tooltip rendering with sample data from the system
4. **Field mapping validator** — For systems with `foundry_path` annotations,
   show a table of field→Foundry path mappings with green/yellow/red status
   (mapped, read-only, unmapped)
5. **System error log** — Surface manifest parse warnings, data file issues,
   and runtime errors in the campaign settings UI

**Key files:** `internal/systems/`, new diagnostic templates, `internal/plugins/addons/`

#### Sprint X-5: Character Sheet Layout Blocks (Foundation)

First step toward character sheets — system-aware layout blocks that display
character data in a formatted sheet style.

**Deliverables:**
1. **Character sheet layout block type** — New block type `character_sheet` in
   template editor. Renders entity fields in a system-specific layout
2. **Default sheet templates** — D&D 5e and PF2e get styled character sheet
   layouts (ability scores grid, HP bar, class/level header)
3. **Field grouping** — Manifest presets gain optional `field_groups` for
   organizing fields into visual sections (e.g., "Abilities", "Combat", "Info")
4. **Editable inline** — Fields in the character sheet block are directly
   editable (click to edit, auto-save via attributes widget API)

**Key files:** `internal/systems/manifest.go`, `static/js/widgets/character_sheet.js` (new),
template editor block types, entity page templates

---

### Phase 3: Maps, Drawing & Spatial Features

_Bring Chronicle maps to parity with LegendKeeper. Drawing tools, nested maps,
regions, and measurement are the most-requested map features._

#### Sprint W-2: Map Drawing Tools, Regions & Measurement

Leaflet.Draw integration (freehand, polygons, circles, rectangles, text labels).
Uses existing `map_drawings` table. Per-drawing visibility, color/opacity. Map
regions with fills/strokes/labels. Measurement/distance tool. Map embed layout
block for entity pages.

**Key files:** `internal/plugins/maps/`, `static/js/widgets/map_widget.js`,
template editor block types

#### Sprint W-2.5: Nested / Linked Maps

Click a map marker to open a sub-map (world → region → city → dungeon). Markers
gain optional `linked_map_id` field. Breadcrumb navigation between map levels.

**Key files:** migration (markers), `internal/plugins/maps/`, `map_widget.js`

---

### Phase 4: Collaboration & Platform Polish

_Multi-user features, power tools, and platform maturity. Makes Chronicle ready
for teams and public instances._

#### Sprint U-1: Role-Aware Dashboards (finish)

Two-dashboard architecture already exists. Add role selector in dashboard editor
so Players/Scribes see role-specific campaign page layouts.

#### Sprint U-2: Invite System

`campaign_invites` table. Email invitations with one-click accept. Non-public
campaigns require invitation. Invite management UI on campaign settings page.

#### Sprint W-1: Command Palette & Saved Filters

Ctrl+Shift+P action palette with fuzzy search. `saved_filters` table for entity
list filter presets as sidebar links.

#### Sprint W-4: Bulk Operations & Persistent Filters

Multi-select entity lists with batch tag/move/visibility/delete. Entity tag/field
filtering on list pages. Persistent filters per category in localStorage.

#### Sprint U-3: 2FA/TOTP

TOTP enrollment with QR code. Login redirect to TOTP input. Recovery codes (8,
hashed with argon2id). Admin force-disable.

#### Sprint U-4: Accessibility Audit (WCAG 2.1 AA)

ARIA labels, focus traps, skip-to-content, color contrast 4.5:1, keyboard nav,
screen reader live region announcements. axe-core scanning in CI.

#### Sprint U-5: Infrastructure & Deployment

Docker-compose full stack with health checks. `CONTRIBUTING.md`. CI against
docker-compose.

---

### Phase 5: Content Depth & Integrations

_Game system content, worldbuilding aids, external integrations._

#### Sprint T-3: Guided Worldbuilding Prompts

`worldbuilding_prompts` table. "Writing Prompts" panel on entity edit page.
Default packs per entity type (fantasy/sci-fi/horror). Owner-customizable.

#### Sprint T-4: Entity Type Template Library

Genre presets (fantasy, sci-fi, horror, modern, historical) as JSON fixtures.
Campaign creation genre selection. "Import preset" in Customization Hub.

#### Sprint W-3: Discord Bot Integration

Plugin at `internal/plugins/discord/`. Bot token config. Webhook session
notifications. Reaction-based RSVP (ADR-012).

#### Sprint W-5: Editor Import/Export & Themes

Markdown import/export via `goldmark`. Sepia + high-contrast themes. Custom
accent color picker. Embed media blocks (video/audio URLs).

#### Sprint W-6: Timeline List View & Meter Blocks

Chronological event list view alongside D3 viz. Meter/tracker layout block for
numeric values (HP, spell slots) with bar/circle/dot styles.

---

### Phase 6: Foundry Advanced & Economy

_Deep Foundry integration: inventory, shops, marketplace. Only after Phase 1
(generic adapter) is stable._

#### Sprint F-6: Armory / Inventory System

Items as entities with game-mechanic fields (weight, cost, rarity). Character
"Inventory" tab via entity relations with metadata (equipped, quantity, attunement).
System-specific item templates. Foundry Actor inventory sync.

#### Sprint F-7: Shop / Marketplace Enhancement

Transaction logging, currency tracking per character, stock management. Foundry:
purchase from shop window updates character inventory on both sides.

---

## Backlog (Address Opportunistically)

Items to pick up during related sprints or as standalone tasks. Full details
in `todo.md` under "Backlog" sections.

**UI Consistency:** Alert styling, admin pagination, modal standardization, rate
limiting on mutations, recurring calendar events (beyond yearly).

**Documentation:** Posts widget .ai.md, 16 JS widgets missing .ai.md.

**Player & DM Experience Gaps:** Entity tag/field filtering, print/PDF export,
share links, soft delete/archive, fog of war native UI, initiative tracker,
session prep checklist, NPC generator, account deletion, activity tracking,
timeline search/zoom-to-era, entity version history, toast grouping, entity
image gallery.

**Deferred / Community:** Module Builder UI, Draw Steel module, whiteboards,
offline mode, collaborative editing, calendar timezones, map grids, webhooks,
Knowledge Graph addon, dice roller, family tree, cross-campaign search.

---

## Recommended Execution Order

```
PHASE 1: Foundry Completion & QoL ────────────────────────
  F-4.5 (generic adapter)  ─┐
  F-QoL (diagnostics)       ├── Make Foundry fully useful
  F-5  (NPC viewer/hall)    ┘
              │
              ▼
PHASE 2: System Modularity & Owner Experience ────────────
  X-1 (upload UX)           ─┐
  X-2 (entity presets)       │  Validate full owner pipeline:
  X-3 (system widgets)       ├── upload → enable → presets →
  X-4 (diagnostics)          │   widgets → Foundry sync
  X-5 (character sheets)     ┘
              │
              ▼
PHASE 3: Maps & Spatial ──────────────────────────────────
  W-2   (drawing tools)     ─┐
  W-2.5 (nested maps)        ┘── LegendKeeper parity
              │
              ▼
PHASE 4: Collaboration & Polish ──────────────────────────
  U-1 (dashboards)  → U-2 (invites)  → W-1 (command palette)
  W-4 (bulk ops)     → U-3 (2FA)      → U-4 (a11y)
  U-5 (infrastructure)
              │
              ▼
PHASE 5: Content & Integrations ──────────────────────────
  T-3 (prompts)  → T-4 (presets) → W-3 (Discord)
  W-5 (editor)   → W-6 (timeline/meters)
              │
              ▼
PHASE 6: Foundry Advanced ────────────────────────────────
  F-6 (armory/inventory) → F-7 (shop/marketplace)
              │
              ▼
BACKLOG ──────────────────────────────────────────────────
  Opportunistic / community contributions
```

**Total remaining sprints:** ~24 feature sprints + backlog items

---

## Verification Checklist (Per Sprint)

1. `make build` — compilation
2. `make test` — no regressions
3. `make lint` — code quality
4. Manual testing of sprint deliverable
5. Update `.ai/status.md` and `.ai/todo.md`
