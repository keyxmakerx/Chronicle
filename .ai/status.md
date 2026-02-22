# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-22 -- Phase D Sprint 1 + 1.5: Campaign Customization Hub, settings cleanup.

## Current Phase
**Phase D: IN PROGRESS.** Campaign Customization Hub at `/campaigns/:id/customize`
with Navigation, Dashboard, Categories, and Category Dashboards tabs. Settings
page cleaned up (no more entity-type-config duplication). Custom sidebar sections
and links editor wired up.

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

### In Progress
- Phase D Sprint 2: Dashboard Editor (migration, widget, block rendering)

### Blocked
- Nothing blocked

## Active Branch
`claude/review-project-foundation-8rzHX`

## Next Session Should
1. **Sprint 2:** Dashboard Editor — Migration 000021 (`dashboard_layout` JSON on
   campaigns + entity_types), `dashboard_editor.js` widget, Templ components for
   each block type, campaign dashboard conditional render from layout JSON.
2. **Sprint 3:** Category Dashboards — Category dashboard editor, category-specific
   blocks, conditional render with fallback to hardcoded default.
3. **Sprint 4:** Player Notes Overhaul — Migration 000022, edit locking backend
   (pessimistic), rich text integration (TipTap), shared notes, version history,
   template block mount.
4. **Sprint 5:** Polish — hx-boost sidebar navigation, "View as player" toggle, testing.

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
