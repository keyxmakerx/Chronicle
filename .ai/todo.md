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
- [x] **Plugin migrations not applied in Docker (entity page errors, 0/0 in DB Explorer)** — Plugin migrations used relative filesystem paths (`internal/plugins/*/migrations/`) that only resolve when CWD is the project root. In Docker, binary runs from `/app` so dirs were never found, tables never created, entity pages crashed on missing tables, and DB Explorer showed 0/0 for all plugins. Fixed by embedding migrations in the binary via Go's `embed.FS` (ADR-030). Each plugin now has `embed.go` exporting `MigrationsFS`. `PluginSchema.MigrationsDir` replaced with `MigrationsFS` (`fs.FS`).

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

- [x] **Drag-and-drop sidebar reorder fails with 403** — `sidebar_tree.js:reorderEntity()` used raw `fetch()` without CSRF token. Fixed by adding `Chronicle.getCsrf()` to headers.
- [x] **Nav menu flashes stale content on category switch** — `sidebar_drill.js` kept old content visible during HTMX fetch. Fixed with immediate loading spinner and prefetch cache usage.
- [x] **Entity template creation form broken for HTMX** — Form lacked `hx-target`/`hx-swap`, and error handler returned full page for HTMX requests. Added proper targeting, partial response, and `chronicle:notify` HX-Trigger support.
- [x] **Features page appears duplicated** — Plugin Hub (read-only) and Addon Settings (management) were separate pages. Consolidated into single `/plugins` page with inline enable/disable toggles for owners.
- [x] **Quick notes not discoverable after creation** — Toast only said "Note created" with no link. Added "View in Journal" clickable link in toast. Added `html` option to `Chronicle.notify()`.

- [x] **Unified permission model (Phase 1)** — Created `internal/permissions/role.go` with shared role constants and `CanSeeDmOnly`/`CanSetDmOnly` helpers. Replaced ~30 magic `role >= 3` checks across calendar, timeline, maps, entities, syncapi, app/routes. Fixed dm_only inconsistencies: tags now Owner-only (was Scribe+). Added Owner-only guards on dm_only creation in calendar events, timeline events, map markers. JS `permissions.js` uses named `ROLE_OWNER` constant. Permission matrix documented in `.ai/conventions.md`.
- [x] **Per-player visibility + Co-DM grants (Phase 2)** — Maps: `visibility_rules` JSON on markers/drawings with `JSON_CONTAINS` filtering. Notes: `shared_with` JSON with Private/Everyone/Specific Players UI + member picker popover. Co-DM: `DmGrantIDs` in CampaignSettings, `IsDmGranted` in CampaignContext, `VisibilityRole()` method, DM grants API + settings UI. DM-granted users see dm_only content but cannot create it.
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
- [x] **Calendar shows as pending plugin in admin** — Fixed: set calendar and API to PluginActive, added 5 missing active plugins to registry.
- [x] **False "confirm before leaving" dialog** — Fixed: added htmx:beforeRedirect listener to clear dirty form state before HTMX redirects.
- [x] **Session "Mark Complete" button fails with red error bar** — `hx-vals` sends form-encoded data but `UpdateSessionAPI` expects JSON. Replaced with `Chronicle.apiFetch()` onclick. Also fixed XSS risk from session name interpolation into JSON string.
- [x] **Image upload 500 error (ClamAV)** — Removed ClamAV entirely, added structured error logging to upload pipeline.
- [x] **Nav panel not showing new entities** — Fixed: sidebar drill panel refreshes content on navigation instead of closing.
- [x] **Shop inventory "entity type not found" in template editor** — Fixed: added missing data-campaign-id to layout editor fragment widget mount.
- [x] **Duplicate admin plugin management pages** — Removed Plugins page, renamed Addons→Features, Modules→Game Systems. Full terminology consolidation across admin and campaign pages.
- [x] **SMTP diagnostics insufficient** — Added step-by-step logging and actionable error messages to TestConnection.
- [x] **Email change feature** — Full email change with password verification, verification link to new email, session invalidation on confirm. Migration 000056.
- [x] **SMTP test should ask which email** — Added SendTestEmail endpoint with recipient input field and actionable error wrapping.
- [x] **Campaign Scan mismatch** — 3 `Scan` calls in `campaigns/repository.go` missing `ArchivedAt`/`JoinCode`, causing row scan errors.
- [x] **Media 404s on path-format fileIDs** — `normalizeMediaID()` extracts UUID from path-format fileIDs (e.g., `/media/uuid.ext` → `uuid`).
- [x] **Alpine.js load order race** — Reordered scripts so `boot.js` loads before Alpine, preventing undefined `Chronicle` references.
- [x] **Accent color CSS not applying** — `templ.Raw()` fix for inline style injection; was the root cause of accent color failures.
- [x] **Accent color JS RGB vars** — `applyAccentToPage()` now sets RGB CSS custom properties for opacity utilities.
- [x] **Device fingerprint race condition** — Atomic binding with `WHERE device_fingerprint IS NULL` prevents concurrent clobbering.
- [x] **DB TLS support missing** — Added `DB_TLS_MODE` env var + health check warning when TLS disabled in production.
- [x] **Layout studio style escaping** — `Chronicle.escapeAttr()` for colors/icons in style attributes.
- [x] **Media uploader script order** — Moved `<script>` before `x-data` element to prevent Alpine init errors.
- [x] **Save button stuck after save** — `innerHTML` reset moved before success/failure branch so button always re-enables.
- [x] **Sidebar drill reset on navigation** — Server-side `SetActivePath` override in entity handler + `LayoutInjector` check.
- [x] **Chronicle.toast stale API** — Replaced `Chronicle.toast` with `Chronicle.notify` in `media_browser.templ`.
- [x] **Hardcoded GitHub URLs** — `GITHUB_ORG` env var, generic placeholders replace hardcoded URLs.

### Recently Fixed (2026-04-12)

- [x] **Systems API `foundry_system_id` missing + no self-healing** — Removed `omitempty`, added API-level self-healing in `ListSystems()` that auto-enables system addon when campaign has selected system but addon not enabled.
- [x] **System manifest version mismatch** — Added `rewriteSystemManifestVersion()` in packages service to overwrite `manifest.json` version with release tag during install.
- [x] **Package usage query slug mismatch** — `GetUsage()` passed `pkg.Slug` not manifest ID. Added `getManifestIDFromDir()` to resolve system package slug to addon slug.
- [x] **Admin systems diagnostics** — Added Foundry System ID + Campaign Count columns to admin systems page. New `CountCampaignsUsingAddon()` in addons.

### Low (Original)

_See `.ai/audit.md` for the full feature parity & completeness audit. Audit items now organized into Phases M0-M3 and Backlog below._

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
- [x] **Calendar event drag-and-drop** — HTML5 DnD on monthly grid view. Scribe+ only.
- [x] **Calendar day view** — Single-day detailed view at `/calendar/day` with event cards, time display, day navigation. 5 tests.
- [x] **Map marker clustering** — Leaflet.markercluster integration on both map widget and full map page. Auto-clustering when >5 markers with custom cluster icons. CDN-loaded.
- [x] **Map marker icon picker** — Expanded from 18 to 39 POI icons in 8 organized groups (General, Settlements, Fortifications, Dungeons & Ruins, Nature, Maritime, Sacred & Magic, Resources).
- [x] **Recent entities sidebar** — localStorage-backed "recently viewed" list in sidebar drill panel. Tracks entity visits, renders last 10 with clock icons. `recent_entities.js`.
- [x] **Breadcrumb consistency** — Shared breadcrumb component (`components/breadcrumbs.templ`). Added to maps list/detail, timeline list/detail, sessions list/detail, calendar grid/timeline/week views.
- [x] **Editor find/replace** — Ctrl+F opens find bar, Ctrl+H opens find+replace. Match navigation, replace, replace-all. Selection-based highlighting.
- [x] **Editor code syntax highlighting** — @tiptap/extension-code-block-lowlight with highlight.js common languages. Tokyo Night-inspired dark/light theme in input.css.

### Phase K: Permissions & Competitive Gap Closers

- [x] **Sprint K-1: Per-Entity Permissions Model** — Migration 000048: `entity_permissions` table + `visibility` column on entities. Models: VisibilityMode, SubjectType, Permission, EntityPermission, EffectivePermission, SetPermissionsInput, PermissionGrant. Repository: EntityPermissionRepository (ListByEntity, SetPermissions, DeleteByEntity, GetEffectivePermission, UpdateVisibility) + visibilityFilter() SQL helper. Service: CheckEntityAccess, SetEntityPermissions, GetEntityPermissions. All list/search/count queries updated with userID param for permission-aware filtering. 13 new unit tests.
- [x] **Sprint K-2: Per-Entity Permissions UI** — Permissions widget (`permissions.js`) with three visibility modes (Everyone/DM Only/Custom), per-role and per-user grant toggles (None/View/Edit), auto-save. Replaced `is_private` checkbox on entity edit form. API: GET/PUT `/entities/:eid/permissions` (Owner only). Multi-mode visibility indicators in entity cards, category dashboard table/tree, show page title+children. Fixed sync API `GetEntity` custom visibility gap. MemberLister interface for campaign member picker.
- [x] **Sprint K-3: Group-Based Visibility** — Migration 000049 (`campaign_groups` + `campaign_group_members`), subject_type ENUM gains "group". GroupRepository (8 methods), GroupService (validation, CRUD). Group CRUD handlers + routes (Owner only). Groups management page with JS widget (`groups.js`). Permissions widget updated with group grants section. `visibilityFilter()` SQL updated for group membership check. Settings page "Groups" link. 7 unit tests.
- [x] **Sprint K-4: Auto-Linking in Editor** — Entity names API (`GET /entity-names`) with Redis caching (5-min TTL). `ListNames` repository method sorted by name length DESC. Auto-link JS module (`editor_autolink.js`) scans text nodes for entity names, creates @mention links. Insert menu item + Ctrl+Shift+L shortcut. Whole-word, case-insensitive, min 3 chars, skips existing links.
- [x] **Sprint K-5: Relations Graph Visualization** — D3.js force-directed graph (`relation_graph.js`) with dynamic CDN loading, zoom/pan, drag, node coloring, tooltips, legend. Backend: `ListByCampaign` repo (dedup bi-directional), `GetGraphData` service, `GraphAPI`/`GraphPage` handlers. Dashboard block `relations_graph` with configurable height. Standalone page at `/relations-graph/page`.
- [x] **Sprint K-6: Foundry Polish Sprint** — Fixed shop icon null (dead ternary, HBS FA icon rendering). Fog bidirectional sync (dark polygon detection, pixel→percentage conversion, REST push). Connection status UI (event-driven onStateChange, click-to-reconnect, activity flash). SimpleCalendar CRUD hooks (journal hook listeners, SC flag detection, calendar event push).

### Phase L: Content Depth & Editor Power

- [x] **Sprint L-1: Entity Sub-Notes (Posts) UI** — Migration 000050 (`entity_posts` table). Full posts widget: PostRepository (CRUD + reorder), PostService (validation, sort order), Handler (list/create/update/delete/reorder). JS widget (`entity_posts.js`) with collapsible post cards, drag-to-reorder, visibility toggle, inline rename, delete confirmation. Integrated into entity show page. Layout block type `posts` in template editor. Public-capable read route, Scribe+ write routes. 13 unit tests.
- [x] **Sprint L-2: Notes Rich Text (TipTap)** — Replaced plain textarea editing with mini TipTap editor instances. StarterKit+Underline+Placeholder. Saves entry JSON + entryHtml to API. Legacy text block→TipTap HTML conversion on first edit. Checklists remain as interactive checkboxes. Editor instances tracked and cleaned up on save/destroy.
- [x] **Sprint L-3: Note Folders and Organization** — Migration 000051: `parent_id` + `is_folder`. Tree view in notes panel, collapsible folders, move-to-folder dropdown, create folder button. 4 tests.
- [x] **Sprint L-4: Calendar Event Drag-and-Drop** — HTML5 DnD on monthly grid. Event chips draggable (Scribe+), day cells as drop zones. Full PUT on drop with all event fields preserved. Drop zone highlighting via `cal-drop-highlight` CSS. No backend changes needed.
- [x] **Sprint L-5: Calendar Day View** — Single-day detailed view at `/calendar/day`. DayViewData struct with PrevDay/NextDay/WeekdayName/Season helpers. Event cards with time, category, entity links, description. Day view icon in all view toggles. Sessions support. 5 unit tests.

### Phase M0: Data Integrity & Export Completeness ← START HERE

_Fix export/import so backups don't lose data. Highest-priority work._

- [x] **Sprint M0-1: Export Adapters — Permissions, Groups, Posts** — Added ExportEntityPermission, ExportGroup, ExportPost types. Entity export now includes visibility mode and custom permission grants. Group export/import adapter (ListGroups + ListGroupMembers, CreateGroup + AddGroupMember). Post export/import adapter (iterates entities, ListByEntity for posts). Wired in routes.go. All tests pass, zero lint issues.
- [x] **Sprint M0-2: Export Adapters — Timeline & Sessions** — Timeline export now includes event connections (index-based references), entity groups with member slugs. Session export now includes attendee RSVP statuses. Import side creates connections, entity groups with members, and attendees with status restoration.
- [x] **Sprint M0-3: Import Parent Hierarchy Fix** — Added second pass after entity creation that resolves ParentSlug → new entity ID and calls Update to set parent_id. All entities exist before parent resolution runs.
- [x] **Sprint M0-4: Relations Visibility Controls** — Migration 000052 (dm_only column). Full stack: model, repository (all queries), service (Create variadic, GetGraphData filtering), handler (DM auth check, role-based filtering in List/Graph), export/import (DmOnly field), JS widget (toggle + lock badge), template (data-is-dm attribute).

### Phase M1: Quick Wins Sprint

_High-impact, low-effort items that immediately improve the user experience._

- [x] **Sprint M1-1: Account & Settings Quick Wins** — Export/Import button on campaign settings page. In-app password change. Display name editing. Theme persistence.
- [x] **Sprint M1-2: Entity List & Sidebar Quick Wins** — Entity list sort controls. Entity favorites/bookmarks. Notes search/filter. Calendar event search/filter.
- [x] **Sprint M1-3: Session & Member Quick Wins** — Session recap field (migration 000053). Avatar upload UI. Character assignment (migration 000054). Export adapter wiring.
- [x] **Sprint M1-4: Session RSVP Sidebar Visibility** — HTMX lazy-loaded sidebar section with planned sessions, RSVP status dots, attendee counts. Collapsible with localStorage. Members-only, calendar addon required.
- [x] **Sprint M1-5: Plugin Hub Page in Settings** — `/campaigns/:id/plugins` page with enabled addon cards (icon, name, description, quick link). Disabled addon list for owners. AddonLister adapter pattern. Sidebar "Plugins" link.

### Phase M2: JS Code Quality

_Consistency and reliability across all JS widgets._

- [x] **Sprint M2-1: apiFetch Migration & Utility Dedup** — Migrated all raw fetch() to Chronicle.apiFetch() across 9 widgets. Removed local escHtml/escAttr/apiFetch from groups.js, entity_posts.js, relation_graph.js, favorites.js, recent_entities.js (-290 lines).
- [x] **Sprint M2-2: Error Handling — Toast on Failure** — Added Chronicle.notify() toasts to catch blocks in notes, tag_picker, relations, relation_graph, editor, permissions, search_modal, editor_autolink. Tooltip/mention use console.warn only (too noisy for toasts).

### Phase M3: Test Coverage

_Fill the biggest test gaps — zero-test plugins and incomplete service tests._

- [x] **Sprint M3-1: Maps Service Tests** — 45 tests covering maps/markers CRUD, validation, XSS prevention (icon/color patterns), coordinate boundary checks, search results formatting.
- [x] **Sprint M3-2: Sessions & Calendar Service Tests** — Sessions: 40+ tests (CRUD, recurrence, RSVP, entity linking, RSVP tokens, model methods). Calendar: 40+ tests (calendar CRUD, events, date helpers, week view).
- [x] **Sprint M3-3: Timeline Service Tests** — 50+ tests covering timeline CRUD, standalone events, entity groups, event connections, search, visibility filtering, event linking.

### Phase S: Data Integrity & Admin Tooling (COMPLETE)

_Fix orphaned data, cascade gaps, and admin DB visibility. See `.ai/phases.md`._

- [x] **Sprint S-1: Campaign Deletion Cleanup (ADR-025)** — API key FK cascade, media file disk cleanup, multi-step delete service, WASM hook dispatch, orphan extension cleanup.
- [x] **Sprint S-2: Extension Migration System (ADR-024)** — `extension_schema_versions` table, per-extension migration runner, namespaced table enforcement (`ext_<slug>_*`), install/uninstall lifecycle.
- [x] **Sprint S-3: Admin Data Hygiene Dashboard (ADR-026)** — Orphan detection queries, `/admin/data-hygiene` page, guarded cleanup actions, safety guardrails. All actions audit-logged.

### Phase T: Game System Modules & Worldbuilding Tools

- [x] **Sprint T-0 (M-1): D&D 5e Module — Data & Tooltip API** — SRD-legal JSON (spells 27, monsters 14, items 10, classes 12, races 9, conditions 15). Module init wiring, route registration, category-specific tooltip rendering, 9 tests.
- [x] **Sprint T-1: D&D 5e Module — Reference Pages** — Browsable pages at `/modules/dnd5e/`. Category cards, searchable lists, formatted stat block detail pages. Quick-search (Ctrl+K) integration via ModuleSearchAdapter wiring into entity SearchAPI.
- [x] **Sprint T-2: Pathfinder 2e Module** — ORC-licensed data (82 items across 6 categories: spells, creatures, equipment, ancestries, classes, conditions). GenericModule auto-instantiation, no custom Go code needed.
- [x] **Sprint T-3: Guided Worldbuilding Prompts** — Migration 000008 (`worldbuilding_prompts` table). `WorldbuildingPromptRepository` with full CRUD + type filtering. `WorldbuildingPromptService` with validation, `EntityTypeLister` interface for seeding, `SeedDefaults()` inserts 16 prompts across 5 entity types (character: 4, location: 4, organization: 3, item: 2, quest: 3). REST API (GET Player+, POST/PUT/DELETE Owner). `WorldbuildingPromptsPanel` templ component with HTMX lazy-load (`hx-trigger="revealed"`) and `<details>` accordion. Shown on entity show page for Scribe+. Seeded automatically on campaign creation via `WorldbuildingPromptSeeder` interface. 7 unit tests.
- [x] **Sprint T-4a: Layout Presets** — Reusable page layout presets (`layout_presets` table, migration 000017). Full CRUD API, 4 built-in presets seeded on campaign creation, template editor "Load Preset" / "Save as Preset" UI. Security hardening: IDOR fix on content template application, JSON validation, size limits, structured logging.
- [ ] **Sprint T-4b: Entity Type Template Library** — Genre presets (fantasy, sci-fi, horror, modern, historical) as JSON fixtures. Campaign creation genre selection. "Import preset" in Customization Hub.

### Phase U: Collaboration & Platform Maturity

- [x] **Sprint U-1: Role-Aware Dashboards** — Two-dashboard architecture implemented: Campaign Page (public, `/campaigns/:id`) and Owner Dashboard (owner-only, `/campaigns/:id/dashboard`). Both independently customizable via Customization Hub. Migration 000006 adds `owner_dashboard_layout` column. Role selector in dashboard editor: `RoleDashboardLayouts` struct with backward-compatible JSON format (detects legacy bare `{"rows":[...]}` vs role-keyed `{"default":...,"player":...,"scribe":...}`). Alpine.js role toggle (Default/Player/Scribe) in customize.templ. `dashboard_editor.js` appends `?role=` to GET/PUT/DELETE endpoints, listens for `role-change` events. Handler merges role-specific layout via `SetRoleDashboardJSON`. `ParseRoleDashboardLayout(role)` returns role-specific layout with fallback to default. 9 unit tests for model methods.
- [x] **Sprint U-2: Invite System** — Migration 000007 (`campaign_invites` table). InviteRepository, InviteService, InviteHandler. Email invitations with one-click accept link via `/invites/accept?token=xxx`. Invite management UI in campaign settings (HTMX lazy-loaded). Send form with email + role selector. Invite table with status badges (pending/accepted/expired) and revoke button. HTML+plaintext email template. Login/register redirect support (`?redirect=` param). 9 unit tests.
- [ ] **Sprint U-3: 2FA/TOTP Support** — TOTP enrollment with QR code (`pquerna/otp`). Login redirect to TOTP input. Recovery codes (8 hashed). Admin force-disable.
- [ ] **Sprint U-4: Accessibility Audit (WCAG 2.1 AA)** — ARIA labels, focus traps, skip-to-content, color contrast 4.5:1, keyboard nav, screen reader announcements, axe-core scanning.
- [ ] **Sprint U-5: Infrastructure & Deployment** — Docker-compose full stack verification with health checks. Makefile full-stack target. `CONTRIBUTING.md`. CI against docker-compose.

### Phase V: Obsidian-Style Notes & Discovery (COMPLETE except V-4)

_Quick capture, backlinks, enhanced graph, editor power-ups. See `.ai/obsidian-notes-plan.md` and `.ai/competitive-gap-analysis.md`._

- [x] **Sprint V-1: Quick Capture, Session Journal & Slash Commands** — Quick-capture modal (Ctrl+Shift+N), "Session Journal" topbar button, player notes in quick search. TipTap slash command menu (`/` trigger) for headings, lists, callouts, tables, images, code blocks.
- [x] **Sprint V-1.5: Inline Secrets / DM-Only Blocks in Editor** — SecretMark TipTap extension (Ctrl+Shift+S toggle), `sanitize.StripSecretsHTML/JSON` server-side stripping for non-Scribe, `.chronicle-secret` CSS with amber eye-slash icon, toolbar button. Already implemented alongside V-1.
- [x] **Sprint V-2: Backlinks Panel & Entity Aliases** — HTMX lazy-loaded backlinks with Redis caching + context snippets. Entity aliases table (migration 061), alias CRUD API, aliases widget (tag chips), search/auto-linker/mention integration via `ListNames()` UNION. 11 new tests.
- [x] **Full-Page Journal & Category Nav Redesign** — Full-page journal view at `/campaigns/:id/journal` with two-panel Obsidian-like layout (note tree sidebar + TipTap editor, search, folder tabs, autosave). Journal link in sidebar nav (gated on notes addon). Notes FAB hidden on journal page to avoid sync conflicts. Removed "Session Journal" topbar button. Redesigned sidebar drill panel: merged header + action bar into compact clickable row (category name opens full page, replacing "View All"), count as pill badge, inline "+" button. Entity tree: folder/page icons, guide lines, smooth transitions, distinct reorder vs reparent D&D feedback.
- [x] **Sprint V-3: Content Templates** — `content_templates` table, ContentTemplateService with CRUD + seeding, REST API, template picker on entity create form (loads by entity type), "Insert Template" slash command in editor, 4 default templates (Session Recap, NPC Profile, Location, Quest Log) seeded on campaign creation, Customization Hub "Content Templates" tab for management.
- [~] **Sprint V-4: Enhanced Graph View & Cover Images** — @mention links in graph ✅, entity type filtering ✅, tag filtering (deferred — needs service plumbing), local graph (N hops) ✅, clustering ✅, orphan detection ✅, PNG export ✅. Cover/banner image layout block type ✅ (migration 000004, API, block registry, upload UI). Remaining: tag-based filtering on graph (requires TagEntityLister adapter).
- [x] **Sprint V-5: Session Journal Audio Attachments** — `note_attachments` table (migration 000005), `AttachmentRepository` + `AttachmentService` + REST handlers (list/upload/delete/transcript). Media service extended with audio MIME types (mp3/ogg/wav/webm) and magic bytes validation; `sanitizeImage()` guarded to skip audio. Journal UI: microphone upload button in toolbar, inline `<audio>` players per attachment, collapsible editable transcript textarea, delete support. Also fixed: journal save bug (`_tiptapBundle` → `window.TipTap`), added @mentions (MentionLink + MentionExtension) to journal editor, added session edit modal UI.

### Phase W: Polish, Ecosystem & Delight

- [x] **Sprint W-0: Nav Menu Reorg Mode** — Small icon button near Dashboard in sidebar. Click to enter reorg mode for current level (categories or entities). Category level: drag to reorder category icons. Entity level: drag to reorder, create folders/submenus. Click again to exit reorg mode. Must work on desktop, tablet, and mobile. Button is context-aware: on base nav, reorders categories; drilled into a category, reorders entities.
- [x] **Sprint W-0.5: Owner Visual Customization** — Change "Chronicle" brand name per-campaign with optional image/logo. Top bar color/gradient/animation/background image (responsive). Visual customization editor with faux site outline (editable boxes for colors/backgrounds). Appearance-only, not layout editing. Accent color CSS variable now propagates to all Tailwind utilities via `var(--color-accent)` references. Auto-computed hover/light variants.
- [x] **Sprint W-0.75: Topbar Redesign** — 7-step redesign: compact h-14 layout with user avatar dropdown and icon row, inline search with magnifying glass + slide-up + live results, notes icon replacing floating FAB, TopbarContent data model + PUT API, quick-links and quote rendering in center area, dedicated `/campaigns/:id/search` page with live filtering and entity type tabs, topbar content editor in Appearance settings.
- [x] **Sprint W-1: Command Palette & Saved Filters** — Command palette done: `static/js/command_palette.js` (~280 lines) with Ctrl+Shift+P trigger, context-aware commands (campaign navigation, actions, universal), fuzzy substring matching, keyboard navigation, `Chronicle.openCommandPalette()`/`closeCommandPalette()` API. Added to `base.templ` and `shortcuts_help.js`. Saved filter presets done in Sprint N-8 (`saved_filters` table, CRUD API, sidebar_tag_filter.js preset UI).
- [ ] **Sprint W-2: Map Drawing Tools, Regions & Measurement** — Leaflet.Draw integration (freehand, polygons, circles, rectangles, text). Uses existing `map_drawings` table. Per-drawing visibility, color/opacity. Also: map regions (polygon fills/strokes/labels), measurement/distance tool, map embed layout block for entity pages.
- [ ] **Sprint W-2.5: Nested / Linked Maps** — Click marker to open sub-map. `linked_map_id` on markers. Breadcrumb navigation between map levels. Competitive gap vs World Anvil/LegendKeeper.
- [ ] **Sprint W-3: Discord Bot Integration** — Plugin at `internal/plugins/discord/`. Bot token config. Webhook session notifications. Reaction-based RSVP per ADR-012.
- [~] **Sprint W-4: Bulk Operations & Persistent Filters** — Multi-select in sidebar reorg mode done (Ctrl+click, floating action bar, bulk-move API). Remaining: multi-select on entity list page, batch tag, batch visibility, batch delete. Persistent filters per category in localStorage. Entity tag/field filtering on list pages.
- [ ] **Sprint W-5: Editor Import/Export & Additional Themes** — Markdown import/export via `goldmark`. Sepia + high-contrast themes. Custom accent color picker. Embed media blocks (video/audio URLs) in editor.
- [ ] **Sprint W-6: Timeline List View & Meter Blocks** — Simple chronological list view alongside D3 viz. Meter/tracker layout block type for numeric values (HP, spell slots) with bar/circle/dot styles.

### Phase N: Sidebar Navigation Overhaul (COMPLETE — ADR-032)

_Comprehensive sidebar navigation rework. Replaces folder-entity hack, adds
favorites, unified sidebar model, and large campaign support._

- [x] **Sprint N-1: Pure Folders** — `sidebar_nodes` table, `parent_node_id` on entities, migration 000013 (data migration from is_folder), CRUD API, sidebar_tree.js dual-source tree builder.
- [x] **Sprint N-2: DB-Backed Favorites** — `entity_favorites` table (migration 000014), toggle/list/check API, favorites.js rewritten for API with cache.
- [x] **Sprint N-3: Unified Sidebar Model** — `SidebarItem` type in sidebar_config JSON, template renders owner-defined order, `sidebar_layout_editor.js` drag-and-drop widget in Customize Hub.
- [x] **Sprint N-4: Large Campaign Support** — Tag filtering (?tags= AND-logic), lazy loading (50/page + IntersectionObserver), multi-select bulk move (Ctrl+click + floating bar + POST /entities/bulk-move), collapsible Manage section.
- [x] **Sprint N-5: Folder UX Completion** — Inline rename (double-click), folder visibility toggles (HiddenNodeIDs), right-click context menu (Rename/Delete), fixed wrapper data attributes, skipped redundant div→div conversion.
- [x] **Sprint N-6: Layout Editor Polish** — Auto-inject missing addons/entity types into items array on config load.
- [x] **Sprint N-8: Saved Filter Presets** — Migration 000015, SavedFilterRepository CRUD, REST API, sidebar_tag_filter.js preset UI (save/apply/delete).
- [x] **Responsive/Mobile Fixes** — Touch drag for layout editor, WCAG touch targets, long-press context menu, inline rename mobile improvements.

### Backlog: Integrations Tab Redesign (Deferred)

- [ ] **Integrations tab inline API keys** — Move API key management from separate page into campaign settings Integrations tab. Connection details card (Server URL + Campaign ID + API Key with copy buttons). Connection status indicators (green/amber/red per key). VTT setup instructions (collapsible per-VTT). Clear sync mappings button. CORS note. Data export/import section. **Security: owner-only with backend auth checks on every endpoint.** Full design documented in `/root/.claude/plans/zippy-scribbling-backus.md`.

### Backlog: Remaining Audit Items (address opportunistically)

_Lower-priority items to pick up during related sprints or as standalone tasks._

**UI Consistency:**
- [ ] **Alert styling inconsistent** — login.templ and entities/form.templ use inline Tailwind instead of alert-success/alert-error classes.
- [ ] **Admin pagination inline** — admin/users.templ and admin/campaigns.templ have hand-rolled pagination instead of using components.Pagination.
- [ ] **Modal approach mixed** — Sessions uses dialog element; calendar/other modals use Alpine.js. Should standardize.
- [ ] **Rate limiting on mutations** — Campaign/entity/widget mutation endpoints have no rate limiting (auth + media do).
- [ ] **Recurring calendar events (beyond yearly)** — Sessions support weekly/biweekly/monthly, but calendar events only support yearly.

**Documentation:**
- [ ] **Posts widget missing .ai.md** — Only Go widget without documentation file.
- [ ] **12 JS widgets missing .ai.md** — calendar_widget, map_widget, relation_graph, entity_type_config, entity_type_editor, groups, permissions, shop_inventory, timeline_widget, entity_posts, notifications, shortcuts_help. (sidebar_tree, sidebar_reorg, sidebar_tag_filter, sidebar_layout_editor now have .ai.md files.)

**Player & DM Experience Gaps:**
- [ ] **Entity tag/field filtering** — Entity list only has type tabs. No filter by tag, custom field value, or visibility mode.
- [ ] **Entity print/PDF export** — No per-entity print stylesheet or PDF generation.
- [ ] **Share link for entities** — Campaign-level public mode exists but no per-entity shareable links.
- [ ] **Soft delete / entity archive** — Entities are hard-deleted only. Add `archived_at` column or trash/recycle bin pattern.
- [ ] **Map measurement tool** — Can't measure distance between markers. Leaflet supports this via plugins.
- [ ] **Map fog of war native UI** — Backend exists for Foundry sync but no Chronicle-native fog controls.
- [ ] **Initiative tracker** — No combat ordering tool for session management.
- [ ] **Session prep checklist** — No per-session task list for DM prep items.
- [ ] **NPC quick generator** — Random name/trait generator for improvisation.
- [ ] **Account deletion** — No self-service account removal option.
- [ ] **Member activity tracking** — No last-seen, activity feed, or engagement metrics.
- [ ] **Timeline search/filter** — No search within timeline events by name/text.
- [ ] **Timeline zoom-to-era** — No button to jump viewport to a specific era.
- [ ] **Entity version history UI** — Audit log exists but no "view diff / restore version" for entities.
- [ ] **Toast notification grouping** — Duplicate toasts stack separately instead of grouping.
- [ ] **Entity image gallery** — Only one image per entity; no carousel/gallery for multiple images.

### Phase P: Extension System (Content Extensions — Layer 1)

_Declarative content packs: no code execution, manifest-only. See ADR-021._

- [ ] **Sprint P-1: Extension Infrastructure** — Migration (extensions, campaign_extensions, extension_records, extension_assets tables). Extension model/repository/service. Manifest parser + validator. Zip installer with security checks (file type allowlist, path traversal prevention, SVG sanitization, size limits).
- [ ] **Sprint P-2: Admin Extension Management** — Admin UI for listing/installing/uninstalling extensions. `GET/POST/DELETE /admin/extensions`. Extension detail page showing manifest metadata. On-disk storage in `extensions/` directory.
- [ ] **Sprint P-3: Campaign Extension Enable/Disable** — Campaign settings "Extensions" tab. `GET/POST/DELETE /campaigns/:id/extensions/:ext_id`. Preview endpoint showing what enabling will do. Addon requirement checking.
- [ ] **Sprint P-4: Content Appliers** — Calendar preset applier (replaces calendar config). Entity type template applier (creates entity type). Entity preset applier (creates entities). Tag collection applier (merge). Provenance tracking in extension_records for clean uninstall.
- [ ] **Sprint P-5: Marker Icons & Themes** — Marker icon pack registration (namespaced IDs). Theme variant registration (CSS custom property overrides). Asset serving endpoint (`GET /extensions/:ext_id/assets/*path`).
- [ ] **Sprint P-6: Example Extensions** — Forgotten Realms Calendar (Harptos) pack. D&D 5e Character Sheet entity type template. Sample monster pack. Package as reference implementations for extension authors.

### Phase Q: Extension System (Widget Extensions — Layer 2)

_Browser-sandboxed JS widgets that extend the UI. See ADR-021._

- [ ] **Sprint Q-1: Widget Extension API** — `Chronicle.registerWidget(name, {mount, unmount, config})` API in boot.js. Extension widget discovery and auto-mounting. Widget config schema in manifest.
- [ ] **Sprint Q-2: Widget Extension Distribution** — Allow `.js` files in extension zips (scoped to widget registration pattern). Extension widget blocks appear in template editor palette.

### Phase R: Extension System (Logic Extensions — Layer 3/WASM)

_WASM-sandboxed backend logic via Extism/wazero. See ADR-021._

- [x] **Sprint R-1: WASM Runtime Integration** — Extism Go SDK v1.7.1 + wazero v1.9.0. PluginManager (load/unload/reload/call). 10 read-only host functions across 5 capability groups (log, entity_read, calendar_read, tag_read, kv_store). Per-plugin KV store via extension_data. WASMHandler with admin + campaign endpoints. HookDispatcher with 8 event types. Manifest integration with validation. 26 tests.
- [x] **Sprint R-2: App Wiring & Admin UI** — Closure-based WASM adapters (EntityReader/CalendarReader/TagReader). Full app/routes.go wiring with PluginManager, HookDispatcher, WASMHandler. Auto-load WASM plugins on extension enable. Graceful shutdown. Admin UI contributes section shows widgets + WASM plugins. 12 new tests (38 total).
- [x] **Sprint R-3: Write Host Functions** — 6 write host functions (update_entity_fields, create_event, set_entity_tags, get_entity_tags, create_relation, send_message). 5 new capabilities. 4 write adapters. Plugin-to-plugin async messaging. 10 new tests (48 total).
- [x] **Sprint R-4: Plugin SDK & Developer Tools** — Example WASM plugins (Rust auto-tagger, Go session-logger). Go SDK with MockHost test harness (9 tests). Plugin development guide. 7 new manifest tests. **Phase R complete.**

### Phase F: Foundry Sync Enhancements & Character Integration ✓ (F-1 through F-7 COMPLETE)

_Improve Foundry VTT sync fidelity. Add system-aware character sheet sync. Build toward inventory/NPC features._

- [x] **Sprint F-1: Journal Sync Fidelity** — Multi-page journal sync (split entity `entry_html` by headings into Foundry pages, concatenate pages back on Foundry→Chronicle). Ownership change hook (detect ownership changes in `updateJournalEntry` hook, push to Chronicle). Helpers: `_splitByHeadings`, `_collectTextPages`, `_syncPagesToJournal`.
- [x] **Sprint F-2: Granular Permission Mapping** — Map Chronicle `visibility: 'custom'` + `entity_permissions` to Foundry per-user ownership levels (view→OBSERVER, edit→OWNER). New syncapi endpoints: `GET /entities/:eid/permissions`, `PUT /entities/:eid/permissions`. Reverse-map Foundry ownership changes back to Chronicle. Helpers: `_buildOwnership`, `_pushPermissions`. User-specific grants stored in flags but not mapped to Foundry users (requires user ID mapping table — deferred).
- [x] **Sprint F-3: System Detection & Character Field Templates** — Expanded dnd5e character preset (15 fields: class, level, race, alignment + 6 ability scores + HP/AC/speed/proficiency). Added pf2e character preset (15 fields: class, level, ancestry, heritage + 6 ability mods + HP/AC/perception/speed). `CharacterPreset()` helper on `SystemManifest`. New `GET /api/v1/campaigns/:id/systems` endpoint returns available systems with enabled flag. Foundry module: `syncCharacters` + `detectedSystem` settings, `SYSTEM_MAP` table, `_detectSystem()` on start, `getMatchedSystem()` accessor. Dashboard Status tab shows system match info and character sync availability.
- [x] **Sprint F-4: Actor ↔ Entity Sync** — `actor-sync.mjs` with bidirectional Actor ↔ entity sync. System adapters: `dnd5e-adapter.mjs` (15 fields), `pf2e-adapter.mjs` (HP/name back only). Dashboard Characters tab with Push button. Registered in module.mjs. TESTING.md updated. **Note: adapters and SYSTEM_MAP are hardcoded — see F-4.5.**
- [x] **Sprint F-4.5: Generic System Adapter & Dynamic Matching** — Added `foundry_system_id` to manifest, `foundry_path`/`foundry_writable` to FieldDef. New `GET /systems/:id/character-fields` API. `_detectSystem()` now API-driven (matches by `foundry_system_id`). New `generic-adapter.mjs` auto-generates field mappings from API. dnd5e (15 fields), pf2e (15 fields, most read-only), drawsteel annotated. actor-sync.mjs falls back to generic adapter. 7 new tests.
- [x] **Sprint F-QoL: Foundry Sync Diagnostics & Error Handling** — `ValidationReport` type with `BuildValidationReport()` analyzing categories, fields, presets, Foundry compatibility, warnings. Templ component shows capability badges + warnings after upload. API client health metrics (success/error counts, uptime, reconnect attempts). Structured error log. Retry queue for failed writes (processes on reconnect, max 3 retries). Dashboard Status tab: diagnostics grid, error log, field mapping debug info. 3 new tests.
- [x] **Sprint F-5: NPC Viewer / Hall (Plugin + Widget + Foundry Sync)** — Full NPC plugin at `internal/plugins/npcs/` with handler/service/repo/templates. Campaign route `/campaigns/:id/npcs` — gallery/grid of revealed NPCs (non-private character entities). Portrait, name, type label, tags. Search/sort/pagination. "Reveal" = Scribe toggles `is_private` via eye icon. `npc_gallery` layout block type for entity pages and dashboards. Foundry sync: NPC visibility changes sync bidirectionally — Chronicle `is_private` ↔ Foundry `prototypeToken.hidden`. REST API endpoint `POST /entities/:id/reveal`. 7 tests.
- [x] **Sprint F-6: Armory / Inventory System** — Items as entities with game-mechanic fields (weight, cost, rarity, damage, properties). Character "Inventory" tab/block via entity relations. Relation metadata: equipped, quantity, attunement. System-specific item templates (dnd5e ≠ pf2e). Foundry sync: Actor inventory ↔ Chronicle inventory relations. "Armory" campaign page showing all catalogued items. **Implemented across 5 sub-sprints:**
  - B-1: System-dependent item presets (`preset_category` column, `RelationPresetDef`, item fields API, manifest changes for dnd5e/pf2e/drawsteel).
  - B-2: Armory plugin (`internal/plugins/armory/`) — gallery page, sidebar link, dashboard block, addon gating.
  - B-3: Inventory block + Foundry item sync (`blockInventory`, `inventory.js` widget, `item-sync.mjs` Foundry module).
- [x] **Sprint F-7: Shop / Marketplace Enhancement** — Transaction logging (who bought what, when). Currency tracking per character. Stock management (auto-deplete on purchase). Foundry: purchase from shop window → update character inventory on both sides. **Implemented across 2 sub-sprints:**
  - C-1: `shop_transactions` table, transaction service with stock validation, currency fields on character presets, REST endpoints.
  - C-2: Transaction history in `shop_inventory.js`, `transaction_log.js` widget + block, Foundry shop-widget purchase flow.

### Phase X: System Modularity & Owner Experience

_Validate the full owner pipeline: upload custom system → enable → get presets,
tooltips, Foundry sync, widgets, character sheets. Ensure the system framework
is truly modular and self-service._

- [x] **Sprint X-0: Sandbox Hardening & Trust Policy** — Field type enum whitelist in `ValidateManifest()`. Slug format regex validation. Content limits (categories, fields, presets). HTML sanitization of manifest strings. `OwnerUploadPolicy` setting (`auto_approve`/`require_approval`/`disabled`) in `PackageSecuritySettings`. Security settings page now saveable with POST handler.
- [x] **Sprint X-1: System Upload UX & Validation** — Two-step upload flow: upload ZIP → preview (dry-run) → confirm install. `PreviewFromZIP()` validates in memory without writing to disk. `BuildImpactTree()` creates nested tree structure for visual diagram. `impact_tree.js` renders collapsible tree widget. Preview template shows categories with item counts/samples, entity presets with field tables, Foundry compatibility, warnings/errors. Upload policy gating (disabled/auto_approve/require_approval).
- [x] **Sprint X-2: System-Provided Entity Presets & Auto-Setup** — `PresetApplier` interface in addons package. `EnableForCampaign()` auto-creates entity types from system manifest's `entity_presets`. Skips categories that already exist. Graceful degradation on failure. Wired via `app/preset_applier.go`.
- [x] **Sprint X-3: System-Provided Widgets & Layout Blocks** — `WidgetDef` in manifest with slug/name/icon/description/script_file. Validation (max 10, slug format, .js ext, path traversal). `WidgetScriptAPI` serves JS from system dir. `GetSystemWidgetBlockMetas`/`GetSystemWidgetScriptURLs` on `SystemHandler`. Reuses `ext_widget` block type. `widgetBlockListerAdapter` extended. LayoutInjector script injection. Diagnostics page widget section. No hardcoded widgets — all from system packages.
- [x] **Sprint X-4: System Debugging & Diagnostics** — `/campaigns/:id/systems/status` page with system info, stats grid, categories, entity presets, Foundry compatibility, validation warnings. `diagnostics.templ` template. `SystemStatus()` handler on `CampaignSystemHandler`.
- [ ] **Sprint X-5: System-Provided Character Sheet Widgets** — Character sheets are system-authored, not Chronicle core. Each system package ships a widget JS file (via existing `ext_widget` block type from X-3) that reads entity attributes and renders a styled character sheet. Manifest gains `character_sheet` section defining `field_groups` (visual groupings like "Ability Scores", "Combat Stats") with layout hints (grid columns, row spans). D&D 5e gets classic 5e-style layout, PF2e gets PF2e-style, etc. No new block type needed — reuses system widget infrastructure. Chronicle core provides mounting point + attributes API only.

### Phase A-2: Armory Multi-Instance

_Support multiple named inventory collections per campaign. Current armory is a single campaign-wide view._

- [x] **Sprint A2-1: Inventory Instances** — Migration 000011: `inventory_instances` + `inventory_items` tables + `instance_id` on `shop_transactions`. `InstanceRepository`, `InstanceService`, `InstanceHandler` with full CRUD. Gallery page instance dropdown + manage panel. Transactions scoped to instances.
- [ ] **Sprint A2-2: Instance UI Polish** — Add/remove items UI on instance view, drag-and-drop reorder, instance description editing, Foundry sync per-instance.

### Entity Manager Widget

_A drag-and-droppable block for entity/category/dashboard pages showing entities from a selected category with sorting, tag filtering, folder creation, and visibility toggles._

- [x] **Entity Manager Block** — Register `entity_manager` block type. Templ component in `block_entity_manager.templ`. JS widget with search, tag filtering, sort, drag reorder, visibility toggle, folder creation, tree rendering. Reparenting support.
- [x] **Sidebar Hidden Entities** — `hidden_entity_ids[]` in sidebar-config. Filtered for players, grayed for owners. Toggle via entity manager widget and sidebar reorg mode.

### Needs Discussion (Deferred)

- [ ] **Sessions** — Need to discuss session management direction, relationship with calendar, player RSVP flow.
- [ ] **Journals** — Need to discuss journal direction — "Obsidian built into the site" vision, personal vs shared notes, folder structure, linking.

### Deferred to Phase S+ (or community contributions)

- [ ] **Module Builder UI** — Guided wizard that helps users create custom game system modules through the web UI. Step-by-step: name/metadata → define categories → define fields per category → paste/upload reference data → preview tooltips → export as module directory. Eliminates need to hand-write manifest.json + data files.
- [x] Draw Steel module (system data + Foundry adapter)
- [ ] Dagger Heart module (system data + Foundry adapter)
- [ ] Whiteboards / freeform canvas (Tldraw/Excalidraw)
- [ ] Offline mode / service worker caching
- [ ] Collaborative editing presence indicators
- [ ] Calendar timezone support / print-PDF export
- [ ] Map hex/square grid overlay
- [x] Fog of war bidirectional sync (Chronicle ↔ Foundry)
- [x] Foundry module review: runtime bugs (json helper, coord conversion, fog flag, entity_type_id), data flow (scene linking, onSyncMapping, onInitialSync for MapSync, full entity fetch), docs
- [ ] Webhook support for external event notifications
- [ ] Widget inline CSS → CSS classes migration
- [ ] Reusable modal/dropdown component library
- [ ] Dice roller widget
- [ ] Encounter difficulty calculator
- [ ] Family tree / genealogy builder
- [ ] Cross-campaign search
- [ ] Mobile-optimized modals (full-screen on small screens)
- [ ] **Knowledge Graph / Mind Map addon** — Interactive graph visualization showing how campaign content is interconnected. Primary view: **Tag Graph** — nodes are tags, edges connect entities that share tags, click a tag to see all entities tagged with it, click an entity to see all its tags. Additional views: **Mention Graph** — nodes are entities, edges are @mention references between them. **Timeline Graph** — nodes are timeline events, edges show event connections and entity involvement. **Relation Graph** (existing, expand) — add tag-based clustering. Designed as a **self-hosted extension addon** — uploadable via the content extension system (Layer 2: widget extension), not built into core. Ships as a reference implementation of the widget extension API. Uses D3.js or Cytoscape.js. Data sourced from existing APIs (tags, relations, entity-names, timeline). Register as addon (`knowledge-graph` slug) with per-campaign enable/disable.

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

## 4. Technical Debt (Future Refactoring)

Items identified during the 2026-03-09 codebase audit. Not urgent — document for future sessions.

### Handler File Sizes
Large handler files that could benefit from splitting if they grow further:
- [ ] `entities/handler.go` (1,983 lines) — consider splitting entity type CRUD into separate handler
- [ ] `calendar/handler.go` (1,687 lines) — consider splitting event vs calendar CRUD
- [ ] `campaigns/handler.go` (1,245 lines) — consider splitting members/settings into separate handler

### Service Interface Sizes
Interfaces with 30+ methods that could be split into role-based sub-interfaces:
- [ ] `CampaignService` (40 methods) — could split: CampaignCRUD + CampaignMembers + CampaignSettings
- [ ] `EntityService` (38 methods) — could split: EntityCRUD + EntityTypeService + EntityPermissions
- [ ] `TimelineService` (30 methods) — could split: TimelineCRUD + TimelineEvents + TimelineConnections

### Inline CSS in JS Widgets
Six widgets inject `<style>` elements dynamically. Working correctly (ID-based dedup) but could be moved to `input.css`:
- [ ] `permissions.js`, `shop_inventory.js`, `tag_picker.js`, `entity_tooltip.js`, `relations.js`, `template_editor.js`

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

### Sprint K-2: Per-Entity Permissions UI (2026-03-05, batch 36)
- [x] Fixed sync API `GetEntity` visibility gap (only checked `is_private`, now calls `CheckEntityAccess`)
- [x] Added `MemberLister` interface + `SetMemberLister` setter to entities Handler for campaign member picker
- [x] Permissions API: `GET/PUT /campaigns/:id/entities/:eid/permissions` (Owner only)
- [x] Response shape: `{ visibility, is_private, members: [...], permissions: [...] }`
- [x] Permissions widget (`static/js/widgets/permissions.js`): three-mode radio (Everyone/DM Only/Custom), role grants (Player/Scribe), user grants per campaign member, auto-save with abort controller
- [x] Script tag added to `base.templ`
- [x] Entity edit form: replaced `is_private` checkbox with permissions widget mount point + hidden field to preserve `is_private` during form submission
- [x] Entity card: multi-mode visibility icon (shield-halved for custom, lock for DM-only)
- [x] Category dashboard: updated table visibility column + tree view privacy indicator
- [x] Show page: updated title block + blockChildren visibility indicators
- [x] Export adapters: TODO comment for entity_permissions export

### Community Bestiary Backend (2026-03-25)
- [x] Phase 1: Foundation — models, migrations (5 tables), repository CRUD, service with slug gen + statblock validation, handlers, routes
- [x] Phase 2: Search (FULLTEXT + filters), feeds (newest/trending/top-rated/most-imported), creator profiles
- [x] Phase 3: Ratings (1-5 with self-rating prevention, atomic aggregates), reviews, favorites with toggle
- [x] Phase 4: Import/fork into campaigns (EntityCreator + CampaignRoleChecker adapters), flagging with auto-hide
- [x] Phase 5: Admin moderation (flagged queue, approve/archive/restore, audit trail), per-user rate limiting
- [x] Security hardening: transactional writes with row locking, per-user flag deduplication (bestiary_flags table)
- [x] Multi-system support: `system_id` accepted on publish, defaults to "drawsteel"
- [ ] Bestiary unit tests — service tests with mocked repo (not yet written)
- [ ] Widget integration — Draw Steel monster widget to call bestiary API endpoints (external repo)

### Security Hardening — Audit Completion (2026-03-25)
- [x] HTMX `allowEval: false` in boot.js
- [x] Log redaction: added `api_key` to sensitive params
- [x] Progressive login delays (2s→5min cap) replacing hard-reject after 10 failures
- [x] innerHTML audit: fixed escapeHTML typo, consolidated 6 local escape dupes, added missing escaping for icons/colors
- [x] Leaflet vendoring: confirmed already complete
- [x] Updated security-hardening-plan.md with completion status
