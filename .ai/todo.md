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
- [ ] **No media management for campaign owners** — Admin has `/admin/storage` page. Campaign owners have NO way to browse, manage, or delete their uploads. Need campaign-scoped media browser at `/campaigns/:id/media` with "referenced by" tracking, delete with warnings, and upload from browser page.

### Medium

- [x] **Tags not hideable from players** — Implemented `dm_only` column (migration 000038), role-based filtering in repo/service/handler, eye-slash badge + DM checkbox in tag_picker.js.
- [x] **Attributes missing "Use Template" reset** — Added DELETE `/field-overrides` endpoint and "Reset" button in attributes customize panel with confirmation dialog. Clears field_overrides to NULL, restoring category template defaults.

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

- [ ] **Media management for owners + admins** — Campaign-scoped media browser: grid/list view, "referenced by" queries, delete with entity reference warnings, upload from browser page. Admin view spans all campaigns.
- [x] **Tag visibility controls** — Implemented: migration 000038, `dm_only` bool in model/repo/service/handler, role-based filtering, tag_picker.js DM-only badge + create checkbox.
- [x] **Attributes template reset** — Implemented DELETE endpoint + "Reset" button in customize panel with confirmation dialog.
- [ ] **Extension technical documentation** — 1-3 page `.ai.md` writeup per plugin/widget/module. Standard template covering purpose, architecture, API endpoints, widget integration, lifecycle, security. See documentation audit in plan.
- [ ] **Graceful extension degradation** — `RequireAddon` API middleware, human-readable errors for disabled/uninstalled addons, addon dependency checking.

### Alpha-Nice-to-Have

- [ ] **File security audit + ClamAV** — Add ClamAV container to docker-compose, scan uploads before storage, configurable file type allowlist, SVG blocking (XSS vector).
- [ ] **API documentation** — OpenAPI 3.0 spec or handwritten reference for REST v1. Auth guide, endpoint reference, rate limiting docs, sync protocol.
- [ ] **Maps Phase 2** — Layers, marker groups, privacy controls, nested maps (world → continent → city).
- [ ] **Timeline Phase 2B** — Event connections (visual lines between related events), create-from-timeline modal, beautification pass.
- [ ] **Campaign export/import** — JSON bundle for backup/migration. Media as separate zip or URL references.

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
- [ ] **Increase test coverage** — Currently 5.3% (6 test files). Priority: campaigns service tests, relations service tests, tags service tests, media service tests, then handler tests.
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
