# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-24 -- Sprint 4: Player Notes Overhaul. Full backend + frontend for shared
notes, pessimistic edit locking, version history, and rich text display support.

## Current Phase
**Phase D: IN PROGRESS.** Sprint 4 (Player Notes Overhaul) complete. Sprint 5
(Polish) next.

## What Was Built in Phase B (Summary)

### Discover Page Fix
- Split monolithic landing into **DiscoverPublicPage** (Base layout, compact welcome
  banner, campaign grid, signup CTA) and **DiscoverAuthPage** (App layout with sidebar).
- **AboutPage** at `/about` — full marketing/welcome page.
- Discover link added to both sidebar nav states (default + campaign).
- `/` route uses `OptionalAuth` to serve appropriate layout based on session.

### Template Editor Block Resizing
- Added `minHeight` property to block config with presets: auto/sm/md/lg/xl.
- Height dropdown in template editor renders as CSS `min-height` on entity profiles.
- No migration needed — stored in existing `layout_json` on entity types.

### Block-Level Visibility Controls
- Added `visibility` property to block config: `everyone` (default) or `dm_only`.
- DM-only blocks filtered at Go template render time based on campaign role.
- Visual indicators: amber border + lock icon in editor, "DM Only" badge on profile.

### Per-Entity Attribute Overrides
- **Migration 000014:** `field_overrides JSON` column on entities table.
- `FieldOverrides` struct: Added (new fields), Hidden (hide type fields), Modified (label/type/options overrides).
- `MergeFields()` function combines type-level and entity-level field definitions.
- `GET /entities/:eid/fields` now returns effective fields + type_fields + field_overrides.
- `PUT /entities/:eid/field-overrides` endpoint for saving overrides.
- Attributes widget: gear icon opens customization panel (toggle visibility, add custom fields).

### Extension Framework (Addons Plugin)
- **Migration 000015:** `addons` and `campaign_addons` tables with 11 seeded defaults.
- Full plugin: model, repository, service, handler, routes, templ templates.
- **Admin page** (`/admin/addons`): Addon management with status controls, creation form.
- **Campaign page** (`/campaigns/:id/addons/settings`): Per-campaign addon toggle (HTMX).
- Grouped by category (module/widget/integration) with enable/disable buttons.
- Wired into admin sidebar, admin dashboard, campaign settings.

### Sync API Plugin
- **Migration 000016:** `api_keys`, `api_request_log`, `api_security_events`, `api_ip_blocklist`.
- API key management: bcrypt-hashed, per-campaign, permissions (read/write/sync),
  optional IP allowlist, rate limits, expiry.
- **Owner dashboard** (`/campaigns/:id/api-keys`): Create/toggle/revoke keys,
  usage stats, security notes.
- **Admin monitoring dashboard** (`/admin/api`): Stats overview (8 cards),
  request/security time series charts, top IPs/paths/keys tables,
  security event table with resolve actions, IP blocklist management,
  key oversight with activate/deactivate/revoke.
- Security event types: rate_limit, auth_failure, ip_blocked, key_expired, suspicious.
- Wired into admin sidebar, admin dashboard, campaign settings.

### REST API v1 Endpoints
- **Middleware:** `RequireAPIKey` (Bearer token auth + bcrypt verify + IP check +
  request logging), `RateLimit` (fixed-window per-minute), `RequireCampaignMatch`,
  `RequirePermission` (read/write/sync).
- **Read endpoints:** GET campaign info, list/get entity types, list/get entities
  (with search, pagination, type filter, privacy enforcement).
- **Write endpoints:** POST/PUT/DELETE entities, PUT fields-only update.
- **Sync endpoint:** POST bidirectional sync — pull entities modified since timestamp,
  push batch create/update/delete operations, returns server_time for next sync.
- Middleware chain: RequireAPIKey → RateLimit → RequireCampaignMatch → RequirePermission.
- Privacy enforcement uses key owner's campaign role for entity visibility.

### Player Notes Widget (Phase C)
- **Migration 000017:** `notes` table (per-user, per-campaign, optional entity scoping).
- **Migration 000018:** Activates `player-notes` addon from planned to active.
- **Widget backend:** `internal/widgets/notes/` — model, repository, service, handler, routes.
- **Widget frontend:** `static/js/widgets/notes.js` — floating panel (bottom-right),
  tab system (This Page / All Notes), text blocks, interactive checklists.
- **CSS:** Notes panel styles in `input.css` (fab button, panel, cards, checklists).
- **Mount point:** Auto-rendered in app layout when user is in a campaign.
- **API routes:** CRUD at `/campaigns/:id/notes`, checklist toggle, scope filtering.
- **Addon integration:** Uses existing `player-notes` addon (disabled by default,
  owner enables per-campaign via addons settings page).

### Terminology Standardization (Phase C)
- Two-tier user-facing model: **Plugins** (core, always-on) and **Extensions** (optional, toggleable).
- Renamed all user-facing "Addon" → "Extension" in admin and campaign templates.
- Admin dashboard: Removed separate "Modules" card, unified into "Extensions" card with count.
- Admin sidebar: Removed "Modules" link, renamed "Addons" → "Extensions".
- Campaign settings: Removed duplicate "Game Modules" section (modules in extensions system).
- **Migration 000019:** Fixes addon table status mismatches (sync-api→active, game modules→planned,
  dice-roller→planned, media-gallery→planned).
- Internal Go code (types, function names, routes, DB tables) intentionally kept as `addon` —
  only user-facing text was renamed.

### Sidebar Drill-Down Fix
- Root cause: click events bubbled from category links to mainPanel handler,
  which immediately called drillOut(). Fixed with e.stopPropagation().
- Switched from calc() to pixel values in translate3d for reliability.
- Added CSS rules for peek mode: flex-direction: row-reverse on nav links,
  text-align: right on nav content (so icons/text visible in 10px peek strip).

### Notes Widget Addon Gating
- Added `SetEnabledAddons` / `IsAddonEnabled` context helpers in layouts/data.go.
- LayoutInjector queries AddonService.ListForCampaign and populates enabled slugs.
- NotesWidget only renders when `player-notes` addon is enabled per campaign.

### Notes Widget Quick-Capture Overhaul
- Panel opens with always-visible quick-add input at top, auto-focused.
- Type + Enter = instant note creation (no extra clicks).
- Dual mode: "This Page" (auto on entity pages) vs "All Notes" (campaign-wide).
- Responsive sizing: full-width bottom sheet on mobile, 320px on medium, 400px on large.
- Removed separate "+" button; quick-add bar replaces it.

### Unified Entity Type Configuration Page
- New route: `/campaigns/:id/entity-types/:etid/config`
- Tabbed interface (Alpine.js): Layout, Attributes, Dashboard, Nav Panel.
- Layout tab embeds the existing template-editor drag-and-drop widget.
- Attributes tab mounts entity-type-editor widget (field-only mode).
- Dashboard tab has description editor + pinned pages reference.
- Nav Panel tab has icon picker, color picker, name/plural name editing.
- Entity type cards on management page updated: "Configure" link → config page.

### Notes Panel Resize + Taller Default
- Default sizes bumped: mobile 75vh, medium 340×520px, large 400×600px.
- Drag-to-resize handle on top-left corner (min 280×300px).
- Dimensions persist in localStorage (`chronicle_notes_size`).
- Desktop-only restore; mobile always uses full-width bottom sheet.

### Entity Type Editor Fields-Only Mode
- `data-fields-only="true"` hides name/icon/color sections.
- Used by Attributes tab on unified config page.
- Save sends original name/icon/color from data attrs alongside updated fields.

### Campaign Dashboard Enhancement
- Category quick-nav grid with entity type icons, colors, and page counts.
- Reads from layout context (GetEntityTypes/GetEntityCount) — zero new queries.
- Responsive: 2–5 columns depending on viewport.
- Quick actions row tightened to horizontal icon+text layout.

### Grid/Table View Toggle
- Alpine.js-powered toggle on category dashboards and All Pages list.
- Per-category localStorage persistence (`chronicle_cat_view_{id}`).
- Table view: name with image/icon, category type badge, tags, relative time, privacy.
- Shared `EntityTableRow` templ component used by both views.
- `relativeTime()` helper for human-friendly timestamps (1m ago, 3d ago).

### Recent Pages on Campaign Dashboard
- New `ListRecent` repository method: `ORDER BY updated_at DESC LIMIT ?`.
- `RecentEntityLister` interface on campaigns handler (avoids circular imports).
- Adapter in routes.go bridges entities.EntityService → campaigns.RecentEntity.
- Dashboard shows 8 most recent pages in a 4-column grid with type badge + timestamp.

### Password Reset Flow
- **Migration 000020:** `password_reset_tokens` table with SHA-256 hashed tokens.
- Repository: `UpdatePassword`, `CreateResetToken`, `FindResetToken`, `MarkResetTokenUsed`.
- Service: `InitiatePasswordReset` (generates token, sends email via SMTP),
  `ValidateResetToken`, `ResetPassword`. Always returns nil on unknown emails
  to prevent enumeration. Tokens expire in 1 hour, single-use.
- Handler: `GET/POST /forgot-password`, `GET/POST /reset-password` with 3/min rate limit.
- Templates: `forgot_password.templ` (request form + sent confirmation),
  `reset_password.templ` (new password form with expired/used token error states).
- Login page: "Forgot password?" link, green success banner after reset.
- `ConfigureMailSender()` wires SMTP into auth service in routes.go.

### Unit Test Coverage
- **Auth service:** 26 tests — Register, Login helpers, password hashing, password
  reset flow (initiate/validate/reset), token hashing, UUID generation.
- **Addons service:** 28 tests — CRUD, validation, slug uniqueness, enable/disable
  for campaigns, status transitions, input trimming.
- **Notes widget service:** 28 tests — Create/Update/Delete, title validation &
  defaults, checklist toggle (bounds, type, toggle logic), list queries, ID generation.
- **Syncapi service:** 31 tests — Key creation (validation, bcrypt, defaults),
  authentication (prefix lookup, bcrypt verify, deactivated, expired), activate/
  deactivate/revoke, IP blocking, non-critical logging, default limits, model methods.
- **Entities service:** Existing 20+ tests, updated mock for ListRecent interface.

### Phase D Sprint 1 + 1.5 (2026-02-22)
- Campaign Customization Hub page (`/campaigns/:id/customize`) with 4 tabs.
- Navigation tab: sidebar config widget + custom sections/links editor widget.
- Categories tab: entity type grid with links to per-type config pages.
- Dashboard + Category Dashboards tabs: "coming soon" placeholders.
- Settings page: replaced duplicated Categories section with link to Customize.
- Sidebar: "Customize" link (paintbrush icon, owner-only) between Activity Log and Settings.
- Custom nav sections and links render in the actual sidebar.
- New widget: `sidebar_nav_editor.js` (CRUD for custom sections + links).
- Context helpers: `SetCustomSections/GetCustomSections`, `SetCustomLinks/GetCustomLinks`.
- Admin panel flickering fix: `x-cloak` on admin-slide div.
- Sidebar debug logs: removed 3 `console.log()` from `sidebar_drill.js`.

### Phase D Sprint 2: Dashboard Editor (2026-02-22)
- Migration 000021: `dashboard_layout JSON DEFAULT NULL` on campaigns + entity_types.
- Dashboard layout types: DashboardLayout/Row/Column/Block in model.go.
- Repository: all campaign queries updated for dashboard_layout column, UpdateDashboardLayout method.
- Service: UpdateDashboardLayout (validation: max 50 rows, max 20 blocks/row, widths 1-12, valid block types), GetDashboardLayout, ResetDashboardLayout.
- Handler: GET/PUT/DELETE `/campaigns/:id/dashboard-layout` (owner-only).
- Widget: `dashboard_editor.js` — drag-and-drop layout builder with palette (6 block types), row presets (full/half/thirds/quarter/sidebar), block config dialogs, save/reset.
- Templ: `dashboard_blocks.templ` — DashboardBlockSwitch + 6 block components (welcome_banner, category_grid, recent_pages, entity_list, text_block, pinned_pages).
- Show page: `show.templ` refactored — ParseDashboardLayout() → customDashboard (12-col grid) or defaultDashboard (hardcoded original).
- Customize page: Dashboard tab now mounts dashboard-editor widget.
- Helper functions: dashColSpan (col-span CSS), dashGridClass (responsive grid), limitRecentEntities (configurable limit), dashboardRelativeTime.

### Phase D Sprint 3: Category Dashboards (2026-02-22)
- `dashboard_editor.js` parameterized with `data-block-types` attribute for custom palettes.
- EntityType model: added `DashboardLayout *string` field + `ParseCategoryDashboardLayout()` method.
- Repository: all entity type queries updated for `dashboard_layout` column, `UpdateDashboardLayout()` method.
- Service: `GetCategoryDashboardLayout`, `UpdateCategoryDashboardLayout` (validation), `ResetCategoryDashboardLayout`.
- Handler + routes: GET/PUT/DELETE `/campaigns/:id/entity-types/:etid/dashboard-layout` (owner-only).
- 3 new block type constants: `category_header`, `entity_grid`, `search_bar` (+ reuses `pinned_pages`, `text_block`, `recent_pages`).
- `category_blocks.templ`: CategoryBlockSwitch + 6 category block components.
- `category_dashboard.templ`: conditional render — custom layout (12-col grid) or hardcoded default.
- Customize page: Category Dashboards tab — Alpine.js category selector + dashboard-editor widget per category.

### Phase D Sprint 3.5: Page Layouts Tab (2026-02-23)
- Fifth "Page Layouts" tab in Customization Hub for editing entity type page templates.
- HTMX lazy-loading: category selector buttons fetch template-editor fragment on demand.
- `EntityTypeLayoutFetcher` cross-plugin interface + adapter (same pattern as EntityTypeLister).
- `template_editor.js`: added `destroy()` method for HTMX lifecycle cleanup, scoped
  `findSaveBtn()`/`findSaveStatus()` helpers for fragment-embedded save controls.
- Entity type config page back button now returns to Customization Hub.

### Phase D Sprint 4: Player Notes Overhaul (2026-02-24)
- **Migration 000022:** Added collaboration columns to notes table: `is_shared`,
  `last_edited_by`, `locked_by`, `locked_at`, `entry` (JSON), `entry_html` (TEXT).
  Created `note_versions` table with FK cascade delete + indexes.
- **Shared notes:** `is_shared` flag allows campaign members to see/edit shared notes.
  Share toggle button in UI (owner only). Shared badge on other users' shared notes.
  List queries return `user_id = ? OR is_shared = TRUE`.
- **Pessimistic edit locking:** 5-minute auto-expiry with stale lock reclamation.
  `AcquireLock`, `ReleaseLock`, `ForceReleaseLock` (owner override), `RefreshLock`
  (heartbeat). Widget acquires lock before editing shared notes, sends 2-minute
  heartbeat, releases on done/close. Lock indicator shown when another user holds lock.
- **Version history:** Snapshot-on-save pattern — every Update/RestoreVersion creates
  a version snapshot. `MaxVersionsPerNote = 50`, oldest pruned automatically.
  Version history sub-panel with back navigation and one-click restore.
- **Rich text support:** `entry` (ProseMirror JSON) + `entry_html` (pre-rendered HTML)
  dual storage. Widget displays `entryHtml` in view mode when present.
- **Layout data:** Added `SetUserID`/`GetUserID` to layout context, wired in
  LayoutInjector, passed as `data-user-id` on notes widget mount point.
- **Backend:** model.go, repository.go, service.go, handler.go, routes.go all updated.
  Handler enforces: only owner can delete/pin/change sharing. Non-owners can edit
  shared note content but not metadata. ForceUnlock requires campaign owner role.
- **Frontend:** notes.js updated with lock/unlock flow, heartbeat timer, share toggle,
  version history panel, lock error toast, rich text display.
- **CSS:** Shared note accent, lock/shared badges, lock toast animation, rich text
  styles, version history list styles.
- **Tests:** All 28 service tests pass. Mock updated with all new repo methods.

### In Progress
- Phase D Sprint 5: Polish (next)

### Blocked
- Nothing blocked

## Active Branch
`claude/document-architecture-tiers-yuIuH`

## Competitive Analysis & Roadmap
Created `.ai/roadmap.md` with comprehensive comparison vs WorldAnvil, Kanka, and
LegendKeeper. Key findings:
- Chronicle is ahead on page layout editor, dashboards, self-hosting, and modern stack
- Critical gaps: Quick Search (Ctrl+K), entity hierarchy, calendar, maps, inline secrets
- Calendar is identified as a DIRE NEED — Kanka's is the gold standard
- API technical documentation needed for Foundry VTT integration
- Foundry VTT module planned in phases: notes sync → calendar sync → actor sync
- Features organized by tier: Core, Plugin, Module, Widget, External
- Revised priority phases: D (finishing) → E (UX) → F (calendar/time) → G (maps) →
  H (secrets) → I (integrations) → J (visualization) → K (delight)

## Next Session Should
1. **Sprint 5:** Polish — hx-boost sidebar navigation, "View as player" toggle,
   widget lifecycle audit.
2. **Phase E:** Quick Search (Ctrl+K), Entity Nesting (parent_id UI), Backlinks,
   API documentation.
3. **Phase F:** Calendar plugin (custom months, moons, eras, events, entity linking).
   See `.ai/roadmap.md` for full data model and implementation plan.

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`
- Tailwind standalone CLI (`tailwindcss`) is v3; do NOT use `npx @tailwindcss/cli` (v4 syntax)

## Completed Phases
- **2026-02-19: Phase 0** — Project scaffolding, AI docs, build config
- **2026-02-19: Phase 1** — Auth, campaigns, SMTP, admin, entities, editor, UI layouts,
  unit tests, Dockerfile, CI/CD, production deployment, auto-migrations
- **2026-02-19 to 2026-02-20: Phase 2** — Media plugin, security audit (14 fixes),
  dynamic sidebar, entity images, sidebar customization, layout builder, entity type
  config/color picker, public campaigns, visual template editor, dark mode, tags,
  audit logging, site settings, tag picker, @mentions, entity tooltips, relations,
  entity type CRUD, visual polish, semantic color system, notifications, modules page,
  attributes widget, editor view/edit toggle, entity list redesign
- **2026-02-20: Phase 3** — Competitor-inspired UI overhaul: Page/Category rename,
  drill-down sidebar, category dashboards, tighter cards
- **2026-02-20: Phase B** — Discover page split, template editor block resizing &
  visibility, field overrides, extension framework (addons), sync API plugin with
  admin/owner dashboards, REST API v1 endpoints (read/write/sync)
