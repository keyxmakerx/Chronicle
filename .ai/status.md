# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-03-20 -- **Chronicle API extended for Foundry VTT bidirectional sync (chronicle-sync).**

57. **Sync API: Marker + Note CRUD Endpoints.**
    - **Marker CRUD on Sync API** — Added `GET/POST /api/v1/campaigns/:id/maps/:mapID/markers`, `GET/PUT/DELETE .../markers/:markerID` to `map_api_handler.go`. Foundry module can now sync map pins via API key auth (previously only available on session-cookie web routes). Includes `pin_category`, `visibility_rules`, and `entity_id` linking.
    - **Note CRUD on Sync API** — New `note_api_handler.go` with `GET/POST /api/v1/campaigns/:id/notes`, `GET/PUT/DELETE .../notes/:noteID`. Supports ProseMirror `entry`/`entry_html` plus legacy block content. Campaign IDOR check on single-note operations.
    - **Route wiring** — `RegisterAPIRoutes` now accepts `*NoteAPIHandler`. Note service instantiated before sync API registration in `app/routes.go` (shared instance with web handler).
    - Documentation updated: syncapi/.ai.md, maps/.ai.md, entities/.ai.md, notes/.ai.md, data-model.md.
    - All unit tests pass, build succeeds.

56. **Chronicle API Extensions for Foundry VTT Sync.**
    - **Entity Types API Enriched** — `GET /campaigns/:id/entities/types` now returns `slug`, `color`, `name_plural`, `sort_order`, `enabled` in addition to `id`, `name`, `icon`. Unblocks Foundry folder mapping.
    - **Entity Type WebSocket Events** — Added `entity_type.created`, `entity_type.updated`, `entity_type.deleted` message types. Published from entity type CRUD service methods via the existing EventBus pattern. Unblocks real-time folder sync.
    - **Note WebSocket Events** — Added `note.created`, `note.updated`, `note.deleted` message types. `NoteEventPublisher` interface + adapter wired into notes widget service. Unblocks real-time note sync.
    - **Player Notes on Entities** — New `player_notes` (JSON) and `player_notes_html` (TEXT) columns on entities (migration 000016). Dedicated `GET/PUT /campaigns/:id/entities/:eid/player-notes` API. Enables syncing entity content as GM-only page + player-visible page in Foundry JournalEntries.
    - **Map Marker Pin Category** — New `pin_category` column on `map_markers` (migration 003). Supports categories: location, danger, treasure, quest, note. Added `GET /campaigns/:id/maps/:mid/markers` list endpoint (previously markers only loaded with map page). Existing WS events (`marker.created/updated/deleted`) already defined.
    - **Optimistic Concurrency Control** — `UpdateEntityInput` now accepts optional `ExpectedUpdatedAt`. If set, the service compares against current `updated_at` and returns 409 Conflict if the entity was modified since. Uses existing `apperror.NewConflict`. The `UpdateEntityRequest` JSON DTO exposes this as `expected_updated_at`.
    - **Campaign Members** — Already existed at `GET /campaigns/:id/members` (Accept: application/json). No changes needed.
    - All unit tests pass, lint clean, build succeeds.

55. **Module Manager UI Reorganization + Version Bump Fix.**
    - **Dashboard Card Removed** — Removed the standalone "Foundry VTT / Module Manager" card from the admin dashboard. The Foundry module settings are now accessed via a gear icon on the foundry-module package card in the Packages page.
    - **Settings Gear on Package Card** — Added a gear icon button (links to `/admin/foundry`) on foundry-module type package cards, next to the refresh/check-for-updates button.
    - **Redeploy Auto-Bump Removed** — Removed the `RedeployFoundryModule` handler, `bumpPatchVersion` helper, and the POST `/admin/foundry/redeploy` route. The Redeploy button automatically bumped the patch version on every click, which was confusing. Version changes are now explicit via the "Set Version" form. Manifest/download URLs are already dynamically rewritten at serve time by `serveFoundryModuleManifest` in app.go.
    - **Back Link Updated** — Foundry settings page now links back to `/admin/packages` instead of `/admin`.

54. **Sidebar Navigation Overhaul — Final Sprints (N-5 through N-8 + Responsive).**
    - **Sprint N-5: Folder UX Completion (COMPLETE)** — Inline folder rename (double-click in reorg mode, Enter/blur saves via PUT API, Escape cancels). Folder visibility toggles (`HiddenNodeIDs` on SidebarConfig, eye icons on folder nodes). Right-click context menu (Rename/Delete). Fixed wrapper `data-node-id` vs `data-entity-id`. Skipped redundant div→div conversion.
    - **Sprint N-6: Layout Editor Polish (COMPLETE)** — `injectMissing()` auto-appends addons and entity types missing from the items array when loading existing config. Handles addon enabled after initial config creation.
    - **Sprint N-8: Saved Filter Presets (COMPLETE)** — Migration 000015 (`saved_filters` table). `SavedFilterRepository` with List/Create/Delete. REST API (GET/POST/DELETE `/campaigns/:id/saved-filters`). `sidebar_tag_filter.js` rewritten: preset buttons above tag chips, one-click apply, right-click delete, "Save" button when tags active.
    - **Responsive/Mobile Fixes (COMPLETE)** — Touch drag-and-drop for layout editor (touchstart/touchmove/touchend with ghost element). Touch targets increased to WCAG-friendly sizes (tag chips, eye toggles, drag handles, context menu items). Long-press (500ms) context menu for mobile. Inline rename: 14px font prevents iOS zoom, scrollIntoView for virtual keyboard.
    - **Phase N is fully complete.** Admin section was already collapsible (N-7a). Sidebar quick search (N-7b) deferred — Ctrl+K global search covers the use case.
    - **Next Steps**: X-5 (character sheet blocks), W-2 (map drawing tools), W-4 remaining (batch operations on entity list page).

53. **Sidebar Navigation Overhaul (Sprints N-1 through N-4, ADR-032).**
    - **Sprint N-1: Pure Folders (COMPLETE)** — `sidebar_nodes` table for entity-free organizational folders. Migration 000013 adds table + `parent_node_id` on entities + migrates `is_folder` entities + drops column. `SidebarNode` model/repo/handler with full CRUD. `sidebar_tree.js` merges entities + folder nodes. Reparent menu "New empty folder" creates sidebar_nodes record.
    - **Sprint N-2: DB-Backed Favorites (COMPLETE)** — `entity_favorites` table (migration 000014). Per-user, per-campaign. Toggle/list/check API. `favorites.js` rewritten to use API with in-memory cache. Cross-device persistence.
    - **Sprint N-3: Unified Sidebar Model (COMPLETE)** — `SidebarItem` type in sidebar_config JSON. `Items` array replaces entity_type_order + hidden_type_ids + custom_sections + custom_links. Template renders all items in owner-defined order. `sidebar_layout_editor.js` widget in Customize Hub — single drag-and-drop list for ALL sidebar items.
    - **Sprint N-4: Large Campaign Support (COMPLETE)** — Tag filtering in drill panel (`?tags=slug1,slug2` with AND-logic SQL). Lazy loading at 50/page with IntersectionObserver. Multi-select bulk move (Ctrl+click, floating action bar, POST /entities/bulk-move). Collapsible Manage section with localStorage persistence.
    - **Auto-Registration of Systems as Addons (COMPLETE, ADR-031)** — Removed hardcoded dnd5e/pathfinder2e/drawsteel addon definitions. `systems.AddonInfos()` → `addons.RegisterSystemAddon()` pipeline.

52. **Sprint X-3: System-Provided Widgets + Auto-Registration + Sidebar Reorder Bugfix.**
    - **Sidebar Reorder 404 Fix (COMPLETE)** — MySQL `RowsAffected=0` on `UpdateParent`/`UpdateSortOrder` when values unchanged caused false "entity not found" 404. Removed `RowsAffected` checks, added explicit `FindByID` validation in service layer. Also added proper error logging in JS catch handler, cleared orphan `data-parent-id` on cross-type tree nodes, added NaN guard for sort_order, and used `*int` for nullable sort_order in handler.
    - **Sprint X-3: System-Provided Widgets (COMPLETE)** — Game system modules can now declare JS widgets in their manifest that appear in the template editor palette and render on entity pages. Infrastructure:
      - `WidgetDef` struct added to `SystemManifest` (slug, name, icon, description, script_file)
      - Manifest validation: max 10 widgets, slug format, .js extension, path traversal prevention, XSS sanitization
      - `WidgetScriptAPI` handler serves widget JS files from system directory with path security checks
      - `GetSystemWidgetBlockMetas` and `GetSystemWidgetScriptURLs` on `SystemHandler` (uses `ext_widget` block type — reuses existing renderer)
      - `widgetBlockListerAdapter` extended to query both extension and system widgets
      - LayoutInjector injects system widget `<script>` tags alongside extension widget scripts
      - Widget route: `GET /campaigns/:id/systems/:mod/widgets/:slug`
      - Diagnostics page shows widget count and details
      - `Dir()` package-level function and `CampaignSystemManager.Dir()` for filesystem access
      - ZIP upload `Install()` now accepts `widgets/*.js` files alongside `data/*.json`
    - All widgets come from installed system packages (no hardcoded widgets in Chronicle source). D&D 5e stat-block widget is a reference implementation shipped with the dnd5e system package.
    - **Auto-Registration of Systems as Addons (COMPLETE, ADR-031)** — Game systems are now fully self-service. Removed hardcoded dnd5e/pathfinder2e/drawsteel addon definitions from `builtinAddons`. Added `systems.AddonInfos()` → `addons.RegisterSystemAddon()` pipeline called during app wiring. Dropping a system folder (manifest.json + data/*.json + widgets/*.js) into `internal/systems/` or installing via the package manager now auto-registers it as an addon — zero code changes needed. Only systems with custom Go code (e.g., dnd5e's tooltip renderer) still need a blank import in main.go.
    - **Next Steps**: X-5 (character sheet layout blocks), A2-2 (armory instance UI polish), W-2 (map drawing tools).

51. **Bugfix: CORS, NPC sidebar, entity reorg, lint.**
    - **CORS BASE_URL Normalization (COMPLETE)** — Trailing slash on BASE_URL caused CORS origin mismatch (exact-match failed). Now stripped at config load time in config.go. Removed redundant TrimRight calls in app.go.
    - **Unused formatError Removal (COMPLETE)** — Removed dead `formatError` func and unused `fmt` import from armory/instance_service.go (golangci-lint fix).
    - **NPC Tab Indentation Fix (COMPLETE)** — Removed conditional `pl-8` class that indented NPC link under Journal when notes addon enabled. NPC now renders at same level as other sidebar nav items.
    - **Sidebar Entity Reorg Fix (COMPLETE)** — HTMX lazy-load of entity list used `hx-swap="outerHTML"` on anonymous inner div. `htmx:afterSwap` target had no id, so sidebar_tree.js never called `initTree()`. Changed to `hx-target="#sidebar-cat-results"` + `hx-swap="innerHTML"` so event target matches and tree initializes correctly.
    - **Next Steps**: Merge branch, continue with X-3 (system-provided widgets), X-5 (character sheet blocks), A2-2 (instance UI polish).

50. **Entity Manager Folders + Preset Auto-Setup + Diagnostics + Armory Tests.**
    - **Entity Manager Folder Creation (COMPLETE)** — "New Folder" button (Scribe+) in entity_manager widget toolbar. Calls QuickCreateAPI to create folder entities. Tree rendering with indentation and folder icons (`fa-folder-open`). Reparenting via drag-and-drop (bottom 2/3 of row = nest inside, top 1/3 = reorder). JSON search response now includes `parent_id` and `sort_order` fields. `buildTree()` helper constructs parent-child hierarchy from flat entity list.
    - **Phase X-2: Preset Auto-Setup (COMPLETE)** — New `PresetApplier` interface in addons package. When a game system addon is enabled, `EnableForCampaign()` auto-creates entity types from the system manifest's `entity_presets`. `presetApplier` implementation in `app/preset_applier.go` bridges systems registry and entity service. Skips presets whose category already exists (avoids duplicates on re-enable). Graceful degradation — addon stays enabled even if preset creation fails.
    - **Phase X-4: System Diagnostics Page (COMPLETE)** — New `/campaigns/:id/systems/status` route. Shows enabled system info (name, version, author, source), stats grid (categories, items, presets, Foundry fields), reference data category list with counts, entity presets with field counts and Foundry actor types, campaign entity types with preset badges and entity counts, Foundry VTT compatibility card with mapped field stats, validation warnings. Template: `diagnostics.templ`. Handler: `SystemStatus()` on `CampaignSystemHandler` with `EntityCountProvider` and `EntityTypeProvider` function adapters.
    - **Armory Test Coverage (COMPLETE)** — 47 unit tests across 4 new test files: `model_test.go` (offset/order/defaults/field helpers), `service_test.go` (list/count/tags/errors/empty types), `instance_service_test.go` (CRUD, IDOR, validation, slugify), `transaction_service_test.go` (purchase flow, stock decrement, IDOR, currency defaults, shopMeta parsing).
    - **Next Steps**: X-3 (system-provided widgets), X-5 (character sheet blocks), A2-2 (instance UI polish), command palette saved filters.

49. **Sidebar Hidden Entities + Entity Manager Widget.**
    - **Armory pluginURL Fix (COMPLETE)** — Added missing `armory` case to `pluginURL()` in settings.templ so Features page links to `/campaigns/:id/armory`.
    - **Sidebar Hidden Entities (COMPLETE)** — New `HiddenEntityIDs []string` field in `SidebarConfig` + `UpdateSidebarConfigRequest`. Handler filters hidden entities from sidebar for players (role < Scribe), shows them dimmed (opacity-40 + eye-slash icon) for owners. Template updated with `data-sidebar-hidden` attribute. `sidebar_tree.js` propagates hidden flag to tree nodes. `sidebar_reorg.js` fetches config on entity reorg entry, adds eye/eye-slash toggle per entity node, saves via PUT sidebar-config. Event-driven: `chronicle:toggle-entity-visibility` / `chronicle:entity-visibility-changed` DOM events.
    - **Entity Manager Widget Block (COMPLETE)** — New `entity_manager` block type registered in routes.go. Mount-point templ component `block_entity_manager.templ`. JS widget `static/js/widgets/entity_manager.js` self-fetches entity data via existing search API (JSON mode). Features: search with 300ms debounce, sort by name/manual, drag-and-drop reorder (Scribe+), entity visibility toggle (Owner), type badge, icon display. Auto-mounted by boot.js. Script loaded in base.templ.
    - **New `BlockConfigInt` helper** in block_registry.go for extracting integer config values.

48. **Bugfix Sprint + Armory Multi-Instance.**
    - **CORS Origin Normalization (COMPLETE)** — `UpdateCORSOrigins` now auto-prepends `https://` when bare hostnames entered (e.g., `vtt.bnuuy.haus` → `https://vtt.bnuuy.haus`). Strips trailing path components.
    - **NPC Server Error Fix (COMPLETE)** — `ListNPCs` and `CountNPCs` handle missing character entity type gracefully (empty gallery instead of 500).
    - **Sidebar NPC Under Journal (REVERTED in session 51)** — Was indented under Journal; now always at same level as other nav items.
    - **Invalid CSS Classes (COMPLETE)** — Fixed `bg-bg-secondary`, `border-border`, `bg-surface-raised` across 6 template files.
    - **Armory Multi-Instance (COMPLETE)** — Standalone named inventory collections per campaign. Migration 000011: `inventory_instances` + `inventory_items` tables + `instance_id` on `shop_transactions`. Full CRUD for instances (Owner), add/remove items (Scribe+). Gallery page has instance dropdown selector + manage panel. Transactions scoped to instances. New files: `instance_repository.go`, `instance_service.go`, `instance_handler.go`, `instances.templ`. Modified: `model.go`, `repository.go`, `handler.go`, `gallery.templ`, `routes.go`, `transaction_model.go`, `transaction_repository.go`, `app/routes.go`.
    - **Sidebar Cleanup (COMPLETE)** — Removed Armory static link from sidebar. Addon features are accessible via Features page and embeddable as widget blocks, not static nav links.
    - **Sidebar Entity Reorder Fix (COMPLETE)** — Fixed three interlocking bugs preventing entity drag-and-drop in drilled-in categories: (1) MutationObserver killed reorg on drill-state change — now transitions between category/entity reorg instead of deactivating; (2) Race condition where `#sidebar-entity-tree` wasn't loaded yet when reorg activated — added HTMX afterSwap retry with timeout; (3) Stale container reference in `sidebar_tree.js` event listener — now re-queries DOM; (4) Preserved reorg state across HTMX tree re-inits.
    - **Next Steps**: Phase X-2 (preset auto-setup), Phase X-4 (diagnostics), Armory test coverage.

47. **Sprint: Phase X-0/X-1 — Sandbox Hardening + System Preview.**
    - **Foundry Admin Fix (COMPLETE)** — Switched admin handler from `FoundryPathProvider` interface to `func() string` closure (`a.foundryModuleDir`) — same function serving layer uses. Added hygiene scanner `packages/` exclusion, `tryReuseExistingInstall()` for DB wipe recovery, `ReconcileOrphanedInstalls()` for orphan detection.
    - **X-0: Sandbox Hardening (COMPLETE)** — Field type enum whitelist (`string/number/boolean/list/markdown/enum/url`) enforced in `ValidateManifest()`. Slug format validation (`^[a-z0-9][a-z0-9_-]*$`) on manifest IDs, category slugs, preset slugs. Content limits (20 categories, 100 fields/category, 50 fields/preset, 10 presets). HTML sanitization of all manifest text fields via `html.EscapeString`. `OwnerUploadPolicy` setting: `auto_approve`/`require_approval`/`disabled`. Security settings page now has working Save button.
    - **X-1: Preview & Impact Visualization (COMPLETE)** — `PreviewFromZIP()` dry-run validates ZIP in memory without writing to disk. `PreviewFromPackage()` for admin approval flow. `BuildImpactTree()` creates nested tree structure (system → categories → presets → fields → Foundry mappings → warnings). Preview template with impact tree diagram, category cards with sample items, entity preset tables with Foundry path annotations, error/warning sections. `impact_tree.js` widget renders collapsible tree with CSS connection lines. Two-step upload flow: upload → preview → confirm install. Upload policy gating in campaign handler.
    - **Next Steps**: X-2 (entity preset auto-setup on addon enable), X-4 (diagnostics page), admin package preview approval.

46. **Sprint: Security Hardening + System Detachment + Usage Tracking.**
    - **Admin Re-Authentication (COMPLETE)** — `RequireReauth` middleware in `auth/middleware.go` checks for recent password confirmation via Redis key (5-min TTL). `ConfirmReauth` + `IsReauthValid` service methods. Handler at `POST /account/reauth`. Reauth modal template (`reauth_modal.templ`) with Alpine.js + HTMX integration, included globally in `base.templ`. Applied to sensitive admin routes: ToggleAdmin, DisableUser, EnableUser, ForceLogoutUser.
    - **Usage Tracking (COMPLETE)** — `GetUsageByCampaign` repository method queries `campaign_addons` → `addons` → `campaigns`. `GetUsage()` service wired to real query (was empty stub).
    - **System Detachment (COMPLETE)** — Removed bundled system data: `dnd5e/data/` (6 JSON files), `dnd5e/manifest.json`, `pathfinder2e/` (entire directory), `drawsteel/` (entire directory). Preserved `dnd5e/dnd5e.go` custom factory + tooltip renderer + templates. Updated dnd5e tests to use temp fixture data instead of deleted bundled files. `Init()` logs "no bundled systems found" when directory is empty. Systems now load exclusively from package manager via `LoadAdditionalDir()`. Foundry module fallback preserved.
    - **Next Steps**: TOTP/MFA implementation (Phase 1 of plan). Accessibility audit.

45. **Sprint: Package Manager Plugin + Repo Separation Docs.**
    - **Repo Separation Documentation (COMPLETE)** — Created comprehensive documentation templates in `docs/repo-templates/` for `chronicle-foundry-module` and `chronicle-systems` repos. Includes README, CLAUDE.md, .ai/ architecture docs, API contracts, adapter docs, JSON schemas, and step-by-step system creation guides.
    - **Package Manager Plugin (COMPLETE)** — New `internal/plugins/packages/` plugin with full admin UI. Features: GitHub release fetching, version management (install/pin/unpin), auto-update policies (off/nightly/weekly/on_release), background update worker, package CRUD, HTMX version picker and usage tracking UI. Database: `packages` and `package_versions` tables. Routes under `/admin/packages`. Sidebar link added.

44. **Sprint: Draw Steel System Module + Infrastructure.**
    - **Draw Steel System (COMPLETE)** — Full manifest expansion: 24-field character preset with Foundry path annotations, creature preset with EV/role/role_type, kit preset. Reference data: 8 abilities, 6 creatures, 10 ancestries, 6 kits (CC-BY-4.0). Status changed from `coming_soon` to `available`.
    - **Dynamic Foundry Actor Types (COMPLETE)** — Added `foundry_actor_type` to `EntityPresetDef` struct and `CharacterFieldsResponse` API. Draw Steel uses `"hero"` (not `"character"`). Generic adapter reads `actorType` from API. `actor-sync.mjs` uses `this._actorType` everywhere instead of hardcoded `'character'`.
    - **CORS Admin Whitelist (COMPLETE)** — `cors.allowed_origins` in `site_settings`. `GetCORSOrigins()`/`UpdateCORSOrigins()` on SettingsService. CORS middleware accepts `DynamicOrigins` function that reads from DB. Admin UI on `/admin/api` page with textarea for managing origins. Fixes Foundry VTT CORS-blocked connections.
    - **SYSTEM_MAP_FALLBACK Fix (COMPLETE)** — Changed `drawsteel` key to `'draw-steel'` to match the Foundry system ID.

43. **Sprint: F-6 + F-7 (Armory, Inventory, Transactions, Shop Enhancement).**
    - **A-2: Chronicle Sync Dashboard Expansion (COMPLETE)** — Owner API Keys page expansion with sync status overview, mappings table, error display. Admin API Monitor expansion with per-campaign sync stats.
    - **B-1: System-Dependent Item Presets (COMPLETE)** — `preset_category` column on `entity_types` (migration 000009). `RelationPresetDef` on SystemManifest. Item entity presets for dnd5e (gem), pf2e (equipment), drawsteel (kits). `ItemPreset()` + `ItemFieldsForAPI()` helpers. Item fields API endpoint.
    - **B-2: Armory Page + Sidebar (COMPLETE)** — `internal/plugins/armory/` plugin: gallery page, handler, service, repo. HTMX search/filter/sort/pagination. Sidebar link gated on "armory" addon. `armory_preview` dashboard block. Adapter pattern for cross-plugin deps.
    - **B-3: Inventory Block + Foundry Item Sync (COMPLETE)** — `blockInventory` templ mount point. `inventory.js` Chronicle widget with quantity/equipped/attuned controls. `item-sync.mjs` Foundry module: bidirectional "Has Item" relation ↔ Actor item sync. Hooks: createItem/deleteItem/updateItem. WS events: relation.created/deleted/metadata_updated.
    - **C-1: Transaction Table + Service + Currency Fields (COMPLETE)** — Migration 000010: `shop_transactions` table. Transaction model/repo/service/handler. Purchase flow with stock validation. Currency fields on all character presets (dnd5e: cp/sp/ep/gp/pp, pf2e: cp/sp/gp/pp, drawsteel: wealth). REST endpoints: POST purchase, POST/GET transactions.
    - **C-2: Shop Management UI + Foundry Purchase Flow (COMPLETE)** — Transaction history in `shop_inventory.js` widget. `transaction_log.js` standalone widget + `transaction_log` block type. Foundry `shop-widget.mjs` enhanced with `_executePurchase()` method, buy buttons, stock/buyer validation.

42. **Sprint: U-1 + W-1 (partial) + T-3.**
    - **U-1: Role-Aware Dashboard Editor (COMPLETE)** — Role selector in dashboard editor. `RoleDashboardLayouts` struct with backward-compatible JSON format: detects legacy bare `{"rows":[...]}` vs role-keyed `{"default":...,"player":...,"scribe":...}`. Alpine.js toggle (Default/Player/Scribe) in customize.templ. `dashboard_editor.js` appends `?role=` param, listens for `role-change` events. Handler merges role layouts via `SetRoleDashboardJSON`/`RemoveRoleDashboardJSON`. `ParseRoleDashboardLayout(role)` with fallback chain. `UpdateDashboardLayoutRaw` service method. 9 unit tests in `model_test.go`.
    - **W-1: Command Palette (PARTIAL)** — `static/js/command_palette.js` (~280 lines). Ctrl+Shift+P trigger, modal with search input, scrollable command list. Context-aware: detects campaign ID from URL, admin from DOM. 13 campaign nav commands, 3 action commands, 3 universal commands. Fuzzy substring match, keyboard nav (arrows/enter/escape). Added to `base.templ` and `shortcuts_help.js`. Saved filters not yet implemented.
    - **T-3: Worldbuilding Prompts (COMPLETE)** — Full stack:
      - Migration 000008: `worldbuilding_prompts` table with campaign_id, entity_type_id, name, prompt_text, icon, sort_order, is_global, timestamps, foreign keys.
      - Model: `WorldbuildingPrompt`, `CreatePromptInput`, `UpdatePromptInput` structs.
      - Repository: `WorldbuildingPromptRepository` with Create, FindByID, ListForCampaign, ListForCampaignAndType, Update, Delete.
      - Service: `WorldbuildingPromptService` with validation (name max 200, text max 5000), default icon. `EntityTypeLister` interface (subset of EntityTypeRepository). `SeedDefaults` inserts 16 prompts across 5 types.
      - Handler: ListAPI (HTMX fragment support), CreateAPI, UpdateAPI, DeleteAPI with IDOR checks.
      - Routes: GET (Player+), POST/PUT/DELETE (Owner) at `/campaigns/:id/worldbuilding-prompts`.
      - Templates: `WorldbuildingPromptsPanel` (collapsible card, HTMX lazy-load) + `WorldbuildingPromptsFragment` (`<details>` accordion).
      - Seeding: `WorldbuildingPromptSeeder` interface in campaigns package, called during campaign creation.
      - Tests: 7 unit tests.
    - **Next up:** W-1 saved filters, or Phase 2 (X-1: System Upload UX), or other backlog items.

41. **Post-Phase-1 Sprint: W-0.5 + V-4b + U-2.**
    - **W-0.5 completion** — Accent color CSS variable now propagates to all 258 Tailwind utility usages. Updated `tailwind.config.js` to reference `var(--color-accent)` instead of hardcoded hex. Added `--color-accent-hover` and `--color-accent-light` CSS variables with auto-computed darker/lighter variants from the base hex color. New `AccentColorCSS()` helper in `layouts/data.go` generates the CSS block with all three variants. Topbar styling, brand name, brand logo were already working.
    - **V-4a (cover images)** — Already fully implemented: migration 000004 (`cover_image_path`), API (`PUT /entities/:eid/cover-image`), `cover_image` layout block type in block registry, upload/change UI with hover overlay. No new work needed.
    - **V-4b (graph export)** — Added PNG export button to relations graph widget. Uses SVG→Canvas→PNG pipeline with 2x resolution for retina clarity. Download button in the graph controls bar alongside zoom buttons.
    - **U-2: Campaign Invite System (NEW)** — Full invite flow:
      - Migration 000007: `campaign_invites` table with token, role, expiry, accept tracking.
      - Model: `Invite` struct with `IsExpired()`, `IsPending()` helpers.
      - Repository: `InviteRepository` with Create, GetByToken, ListByCampaign, MarkAccepted, Delete, DeleteExpired, GetByEmailAndCampaign.
      - Service: `InviteService` with CreateInvite (token generation, duplicate check, email send), AcceptInvite (token validation, membership creation), ListInvites, RevokeInvite, GetInviteByToken.
      - Handler: `InviteHandler` with ListInvitesAPI, CreateInviteAPI, RevokeInviteAPI, AcceptInvitePage, InvitesPage.
      - Templates: `InviteAcceptPage` (standalone page for accept flow with login/register redirect), `InviteListFragment` (HTMX fragment for settings page with send form + invite table).
      - Routes: `RegisterInviteRoutes` — accept page at `/invites/accept?token=xxx`, CRUD at `/campaigns/:id/invites`.
      - Email: HTML+plaintext invite email with accept link, campaign name, role.
      - Auth redirect: Login and register handlers now support `?redirect=` parameter for post-auth redirect to invite accept page.
      - Tests: 9 tests covering create, validation, duplicate, accept, expired, already-accepted, revoke, list, default role.
    - **Next up:** Phase 2 (X-1: System Upload UX) or V-4 graph tag filtering, or Phase 4 collaboration features.

40. **Post-F-5 QoL: NPC sidebar navigation link.**
    - Added "NPCs" entry to campaign sidebar in `internal/templates/layouts/app.templ` below "All Pages" link.
    - Uses `fa-users` icon, `isPathPrefix` for active highlighting, links to `/campaigns/:id/npcs`.
    - Updated `.ai/phases.md` with 6-phase priority rewrite (already done in session 36).
    - Phase X sprints (X-1 through X-5) already present in `todo.md`.
    - **Next up:** Phase 1 continues — Sprint F-4.5/F-QoL/F-5 all complete. Next is Phase 2 (X-1: System Upload UX) or Phase 6 (F-6: Armory/Inventory), depending on owner priority.

39. **Sprint F-5: NPC Viewer / Hall (DONE).**
    - **NPC plugin** — New `internal/plugins/npcs/` with model/repo/service/handler/templates/routes. View-layer plugin — no new database tables, queries existing `entities` table filtered by character type + visibility.
    - **Gallery page** — `/campaigns/:id/npcs` shows a responsive grid of revealed character entities. Search by name (debounced HTMX), sort (name/updated/created), tag filter, pagination. Players see non-private characters; Scribes/Owners see all.
    - **NPC card** — Portrait (aspect 3:4 or placeholder), name, type label/race/class, tags. Responsive grid (2→6 columns).
    - **Reveal toggle** — Eye icon on each card (Scribe+). Toggles `is_private` via `POST /npcs/:eid/reveal` with HTMX swap. Green eye = visible, red eye-slash = hidden.
    - **Entity service** — Added `TogglePrivate(entityID)` to `EntityService` interface. New `UpdatePrivate(entityID, isPrivate)` on `EntityRepository`. Publishes entity "updated" event via WebSocket.
    - **npc_gallery layout block** — Registered in block registry (no addon required). Shows compact 3-4 column grid of up to 8 NPCs with "View all" link. Configurable via `{"limit": N}`.
    - **Foundry sync** — Bidirectional NPC visibility sync. Chronicle→Foundry: entity `is_private` change updates actor's `prototypeToken.hidden`. Foundry→Chronicle: actor hidden toggle calls `POST /entities/:id/reveal` API.
    - **Sync API** — New `POST /api/v1/campaigns/:id/entities/:entityID/reveal` endpoint. Accepts `{"is_private": bool}` or toggles if omitted. Verifies entity belongs to campaign.
    - **Tests** — 7 tests: `ListNPCs_ReturnsCards`, `ListNPCs_EmptyWhenNoCharacterType`, `CountNPCs`, `NPCListOptions_Offset`, `NPCListOptions_OrderByClause`, `NPCCard_FieldString`, `TogglePrivate_EntityService`. All pass.
    - **Wiring** — `npcEntityTypeFinderAdapter` and `npcVisibilityTogglerAdapter` in `app/routes.go`. Routes registered with `RequireRole(Player)` for view, `RequireRole(Scribe)` for reveal.

38. **Sprint F-QoL: Foundry Sync Diagnostics (DONE).**
    - **Validation report** — New `ValidationReport` type and `BuildValidationReport()` on `SystemManifest`. Analyzes categories, fields, presets, Foundry compatibility, mapped/writable fields, and generates warnings. Shown in custom system section after upload.
    - **Template update** — `SystemValidationReport` templ component renders capability badges (categories, fields, presets, character fields), Foundry compatibility status, field mapping summary, and warnings.
    - **API client health metrics** — `api-client.mjs` now tracks REST success/error counts, reconnect attempts, connection uptime, last success/error timestamps. New `_errorLog` array with structured error entries (method, path, status, message). `getUptimePercent()` computes session uptime.
    - **Retry queue** — New `queueForRetry()` method for failed write operations. `processRetryQueue()` runs on WebSocket reconnect, retrying up to 3 times with structured logging. Queue capped at 50 entries.
    - **Dashboard diagnostics** — Status tab now shows: 4-column diagnostics grid (uptime%, API OK, API errors, reconnects), last success/error timestamps, pending retry count, field mapping debug info (adapter type, system ID, character type slug), and separate error log section.
    - **Dashboard CSS** — Added `.diagnostics-grid`, `.diagnostics-detail`, `.error-value`, `.error-text`, `.error-log` styles.
    - **Tests** — 3 new tests: `BuildValidationReport_FullSystem`, `BuildValidationReport_MinimalSystem`, `BuildValidationReport_NoFoundryPaths`. All pass.
    - **Dagger Heart** added to deferred systems list alongside Draw Steel.

37. **Sprint F-4.5: Generic System Adapter (DONE).**
    - **Manifest schema** — Added `foundry_system_id` to `SystemManifest`, `foundry_path` and `foundry_writable` to `FieldDef`. `IsFoundryWritable()` helper defaults to true when nil. New `CharacterFieldsForAPI()` builds API response with field annotations.
    - **API endpoint** — `GET /api/v1/campaigns/:id/systems/:systemId/character-fields` returns field definitions with Foundry path annotations. Supports both built-in and custom campaign systems via `CampaignSystemLister` interface.
    - **Manifest annotations** — dnd5e: all 15 character fields annotated with `foundry_path`; AC, speed, proficiency_bonus marked `foundry_writable: false`. PF2e: all 15 fields annotated; only hp_current/hp_max writable (everything else derived). DrawSteel: `foundry_system_id: "draw-steel"` added.
    - **Foundry sync-manager.mjs** — `_detectSystem()` now API-driven: queries `/systems` and matches by `foundry_system_id` first, falls back to `SYSTEM_MAP_FALLBACK` for legacy support. Custom-uploaded systems with `foundry_system_id` auto-match.
    - **Foundry generic-adapter.mjs** — New data-driven adapter that fetches field definitions from `/systems/:id/character-fields` API. Auto-generates `toChronicleFields()` and `fromChronicleFields()` from field annotations. Respects `foundry_writable` and field types for casting.
    - **Foundry actor-sync.mjs** — `_loadAdapter()` tries built-in adapters (dnd5e, pf2e) first, then falls back to generic adapter for any other system.
    - **Tests** — Added 7 tests for `IsFoundryWritable`, `CharacterPreset`, `CharacterFieldsForAPI`, and `LoadManifest` with Foundry annotations. All pass.
    - **Impact:** Any game system (including custom uploads) can now participate in character sync by including `foundry_system_id` and `foundry_path` annotations in its manifest.

36. **Phase & Sprint Plan Reorg.**
    - Rewrote `.ai/phases.md` with 6 phases in new priority order based on owner direction:
      (1) Foundry Completion & QoL (F-4.5, F-QoL, F-5),
      (2) System Modularity & Owner Experience (X-1 through X-5 — new phase),
      (3) Maps & Spatial (W-2, W-2.5),
      (4) Collaboration & Polish (U-series, W-1, W-4),
      (5) Content & Integrations (T-3/4, W-3/5/6),
      (6) Foundry Advanced (F-6, F-7).
    - Added new Phase X sprints to `todo.md`: X-1 (upload UX wizard), X-2 (auto entity presets),
      X-3 (system-provided widgets), X-4 (system diagnostics), X-5 (character sheet blocks).
    - Added F-QoL sprint (Foundry sync diagnostics & error handling).
    - Key insight: the system framework needs end-to-end validation that a non-technical
      owner can upload a custom system and get full functionality (presets, tooltips,
      Foundry sync, widgets, character sheets) without manual setup.
    - Next up: Sprint F-4.5 (generic system adapter).

35. **F-4.5 planning: Generic System Adapter & Dynamic Matching.**
    - Identified that F-4's `SYSTEM_MAP` and `_loadAdapter()` switch are hardcoded to only dnd5e/pf2e/drawsteel. Custom-uploaded game systems can't participate in character sync despite having the server infrastructure (entity presets, `CharacterPreset()` helper, campaign system upload).
    - Planned F-4.5 sprint: add `foundry_system_id` to system manifest, add `foundry_path` + `foundry_writable` annotations on character preset field definitions, new `GET /systems/:id/character-fields` API endpoint, new `generic-adapter.mjs` that reads field definitions from API and auto-generates field mappings. dnd5e/pf2e remain as overrides.
    - Updated `.ai/todo.md`, `foundry-module/.ai.md` (known limitations + F-4.5 plan), and Phase F master plan with full F-4.5 sprint spec.

34. **Sprint F-4: Actor ↔ Entity Sync (DONE).**
    - **actor-sync.mjs** — New `ActorSync` module class. Bidirectional sync between Foundry Actors (type: character) and Chronicle character entities. Registers `createActor`/`updateActor`/`deleteActor` hooks. Handles `entity.created/updated/deleted` WS messages filtered by character type. Uses `_syncing` guard. `_onCharacterDeleted()` unlinks (unsets flags) rather than deleting Actor.
    - **System adapters** — `adapters/dnd5e-adapter.mjs` maps 15 D&D 5e fields (ability scores, HP, AC, speed, level, class, race, alignment, proficiency_bonus). `adapters/pf2e-adapter.mjs` maps PF2e fields (ability mods, HP, AC, perception, ancestry, heritage); only pushes HP/name back to Foundry (PF2e derives most values from items/rules).
    - **Dashboard Characters tab** — New tab in sync dashboard showing synced/unlinked actors with Push button for manual push. Empty states for no actors, disabled sync, no system match.
    - **module.mjs** — Registered `ActorSync` as sync module.
    - **TESTING.md** — Added 30+ character sync test items covering both directions, dashboard, adapters, edge cases.
    - **Next:** F-5 (NPC Viewer / Hall) or F-6 (Armory / Inventory).

33. **Sprint F-3: System detection & character field templates (DONE).**
    - **Server: Manifest expansion** — dnd5e character preset expanded from 4 to 15 fields (added ability scores, HP, AC, speed, proficiency_bonus). New pf2e character preset with 15 PF2e-specific fields (ancestry, heritage, ability mods, perception, etc). Added `CharacterPreset()` method on `SystemManifest`.
    - **Server: Systems API** — New `GET /api/v1/campaigns/:id/systems` endpoint returning all registered systems with `enabled` flag per campaign (via `AddonChecker`). `addonChecker` injected into `APIHandler` via `SetAddonChecker()`.
    - **Foundry: System detection** — `SYSTEM_MAP` maps Foundry `game.system.id` → Chronicle system IDs (`dnd5e`, `pf2e`, `drawsteel`). `SyncManager._detectSystem()` queries systems API on start, stores matched system in `detectedSystem` setting. New `syncCharacters` boolean setting (gated on system match).
    - **Foundry: Dashboard** — Status tab shows Foundry system, Chronicle system match (green check/red X), and character sync availability.
    - **Next:** F-4 (Actor ↔ Entity Sync) — new `actor-sync.mjs` with system-specific adapters.

32. **Foundry enhancements — planning + F-1/F-2 implementation.**
    - **Planning:** Captured Phase F roadmap (F-1 through F-7) in `.ai/todo.md` and `foundry-module/.ai.md`.
    - **F-1: Journal sync fidelity (DONE):** Multi-page sync — entity content with h1/h2 headings splits into separate Foundry pages via `_splitByHeadings()`. Multiple Foundry pages concatenate back into single Chronicle entry via `_collectTextPages()`. `_syncPagesToJournal()` adds/updates/removes pages incrementally. Ownership change hook now pushes `is_private` on every update.
    - **F-2: Granular permission mapping (DONE):** New syncapi endpoints `GET/PUT /entities/:eid/permissions` wrapping existing `EntityService.GetEntityPermissions` / `SetEntityPermissions`. Foundry module: `_buildOwnership()` fetches Chronicle permissions and maps role grants to Foundry default ownership levels (custom visibility player view→OBSERVER, player edit→OWNER, no player grant→NONE). `_pushPermissions()` reverse-maps Foundry ownership changes to Chronicle visibility/permission updates. User-specific grants stored but not mapped (needs user ID mapping table — deferred). TESTING.md updated with multi-page and permission test items.
    - **Remaining planned:** F-3 (system detection), F-4 (actor sync), F-5 (NPC hall), F-6 (armory/inventory), F-7 (shop enhancements).

31. **Foundry module review.** Comprehensive code review of the Foundry VTT sync module found 13 issues. Fixed 9 (deferred ApplicationV2 upgrade):
    - **Runtime bugs**: Shop window `{{json}}` helper crash (replaced with data-item-id lookup), drawing coordinate conversion missing percentage↔pixel (tokens had it, drawings didn't), fog reconciliation `_syncing` flag corruption (extracted `_createFogDrawingData`, batch creates), entity_type_id:0 invalid in syncapi handler (added first-type fallback).
    - **Data flow gaps**: Scene-to-map linking (context menu + auto-link), `_onEntityCreated` fetches full entity from REST (WS payload partial), `onSyncMapping` added to MapSync and CalendarSync, `onInitialSync` added to MapSync (pulls drawings/tokens/fog on connect).
    - **Docs/metadata**: SimpleCalendar in module.json, calendar hint fix, .ai.md corrections (fog status, App v1 not v2), TESTING.md fog + scene-linking sections.

30. **Session completion fix + two-dashboard architecture.** Two changes:
    - **Session "Mark Complete" bug fix**: `hx-vals` always sends form-encoded data regardless of `hx-headers` Content-Type. `UpdateSessionAPI` expects JSON, so the decode failed → 400 error → red notification bar. Replaced with `Chronicle.apiFetch()` onclick handler using `data-url` and `data-name` attributes. Also fixes XSS risk from session name interpolation into JSON template string.
    - **Two-dashboard architecture**: Split campaign dashboard into two independently customizable dashboards:
      - **Campaign Page** (`GET /campaigns/:id`) — visible to all members and public visitors. The "front page" of the campaign.
      - **Owner Dashboard** (`GET /campaigns/:id/dashboard`) — visible only to campaign owner. Campaign management with quick links (Settings, Customize, Members, Plugins), category grid, and recent entities.
      New migration (000006) adds `owner_dashboard_layout` JSON column. Full CRUD: model field + parser, repository query updates, service methods with shared `validateDashboardLayout()` helper, handler + routes, `OwnerDashboardPage` templ component, sidebar "Dashboard" link for owners, and second dashboard editor section in Customization Hub.

### Previous Update
29. **Journal + Sessions + Audio Attachments sprint.** Four changes:
    - **Journal save bug fix**: `journal.js` referenced `window.Chronicle._tiptapBundle` which doesn't exist — the TipTap bundle is `window.TipTap`. Fixed the reference so TipTap editor loads correctly and notes save.
    - **Session edit UI**: Added edit button + modal on session detail page. Pre-populates all fields (name, date, summary, status, recurrence). Submits JSON PUT to existing `UpdateSessionAPI` endpoint. Visible to Scribe+ users.
    - **Journal @mentions**: Added `MentionLink` mark and `MentionExtension` lifecycle wiring to journal's TipTap editor. Users can type `@` to search and link entities with tooltip cards, matching the main entity editor's behavior.
    - **Audio attachments (Sprint V-5)**: New `note_attachments` table (migration 000005). Full backend: `AttachmentRepository` + `AttachmentService` + REST handlers (list/upload/delete/transcript). Media service extended with audio MIME types (mp3/ogg/wav/webm) and magic bytes validation, with `sanitizeImage()` guarded to skip audio files. Journal UI: microphone upload button, inline `<audio>` players, collapsible transcript textarea per attachment, delete support.

### Previous Update
2026-03-11 -- **Sprint W-0.5: Visual Customization + Admin DB Explorer (IN PROGRESS).**

28. **Customization Hub & Features page bug fixes.** Five issues resolved:
    - **Settings page deduplication**: Removed backdrop upload and accent color picker from campaign settings page — appearance customization now lives exclusively in the Customization Hub's Appearance tab.
    - **Accent color page refresh fix**: `UpdateAccentColorAPI` no longer sends `HX-Refresh: true`, preventing the full page reload that navigated users away from the Appearance tab.
    - **Draft+Save model for appearance**: Appearance tab now uses a local draft model — brand name, accent color, and topbar style changes preview instantly but are only persisted when the user clicks "Save Changes". Backdrop upload remains immediate (file upload requires server storage).
    - **Features tab removed from Customization Hub**: The "Features & Packs" tab was removed; features are managed exclusively on the dedicated Plugin Hub page (`/plugins`). The per-category Attributes editor was moved into the Categories tab where it fits naturally.
    - **Plugin Hub in-place toggle**: Feature enable/disable on the Plugin Hub now uses `HX-Trigger: plugin-hub-refresh` instead of `HX-Redirect`, enabling instant in-place list refresh via a new `/plugins/fragment` endpoint.

### Previous Update
2026-03-11 -- **Sprint W-0.5: Visual Customization + Admin DB Explorer (IN PROGRESS).**

27. **Fix: Embed plugin migrations in binary (ADR-030).** Root cause of entity page errors and DB Explorer showing 0/0: plugin migrations used relative filesystem paths (`internal/plugins/*/migrations/`) that only resolve when the binary's CWD is the project root. In Docker, the binary runs from `/app` so migration directories were never found, tables were never created, and entity pages crashed. Fix: each plugin now embeds its `migrations/*.sql` via Go's `embed.FS`. `PluginSchema.MigrationsDir` (string) replaced with `MigrationsFS` (`fs.FS`). `RegisteredPlugins()` moved from `database` package to `cmd/server/main.go` to avoid import cycles (database can't import plugin packages). `PluginSchemas` stored on `App` struct and passed to `DatabaseExplorer` for on-demand re-migration. New `embed.go` files in calendar, maps, sessions, timeline, syncapi plugins.

### Previous Update
2026-03-11 -- **Sprint W-0.5: Visual Customization + Admin DB Explorer (IN PROGRESS).**

26. Admin Database Explorer: New `/admin/database` page with interactive D3.js schema diagram showing all tables, FK relationships, plugin grouping, and migration status. Table detail panel on click. "Apply Pending Migrations" button (fixes sessions table missing issue). Removed "Manual DB Record (Advanced)" form from Features page. New files: `database_service.go` (info_schema introspection), `database.templ`, `db_explorer.js` widget. Added `LatestMigrationVersion()` to database package. Dashboard card + sidebar link.

### Previous Update
2026-03-11 -- **Sprint W-0.5: Visual Customization (IN PROGRESS).**

25. Starting W-0.5: Per-campaign brand name/logo, topbar color/gradient/image customization, visual editor Appearance tab in Customization Hub. Also fixing 3 bugs from W-0 (event listener leak in sidebar_tree.js, touch listener cleanup in sidebar_reorg.js, ES2020 optional chaining compat) and updating stale entity/campaign documentation.

### Previous Update
2026-03-11 -- **Sprint W-0: Nav Menu Reorg Mode (COMPLETE).**

24. W-0 complete: Sidebar reorg mode toggle button (Owner-only, grip icon next to "Categories" header), inline category drag-to-reorder with visibility toggles, conditional entity drag-and-drop (only active in reorg mode), touch D&D support for mobile, `data-entity-type-id` on category links, auto-exit on navigation. New file `sidebar_reorg.js`. Modified `sidebar_tree.js` to gate D&D behind `data-reorg-active`. Relations `.ai.md` updated with graph visualization features.

### Previous Update
2026-03-10 -- **Sprint V-4: Enhanced Graph View & Cover Images (COMPLETE).**

18. **@Mention edges in graph**: `FindAllMentionLinks()` in entity repository scans `entry_html` for `data-mention-id` attributes across a campaign. `MentionLink` model. `GetMentionLinks()` service method. `MentionLinkProvider` interface bridges entities→relations without circular imports. Mention edges appear as dashed purple lines in the D3 graph.

19. **Graph API filtering**: `GetFilteredGraphData()` with `GraphFilter` struct supporting `types` (entity type slugs), `search` (name match), `focus`+`hops` (BFS local/ego graph), `include_mentions`, `include_orphans`. BFS subgraph extraction via `bfsSubgraph()`. `GraphEdge.Kind` field distinguishes "relation" vs "mention" edges.

20. **Graph UI enhancements**: Updated `relation_graph.js` with: filter toolbar (type multi-select, search input, mention toggle, orphan toggle), dashed mention edges with purple color, node sizing by connection count, type-based clustering via `d3.forceX/forceY`, orphan nodes with dotted borders, enhanced legend showing edge types and orphan indicator.

21. **Graph page template updates**: `GraphPage` handler now fetches entity types for filter dropdown. Template passes entity types as JSON via `data-entity-types` attribute. `EntityTypeListerForGraph` interface with adapter.

22. **Cover image layout block**: Migration `000004_cover_image` adds `cover_image_path` column to entities. `cover_image` block type registered in block registry. `blockCoverImage()` templ component with configurable height (sm/md/lg) and overlay (none/gradient/dark). `UpdateCoverImageAPI` endpoint at `PUT /campaigns/:id/entities/:eid/cover-image`. Reuses `image-upload` widget for upload.

23. **Local graph block**: `local_graph` block type registered in block registry. `blockLocalGraph()` renders a mini `relation-graph` widget with `data-focus-entity` and `data-hops` attributes for ego-graph mode on entity profile pages.

### Previous Update
2026-03-10 -- **Sprint V-3: Content Templates (COMPLETE).**

12-17. Content templates: migration, CRUD, template picker, editor slash command, default templates, Customization Hub tab.

### Previous Update
2026-03-10 -- **Cleanup & consolidation pass after bug fixes.**

7. **JSON injection fix**: HX-Trigger header in `CreateEntityType` error path used string concatenation to build JSON. Replaced with `json.Marshal()` to prevent malformed JSON from error messages containing special characters.

8. **Orphaned addon settings page removed**: Deleted `/addons/settings` route, `CampaignAddonsPage` handler, and `CampaignAddonsPageTempl` template — all superseded by the unified Plugin Hub at `/plugins`. Fragment route (`/addons/fragment`) kept for Customization Hub.

9. **Fallback redirect updated**: `ToggleCampaignAddon` non-HTMX fallback now redirects to `/plugins` instead of the removed `/addons/settings`.

10. **Dead HTMX attributes cleaned**: Removed `hx-target`/`hx-swap` from Plugin Hub toggle forms since the handler always responds with `HX-Redirect` (the swap targets were never used).

11. **ADR-029 recorded**: Features page consolidation decision documented in `.ai/decisions.md`.

### Previous Update
2026-03-10 -- **Bug fixes & UX improvements (navbar, features page, entity templates).**

1. **Drag-and-drop CSRF fix**: `sidebar_tree.js` reorderEntity() was using raw `fetch()` without CSRF token, causing 403 on drag-drop reorder. Added `Chronicle.getCsrf()` header.

2. **Nav menu lag fix**: `sidebar_drill.js` category switching caused stale content flash. Now shows loading spinner immediately and uses prefetch cache for instant swaps.

3. **Entity template creation fix**: `entity_types.templ` form lacked `hx-target`/`hx-swap`, causing broken page when HTMX swapped full page into form. Added proper targeting, partial response on error, and `chronicle:notify` HX-Trigger support in notifications.js.

4. **Features page consolidation**: Merged Plugin Hub (read-only) and Addon Settings (management) into a single Features page at `/campaigns/:id/plugins`. Owners see inline enable/disable toggles. Added `AddonID` and `Installed` fields to `PluginHubAddon`. Settings page link now points to `/plugins`. Toggle handler supports `redirect_to=plugins` for redirect after toggle.

5. **Quick notes discoverability**: After creating a quick note, toast now includes a clickable "View in Journal" link. Added `html` option to `Chronicle.notify()`.

6. **Fully hideable navbar**: Added sidebar hide button (double-angle-left icon) next to pin button. When hidden, sidebar fully disappears (0px width). Floating restore button appears at top-left. State persists in `localStorage`.

Branch: `claude/fix-navbar-features-page-8RZuE`.

### Previous Update
2026-03-10 -- **Per-player visibility + Co-DM grants (Phase 2 complete).** Implemented per-player content sharing across all content types:

1. **Maps**: Added `visibility_rules` JSON column to `map_markers` and `map_drawings` (migration 002). Updated repository to filter with `JSON_CONTAINS` for non-owners. Updated `ListMarkers` signature to include `userID` for per-player filtering.

2. **Notes**: Added `shared_with` JSON column to notes (migration 000002). Three sharing modes: Private, Everyone, Specific Players. Full UI with member picker popover in `notes.js`. New `/notes/members` endpoint for fetching campaign members.

3. **Co-DM Grants**: Added `DmGrantIDs []string` to `CampaignSettings` and `IsDmGranted bool` to `CampaignContext`. Middleware resolves grants on every request. `CanSeeDmOnly(role, ...dmGranted)` accepts optional variadic bool. New `VisibilityRole()` method on CampaignContext returns Owner-level for dm-granted users (view-only, not create). All handlers updated to use `cc.VisibilityRole()` for visibility filtering. `PUT/GET /campaigns/:id/dm-grants` endpoints. Settings page has Alpine.js member picker for granting DM privileges. DM-granted users can see dm_only content but cannot create/toggle dm_only flags.

4. **Timeline/Calendar UI**: Visibility dropdowns already had "Specific Players" option with visibility_rules backend — confirmed working.

Branch: `claude/fix-journal-button-placement-UF4hD`.

## Phase & Sprint Plan
See `.ai/phases.md` for the full roadmap. Phases organized by priority:
1. **F**: Foundry Completion & QoL (F-4.5, F-QoL, F-5) ← COMPLETE
2. **X**: System Modularity & Owner Experience (X-1 through X-5)
3. **W**: Maps & Spatial (W-2, W-2.5)
4. **U**: Collaboration & Polish (U-1/2/3/4/5, W-1, W-4)
5. **T**: Content & Integrations (T-3/4, W-3/5/6)
6. **F**: Foundry Advanced (F-6, F-7)

## Current Phase
**Sprint 42: U-1 + W-1 + T-3.** Phase 1 (Foundry Completion: F-4.5, F-QoL, F-5) done. Next: Phase 2 (X-1: System Upload UX) or Phase 6 (F-6: Armory/Inventory).

### Sprint V-2: Backlinks Panel & Entity Aliases (COMPLETE)
- **Fixed migration 060**: Removed incorrect first ALTER that tried to drop 'module' from ENUM directly, causing Error 1265. Kept correct 3-step approach.
- **Migration 061**: `entity_aliases` table with FULLTEXT index, FK cascade to entities.
- **Entity Aliases** (model/repo/service/handler): `EntityAlias` struct, `SetAliasesInput` with validation (max 10, length 2-200, case-insensitive dedup). CRUD via `GET/PUT /campaigns/:id/entities/:eid/aliases` (Scribe+ for write).
- **Alias search integration**: `Search()` now includes alias LIKE matches via subquery. `ListNames()` returns alias entries as separate `EntityNameEntry` rows with `IsAlias=true` — auto-linker and @mention popup match aliases with zero JS changes.
- **Backlinks HTMX lazy-load**: Removed synchronous `GetBacklinks()` from Show handler. Added `GET /campaigns/:id/entities/:eid/backlinks` endpoint returning HTMX fragment or JSON. Show page uses `hx-get` + `hx-trigger="load"`.
- **Backlinks Redis caching**: 5-minute TTL, cache key `backlinks:<entityID>:<role>:<userID>`.
- **Context snippets**: `BacklinkEntry` struct pairs entity + text snippet. `extractMentionSnippet()` finds mention in HTML, strips tags, extracts ~120 chars around mention.
- **Aliases widget** (`aliases.js`): Tag-chip UI below entity title. Add/remove aliases inline. Invalidates auto-link cache on change.
- **Cache invalidation**: `invalidateCachePattern()` helper for Redis SCAN+DEL. Entity names cache invalidated on alias change.
- **Tests**: 11 new tests — alias validation (7 cases), alias dedup, snippet extraction (2), backlinks with snippets.

### Sprint V-1: Quick Capture, Session Journal & Slash Commands (COMPLETE)
- **Slash command menu** (`editor_slash.js`): TipTap `/` trigger shows command palette with 9 block commands (3 heading levels, bullet/numbered list, callout, table, horizontal rule, code block). Filters as user types, keyboard navigation, follows mention extension pattern.
- **Quick capture modal** (`quick_capture.js`): Ctrl+Shift+N opens lightweight modal with auto-timestamped title + content area. Creates campaign-wide note via existing notes API. Ctrl+Enter to submit.
- **Session Journal** (`quick_capture.js`): Topbar "Journal" button creates or opens "Session Journal - YYYY-MM-DD" note. Finds existing journal by title match, creates new if none exists. Opens notes panel with highlight animation.
- **Notes in Ctrl+K** (`search_modal.js`): Quick search now fetches notes in parallel with entities. Notes filtered client-side by title, appended to results with sticky-note icon. Selecting a note opens the notes panel via `chronicle:open-note` event.
- **Notes panel events** (`notes.js`): Added `chronicle:note-created` (refresh) and `chronicle:open-note` (open + scroll-to) event handlers with cleanup in destroy.
- **Shortcuts help** (`shortcuts_help.js`): Added Ctrl+Shift+N and `/` to help overlay.

### Generic Module Framework (COMPLETE)
- **GenericTooltipRenderer** (`generic_tooltip.go`): Reads field definitions from the manifest's `categories[].fields[]` to render tooltips. Shows only manifest-declared fields in manifest-defined order. Works for any game system.
- **GenericModule** (`generic_module.go`): Wraps JSONProvider + GenericTooltipRenderer. Implements the Module interface with zero game-system-specific code.
- **Auto-instantiation** (`loader.go`): When a module directory has `manifest.json` + `data/` but no registered Go factory, the loader automatically creates a GenericModule instance. New game systems work by just dropping files.
- **Tests**: 6 new tests — generic renderer (supported categories, render with manifest fields, field filtering, field ordering, nil item), generic module end-to-end, loader auto-instantiation.
- **D&D 5e** retains its custom `TooltipRenderer` as an override for richer formatting, but could also work with the generic renderer.

### Sprint M-1: D&D 5e Module — Data & Tooltip API (COMPLETE)
- **Module wiring**: `modules.Init("internal/modules")` in main.go with blank import of dnd5e for factory registration, `modules.RegisterRoutes()` in app/routes.go
- **SRD data files** (6 categories, 87 items):
  - `spells.json`: 27 spells across levels 0-9 (cantrips through Wish)
  - `monsters.json`: 14 monsters (Goblin through Lich, CR 1/4 to 21)
  - `items.json`: 10 magic items (Potion of Healing through Vorpal Sword)
  - `classes.json`: All 12 SRD classes with hit die, primary ability, saving throws, proficiencies
  - `races.json`: 9 SRD races with speed, size, ability bonuses, traits, languages
  - `conditions.json`: All 15 SRD conditions with effect summaries
- **Manifest**: Added conditions category (slug, name, icon, fields) to dnd5e manifest.json
- **Tooltip renderer**: Category-specific `writeCategoryProperties()` switch — each category shows only its relevant fields
- **Tests**: 9 tests in dnd5e_test.go
- **Build**: Full project compiles clean, all module tests pass

### Sprint R-1: WASM Runtime Integration (COMPLETE)
- Added Extism Go SDK v1.7.1 + wazero v1.9.0 dependencies
- **WASM model types** (`wasm_model.go`): WASMPlugin, WASMContribution, WASMCapability, WASMCallRequest/Response, HookEvent, WASMPluginInfo, WASMLimits — full type system for WASM logic extensions
- **Manifest integration**: Added `WASMPlugins` to `ManifestContributes`, validation for slug/name/file (.wasm required), capabilities (5 types), hooks (8 event types), memory limits (max 256 MB), timeouts (max 300s)
- **PluginManager** (`wasm_manager.go`): Load/Unload/Reload/Call lifecycle with capability-based host function filtering, timeout enforcement, concurrent-safe plugin map, error state tracking
- **Host functions** (`wasm_host.go`): 10 host functions across 5 capability groups:
  - `log`: chronicle_log (with message truncation)
  - `entity_read`: get_entity, search_entities, list_entity_types
  - `calendar_read`: get_calendar, list_events
  - `tag_read`: list_tags
  - `kv_store`: kv_get, kv_set (64KB limit), kv_delete
- **Service adapter interfaces**: EntityReader, CalendarReader, TagReader — read-only data access for WASM plugins via adapter pattern
- **KV store** (`wasm_kvstore.go`): Per-plugin key-value storage backed by existing `extension_data` table (namespace "wasm_kv"), no new DB tables needed
- **Hook dispatcher** (`wasm_hooks.go`): Fire-and-forget async dispatch to WASM plugins registered for events. 8 convenience methods (DispatchEntityCreated/Updated/Deleted, DispatchCalendarEvent*, DispatchTag*)
- **WASM handler** (`wasm_handler.go`): Admin endpoints (list/get/reload/stop plugins) + campaign endpoints (list campaign plugins, call plugin function with Scribe+ role)
- **Routes** (`routes.go`): RegisterWASMAdminRoutes, RegisterWASMCampaignRoutes with proper auth/role middleware
- **Security**: .wasm added to allowedFileExts, capability-based host function filtering (principle of least privilege), memory/timeout limits, log truncation, KV value size cap
- **Repository**: Added DeleteDataByKey method for per-key KV deletion
- **Tests**: 26 new tests — manifest validation (15 cases), model defaults, capabilities, hook types, plugin key generation, serialization, manager lifecycle, security allowlist, zip entry validation, context helpers, log drain/limit
- **New files**: wasm_model.go, wasm_manager.go, wasm_host.go, wasm_kvstore.go, wasm_hooks.go, wasm_handler.go, wasm_test.go

### Sprint R-4: Plugin SDK & Developer Tools (COMPLETE)
- **Example Rust plugin** (`extensions/example-wasm-rust/`): Auto-tagger that hooks into entity.created, looks up entity type, and applies matching tags. Demonstrates Extism Rust PDK with host function declarations, hook handling, dice roller export, and messaging.
- **Example Go plugin** (`extensions/example-wasm-go/`): Session logger that records entity activity to KV store, creates calendar event summaries. Demonstrates Extism Go PDK with `//go:wasmimport` host functions, KV storage, and calendar write.
- **Go SDK** (`sdk/go/chronicle/`): Type definitions for all host function inputs/outputs, hook events, capability constants, and `MockHost` test harness with in-memory implementations of all host functions.
- **MockHost test harness**: Entity/tag/calendar/relation/KV/log/message mocks with setup helpers and inspection methods. 9 unit tests.
- **Plugin development guide** (`.ai/plugin-development.md`): Complete documentation covering capabilities, all 16 host functions, hooks, building (Rust/Go/TinyGo), testing with MockHost, resource limits, API endpoints.
- **Manifest tests**: 7 new test cases — 5 example manifests validated + 2 WASM-specific assertion tests (capabilities, hooks, config, limits).
- **AI docs**: `.ai.md` files for both example plugins, README index updated.

### Sprint R-3: Write Host Functions & Messaging (COMPLETE)
- **5 new write capabilities**: entity_write, calendar_write, tag_write, relation_write, message
- **6 new host function builders** (`wasm_host.go`):
  - `update_entity_fields` — updates entity custom fields via EntityWriter adapter
  - `create_event` — creates calendar events via CalendarWriter adapter
  - `set_entity_tags` — replaces entity tag set via TagWriter adapter
  - `get_entity_tags` — reads entity tags via TagWriter adapter
  - `create_relation` — creates entity relations via RelationWriter adapter
  - `send_message` — async plugin-to-plugin messaging via PluginManager back-reference
- **4 new write interfaces**: EntityWriter, CalendarWriter, TagWriter, RelationWriter
- **4 new write adapters** (`wasm_adapters.go`): NewWASMEntityWriteAdapter, NewWASMCalendarWriteAdapter, NewWASMTagWriteAdapter, NewWASMRelationWriteAdapter
- **App wiring** (`app/routes.go`): All write adapters wired with closure-based JSON marshaling/unmarshaling, PluginManager back-reference set for messaging
- **Security**: Input size limits on all write functions (256KB fields, 64KB events/relations/messages), required field validation
- **Total host functions**: 16 across 10 capability groups (was 10 across 5)
- **Tests**: 10 new tests — write adapter delegates (4), write capability counts (6), nil-adapter guard tests (2)
- **AllCapabilities** updated from 5 to 10 entries

### Sprint R-2: App Wiring & Admin UI (COMPLETE)
- **App wiring** (`app/routes.go`): EntityReader, CalendarReader, TagReader adapters with JSON serialization closures wrapping concrete services. KV store backed by extension_data. PluginManager, HookDispatcher, WASMHandler all instantiated and registered
- **Adapters** (`wasm_adapters.go`): NewWASMEntityAdapter, NewWASMCalendarAdapter, NewWASMTagAdapter — closure-based adapters avoiding direct plugin imports
- **Auto-loading**: Content applier now auto-loads WASM plugins when extensions are enabled (SetWASMLoader interface on ContentApplier)
- **Graceful shutdown**: main.go unloads all WASM plugins before stopping server
- **App struct**: WASMPluginManager + WASMHookDispatcher fields for lifecycle management
- **Admin UI**: Extension detail page now shows Widgets and WASM Plugins in the contributes section (capabilities as violet badges, hooks as amber badges)
- **Tests**: 12 new tests — adapter delegates (entity/calendar/tag), capability-based host function filtering (8 cases), call context lifecycle, unload/reload/call error paths
- **New files**: wasm_adapters.go, wasm_adapters_test.go

### Phase P Summary (Sprints P-1 through P-6)
- **P-1**: Extension infrastructure — migration 000055 (4 tables), manifest parser/validator, zip security, repository (16 methods), service, handler, routes, config
- **P-2**: Admin UI — polished extension list with card layout, extension detail page (manifest metadata, author, contributes breakdown, dependencies), admin sidebar link, HTMX rescan/update
- **P-3**: Campaign integration — content extensions lazy-loaded in campaign addons settings page and customization hub extensions tab
- **P-4**: Content appliers — entity type templates and tag collections applied on enable with provenance tracking. Adapter pattern bridges entity/tag services
- **P-5**: Marker icons and themes — icon pack registration in extension_data, theme CSS registration, API endpoints for marker-icons and themes
- **P-6**: Example extensions — Harptos Calendar (Forgotten Realms) and D&D 5e Character Sheet with 4 entity types, creature tags, relation types. Unit tests validate manifests
- **Package**: `internal/extensions/` — 11 files (model, manifest, security, repository, service, handler, routes, applier, adapters, templ, tests)
- **Tests**: 41 tests (manifest parsing, security, SVG/CSS validation, UUID, example manifests)

### Extension System Research (batch 56)
- **ADR-021**: Layered third-party extension strategy recorded in `.ai/decisions.md`.
- **Decision**: Three layers — (1) Content Extensions (manifest-only, no code), (2) Widget Extensions (browser-sandboxed JS), (3) Logic Extensions (WASM via Extism/wazero, future).

### Entity Block Registry (batch 55-56)
- **Bug fix**: `validBlockTypes` in entities/service.go was missing most block types, causing "invalid block type" errors for shop_inventory, calendar, timeline, etc.
- **Architecture**: Replaced hardcoded block type lists with a self-registering `BlockRegistry`. Plugins register their block types at startup; validation, rendering, and the template editor palette all derive from the registry.
- **New files**: `entities/block_registry.go` (registry types + config helpers), `entities/block_registry_core.go` (core block registrations), `calendar/blocks.templ`, `timeline/blocks.templ`, `maps/blocks.templ` (plugin block renderers moved to owning plugin packages).
- **Modified**: `entities/show.templ` (switch → registry dispatch), `entities/service.go` (registry-based validation), `entities/handler.go` (block-types API), `entities/routes.go` (new endpoint), `app/routes.go` (registry wiring), `template_editor.js` (fetches block types from API), `template_editor.templ` + `entity_type_config.templ` (added data-campaign-id).
- **API**: `GET /campaigns/:id/entity-types/block-types` — returns available block types filtered by campaign addons.
- **Build fix**: Renamed `blockEntry` struct to `registeredBlock` in `block_registry.go` to resolve naming collision with `blockEntry` templ function in `show.templ`.

### Sprint M0-4: dm_only Visibility on Entity Relations (batch 48)
- Migration 000052: `dm_only BOOLEAN NOT NULL DEFAULT FALSE` on `entity_relations`
- Model: `DmOnly bool` on `Relation`, `CreateRelationRequest`, `GraphRelation`
- Repository: all queries updated (Create, FindByID, ListByEntity, ListByCampaign)
- Service: `Create()` accepts variadic `dmOnly`, `GetGraphData()` accepts `includeDmOnly` filter
- Handler: DM authorization check, role-based filtering in List/Graph endpoints
- Export: `ExportRelation.DmOnly` field, export/import adapters propagate dm_only
- JS widget: DM-only toggle in create modal, lock badge on DM-only relations
- Template: `data-is-dm` attribute on relations widget mount point
- **Files changed**: migration files, model.go, repository.go, service.go, handler.go, export.go, export_adapters.go, show.templ, relations.js

### Sprint M0-1 through M0-3 (batch 47)
- M0-1: Export adapters for permissions, groups, posts
- M0-2: Timeline connections, entity groups, session attendees
- M0-3: Entity parent hierarchy resolution on import

### UX & Feature Gap Audit (batch 46)
Deep audit of player/DM experience, account settings, campaign management, and missing UI surfaces:
- **Account gaps**: No in-app password change (only email reset), display name read-only after registration, avatar infrastructure exists (DB column + media service) but no upload UI, 2FA DB columns exist but incomplete.
- **Player experience**: No entity favorites/bookmarks, no sort controls on entity lists, no tag/field filtering, no character assignment to campaign members, no session recap field, no dice roller.
- **DM tools**: No soft delete/archive (hard delete only), no bulk entity operations, no map measurement/drawing/fog UI (backend exists for Foundry sync), no initiative tracker, no session prep checklist.
- **Campaign settings**: Export/import handlers exist but no visible button on settings page, no invite link system (email-add only), no member kick confirmation dialog.
- **Quick wins identified**: 10 small-but-impactful items added to todo.md (export button, password change, sort controls, notes search, calendar search, favorites, etc.).
- **17 new items** added to `.ai/todo.md` under "Quick Wins" and "Player & DM Experience Gaps" sections.
- Fixed errcheck lint errors in `json_provider_test.go` (2 unchecked `os.WriteFile` calls that failed CI).

### Audit Summary (batch 45)
Created `.ai/audit.md` — comprehensive feature parity and completeness audit covering:
- **Test coverage**: 530+ tests but 4 plugins have zero tests (maps, sessions, admin, smtp), calendar/timeline have only domain-logic tests. No handler or repository tests anywhere.
- **Export gaps**: entity_permissions, campaign_groups, entity_posts not exported (data loss on backup).
- **JS consistency**: 17 widgets still use raw fetch() instead of Chronicle.apiFetch(). Only 2 widgets show toast on error; rest log to console silently. groups.js and relation_graph.js duplicate escHtml().
- **Feature parity**: Relations lack visibility controls (unlike tags, posts, markers). Notes and posts have no search.
- **Documentation**: 16 JS widgets lack .ai.md files. Posts Go widget has no .ai.md.
- 14 new items added to `.ai/todo.md` under "Low (Audit-Discovered)" section.

### Summary of Recent Work (batches 25-44)
- **Batch 44**: Sprint L-5 Calendar Day View — Single-day detailed view at
  `/calendar/day`. DayViewData struct with PrevDay/NextDay/WeekdayName/Season
  helpers. Full-page template with event cards showing time, category, entity
  links, description. Day view icon added to all view toggles (Grid/Week/Day/
  Timeline). Session display for real-life calendars. 5 unit tests. Route:
  `GET /calendar/day` (Player+).
- **Batch 43**: Sprint L-4 Calendar Event Drag-and-Drop — HTML5 DnD on monthly
  grid view. Event chips gain `draggable="true"` (Scribe+ only), day cells become
  drop zones with `data-drop-year/month/day` attributes. Drag handlers: dragStart
  captures event ID + applies opacity, dragOver highlights cell, drop reads all
  event data attributes and sends full PUT to `/calendar/events/:eid` with new
  date. Visual feedback via `cal-drop-highlight` CSS class (accent ring + tint).
  Same-date drops ignored. No backend changes needed (existing PUT handler works).
- **Batch 42**: Sprint L-3 Note Folders — Migration 000051 adds `parent_id` (FK
  with CASCADE) and `is_folder` columns to notes table. Backend: model, repository,
  and service updated for folder create/update/move operations. JS widget (`notes.js`)
  updated with tree view rendering: `buildTree()` groups notes by parentId, folders
  render as collapsible containers with expand/collapse (persisted in localStorage),
  child count badges, add-note-in-folder and rename-folder buttons. Move-to-folder
  dropdown menu on note cards. "New Folder" button in quick-add row. Folder delete
  with cascade warning. CSS styles for folders, move menu, collapse toggle. 4 new
  unit tests (create folder, create with parentId, move to folder, move to top level).
- **Batch 41**: Sprint L-2 Notes Rich Text (TipTap) — Replaced plain textarea
  editing in notes widget with mini TipTap editor instances. When entering edit
  mode, creates TipTap editor with StarterKit+Underline+Placeholder, populated
  from note's `entry` JSON or converted from legacy text blocks to HTML. Saves
  TipTap content (entry JSON + entryHtml) to API. Legacy block→TipTap conversion
  on first edit. Checklists remain as interactive checkboxes (separate from TipTap).
  Editor instances tracked in `miniEditors` map, cleaned up on note save/destroy.
- **Batch 40**: Sprint L-1 Entity Posts (Sub-Notes) UI — Migration 000050
  (`entity_posts` table). Full widget: PostRepository (CRUD + reorder),
  PostService (validation, sort order), Handler (list/create/update/delete/reorder).
  JS widget (`entity_posts.js`) with collapsible post cards, drag-to-reorder,
  visibility toggle (DM only), inline rename, delete confirmation. Integrated into
  entity show page below main entry. Added as layout block type `posts` in
  template editor. Public-capable read route, Scribe+ write routes. 13 unit tests.
- **Batch 39**: Sprint K-5 Relations Graph Visualization — D3.js force-directed graph
  widget (`relation_graph.js`) with dynamic CDN loading, zoom/pan, drag, node coloring
  by entity type, edge labels, tooltips, click-to-navigate, type legend. Backend:
  `ListByCampaign` repository (dedup bi-directional pairs via `source < target`),
  `GetGraphData` service, `GraphAPI` + `GraphPage` handlers. Standalone page at
  `/relations-graph/page`. Dashboard block type `relations_graph` with configurable
  height. Model types: GraphRelation, GraphNode, GraphEdge, GraphData. Phase K complete.
- **Batch 38**: Sprint K-4 Auto-Linking in Editor — Entity names API endpoint
  (`GET /entity-names`) with Redis caching (5-min TTL). Repository `ListNames`
  method returns lightweight name entries (id, name, slug, type info) sorted by
  name length DESC for longest-first matching. Auto-link JS module
  (`editor_autolink.js`) scans editor text nodes for entity names, creates
  @mention links with data-mention-id attributes. Integrated into Insert menu
  ("Auto-link Entities" with wand icon) and Ctrl+Shift+L shortcut. Whole-word,
  case-insensitive matching, min 3 chars, skips text already inside links.
  `EntityNameEntry` model type. Handler gains Redis `cache` field.
- **Batch 37**: Sprint K-3 Group-Based Visibility — Migration 000049
  (`campaign_groups` + `campaign_group_members` tables, subject_type ENUM gains
  "group"). Full GroupRepository (8 methods) and GroupService (validation, CRUD).
  Group CRUD handlers (list/create/get/update/delete groups + add/remove members)
  with Owner-only routes. Groups management page (`groups.templ`) with JS widget
  (`groups.js`) — collapsible group cards, member add/remove, inline rename. Entity
  permissions widget updated with "Group Permissions" section. `visibilityFilter()`
  SQL extended for group membership subquery. Settings page "Groups" link. 7 unit
  tests. Entity handler gains `GroupLister` interface for permissions API.
- **Batch 36**: Sprint K-2 Per-Entity Permissions UI — Permissions widget
  (`permissions.js`) with three visibility modes (Everyone/DM Only/Custom),
  per-role and per-user grant toggles (None/View/Edit), auto-save. Replaced
  `is_private` checkbox on entity edit form. API endpoints: GET/PUT
  `/entities/:eid/permissions` (Owner only). Multi-mode visibility indicators
  in entity cards (shield-halved for custom, lock for DM-only), category
  dashboard table/tree, show page title block + children list. Fixed sync API
  `GetEntity` to check custom visibility via `CheckEntityAccess`. Added
  `MemberLister` interface + wiring for campaign member picker. Export TODO for
  permissions data.
- **Batch 35**: Sprint K-1 Per-Entity Permissions Model — Migration 000048
  (`entity_permissions` table, `visibility` ENUM column on entities). Permission
  model types (VisibilityMode, SubjectType, Permission, EntityPermission,
  EffectivePermission, SetPermissionsInput, PermissionGrant). Full
  EntityPermissionRepository (ListByEntity, SetPermissions transactional,
  DeleteByEntity, GetEffectivePermission, UpdateVisibility). visibilityFilter()
  SQL helper handles both legacy is_private and custom permission modes in a
  single WHERE clause. Service: CheckEntityAccess, SetEntityPermissions,
  GetEntityPermissions. All entity list/search/count/children/backlinks queries
  updated with userID parameter across handlers, sync API, export adapters,
  layout injector, campaign dashboard. 13 new unit tests. Pure backend — no UI.
- **Batch 34**: Sprint J-4 File Security — ClamAV antivirus integration for upload
  scanning via clamd TCP protocol (INSTREAM). Fail-open when clamd unavailable.
  ClamAV container in docker-compose (clamav/clamav:1.4). CLAMAV_ADDRESS env var.
  3 unit tests. SVG blocked by MIME allowlist. CDR strips metadata/polyglots.
- **Batch 33**: Sprint J-3 Testing & Infrastructure — Verified HTMX edge cases (CSRF
  propagation, double-init prevention, widget cleanup, form tracking all covered by boot.js).
  Created `.air.toml` for hot reload config. Fixed docker-compose.yml em-dash in error
  message. Added `doc.go` for templates/components package. Verified all Go packages have
  package doc comments.
- **Batch 32**: Sprint J-2 Editor Enhancements — Code block syntax highlighting via
  @tiptap/extension-code-block-lowlight with highlight.js common languages (JS, Python,
  Go, SQL, etc.). Tokyo Night-inspired dark/light syntax theme in input.css. Find/replace
  bar (Ctrl+F find, Ctrl+H replace) with match navigation, replace, replace-all.
  TipTap bundle rebuilt with lowlight extension (~558KB).
- **Batch 31**: Sprint J-1 Breadcrumbs + Navigation — Shared breadcrumb component
  (`components/breadcrumbs.templ`). Added breadcrumbs to: maps list, map detail, timeline
  list, timeline detail, sessions list, session detail, calendar grid header, calendar
  timeline view, calendar week view. Recently Viewed Entities tracker
  (`recent_entities.js`) using localStorage, renders in sidebar drill panel.
- **Batch 30**: Sprint I-4 Map UX Polish — Leaflet.markercluster integration for both
  map widget and full map page (auto-clustering when >5 markers). Expanded POI icon
  picker from 18 to 39 icons organized in 8 groups (General, Settlements, Fortifications,
  Dungeons & Ruins, Nature, Maritime, Sacred & Magic, Resources). Custom cluster icon
  styling. CDN-loaded MarkerCluster CSS + JS.
- **Batch 29**: Sprint I-3 Calendar Week View — 7-column day grid with event cards.
  Repo: `ListEventsForDateRange`. Handler: `ShowWeek`. Template: `WeekPage/WeekFragment`.
  View toggle (Grid/Week/Timeline) added to all 3 calendar views. Navigation: prev/next/today.
  Cross-month and cross-year boundary handling. 5 unit tests. Route: GET /calendar/week.
- **Batch 28**: Sprint I-2 Timeline Phase 2B — Event connections (migration 000047,
  model/repo/service/handler, D3 SVG lines/arrows with arrowhead markers, 4 line styles),
  create-from-timeline (double-click empty space opens modal with date pre-filled),
  visual polish (connection line CSS, event marker hover effects, ruler label improvements).
  3 unit tests. Routes: GET/POST/DELETE /timelines/:tid/connections.
- **Batch 27**: Sprint I-1 Campaign Export/Import — Full JSON export/import for campaigns
  including entity types, entities, tags, relations, calendar (config + events),
  timelines (standalone events), sessions, maps (markers, drawings, layers, tokens, fog),
  addons, media manifest. 6 new files (export.go, import.go, export_service.go,
  export_handler.go, export_adapters.go, import_test.go). Adapter pattern for 7 plugin
  services. Routes: GET /campaigns/:id/export, POST /campaigns/import.
- **Batch 26**: Phase H Release Readiness — error standardization (249 calls), code dedup,
  OpenAPI 3.0.3 spec, extension docs.
- **Batch 25**: Alpha Hardening — CI pipeline, 3 service test suites, IDOR helper,
  input validation, TipTap table support.

### Earlier Batches (summary)
- **Batch 20**: Fixed duplicate migration 000041, mobile nav cleanup, 3 dashboard widgets.
- **Batch 19**: Shop entity type, relation metadata, shop inventory widget, README cleanup.
- **Batch 16-18**: Sessions-Calendar integration, RSVP emails, recurring sessions, Foundry
  VTT bidirectional sync (WebSocket hub, sync mappings, journal sync, map API, calendar sync).
- **Batches 1-15**: Core platform (auth, campaigns, entities, editor, media, calendar, maps,
  timelines, sessions, addons, admin), 294+ tests, security audit (14 fixes), code quality
  sprint (138 lint fixes), mobile responsive, dark mode, extension framework.

---

### Sprint Q-1: Widget Extension API (COMPLETE)
- Added `WidgetContribution` type to `ManifestContributes` with slug, name, description, icon, file, config fields
- Added `WidgetConfigField` type for configurable data-* attributes (key, label, type, default, options)
- Validation in `validateContributes()`: slug, name, file required; file must be `.js`; path traversal blocked
- `.js` files added to `allowedFileExts` allowlist in security.go and asset serving handler
- Widget registration in `applier.go`: stores widget metadata in `extension_data` (namespace "widgets") with script URLs
- New handler: `ListWidgets` (GET /campaigns/:id/extensions/widgets) — returns all enabled extension widgets
- New handler: `GetWidgetScriptURLs` — used by layout injector to discover widget scripts
- Layout integration: `SetExtWidgetScripts`/`GetExtWidgetScripts` in data.go, `<script>` injection in base.templ
- Layout injector in app/routes.go wires extension widget script discovery per campaign
- Tests updated: security test accepts .js, manifest test covers widget validation (6 new cases)

### Sprint Q-2: Widget Extension Distribution (COMPLETE)
- `ext_widget` block type registered in block registry with generic renderer (`blockExtWidget` templ)
- `BlockMeta.WidgetSlug` field added for template editor to identify extension widgets
- `WidgetBlockLister` interface in entities handler, wired via `widgetBlockListerAdapter`
- `BlockTypesAPI` now appends extension widget blocks from enabled extensions
- `GetWidgetBlockInfos` method on extensions handler returns widget metadata
- Template editor JS updated: palette shows "Extension Widgets" section, drag data includes `widget_slug`, drop handlers set `config.widget_slug`
- Example extension: `dice-roller` with `widgets/dice-roller.js` — d4-d100 roller with history, nat1/natMax highlighting
- Example test updated to validate dice-roller manifest

### Security Audit Remediation (2026-03-06) (COMPLETE)
- Full codebase security audit performed (`.ai/security-audit-2026-03-06.md`)
- **H-1 FIXED**: Session tokens no longer exposed in admin HTML — uses SHA-256 hash-based lookup
- **H-2 FIXED**: RecapHTML sanitized via `sanitize.HTML()` before storage (stored XSS)
- **M-1 FIXED**: `ChangePassword()` now invalidates all existing sessions
- **M-3 FIXED**: Session cookie MaxAge uses configured `SESSION_TTL` instead of hardcoded 30 days
- **M-4 SKIPPED**: Registration already has rate limiting (`middleware.RateLimit(5, time.Minute)`)
- **M-5 FIXED**: Per-email rate limit (3 req/15 min) on password reset via Redis
- **L-1 FIXED**: Avatar upload validates MIME via `http.DetectContentType()` on file bytes
- **L-7 FIXED**: Dockerfile runs as non-root `chronicle` user
- Remaining (accepted/deferred): M-2 (CSP unsafe-inline, Alpine.js dependency), L-2 (ClamAV for avatars), L-3 (in-memory rate limiter), L-4 (CSRF cookie binding), L-5 (query string logging), L-6 (default DB password)

### Admin UI Bug Fixes & Features (2026-03-06) (COMPLETE)
- **Bug 1**: Calendar and API plugins now show as Active in admin registry. Added 5 missing active plugins (timeline, sessions, addons, extensions, syncapi).
- **Bug 2**: Fixed false "confirm before leaving" dialog — `htmx:beforeRedirect` listener clears dirty form state before HTMX redirects.
- **Bug 3**: Removed ClamAV entirely (rely on magic bytes, CDR, MIME allowlist). Added structured error logging to media upload pipeline.
- **Bug 4**: Sidebar drill panel now refreshes content on navigation instead of closing, so newly created entities appear immediately.
- **Bug 5**: Added missing `data-campaign-id` to template editor widget mount in layout editor fragment.
- **Bug 7**: SMTP TestConnection now has step-by-step logging and actionable error messages for each phase (TCP, TLS, auth).
- **Bug 8**: Email change with verification — migration 000056 (pending_email/verify_token columns), full service/handler/template flow. Verifies new email first, requires password re-entry, invalidates all sessions on confirm.
- **Bug 9**: SMTP test email — SendTestEmail endpoint with recipient input, actionable error wrapping for TLS/auth/connection failures.
- **Bug 6**: Terminology consolidation — removed Plugins admin page, renamed Addons→Features, kept Content Packs, renamed Modules→Game Systems across admin sidebar, dashboard, admin pages, and campaign-level pages.

### Bug Fixes & UX Improvements (2026-03-07)
- **Category Nav Page Listing**: Sidebar drill panel now auto-loads entity pages when a category is opened. Uses HTMX `hx-trigger="load"` to fetch entities via the search endpoint with `sidebar=1` rendering mode. New `SidebarEntityList` template renders compact entity links.
- **Shop Widget Create Items**: Added "Create & Add" functionality to shop inventory widget. New `QuickCreateAPI` endpoint (`POST /entities/quick-create`) creates entities from JSON. Shop widget now shows "or create a new item" section in the add panel with inline name input.
- **Image Upload Error Handling**: Improved upload handler with structured error logging at each failure point. Added MIME type fallback to `http.DetectContentType` when browser sends empty/generic Content-Type. `MaxBytesReader` errors now return 400 instead of 500. Frontend shows server error messages.
- **Dirty Form Fix**: Added document-level `htmx:afterRequest` listener that checks for `HX-Redirect` response header and clears all dirty sources. This catches cases where `htmx:beforeRedirect` doesn't fire due to timing differences.
- **Admin Features Page**: Filtered module-category addons (dnd5e, drawsteel, pathfinder2e) from the admin Features page. These game systems are managed on the Content Packs page. Added `CountFeatures` method to exclude modules from the dashboard count.

### Phase S: Data Integrity & Admin Tooling (COMPLETE)
- **Sprint S-1: Campaign Deletion Cleanup** (ADR-025) — Migration 000058 adds FK CASCADE on `api_keys.campaign_id` and SET NULL FK on `api_request_log.campaign_id`. `DeleteCampaignFiles()` on media service cleans disk files before SQL DELETE. Multi-step `Delete()` on campaign service: media cleanup → WASM hook dispatch (`campaign.deleted`) → SQL CASCADE. 4 new tests.
- **Sprint S-2: Extension Migration System** (ADR-024) — Migration 000059 creates `extension_schema_versions` tracking table. SQL validator enforces `ext_<slug>_*` namespace on all DDL/DML. `MigrationRunner` with `RunUp`/`RunDown`/`DropExtensionTables`. Integrated into extension `Install()` and `Uninstall()` lifecycle. 23 tests (17 validator + 6 runner).
- **Sprint S-3: Admin Data Hygiene Dashboard** (ADR-026) — `DataHygieneScanner` interface with scan and purge for orphaned media, API keys, and stale files. Dashboard at `/admin/data-hygiene` with summary cards, data tables, HTMX purge buttons with confirmation. Safety guardrails: referenced media protected from purge, recent files skipped, security event audit logging. 7 new tests.
- **New files**: 2 migrations (up/down each), `sql_validator.go`, `migration_runner.go`, `hygiene_service.go`, `data_hygiene.templ`, plus test files
- **Modified**: campaign service (multi-step delete), media service/repo (bulk cleanup), extension service (migration lifecycle), admin handler/routes/dashboard (hygiene UI), wasm hooks (campaign.deleted event), app routes (wiring)

### Bug Fixes Round 2 (2026-03-08)
- **Category Nav Fix (root cause)**: The `Search()` service method required queries >= 2 characters, but sidebar auto-load sent no query. Fixed `SearchAPI` to use `List()` (no search filter) when query is empty, correctly returning all entities of the selected type.
- **Image Upload 500 Fix (root cause)**: `isAPIRequest()` in error handler only checked Content-Type, not Accept header. Image uploads use `multipart/form-data`, so 500 errors returned HTML error pages that the JS couldn't parse. Fixed `isAPIRequest` to also check Accept header. Switched `image_upload.js` to use `Chronicle.apiFetch` (sends Accept: application/json). Added `ValidateMediaPath()` startup check to verify media directory exists and is writable.
- **Dirty Form Fix (root cause)**: Form change tracking marked forms dirty on input, but dirty state wasn't cleared until response redirect. Added `htmx:beforeRequest` listener that clears form dirty state when a tracked form submits — the user is saving, so the form is clean from that point.
- **Shop Widget Fix (root cause)**: `Chronicle.apiFetch` returns a raw Response object, but all shop widget API calls treated it as parsed JSON (missing `.json()` chains). Every API call was silently failing. Fixed all calls with proper `.then(res => res.json())` chains. Redesigned add panel: single search bar with inline "Create & Add" section (name input + entity type dropdown).
- **Admin Features Fix**: Planned addons without backing code were showing on the Features page. Now filtered out — only active/installed addons appear. Dashboard count updated to match. (Media Gallery was subsequently converted to a proper addon — see below.)
- **New API**: `GET /campaigns/:id/entities/types` — returns entity types as JSON for widget dropdowns (used by shop widget create flow).
- **Media Gallery as Addon**: The existing media plugin (`internal/plugins/media/`) is now properly registered as the `media-gallery` addon. Campaign media browser routes (`/campaigns/:id/media*`) are gated behind `RequireAddon("media-gallery")`. Sidebar "Media" link conditionally shown via `IsAddonEnabled`. Base upload/serve routes remain ungated (avatars, backdrops work regardless). Migration 000057 updates addon description and sets status to active. Future expansion: albums, tagging, lightbox.

### Sprint T-1: D&D 5e Reference Pages (COMPLETE)
- Module handler, routes, shared templates (`module_pages.templ`), and all D&D 5e data files were already built in earlier sprints
- **New**: Wired `ModuleSearchAdapter` into entity handler `SearchAPI` so Ctrl+K quick search now returns game system module results alongside entity/timeline/map/calendar/session results
- **Files modified**: `entities/handler.go` (added `ModuleSearcher` interface + field + setter + search call), `app/routes.go` (wired adapter)

### Sprint T-2: Pathfinder 2e Module (COMPLETE)
- **Zero Go code** — GenericModule auto-instantiation picks up the module from manifest + data files
- **6 data categories** (82 items total): spells (20), creatures (14), equipment (12), ancestries (8), classes (12), conditions (16)
- **Manifest updated**: Added classes and conditions categories, set status to "available", bumped version to 1.0.0
- **All ORC-licensed**: Content from Player Core and GM Core only
- **Auto-features**: Reference pages, tooltips, and Ctrl+K search all work via GenericModule infrastructure

### Custom Game System Upload (2026-03-09)
- **CampaignModuleManager** (`campaign_modules.go`): Per-campaign custom module storage under `media/modules/<campaignID>/`. ZIP upload with validation (manifest + data/*.json), security checks (path traversal, size limits), auto-prefixes ID with `custom-` to avoid collisions. Discovers existing uploads on startup. Install/Uninstall lifecycle.
- **CampaignModuleHandler** (`campaign_handler.go`): Upload (POST), delete (DELETE), and status (GET) endpoints at `/campaigns/:id/modules/upload|custom`. Owner-only. Returns HTMX fragments for inline UI updates.
- **Module handler updated**: `resolveModule` now checks campaign-specific custom modules after global registry. All 5 handler methods use the new resolution path.
- **Route middleware updated**: `requireModuleAddon` now also checks if the requested module is the campaign's custom module (bypasses addon check).
- **UI**: Custom Game System section on campaign addons page with upload form (empty state) or installed module info card with Browse/Remove buttons. HTMX-powered upload and delete with inline swap.
- **Templ component**: `CustomModuleSection` renders the upload/manage UI.
- **App wiring**: `CampaignModuleManager` initialized at startup under `media/modules/`, wired into module handler and custom module routes.
- **Files**: `campaign_modules.go`, `campaign_handler.go`, `custom_module.templ` (new), `handler.go`, `routes.go`, `campaign_addons.templ`, `app/routes.go`

### Bug Fixes Round 3 (2026-03-09)
- **Single Game System per Campaign**: Game systems (module category) are now mutually exclusive — enabling one auto-disables any other active game system for that campaign. Service logs the swap. Campaign addons page shows explanatory text.
- **"Register New Feature" Restyled**: The admin features page form was confusing (created empty DB records with no backing code). Restyled as a collapsed `<details>` element with red/warning styling, danger icon, and clear warnings. Renamed to "Manual DB Record (Advanced)". Removed "Module" from category dropdown (game systems come from code, not DB forms). Button changed from "Register Feature" to "Create DB Record".
- **Alert() → Toast Conversion**: All 23 raw `alert()` calls across the codebase converted to `Chronicle.notify()` toasts:
  - Calendar plugin (6 calls): save/move/delete event errors + network errors
  - Maps plugin (10 calls): marker save/delete, settings save, upload, map delete + network errors
  - Sessions plugin (1 call): recap save error
  - Settings plugin (2 calls): user ID / campaign ID validation
  - Image upload widget (3 calls): file type/size validation, upload error
  - Shop inventory widget (1 call): item creation error
- **Files modified**: `addons/service.go` (mutual exclusivity), `addons/admin_addons.templ` (failover restyle), `addons/campaign_addons.templ` (info text), `calendar/calendar.templ`, `maps/maps.templ`, `sessions/sessions.templ`, `settings/storage_settings.templ`, `image_upload.js`, `shop_inventory.js`

### Terminology Rename: Modules → Systems (2026-03-09)
- Renamed `internal/modules/` → `internal/systems/` across the entire codebase
- All Go package references, import paths, route paths, template references, JS widget references updated
- Admin UI labels: "Content Packs" / "Game Systems" (no longer uses "Module" anywhere user-facing)
- Documentation updated: CLAUDE.md, architecture.md, phases.md, README.md, api-routes.md, plugin .ai.md files

### Campaign Customization: Backdrop & Accent Color (2026-03-09)
- **Backdrop image**: Campaign-level hero image upload stored as `backdrop_path` in campaigns table
- **Accent color**: Per-campaign CSS custom property (`--color-accent`) override via `accent_color` column
- Campaign settings page gains image upload and color picker sections
- Layout injects `<style>` tag with accent color override when set

### Dashboard Blocks: 7 New Types (2026-03-09)
- **calendar_full**: Full calendar grid embed via `/calendar/embed` with HTMX lazy-loading
- **timeline_full**: Timeline D3 widget embed via `/timelines/embed`
- **relations_graph_full**: Full-height relations graph (reuses existing D3 widget)
- **map_full**: Interactive Leaflet map embed with configurable map_id and Phase 2 objects
- **session_tracker**: Upcoming sessions list via `/sessions/embed`
- **activity_feed**: Campaign activity feed via `/activity/embed` (Owner only)
- **sync_status**: API key status via `/sync-status` (Owner only)
- All use HTMX `hx-trigger="intersect once"` for lazy-loading
- Dashboard editor updated with new block type options and config fields

### Embed Endpoints Pattern (2026-03-09)
- New lightweight handler endpoints return HTMX fragments for dashboard block lazy-loading
- calendar: `EmbedCalendar` — compact calendar grid with inline month navigation
- timeline: `EmbedTimeline` — mounts timeline-viz D3 widget, auto-selects first timeline
- sessions: `EmbedSessions` — lists planned sessions with RSVP badges
- audit: `EmbedActivity` — compact activity feed with avatars and relative time
- syncapi: `SyncStatusEmbed` — active key count, 24h request stats, per-key status

### Map Widget Phase 2 Objects (2026-03-09)
- Map widget now supports optional Phase 2 object rendering via `data-show-drawings` and `data-show-tokens` attributes
- Drawings rendered as Leaflet shapes (polylines, polygons, rectangles, circles)
- Tokens rendered as icon markers with HP popup tooltips
- Configurable height via `data-height` attribute

### Documentation Update (2026-03-09)
- Updated 15+ documentation files for modules→systems terminology
- Added new embed endpoint routes to api-routes.md
- Updated plugin .ai.md files with new embed handlers and dashboard block info
- Updated architecture.md directory structure to reflect systems/ path

## Next Session Should
- **W-1 Saved Filters** — Persist command palette search filters per campaign
- **Phase 2: X-1 System Upload UX** — Guided wizard for system install
- **Phase 6: F-6 Armory / Inventory System** — Items as entities, character inventory, Foundry sync
- See `.ai/phases.md` for full execution order

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`
- Tailwind standalone CLI (`tailwindcss`) is v3; do NOT use `npx @tailwindcss/cli` (v4 syntax)
- Fog-of-war sync is bidirectional (Chronicle ↔ Foundry) using dark-polygon heuristic with pixel↔percentage conversion.
- SimpleCalendar integration uses journal listeners with SC flag detection for CRUD hooks.

## Completed Phases
- **2026-02-19: Phase 0** — Project scaffolding, AI docs, build config
- **2026-02-19: Phase 1** — Auth, campaigns, SMTP, admin, entities, editor, UI layouts,
  unit tests, Dockerfile, CI/CD, production deployment, auto-migrations
- **2026-02-19 to 2026-02-20: Phase 2** — Media, security audit, sidebar, entity images,
  layout builder, dark mode, tags, relations, attributes, editor, semantic colors
- **2026-02-20: Phase 3** — UI overhaul: Page/Category rename, drill-down sidebar
- **2026-02-20: Phase B** — Extension framework, Sync API, REST API v1
- **2026-02-20: Phase C** — Player notes, terminology standardization
- **2026-02-22 to 2026-02-24: Phase D** — Customization Hub, Dashboard Editor, Page Layouts
- **2026-02-24 to 2026-02-25: Phase E** — Quick Search, Entity Hierarchy, Editor Insert Menu
- **2026-02-25 to 2026-02-28: Phase F** — Calendar & Time (monthly grid, events, settings,
  import/export, timeline view, eras, seasons)
- **2026-02-28 to 2026-03-03: Phase G** — Maps & Geography (Leaflet.js, markers, DM-only),
  Timeline standalone events, D3 visualization (ruler, grid, eras, clustering, minimap)
- **2026-03-03: Alpha Documentation Sprint** — Bug fixes, extension docs, README
- **2026-03-03: Code Quality Sprint** — golangci-lint v2, 138 lint fixes, JS dedup
- **2026-03-04: Bug Fixes & Testing** — Image upload, apiFetch, HTMX fixes, service tests
- **2026-03-04: Foundry VTT Completion** — Sessions-calendar integration, RSVP emails,
  recurring sessions, Foundry sync (WebSocket, maps API, calendar, shop, fog), security
  hardening, dashboard widgets, mobile responsive, extension documentation
- **2026-03-04: Alpha Hardening** — CI pipeline (golangci-lint + govulncheck), 3 service
  test suites (audit/media/settings), generic IDOR helper, input validation, widget docs,
  TipTap table extensions
- **2026-03-04: Phase H Release Readiness** — Error standardization (249 calls, 15+ files),
  code dedup (MemberLister, LIKE escape), OpenAPI 3.0.3 spec (63 endpoints), extension docs
- **2026-03-05: Sprint I-1 Campaign Export/Import** — Full JSON export/import for campaigns
  (entities, calendar, timelines, sessions, maps, addons, media manifest). 7 adapter services.
- **2026-03-05: Phase I Core UX** — Sprint I-2 (timeline connections + create-from-timeline),
  Sprint I-3 (calendar week view), Sprint I-4 (map marker clustering + expanded POI icons).
- **2026-03-05: Sprint J-1** — Breadcrumbs on all non-entity pages, recently viewed entities sidebar widget.
- **2026-03-05: Sprint J-2** — Code syntax highlighting (lowlight + highlight.js), find/replace bar (Ctrl+F/H).
- **2026-03-05: Sprint J-3** — HTMX verification, `.air.toml`, docker-compose fix, package doc comments.
- **2026-03-05: Sprint J-4** — ClamAV antivirus scanning, docker-compose ClamAV container.
- **2026-03-05: ALL PHASES COMPLETE** — H (release readiness), I (core UX), J (polish & infra).
- **2026-03-05: Sprint K-1** — Per-entity permissions model (backend): migration 000048, model types, permission repository, service methods, visibility filter, 13 tests.
- **2026-03-05: Sprint K-2** — Per-entity permissions UI: permissions widget (permissions.js), visibility modes, role/user grants, auto-save. Sync API GetEntity custom visibility fix.
- **2026-03-05: Sprint K-3** — Group-based visibility: migration 000049 (campaign_groups/members), GroupRepository, GroupService, group CRUD handlers, groups management page + widget, permissions widget group grants, 7 tests.
- **2026-03-05: Sprint K-4** — Auto-linking in editor: entity names API with Redis caching, auto-link JS module (text scanner, mention link creation), Insert menu + Ctrl+Shift+L shortcut.
- **2026-03-05: Sprint K-5** — Relations graph visualization: D3.js force-directed graph widget, graph API + standalone page, dashboard block type.
- **2026-03-05: Sprint K-6** — Foundry Polish: shop icon null fix (FA icons with entity colors), connection status UI (event-driven, click-to-reconnect, activity flash), SimpleCalendar CRUD hooks (journal listeners, SC flag detection), fog bidirectional sync (dark polygon heuristic, pixel↔percentage conversion). **Phase K complete.**
- **2026-03-05: Sprint L-1** — Entity posts (sub-notes): migration 000050, full widget (model/repo/service/handler), JS widget with collapsible cards, drag-to-reorder, visibility toggle, layout block type.
- **2026-03-05: Sprint L-2** — Notes rich text: TipTap mini editor instances replace plain textareas, legacy block→TipTap conversion, entry JSON + HTML saved to API.
- **2026-03-05: Sprint L-3** — Note folders: migration 000051 (parent_id + is_folder), tree view rendering, collapsible folders, move-to-folder, create folder, 4 tests.
- **2026-03-05: Sprint L-4** — Calendar event drag-and-drop: HTML5 DnD on monthly grid, draggable event chips, drop zone highlighting, full PUT on drop. Pure frontend.
- **2026-03-05: Sprint L-5** — Calendar day view: single-day detailed view, event cards with time/category/entity/description, day navigation, view toggle icon. Phase L complete.
