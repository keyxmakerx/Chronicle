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

---

## Upcoming Phases

### Phase S: Data Integrity & Admin Tooling

_Fix orphaned data, cascade gaps, and give admins visibility into DB health.
Priority: Immediate — data safety._

#### Sprint S-1: Campaign Deletion Cleanup (ADR-025)

| Item | Description |
|------|-------------|
| API key cascade fix | Migration: FK `campaign_id → campaigns(id) ON DELETE CASCADE` on `api_keys`. `api_request_log` gets `ON DELETE SET NULL` (audit retention). |
| Media file disk cleanup | Before SQL DELETE, query `media_files WHERE campaign_id = ?`, delete physical files (main + thumbnails), then delete DB rows. Replaces `ON DELETE SET NULL`. |
| Multi-step delete service | Refactor `campaigns/service.go Delete()`: media cleanup → WASM hook dispatch (`campaign.deleted`) → SQL DELETE (cascades handle rest). |
| Orphan extension cleanup | After campaign delete, check uploaded extensions only enabled for that campaign. If no other campaign uses it, queue uninstall. |

**Key files:** `db/migrations/000058_*.sql`, `campaigns/service.go`, `campaigns/repository.go`, `media/service.go`

#### Sprint S-2: Extension Migration System (ADR-024)

| Item | Description |
|------|-------------|
| Schema tracking table | Migration: `extension_schema_versions` (extension_id, version, applied_at). |
| Per-extension migration runner | Separate from core golang-migrate. Reads `migrations/` from extension zip. |
| Namespaced table enforcement | SQL validator: only `CREATE/ALTER TABLE ext_<slug>_*` allowed. Rejects operations on core tables. |
| Install/uninstall lifecycle | Install: run `up` migrations. Uninstall: run `down` in reverse, drop all `ext_<slug>_*`, delete tracking rows. Disable/enable: no-op (data preserved). |

**Key files:** `extensions/service.go`, `extensions/migration_runner.go` (new), `extensions/sql_validator.go` (new)

#### Sprint S-3: Admin Data Hygiene Dashboard (ADR-026)

| Item | Description |
|------|-------------|
| Orphan detection queries | Read-only scans: orphaned media (campaign_id NULL, not avatars), stale filesystem files (no DB record), orphaned API keys, stale extension provenance. |
| Data Hygiene admin page | `/admin/data-hygiene` with summary stats cards (disk vs DB usage delta, orphan counts), diagnostic results table. |
| Guarded cleanup actions | Purge orphaned media (blocked if entity-referenced), purge stale files, purge orphaned API keys. All logged to `security_events`. |
| Safety guardrails | Cannot delete referenced media or active-campaign extensions. Confirmation dialog with affected item count. Admin audit trail. |

**Key files:** `admin/handler.go`, `admin/repository.go`, `admin/templates/data_hygiene.templ` (new), `admin/routes.go`, `media/repository.go`

---

### Phase T: Game System Modules & Worldbuilding Tools

_Expand reference content and add worldbuilding aids.
Priority: High — content depth._

#### Sprint T-1: D&D 5e Reference Pages

Browsable pages at `/modules/dnd5e/` — category index with cards, searchable
lists per category, formatted stat block detail pages. Quick-search integration
(Ctrl+K returns module reference results).

**Key files:** `modules/dnd5e/handler.go`, `modules/dnd5e/templates/` (new), `modules/dnd5e/routes.go`

#### Sprint T-2: Pathfinder 2e Module

ORC-licensed data following D&D 5e pattern: spells, monsters, ancestries,
classes, conditions, feats. Uses GenericModule (auto-instantiation via
`manifest.json` + `data/`).

**Key files:** `modules/pathfinder2e/` (new directory)

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

**Deferred / Community:** Module Builder UI, Draw Steel module, whiteboards,
offline mode, collaborative editing, calendar timezones, map grids, webhooks,
Knowledge Graph addon, dice roller, family tree, cross-campaign search, mobile
modals.

---

## Recommended Execution Order

```
S-1 (campaign deletion)  ─┐
S-2 (extension migrations) ├── Data safety first
S-3 (data hygiene)        ─┘
        │
        ▼
T-1 (D&D 5e pages) ──── Content depth
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
