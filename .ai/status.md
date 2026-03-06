# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-03-06 -- Generic module framework + Sprint M-1 D&D 5e data COMPLETE.
Branch: `claude/phase-r-logic-extensions-HybRz`.

## Current Phase
**Phase M: Game System Modules — Sprint M-1 COMPLETE + Generic Module Framework.** Any game system can now be added with just a `manifest.json` + data files — zero custom Go code required.

### Generic Module Framework (COMPLETE)
- **GenericTooltipRenderer** (`generic_tooltip.go`): Reads field definitions from the manifest's `categories[].fields[]` to render tooltips. Shows only manifest-declared fields in manifest-defined order. Works for any game system.
- **GenericModule** (`generic_module.go`): Wraps JSONProvider + GenericTooltipRenderer. Implements the Module interface with zero game-system-specific code.
- **Auto-instantiation** (`loader.go`): When a module directory has `manifest.json` + `data/` but no registered Go factory, the loader automatically creates a GenericModule instance. New game systems work by just dropping files.
- **Tests**: 6 new tests — generic renderer (supported categories, render with manifest fields, field filtering, field ordering, nil item), generic module end-to-end, loader auto-instantiation.
- **D&D 5e** retains its custom `TooltipRenderer` as an override for richer formatting, but could also work with the generic renderer.

### Sprint M-1: D&D 5e Module — Data & Tooltip API (COMPLETE)
- **Module wiring**: `modules.Init("internal/modules")` in main.go with blank import of dnd5e for factory registration, `modules.RegisterRoutes()` in app/routes.go
- **SRD data files** (6 categories, 87 items):
  - `spells.json`: 27 spells across levels 0-9 (cantrips through Wish)
  - `monsters.json`: 14 monsters (Goblin through Lich, CR 1/4 to 21)
  - `items.json`: 10 magic items (Potion of Healing through Vorpal Sword)
  - `classes.json`: All 12 SRD classes with hit die, primary ability, saving throws, proficiencies
  - `races.json`: 9 SRD races with speed, size, ability bonuses, traits, languages
  - `conditions.json`: All 15 SRD conditions with effect summaries
- **Manifest**: Added conditions category (slug, name, icon, fields) to dnd5e manifest.json
- **Tooltip renderer**: Category-specific `writeCategoryProperties()` switch — each category shows only its relevant fields
- **Tests**: 9 tests in dnd5e_test.go
- **Build**: Full project compiles clean, all module tests pass

### Sprint R-1: WASM Runtime Integration (COMPLETE)
- Added Extism Go SDK v1.7.1 + wazero v1.9.0 dependencies
- **WASM model types** (`wasm_model.go`): WASMPlugin, WASMContribution, WASMCapability, WASMCallRequest/Response, HookEvent, WASMPluginInfo, WASMLimits — full type system for WASM logic extensions
- **Manifest integration**: Added `WASMPlugins` to `ManifestContributes`, validation for slug/name/file (.wasm required), capabilities (5 types), hooks (8 event types), memory limits (max 256 MB), timeouts (max 300s)
- **PluginManager** (`wasm_manager.go`): Load/Unload/Reload/Call lifecycle with capability-based host function filtering, timeout enforcement, concurrent-safe plugin map, error state tracking
- **Host functions** (`wasm_host.go`): 10 host functions across 5 capability groups:
  - `log`: chronicle_log (with message truncation)
  - `entity_read`: get_entity, search_entities, list_entity_types
  - `calendar_read`: get_calendar, list_events
  - `tag_read`: list_tags
  - `kv_store`: kv_get, kv_set (64KB limit), kv_delete
- **Service adapter interfaces**: EntityReader, CalendarReader, TagReader — read-only data access for WASM plugins via adapter pattern
- **KV store** (`wasm_kvstore.go`): Per-plugin key-value storage backed by existing `extension_data` table (namespace "wasm_kv"), no new DB tables needed
- **Hook dispatcher** (`wasm_hooks.go`): Fire-and-forget async dispatch to WASM plugins registered for events. 8 convenience methods (DispatchEntityCreated/Updated/Deleted, DispatchCalendarEvent*, DispatchTag*)
- **WASM handler** (`wasm_handler.go`): Admin endpoints (list/get/reload/stop plugins) + campaign endpoints (list campaign plugins, call plugin function with Scribe+ role)
- **Routes** (`routes.go`): RegisterWASMAdminRoutes, RegisterWASMCampaignRoutes with proper auth/role middleware
- **Security**: .wasm added to allowedFileExts, capability-based host function filtering (principle of least privilege), memory/timeout limits, log truncation, KV value size cap
- **Repository**: Added DeleteDataByKey method for per-key KV deletion
- **Tests**: 26 new tests — manifest validation (15 cases), model defaults, capabilities, hook types, plugin key generation, serialization, manager lifecycle, security allowlist, zip entry validation, context helpers, log drain/limit
- **New files**: wasm_model.go, wasm_manager.go, wasm_host.go, wasm_kvstore.go, wasm_hooks.go, wasm_handler.go, wasm_test.go

### Sprint R-4: Plugin SDK & Developer Tools (COMPLETE)
- **Example Rust plugin** (`extensions/example-wasm-rust/`): Auto-tagger that hooks into entity.created, looks up entity type, and applies matching tags. Demonstrates Extism Rust PDK with host function declarations, hook handling, dice roller export, and messaging.
- **Example Go plugin** (`extensions/example-wasm-go/`): Session logger that records entity activity to KV store, creates calendar event summaries. Demonstrates Extism Go PDK with `//go:wasmimport` host functions, KV storage, and calendar write.
- **Go SDK** (`sdk/go/chronicle/`): Type definitions for all host function inputs/outputs, hook events, capability constants, and `MockHost` test harness with in-memory implementations of all host functions.
- **MockHost test harness**: Entity/tag/calendar/relation/KV/log/message mocks with setup helpers and inspection methods. 9 unit tests.
- **Plugin development guide** (`.ai/plugin-development.md`): Complete documentation covering capabilities, all 16 host functions, hooks, building (Rust/Go/TinyGo), testing with MockHost, resource limits, API endpoints.
- **Manifest tests**: 7 new test cases — 5 example manifests validated + 2 WASM-specific assertion tests (capabilities, hooks, config, limits).
- **AI docs**: `.ai.md` files for both example plugins, README index updated.

### Sprint R-3: Write Host Functions & Messaging (COMPLETE)
- **5 new write capabilities**: entity_write, calendar_write, tag_write, relation_write, message
- **6 new host function builders** (`wasm_host.go`):
  - `update_entity_fields` — updates entity custom fields via EntityWriter adapter
  - `create_event` — creates calendar events via CalendarWriter adapter
  - `set_entity_tags` — replaces entity tag set via TagWriter adapter
  - `get_entity_tags` — reads entity tags via TagWriter adapter
  - `create_relation` — creates entity relations via RelationWriter adapter
  - `send_message` — async plugin-to-plugin messaging via PluginManager back-reference
- **4 new write interfaces**: EntityWriter, CalendarWriter, TagWriter, RelationWriter
- **4 new write adapters** (`wasm_adapters.go`): NewWASMEntityWriteAdapter, NewWASMCalendarWriteAdapter, NewWASMTagWriteAdapter, NewWASMRelationWriteAdapter
- **App wiring** (`app/routes.go`): All write adapters wired with closure-based JSON marshaling/unmarshaling, PluginManager back-reference set for messaging
- **Security**: Input size limits on all write functions (256KB fields, 64KB events/relations/messages), required field validation
- **Total host functions**: 16 across 10 capability groups (was 10 across 5)
- **Tests**: 10 new tests — write adapter delegates (4), write capability counts (6), nil-adapter guard tests (2)
- **AllCapabilities** updated from 5 to 10 entries

### Sprint R-2: App Wiring & Admin UI (COMPLETE)
- **App wiring** (`app/routes.go`): EntityReader, CalendarReader, TagReader adapters with JSON serialization closures wrapping concrete services. KV store backed by extension_data. PluginManager, HookDispatcher, WASMHandler all instantiated and registered
- **Adapters** (`wasm_adapters.go`): NewWASMEntityAdapter, NewWASMCalendarAdapter, NewWASMTagAdapter — closure-based adapters avoiding direct plugin imports
- **Auto-loading**: Content applier now auto-loads WASM plugins when extensions are enabled (SetWASMLoader interface on ContentApplier)
- **Graceful shutdown**: main.go unloads all WASM plugins before stopping server
- **App struct**: WASMPluginManager + WASMHookDispatcher fields for lifecycle management
- **Admin UI**: Extension detail page now shows Widgets and WASM Plugins in the contributes section (capabilities as violet badges, hooks as amber badges)
- **Tests**: 12 new tests — adapter delegates (entity/calendar/tag), capability-based host function filtering (8 cases), call context lifecycle, unload/reload/call error paths
- **New files**: wasm_adapters.go, wasm_adapters_test.go

### Phase P Summary (Sprints P-1 through P-6)
- **P-1**: Extension infrastructure — migration 000055 (4 tables), manifest parser/validator, zip security, repository (16 methods), service, handler, routes, config
- **P-2**: Admin UI — polished extension list with card layout, extension detail page (manifest metadata, author, contributes breakdown, dependencies), admin sidebar link, HTMX rescan/update
- **P-3**: Campaign integration — content extensions lazy-loaded in campaign addons settings page and customization hub extensions tab
- **P-4**: Content appliers — entity type templates and tag collections applied on enable with provenance tracking. Adapter pattern bridges entity/tag services
- **P-5**: Marker icons and themes — icon pack registration in extension_data, theme CSS registration, API endpoints for marker-icons and themes
- **P-6**: Example extensions — Harptos Calendar (Forgotten Realms) and D&D 5e Character Sheet with 4 entity types, creature tags, relation types. Unit tests validate manifests
- **Package**: `internal/extensions/` — 11 files (model, manifest, security, repository, service, handler, routes, applier, adapters, templ, tests)
- **Tests**: 41 tests (manifest parsing, security, SVG/CSS validation, UUID, example manifests)

### Extension System Research (batch 56)
- **ADR-021**: Layered third-party extension strategy recorded in `.ai/decisions.md`.
- **Decision**: Three layers — (1) Content Extensions (manifest-only, no code), (2) Widget Extensions (browser-sandboxed JS), (3) Logic Extensions (WASM via Extism/wazero, future).

### Entity Block Registry (batch 55-56)
- **Bug fix**: `validBlockTypes` in entities/service.go was missing most block types, causing "invalid block type" errors for shop_inventory, calendar, timeline, etc.
- **Architecture**: Replaced hardcoded block type lists with a self-registering `BlockRegistry`. Plugins register their block types at startup; validation, rendering, and the template editor palette all derive from the registry.
- **New files**: `entities/block_registry.go` (registry types + config helpers), `entities/block_registry_core.go` (core block registrations), `calendar/blocks.templ`, `timeline/blocks.templ`, `maps/blocks.templ` (plugin block renderers moved to owning plugin packages).
- **Modified**: `entities/show.templ` (switch → registry dispatch), `entities/service.go` (registry-based validation), `entities/handler.go` (block-types API), `entities/routes.go` (new endpoint), `app/routes.go` (registry wiring), `template_editor.js` (fetches block types from API), `template_editor.templ` + `entity_type_config.templ` (added data-campaign-id).
- **API**: `GET /campaigns/:id/entity-types/block-types` — returns available block types filtered by campaign addons.
- **Build fix**: Renamed `blockEntry` struct to `registeredBlock` in `block_registry.go` to resolve naming collision with `blockEntry` templ function in `show.templ`.

### Sprint M0-4: dm_only Visibility on Entity Relations (batch 48)
- Migration 000052: `dm_only BOOLEAN NOT NULL DEFAULT FALSE` on `entity_relations`
- Model: `DmOnly bool` on `Relation`, `CreateRelationRequest`, `GraphRelation`
- Repository: all queries updated (Create, FindByID, ListByEntity, ListByCampaign)
- Service: `Create()` accepts variadic `dmOnly`, `GetGraphData()` accepts `includeDmOnly` filter
- Handler: DM authorization check, role-based filtering in List/Graph endpoints
- Export: `ExportRelation.DmOnly` field, export/import adapters propagate dm_only
- JS widget: DM-only toggle in create modal, lock badge on DM-only relations
- Template: `data-is-dm` attribute on relations widget mount point
- **Files changed**: migration files, model.go, repository.go, service.go, handler.go, export.go, export_adapters.go, show.templ, relations.js

### Sprint M0-1 through M0-3 (batch 47)
- M0-1: Export adapters for permissions, groups, posts
- M0-2: Timeline connections, entity groups, session attendees
- M0-3: Entity parent hierarchy resolution on import

### UX & Feature Gap Audit (batch 46)
Deep audit of player/DM experience, account settings, campaign management, and missing UI surfaces:
- **Account gaps**: No in-app password change (only email reset), display name read-only after registration, avatar infrastructure exists (DB column + media service) but no upload UI, 2FA DB columns exist but incomplete.
- **Player experience**: No entity favorites/bookmarks, no sort controls on entity lists, no tag/field filtering, no character assignment to campaign members, no session recap field, no dice roller.
- **DM tools**: No soft delete/archive (hard delete only), no bulk entity operations, no map measurement/drawing/fog UI (backend exists for Foundry sync), no initiative tracker, no session prep checklist.
- **Campaign settings**: Export/import handlers exist but no visible button on settings page, no invite link system (email-add only), no member kick confirmation dialog.
- **Quick wins identified**: 10 small-but-impactful items added to todo.md (export button, password change, sort controls, notes search, calendar search, favorites, etc.).
- **17 new items** added to `.ai/todo.md` under "Quick Wins" and "Player & DM Experience Gaps" sections.
- Fixed errcheck lint errors in `json_provider_test.go` (2 unchecked `os.WriteFile` calls that failed CI).

### Audit Summary (batch 45)
Created `.ai/audit.md` — comprehensive feature parity and completeness audit covering:
- **Test coverage**: 530+ tests but 4 plugins have zero tests (maps, sessions, admin, smtp), calendar/timeline have only domain-logic tests. No handler or repository tests anywhere.
- **Export gaps**: entity_permissions, campaign_groups, entity_posts not exported (data loss on backup).
- **JS consistency**: 17 widgets still use raw fetch() instead of Chronicle.apiFetch(). Only 2 widgets show toast on error; rest log to console silently. groups.js and relation_graph.js duplicate escHtml().
- **Feature parity**: Relations lack visibility controls (unlike tags, posts, markers). Notes and posts have no search.
- **Documentation**: 16 JS widgets lack .ai.md files. Posts Go widget has no .ai.md.
- 14 new items added to `.ai/todo.md` under "Low (Audit-Discovered)" section.

### Summary of Recent Work (batches 25-44)
- **Batch 44**: Sprint L-5 Calendar Day View — Single-day detailed view at
  `/calendar/day`. DayViewData struct with PrevDay/NextDay/WeekdayName/Season
  helpers. Full-page template with event cards showing time, category, entity
  links, description. Day view icon added to all view toggles (Grid/Week/Day/
  Timeline). Session display for real-life calendars. 5 unit tests. Route:
  `GET /calendar/day` (Player+).
- **Batch 43**: Sprint L-4 Calendar Event Drag-and-Drop — HTML5 DnD on monthly
  grid view. Event chips gain `draggable="true"` (Scribe+ only), day cells become
  drop zones with `data-drop-year/month/day` attributes. Drag handlers: dragStart
  captures event ID + applies opacity, dragOver highlights cell, drop reads all
  event data attributes and sends full PUT to `/calendar/events/:eid` with new
  date. Visual feedback via `cal-drop-highlight` CSS class (accent ring + tint).
  Same-date drops ignored. No backend changes needed (existing PUT handler works).
- **Batch 42**: Sprint L-3 Note Folders — Migration 000051 adds `parent_id` (FK
  with CASCADE) and `is_folder` columns to notes table. Backend: model, repository,
  and service updated for folder create/update/move operations. JS widget (`notes.js`)
  updated with tree view rendering: `buildTree()` groups notes by parentId, folders
  render as collapsible containers with expand/collapse (persisted in localStorage),
  child count badges, add-note-in-folder and rename-folder buttons. Move-to-folder
  dropdown menu on note cards. "New Folder" button in quick-add row. Folder delete
  with cascade warning. CSS styles for folders, move menu, collapse toggle. 4 new
  unit tests (create folder, create with parentId, move to folder, move to top level).
- **Batch 41**: Sprint L-2 Notes Rich Text (TipTap) — Replaced plain textarea
  editing in notes widget with mini TipTap editor instances. When entering edit
  mode, creates TipTap editor with StarterKit+Underline+Placeholder, populated
  from note's `entry` JSON or converted from legacy text blocks to HTML. Saves
  TipTap content (entry JSON + entryHtml) to API. Legacy block→TipTap conversion
  on first edit. Checklists remain as interactive checkboxes (separate from TipTap).
  Editor instances tracked in `miniEditors` map, cleaned up on note save/destroy.
- **Batch 40**: Sprint L-1 Entity Posts (Sub-Notes) UI — Migration 000050
  (`entity_posts` table). Full widget: PostRepository (CRUD + reorder),
  PostService (validation, sort order), Handler (list/create/update/delete/reorder).
  JS widget (`entity_posts.js`) with collapsible post cards, drag-to-reorder,
  visibility toggle (DM only), inline rename, delete confirmation. Integrated into
  entity show page below main entry. Added as layout block type `posts` in
  template editor. Public-capable read route, Scribe+ write routes. 13 unit tests.
- **Batch 39**: Sprint K-5 Relations Graph Visualization — D3.js force-directed graph
  widget (`relation_graph.js`) with dynamic CDN loading, zoom/pan, drag, node coloring
  by entity type, edge labels, tooltips, click-to-navigate, type legend. Backend:
  `ListByCampaign` repository (dedup bi-directional pairs via `source < target`),
  `GetGraphData` service, `GraphAPI` + `GraphPage` handlers. Standalone page at
  `/relations-graph/page`. Dashboard block type `relations_graph` with configurable
  height. Model types: GraphRelation, GraphNode, GraphEdge, GraphData. Phase K complete.
- **Batch 38**: Sprint K-4 Auto-Linking in Editor — Entity names API endpoint
  (`GET /entity-names`) with Redis caching (5-min TTL). Repository `ListNames`
  method returns lightweight name entries (id, name, slug, type info) sorted by
  name length DESC for longest-first matching. Auto-link JS module
  (`editor_autolink.js`) scans editor text nodes for entity names, creates
  @mention links with data-mention-id attributes. Integrated into Insert menu
  ("Auto-link Entities" with wand icon) and Ctrl+Shift+L shortcut. Whole-word,
  case-insensitive matching, min 3 chars, skips text already inside links.
  `EntityNameEntry` model type. Handler gains Redis `cache` field.
- **Batch 37**: Sprint K-3 Group-Based Visibility — Migration 000049
  (`campaign_groups` + `campaign_group_members` tables, subject_type ENUM gains
  "group"). Full GroupRepository (8 methods) and GroupService (validation, CRUD).
  Group CRUD handlers (list/create/get/update/delete groups + add/remove members)
  with Owner-only routes. Groups management page (`groups.templ`) with JS widget
  (`groups.js`) — collapsible group cards, member add/remove, inline rename. Entity
  permissions widget updated with "Group Permissions" section. `visibilityFilter()`
  SQL extended for group membership subquery. Settings page "Groups" link. 7 unit
  tests. Entity handler gains `GroupLister` interface for permissions API.
- **Batch 36**: Sprint K-2 Per-Entity Permissions UI — Permissions widget
  (`permissions.js`) with three visibility modes (Everyone/DM Only/Custom),
  per-role and per-user grant toggles (None/View/Edit), auto-save. Replaced
  `is_private` checkbox on entity edit form. API endpoints: GET/PUT
  `/entities/:eid/permissions` (Owner only). Multi-mode visibility indicators
  in entity cards (shield-halved for custom, lock for DM-only), category
  dashboard table/tree, show page title block + children list. Fixed sync API
  `GetEntity` to check custom visibility via `CheckEntityAccess`. Added
  `MemberLister` interface + wiring for campaign member picker. Export TODO for
  permissions data.
- **Batch 35**: Sprint K-1 Per-Entity Permissions Model — Migration 000048
  (`entity_permissions` table, `visibility` ENUM column on entities). Permission
  model types (VisibilityMode, SubjectType, Permission, EntityPermission,
  EffectivePermission, SetPermissionsInput, PermissionGrant). Full
  EntityPermissionRepository (ListByEntity, SetPermissions transactional,
  DeleteByEntity, GetEffectivePermission, UpdateVisibility). visibilityFilter()
  SQL helper handles both legacy is_private and custom permission modes in a
  single WHERE clause. Service: CheckEntityAccess, SetEntityPermissions,
  GetEntityPermissions. All entity list/search/count/children/backlinks queries
  updated with userID parameter across handlers, sync API, export adapters,
  layout injector, campaign dashboard. 13 new unit tests. Pure backend — no UI.
- **Batch 34**: Sprint J-4 File Security — ClamAV antivirus integration for upload
  scanning via clamd TCP protocol (INSTREAM). Fail-open when clamd unavailable.
  ClamAV container in docker-compose (clamav/clamav:1.4). CLAMAV_ADDRESS env var.
  3 unit tests. SVG blocked by MIME allowlist. CDR strips metadata/polyglots.
- **Batch 33**: Sprint J-3 Testing & Infrastructure — Verified HTMX edge cases (CSRF
  propagation, double-init prevention, widget cleanup, form tracking all covered by boot.js).
  Created `.air.toml` for hot reload config. Fixed docker-compose.yml em-dash in error
  message. Added `doc.go` for templates/components package. Verified all Go packages have
  package doc comments.
- **Batch 32**: Sprint J-2 Editor Enhancements — Code block syntax highlighting via
  @tiptap/extension-code-block-lowlight with highlight.js common languages (JS, Python,
  Go, SQL, etc.). Tokyo Night-inspired dark/light syntax theme in input.css. Find/replace
  bar (Ctrl+F find, Ctrl+H replace) with match navigation, replace, replace-all.
  TipTap bundle rebuilt with lowlight extension (~558KB).
- **Batch 31**: Sprint J-1 Breadcrumbs + Navigation — Shared breadcrumb component
  (`components/breadcrumbs.templ`). Added breadcrumbs to: maps list, map detail, timeline
  list, timeline detail, sessions list, session detail, calendar grid header, calendar
  timeline view, calendar week view. Recently Viewed Entities tracker
  (`recent_entities.js`) using localStorage, renders in sidebar drill panel.
- **Batch 30**: Sprint I-4 Map UX Polish — Leaflet.markercluster integration for both
  map widget and full map page (auto-clustering when >5 markers). Expanded POI icon
  picker from 18 to 39 icons organized in 8 groups (General, Settlements, Fortifications,
  Dungeons & Ruins, Nature, Maritime, Sacred & Magic, Resources). Custom cluster icon
  styling. CDN-loaded MarkerCluster CSS + JS.
- **Batch 29**: Sprint I-3 Calendar Week View — 7-column day grid with event cards.
  Repo: `ListEventsForDateRange`. Handler: `ShowWeek`. Template: `WeekPage/WeekFragment`.
  View toggle (Grid/Week/Timeline) added to all 3 calendar views. Navigation: prev/next/today.
  Cross-month and cross-year boundary handling. 5 unit tests. Route: GET /calendar/week.
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

### Sprint Q-1: Widget Extension API (COMPLETE)
- Added `WidgetContribution` type to `ManifestContributes` with slug, name, description, icon, file, config fields
- Added `WidgetConfigField` type for configurable data-* attributes (key, label, type, default, options)
- Validation in `validateContributes()`: slug, name, file required; file must be `.js`; path traversal blocked
- `.js` files added to `allowedFileExts` allowlist in security.go and asset serving handler
- Widget registration in `applier.go`: stores widget metadata in `extension_data` (namespace "widgets") with script URLs
- New handler: `ListWidgets` (GET /campaigns/:id/extensions/widgets) — returns all enabled extension widgets
- New handler: `GetWidgetScriptURLs` — used by layout injector to discover widget scripts
- Layout integration: `SetExtWidgetScripts`/`GetExtWidgetScripts` in data.go, `<script>` injection in base.templ
- Layout injector in app/routes.go wires extension widget script discovery per campaign
- Tests updated: security test accepts .js, manifest test covers widget validation (6 new cases)

### Sprint Q-2: Widget Extension Distribution (COMPLETE)
- `ext_widget` block type registered in block registry with generic renderer (`blockExtWidget` templ)
- `BlockMeta.WidgetSlug` field added for template editor to identify extension widgets
- `WidgetBlockLister` interface in entities handler, wired via `widgetBlockListerAdapter`
- `BlockTypesAPI` now appends extension widget blocks from enabled extensions
- `GetWidgetBlockInfos` method on extensions handler returns widget metadata
- Template editor JS updated: palette shows "Extension Widgets" section, drag data includes `widget_slug`, drop handlers set `config.widget_slug`
- Example extension: `dice-roller` with `widgets/dice-roller.js` — d4-d100 roller with history, nat1/natMax highlighting
- Example test updated to validate dice-roller manifest

## Next Session Should
**Sprint M-1 is complete.** Next priorities from `.ai/todo.md`:
- Sprint M-2: D&D 5e Module — Reference Pages (browsable pages at `/modules/dnd5e/`, category cards, searchable lists, stat block detail pages)
- Quick wins from the UX audit (export button, password change, sort controls, etc.)
- Phase S+ deferred items (Draw Steel module, whiteboards, offline mode)
- Test coverage gaps (handler/repository tests for maps, sessions, admin, smtp)

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`
- Tailwind standalone CLI (`tailwindcss`) is v3; do NOT use `npx @tailwindcss/cli` (v4 syntax)
- Fog-of-war sync is bidirectional (Chronicle ↔ Foundry) using dark-polygon heuristic with pixel↔percentage conversion.
- SimpleCalendar integration uses journal listeners with SC flag detection for CRUD hooks.

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
- **2026-03-05: Phase I Core UX** — Sprint I-2 (timeline connections + create-from-timeline),
  Sprint I-3 (calendar week view), Sprint I-4 (map marker clustering + expanded POI icons).
- **2026-03-05: Sprint J-1** — Breadcrumbs on all non-entity pages, recently viewed entities sidebar widget.
- **2026-03-05: Sprint J-2** — Code syntax highlighting (lowlight + highlight.js), find/replace bar (Ctrl+F/H).
- **2026-03-05: Sprint J-3** — HTMX verification, `.air.toml`, docker-compose fix, package doc comments.
- **2026-03-05: Sprint J-4** — ClamAV antivirus scanning, docker-compose ClamAV container.
- **2026-03-05: ALL PHASES COMPLETE** — H (release readiness), I (core UX), J (polish & infra).
- **2026-03-05: Sprint K-1** — Per-entity permissions model (backend): migration 000048, model types, permission repository, service methods, visibility filter, 13 tests.
- **2026-03-05: Sprint K-2** — Per-entity permissions UI: permissions widget (permissions.js), visibility modes, role/user grants, auto-save. Sync API GetEntity custom visibility fix.
- **2026-03-05: Sprint K-3** — Group-based visibility: migration 000049 (campaign_groups/members), GroupRepository, GroupService, group CRUD handlers, groups management page + widget, permissions widget group grants, 7 tests.
- **2026-03-05: Sprint K-4** — Auto-linking in editor: entity names API with Redis caching, auto-link JS module (text scanner, mention link creation), Insert menu + Ctrl+Shift+L shortcut.
- **2026-03-05: Sprint K-5** — Relations graph visualization: D3.js force-directed graph widget, graph API + standalone page, dashboard block type.
- **2026-03-05: Sprint K-6** — Foundry Polish: shop icon null fix (FA icons with entity colors), connection status UI (event-driven, click-to-reconnect, activity flash), SimpleCalendar CRUD hooks (journal listeners, SC flag detection), fog bidirectional sync (dark polygon heuristic, pixel↔percentage conversion). **Phase K complete.**
- **2026-03-05: Sprint L-1** — Entity posts (sub-notes): migration 000050, full widget (model/repo/service/handler), JS widget with collapsible cards, drag-to-reorder, visibility toggle, layout block type.
- **2026-03-05: Sprint L-2** — Notes rich text: TipTap mini editor instances replace plain textareas, legacy block→TipTap conversion, entry JSON + HTML saved to API.
- **2026-03-05: Sprint L-3** — Note folders: migration 000051 (parent_id + is_folder), tree view rendering, collapsible folders, move-to-folder, create folder, 4 tests.
- **2026-03-05: Sprint L-4** — Calendar event drag-and-drop: HTML5 DnD on monthly grid, draggable event chips, drop zone highlighting, full PUT on drop. Pure frontend.
- **2026-03-05: Sprint L-5** — Calendar day view: single-day detailed view, event cards with time/category/entity/description, day navigation, view toggle icon. Phase L complete.
