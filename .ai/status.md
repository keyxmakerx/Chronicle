# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-21 -- Notes widget overhaul, addon gating, unified entity type config page,
sidebar drill-down fix.

## Current Phase
**Phase C: IN PROGRESS.** Notes widget overhauled with quick-capture UX.
Unified entity type configuration page consolidates Layout, Attributes,
Dashboard, and Nav Panel tabs.

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

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

## Active Branch
`claude/explore-project-soSu8`

## Next Session Should
1. **Docker rebuild:** `docker compose build --no-cache && docker compose up -d`
   to pick up all changes (templ/tailwind/migrations handled automatically in build).
2. **Test the sidebar drill-down** — verify panel slides on category click
   and the peek strip shows faded icons on the right edge.
3. **Test notes widget** — enable player-notes addon for a campaign, verify
   quick-add input works, responsive sizing on different viewports.
4. **Test config page** — navigate to Categories → Configure → verify all
   four tabs render correctly (Layout loads template editor widget).
5. **Dashboard layout configurability** — add `dashboard_layout` JSON column
   to campaigns table, create dashboard editor widget, make campaign and
   category dashboards owner-configurable.
6. **Tests** — Many plugins have zero tests. Priority: syncapi service, addons,
   notes widget service.
7. **Grid/Table view toggle** on category dashboards.
8. **Password reset** — Wire auth password reset with SMTP.

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
