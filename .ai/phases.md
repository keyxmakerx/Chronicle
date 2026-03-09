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
| F | Calendar & Time | 2026-02-28 |
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

---

## Upcoming Phases

### Phase V: Obsidian-Style Notes & Discovery ← NEXT

_Quick capture, backlinks, enhanced graph, content templates, editor power-ups.
Priority: **Highest** — DM workflow & competitive parity with LegendKeeper/Obsidian.
See `.ai/obsidian-notes-plan.md` and `.ai/competitive-gap-analysis.md` for details._

#### Sprint V-1: Quick Capture, Session Journal & Slash Commands

Quick-capture modal (Ctrl+Shift+N) creates a timestamped note instantly.
"Session Journal" button in topbar: append to today's journal entry. Add player
notes to Ctrl+K quick search results.

**Also:** TipTap slash command menu (`/` trigger). Commands: heading levels,
bulleted/numbered list, callout, table, image, horizontal rule, code block.
Inspired by LegendKeeper's `/secret`, `/layout`, `/help` pattern.

**Key files:** `static/js/widgets/editor.js`, topbar template, notes widget

#### Sprint V-1.5: Inline Secrets / DM-Only Blocks in Editor (NEW)

Inline secret blocks within the TipTap editor — content visible only to
Owner/Scribe roles, hidden from Players. Rendered with a lock icon and subtle
border in edit mode. Stripped from HTML in read mode for unauthorized roles.

Competitive gap: World Anvil has inline secrets, LegendKeeper has `/secret`
blocks, Kanka has secret posts. Chronicle only has entity-level visibility
and DM-only posts — no inline content hiding within the main editor.

**Key files:** `editor.js` (custom TipTap node), `editor_secret.js`, `sanitize/`

#### Sprint V-2: Backlinks Panel & Entity Aliases

"Referenced By" section on entity show pages (layout block type). Query scans
`entry_html` for `data-mention-id` matching current entity. Collapsible panel
with source entity + context snippet. Redis caching.

**Also:** Entity aliases (migration: `entity_aliases` table). Multiple canonical
names per entity for auto-linking and search. Aliases appear in Ctrl+K results
and auto-linker name detection. Inspired by LegendKeeper's alias system.

**Key files:** entities repo/service/handler, `editor_autolink.js`, search

#### Sprint V-3: Content Templates

Pre-fill editor with structured content (e.g., Session Recap template with
headings). Template picker in entity create flow and editor insert menu.
Per-campaign or global. Owner-customizable.

**Key files:** migration, entities handler/templates, editor widget

#### Sprint V-4: Enhanced Graph View & Cover Images

Include @mention links in graph (not just explicit relations). Filter by entity
type, tag, search. Local graph view (N hops). Cluster by type/tag. Orphan
detection.

**Also:** Cover/banner images on entity pages as a layout block type. Currently
entities show a single avatar image. Add `cover_image` layout block that renders
a full-width hero banner at the top of the entity page. Inspired by World Anvil
and LegendKeeper's page headers.

**Key files:** `relation_graph.js`, layout block types, entity templates

---

### Phase W: Polish, Ecosystem & Delight

_Power-user features, map tools, integrations, bulk operations.
Priority: High — power users & map parity with LegendKeeper._

#### Sprint W-1: Command Palette & Saved Filters

Ctrl+Shift+P action palette with fuzzy search. Saved entity list filter presets
as sidebar links (`saved_filters` table).

#### Sprint W-2: Map Drawing Tools, Regions & Measurement

Leaflet.Draw integration (freehand, polygons, circles, rectangles, text). Uses
existing `map_drawings` table. Per-drawing visibility, color/opacity.

**Also (competitive gaps):**
- **Map regions / territory outlines** — polygon drawing with customizable fills,
  strokes, and labels. Inspired by LegendKeeper's region tool.
- **Map measurement / distance** — calibrate map scale, measure between points.
  Leaflet.Measure plugin or custom ruler tool.
- **Map embed layout block** — embed a mini-map in entity pages that auto-centers
  on the entity's marker. New layout block type `map_embed`.

#### Sprint W-2.5: Nested / Linked Maps (NEW)

Click a map marker to open a sub-map (world → region → city → dungeon). Map
markers gain an optional `linked_map_id` field. Breadcrumb navigation between
map levels. Both World Anvil and LegendKeeper have this — Chronicle maps are
currently flat.

**Key files:** migration (markers), maps handler/templates, `map_widget.js`

#### Sprint W-3: Discord Bot Integration

Plugin at `internal/plugins/discord/`. Bot token config. Webhook session
notifications. Reaction-based RSVP (ADR-012).

#### Sprint W-4: Bulk Operations & Persistent Filters

Multi-select entity lists with batch actions (tag, move, visibility, delete).
Persistent filters per category in localStorage.

**Also:** Entity tag/field filtering on entity list pages (currently only type
tabs exist). Filter by tag, custom field value, or visibility mode.

#### Sprint W-5: Editor Import/Export & Additional Themes

Markdown import/export via `goldmark`. Sepia + high-contrast themes. Custom
accent color picker.

**Also:** Embed media blocks in editor (video/audio URLs). World Anvil embeds
SFX and ambient audio — a simple URL embed block covers the gap.

#### Sprint W-6: Timeline List View & Meter Blocks (NEW)

**Timeline list view** — simple chronological list of events alongside the D3
visualization. LegendKeeper offers list/gantt/calendar views; a list view is
low-effort, high-value for scanning events quickly.

**Meter / tracker blocks** — layout block type for tracking numeric values
(HP, spell slots, ability scores). Configurable min/max, bar/circle/dot styles.
Inspired by LegendKeeper's meter block with 6 visualization styles.

---

### Phase T: Game Systems & Worldbuilding Tools

_Expand reference content and add worldbuilding aids.
Priority: Medium — content depth. T-0 through T-2 already complete._

#### Sprint T-3: Guided Worldbuilding Prompts

`worldbuilding_prompts` table with genre-aware writing prompts per entity type.
"Writing Prompts" collapsible panel on entity edit page. Default prompt packs
(fantasy, sci-fi, horror). Owner-customizable. Inspired by World Anvil's
worldbuilding meta tool.

**Key files:** migration, `entities/handler.go`, `entities/templates/`, `entities/service.go`

#### Sprint T-4: Entity Type Template Library

Genre presets (fantasy, sci-fi, horror, modern, historical) as JSON fixtures.
Campaign creation genre selection. "Import preset" in Customization Hub.

**Key files:** `campaigns/handler.go`, `campaigns/templates/`, fixture JSON files

---

### Phase U: Collaboration & Platform Maturity

_Multi-user collaboration, invites, security hardening, accessibility.
Priority: Medium — multi-user features._

#### Sprint U-1: Role-Aware Dashboards

Role-keyed dashboard layouts (Owner/Scribe/Player each see different dashboard).
Dashboard editor gains role selector dropdown. Players see role-specific or
default fallback.

#### Sprint U-2: Invite System

Migration: `campaign_invites` table (token, email, role, expires_at, used_at).
Email invitations with one-click accept link. Non-public campaigns require
invitation. Invite management UI on campaign settings page.

#### Sprint U-3: 2FA/TOTP

TOTP enrollment with QR code (`pquerna/otp`). Login redirect to TOTP input.
Recovery codes (8, hashed with argon2id). Admin force-disable.

#### Sprint U-4: Accessibility Audit (WCAG 2.1 AA)

ARIA labels, focus traps, skip-to-content, color contrast 4.5:1, keyboard nav,
screen reader live region announcements. axe-core scanning in CI.

#### Sprint U-5: Infrastructure & Deployment

Docker-compose full stack with health checks. Makefile full-stack target.
`CONTRIBUTING.md`. CI against docker-compose.

---

## Backlog (Address Opportunistically)

Items to pick up during related sprints or as standalone tasks. Full details
in `todo.md` under "Backlog" sections.

**UI Consistency:** Alert styling, admin pagination, modal standardization, rate
limiting on mutations, recurring calendar events (beyond yearly).

**Documentation:** Posts widget .ai.md, 16 JS widgets missing .ai.md.

**Player & DM Experience Gaps:** Entity tag/field filtering, print/PDF export,
share links, soft delete/archive, fog of war native UI,
initiative tracker, session prep checklist, NPC generator, account deletion,
activity tracking, timeline search/zoom-to-era, entity version history, toast
grouping, entity image gallery.

**Deferred / Community:** System Builder UI, Draw Steel system, whiteboards,
offline mode, collaborative editing, calendar timezones, map grids, webhooks,
Knowledge Graph addon, dice roller, family tree, cross-campaign search, mobile
modals, moons/seasons/weather in calendar, character sheets, combat tracker.

---

## Recommended Execution Order

```
V-1 (quick capture + slash commands) ─┐
V-1.5 (inline secrets)                │
V-2 (backlinks + aliases)             ├── DM workflow & discovery
V-3 (content templates)               │
V-4 (enhanced graph + cover images)   ┘
        │
        ▼
W-1 (command palette + saved filters) ─┐
W-2 (map drawing + regions + measure)  │
W-2.5 (nested maps)                    ├── Power tools & map parity
W-4 (bulk ops + entity filtering)      │
W-5 (markdown import/export + themes)  │
W-6 (timeline list + meter blocks)     ┘
        │
        ▼
W-3 (Discord) ──── Integration (lower priority)
        │
        ▼
T-3 (prompts) → T-4 (genre presets) ──── Content depth
        │
        ▼
U-1 → U-2 → U-3 → U-4 → U-5 ──── Collaboration & platform
```

---

## Verification Checklist (Per Sprint)

1. `make build` — compilation
2. `make test` — no regressions
3. `make lint` — code quality
4. Manual testing of sprint deliverable
5. Update `.ai/status.md` and `.ai/todo.md`
