# Chronicle Roadmap & Competitive Analysis

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Strategic feature planning based on competitive analysis of     -->
<!--          WorldAnvil, Kanka, LegendKeeper, and Obsidian. Organized by    -->
<!--          Chronicle's three-tier architecture (Plugin/Module/Widget).     -->
<!-- Update: When priorities shift, features are completed, or new           -->
<!--         competitive insights emerge.                                    -->
<!-- ====================================================================== -->

## Competitive Landscape (as of 2026-03)

| Platform | Users | Strengths | Weaknesses |
|----------|-------|-----------|------------|
| **WorldAnvil** | ~1.5M | 25+ article templates, guided prompts, interactive maps, Chronicles (map+timeline), secrets system with per-player granularity, 45+ RPG system support, family trees, diplomacy webs | BBCode editor (dated), steep learning curve, cluttered UI, aggressive paywall (privacy requires paid), heavy auto-renewal complaints |
| **Kanka** | ~300K | Structured 20-type entity model, generous free tier (unlimited entities), deep permissions (visibility per role/user), best-in-class calendar (custom months/moons/-2B to +2B years), GPL source-available, full REST API, marketplace | Summernote editor (mediocre), complex permission UI, self-hosted deprioritized, entity dashboard locked to premium |
| **LegendKeeper** | Small | Best-in-class WebGL maps (regions, navigation, paths), speed/performance focus, real-time co-editing, block-based wiki editor, auto-linking, offline-first architecture, clean minimal UI | Limited entity types, minimal game system support, no formal relation system, newer/smaller feature set |
| **Obsidian** | ~4M+ | Local-first markdown vault, 1000+ community plugins, graph view with backlinks, community themes, full offline support, privacy by default, canvas/whiteboard, extremely fast, extensible via plugin API | Not purpose-built for TTRPGs (requires plugin cobbling: Fantasy Calendar, Leaflet, TTRPG plugin), single-user only (no campaign sharing/roles), no web UI, steep plugin setup, no structured entity types, no built-in calendar/maps/timeline |

### Obsidian Deep Dive

Obsidian deserves special attention because many TTRPG worldbuilders use it despite
it not being purpose-built for the task. Key takeaways:

- **Plugin ecosystem model**: 1000+ plugins created by community. Chronicle's addon
  system is the foundation for similar extensibility. Aspirational target.
- **Graph visualization**: Obsidian's graph view showing note connections is beloved.
  Chronicle should build a relation graph widget (D3.js/Cytoscape.js) to match.
- **Local-first philosophy**: Obsidian works fully offline with files on disk. Chronicle
  is server-based but should consider offline-friendly features (service worker, PWA).
- **Community extensibility**: Obsidian's success comes from empowering developers.
  Chronicle should document its addon API and make extension development easy.
- **TTRPG plugin ecosystem gap**: Obsidian TTRPG users cobble together Fantasy Calendar
  plugin + Leaflet plugin + Dataview + TTRPG Statblocks to approximate what Chronicle
  offers as integrated first-class features. Chronicle's advantage: purpose-built
  calendar with moons/eras, maps with entity-linked pins, campaign roles, timeline
  visualization — all working together natively.

### Where Chronicle Already Wins

1. **Drag-and-drop page layout editor** -- nobody else has visual page design
2. **Customizable dashboards** (campaign + per-category) -- most flexible dashboard system
3. **Self-hosted as primary target** -- no paywall, no forced public content, no storage limits
4. **Modern tech stack** -- TipTap + HTMX + Templ vs BBCode/Summernote
5. **Per-entity field overrides** -- unique; entities can customize their own attribute schema
6. **REST API from day one** -- matches Kanka, beats WorldAnvil and LegendKeeper
7. **Extension framework** -- addons system with per-campaign toggle
8. **Audit logging** -- none of the competitors have this
9. **Interactive D3 timeline** with eras, clustering, minimap -- exceeds Kanka, matches WorldAnvil
10. **Multi-user campaign sharing** -- built-in roles (Owner/Scribe/Player), beats Obsidian entirely

---

## Feature Inventory by Architectural Tier

Everything below is organized by WHERE it lives in Chronicle's architecture:
- **Core** = base website infrastructure, shared templates, middleware
- **Plugin** = `internal/plugins/<name>/` -- feature app with handler/service/repo/templates
- **System** = `internal/systems/` -- game system content pack (read-only, installed via package manager)
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

### Built (since initial roadmap)
- Quick search (Ctrl+K) -- search modal with entity/category results
- Entity hierarchy -- parent selector, tree view, breadcrumbs
- Campaign export/import -- JSON bundle with 7 plugin adapters, slug-based cross-refs
- "View as Player" toggle -- topbar switch to preview player-visible content
- Keyboard shortcuts -- Ctrl+K (search), Ctrl+N (new entity), Ctrl+Shift+L (auto-link)
- Editor find/replace -- Ctrl+F/Ctrl+H with match navigation

### Planned -- Quality of Life

#### hx-boost Sidebar Navigation -- HIGH
**What**: Add `hx-boost="true"` to sidebar links for instant navigation.
**Why**: Biggest perceived performance improvement. Makes Chronicle feel fast like LK.
**Tier**: Core (sidebar templ + boot.js adjustments)

#### Bulk Entity Operations
**What**: Multi-select on entity lists with batch tag, move, delete, privacy toggle.
**Why**: No competitor does this well either. Essential for large worlds.
**Tier**: Core (entities plugin enhancement)

#### Concurrent Editing Safeguards
**What**: Prevent two users from silently overwriting each other's changes.
**Phase 1**: Optimistic concurrency with `updated_at` check (409 Conflict if stale).
**Phase 2**: Pessimistic edit locking with auto-expire (implemented for Notes).
**Phase 3**: Real-time co-editing (LegendKeeper-level, very complex -- long-term).
**Tier**: Core (middleware + service layer)

---

## PLUGIN Features

### Built
- **auth** -- registration, login, logout, password reset, 2FA-ready, admin users
- **campaigns** -- CRUD, roles, membership, ownership transfer, customization hub, export/import
- **entities** -- dynamic types, CRUD, images, layouts, field overrides, per-entity permissions, groups
- **media** -- upload, thumbnails, validation, rate limiting, storage limits, signed URLs (addon: media-gallery)
- **addons** -- extension framework, per-campaign toggle, admin management
- **syncapi** -- API keys, REST v1 endpoints, rate limiting, security events, Foundry VTT sync
- **admin** -- dashboard, user/campaign management, settings, security events
- **settings** -- storage limits, per-user/campaign overrides, storage bypass
- **smtp** -- encrypted SMTP config, email sending
- **audit** -- campaign activity timeline
- **calendar** -- custom months, moons, eras, seasons, events, time system, fantasy/reallife modes, week/day views, drag-and-drop, event categories (addon: calendar)
- **maps** -- Leaflet.js viewer, markers, layers, drawings, tokens, fog of war, marker clustering, Foundry sync (addon: maps)
- **timeline** -- D3.js visualization, standalone events, event connections, entity groups, zoom/pan/drag (addon: timeline)
- **sessions** -- CRUD, attendees, RSVP, recurrence, session recap, entity linking, RSVP sidebar (addon: sessions/calendar)
- **extensions** -- content extension system (Layer 1: declarative packs), WASM runtime (Layer 3: Extism/wazero)
- **relations** -- bi-directional linking, typed connections, D3.js force-directed graph, dm_only
- **tags** -- picker, search, create, colored chips, dm_only, hierarchical
- **posts** -- entity sub-notes with separate visibility, drag-to-reorder

### Planned -- Plugin Enhancements

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
- **Module framework** -- manifest-driven with factory registry, JSON data provider, route registration
- **dnd5e** -- SRD-legal reference data (spells 27, monsters 14, items 10, classes 12, races 9, conditions 15). Category-specific tooltip rendering. 9 tests. Browsable reference pages at `/modules/dnd5e/` (Sprint M-2 in progress)
- **pathfinder2e** -- Scaffold with manifest, no data populated
- **drawsteel** -- Scaffold with manifest, no data populated

### Planned

#### D&D 5e Reference Pages -- IN PROGRESS
**What**: Browsable pages at `/modules/dnd5e/`. Category cards, searchable lists,
formatted stat block detail pages. Quick-search integration.

#### Pathfinder 2e System
**Location**: Installed via package manager (Admin > Packages)
**What**: Same pattern as D&D 5e but for PF2e ORC content.

#### Draw Steel System
**Location**: Installed via package manager (Admin > Packages)
**What**: MCDM's Draw Steel reference data.

---

## WIDGET Features

### Built
- **editor** -- TipTap rich text with auto-save, view/edit toggle, @mentions, find/replace, code syntax highlighting
- **title** -- Inline entity name editor
- **tags** -- Picker with search, create, colored chips, dm_only toggle
- **attributes** -- Dynamic field editor for all types (text, number, select, etc.)
- **relations** -- Bi-directional linking with typed connections, dm_only toggle
- **relation_graph** -- D3.js force-directed graph, zoom/pan/drag, entity tooltips, dashboard block + standalone page
- **mentions** -- @mention search popup with keyboard nav
- **notes** -- Floating panel, TipTap rich text, folders, locking, versions, shared notes
- **dashboard_editor** -- Drag-and-drop layout builder (campaign + category)
- **template_editor** -- Page template builder with 12+ block types
- **entity_type_editor** -- Field definition CRUD
- **sidebar_config** -- Entity type reorder
- **sidebar_nav_editor** -- Custom sections/links CRUD
- **image_upload** -- Drag-and-drop with progress
- **entity_posts** -- Sub-notes with visibility, drag-to-reorder
- **editor_secret** -- Inline secrets (TipTap mark extension, server-side role filtering)
- **editor_autolink** -- Auto-linking entity names (LegendKeeper-style, Ctrl+Shift+L)
- **permissions** -- Per-entity visibility (Everyone/DM Only/Custom with role/user/group grants)
- **groups** -- Campaign group management
- **favorites** -- Entity bookmarks with localStorage
- **recent_entities** -- Recently viewed entities in sidebar
- **search_modal** -- Ctrl+K global search
- **shop_inventory** -- Shop entity type with relation-based inventory
- **shortcuts_help** -- Keyboard shortcuts help panel

### Planned -- New Widgets

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

### Built
- **API documentation** -- OpenAPI 3.0.3 spec at `docs/api/openapi.yaml` (63 endpoints, 42 schemas)
- **Foundry VTT Sync** -- Bidirectional sync: journal entries, maps, calendar events, fog of war.
  WebSocket + REST. Sync mappings, EventBus, shop widget. SimpleCalendar CRUD hooks.

### Planned
- **Foundry Actor Sync** -- Sync character entities with Foundry actors
- **Discord Bot Integration** -- Webhook session notifications, reaction-based RSVP

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

### Service Test Coverage
Tests added for: maps (45), sessions (40+), calendar (40+), timeline (50+), media,
addons, entities, notes, auth. Remaining gaps:
- [ ] Campaigns service -- membership, transfers, dashboard layouts, sidebar config
- [ ] Relations service -- bi-directional create/delete, validation
- [ ] Tags service -- CRUD, slug generation, diff-based assignment
- [ ] Audit service -- pagination, validation, fire-and-forget
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

**Done:**
- ~~Auto-linking~~ -- built (editor_autolink.js, Ctrl+Shift+L)
- ~~Entity sub-notes~~ -- built (entity_posts widget)
- ~~Auto-save indicator~~ -- built (editor auto-save with visual feedback)

**Remaining:**
1. **Backlinks / "Referenced by"** -- surface @mention reverse references on entity pages
2. **Guided prompts** -- "smart questions" per entity type (WorldAnvil-style)
3. **Richer tooltips** -- image + key attributes + excerpt in hover preview
4. **Saved filters** -- filter presets as sidebar smart links
5. **Role-aware dashboards** -- different views per campaign role
6. **Entity type template library** -- genre presets (Fantasy, Sci-Fi, Modern)
7. **Bulk operations** -- multi-select for batch tag/move/delete

---

## Priority Phases

### Completed Phases (D through R)
All major feature phases have been completed. See `.ai/todo.md` for the detailed
sprint-by-sprint completion log. Key milestones:

- **Phase D**: Notes overhaul (locking, rich text, versions, shared)
- **Phase E**: Quick search (Ctrl+K), entity hierarchy, inline secrets
- **Phase F-G**: Calendar + maps plugins (full feature set)
- **Phase H**: Per-entity permissions with groups, campaign export/import
- **Phase I**: Foundry VTT sync (journals, maps, calendar, fog of war)
- **Phase J**: Relation graph, sessions plugin, timeline plugin
- **Phase K**: Auto-linking, per-entity permissions UI, group-based visibility
- **Phase L**: Entity posts, notes folders, calendar day view, DnD
- **Phase M0-M3**: Data integrity, quick wins, JS code quality, test coverage
- **Phase M**: D&D 5e module (data + tooltips, Sprint M-1)
- **Phase P-R**: Extension system (content packs, widget extensions, WASM runtime)

### Current Focus
- Sprint M-2: D&D 5e Reference Pages (browsable `/modules/dnd5e/`)
- Obsidian-style notes (see `.ai/obsidian-notes-plan.md`)
- Quick wins from UX audit

### Future Phases
- **Phase N**: Collaboration & Platform Maturity (role-aware dashboards, invites, 2FA, a11y)
- **Phase O**: Polish & Ecosystem (command palette, map drawing tools, Discord, bulk ops)
- **Phase S+**: Draw Steel module, whiteboards, offline mode, collaborative editing

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

**Remaining competitive gaps**:
- hx-boost for faster navigation (perceived performance)
- Richer entity tooltips (image + attributes + excerpt)
- Guided worldbuilding prompts (WorldAnvil-style)
- Bulk entity operations
- Family tree / genealogy visualization
- Dice roller widget
