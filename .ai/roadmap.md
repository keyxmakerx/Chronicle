# Chronicle Roadmap & Competitive Analysis

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Strategic feature planning based on competitive analysis of     -->
<!--          WorldAnvil, Kanka, and LegendKeeper. Organized by Chronicle's  -->
<!--          three-tier architecture (Plugin/Module/Widget) plus core.       -->
<!-- Update: When priorities shift, features are completed, or new           -->
<!--         competitive insights emerge.                                    -->
<!-- ====================================================================== -->

## Competitive Landscape (as of 2026-02)

| Platform | Users | Strengths | Weaknesses |
|----------|-------|-----------|------------|
| **WorldAnvil** | ~1.5M | 25+ article templates, guided prompts, interactive maps, Chronicles (map+timeline), secrets system with per-player granularity, 45+ RPG system support, family trees, diplomacy webs | BBCode editor (dated), steep learning curve, cluttered UI, aggressive paywall (privacy requires paid), heavy auto-renewal complaints |
| **Kanka** | ~300K | Structured 20-type entity model, generous free tier (unlimited entities), deep permissions (visibility per role/user), best-in-class calendar (custom months/moons/-2B to +2B years), GPL source-available, full REST API, marketplace | Summernote editor (mediocre), complex permission UI, self-hosted deprioritized, entity dashboard locked to premium |
| **LegendKeeper** | Small | Best-in-class WebGL maps (regions, navigation, paths), speed/performance focus, real-time co-editing, block-based wiki editor, auto-linking, offline-first architecture, clean minimal UI | Limited entity types, minimal game system support, no formal relation system, newer/smaller feature set |

### Where Chronicle Already Wins

1. **Drag-and-drop page layout editor** -- nobody else has visual page design
2. **Customizable dashboards** (campaign + per-category) -- most flexible dashboard system
3. **Self-hosted as primary target** -- no paywall, no forced public content, no storage limits
4. **Modern tech stack** -- TipTap + HTMX + Templ vs BBCode/Summernote
5. **Per-entity field overrides** -- unique; entities can customize their own attribute schema
6. **REST API from day one** -- matches Kanka, beats WorldAnvil and LegendKeeper
7. **Extension framework** -- addons system with per-campaign toggle
8. **Audit logging** -- none of the competitors have this

---

## Feature Inventory by Architectural Tier

Everything below is organized by WHERE it lives in Chronicle's architecture:
- **Core** = base website infrastructure, shared templates, middleware
- **Plugin** = `internal/plugins/<name>/` -- feature app with handler/service/repo/templates
- **Module** = `internal/modules/<name>/` -- game system content pack (read-only)
- **Widget** = `internal/widgets/<name>/` + `static/js/widgets/` -- reusable UI block
- **External** = separate repositories (Foundry VTT module, API docs site)

---

## CORE (Base Website) Features

### Built
- Auth (login, register, password reset, PASETO sessions, argon2id)
- Campaign CRUD with role-based membership (Owner/Scribe/Player)
- Entity CRUD with dynamic entity types and FULLTEXT search
- Sidebar drill-down navigation
- Dark mode with semantic color system
- CSRF protection, rate limiting, HSTS
- Admin panel (users, campaigns, extensions, SMTP, storage settings)
- Toast notifications + flash messages
- Pagination
- Landing/discover page (split public/auth)

### Planned -- Mandatory (Table Stakes)

#### Quick Search (Ctrl+K) -- CRITICAL
**What**: Global search modal activated by Ctrl+K. Searches entities, categories, notes.
**Why**: Every competitor has search. LegendKeeper's speed is their brand identity.
**Competitors**: WorldAnvil full-text across all content. Kanka global search. LK auto-linking.
**Implementation**: HTMX-powered modal with debounced input → `GET /campaigns/:id/search?q=`.
Returns entities (with type badge), categories, notes. Keyboard navigation (arrow keys + Enter).
Recent searches in localStorage. No dependency on other features -- can build immediately.
**Tier**: Core (new route + shared templ component + JS in boot.js or new widget)

#### Entity Hierarchy (Parent/Child) -- CRITICAL
**What**: Parent selector on entity create/edit, tree view on category dashboard, breadcrumbs.
**Why**: Every competitor has entity nesting. Serious worldbuilders need hierarchy.
**Competitors**: WorldAnvil infinitely nestable categories. Kanka parent entities. LK tree sidebar.
**Status**: `parent_id` column already in entities table. Zero UI built.
**Implementation**: Parent dropdown in entity form (search existing entities in same category),
breadcrumb trail on entity profile (Campaign > Category > Parent > Entity), tree view toggle
on category dashboard (indented, collapsible), "Create sub-page" button on entity profile.
**Tier**: Core entities plugin enhancement

#### hx-boost Sidebar Navigation -- HIGH
**What**: Add `hx-boost="true"` to sidebar links for instant navigation.
**Why**: Biggest perceived performance improvement. Makes Chronicle feel fast like LK.
**Status**: On backlog (Sprint 5).
**Implementation**: Single attribute on sidebar link container. Main content area swaps
without full page reload. Need to handle page title updates and active state.
**Tier**: Core (sidebar templ + boot.js adjustments)

#### "View as Player" Toggle -- HIGH
**What**: Toggle in topbar that shows only what a Player-role member would see.
**Why**: GMs need confidence that secrets/DM-only blocks are actually hidden.
**Competitors**: WorldAnvil subscriber group previews. Kanka permission previews.
**Status**: On backlog (Sprint 5).
**Implementation**: Toggle button in topbar sets a session/cookie flag. Go templates
check flag + role to filter dm_only blocks, private entities, and (future) inline secrets.
**Tier**: Core (middleware + templ layouts)

#### Campaign Export/Import -- HIGH
**What**: Full JSON export of all campaign data. Import with conflict resolution.
**Why**: Self-hosted users need backup/migration capability. All competitors offer export.
**Competitors**: Kanka export (premium). WorldAnvil export. LK HTML/JSON export.
**Implementation**: `GET /campaigns/:id/export` → JSON bundle (entities, types, layouts,
tags, relations, media manifest). `POST /campaigns/import` with conflict resolution
(skip/overwrite/rename). Media files as separate zip or URL references.
**Tier**: Core (campaigns plugin enhancement)

### Planned -- Quality of Life

#### Keyboard Shortcuts
**What**: Beyond Ctrl+K: Ctrl+N (new entity), Ctrl+E (edit), Ctrl+S (save).
**Why**: No competitor does this well. Power user differentiator.
**Tier**: Core (boot.js enhancement)

#### Bulk Entity Operations
**What**: Multi-select on entity lists with batch tag, move, delete, privacy toggle.
**Why**: No competitor does this well either. Essential for large worlds.
**Tier**: Core (entities plugin enhancement)

#### Persistent Category Filters
**What**: Per-category filter state saved in localStorage.
**Status**: On backlog.
**Tier**: Core (client-side, minimal backend)

#### Concurrent Editing Safeguards
**What**: Prevent two users from silently overwriting each other's changes.
**Phase 1**: Optimistic concurrency with `updated_at` check (409 Conflict if stale).
**Phase 2**: Pessimistic edit locking with auto-expire (planned for Notes overhaul).
**Phase 3**: Real-time co-editing (LegendKeeper-level, very complex -- long-term).
**Tier**: Core (middleware + service layer)

---

## PLUGIN Features

### Built
- **auth** -- registration, login, logout, password reset, admin users
- **campaigns** -- CRUD, roles, membership, ownership transfer, customization hub
- **entities** -- dynamic types, CRUD, images, layouts, field overrides
- **media** -- upload, thumbnails, validation, rate limiting, storage limits
- **addons** -- extension framework, per-campaign toggle, admin management
- **syncapi** -- API keys, REST v1 endpoints, rate limiting, security events
- **admin** -- dashboard, user/campaign management, settings
- **settings** -- storage limits, per-user/campaign overrides
- **smtp** -- encrypted SMTP config, email sending
- **audit** -- campaign activity timeline

### Planned -- New Plugins

#### Calendar Plugin -- DIRE NEED
**Location**: `internal/plugins/calendar/`
**What**: Custom calendars with non-Gregorian months, moons, eras, events linked to entities.
**Why**: Essential for campaign play. Fantasy worlds need 13-month calendars with 3 moons.
**Competitors**: Kanka has the gold standard (custom months, moons, intercalary months,
weather, eras, -2B to +2B year range, entity age auto-calculation). WorldAnvil has
Chronicles combining calendar + map.

**Data model**:
```
calendars:           id, campaign_id, name, description, year_length,
                     week_length, start_year, current_year, current_month,
                     current_day, era_name, created_at, updated_at
calendar_months:     id, calendar_id, name, days, sort_order, is_intercalary
calendar_weekdays:   id, calendar_id, name, sort_order
calendar_moons:      id, calendar_id, name, cycle_days, phase_offset, color
calendar_seasons:    id, calendar_id, name, start_month, start_day,
                     end_month, end_day, description
calendar_events:     id, calendar_id, entity_id (nullable), name,
                     description, year, month, day, is_recurring,
                     recurrence_type, visibility, created_at
```

**UI**: Monthly grid view (like a real calendar), event dots/chips, year overview,
moon phase indicators, era headers, "today" marker for current in-game date.
**Entity integration**: "Born: Day 15 of Flamerule, 1492 DR (age: 34)" on profile.
**Dashboard block**: "Upcoming events" widget for campaign dashboard.
**API**: Calendar endpoints for Foundry VTT sync (see External section).
**Extension**: Register as addon (`calendar` slug) so owners can enable per-campaign.

#### Maps Plugin -- HIGH PRIORITY
**Location**: `internal/plugins/maps/`
**What**: Interactive maps with entity-linked pins, layers, and privacy controls.
**Why**: Genre-defining feature. 2 of 3 competitors treat maps as core.
**Competitors**: LegendKeeper WebGL maps (best-in-class). WorldAnvil Leaflet-based with
pins, layers, marker groups. Kanka basic pin-on-image.

**Phase 1 (Kanka-level)**: Image upload as map base, Leaflet.js viewer, draggable pins
with entity linking, pin popup shows entity tooltip, DM-only pins.
**Phase 2**: Layers with separate images, marker groups with toggle visibility,
polygon regions, nested maps (world → continent → city → dungeon).
**Phase 3 (LK-level)**: WebGL rendering, navigation mode, zoom-dependent visibility.

**Data model**:
```
maps:        id, campaign_id, name, image_path, width, height, parent_map_id,
             bounds_json, created_at, updated_at
map_layers:  id, map_id, name, image_path, sort_order, is_visible, visibility
map_pins:    id, map_id, layer_id, entity_id (nullable), name, description,
             lat, lng, icon, color, visibility, created_at
map_regions: id, map_id, layer_id, entity_id (nullable), name, geojson,
             fill_color, visibility
```

**Extension**: Register as addon (`interactive-maps` slug).

#### Timeline Plugin -- MEDIUM PRIORITY
**Location**: `internal/plugins/timeline/`
**What**: Chronological event display with eras, entity linking, visual layout.
**Why**: WorldAnvil has 3 timeline modes. Kanka has era-based timelines.
**Competitors**: WorldAnvil timescale, list, and Chronicles modes. Kanka era-based.

**Implementation**: Shares infrastructure with calendar plugin (events are time-anchored).
Horizontal scrolling timeline with era sections, event cards linked to entities, zoom
levels (century → decade → year → month). Could be a view mode of the calendar rather
than a fully separate plugin.

**Data model**: Reuses `calendar_events` + adds era definitions.
**Extension**: Register as addon (`timelines` slug).

#### Sessions Plugin -- MEDIUM PRIORITY
**Location**: `internal/plugins/sessions/`
**What**: Session CRUD with date/summary, linked entities, session reports.
**Why**: Bridges worldbuilding and actual play. WorldAnvil's DSTS is a major feature.
**Competitors**: WorldAnvil Digital Storyteller Screen. Kanka session logs.

**MVP**: Session CRUD (date, title, summary), linked entities (NPCs encountered,
locations visited), session report/recap (TipTap editor). No dice roller or live
GM screen in v1 -- that's a much larger undertaking.

**Data model**:
```
sessions:         id, campaign_id, name, date, summary, notes, sort_order,
                  created_at, updated_at
session_entities: session_id, entity_id, role (encountered/mentioned/key)
```

**Extension**: Register as addon (`session-tracker` slug).

### Planned -- Plugin Enhancements

#### Entity Sub-Notes / Posts
**What**: Sub-documents attached to entities with separate visibility and pinning.
**Why**: Kanka's "entity notes" let you attach session notes, GM observations, and
player theories without cluttering the main entry content.
**Status**: Database schema for `entity_posts` referenced in codebase but UI not built.
**Implementation**: Expand existing posts infrastructure. List on entity profile,
CRUD with visibility (everyone/dm_only), pinning, TipTap editor per post.
**Tier**: Entities plugin enhancement

#### Backlinks / "Referenced by"
**What**: Entity B shows "Referenced by: Entity A" when A @mentions B.
**Why**: WorldAnvil does this. Creates organic discovery without manual effort.
**Implementation**: Parse @mentions on entity save, store in `entity_mentions` table.
Display on entity profile as "Referenced by" section. Could be a template block type.
**Tier**: Entities plugin enhancement + new widget block type

#### Saved Filters / Smart Lists
**What**: Save filter presets as custom sidebar links (e.g., "NPCs in Waterdeep").
**Why**: Kanka lets you save filters. Chronicle's custom links could support query params.
**Implementation**: Sidebar link creation supports URL with query parameters.
Entity list page reads query params for initial filter state. Minimal backend change.
**Tier**: Campaigns plugin enhancement (sidebar links already support URLs)

#### Role-Aware Dashboards
**What**: Different dashboard layouts per campaign role (GMs see planning tools,
players see quest logs).
**Why**: Kanka has per-role dashboards (premium).
**Implementation**: Add `role_visibility` field to dashboard blocks OR alternative
layout JSON per role. Natural extension of existing dashboard system.
**Tier**: Campaigns plugin enhancement

---

## MODULE Features

### Built
- **dnd5e** -- Directory structure exists (`internal/modules/dnd5e/`), no reference data populated
- **Module registry** -- `internal/modules/registry.go` with registration system

### Planned

#### D&D 5e SRD Reference Data -- MEDIUM PRIORITY
**Location**: `internal/modules/dnd5e/data/`
**What**: SRD spells, monsters, items, conditions, classes, races as JSON reference data.
Served as searchable reference pages with tooltip integration in the editor.
**Why**: WorldAnvil supports 45+ systems via community statblocks. None of the
competitors serve reference data natively -- this is Chronicle's planned differentiator.
**Implementation**: Populate JSON data files from SRD 5.1 (OGL content). Handler serves
search/list/detail pages. Tooltip API returns summary data for @mention hover previews.
Register as addon (`dnd5e-srd` slug).

#### Pathfinder 2e Module
**Location**: `internal/modules/pathfinder/`
**What**: Same pattern as D&D 5e but for PF2e ORC content.
**Status**: Slot exists in architecture, not started.

#### Draw Steel Module
**Location**: `internal/modules/drawsteel/`
**What**: MCDM's Draw Steel reference data.
**Status**: Slot exists, not started.

---

## WIDGET Features

### Built
- **editor** -- TipTap rich text with auto-save, view/edit toggle, @mentions
- **title** -- Inline entity name editor
- **tags** -- Picker with search, create, colored chips
- **attributes** -- Dynamic field editor for all types (text, number, select, etc.)
- **relations** -- Bi-directional linking with typed connections
- **mentions** -- @mention search popup with keyboard nav
- **notes** -- Floating panel with quick-capture, dual modes, checklists
- **dashboard_editor** -- Drag-and-drop layout builder (campaign + category)
- **template_editor** -- Page template builder with 12 block types
- **entity_type_editor** -- Field definition CRUD
- **sidebar_config** -- Entity type reorder
- **sidebar_nav_editor** -- Custom sections/links CRUD
- **image_upload** -- Drag-and-drop with progress

### Planned -- New Widgets

#### Relation Graph Visualization -- MEDIUM PRIORITY
**Location**: `static/js/widgets/relation_graph.js`
**What**: Force-directed graph showing entity relationships. Filter by type, category, depth.
**Why**: Kanka has relation explorer (premium). WorldAnvil has family trees and diplomacy webs.
**Implementation**: D3.js or Cytoscape.js. Data from existing relations API.
Render as a template block type AND standalone page (`/campaigns/:id/relations/graph`).
Filter controls: relation type checkboxes, category filter, depth slider (1-3 hops).
**Tier**: Widget (JS) + template block type

#### Dice Roller -- LOW PRIORITY
**Location**: `static/js/widgets/dice_roller.js`
**What**: Floating panel with dice expression parser (`2d6+3`, `1d20 advantage`).
**Why**: WorldAnvil has integrated dice. Low effort, high fun factor.
**Status**: `dice-roller` addon exists in extension table (status: planned).
**Implementation**: Floating panel (like Notes widget), expression parser, result history,
animated roll effect. No server-side needed -- pure client-side.
**Tier**: Widget (JS only)

#### Whiteboards / Freeform Canvas -- LOW PRIORITY
**What**: Tldraw or Excalidraw integration for relationship maps and plot planning.
**Why**: LegendKeeper has Tldraw whiteboards. WorldAnvil added whiteboards.
**Implementation**: Embed Tldraw (MIT licensed) as a widget. Store canvas state as JSON
on campaign. Lower priority since relation graph covers the primary use case.
**Tier**: Widget (embed library)

### Planned -- Widget Enhancements

#### Inline Secrets (TipTap Extension) -- HIGH PRIORITY
**What**: `<secret>` marks in TipTap content filtered server-side by role.
**Why**: WorldAnvil's most loved feature. Chronicle has block-level dm_only but not inline.
**Phase 1**: TipTap custom mark for `secret` text. Styled with lock icon + dotted border.
Server-side: strip `<secret>` marks from HTML before rendering for Player role.
**Phase 2**: Per-player reveals (group-based visibility, like WorldAnvil subscriber groups).
**Tier**: Editor widget enhancement + service-layer filtering

#### Auto-Linking (LegendKeeper-style) -- MEDIUM PRIORITY
**What**: Editor automatically recognizes entity names and suggests/creates links.
**Why**: LegendKeeper's auto-linking makes cross-referencing feel magical.
**Implementation**: TipTap extension that fetches campaign entity names on editor init,
matches text patterns, shows suggestion popup. Could be opt-in per campaign.
**Tier**: Editor widget enhancement

#### Guided Worldbuilding Prompts -- LOW PRIORITY
**What**: Collapsible "Inspiration" sidebar when editing entities with contextual questions.
**Why**: WorldAnvil's "smart questions" cure blank-page syndrome. Unique to them.
**Implementation**: Store prompt sets as JSON on entity types (admin-editable).
Display in collapsible panel during entity edit. Seed defaults per category type.
**Tier**: Widget (new sidebar component) + entities plugin (prompt storage)

#### Richer Entity Tooltips
**What**: Expand hover tooltips to show entity image thumbnail, key attributes, first paragraph.
**Why**: WorldAnvil's rich tooltips are praised. LK shows first few lines.
**Implementation**: Expand existing tooltip widget data. API returns image URL + top 3
attributes + first 200 chars. CSS: wider tooltip with image on left.
**Tier**: Mentions widget enhancement

---

## EXTERNAL Features

### API Technical Documentation -- HIGH PRIORITY
**What**: Full REST API documentation (OpenAPI/Swagger spec or handwritten reference).
**Why**: The API exists and works, but has no public documentation. Third-party integrations
(Foundry VTT, mobile apps, custom tools) need documentation to build against.
**Deliverables**:
- OpenAPI 3.0 spec file (`docs/openapi.yaml`) OR handwritten API reference
- Authentication guide (API key creation, Bearer token usage)
- Endpoint reference (request/response schemas, error codes)
- Rate limiting documentation
- Sync protocol documentation (pull/push, conflict resolution)
- Hosted at `/docs/api` or as static site

### Foundry VTT Sync Module -- HIGH PRIORITY
**What**: Foundry VTT module that syncs journal entries, actors, and scenes with Chronicle.
**Why**: The Sync API exists specifically for this use case. Notes sync is the immediate goal,
calendar sync with Foundry calendar modules is the next step.

**Phase 1 -- Notes/Journal Sync**:
- Foundry module connects to Chronicle API using API key
- Syncs journal entries ↔ Chronicle entities (bidirectional)
- Supports pull-on-demand and push-on-save
- Maps Foundry journal folders to Chronicle entity types

**Phase 2 -- Calendar Sync (after Calendar Plugin)**:
- Chronicle calendar ↔ Foundry calendar module (Simple Calendar / Calendaria)
- Sync events, current date, moon phases
- Chronicle is the source of truth; Foundry reflects it during play
- Requires Calendar Plugin API endpoints to be built first

**Phase 3 -- Actor/Entity Sync**:
- Foundry actors ↔ Chronicle character entities
- Sync NPC statblocks, HP, conditions
- Requires D&D 5e module reference data

**Architecture**: Separate git repository (`chronicle-foundryvtt`). Node.js Foundry module
format. Communicates via Chronicle's REST API v1 `/api/v1/` endpoints + sync endpoint.

### Foundry VTT Calendar Integration (Calendaria/Simple Calendar)
**What**: Chronicle calendar data synced to Foundry's calendar plugins via the Foundry module.
**Why**: During play, GMs advance the calendar in Foundry. After play, it syncs back to
Chronicle so the worldbuilding tool and VTT stay in sync.
**Dependencies**: Calendar plugin + Foundry module Phase 2.
**Shenanigans**: The Foundry module acts as a bridge -- it reads Chronicle's calendar API
and writes to Calendaria/Simple Calendar's API, and vice versa. Chronicle defines the
canonical calendar structure; Foundry modules consume it during play.

---

## Testing & Robustness Backlog

### Widget Lifecycle Audit
The `template_editor.js` destroy() fix exposed a broader issue: **all widgets should
be audited for memory leaks when HTMX swaps content.**

Widgets to audit for missing destroy() / cleanup:
- `editor.js` -- TipTap instance destruction, event listeners
- `dashboard_editor.js` -- drag-and-drop listeners
- `sidebar_nav_editor.js` -- event listeners
- `entity_type_editor.js` -- event listeners
- `sidebar_config.js` -- drag listeners
- `notes.js` -- resize handlers, global keydown
- `tag_picker.js` -- document click listener for close
- `attributes.js` -- event listeners
- `relations.js` -- event listeners
- `mentions.js` -- document keydown/click listeners
- `image_upload.js` -- drag-and-drop listeners

Check for: global event listeners without cleanup, setInterval/setTimeout without clear,
fetch requests without abort controllers, DOM references to removed elements.

### Service Test Coverage (Zero Tests)
- [ ] Campaigns service -- membership, transfers, dashboard layouts, sidebar config
- [ ] Relations service -- bi-directional create/delete, validation
- [ ] Tags service -- CRUD, slug generation, diff-based assignment
- [ ] Audit service -- pagination, validation, fire-and-forget
- [ ] Media service -- file validation, thumbnail generation
- [ ] Settings service -- limit resolution, override priority

### Plugin System Robustness
- What happens if an addon's widget JS fails to load?
- Can malformed widget registration crash boot.js?
- Are addon-gated routes properly returning 403/404 when disabled?
- Performance impact of many addons on layout injector query?

### HTMX Fragment Edge Cases
- CSRF token propagation in dynamically loaded fragments
- Widget double-initialization prevention (boot.js WeakMap)
- Nested HTMX targets and event bubbling
- Back/forward browser navigation with boosted links

### Concurrent Editing
- Phase 1: Optimistic concurrency (`updated_at` check → 409 Conflict)
- Phase 2: Pessimistic edit locking with auto-expire
- Phase 3: Real-time co-editing (long-term, complex)

---

## Refinement Ideas (From Competitor Analysis)

These are polish ideas inspired by what works well in competing platforms.
Not new features -- ways to deepen what Chronicle already has.

1. **Backlinks / "Referenced by"** -- surface @mention reverse references on entity pages
2. **Auto-linking** -- TipTap extension to auto-detect entity names (LegendKeeper-style)
3. **Guided prompts** -- "smart questions" per entity type (WorldAnvil-style)
4. **Entity sub-notes** -- sub-documents with separate visibility (Kanka-style)
5. **Richer tooltips** -- image + key attributes + excerpt in hover preview
6. **Saved filters** -- filter presets as sidebar smart links
7. **Role-aware dashboards** -- different views per campaign role
8. **Entity type template library** -- genre presets (Fantasy, Sci-Fi, Modern)
9. **Bulk operations** -- multi-select for batch tag/move/delete
10. **Auto-save indicator** -- clear visual feedback for save state

---

## Priority Phases (Revised)

### Phase D (Current -- Finishing)
- [ ] Player Notes Overhaul (Sprint 4: locking, rich text, versions, shared)
- [ ] hx-boost sidebar navigation (Sprint 5)
- [ ] "View as player" toggle (Sprint 5)
- [ ] Widget lifecycle audit

### Phase E: Core UX & Discovery
- [ ] Quick Search (Ctrl+K)
- [ ] Entity hierarchy (parent_id UI, tree view, breadcrumbs)
- [ ] Backlinks ("Referenced by" on entity profiles)
- [ ] API technical documentation (OpenAPI spec or reference docs)

### Phase F: Calendar & Time
- [ ] Calendar plugin (custom months, moons, eras, events, entity linking)
- [ ] Timeline view (chronological event display, may be a calendar view mode)
- [ ] Dashboard block: "Upcoming events"

### Phase G: Maps & Geography
- [ ] Maps plugin Phase 1 (Leaflet.js, image upload, pins, entity linking)
- [ ] Maps plugin Phase 2 (layers, marker groups, privacy, nested maps)

### Phase H: Secrets & Permissions
- [ ] Inline secrets (TipTap extension, server-side role filtering)
- [ ] Per-entity permissions (beyond everyone/dm_only)
- [ ] Campaign export/import

### Phase I: External Integrations
- [ ] Foundry VTT module Phase 1 (notes/journal sync)
- [ ] Foundry VTT module Phase 2 (calendar sync with Calendaria/Simple Calendar)
- [ ] D&D 5e SRD reference data

### Phase J: Visualization & Play
- [ ] Relation graph visualization
- [ ] Session management plugin
- [ ] Dice roller widget

### Phase K: Delight
- [ ] Auto-linking in editor
- [ ] Guided worldbuilding prompts
- [ ] Role-aware dashboards
- [ ] Entity type template library
- [ ] Whiteboards

---

## Strategic Positioning

**Chronicle's pitch**: "The self-hosted worldbuilding platform that gives you WorldAnvil's
depth, Kanka's structure, and LegendKeeper's speed -- with full data ownership and no paywall."

**Protect and expand these differentiators**:
- Drag-and-drop page layouts (nobody else has this)
- Customizable dashboards at every level
- Self-hosted with no feature tiers
- Three-tier extension architecture
- Game system modules with native reference data
- REST API with sync protocol

**Close these gaps urgently**:
- Quick search (every competitor has it)
- Entity hierarchy (every competitor has it)
- Calendar (essential for campaign play)
- Maps (genre-defining feature)
- Inline secrets (WorldAnvil's most loved feature)
- API documentation (API exists but undocumented)
- Faster navigation via hx-boost
