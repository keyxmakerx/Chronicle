# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-03-01 -- Rich text event descriptions with @mentions (Sprint 10). Added TipTap
rich text editor to event create/edit modal with @mention entity search support. Dual
storage: ProseMirror JSON in `description`, sanitized HTML in `description_html` (new
column via migration 000034). Timeline renders rich HTML descriptions. Legacy plain text
events handled gracefully. Calendar V2 plan (Sprints 5-10) is now fully complete.

## Current Phase
**Phase H: Secrets & Permissions.** Inline secrets complete. Documentation audit
complete. Next: per-entity permissions, campaign export/import, or Maps Phase 2.

### Migration ENUM Bug Fix — COMPLETE
- **Root cause**: Migrations 000027 (calendar) and 000029 (maps) used `INSERT INTO
  addons ... category='plugin'` but `'plugin'` was not in the ENUM. Secondary issue:
  slugs 'calendar' and 'maps' already existed from migration 000015's seed data,
  causing duplicate key conflicts hidden behind the ENUM error.
- **Fix**: Migration 000027 now ALTERs the ENUM to add `'plugin'` before any
  INSERT/UPDATE. Both 000027 and 000029 use UPDATE (not INSERT) to modify existing
  addon rows. Down migrations revert to original 000015 values instead of deleting.
- **Go code**: Added `CategoryPlugin` constant to `model.go`, added `'plugin'` to
  `validCategories` in `service.go`.
- **Safeguards**: Created `internal/database/migrate_test.go` with tests that scan
  migration SQL for invalid ENUM values. Added Migration Safety Rules to conventions.
  Added protective comments to migration 000015. Recorded ADR-017.
- **Files changed**: `000027_calendar_plugin.{up,down}.sql`,
  `000029_maps_plugin.{up,down}.sql`, `000015_create_addons.up.sql` (comment),
  `addons/model.go`, `addons/service.go`, `database/migrate_test.go` (new).

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

### Documentation Audit — COMPLETE
- **calendar/.ai.md**: Created missing plugin documentation (architecture, data model,
  routes, design decisions, sync API integration).
- **data-model.md**: Added migrations 24-29 (security_events, attributes addon,
  notes addon rename, 6 calendar tables, maps + map_markers). Updated ER diagram
  and indexes section.
- **architecture.md**: Updated directory structure with calendar plugin, maps plugin,
  sanitize package, new JS files (editor_secret, keyboard_shortcuts, search_modal,
  sidebar_drill, dashboard_editor, sidebar_nav_editor, notes). Updated plugin list.
- **tech-stack.md**: Leaflet.js marked as active (was "Phase 2").
- **decisions.md**: ADR-015 (Maps with percentage coords + Leaflet CRS.Simple),
  ADR-016 (Inline secrets via TipTap Mark extension).
- **editor/.ai.md**: Added editor_secret.js to files table, secret mark to current
  state checklist, inline secrets documentation to notes section.

### Entity Page 500 Fix — COMPLETE
- **Root cause**: `scanEntity()` in `repository.go` selected 22 columns (including
  `e.popup_config`) but only had 21 `Scan()` targets — `popup_config` was missing.
  The Go SQL driver returned `"expected 22 destination arguments in Scan, not 21"`
  for every `FindByID`/`FindBySlug` call, breaking all entity detail pages.
- **Why it was silent**: Raw `fmt.Errorf` from the SQL driver bypassed both
  `AppError` and `echo.HTTPError` checks in the error handler, falling through to
  the default 500 case. The old error handler had blind spots that didn't log these.
- **Fix**: Added `popupRaw` to the `scanEntity` scan targets and unmarshal logic,
  matching the already-correct `scanEntityRow` (used by list queries).
- **Also fixed**: Error handler now logs ALL 5xx errors (not just AppErrors with
  Internal != nil). Calendar HTMX lazy-load gated behind addon check.
- **Files**: `entities/repository.go` (scanEntity), `app/app.go` (error handler),
  `entities/handler.go` (showCalendar gate), `entities/show.templ` (conditional render).

### Calendar Block Support — COMPLETE
- **Dashboard**: `calendar_preview` block already existed in dashboard editor palette.
  Fixed route auth: moved `/calendar/upcoming` and `/calendar/entity-events/:eid` from
  RequireAuth group to AllowPublicCampaignAccess group (matching `/calendar` and
  `/calendar/timeline`). HTMX lazy-loads from the dashboard use OptionalAuth.
- **Category dashboards**: Added `calendar_preview` block type to category block palette
  (`categoryDashboardBlockTypes()`) and `CategoryBlockSwitch` renderer (`catCalendarPreview`).
- **Entity pages**: Added `calendar` block type to template editor palette and
  `entityBlockInner` renderer (`blockCalendarEvents`). Uses `layouts.IsAddonEnabled`
  for addon gating. `layoutHasBlock` helper skips hardcoded bottom section when block
  is placed in the layout to avoid duplicates.
- **Customize page**: Added calendar settings info section to Extensions tab showing
  where to add calendar blocks across the three layout editors.
- **Files**: `calendar/routes.go`, `campaigns/customize.templ`, `entities/show.templ`,
  `entities/category_blocks.templ`, `entities/entity_type_config.templ`,
  `static/js/widgets/template_editor.js`.

### Calendar Plugin Sprint 5 — Time System — COMPLETE
- **Migration 000030**: Added time columns to `calendars` (hours_per_day, minutes_per_hour,
  seconds_per_minute, current_hour, current_minute) and `calendar_events` (start_hour,
  start_minute, end_hour, end_minute). All nullable/defaulted for backwards compatibility.
- **Model**: Calendar gets HoursPerDay, MinutesPerHour, SecondsPerMinute, CurrentHour,
  CurrentMinute fields. Event gets StartHour, StartMinute, EndHour, EndMinute (*int, nullable).
  Helper methods: FormatCurrentTime(), HasTime(), FormatTime(), FormatEndTime(),
  FormatTimeRange(), IsMultiDay(). All DTOs updated with time fields.
- **Repository**: All scan/insert/update queries updated for new columns. Event listing
  now sorts by date AND time (COALESCE for null time = sort last).
- **Service**: AdvanceTime() method with minute→hour→day rollover. CreateCalendar defaults
  to 24/60/60 time system. All CRUD methods pass time fields through.
- **Handler**: AdvanceTimeAPI endpoint (POST /calendar/advance-time). UpdateCalendarAPI,
  CreateEventAPI, UpdateEventAPI all accept time fields.
- **Settings UI**: General tab now has Current Hour/Minute inputs, Time System section
  (hours/day, minutes/hour, seconds/minute). Save button sends all time fields.
- **Event modal**: "Set specific time" checkbox reveals start/end time picker (hour:minute
  inputs). Edit mode restores time from data attributes. Reset clears time fields.
- **Calendar grid**: Event chips show time prefix (e.g. "14:30 Meeting") when HasTime().
  Data attributes carry time fields for edit roundtrip.
- **Calendar header**: Shows FormatCurrentTime() next to season indicator.
- **Dashboard preview**: Shows current time alongside current date.
- **Timeline view**: Shows FormatTimeRange() on events with times.
- **Entity events**: Shows time range next to event date.
- **Sync API**: All event endpoints (create/update) accept time fields. Settings endpoint
  accepts time system fields. GetCurrentDate returns hour/minute. AdvanceTime endpoint added.
- **Routes**: advance-time route on both internal and sync API.
- **Files**: migration 000030, model.go, repository.go, service.go, handler.go, routes.go,
  calendar.templ, calendar_settings.templ, syncapi/calendar_api_handler.go, syncapi/routes.go.

### Calendar Plugin Sprint 6 — Calendar Modes + Timezone — COMPLETE
- **Migration 000031**: Added `mode` VARCHAR(20) to calendars (fantasy/reallife),
  `timezone` VARCHAR(50) to users.
- **Two calendar modes**: Fantasy (full customization, existing behavior) and Real-Life
  (Gregorian months/weekdays, synced to wall clock via user timezone).
- **Model**: `ModeFantasy`/`ModeRealLife` constants, `IsRealLife()` method, `Mode` field
  on Calendar and CreateCalendarInput.
- **Repository**: `mode` column in all calendar scan/insert queries.
- **Service**: Mode validation in CreateCalendar (defaults to "fantasy" if not "reallife").
- **Handler CreateCalendar**: Mode-aware creation — real-life seeds Gregorian months (correct
  day counts including Feb 28+1 leap), Gregorian weekdays (Sun–Sat), current date/time from
  `time.Now().UTC()`, epoch "AD", leap year every 4. Fantasy seeds generic 12×30 months.
- **Setup page redesign**: Three mode cards (Real Life / Custom Fantasy / Import Coming Soon)
  with Alpine.js mode selection. Each mode opens its own form with appropriate fields.
- **Settings page**: Months/Weekdays tabs hidden for real-life mode. Date/time/time-system/
  leap-year fields hidden for real-life mode. Info box shown instead explaining Gregorian sync.
- **User timezone**: Added `Timezone *string` to User model. Updated FindByID, FindByEmail,
  ListUsers to scan timezone. Added `UpdateTimezone` to UserRepository and AuthService.
  Validates IANA timezone via `time.LoadLocation()`.
- **Account page**: New `GET /account` settings page with timezone dropdown (curated list
  of ~65 IANA timezones). `PUT /account/timezone` API for saving. Account link added to
  app header navigation next to Logout.
- **Sync API**: GetCurrentDate response includes `mode` field. GetCalendar already returns
  full struct with mode.
- **Files**: migration 000031, calendar/model.go, repository.go, service.go, handler.go,
  calendar.templ, calendar_settings.templ, auth/model.go, auth/repository.go, auth/service.go,
  auth/handler.go, auth/routes.go, auth/account.templ (new), auth/service_test.go (mock fix),
  layouts/app.templ (account link), syncapi/calendar_api_handler.go.

### Sessions Plugin (Sprint 7) — COMPLETE
- **Migration 000032**: `sessions` table (id, campaign_id, name, summary, notes/notes_html,
  scheduled_date, calendar_year/month/day, status, sort_order, created_by), `session_attendees`
  table (session_id, user_id, status RSVP, responded_at), `session_entities` table (session_id,
  entity_id, role). Addon registered as "sessions".
- **Model**: Session, Attendee, SessionEntity structs with DTOs. Status constants (planned,
  completed, cancelled). RSVP constants (invited, accepted, declined, tentative). Entity role
  constants (mentioned, encountered, key).
- **Repository**: Full CRUD for sessions, attendees (upsert via ON DUPLICATE KEY UPDATE),
  entity linking. ListByCampaign sorts planned first, then by scheduled_date.
- **Service**: Validation (name required, max 200 chars), CRUD orchestration, InviteAll for
  batch attendee invites, UpdateRSVP with status validation, entity linking with role defaults.
- **Handler**: MemberLister interface for cross-plugin campaign member access. All HTTP handlers
  with IDOR protection (requireSessionInCampaign). Auto-invites all campaign members on create.
  HTMX-aware RSVP responses (returns AttendeeList fragment or JSON).
- **Templates**: Session list page with status grouping (Upcoming/Completed/Cancelled), detail
  page with attendee list and RSVP buttons (Going/Maybe/Can't), create modal with name/date/summary.
- **Wiring**: Added to app/routes.go, installedAddons, sidebar nav (dice-d20 icon).
- **Files**: migration 000032, sessions/model.go, repository.go, service.go, handler.go,
  routes.go, sessions.templ, sessions/.ai.md (new). Modified: app/routes.go, addons/service.go,
  layouts/app.templ.

### Eras, Weather & Setup Wizard (Sprint 8) — COMPLETE
- **Migration 000033**: `calendar_eras` table (name, start_year, end_year, description,
  color, sort_order). Added `weather_effect` VARCHAR(200) column to `calendar_seasons`.
- **Model**: Era struct with IsOngoing(), ContainsYear() methods. Calendar.Eras field,
  Calendar.CurrentEra(), Calendar.EraForYear() helpers. Season.WeatherEffect field.
  EraInput DTO.
- **Repository**: SetEras/GetEras (same bulk pattern as other sub-resources). Season
  scan/insert updated for weather_effect column.
- **Service**: SetEras validation (name required, end >= start), color defaults.
  eagerLoad includes eras.
- **Handler**: UpdateErasAPI (PUT /calendar/eras). CreateCalendar redirects fantasy
  mode to settings page for immediate customization (real-life still goes to grid).
- **Templates**: 6th tab "Eras" in settings page (name, start/end year, color,
  description). Weather effect field added to seasons editor. Era indicator shown in
  calendar header, timeline header, and dashboard preview (landmark icon + era name
  with era color). Season weather effect shown alongside season name (middot separator).
  Enhanced fantasy setup form with post-creation guidance grid showing all customizable
  features.
- **Sync API**: Added PUT /api/v1/campaigns/:id/calendar/eras endpoint.
- **Files**: migration 000033, calendar/model.go, repository.go, service.go, handler.go,
  routes.go, calendar.templ, calendar_settings.templ, syncapi/calendar_api_handler.go,
  syncapi/routes.go, calendar/.ai.md.

### Calendar Import/Export (Sprint 9) — COMPLETE
- **export.go** (NEW): Chronicle native export format (`chronicle-calendar-v1`).
  `ChronicleExport` envelope with calendar config + optional events. `BuildExport()`
  constructs export from fully-loaded Calendar struct.
- **import.go** (NEW): Auto-detection and parsing of 4 calendar JSON formats.
  `DetectAndParse()` inspects top-level JSON keys: `"format"` for Chronicle,
  `"calendar"` for Simple Calendar v1, `"exportVersion"+"calendars"` for SC v2,
  `"static_data"+"dynamic_data"` for Fantasy-Calendar, `"days.hoursPerDay"` or
  months-as-object for Calendaria. Format-specific parsers handle: 0-indexed→1-indexed
  conversion (SC), localization key stripping (Calendaria), day-of-year→month+day
  conversion (Calendaria seasons), `{values: {...}}` nesting (Calendaria), SC v1
  legacy field aliases, leap day accumulation (Fantasy-Calendar).
- **service.go**: Added `ApplyImport()` (destructive replacement of all sub-resources)
  and `ListAllEvents()` (owner-visibility for export).
- **handler.go**: 4 new handlers: `ExportCalendarAPI` (GET, JSON download),
  `ImportCalendarAPI` (POST, multipart or JSON, 10MB limit, preview mode),
  `ImportPreviewAPI` (POST, preview without applying), `ImportFromSetupAPI` (POST,
  creates new calendar + applies import + auto-enables addon).
- **routes.go**: 4 new routes (all Owner only).
- **syncapi**: `ExportCalendar` and `ImportCalendar` methods on CalendarAPIHandler.
  Routes: `GET /calendar/export` (read), `POST /calendar/import` (write).
- **calendar.templ**: Setup page import card now functional with file upload form
  (multipart, CSRF, supported formats list).
- **calendar_settings.templ**: Export download button in settings header.

### Rich Text Event Descriptions (Sprint 10) — COMPLETE
- **Migration 000034**: `ALTER TABLE calendar_events ADD COLUMN description_html TEXT`
- **model.go**: Added `DescriptionHTML *string` to Event struct and input DTOs.
  Added `HasRichText()` and `PlainDescription()` helper methods.
- **repository.go**: Updated eventCols, scanEvents, CreateEvent, UpdateEvent to
  include description_html column.
- **service.go**: Added `sanitize.HTML()` call in CreateEvent and UpdateEvent for
  rich text HTML. Imported sanitize package.
- **handler.go**: Both CreateEventAPI and UpdateEventAPI accept `description_html` in
  JSON request bodies.
- **syncapi/calendar_api_handler.go**: Both apiCreateEventRequest and
  apiUpdateEventRequest accept `description_html`.
- **export.go**: ExportEvent includes `description_html` for round-trip fidelity.
- **calendar.templ**: Replaced textarea with inline TipTap editor in event modal.
  Editor supports @mentions via Chronicle.MentionExtension. Editor initialized lazily
  on modal open, content stored as ProseMirror JSON + rendered HTML. Modal widened to
  max-w-lg. Timeline renders rich HTML via `templ.Raw()` with plain text fallback.
  Tooltip uses PlainDescription() for clean display.

### Calendar V2 Plan — FULLY COMPLETE
All 6 sprints (5-10) are complete:
- Sprint 5: Time System
- Sprint 6: Calendar Modes + Timezone
- Sprint 7: Session Scheduling + RSVP
- Sprint 8: Eras, Weather & Setup Wizard
- Sprint 9: Calendar Import/Export
- Sprint 10: Rich Text Event Descriptions + @Mentions

### In Progress
- Nothing currently in progress.

### Blocked
- Nothing blocked

## Active Branch
`claude/fix-calendar-migration-67cQE`

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
