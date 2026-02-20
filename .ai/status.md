# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Phase B features: discover page fix, template editor upgrades, field overrides, extension framework, sync API plugin.

## Current Phase
**Phase B: COMPLETE.** Extension framework and Sync API plugin implemented. All
Phase B features from the approved plan are done. Remaining Phase B items
(player notes, attribute template editing) are queued for next session.

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

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

## Active Branch
`claude/explore-project-soSu8`

## Next Session Should
1. **Run `make templ`, `make tailwind`, and `make migrate-up`** before testing —
   generated files are gitignored, migrations 000013-000016 need to be applied.
2. **Attribute template editing in campaign settings** — Allow campaign owners
   to edit entity type field definitions from a more accessible settings UI.
3. **Player Notes block type** — New `player_notes` table, block type in template
   editor, standalone notes pages per entity, scoped per user.
4. **Actual sync API endpoints** — The management infrastructure is built (keys,
   logging, monitoring). Next step is the actual `/api/v1/` REST endpoints for
   external clients to read/write campaign data.
5. **Rate limiting middleware** — The per-key rate limits are configured but the
   actual middleware to enforce them on `/api/v1/` routes needs to be implemented.
6. **Tests** — Many plugins have zero tests. Priority: syncapi service (key
   creation, authentication, bcrypt), addons service.
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
  admin/owner dashboards
