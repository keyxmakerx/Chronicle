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

### Phase T: Game Systems & Worldbuilding Tools

_Expand reference content and add worldbuilding aids.
Priority: High — content depth._

#### Sprint T-1: D&D 5e Reference Pages

Browsable pages at `/systems/dnd5e/` — category index with cards, searchable
lists per category, formatted stat block detail pages. Quick-search integration
(Ctrl+K returns game system reference results).

**Key files:** `systems/dnd5e/handler.go`, `systems/dnd5e/templates/` (new), `systems/dnd5e/routes.go`

#### Sprint T-2: Pathfinder 2e System

ORC-licensed data following D&D 5e pattern: spells, creatures, ancestries,
classes, conditions, equipment. Uses GenericModule (auto-instantiation via
`manifest.json` + `data/`).

**Key files:** `systems/pathfinder2e/` (new directory)

#### Sprint T-3: Worldbuilding Prompts

`worldbuilding_prompts` table with genre-aware writing prompts per entity type.
"Writing Prompts" collapsible panel on entity edit page. Default prompt packs
(fantasy, sci-fi, horror). Owner-customizable.

**Key files:** migration, `entities/handler.go`, `entities/templates/`, `entities/service.go`

#### Sprint T-4: Entity Type Template Library

Genre presets (fantasy, sci-fi, horror, modern, historical) as JSON fixtures.
Campaign creation genre selection. "Import preset" in Customization Hub.

**Key files:** `campaigns/handler.go`, `campaigns/templates/`, fixture JSON files

---

### Phase U: Collaboration & Platform Maturity

_Multi-user collaboration, invites, security hardening, accessibility.
Priority: High — multi-user & security._

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

### Phase V: Obsidian-Style Notes & Discovery

_Quick capture, backlinks, enhanced graph, content templates.
Priority: Medium — DM workflow. See `.ai/obsidian-notes-plan.md` for details._

#### Sprint V-1: Quick Capture & Session Journal

Quick-capture modal (Ctrl+Shift+N) creates a timestamped note instantly.
"Session Journal" button in topbar: append to today's journal entry. Add player
notes to Ctrl+K quick search results.

#### Sprint V-2: Backlinks Panel

"Referenced By" section on entity show pages (layout block type). Query scans
`entry_html` for `data-mention-id` matching current entity. Collapsible panel
with source entity + context snippet. Redis caching.

#### Sprint V-3: Content Templates

Pre-fill editor with structured content (e.g., Session Recap template with
headings). Template picker in entity create flow and editor insert menu.
Per-campaign or global. Owner-customizable.

#### Sprint V-4: Enhanced Graph View

Include @mention links in graph (not just explicit relations). Filter by entity
type, tag, search. Local graph view (N hops). Cluster by type/tag. Orphan
detection.

---

### Phase W: Polish, Ecosystem & Delight

_Power-user features, map tools, integrations, bulk operations.
Priority: Medium — power users._

#### Sprint W-1: Command Palette & Saved Filters

Ctrl+Shift+P action palette with fuzzy search. Saved entity list filter presets
as sidebar links (`saved_filters` table).

#### Sprint W-2: Map Drawing Tools

Leaflet.Draw integration (freehand, polygons, circles, rectangles, text). Uses
existing `map_drawings` table. Per-drawing visibility, color/opacity.

#### Sprint W-3: Discord Bot Integration

Plugin at `internal/plugins/discord/`. Bot token config. Webhook session
notifications. Reaction-based RSVP (ADR-012).

#### Sprint W-4: Bulk Operations

Multi-select entity lists with batch actions (tag, move, visibility, delete).
Persistent filters per category in localStorage.

#### Sprint W-5: Editor Import/Export & Themes

Markdown import/export via `goldmark`. Sepia + high-contrast themes. Custom
accent color picker.

---

## Backlog (Address Opportunistically)

Items to pick up during related sprints or as standalone tasks. Full details
in `todo.md` under "Backlog" sections.

**UI Consistency:** Alert styling, admin pagination, modal standardization, rate
limiting on mutations, recurring calendar events (beyond yearly).

**Documentation:** Posts widget .ai.md, 16 JS widgets missing .ai.md.

**Player & DM Experience Gaps:** Entity tag/field filtering, print/PDF export,
share links, soft delete/archive, map measurement, fog of war native UI,
initiative tracker, session prep checklist, NPC generator, account deletion,
activity tracking, timeline search/zoom-to-era, entity version history, toast
grouping, entity image gallery.

**Deferred / Community:** System Builder UI, Draw Steel system, whiteboards,
offline mode, collaborative editing, calendar timezones, map grids, webhooks,
Knowledge Graph addon, dice roller, family tree, cross-campaign search, mobile
modals.

---

## Recommended Execution Order

```
T-1 (D&D 5e pages) ──── Content depth (Ctrl+K integration remaining)
        │
        ▼
U-2 (invite system) ──── Collaboration unlock
        │
        ▼
V-1 (quick capture) ─┐
V-2 (backlinks)      ├── DM workflow
        │             ┘
        ▼
    Fill remaining sprints by priority
    (T-2, U-1, U-3, V-3, V-4, W-*, T-3, T-4, U-4, U-5)
```

---

## Verification Checklist (Per Sprint)

1. `make build` — compilation
2. `make test` — no regressions
3. `make lint` — code quality
4. Manual testing of sprint deliverable
5. Update `.ai/status.md` and `.ai/todo.md`
