# Chronicle Backlog

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Single source of truth for what needs to be done, priorities,    -->
<!--          and what has been completed.                                     -->
<!-- Update: At the start of a session (to understand priorities), during      -->
<!--         work (to mark progress), and at session end (to reflect).        -->
<!-- Legend: [ ] Not started  [~] In progress  [x] Complete  [!] Blocked      -->
<!-- ====================================================================== -->

## 1. Bugfixes & Problems

Known broken or missing things, ordered by severity.

### Critical

- [x] **Public campaign widget 403s** — Editor and attributes widgets return 403 for non-member visitors on public campaigns. Root cause: GET `/entry`, `/fields`, `/tags`, `/relations` only registered in authenticated route group, not in public-capable group. Fixed by adding routes to `pub` group in entities/routes.go, tags/routes.go, relations/routes.go.

### High

- [x] **@mention popup won't dismiss** — Fixed by adding link mark guard in `onUpdate` (skips `@` inside existing mention links) and removing `selectionUpdate` event binding. Mentions still stored as Link marks, but popup no longer re-triggers.
- [x] **Image upload click does nothing** — Fixed event recursion: file input's click event bubbled back to parent widget, causing Firefox to suppress file picker. Added stopPropagation on file input click, campaign_id to upload FormData, and fixed hover feedback on placeholder.
- [x] **No media management for campaign owners** — Fixed: campaign-scoped media browser at `/campaigns/:id/media` (Owner-only). Grid view with thumbnails, "referenced by" tracking (queries entities by image_path and entry_html), delete with confirmation warnings, upload from browser page (Alpine.js), pagination, storage stats header. Sidebar "Media" link in Manage section.
- [x] **Sidebar drill 403 for public visitors** — Fixed: added `GET /campaigns/:id/sidebar/drill/:slug` to `pub` group in `campaigns/routes.go`. Public visitors can now click categories in sidebar without 403.
- [x] **Timeline eras not editable** — Fixed: added "Edit Eras" button on timeline detail page (links to `/calendar/settings?tab=eras`). Calendar settings now reads `?tab` query param to open correct tab. Added confirmation dialog to era delete button.
- [x] **Sessions addon not discoverable** — Fixed: added Sessions cross-link in calendar header (dice icon). When Sessions addon enabled, links to sessions page. When disabled, owners see dimmed icon linking to addon settings. Users on Calendar can now discover Sessions naturally.
- [x] **Calendar events lack view→edit mode** — Fixed: added read-only event detail modal (`eventViewModal`) showing name, date, time, category, visibility, description, and entity link. Scribes see "Edit" button that transitions to edit modal. All users can click event chips to view details.
- [x] **Calendar click-to-create on date** — Fixed: entire date cell is clickable for Scribes+ (cursor-pointer, hover highlight). Clicks on empty space open create modal with date pre-filled. Event chip clicks are properly intercepted.
- [x] **No unsaved changes warning** — Fixed: global dirty state tracker in boot.js (`Chronicle.markDirty/markClean/isDirty`) with `beforeunload` handler. Editor widget hooks in. Forms with `data-track-changes` auto-tracked (entity create/edit, campaign create/settings).
- [x] **Empty states inconsistent** — Fixed: added empty states to campaign members, admin campaigns, admin users, admin modules. Fixed entity_types.templ if/else structure. Calendar `UpcomingEventsEmpty()` was already good. Maps/timelines already have empty states.
- [x] **Calendar event categories not customizable** — Fixed: added `calendar_event_categories` table (migration 000039) with slug/name/icon/color per calendar. Default categories seeded on creation. Categories tab in calendar settings for full CRUD. Event modal dropdown and categoryIcon() now dynamic. JS view modal uses categories data attribute for display.

### Medium

- [x] **Tags not hideable from players** — Implemented `dm_only` column (migration 000038), role-based filtering in repo/service/handler, eye-slash badge + DM checkbox in tag_picker.js.
- [x] **Attributes missing "Use Template" reset** — Added DELETE `/field-overrides` endpoint and "Reset" button in attributes customize panel with confirmation dialog. Clears field_overrides to NULL, restoring category template defaults.
- [x] **Search scope limited to entities** — Fixed: Ctrl+K now searches entities, timelines, maps, calendar events, and sessions. Added MapSearcher, CalendarSearcher, SessionSearcher interfaces following the TimelineSearcher pattern. Each plugin implements Search repo method + formats results. Wired in routes.go.
- [x] **No confirmation dialogs for destructive actions** — Audited all delete operations. Most already had confirms (campaigns, entities, maps, markers, timelines, sessions, calendar events, sidebar nav, admin pages). Added confirms to notes.js and relations.js (the two missing ones). Dashboard editor row/block delete is safe (not persisted until explicit save).
- [x] **No loading/spinner states** — Fixed: added HTMX loading indicator (3px accent-colored progress bar at top of viewport). CSS animation in input.css, request tracking in boot.js, indicator div in app.templ layout. Tracks concurrent requests, only hides when all complete.
- [x] **Keyboard shortcuts help** — Fixed: press `?` to open shortcuts help overlay showing all 4 global shortcuts (Ctrl+K/N/E/S). Closes with `?`, Escape, or clicking outside. Mac-aware (shows ⌘ vs Ctrl).
- [x] **Form validation feedback** — Fixed: added `:user-invalid` and `.input-error` CSS for red borders on invalid fields. JS in boot.js listens for `invalid` events and inserts `.field-error` inline hints with the browser's validation message. Errors clear on input.
- [x] **Mobile sidebar toggle** — Already implemented: hamburger button in topbar (md:hidden), Alpine.js sidebarOpen state, CSS translate slide-in animation, backdrop overlay, auto-close on navigation.
- [x] **Calendar recurring events limited** — Sessions now support weekly/biweekly/monthly/custom recurrence. Calendar events still yearly-only (separate concern). Session recurrence via migration 000041.
- [x] **Editor lacks table support** — Added TipTap table extensions (Table, TableRow, TableCell, TableHeader), insert menu entry, CSS styles, rebuilt vendor bundle via esbuild.
- [x] **Editor lacks callout/highlight blocks** — Fixed: blockquote restyled as callout block with accent border, subtle background, info icon. Insert menu renamed "Blockquote" → "Callout Block". Read-only prose views also styled. TipTap bundle limits prevent custom node types; blockquote serves as callout.
- [x] **Entity cloning** — Fixed: Clone button on entity show page (Scribe+). POST creates copy with "(Copy)" suffix, clones entry, image, fields, field overrides, popup config, tags via INSERT...SELECT. Redirects to edit page. Does NOT copy relations.
- [x] **Map marker search** — Fixed: added search input in map header. Client-side filtering dims non-matching markers (opacity 0.15). Enter pans to first match and opens tooltip. Searches name and description.
- [x] **Timeline event creation from timeline page** — Already implemented: "Create Event" button in header opens modal with full form (name, date, description, category, visibility, color, multi-day, recurrence). POST to standalone-events API.

### Low

- [x] **API endpoints ignore addon disabled state** — RequireAddon middleware gates calendar, maps, sessions, timeline routes. RequireAddonAPI middleware gates API v1 routes (syncapi). Fail-closed on DB errors.
- [x] **API technical documentation missing** — Created OpenAPI 3.0.3 spec at `docs/api/openapi.yaml` with 63 endpoints, 42 schemas, auth details, and error responses.
- [x] **Calendar HTMX detection inconsistency** — Replaced 5 raw `HX-Request` header checks in calendar handler with `middleware.IsHTMX(c)`, which also checks `HX-Boosted` to avoid returning fragments on boosted navigation.
- [x] **Cross-plugin adapter interface duplication** — Extracted `campaigns.MemberLister` interface. Timeline and sessions handlers now import it instead of defining local copies.
- [x] **IDOR check functions duplicated** — Extracted generic `middleware.RequireInCampaign[T]()` helper with Go generics. Updated maps, timeline, sessions handlers. Calendar/markers left as-is (parent traversal needed).
- [x] **logAudit fire-and-forget duplicated** — Assessed: three plugins use different logAudit signatures (entities: entityID+name, campaigns: details map, tags: tagName). Not worth abstracting — left as-is.
- [x] **JS fetch header setup duplicated** — Added `Chronicle.apiFetch()` wrapper to boot.js (auto-sets headers, CSRF, JSON serialization). Migrated sidebar_config, entity_type_config, sidebar_nav_editor, dashboard_editor widgets. Simplified notes.js CSRF reading.
- [x] **Mixed error types** — Replaced all 249 `echo.NewHTTPError` calls with `apperror` domain errors across 15 handler files + middleware + websocket. Zero remaining.
- [x] **LIKE metacharacter in backlinks** — Added `strings.NewReplacer` to escape `%` and `_` in entityID before LIKE pattern in `entities/repository.go:FindBacklinks`.
- [x] **No Content Security Policy headers** — CSP implemented in `middleware/security.go` (default-src 'self', script-src, style-src, img-src, font-src, connect-src, frame-ancestors, base-uri, form-action). Alpine.js requires 'unsafe-inline'/'unsafe-eval'; documented tradeoff.
- [x] **No input size validation on text fields** — Added `apperror/validate.go` helpers (ValidateStringLength, ValidateRequired) and wired into entities, campaigns, maps, timeline, sessions create handlers.
- [x] **Package-level Go doc comments** — All Go packages have `// Package ...` comments. Added `doc.go` for `templates/components`. Widget "packages" are JS-only (`.ai.md` docs).
- [x] **Missing JS widget .ai.md docs** — All done: image_upload, timeline_viz, dashboard_editor, template_editor, entity_tooltip, foundry-module, websocket, attributes, mentions, title, boot.js, editor, tags. Relations and notes already existed.

---

## 2. Features To Do

New capabilities ordered by priority for alpha release.

### Alpha-Critical (Must Have)

- [x] **Media management for owners** — Campaign-scoped media browser at `/campaigns/:id/media` (Owner-only): grid view with thumbnails, "referenced by" entity queries, delete with warnings, upload from browser, pagination, storage stats. Admin already had `/admin/storage`.
- [x] **Tag visibility controls** — Implemented: migration 000038, `dm_only` bool in model/repo/service/handler, role-based filtering, tag_picker.js DM-only badge + create checkbox.
- [x] **Attributes template reset** — Implemented DELETE endpoint + "Reset" button in customize panel with confirmation dialog.
- [x] **Extension technical documentation** — All `.ai.md` writeups complete: foundry-module, websocket, media, image_upload, timeline_viz, dashboard_editor, template_editor, entity_tooltip, syncapi, maps, editor, tags, attributes, mentions, title, boot.js, relations, notes.
- [x] **Graceful extension degradation** — `RequireAddon` middleware (web) and `RequireAddonAPI` middleware (API) gate routes. Human-readable errors for disabled addons. Fail-closed on DB errors.
- [x] **Permissions & UX completeness audit** — Completed 2026-03-04. Audited all 17 route files, 24 JS widgets, all templ templates. Found 10 MUST-haves, 15 NEED-to-haves, 20 WANTs, 15 MAYBEs. Key findings: sidebar drill public access, sessions discoverability, calendar UX gaps, missing editor features (tables, callouts), no unsaved changes warning, inconsistent empty states. All items added to Bugfixes section above.
- [x] **README.md** — Full open-source README with features, setup instructions, tech stack, architecture, project structure, screenshots placeholders, inspiration credits. Created 2026-03-04.

### Alpha-Nice-to-Have

- [x] **File security audit + ClamAV** — ClamAV integration via clamd TCP protocol (INSTREAM). Fail-open when unavailable. Docker-compose ClamAV container (clamav/clamav:1.4). CLAMAV_ADDRESS config. 3 tests. SVG already blocked by MIME allowlist. CDR pipeline strips metadata/polyglots.
- [x] **API documentation** — OpenAPI 3.0.3 spec at `docs/api/openapi.yaml`. 63 endpoints, 42 schemas, auth guide, rate limiting headers, sync protocol.
- [x] **Foundry VTT Sync** — Bidirectional sync between Chronicle and Foundry VTT. Phases 1-4 complete (WebSocket, sync mappings, journal sync, map expansion, EventBus, Map API v1, calendar live sync). Phase 5 (shop entity type + Chronicle inventory widget + relation metadata, Foundry shop widget wiring, RequireAddonAPI permission hardening, E2E testing checklist) complete.
- [x] **Maps Phase 2** — Layers, drawings, tokens, fog of war. Migration 000042, full CRUD service + repository + REST API handler. Role-based visibility filtering. Percentage-based coordinates for resolution independence.
- [x] **Timeline Phase 2B** — Event connections (migration 000047, SVG lines/arrows, 4 styles), create-from-timeline (double-click opens modal with date), visual polish (hover effects, ruler labels, connection CSS). 3 tests.
- [x] **Campaign export/import** — JSON bundle for backup/migration. Export/import service with adapter pattern for 7 plugins (entities, calendar, timeline, sessions, maps, addons, media). Slug-based cross-references, ID remapping on import. 6 tests.
- [x] **Image drag-and-drop upload** — Media browser has drag-and-drop + multi-file upload with per-file progress bars (Alpine.js + XHR). Entity image widget (`image_upload.js`) still click-only.
- [x] **Calendar week view** — 7-column day grid with event cards, cross-month handling, prev/next/today navigation. View toggle added to all calendar views. 5 tests.
- [ ] **Calendar event drag-and-drop** — Can't drag events between dates (standard Google Calendar UX).
- [ ] **Calendar day view** — No single-day detailed view with time blocks.
- [x] **Map marker clustering** — Leaflet.markercluster integration on both map widget and full map page. Auto-clustering when >5 markers with custom cluster icons. CDN-loaded.
- [x] **Map marker icon picker** — Expanded from 18 to 39 POI icons in 8 organized groups (General, Settlements, Fortifications, Dungeons & Ruins, Nature, Maritime, Sacred & Magic, Resources).
- [x] **Recent entities sidebar** — localStorage-backed "recently viewed" list in sidebar drill panel. Tracks entity visits, renders last 10 with clock icons. `recent_entities.js`.
- [ ] **Command palette (Ctrl+Shift+P)** — Quick action palette beyond Ctrl+K search.
- [x] **Breadcrumb consistency** — Shared breadcrumb component (`components/breadcrumbs.templ`). Added to maps list/detail, timeline list/detail, sessions list/detail, calendar grid/timeline/week views.
- [ ] **Timeline search/filter** — No search within timeline events by name/text.
- [ ] **Timeline zoom-to-era** — No button to jump viewport to a specific era.
- [x] **Editor find/replace** — Ctrl+F opens find bar, Ctrl+H opens find+replace. Match navigation, replace, replace-all. Selection-based highlighting.
- [x] **Editor code syntax highlighting** — @tiptap/extension-code-block-lowlight with highlight.js common languages. Tokyo Night-inspired dark/light theme in input.css.
- [ ] **Entity version history UI** — Audit log exists but no "view diff / restore version" for entities.
- [ ] **Notes search/filter** — No search within notes panel.
- [ ] **Toast notification grouping** — Duplicate toasts stack separately instead of grouping.
- [ ] **Entity image gallery** — Only one image per entity; no carousel/gallery for multiple images.

### Phase K: Permissions & Competitive Gap Closers

- [x] **Sprint K-1: Per-Entity Permissions Model** — Migration 000048: `entity_permissions` table + `visibility` column on entities. Models: VisibilityMode, SubjectType, Permission, EntityPermission, EffectivePermission, SetPermissionsInput, PermissionGrant. Repository: EntityPermissionRepository (ListByEntity, SetPermissions, DeleteByEntity, GetEffectivePermission, UpdateVisibility) + visibilityFilter() SQL helper. Service: CheckEntityAccess, SetEntityPermissions, GetEntityPermissions. All list/search/count queries updated with userID param for permission-aware filtering. 13 new unit tests.
- [ ] **Sprint K-2: Per-Entity Permissions UI** — "Permissions" tab on entity edit page. Visibility selector (everyone/dm_only/custom). User/role picker with view/edit toggles. Entity list + sidebar filter by resolved permissions.
- [ ] **Sprint K-3: Group-Based Visibility** — Migration: `campaign_groups` + `campaign_group_members`. Permission subject_type gains "group." Groups in Campaign Settings. Entity permission UI gets group selector.
- [ ] **Sprint K-4: Auto-Linking in Editor** — Backend: entity-names API (Redis-cached). Frontend: TipTap InputRule matches entity names, inline suggestion popup. Per-campaign toggle. Whole-word, case-insensitive, min 3 chars.
- [ ] **Sprint K-5: Relations Graph Visualization** — D3.js force-directed graph (`relation_graph.js`). Backend: relation-graph API (nodes/edges JSON). Dashboard block + standalone page `/campaigns/:id/relations`.

### Phase L: Content Depth & Editor Power

- [ ] **Sprint L-1: Entity Sub-Notes (Posts) UI** — `entity_posts` table exists but no UI. Full CRUD: list below entry, create/edit modal with TipTap, per-post visibility, drag-to-reorder.
- [ ] **Sprint L-2: Notes Rich Text (TipTap)** — Replace plain-text blocks with TipTap in notes widget. `entry`/`entry_html` columns exist. Support bold, italic, lists, links, @mentions. Block→TipTap migration on first edit.
- [ ] **Sprint L-3: Note Folders and Organization** — Migration: `parent_id` + `folder` boolean. Tree view in notes panel, expand/collapse, drag-to-reorder within/between folders.
- [ ] **Sprint L-4: Calendar Event Drag-and-Drop** — HTML5 DnD on monthly grid. Events draggable, date cells as drop targets. HTMX PUT on drop. Ghost element + drop zone highlighting. Scribe+ only.
- [ ] **Sprint L-5: Calendar Day View + Recurring Events** — Single-day view with hourly time blocks. Expand event recurrence to monthly/weekly/daily/custom matching session recurrence.

### Phase M: Game System Modules & Worldbuilding Tools

- [ ] **Sprint M-1: D&D 5e Module — Data & Tooltip API** — SRD-legal JSON (spells, monsters, items, conditions, classes, races). Tooltip endpoint. Wire into entity_tooltip widget. Register as addon.
- [ ] **Sprint M-2: D&D 5e Module — Reference Pages** — Browsable pages at `/modules/dnd5e/`. Category cards, searchable lists, formatted stat block detail pages. Quick-search integration.
- [ ] **Sprint M-3: Pathfinder 2e Module** — ORC-licensed data following D&D 5e pattern. Spells, monsters, ancestries, classes, conditions, feats.
- [ ] **Sprint M-4: Guided Worldbuilding Prompts** — `worldbuilding_prompts` table. "Writing Prompts" collapsible panel on entity edit page. Default prompt packs per entity type. Owner-customizable.
- [ ] **Sprint M-5: Entity Type Template Library** — Genre presets (fantasy, sci-fi, horror, modern, historical) as JSON fixtures. Campaign creation genre selection. "Import preset" in Customization Hub.

### Phase N: Collaboration & Platform Maturity

- [ ] **Sprint N-1: Role-Aware Dashboards** — Role-keyed dashboard layouts. Dashboard editor gains role selector. Players see role-specific dashboard or default fallback.
- [ ] **Sprint N-2: Invite System** — Migration: `campaign_invites` table. Email invitations with one-click accept link. Non-public campaigns require invitation. Invite management UI.
- [ ] **Sprint N-3: 2FA/TOTP Support** — TOTP enrollment with QR code (`pquerna/otp`). Login redirect to TOTP input. Recovery codes (8 hashed). Admin force-disable.
- [ ] **Sprint N-4: Accessibility Audit (WCAG 2.1 AA)** — ARIA labels, focus traps, skip-to-content, color contrast 4.5:1, keyboard nav, screen reader announcements, axe-core scanning.
- [ ] **Sprint N-5: Infrastructure & Deployment** — Docker-compose full stack verification with health checks. Makefile full-stack target. `CONTRIBUTING.md`. CI against docker-compose.

### Phase O: Polish, Ecosystem & Delight

- [ ] **Sprint O-1: Command Palette & Saved Filters** — Ctrl+Shift+P action palette with fuzzy search. Saved entity list filter presets as sidebar links in `saved_filters` table.
- [ ] **Sprint O-2: Map Drawing Tools** — Leaflet.Draw integration (freehand, polygons, circles, rectangles, text). Uses existing `map_drawings` table. Per-drawing visibility, color/opacity.
- [ ] **Sprint O-3: Discord Bot Integration** — Plugin at `internal/plugins/discord/`. Bot token config. Webhook session notifications. Reaction-based RSVP per ADR-012.
- [ ] **Sprint O-4: Bulk Operations & Persistent Filters** — Multi-select entity lists with batch actions (tag, move, visibility, delete). Persistent filters per category in localStorage.
- [ ] **Sprint O-5: Editor Import/Export & Additional Themes** — Markdown import/export via `goldmark`. Sepia + high-contrast themes. Custom accent color picker.

### Deferred to Phase P+ (or community contributions)

- [ ] Draw Steel module
- [ ] Whiteboards / freeform canvas (Tldraw/Excalidraw)
- [ ] Offline mode / service worker caching
- [ ] Collaborative editing presence indicators
- [ ] Calendar timezone support / print-PDF export
- [ ] Map hex/square grid overlay / measurement tool
- [~] Fog of war Foundry→Chronicle push (Chronicle→Foundry done)
- [ ] Webhook support for external event notifications
- [ ] Widget inline CSS → CSS classes migration
- [ ] Reusable modal/dropdown component library
- [ ] Toast notification grouping
- [ ] Entity image gallery (multi-image carousel)
- [ ] Entity version history UI (view diff / restore)
- [ ] Timeline search/filter + zoom-to-era
- [ ] Notes search/filter
- [ ] Dice roller widget

---

## 3. Competitive Analysis

Summary of strengths/weaknesses for strategic positioning. Full analysis in `.ai/roadmap.md`.

| Platform | Users | Key Strengths | Key Weaknesses | What Chronicle Should Learn |
|----------|-------|--------------|----------------|----------------------------|
| **WorldAnvil** | ~1.5M | 25+ templates, guided prompts, inline secrets, Chronicles (map+timeline combo), 45+ RPG systems, family trees | BBCode editor, steep learning curve, cluttered UI, aggressive paywall, privacy requires paid | Guided prompts, deep secrets system, RPG system breadth |
| **Kanka** | ~300K | Structured 20-type entities, generous free tier, deep per-role/user permissions, best calendar (-2B to +2B years), GPL source, REST API, marketplace | Summernote editor, complex permission UI, self-hosted deprioritized | Permission granularity, calendar depth, marketplace concept |
| **LegendKeeper** | Small | Best WebGL maps (regions, navigation), real-time co-editing, auto-linking, offline-first, clean UI, speed as brand | Limited entity types, no formal relations, minimal game systems | Auto-linking magic, speed obsession, map interaction depth |
| **Obsidian** | ~4M+ | Local-first markdown, 1000+ plugins, graph view, backlinks, community themes, offline, privacy by default, canvas/whiteboard | Not TTRPG-specific, no calendar/maps/timeline natively (requires plugin cobbling), single-user (no campaign sharing), no web UI | Plugin ecosystem model, graph visualization, local-first philosophy, community extensibility |

### Where Chronicle Already Wins

1. **Drag-and-drop page layout editor** — nobody else has visual page design
2. **Customizable dashboards** (campaign + per-category) — most flexible dashboard system
3. **Self-hosted as primary target** — no paywall, no forced public content
4. **Modern tech stack** — TipTap + HTMX + Templ vs BBCode/Summernote
5. **Per-entity field overrides** — unique; entities customize their own schema
6. **REST API from day one** — matches Kanka, beats WorldAnvil and LegendKeeper
7. **Extension framework** — per-campaign addon toggle
8. **Audit logging** — no competitor has this
9. **Interactive D3 timeline** with eras, clustering, minimap — exceeds Kanka, matches WorldAnvil

### Chronicle vs Obsidian

- Obsidian users cobble TTRPG workflows from community plugins (Fantasy Calendar, Leaflet, TTRPG plugin). Chronicle offers purpose-built calendar/maps/timelines/entity types as first-class features.
- Chronicle has multi-user campaign sharing built-in; Obsidian is single-user.
- Obsidian's plugin ecosystem (1000+) is aspirational — Chronicle's addon system is the foundation for similar extensibility.

---

## Completed Sprints

### Phase 0: Project Scaffolding (2026-02-19)
- [x] AI documentation system (`.ai/` directory, 13 files)
- [x] `CLAUDE.md` root context file
- [x] Project directory skeleton (plugins, modules, widgets)
- [x] Plugin/module/widget `.ai.md` files
- [x] Build configuration (Makefile, Dockerfile, docker-compose)
- [x] `.gitignore`, `.env.example`, `tailwind.config.js`
- [x] Coding conventions and 8 architecture decisions (ADRs 001-008)

### Phase 1: Foundation (2026-02-19)
- [x] Core infrastructure (config, database, middleware, app, server)
- [x] Auth plugin (register, login, logout, session management, argon2id)
- [x] Campaigns plugin (CRUD, roles, membership, ownership transfer)
- [x] SMTP plugin (AES-256-GCM encrypted password, STARTTLS/SSL, test)
- [x] Admin plugin (dashboard, user management, campaign oversight)
- [x] Entities plugin (CRUD, entity types, FULLTEXT search, privacy, dynamic fields)
- [x] Editor widget (TipTap, boot.js auto-mounter, entry API)
- [x] UI & Layouts (sidebar, topbar, pagination, flash messages, error pages)
- [x] Vendor HTMX + Alpine.js, campaign selector dropdown
- [x] CSS component library, landing page
- [x] Entity service unit tests (30 tests)
- [x] Dockerfile (multi-stage, Go 1.24, pinned Tailwind)
- [x] CI/CD pipeline (GitHub Actions)
- [x] Production deployment hardening
- [x] Auto-migrations on startup, first-user-is-admin, /health alias

### Phase 2: Media & UI (2026-02-19 to 2026-02-20)
- [x] Media plugin, Audit plugin, Settings plugin, Admin modules page
- [x] Editor view/edit, @mention, Attributes, Tag picker, Relations, Template editor, Entity tooltip widgets
- [x] Entity type CRUD, list redesign, image upload, sidebar customization, layout-driven profiles
- [x] Security audit (14 fixes), IDOR protection, HSTS, rate limiting, storage limits
- [x] Dark mode, semantic color system, toast notifications, public campaign support

### Phase 3: Competitor-Inspired UI Overhaul (2026-02-20)
- [x] Terminology rename (Entity→Page, Entity Type→Category)
- [x] Drill-down sidebar, category dashboards, tighter card spacing

### Phase B: Extensions & API (2026-02-20)
- [x] Discover page, template editor resizing, block visibility, field overrides
- [x] Extension framework (addons plugin), Sync API plugin, API key management

### Phase C: Notes & Terminology (2026-02-20)
- [x] Player Notes widget, terminology standardization, admin cleanup

### Phase D: Campaign Customization Hub (2026-02-22 to 2026-02-24)
- [x] Sprint 1-1.5: Customization Hub (4 tabs, sidebar config, custom nav)
- [x] Sprint 2: Dashboard Editor (drag-and-drop, 6 block types)
- [x] Sprint 3: Category Dashboards (per-category layout editor)
- [x] Sprint 3.5: Page Layouts Tab (HTMX lazy-loaded template-editor)
- [x] Sprint 4: Player Notes Overhaul (locking, rich text, versions, shared)
- [x] Sprint 5: Polish (hx-boost, "View as player", widget lifecycle)

### Phase E: Core UX & Discovery (2026-02-24 to 2026-02-25)
- [x] Sprint 1: Quick Search (Ctrl+K)
- [x] Sprint 2: Customization Hub Rework (consolidated tabs)
- [x] Sprint 3: Extension Enable Bug Fix (installed addons registry)
- [x] Sprint 4-7: Entity Hierarchy (parent/child, tree view, breadcrumbs)
- [x] Sprint 8: Editor Insert Menu + Backlinks
- [x] Sprint 9: Entity Preview Tooltip + Popup Config
- [x] Sprint 10: Keyboard Shortcuts (Ctrl+N, Ctrl+E, Ctrl+S)

### Phase F: Calendar & Time (2026-02-25 to 2026-02-28)
- [x] Sprint 1: Calendar Plugin (model, repo, service, handler, monthly grid)
- [x] Sprint 2: Calendar Feature Parity + Sync API (leap years, seasons, event categories, multi-day, device fingerprint)
- [x] Sprints 3-9: Calendar settings, event modal, sidebar/dashboard, timeline view, event edit/delete, calendar import/export, eras

### Phase G: Maps & Geography + Timeline (2026-02-28 to 2026-03-03)
- [x] Maps Phase 1 (Leaflet.js, image upload, pins, entity linking, DM-only)
- [x] Timeline standalone events (calendar-free timelines)
- [x] Timeline visualization Phase 1 (D3 overhaul: SVG, zoom, detail panel)
- [x] Timeline visualization Phase 2A (ruler, grid, era bars, range bars, clustering, minimap)
- [x] FOUC prevention (SVG dimension fallbacks, inline bg-color, body opacity guard)
- [x] SVG favicon

### Alpha Documentation Sprint (2026-03-03)
- [x] Public campaign widget 403 fix (entities, tags, relations route groups)
- [x] Todo.md restructure (3 categories: Bugs, Features, Competitive Analysis)
- [x] Roadmap.md Obsidian competitive analysis
- [x] Extension documentation sprint (media plugin, 5 JS widgets: image_upload, timeline_viz, dashboard_editor, template_editor, entity_tooltip)
- [x] Status.md update

### Code Quality Sprint (2026-03-03)
- [x] golangci-lint v2 config fixes (.golangci.yml: version field, removed typecheck/gosimple)
- [x] Fixed all 138 golangci-lint issues (errcheck, staticcheck S1016, unused dead code)
- [x] Consolidated JS utility duplication: escapeHtml (9 copies), escapeAttr (7 copies), getCsrf (3 copies) → shared Chronicle.* in boot.js
- [x] Syncapi repository errcheck fixes (Row.Scan error handling, json.Unmarshal acknowledgement)

### Bug Fixes & Testing Sprint (2026-03-04)
- [x] Image upload click fix (event recursion prevention, campaign_id in FormData, hover feedback)
- [x] Chronicle.apiFetch() shared wrapper in boot.js (auto-headers, CSRF, JSON serialization)
- [x] Migrated 4 widgets to Chronicle.apiFetch() (sidebar_config, entity_type_config, sidebar_nav_editor, dashboard_editor)
- [x] Calendar HTMX detection fix (5 raw header checks → middleware.IsHTMX())
- [x] Relations service tests (25 tests)
- [x] Tags service tests (40 tests)

### Production Fix + Mobile Nav + Widgets + Foundry Completion (2026-03-04, batch 20)
- [x] Fixed duplicate migration 000041 (renumbered sync_mappings→044, map_expansion→045, relation_metadata→046)
- [x] Removed Calendar/Maps/Timelines addon sidebar links from mobile nav
- [x] Added map_preview dashboard block type with Leaflet-based widget
- [x] Created 3 interactive dashboard widgets (calendar, timeline, map) with boot.js auto-mount
- [x] Mobile responsive dashboard/category/entity grids (1-col mobile, 12-col desktop)
- [x] Relations API endpoint for Foundry shop inventory (GET /entities/:entityID/relations)
- [x] Foundry shop widget wired to relations API with inventory filtering
- [x] RequireAddonAPI middleware gating calendar and map API v1 routes
- [x] Foundry VTT E2E testing checklist (TESTING.md)

### Calendar Sessions + Entity Widgets + Foundry Security (2026-03-04, batches 21-24)
- [x] Calendar sessions modal overlay with inline RSVP controls
- [x] Sessions fragment endpoint (GET /calendar/sessions-fragment)
- [x] Recurring session auto-generation on completion (server-side)
- [x] Entity page widget blocks (timeline, map_preview, upcoming_events, shop_inventory, text_block)
- [x] WebSocket security (origin validation, message type validation, campaign-scoped)
- [x] Device fingerprint binding race condition fix (async→synchronous)
- [x] Sync action input sanitization (removed user input echo from errors)
- [x] Rate limit bounds validation (clamped to 1-10000)
- [x] RequireAddonAPI fail-closed on DB errors (was fail-open)
- [x] Fog-of-war Chronicle→Foundry sync (polygon overlay drawings on Foundry scene)
- [x] Extension .ai.md documentation (foundry-module, websocket)

### Alpha Hardening Batch (2026-03-04, batch 25)
- [x] CI pipeline: golangci-lint job + govulncheck security scan job in GitHub Actions
- [x] Service tests: audit (12), media (20+), settings (30+), IDOR middleware (3)
- [x] Generic IDOR helper: `middleware.RequireInCampaign[T]()` with Go generics
- [x] Input validation: `apperror/validate.go` helpers, wired into 5 create handlers
- [x] Widget documentation: attributes, mentions, title `.ai.md` + boot.js `.ai.md`
- [x] TipTap table support: Table/TableRow/TableCell/TableHeader extensions, esbuild pipeline, CSS styles

### Phase H: Release Readiness (2026-03-04, batches 26-27)
- [x] Error type standardization: replaced all 249 `echo.NewHTTPError` calls with `apperror` domain errors across 15 handler files
- [x] Code dedup: MemberLister interface extraction, LIKE metacharacter escaping
- [x] API documentation: OpenAPI 3.0.3 spec at `docs/api/openapi.yaml` (63 endpoints, 42 schemas)
- [x] Extension `.ai.md` docs: syncapi, maps, editor, tags (all widget/plugin docs complete)

### Phase I Sprint 1: Campaign Export/Import (2026-03-04, batch 27)
- [x] Export model (`campaigns/export.go`): 20+ types covering all campaign data sections
- [x] Import model (`campaigns/import.go`): format detection, ID mapping structure
- [x] Export/import service (`campaigns/export_service.go`): adapter interfaces for 7 plugins
- [x] HTTP handler (`campaigns/export_handler.go`): GET export download, POST import upload
- [x] Export adapters (`app/export_adapters.go`): adapter implementations for all plugins
- [x] Route wiring in `campaigns/routes.go` and `app/routes.go`
- [x] Tests: 6 unit tests for import detection and ID mapping

### Phase I Sprint 2: Timeline Phase 2B (2026-03-05, batch 28)
- [x] Migration 000047: `timeline_event_connections` table
- [x] Model: `EventConnection` struct, `CreateConnectionInput`, connection style validation
- [x] Repository: `CreateConnection`, `DeleteConnection`, `ListConnections` with IDOR protection
- [x] Service: connection CRUD with validation (source/target type, self-connect, color, style)
- [x] Handler: `CreateConnectionAPI`, `DeleteConnectionAPI`, `ListConnectionsAPI`
- [x] Routes: GET/POST/DELETE `/timelines/:tid/connections` (Scribe+)
- [x] D3 visualization: SVG arrowhead marker, quadratic Bézier curves, 4 line styles, labels
- [x] Create-from-timeline: double-click empty space opens modal with date pre-filled
- [x] Visual polish: connection line CSS (hover), event marker hover effects, ruler labels
- [x] Tests: 3 unit tests for connection style validation and model fields

### Phase I Sprint 3: Calendar Week View (2026-03-05, batch 29)
- [x] Repository: `ListEventsForDateRange` with composite date value SQL
- [x] Service: `ListEventsForDateRange` with per-user visibility filtering
- [x] Handler: `ShowWeek` with week-start snapping, cross-month event fetching
- [x] Model: `WeekViewData`, `WeekDay` structs with helper methods (WeekDays, PrevWeek, NextWeek, EndDate)
- [x] Template: `WeekPage`, `WeekFragment`, `weekContent` with 7-column day grid
- [x] View toggle: Grid/Week/Timeline button group added to all 3 calendar views
- [x] Route: GET `/calendar/week` (public-capable)
- [x] Tests: 5 unit tests for week data helpers (WeekDays, CrossMonth, PrevNext, WeekdayName)
