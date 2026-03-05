# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-03-05 -- Sprint L-5 (Calendar Day View) complete (batch 44).
Branch: `claude/project-review-planning-Yr4CL`.

## Current Phase
**Phase L: Content Depth & Editor Power.** Phase L complete (batch 44). Next: Phase M (Game System Modules & Worldbuilding Tools).

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

## Next Session Should
Continue with **Phase M** (Game System Modules & Worldbuilding Tools). Sprint M-1: D&D 5e Module with SRD data, tooltip API, reference pages. Full post-alpha roadmap (Phases M through O, 15 remaining sprints) documented in `.ai/todo.md`.

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
