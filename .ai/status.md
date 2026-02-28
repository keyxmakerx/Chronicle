# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-28 -- Phase H started: inline secrets (GM-only text). TipTap secret mark
extension, server-side stripping for non-scribe users, CSS styling with eye-slash
indicator.

## Current Phase
**Phase H: Secrets & Permissions.** Inline secrets complete. Next: per-entity
permissions, campaign export/import, or Maps Phase 2.

## Phase E: Entity Hierarchy & Extension Bug Fix (2026-02-24)

### Extension Enable Bug Fix — COMPLETE
- **Root cause**: Admin panel allowed activating planned addons (Calendar, Maps, Dice Roller)
  that have no backing code. Once activated, campaign owners could "enable" them — nothing
  would happen because the code doesn't exist.
- **Fix**: Added `installedAddons` registry in `service.go` — a `map[string]bool` listing
  slugs with real backing code (currently only `"sync-api"`).
- **Service layer**: `UpdateStatus()` blocks activating uninstalled addons. `EnableForCampaign()`
  blocks enabling uninstalled addons. `List()` and `ListForCampaign()` annotate `Installed` field.
- **Admin UI**: Uninstalled addons show "Not installed" label and disabled activate button.
- **Campaign UI**: Uninstalled addons show "Coming Soon" badge instead of enable/disable toggle.
- **Model**: Added `Installed bool` to both `Addon` and `CampaignAddon` structs (not persisted).
- **Tests**: 5 new tests (TestIsInstalled, TestUpdateStatus_ActivateInstalled,
  TestUpdateStatus_ActivateUninstalled, TestEnableForCampaign_NotInstalled,
  TestListForCampaign_AnnotatesInstalled). All 32 addon tests pass.

### Entity Hierarchy — COMPLETE (4 Sprints)

**Sprint 1: Data Plumbing**
- Added `ParentID` to `CreateEntityRequest`, `UpdateEntityRequest`, `CreateEntityInput`,
  `UpdateEntityInput` in `model.go`.
- Added `FindChildren`, `FindAncestors`, `UpdateParent` to `EntityRepository` interface.
- `FindAncestors` uses recursive CTE with depth limit of 20.
- Service: parent validation (exists, same campaign, no self-reference, circular reference
  detection by walking ancestor chain of proposed parent).
- 8 new hierarchy tests, all passing.

**Sprint 2: Parent Selector on Forms**
- `form.templ`: Added `parentSelector` component — Alpine.js searchable dropdown using
  existing entity search endpoint (`Accept: application/json` header).
- Pre-fill parent from `?parent_id=` query param ("Create sub-page" flow).
- Edit form pre-populates current parent with "Clear" button.
- `EntityNewPage` and `EntityEditPage` accept `parentEntity *Entity` parameter.

**Sprint 3: Breadcrumbs + Children + Create Sub-page**
- `show.templ`: Breadcrumb now shows ancestor chain (furthest first, immediate parent last).
  Each ancestor is a clickable link.
- `blockChildren` component: sub-pages section with card grid + "Create sub-page" button
  linking to `/campaigns/:id/entities/new?parent_id=:eid&type=:typeID`.
- Handler `Show()` fetches ancestors via `GetAncestors()` and children via `GetChildren()`.

**Sprint 4: Tree View on Category Dashboard**
- `category_dashboard.templ`: Third view toggle (grid/table/tree) with localStorage persistence.
- `EntityTreeNode` struct + `buildEntityTree()` builds tree from flat entity list using parent_id.
- `entityTreeLevel` recursive templ component: collapsible nodes with expand/collapse chevrons,
  entity icon/name links, privacy indicators, child count badges, indented border-left nesting.

### Editor Insert Menu — COMPLETE
- Added `+` button to editor toolbar that opens a dropdown with discoverable insertions.
- Menu items: Mention Entity (Type @), Insert Link, Horizontal Rule (---),
  Blockquote (>), Code Block (```). Each shows shortcut hint.
- "Mention Entity" inserts `@` at cursor and triggers the mention popup.
- "Insert Link" prompts for URL and wraps selection or inserts link text.
- Extensible for future features (secrets, embeds, etc.).
- Files: `editor.js` (createInsertMenu, executeInsert), `input.css` (dropdown styles).

### Backlinks / "Referenced by" — COMPLETE
- **Repository**: `FindBacklinks()` searches `entry_html` for `data-mention-id="<entityID>"`
  pattern using LIKE query. Returns up to 50 results, respects privacy by role.
- **Service**: `GetBacklinks()` delegates to repo with error wrapping.
- **Handler**: `Show()` fetches backlinks and passes to template.
- **Template**: `blockBacklinks` component renders a "Referenced by" section with
  entity type icon/name chips, styled as pill links. Only shown when backlinks exist.
- **Tests**: 1 new test (TestGetBacklinks_DelegatesToRepo). All 39 entity tests pass.

### Entity Preview Tooltip Enhancement — COMPLETE
- **Migration 000023**: Added `popup_config` JSON column to entities table.
- **Model**: `PopupConfig` struct with `ShowImage`, `ShowAttributes`, `ShowEntry` booleans.
  `EffectivePopupConfig()` returns entity config or defaults (all true).
- **Preview API**: Enhanced `GET /entities/:eid/preview` to include attributes (up to 5
  key-value pairs from entity type fields) and respect popup_config visibility toggles.
- **Popup Config API**: New `PUT /entities/:eid/popup-config` saves per-entity preview settings.
- **Tooltip Widget**: Enhanced `entity_tooltip.js` with side-by-side layout (gradient-bordered
  image on left + type badge/name/attributes on right), entry excerpt below, dynamic layout
  adapting based on available data.
- **Edit Form**: "Hover Preview Settings" collapsible section with checkboxes for
  Show Image / Show Attributes / Show Entry. Auto-saves via API with inline status feedback.
  Clears tooltip cache after save so changes are immediately visible.

### Admin Security Dashboard — COMPLETE
- **Migration 000024**: `security_events` table + `is_disabled` column on users table.
- **Security Event Logging**: Site-wide event log tracking logins (success/failed), logouts,
  password resets (initiated/completed), admin privilege changes, user disable/enable,
  session terminations, and force logouts.
- **Active Sessions View**: Admin can see all active sessions (user, IP, client, created time)
  with ability to terminate individual sessions.
- **User Account Disable**: Admin can disable user accounts (blocks login, destroys all sessions).
  Disabled users see "your account has been disabled" on login attempt. Cannot disable admins.
- **Force Logout**: Admin can force-logout all sessions for any user.
- **Session IP/UA Tracking**: Login sessions now record client IP and User-Agent for the
  active sessions view and security event context.
- **Dashboard Integration**: Security card on admin dashboard shows failed login count (24h).
  Security nav link added to admin sidebar.
- **Auth Integration**: Auth handler fires security events on login success/failure, logout,
  password reset initiation/completion. Login checks is_disabled before password verification.
- **Files**: `security_model.go`, `security_repository.go`, `security_service.go`,
  `security.templ` (new). Modified: `handler.go`, `routes.go`, `dashboard.templ`, `users.templ`,
  `auth/model.go`, `auth/service.go`, `auth/handler.go`, `auth/repository.go`,
  `layouts/app.templ`, `app/routes.go`.

### Sidebar Drill-Down Rework — COMPLETE
- **Zone-based layout**: Sidebar reorganized into 4 zones: global nav (top),
  campaign context with drill-down (middle), manage (bottom), admin (bottom).
- **Static sections**: Manage, Admin, Dashboard, All Campaigns, and Discover
  remain fixed during category drill-down — only categories area transforms.
- **Overlay approach**: Replaced 2-panel slider with absolute-positioned overlay
  that slides from right with paper-style box-shadow effect.
- **Icon-only collapse**: When drilled in, categories collapse to 48px icon strip
  with gradient shadow pseudo-element for depth effect.
- **Files**: `app.templ` (sidebar restructure), `sidebar_drill.js` (overlay logic),
  `input.css` (replaced `.sidebar-peek` with `.sidebar-icon-only` + `.sidebar-cat-active`).

### Customize Page Restructure — COMPLETE
- **Dashboard tab**: Now uses full-page flex layout matching Page Templates tab.
  Header text constrained to `max-w-3xl`, editor fills remaining space.
- **Categories tab**: 2-column desktop layout (identity left `md:w-80`, category
  dashboard right `flex-1`). Stacks on mobile. Attributes card removed.
  Width expanded to `max-w-5xl` for 2-column room.
- **Files**: `customize.templ` (dashboard/categories/extensions tabs),
  `entity_type_config.templ` (category fragment restructure).

### Extensions — Notes, Player Notes & Attributes — COMPLETE
- **Notes addon rename**: Migration 000026 renames "player-notes" to "notes" (the
  floating notebook widget). "player-notes" re-added as separate planned addon for
  future entity-page collaborative notes with real-time editing.
- **installedAddons**: `"notes"` and `"attributes"` are installed; `"player-notes"`
  is planned (not installed, no backing code yet).
- **Attributes addon**: Migration 000025 registers "attributes" addon in DB.
  Added to `installedAddons`. New `EntityTypeAttributesFragmentTmpl` template
  and `EntityTypeAttributesFragment` handler for HTMX lazy-loading.
  Extensions tab shows category selector that loads field editor per category.
- **Entity show**: Respects attributes addon enabled state. `AddonChecker`
  interface on Handler, wired via `SetAddonChecker()` in routes.go.
- **Tests**: Updated `TestIsInstalled` for both addons. All 32+ addon tests pass.

### Keyboard Shortcuts — COMPLETE
- Global shortcuts: Ctrl+N (new entity), Ctrl+E (edit entity), Ctrl+S (save).
- IIFE pattern matching `search_modal.js`. Suppresses shortcuts in inputs (except Ctrl+S).
- Save priority: `#te-save-btn` → `.chronicle-editor__btn--save.has-changes` → `form .btn-primary` → `chronicle:save` event.
- Files: `static/js/keyboard_shortcuts.js`, `base.templ` (script tag).

### Calendar Plugin Sprint 1 — COMPLETE
- **Migration 000027**: 6 tables (`calendars`, `calendar_months`, `calendar_weekdays`,
  `calendar_moons`, `calendar_seasons`, `calendar_events`). Registers "calendar" addon.
- **Model**: Domain types + DTOs. `Moon.MoonPhase()` and `MoonPhaseName()` for phase
  calculation. `Calendar.YearLength()` sums month days.
- **Repository**: Full CRUD, transactional bulk-update for sub-resources, event listing
  with recurring event support and role-based visibility filtering.
- **Service**: Validation, one-calendar-per-campaign, date advancement with month/year rollover.
- **Handler**: Setup page (create form), monthly grid view, API endpoints for settings/events/advance.
  Seeds 12 default months (30 days) and 7 default weekdays on create.
- **Templates**: `CalendarSetupPage`, `CalendarPage`, monthly grid with weekday headers,
  day cells, event chips, moon phase icons, month navigation.
- **Routes**: Owner (create, settings, advance), scribe (events), public (view).
- **Wiring**: Added to `app/routes.go` and `installedAddons` registry.

### Calendar Plugin Sprint 2 — COMPLETE
- **Migration 000028**: Leap year fields (`leap_year_every`, `leap_year_offset` on calendars,
  `leap_year_days` on months), season `color`, event end dates (`end_year`, `end_month`,
  `end_day`), event `category`, device fingerprint (`device_fingerprint`, `device_bound_at`
  on api_keys).
- **Leap years**: `IsLeapYear()`, `YearLengthForYear()`, `MonthDays()` all account for
  per-month leap year extra days. `AdvanceDate` is leap-year-aware.
- **Seasons**: `Color` field, `ContainsDate()` with wrap-around support, `SeasonForDate()`
  method, season color borders on calendar day cells.
- **Events**: Multi-day events (EndYear/EndMonth/EndDay), event categories (holiday, battle,
  quest, birthday, festival, travel, custom) with category icons.
- **Calendar settings page**: 5-tab page (General, Months, Weekdays, Moons, Seasons) with
  Alpine.js list management, JSON serialization for x-data attributes, fetch-based saves.
  Accessible via gear icon on calendar header (Owner only).
- **Event creation modal**: Alpine.js + fetch form with name, description, date, visibility,
  category, entity link, recurring flag. Quick-add button on day cell hover.
- **Entity-event reverse lookup**: HTMX lazy-loaded section on entity show pages. Calendar
  plugin serves fragment at `GET /calendar/entity-events/:eid`.
- **Sync API calendar endpoints**: Full REST API for external tools (Foundry VTT). GET/POST/
  PUT/DELETE for calendar, events, months, weekdays, moons. Advance date endpoint.
- **Device fingerprint binding**: Auto-bind on first `X-Device-Fingerprint` header, reject
  mismatches with 403 + security event logging. `BindDevice`/`UnbindDevice` on service.

### Calendar Plugin Sprint 3 — COMPLETE
- **Sidebar link**: Calendar icon link in Zone 2 (campaign context), between Dashboard
  and Categories. Gated behind `IsAddonEnabled(ctx, "calendar")`, active state highlighting.
- **Dashboard block**: `calendar_preview` block type for campaign dashboard editor. HTMX
  lazy-loaded from `GET /calendar/upcoming?limit=N`. Shows current date, season, and
  upcoming events with category icons, entity links, and date formatting.
- **Timeline view**: `GET /calendar/timeline?year=N` chronological event list grouped by
  month. Year navigation, view toggle between grid and timeline on calendar header.
  Timeline events show description, entity link, multi-day range, visibility badges.
- **New service methods**: `ListUpcomingEvents()`, `ListEventsForYear()` on service interface.
- **New repo method**: `ListUpcomingEvents()` with combined date comparison + recurring
  event handling.
- **Files**: `app.templ` (sidebar link), `campaigns/model.go` + `dashboard_blocks.templ`
  (block type), `dashboard_editor.js` (palette), `calendar/handler.go` (2 new handlers +
  TimelineViewData), `calendar/service.go` (2 new methods), `calendar/repository.go`
  (ListUpcomingEvents query), `calendar/calendar.templ` (5 new components),
  `calendar/routes.go` (2 new routes).

### Calendar Plugin Sprint 4 — COMPLETE
- **Dual-purpose event modal**: Transformed create-only modal into create/edit modal.
  Hidden `event_id` field triggers PUT (edit) vs POST (create). Title, submit button
  text/icon dynamically switch between modes.
- **Clickable event chips**: Scribe+ users see event chips as `<button>` elements with
  `data-event-*` attributes. Clicking opens the edit modal pre-filled with all event fields.
  Players still see static `<div>` chips.
- **Delete with confirmation**: Delete button visible only in edit mode. Clicking shows
  a confirmation overlay within the modal (hides the form, shows warning + confirm/cancel).
  DELETE request sent on confirm, page reloads on success.
- **Helper function**: `derefStr()` for safe nil string pointer dereferencing in templates.

### Maps Plugin Phase 1 — COMPLETE
- **Migration 000029**: `maps` table (id, campaign_id, name, description, image_id FK,
  image_width, image_height, sort_order) + `map_markers` table (id, map_id, name,
  description, x/y percentage coords, icon, color, entity_id FK, visibility, created_by).
  Addon registered as `maps` in addons table.
- **Model**: Map, Marker structs with DTOs. MapViewData, MapListData for templates.
- **Repository**: Full CRUD for maps and markers. Entity LEFT JOIN for display data.
  Visibility filtering by role (role >= 3 sees dm_only).
- **Service**: Validation, default icon/color, coordinate clamping (0-100), CRUD.
- **Handler**: Index (map list), Show (Leaflet viewer), CRUD APIs for maps and markers.
  Form-based map creation + JSON APIs.
- **Templates**: Map list page with card grid + create modal. Leaflet.js map viewer
  with CRS.Simple for image overlay. Marker create/edit modal with icon picker,
  color picker, visibility, entity linking. Map settings modal with image upload.
  Delete confirmation for markers (in-modal) and maps (confirm dialog).
- **Leaflet features**: Draggable markers (Scribe+) with silent PUT on dragend.
  Place mode (crosshair cursor, click to place). Tooltip on hover, popup for players.
  DM-only markers hidden from players via server-side filtering.
- **Wiring**: Added to `installedAddons`, `app/routes.go`, sidebar nav, admin plugin registry.

### Phase H: Inline Secrets — COMPLETE
- **TipTap secret mark**: `editor_secret.js` creates a `secret` mark via
  `TipTap.Underline.extend()`. Renders as `<span data-secret="true" class="chronicle-secret">`.
  Keyboard shortcut: `Ctrl+Shift+S`. Toolbar button with eye-slash icon.
- **Editor integration**: SecretMark added to extensions array. Toolbar button in
  text formatting group. Active state tracking. Insert via toolbar or keyboard shortcut.
- **Server-side stripping**: `sanitize.StripSecretsHTML()` regex-strips secret spans from
  HTML. `sanitize.StripSecretsJSON()` walks ProseMirror JSON tree and removes text nodes
  with secret marks. Applied in `GetEntry` handler when `role < RoleScribe`.
- **Sanitizer whitelist**: `data-secret` attribute allowed on `<span>` in bluemonday policy.
- **CSS styling**: Amber background tint, dashed bottom border, eye-slash pseudo-element
  indicator. Visible to owners/scribes, invisible to players (server-stripped).

### In Progress
- Nothing currently in progress.

### Blocked
- Nothing blocked

## Active Branch
`claude/fix-navbar-swiping-SZHP0`

## Competitive Analysis & Roadmap
Created `.ai/roadmap.md` with comprehensive comparison vs WorldAnvil, Kanka, and
LegendKeeper. Key findings:
- Chronicle is ahead on page layout editor, dashboards, self-hosting, and modern stack
- Critical gaps: Quick Search (Ctrl+K), entity hierarchy, calendar, maps, inline secrets
- Calendar is identified as a DIRE NEED — Kanka's is the gold standard
- API technical documentation needed for Foundry VTT integration
- Foundry VTT module planned in phases: notes sync → calendar sync → actor sync
- Features organized by tier: Core, Plugin, Module, Widget, External
- Revised priority phases: D (complete) → E (UX) → F (calendar/time) → G (maps) →
  H (secrets) → I (integrations) → J (visualization) → K (delight)

## Next Session Should
1. **Phase H continued:** Per-entity permissions (view/edit per role/user), group-based
   visibility (beyond everyone/dm_only), campaign export/import.
2. **Maps Phase 2 (optional):** Layers, marker groups, nested maps, fog of war.
3. **Phase E continued:** API technical documentation (OpenAPI spec or handwritten reference).
4. **Handler-level "view as player":** Extend toggle to filter is_private entities
   at repository level (currently template-only).
5. **UX polish:** Entity search typeahead for calendar event + map marker entity linking.

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
- **2026-02-20: Phase C** — Player notes widget, terminology standardization
- **2026-02-22 to 2026-02-24: Phase D** — Customization Hub (sidebar config, custom
  nav, dashboard editor, category dashboards, page layouts tab), Player Notes Overhaul
  (shared notes, edit locking, version history, rich text), Sprint 5 polish (hx-boost
  sidebar, widget lifecycle, "view as player" toggle)
