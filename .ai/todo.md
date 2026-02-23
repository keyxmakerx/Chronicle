# Chronicle Backlog

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Single source of truth for what needs to be done, priorities,    -->
<!--          and what has been completed.                                     -->
<!-- Update: At the start of a session (to understand priorities), during      -->
<!--         work (to mark progress), and at session end (to reflect).        -->
<!-- Legend: [ ] Not started  [~] In progress  [x] Complete  [!] Blocked      -->
<!-- ====================================================================== -->

## Next Up: Priority Tasks

These are the highest-priority items across all future phases. Pick from here.

### Phase D: Campaign Customization Hub (Active)
- [x] Campaign Customization Hub page shell (4 tabs, owner-only route)
- [x] Navigation tab (sidebar config widget + custom sections/links editor)
- [x] Settings cleanup (removed Categories duplication, added Customize link)
- [x] Custom sections/links sidebar rendering (context helpers + app.templ)
- [x] Admin panel flickering fix (x-cloak)
- [x] Sidebar debug log cleanup
- [x] Dashboard Editor (Sprint 2: migration 000021, dashboard_editor.js, block rendering)
- [x] Category Dashboards (Sprint 3: per-category layout editor)
- [x] Page Layouts tab (Sprint 3.5: HTMX lazy-loaded template-editor in Customize hub)
- [ ] Player Notes Overhaul (Sprint 4: locking, rich text, versions, shared, template block)
- [ ] hx-boost sidebar navigation (Sprint 5: prevent full page reloads)
- [ ] "View as player" toggle (Sprint 5)

### Phase B Follow-ups
- [x] Notes addon-gated rendering (IsAddonEnabled context check)
- [ ] Attribute template editing in campaign settings UI
- [ ] Foundry VTT companion module documentation
- [ ] API enhancements: entity tags/relations in responses, efficient sync pull

### Testing (High Priority -- Many plugins have zero tests)
- [x] Entity service unit tests (30 tests passing)
- [x] Sync API service tests (31 tests — key creation, bcrypt auth, IP check)
- [x] Addons service tests (28 tests — CRUD, campaign enable/disable)
- [x] Auth service tests (26 tests — register, password hashing, reset flow)
- [x] Notes widget service tests (28 tests — CRUD, checklists, scoping)
- [ ] Campaigns service tests
- [ ] Relations service tests (bi-directional create/delete, validation)
- [ ] Tags service tests (CRUD, slug generation, diff-based assignment)
- [ ] Audit service tests (pagination, validation, fire-and-forget)
- [ ] Media service tests (file validation, thumbnail generation)
- [ ] Settings service tests (limit resolution, override priority)

### Auth & Security
- [x] Password reset flow (migration 000020, forgot/reset pages, SMTP integration)
- [ ] 2FA/TOTP support
- [ ] Per-entity permissions (view/edit per role/user)
- [ ] Invite system (email invitations for campaigns)
- [ ] Group-based visibility (beyond everyone/dm_only)

### Maps & Geography
- [ ] Leaflet.js map viewer widget
- [ ] Map pin CRUD with entity linking

### Game System Modules
- [ ] D&D 5e module (SRD reference data, tooltips, pages) — registry in `internal/modules/registry.go`
- [ ] Pathfinder 2e module
- [ ] Draw Steel module

### API & Integrations
- [x] `/api/v1/` REST endpoints (campaign entities, types, fields read/write)
- [x] API key authentication middleware for `/api/v1/` routes
- [x] Rate limiting enforcement on API routes
- [ ] API enhancements: `modified_since` repo method for efficient sync pull
- [ ] API enhancements: tags/relations in API entity responses
- [ ] Campaign export/import
- [ ] Foundry VTT sync module (companion module)
- [ ] AI integration endpoint
- [ ] Webhook support for external event notifications

### UI & Navigation -- Phase 3 Follow-ups
- [x] Terminology rename (Entity→Page, Entity Type→Category) — UI labels only
- [x] Drill-down sidebar (iOS-style push navigation with peek)
- [x] Category dashboard pages (header, description, pinned, grid view)
- [x] Tighter card grid (4-col XL, reduced padding, compact badges)
- [x] DB migration 000013 (description + pinned_entity_ids on entity_types)
- [x] Grid/Table view toggle (Alpine.js toggle + localStorage + EntityTableRow component)
- [ ] Sub-folder support (parent_id tree view on category dashboard)
- [x] Settings consolidation (Customize page replaces scattered config)
- [ ] Persistent filters per category (localStorage)
- [x] Quick Links / Bookmarks in sidebar (custom sections/links via sidebar_nav_editor)

### QoL Features (Identified from Competitive Research)
- [ ] Quick search (Ctrl+K) — global search modal for entities, categories, notes
- [ ] Entity nesting (parent_id + tree view + breadcrumbs + "Create sub-page")
- [ ] Inline secrets / GM-only text (TipTap extension, server-side filtering)
- [ ] Breadcrumb navigation (Campaign > Category > Entity in topbar)

### Entities -- Remaining Features
- [ ] Entity nesting (parent/child relationships, tree view on dashboard)
- [ ] Entity posts (additional sections on entity profile)
- [ ] Relations graph visualization widget

### Infrastructure
- [ ] docker-compose.yml full stack verification (app + MariaDB + Redis)
- [ ] `air` hot reload setup for dev workflow
- [ ] Verify `make docker-up` -> `make dev` works end-to-end

### Future Plugins
- [ ] Calendar plugin (custom months, days, moons)
- [ ] Timeline plugin (eras, events, zoomable)

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

**Plugins:**
- [x] Media plugin (upload, thumbnails, magic byte validation, rate limiting)
- [x] Audit plugin (campaign activity timeline, stats, wired into handlers)
- [x] Site settings plugin (storage limits, per-user/campaign overrides)
- [x] Admin modules page (registry, card grid, status badges)

**Widgets:**
- [x] Editor view/edit toggle (read-only default, Edit/Done, autosave)
- [x] @mention system (search popup, keyboard nav, styled links)
- [x] Attributes widget (inline edit for all field types, full-stack)
- [x] Tag picker widget (search, create, assign on entity profiles)
- [x] Tag display on entity list cards (batch fetch, colored chips)
- [x] Relations widget (bi-directional linking, common types, reverse auto-create)
- [x] Template editor (drag-and-drop page builder, 2-col/3-col/tabs/sections, preview)
- [x] Entity tooltip/popover (hover preview, LRU cache, smart positioning)

**Entity enhancements:**
- [x] Entity type CRUD (create, edit, delete, icon/color/fields management)
- [x] Entity list redesign (horizontal tabs, search bar, stats)
- [x] Entity image upload pipeline + UI
- [x] Descriptor rename (Subtype Label -> Descriptor)
- [x] Dynamic sidebar with entity types from DB + count badges
- [x] Sidebar customization (drag-to-reorder, hide/show per campaign)
- [x] Layout-driven entity profile pages (layout_json)

**Security:**
- [x] Comprehensive security audit (14 vulnerability fixes)
- [x] IDOR protection on all entity endpoints
- [x] HSTS security header
- [x] Rate limiting (auth + uploads)
- [x] Storage limit enforcement in media upload

**UI & Styling:**
- [x] Dark mode toggle (theme.js, localStorage, sidebar button)
- [x] Semantic color system (CSS custom properties + Tailwind tokens)
- [x] All templ files migrated to semantic color tokens (20+ files)
- [x] All CSS components migrated to semantic tokens
- [x] Visual polish pass (gradient hero, icon cards, refined buttons/cards)
- [x] Public landing page with discoverable campaign cards
- [x] Collapsible admin sidebar with modules section
- [x] Toast notification system (Chronicle.notify API + HTMX integration)
- [x] Public campaign support (is_public flag, OptionalAuth)

**Phase 2 Polish (2026-02-20):**
- [x] Entity type badge contrast (luminance-based text color for light backgrounds)
- [x] Dark mode fix for entity type config widget (semantic tokens)
- [x] Merged campaign Edit + Settings into unified settings page
- [x] Game Modules section in campaign settings (shows available modules)
- [x] Admin plugins page (plugin registry, active/planned status, categories)

### Phase 3: Competitor-Inspired UI Overhaul (2026-02-20)
- [x] Terminology rename (Entity→Page, Entity Type→Category)
- [x] Drill-down sidebar (iOS Settings-style push nav with peek)
- [x] Category dashboard pages (customizable landing with pinned, tags, grid)
- [x] DB migration 000013 (description + pinned_entity_ids on entity_types)
- [x] Tighter card spacing (4-col XL, reduced padding, compact badges)

### Phase B: Extensions & API (2026-02-20)
- [x] Discover page split (DiscoverPublicPage + DiscoverAuthPage + AboutPage)
- [x] Discover link in sidebar (authenticated users can browse public campaigns)
- [x] Template editor block resizing (minHeight presets: auto/sm/md/lg/xl)
- [x] Block-level visibility controls (everyone/dm_only with role-based filtering)
- [x] Per-entity field overrides (migration 000014, MergeFields, customization panel)
- [x] Extension framework — addons plugin (migration 000015, model/repo/service/handler)
- [x] Admin addon management page with status controls + creation form
- [x] Campaign addon settings with per-campaign toggle (HTMX)
- [x] Sync API plugin (migration 000016, model/repo/service/handler)
- [x] Owner API key management (create/toggle/revoke, usage stats)
- [x] Admin API monitoring dashboard (stats, charts, security events, IP blocklist, keys)

### Phase C: Notes & Terminology (2026-02-20)
- [x] Player Notes widget (migration 000017-000018, floating panel, checklists)
- [x] Terminology standardization: all user-facing "Addon" → "Extension"
- [x] Admin dashboard: removed "Modules" card, unified "Extensions" card with count
- [x] Admin sidebar: removed "Modules" link, renamed "Addons" → "Extensions"
- [x] Campaign settings: removed duplicate "Game Modules" section
- [x] Migration 000019: fixed addon table status mismatches (sync-api, game modules, dice-roller, media-gallery)

### Phase D Sprint 1 + 1.5: Customization Hub (2026-02-22)
- [x] Admin panel flickering fix (x-cloak on admin-slide div)
- [x] Sidebar debug log cleanup (removed 3 console.log from sidebar_drill.js)
- [x] Campaign Customization Hub page (`/campaigns/:id/customize`, 4 tabs)
- [x] Navigation tab with sidebar config widget + custom sections/links editor
- [x] Categories tab with entity type grid linking to per-type config pages
- [x] Dashboard + Category Dashboards tabs with "coming soon" placeholders
- [x] Sidebar "Customize" link (paintbrush icon, owner-only)
- [x] Settings page: replaced duplicated Categories section with link card
- [x] New widget: `sidebar_nav_editor.js` (custom sections + links CRUD)
- [x] Context helpers: SidebarSection/SidebarLink types, Set/Get functions in data.go
- [x] Custom nav items rendered in sidebar (sections as headers, links with icons)
- [x] External link detection (target="_blank" + arrow icon for https:// URLs)

### Phase D Sprint 2: Dashboard Editor (2026-02-22)
- [x] Migration 000021: `dashboard_layout JSON DEFAULT NULL` on campaigns + entity_types
- [x] Dashboard layout Go types (DashboardLayout, DashboardRow, DashboardColumn, DashboardBlock)
- [x] ParseDashboardLayout() method + ValidBlockTypes map + block type constants
- [x] Repository: all queries include dashboard_layout, UpdateDashboardLayout method
- [x] Service: UpdateDashboardLayout (validation), GetDashboardLayout, ResetDashboardLayout
- [x] Handler + routes: GET/PUT/DELETE `/campaigns/:id/dashboard-layout` (owner-only)
- [x] `dashboard_editor.js` widget (drag-and-drop, 6 block types, row presets, config dialogs)
- [x] `dashboard_blocks.templ` (DashboardBlockSwitch + 6 block components)
- [x] `show.templ` refactored: custom layout → 12-col grid render, NULL → hardcoded default
- [x] Customize page Dashboard tab mounts dashboard-editor widget
- [x] dashColSpan, dashGridClass, limitRecentEntities helper functions

### Phase D Sprint 3: Category Dashboards (2026-02-22)
- [x] `dashboard_editor.js` parameterized with `data-block-types` attribute for custom palettes
- [x] EntityType model: `DashboardLayout *string` field + `ParseCategoryDashboardLayout()` method
- [x] Repository: all entity type queries include `dashboard_layout`, `UpdateDashboardLayout()` method
- [x] Service: `GetCategoryDashboardLayout`, `UpdateCategoryDashboardLayout` (validation), `ResetCategoryDashboardLayout`
- [x] Handler + routes: GET/PUT/DELETE `/entity-types/:etid/dashboard-layout` (owner-only)
- [x] New block type constants: `category_header`, `entity_grid`, `search_bar` in campaigns/model.go
- [x] `category_blocks.templ`: CategoryBlockSwitch + 6 category block components
- [x] `category_dashboard.templ`: conditional render from custom layout or hardcoded default
- [x] Customize page Category Dashboards tab: Alpine.js category selector + dashboard-editor per category

### Phase D Sprint 3.5: Page Layouts Tab (2026-02-23)
- [x] `template_editor.js`: `destroy()` method for HTMX lifecycle cleanup
- [x] `template_editor.js`: scoped `findSaveBtn()`/`findSaveStatus()` helpers (fragment + fallback)
- [x] `EntityTypeLayoutFetcher` interface + `LayoutEditorEntityType` struct in campaigns/handler.go
- [x] `LayoutEditorFragment` handler (GET `/customize/layout-editor/:etid`, IDOR protection)
- [x] Route registration in campaigns/routes.go (owner-only)
- [x] Customize page: fifth "Page Layouts" tab button + tab content panel
- [x] `pageLayoutsTab` component: category selector + HTMX lazy-load triggers
- [x] `LayoutEditorFragment` templ component: scoped save controls + template-editor mount
- [x] `entityTypeLayoutFetcherAdapter` in app/routes.go (bridges entities service → campaigns handler)
- [x] Entity type config page: back button → `/campaigns/:id/customize`
