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
- [ ] **Calendar recurring events limited** — Only "yearly" recurrence type. No monthly/weekly/daily/custom recurrence patterns.
- [ ] **Editor lacks table support** — TipTap editor has no table insert/edit (common need for TTRPG stat blocks, encounter tables).
- [x] **Editor lacks callout/highlight blocks** — Fixed: blockquote restyled as callout block with accent border, subtle background, info icon. Insert menu renamed "Blockquote" → "Callout Block". Read-only prose views also styled. TipTap bundle limits prevent custom node types; blockquote serves as callout.
- [x] **Entity cloning** — Fixed: Clone button on entity show page (Scribe+). POST creates copy with "(Copy)" suffix, clones entry, image, fields, field overrides, popup config, tags via INSERT...SELECT. Redirects to edit page. Does NOT copy relations.
- [x] **Map marker search** — Fixed: added search input in map header. Client-side filtering dims non-matching markers (opacity 0.15). Enter pans to first match and opens tooltip. Searches name and description.
- [x] **Timeline event creation from timeline page** — Already implemented: "Create Event" button in header opens modal with full form (name, date, description, category, visibility, color, multi-day, recurrence). POST to standalone-events API.

### Low

- [ ] **API endpoints ignore addon disabled state** — Routes are hardcoded at startup. If calendar addon is disabled for a campaign, `/api/v1/campaigns/:id/calendar` still executes. Need `RequireAddon` middleware on API route groups.
- [ ] **API technical documentation missing** — REST API v1 exists and works but has no public documentation (OpenAPI spec or reference).
- [x] **Calendar HTMX detection inconsistency** — Replaced 5 raw `HX-Request` header checks in calendar handler with `middleware.IsHTMX(c)`, which also checks `HX-Boosted` to avoid returning fragments on boosted navigation.
- [ ] **Cross-plugin adapter interface duplication** — `MemberLister` interface duplicated in timeline and sessions plugins. Should extract to shared package.
- [ ] **IDOR check functions duplicated** — `requireTimelineInCampaign`, `requireMapInCampaign`, `requireSessionInCampaign`, `requireEventInCampaign` follow identical patterns in 4 plugins. Extract to shared generic helper.
- [ ] **logAudit fire-and-forget duplicated** — Similar audit logging patterns in entities, campaigns, tags handlers. Could extract to shared `FireAudit()` utility.
- [x] **JS fetch header setup duplicated** — Added `Chronicle.apiFetch()` wrapper to boot.js (auto-sets headers, CSRF, JSON serialization). Migrated sidebar_config, entity_type_config, sidebar_nav_editor, dashboard_editor widgets. Simplified notes.js CSRF reading.
- [ ] **Mixed error types** — `echo.NewHTTPError` used directly in 30+ places instead of centralized `apperror` package. Should standardize.
- [ ] **LIKE metacharacter in backlinks** — `entities/repository.go:1011` concatenates entityID into LIKE pattern without escaping `%`/`_`. Low risk (UUIDs only) but should escape for safety.
- [ ] **No Content Security Policy headers** — CSP not implemented. Would provide XSS defense-in-depth alongside bluemonday sanitization.
- [ ] **No input size validation on text fields** — Relies on DB column limits. Handler-level validation (name max 200, description max 5000, etc.) would be better.
- [ ] **Package-level Go doc comments missing** — ~80% of .go files lack `// Package ...` comments (handler.go, service.go, repository.go, routes.go across all plugins).
- [ ] **Missing JS widget .ai.md docs** — 11 widget files + 6 core JS files lack documentation. Priority: editor.js, attributes.js, tag_picker.js, relations.js, notes.js.

---

## 2. Features To Do

New capabilities ordered by priority for alpha release.

### Alpha-Critical (Must Have)

- [x] **Media management for owners** — Campaign-scoped media browser at `/campaigns/:id/media` (Owner-only): grid view with thumbnails, "referenced by" entity queries, delete with warnings, upload from browser, pagination, storage stats. Admin already had `/admin/storage`.
- [x] **Tag visibility controls** — Implemented: migration 000038, `dm_only` bool in model/repo/service/handler, role-based filtering, tag_picker.js DM-only badge + create checkbox.
- [x] **Attributes template reset** — Implemented DELETE endpoint + "Reset" button in customize panel with confirmation dialog.
- [ ] **Extension technical documentation** — 1-3 page `.ai.md` writeup per plugin/widget/module. Standard template covering purpose, architecture, API endpoints, widget integration, lifecycle, security. See documentation audit in plan.
- [ ] **Graceful extension degradation** — `RequireAddon` API middleware, human-readable errors for disabled/uninstalled addons, addon dependency checking.
- [x] **Permissions & UX completeness audit** — Completed 2026-03-04. Audited all 17 route files, 24 JS widgets, all templ templates. Found 10 MUST-haves, 15 NEED-to-haves, 20 WANTs, 15 MAYBEs. Key findings: sidebar drill public access, sessions discoverability, calendar UX gaps, missing editor features (tables, callouts), no unsaved changes warning, inconsistent empty states. All items added to Bugfixes section above.
- [x] **README.md** — Full open-source README with features, setup instructions, tech stack, architecture, project structure, screenshots placeholders, inspiration credits. Created 2026-03-04.

### Alpha-Nice-to-Have

- [ ] **File security audit + ClamAV** — Add ClamAV container to docker-compose, scan uploads before storage, configurable file type allowlist, SVG blocking (XSS vector).
- [ ] **API documentation** — OpenAPI 3.0 spec or handwritten reference for REST v1. Auth guide, endpoint reference, rate limiting docs, sync protocol.
- [~] **Foundry VTT Sync** — Bidirectional sync between Chronicle and Foundry VTT. Phase 1 (WebSocket hub, sync mappings, Foundry module skeleton, journal sync), Phase 2 (map expansion: drawings, tokens, layers, fog CRUD + internal API), Phase 3 (EventBus wiring + Map REST API v1, 23 endpoints), and Phase 4 (calendar live sync with Calendaria + SimpleCalendar adapter, PUT /calendar/date endpoint, event CRUD, initial sync) complete. Remaining: shop entity type, Foundry module end-to-end testing, permission hardening.
- [x] **Maps Phase 2** — Layers, drawings, tokens, fog of war. Migration 000042, full CRUD service + repository + REST API handler. Role-based visibility filtering. Percentage-based coordinates for resolution independence.
- [ ] **Timeline Phase 2B** — Event connections (visual lines between related events), create-from-timeline modal, beautification pass.
- [ ] **Campaign export/import** — JSON bundle for backup/migration. Media as separate zip or URL references.
- [x] **Image drag-and-drop upload** — Media browser has drag-and-drop + multi-file upload with per-file progress bars (Alpine.js + XHR). Entity image widget (`image_upload.js`) still click-only.
- [ ] **Calendar week view** — Only month + timeline views exist. Week view is standard calendar UX expectation.
- [ ] **Calendar event drag-and-drop** — Can't drag events between dates (standard Google Calendar UX).
- [ ] **Calendar day view** — No single-day detailed view with time blocks.
- [ ] **Map marker clustering** — Markers don't auto-cluster when zoomed out (Leaflet.markercluster).
- [ ] **Map marker icon picker** — No predefined POI icons (city, dungeon, tavern, etc.).
- [ ] **Recent entities sidebar** — No "recently viewed" quick-access list.
- [ ] **Command palette (Ctrl+Shift+P)** — Quick action palette beyond Ctrl+K search.
- [ ] **Breadcrumb consistency** — Breadcrumbs exist on entity pages but not calendar/timeline/maps.
- [ ] **Timeline search/filter** — No search within timeline events by name/text.
- [ ] **Timeline zoom-to-era** — No button to jump viewport to a specific era.
- [ ] **Editor find/replace** — No Ctrl+F within editor content.
- [ ] **Editor code syntax highlighting** — Code blocks have no language-aware highlighting.
- [ ] **Entity version history UI** — Audit log exists but no "view diff / restore version" for entities.
- [ ] **Notes search/filter** — No search within notes panel.
- [ ] **Toast notification grouping** — Duplicate toasts stack separately instead of grouping.
- [ ] **Entity image gallery** — Only one image per entity; no carousel/gallery for multiple images.

### Post-Alpha

- [ ] Per-entity permissions (view/edit per role/user)
- [ ] Group-based visibility (beyond everyone/dm_only)
- [ ] Foundry VTT sync module Phase 1 (notes/journal sync)
- [ ] Foundry VTT sync module Phase 2 (calendar sync)
- [ ] Relations graph visualization widget (D3.js/Cytoscape.js)
- [ ] Dice roller widget (floating panel, expression parser)
- [ ] Entity sub-notes/posts (sub-documents with separate visibility)
- [ ] Auto-linking in editor (LegendKeeper-style entity name detection)
- [ ] Guided worldbuilding prompts per entity type (WorldAnvil-style)
- [ ] Role-aware dashboards (different views per campaign role)
- [ ] Entity type template library (genre presets for new campaigns)
- [ ] Saved filters / smart lists (filter presets as sidebar links)
- [ ] Bulk entity operations (multi-select for batch tag/move/delete)
- [ ] Whiteboards / freeform canvas (Tldraw/Excalidraw)
- [ ] Persistent filters per category (localStorage)
- [ ] 2FA/TOTP support
- [ ] Invite system (email invitations for campaigns)
- [ ] Webhook support for external event notifications
- [ ] Fog of war for maps (DM-only visibility per marker/region)
- [ ] Map drawing tools (freehand, shapes, annotations)
- [ ] Map hex/square grid overlay for tactical combat
- [ ] Map measurement tool (distance/area)
- [ ] Accessibility audit (ARIA labels, focus traps, screen readers, skip-to-content)
- [ ] Offline mode / service worker caching
- [ ] Collaborative editing presence indicators ("user X is editing")
- [ ] Additional themes beyond light/dark (sepia, high-contrast, custom accent colors)
- [ ] Editor markdown import/export
- [ ] Calendar timezone support (real-life mode uses UTC; need per-user timezone)
- [ ] Calendar print/PDF export
- [ ] Notes rich text (TipTap in notes instead of plain text blocks)
- [ ] Note folders/nesting organization
- [ ] Reusable modal/dropdown component library (widgets each build their own)
- [ ] Widget inline CSS → CSS classes migration (consistency, maintainability)

### Testing (High Priority)

- [x] Entity service unit tests (40 tests passing)
- [x] Sync API service tests (31 tests)
- [x] Addons service tests (32 tests)
- [x] Auth service tests (26 tests)
- [x] Notes widget service tests (28 tests)
- [x] Widget lifecycle audit (destroy methods, event listener leaks)
- [x] Campaigns service tests (72 tests covering CRUD, membership, ownership transfer, sidebar, dashboard, admin ops, model helpers)
- [x] Relations service tests (25 tests: bi-directional create/delete, symmetric relations, validation, conflict handling)
- [x] Tags service tests (40 tests: CRUD, color validation, slug generation, diff-based SetEntityTags, cross-campaign prevention, visibility filtering)
- [ ] Audit service tests (pagination, validation, fire-and-forget)
- [ ] Media service tests (file validation, thumbnail generation)
- [ ] Settings service tests (limit resolution, override priority)
- [ ] HTMX fragment edge cases (CSRF propagation, double-init, nested targets)

### Game System Modules

- [ ] D&D 5e module (SRD reference data, tooltips, pages)
- [ ] Pathfinder 2e module
- [ ] Draw Steel module

### Infrastructure

- [ ] **Add golangci-lint to CI pipeline** — Currently only `go vet` + unit tests run in CI. golangci-lint catches real bugs (unchecked errors, dead code) and should be enforced.
- [ ] **Add security scanning to CI** — gosec (static analysis) and govulncheck (dependency vulnerabilities) should run in CI. Requires Go 1.25+ for latest gosec.
- [ ] **Increase test coverage** — Currently 8 test files (294+ tests). Priority: media service tests, audit service tests, settings service tests, then handler tests.
- [ ] docker-compose.yml full stack verification (app + MariaDB + Redis)
- [ ] `air` hot reload setup for dev workflow

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
