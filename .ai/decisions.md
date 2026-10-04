# Architecture Decision Records

Longer earlier versions of these ADRs: https://github.com/keyxmakerx/Chronicle/blob/ef034a20/.ai/decisions.md

<!-- ====================================================================== -->
<!-- Category: Semi-static (APPEND-ONLY)                                      -->
<!-- Purpose: Records WHY decisions were made. Prevents revisiting settled     -->
<!--          questions. ADR numbers and heading lines are never reused,       -->
<!--          renumbered or removed — code cites them. A record's body may     -->
<!--          be tightened for length, and its status line updated, but its    -->
<!--          decision and rules must not change meaning.                     -->
<!-- Status line: one line right under the heading — Accepted |               -->
<!--   Accepted; amended by ADR-NNN | Superseded by ADR-NNN | Moot (reason).   -->
<!-- Update: Append a new record when a significant decision is made.         -->
<!-- Template: See .ai/templates/decision-record.md.tmpl                      -->
<!-- ====================================================================== -->

---

## ADR-001: Three-Tier Extension Architecture (Plugins, Modules, Widgets)

**Status:** Accepted

**Context:** Extensions come in three fundamentally different shapes: full feature apps, read-only content packs, and reusable UI pieces. A flat structure conflates them and makes naming ambiguous.

**Decision:** Three tiers:
- **Plugins** (`internal/plugins/`): feature apps with handler/service/repo/templates. Core plugins (auth, campaigns, entities) are always enabled; optional ones (maps, timeline) are enabled per campaign.
- **Systems** (`internal/systems/`): read-only game-system content packs (reference data, tooltips), installed via the package manager.
- **Widgets** (`internal/widgets/`): reusable UI blocks that mount to the DOM and fetch their own data.

**Consequences:**
- Dependencies flow one direction only: Plugins may use Widgets; Widgets are self-contained.
- Each tier has its own directory-structure template.

---

## ADR-002: MariaDB Over PostgreSQL

**Status:** Accepted

**Context:** The original spec called for PostgreSQL, but the deployment target and user infrastructure use MariaDB.

**Decision:** MariaDB via `database/sql` + `go-sql-driver/mysql`. No ORM.

**Consequences:**
- No JSONB — use MariaDB `JSON` columns, validated on write.
- No `tsvector` — use `FULLTEXT` indexes.
- No `gen_random_uuid()` — generate UUIDs in Go (`uuid.New()`).
- `?` placeholders, not `$1`.

---

## ADR-003: Hand-Written SQL Over ORM or sqlc

**Status:** Accepted

**Context:** GORM hides N+1 queries and is hard to optimize; sqlc's MySQL support is immature.

**Decision:** Hand-written SQL in repository files, one repository per aggregate root.

**Consequences:**
- Full control over query performance; more verbose but explicit.

---

## ADR-004: HTMX + Templ Over SPA Framework

**Status:** Accepted

**Context:** The frontend needs interactivity without a Node.js build chain.

**Decision:** Server-side rendering with Templ + HTMX; Alpine.js for client-only interactions.

**Consequences:**
- No JSON API is needed for the UI (HTMX speaks HTML).
- Every handler checks `HX-Request`/`HX-Boosted` (`middleware.IsHTMX`) for fragment vs. full page.

---

## ADR-005: PASETO v4 Over JWT

**Status:** Accepted

**Context:** Sessions and API auth need secure tokens; JWT allows algorithm-confusion and `none`-algorithm attacks.

**Decision:** PASETO v4 for all tokens.

**Consequences:**
- No algorithm-confusion attacks (PASETO mandates algorithms per version).
- Less library support than JWT, but Go has solid PASETO libraries.

---

## ADR-006: Go Binary Serves HTTP Directly (No Nginx)

**Status:** Accepted

**Context:** The deployment target (Cosmos Cloud) provides its own reverse proxy, TLS and DDoS protection.

**Decision:** Echo serves HTTP directly; no nginx/caddy in the container.

**Consequences:**
- Single-process container, simpler Dockerfile, faster startup.
- No exposed ports in docker-compose — routing happens at the proxy.

---

## ADR-007: Configurable Entity Types with JSON Fields

**Status:** Accepted

**Context:** Users need custom entity types and fields, not a fixed set.

**Decision:** Entity types are stored in the DB with a `fields` JSON column defining field definitions, driving both edit forms and profile display.

**Consequences:**
- Owners add/remove/reorder fields per entity type per campaign, and add new entity types, without code changes.
- JSON queries are less performant, but entity-type defs are small and cached.

---

## ADR-008: Game Systems as Read-Only Modules

**Status:** Accepted

**Context:** Users want D&D 5e, Pathfinder, Draw Steel reference content as tooltips and pages, without mixing it into user content.

**Decision:** Game systems are "Modules" — a tier separate from Plugins. They ship static data, provide a tooltip API, and render reference pages. Read-only, enabled/disabled per campaign.

**Consequences:**
- Reference data ships with the Docker image; a module's structure is simpler than a plugin's (no service/repo).
- @mentions can reference both campaign entities and module content.
- Must only include SRD/OGL content.

---

## ADR-009: Dual Permission Model (Action vs Content Visibility)

**Status:** Accepted

**Context:** A site admin managing campaigns (delete, force-transfer) should not be spoiled by GM-only content just because they administer the site, especially when the same person also plays in that campaign.

**Decision:** Two distinct permission concepts:
1. **Action permissions** — "can this user perform admin actions?" Checks the `users.is_admin` flag; admin actions go through `/admin` routes only.
2. **Content visibility** — "what content can this user see?" Uses the actual `campaign_members.role` value. No admin bypass for content.

An admin who has not joined a campaign has `MemberRole=RoleNone` (no content access) but can still use the admin panel. Role levels: Player (1) < Scribe (2) < Owner (3); Admin is site-wide, not a campaign role. `RequireRole(min)` checks `MemberRole >= min`.

**Consequences:**
- Admins can play campaigns without spoilers; admin operations stay in `/admin` routes.
- Campaign routes never check `is_admin`, only membership role.

---

## ADR-010: SMTP Password Encryption with AES-256-GCM

**Status:** Accepted

**Context:** The SMTP password must be stored at rest and never returned to the UI.

**Decision:** AES-256-GCM with a key derived from `SHA-256(SECRET_KEY)`; nonce prepended to ciphertext. Decrypted only at send time, never cached. The UI shows `HasPassword: bool` only. An empty password on update keeps the existing one.

**Consequences:**
- No password recovery by design — `SECRET_KEY` rotation makes the stored password unrecoverable; the admin re-enters it.
- If `SECRET_KEY` leaks, the SMTP password is compromised — an accepted tradeoff for self-hosted deployments.

---

## ADR-011: Sidebar Customization via Campaign JSON Column

**Status:** Accepted

**Context:** Owners want to reorder and hide entity types in the sidebar per campaign.

**Decision:** Store sidebar configuration as JSON in `campaigns.sidebar_config` (`entity_type_order`, `hidden_type_ids`); `LayoutInjector` applies it before render. Owner-only; a client-side drag-to-reorder widget auto-saves via PUT.

**Consequences:**
- Malformed JSON falls back to the default `sort_order`.

---

## ADR-012: Entity Type Layout Builder with JSON Column

**Status:** Accepted

**Context:** Entity profile pages need per-type customizable section layouts.

**Decision:** Store layout config as JSON in `entity_types.layout_json`: sections with key/label/type/column (`"left"`/`"right"`) properties; section types are `"fields"`, `"entry"`, or `"posts"`. A client-side two-column drag-and-drop widget edits it.

**Consequences:**
- Sections are validated server-side (valid types/columns, unique keys); a default layout is auto-generated from field definitions when empty.
- The entity show page renders from `layout_json` via `BlockRegistry` + `DefaultLayout()`/`CharacterLayout()`, consumed by `show.templ`.

---

## ADR-013: Pessimistic Locking for Shared Notes

**Status:** Accepted

**Context:** Any campaign member can edit a shared note; without concurrency control, simultaneous edits overwrite each other.

**Decision:** Pessimistic edit locking with 5-minute auto-expiry. A client acquires a lock via `POST /lock`, keeps it alive with a 2-minute heartbeat, and stale locks (no heartbeat for 5 minutes) are reclaimed by the acquisition query itself. Owners can force-unlock any note. Private (non-shared) notes skip locking entirely — only the owner ever edits them.

**Consequences:**
- Only one user edits a shared note at a time; lock state lives on the note row (`locked_by`, `locked_at`).
- Stale locks self-heal via the age check; no cleanup job needed.

---

## ADR-014: Snapshot-on-Save Version History for Notes

**Status:** Accepted

**Context:** Users need to recover previous note versions, especially under shared editing.

**Decision:** A version snapshot is created before every content-changing operation (update, restore). Snapshots store title, content blocks, entry JSON and HTML. Maximum 50 versions per note, oldest auto-pruned; version-creation errors are swallowed since version tracking is non-critical.

**Consequences:**
- Storage grows linearly but is bounded at 50; restore is snapshot-then-apply.

---

## ADR-015: Maps with Percentage Coordinates and Leaflet CRS.Simple

**Status:** Accepted

**Context:** Marker positions must stay correct independent of the uploaded background image's pixel resolution.

**Decision:** Store marker coordinates as percentages (0–100, both axes). Use Leaflet.js with `CRS.Simple` for a non-geographic coordinate system; Leaflet converts percentage to pixel space at render time from the image dimensions stored on the map record. Multiple maps per campaign (unlike calendar's 1:1).

**Consequences:**
- Markers are resolution-independent; the image can be swapped for a different size.
- Leaflet loads from CDN per-page, not globally. Draggable markers PUT silently on dragend.

---

## ADR-016: Inline Secrets via TipTap Mark Extension

**Status:** Accepted

**Context:** GMs need inline secret text within entity entries, visible only to Owner/Scribe. It must be stripped server-side, not just hidden with CSS — CSS-only hiding still ships the HTML to the client, visible in devtools.

**Decision:** A TipTap `secret` mark renders `<span data-secret="true" class="chronicle-secret">`, built by extending `TipTap.Underline` (the vendored bundle exports no raw `Mark` class). `internal/sanitize/` strips it server-side: `StripSecretsHTML()` (regex over the span) and `StripSecretsJSON()` (recursive ProseMirror tree walk), applied in the entry-read handler whenever `role < RoleScribe`.

**Consequences:**
- Secret content never reaches players, in either JSON or HTML.
- The Bluemonday whitelist allows `data-secret` on `<span>`; edit mode shows an amber background + eye-slash indicator for owners/scribes.

---

## ADR-017: Add 'plugin' to Addon Category ENUM

**Status:** Accepted

**Context:** `addons.category` had three values (`module`, `widget`, `integration`); Calendar and Maps are architecturally Plugins, but early seed data miscategorized them as `widget`. Later migrations inserting `category='plugin'` failed with a MariaDB truncation error.

**Decision:** Add `plugin` as a fourth ENUM value; UPDATE (not INSERT) the existing seed rows; add a `CategoryPlugin` Go constant and migration SQL validation tests.

**Consequences:**
- The category ENUM has four values: `plugin`, `module`, `widget`, `integration`.
- Reverting the ENUM requires no rows use `plugin`. `internal/database/migrate_test.go` catches invalid ENUM values at `make test` time.

---

## ADR-018: D3.js for Timeline Visualization

**Status:** Accepted

**Context:** The timeline plugin needs zoom/pan/drag, time scales, and swim-lanes; Leaflet.js (already used for maps) is geographic/tile-based and unsuited to a time axis.

**Decision:** D3.js v7, loaded from CDN per-page (not bundled globally), for SVG-based rendering, `d3.zoom`, and `d3.scaleLinear` time axes.

**Consequences:**
- SVG gives full CSS control, accessibility and crisp text at any zoom level.
- Fantasy calendar dates work naturally by converting to fractional years for `d3.scaleLinear` positioning.

---

## ADR-023: Sessions-Calendar Integration and RSVP Email System

**Status:** Accepted

**Context:** Sessions were a standalone plugin with their own sidebar link and addon toggle; users expected sessions on the calendar, especially real-life-mode calendars, with RSVP from there.

**Decision:**
- Sessions require the calendar addon — no separate "sessions" toggle, no separate sidebar link; sessions open from the calendar's dice icon and Sessions button.
- Sessions display on real-life calendar grids as chips with an inline RSVP modal (Going/Maybe/Can't).
- Recurring sessions: weekly, biweekly, monthly, or a custom N-week interval, stored on the sessions table.
- RSVP by email: `SendHTMLMail` sends multipart/alternative invitations with single-use, 7-day tokens for accept/decline without login.
- `RequireAddon` middleware gates the calendar, maps, sessions, timeline and media-gallery route groups on `AddonService.IsEnabledForCampaign`.
- Session dates render via `FormatScheduledDate()` (`"Mon, Jan 2, 2006"`), not raw ISO 8601.

**Consequences:**
- Disabled addons 404/redirect at the route level, not just a hidden sidebar link.
- `session_rsvp_tokens` cascades on its session FK.

---

## ADR-019: Manifest-Driven Module Framework

**Status:** Accepted

**Context:** The module system was a hardcoded registry of three coming-soon modules with no runtime infrastructure, auto-discovery, or validation.

**Decision:**
1. **manifest.json** — each system declares id, name, version, author, license, categories, API version, entity presets.
2. **SystemLoader** scans `internal/systems/*/manifest.json` and package-installed systems at startup; invalid manifests log a warning without failing startup.
3. **Module interface** is sandboxed: `Info()`, `DataProvider()`, `TooltipRenderer()` — a module can only serve data through these.
4. **DataProvider interface**: `List(category)`, `Get(category, id)`, `Search(query)`, `Categories()`, returning `ReferenceItem` structs.
5. A global `Init()` runs once at startup and populates the singleton registry.

**Consequences:**
- Auto-discovery replaces manual registry maintenance; the admin modules page shows manifest metadata.
- Module slugs are added to `installedAddons` for per-campaign enable/disable.

---

## ADR-020: JSON-File DataProvider with Factory Registry

**Status:** Accepted

**Context:** ADR-019's Module/DataProvider interfaces need a concrete implementation and HTTP handlers, without a circular import between the loader (`modules/`) and module subpackages (`dnd5e/`).

**Decision:**
1. **JSONProvider** — a generic DataProvider that loads `data/*.json` from a module directory (filename stem = category slug) into memory at startup, with case-insensitive search over Name/Summary/Tags.
2. **Factory registry** — modules call `modules.RegisterFactory(id, fn)` in their package `init()`; the loader invokes registered factories during `DiscoverAll()`. `app/routes.go` blank-imports each module (`_ "modules/dnd5e"`) to trigger registration, avoiding the circular import.
3. **Dynamic addon middleware** — module routes use `/campaigns/:id/modules/:mod`, reading `:mod` at request time and checking `addonSvc.IsEnabledForCampaign()` dynamically instead of one route group per module.

**Consequences:**
- A new module needs only manifest.json, data/*.json, an `init()` factory registration, and a blank import in `app/routes.go`.
- Module content appears in entity @mention search when its addon is enabled; `TooltipAPI` renders via the module's `TooltipRenderer`.

---

## ADR-021: Layered Third-Party Extension Strategy

**Status:** Accepted. All three layers are implemented in `internal/extensions/`.

**Context:** Chronicle's Plugins/Modules/Widgets are internal-only; users want to share content, widgets and eventually backend logic without forking. Research across other self-hosted platforms found only sandboxed subprocess/restricted-rendering approaches (Grafana, Shopify) truly isolate plugins; WASM via Extism/wazero is the most promising sandboxed approach for a Go backend.

**Decision:** Three incremental layers:
1. **Content Extensions** — declarative manifest.json + static assets (no code) distributed as zip archives, installed via the admin UI or `extensions/`. Extension data lives in a generic, namespaced extension-data table. No code execution; file types allowlisted.
2. **Widget Extensions** — browser-sandboxed JS that self-registers via `Chronicle.registerWidget()`, bundled inside content-extension zips. Sandboxed by same-origin policy; API calls go through `Chronicle.apiFetch()` (CSRF-protected); no direct filesystem/DB access.
3. **Logic Extensions** — backend logic compiled to WebAssembly, run via Extism + wazero (pure Go, no CGO). Capability-based security: no filesystem, network or database access except through host functions Chronicle explicitly exposes. Distributed as hash-verified `.wasm` files.

**Consequences:**
- Each layer ships independently; later layers don't block earlier ones.
- Manifest format and installer infrastructure are shared across all three.
- Extension signing (SHA-256 manifest checksums) is planned as defense-in-depth for all layers.

---

## ADR-022: WASM Runtime via Extism SDK + wazero

**Status:** Accepted

**Context:** Layer 3 of ADR-021 needs a concrete runtime for community-authored backend logic without direct database or filesystem access.

**Decision:** Extism Go SDK + wazero. Key points:
1. **Capability-based security** — a plugin manifest declares capabilities (`log`, `entity_read`, `calendar_read`, `tag_read`, `kv_store`, plus separate `entity_write`/`calendar_write`); the PluginManager exposes only the matching host functions.
2. **Per-plugin KV store** reuses the `extension_data` table under namespace `wasm_kv`, scoped by campaign + extension id.
3. **Async hook dispatch** — plugins register for manifest `hooks`; events dispatch fire-and-forget in goroutines, so a plugin failure never affects the triggering operation.
4. **Resource limits** — default 16 MB memory / 30s timeout per call, overridable up to 256 MB / 300s; fuel metering is wired but unlimited by default.
5. **Adapter interfaces** (EntityReader, CalendarReader, TagReader) decouple WASM host functions from concrete plugin implementations.

**Consequences:**
- Plugins are truly sandboxed: no filesystem, network or DB access except through declared host functions.
- Any WASM-target language can author a plugin; lifecycle (load/unload/reload) is centralized in the PluginManager.

---

## ADR-024: Extension Migration System (Dynamic Schema)

**Status:** Accepted

**Context:** The core migration pipeline (sequential numbered SQL files) has no mechanism for schema an installed extension needs, or for cleaning it up on removal, without touching the core migration sequence.

**Decision:** Extensions get a separate, per-extension migration system:
1. Core migrations stay as-is and always run.
2. Each extension's manifest declares a `migrations/` directory of numbered SQL files, tracked in `extension_schema_versions` keyed by `(extension_id, version)`.
3. Extension tables MUST be prefixed `ext_<slug>_` to prevent collisions and make cleanup trivial.
4. Lifecycle: install runs `up` migrations in order; uninstall runs `down` migrations in reverse then drops tracking rows and all `ext_<slug>_*` tables; disable/enable touch no schema.
5. Campaign deletion cascades extension data via FKs to `campaigns.id`; non-campaign-scoped data uses the existing `extension_data` table.
6. Extension migrations are validated before execution: only `CREATE`/`ALTER TABLE ext_<slug>_*` is allowed; no DDL on core tables.

**Consequences:**
- Extensions can define proper relational schemas when JSON blobs aren't enough, without risking core tables.
- Uninstalling an extension cleanly removes all its schema artifacts.

---

## ADR-025: Campaign Deletion Cascade and Cleanup

**Status:** Accepted

**Context:** Plain CASCADE left gaps on campaign deletion: media files were only `SET NULL` (orphaned on disk), `api_keys` had no FK at all, and extension-provisioned content needed its own cleanup path.

**Decision:** Campaign deletion becomes a multi-step service operation:
1. Query and delete campaign-scoped `media_files` from disk (main + thumbnails) before the SQL delete; avatars/backdrops (`campaign_id IS NULL`) are unaffected.
2. `api_keys` gets `FOREIGN KEY (campaign_id) ... ON DELETE CASCADE`; API request logs get `ON DELETE SET NULL` to keep the audit trail.
3. Extension-created rows cascade through their own `campaign_id` FKs (already required by ADR-024).
4. WASM plugins with in-memory state receive a `campaign.deleted` hook event to clear caches.
5. Uploaded extensions enabled only for the deleted campaign are flagged for background uninstall if no other campaign uses them.

**Consequences:**
- Deletion is slightly slower (disk I/O) but leaves no orphaned data; API keys are properly invalidated.
- The media `CleanupOrphans()` becomes a safety net, not the primary mechanism.

---

## ADR-026: Admin Data Hygiene Dashboard

**Status:** Accepted

**Context:** The database accumulates orphaned data over time (media without campaigns, API keys pointing at deleted campaigns, orphaned extension records); admins need visibility and guarded cleanup, not silent automated deletion.

**Decision:** `/admin/data-hygiene` page with:
- Read-only orphan-detection scans: campaign-less non-avatar/backdrop media, on-disk files with no DB record, API keys referencing deleted campaigns, orphaned extension provenance and `ext_*` tables, notes for deleted campaigns.
- Safety guardrails: a referenced media file, an extension still enabled somewhere, or a still-existing campaign's keys cannot be purged; every action previews its effect and logs to `security_events`.
- Manual, admin-only, confirmation-required cleanup actions (purge orphaned media / stale files / orphaned API keys; run the media orphan scan with a dry-run option).
- No automated/scheduled cleanup — every action is admin-initiated.

**Consequences:**
- Admins get full DB/filesystem health visibility with no risk of unattended deletion; complements ADR-025's cascade as a catch-all.

---

## ADR-027: RequireAddon Middleware Fail-Open on DB Errors

**Status:** Accepted

**Context:** `RequireAddon` checks whether an addon is enabled for a campaign before allowing its routes; a DB failure must fail open or closed.

**Decision:** `RequireAddon` fails open — if the addon-check query fails, the request is allowed through, since a DB outage breaks every downstream service call anyway and fail-closed just turns a 500 into a less-informative redirect/404. `RequireAddonAPI` (API v1 routes) fails closed instead, since programmatic callers can retry on 503. `Handler.isAddonEnabled()` in entity search follows the same fail-open convention, skipping addon-specific results rather than failing the whole search.

**Consequences:**
- During DB outages a disabled addon's routes may briefly appear reachable; acceptable because the underlying service calls fail anyway.

---

## ADR-028: Plugin-Isolated Database Schema Architecture

**Status:** Accepted; amended by ADR-030

**Context:** 63 sequential migration files mixed core and plugin tables; a bad plugin migration crashed the whole app and left the DB dirty. Plugin failures needed to stop breaking the app, and user-installable extensions needed safe schema isolation.

**Decision:** Two-tier schema system:
- **Tier 1 (Core):** a single baseline migration (`db/migrations/000001_baseline`) with all core tables, run via golang-migrate; failure is fatal.
- **Tier 2 (Plugins):** each built-in plugin has its own `migrations/` directory (`internal/plugins/<name>/migrations/`), run via `RunPluginMigrations()` after core; failure disables only that plugin (tracked in `PluginHealthRegistry`), and routes are conditionally registered on `IsHealthy()`, with a "Feature unavailable" banner shown for degraded plugins.

Version tracking uses `plugin_schema_versions` (separate from `extension_schema_versions`, used by user-installed extensions). SQL validation is skipped for trusted built-in plugins but enforced for user extensions via `ValidateExtensionSQL()` + the `ext_<slug>_` prefix.

**Consequences:**
- Plugin schema failures degrade gracefully instead of crashing the app; each plugin's schema versions independently.
- Cross-plugin FK dependencies require ordered plugin migration execution (e.g. calendar before sessions/timeline).
- Fresh DB only — no backward compatibility with the old 63-migration sequence.

---

## ADR-029: Features Page Consolidation (Plugin Hub + Addon Settings → Single Page)

**Status:** Accepted

**Context:** Feature management was split across a read-only Plugin Hub (`/campaigns/:id/plugins`, visible to all members) and an owner-only Addon Settings page — confusing, and non-owners couldn't tell what was enabled.

**Decision:** Consolidate into one Features page at `/campaigns/:id/plugins`: all members see the card grid with enable/disable status; owners get inline toggle buttons on each card. The old `/addons/settings` route, handler and template are removed; the `/addons/fragment` route remains for the Customization Hub. Toggle forms carry `redirect_to=plugins`.

**Consequences:**
- Single source of truth for feature management; future per-addon enhancements have one target page.

---

## ADR-030: Embed Plugin Migrations via Go embed.FS

**Status:** Accepted; amends ADR-028

**Context:** ADR-028's per-plugin `migrations/` directories were read via `os.Stat`/`os.ReadDir` on real filesystem paths, which worked in dev (CWD = project root) but failed silently in Docker — the runtime image never copies plugin migration directories, so every plugin looked "healthy with 0 migrations": no tables were created, and entity pages crashed.

**Decision:** Embed plugin migration SQL in the binary via `embed.FS`:
- Each plugin package gets an `embed.go` exporting `MigrationsFS embed.FS` (`//go:embed migrations/*.sql`).
- `PluginSchema.MigrationsDir` (string) is replaced by `MigrationsFS` (`fs.FS`); migration parsing reads from `fs.FS`.
- `RegisteredPlugins()` moved from `database` to `cmd/server/main.go` to avoid an import cycle (database can't import plugin packages), using `fs.Sub` to strip the `migrations/` prefix.
- `PluginSchemas` is stored on `App` and passed to the Database Explorer for on-demand re-migration from the admin panel.

**Consequences:**
- Migrations work regardless of working directory or Dockerfile changes.
- Every plugin with migrations must have an `embed.go` exporting `MigrationsFS`.

---

## ADR-031: Auto-Register Game Systems as Addons from Manifests

**Status:** Accepted

**Context:** Game-system addon definitions were hardcoded in `addons.builtinAddons`, so adding a system required both its manifest/data files and a matching `addonDef` entry — blocking self-service system creation.

**Decision:** `systems.AddonInfos()` returns addon metadata for every discovered, available system from its manifest; `addons.RegisterSystemAddon()` appends it to `builtinAddons` and marks it installed. App wiring calls these after `systems.Init()` and before `SeedInstalledAddons()`. The three hardcoded system entries are removed.

**Consequences:**
- New game systems appear as addons automatically, whether from `internal/systems/`, the package manager, or a custom upload.
- `dnd5e`'s blank import in `main.go` is still needed for its custom tooltip renderer; pure-data systems need no import.

---

## ADR-032: Sidebar Navigation Overhaul — Pure Folders & Unified Items

**Status:** Accepted

**Context:** Sidebar folders were entities with `is_folder=TRUE`, polluting search; addon links were hardcoded in `app.templ`; there was no tag filtering or lazy loading for large campaigns; favorites were localStorage-only.

**Decision:**
- **Pure folders:** a new `sidebar_nodes` table holds organizational folders with zero entity records. Entities get `parent_node_id` (mutually exclusive with `parent_id`); `is_folder` is removed from entities.
- **DB-backed favorites:** a new `entity_favorites` table (per-user, per-campaign) replaces localStorage, with toggle/list endpoints and an in-memory client cache.
- **Unified sidebar model:** `SidebarConfig.Items` holds all sidebar content (dashboard, addons, categories, sections, links) in one owner-ordered array; when absent, the legacy format still renders. One sidebar layout editor replaces the separate category-order and custom-links editors.
- **Large-campaign support:** `?tags=` query filtering (AND-logic), lazy loading at 50 entities/page via IntersectionObserver, multi-select bulk move, and a collapsible Manage section.

**Consequences:**
- Folders no longer create entity records or pollute search; favorites persist across devices.
- The dual-parent model (`parent_id` vs `parent_node_id`) requires care to keep mutually exclusive in queries and the reorder service.

---

## ADR-033: Startup Health Check System

**Status:** Accepted

**Context:** An unapplied migration (adding `archived_at`/`join_code` to campaigns) caused runtime errors because repository queries already referenced the new columns — Chronicle had no proactive detection of schema drift or misconfiguration at startup.

**Decision:** `internal/database/healthcheck.go` runs after `RunMigrations()` and before route registration, with five checks: migration version (dirty-state detection), critical columns present (via `information_schema`), DB connectivity + pool-utilization warning, a security audit (weak/default DB password, HTTP BaseURL, overprivileged DB grants, world-writable `schema_migrations`), and a pre-migration `mysqldump` backup (silently skipped if unavailable). The server exits with `os.Exit(1)` on any failed check.

**Consequences:**
- Schema drift is caught before the first request; the DB is backed up before destructive migrations.
- Adds ~100ms to startup; the mysqldump dependency is optional.

---

## ADR-034: Asymmetric Corner Bleed CSS Effect System

**Status:** Accepted

**Context:** Chronicle needed one cohesive, distinctive visual language for interactive elements across the whole UI.

**Decision:** A CSS pseudo-element (`::after` for buttons, `::before` for sidebar nav, to avoid clashing with the icon-only tooltip's `::after`) with stacked `linear-gradient` backgrounds: right edge strongest, bottom heavier than top, bottom-right corner heaviest. Click/active state expands all corners to full width with a 0.2–0.3s transition; a `.btn-pressed` JS class (added on `mousedown`, removed 300ms after `mouseup`, in `boot.js`) adds a tactile linger. Applied across all six button variants and sidebar nav in `static/css/input.css`.

**Consequences:**
- One unified effect, pure CSS except the 300ms linger script; glow is suppressed in icon-only sidebar mode.

---

## ADR-035: Operator Backup as POSIX Shell Script + Make Target

**Status:** Accepted; partly reversed by ADR-036

**Context:** Chronicle needed an operator-runnable backup mechanism. The existing in-process `PreMigrationBackup` covered only a boot-time DB dump and was silently disabled in production because the runtime image didn't ship `mysqldump`; there was no on-demand path, no media/Redis coverage, and no manifest pairing.

**Decision:** `scripts/backup.sh` / `scripts/restore.sh`, invoked via `make backup` / `make restore` / `make backup-check` / `make backup-list`, run inside the chronicle container (or standalone on bare metal). POSIX `sh` (Alpine's `ash`; no bashisms), `set -eu`. Exit codes: `0` success, `1` operator error, `2` precondition failure, `3` backend tool failure. A manifest pairs DB + media + redis artifacts with sha256, Chronicle version and migration version, so restore can refuse a mismatched set. Restore is a sysadmin-only operation — no admin-UI path (reversed by ADR-036).

**Consequences:**
- Operators can update backup logic without rebuilding the image.
- The Dockerfile runtime stage installs `mariadb-client` + `gzip` (~+15MB), which also makes the in-process `PreMigrationBackup` actually work in production.
- Two retention systems coexist by filename prefix: `BackupMaxAge` (hardcoded 7d) for pre-migration files, `BACKUP_RETENTION_DAYS` (default 7d) for operator-script artifacts.

---

## ADR-036: Admin UI for Backup and Restore

**Status:** Accepted; partly reverses ADR-035's sysadmin-only restore stance

**Context:** ADR-035 deferred a web UI on the grounds that restore is destructive enough to want shell-session friction — but operators without host shell access had no realistic recovery path.

**Decision:** Two admin-only plugins:
- `internal/plugins/backup` — `/admin/backup` lists `BACKUP_DIR` artifacts and runs `scripts/backup.sh` synchronously (20-minute timeout).
- `internal/plugins/restore` — `/admin/restore` lists parsed manifests; each row requires typing the literal word `RESTORE` before running `scripts/restore.sh --manifest <path> --yes --force` (30-minute timeout).

On top of `RequireSiteAdmin` + CSRF: an in-process single-flight lock (concurrent requests get 409, never silently coalesced); per-IP rate limits (backup 2/hour, downloads 20/hour, restore 1/hour); every shell-out runs in its own process group so cancel kills all descendants; stdout/stderr capped at 64 KB ring buffers; every filename parameter is validated against `BACKUP_DIR` by basename and prefix (restore additionally requires `chronicle_manifest_*.txt`); restore requires `confirm=RESTORE` in the body, mirroring the script's interactive prompt.

**Consequences:**
- Admins can recover without shell access; `make restore` remains the in-shell escape hatch.
- The UI does not auto-snapshot before a restore — the operator owns that step (the in-process pre-migration backup gives some protection).

---

## ADR-037: Pre-migration backup symmetry with operator backups

**Status:** Accepted; refines ADR-035 and ADR-036

**Context:** `PreMigrationBackup` captured only the database, failed open silently when `mysqldump` was missing (migrations proceeded with no rollback), and stamped no schema version in its filename.

**Decision:** Extend `PreMigrationBackup` to produce output interchangeable with `scripts/backup.sh`: the same artifact prefixes (`chronicle_pre_migrate_db_*.sql.gz`, `_media_*.tar.gz`, `_redis_*.rdb`) and manifest format (`chronicle_pre_migrate_manifest_*.txt`, with `chronicle_version`, `migration_version`, per-artifact sha256+size), plus one distinguishing `chronicle_pre_migrate=1` line. A `BACKUP_REQUIRED=1` env var makes any artifact failure abort startup before migrations apply (default stays fail-open). Artifacts are written atomically (`<file>.partial` renamed only after sha256+size verification), mode `0600`, and a zero-byte artifact is treated as a capture failure.

**Consequences:**
- Pre-migration snapshots are first-class restorable artifacts: `scripts/restore.sh --manifest chronicle_pre_migrate_manifest_<TS>.txt` and the admin restore UI both work with them.
- `redis-cli` is a soft dependency — present, Redis (sessions only) is snapshotted; absent, it's skipped with a debug log.

---

## ADR-038: Widget bindings — polymorphic, FK-free association table

**Status:** Accepted

**Context:** The widget-binding framework maps a host (entity / entity-type / dashboard) to a data instance (a calendar / map / timeline) per widget type. A hard FK is impossible: `instance_id` references a different, plugin-owned table depending on `widget_type`, and core migrations run before plugin migrations, so a binding table can't FK into them.

**Decision:** `widget_bindings(id, campaign_id, host_type, host_id, widget_type, instance_id, …)` is polymorphic and FK-free on both `host_id` and `instance_id`. `host_type`/`widget_type` form an immutable, append-only namespace validated in app code, not a DB enum. The exclusive-arc and join-table-per-type alternatives were rejected: both buy referential integrity at the cost of a migration for every new widget type, which is exactly the hardcoding this framework exists to abolish.

Because there is no DB-enforced integrity, the application is the only backstop, enforced as an AND of three mechanisms:
1. A per-plugin delete hook (`Service.OnInstanceDeleted`) that owning plugins call when an instance is deleted.
2. An always-on render-time orphan guard (`Resolve` validates every candidate via `WidgetType.InstanceExists`, which also enforces campaign scope, and skips/sweeps dead bindings).
3. A periodic campaign integrity sweep (`Service.Sweep`).

Campaign scope is pushed down to the repository signature (an unscoped read is unrepresentable) and checked on both `host_id` and the resolved `instance_id`.

**Consequences:**
- New widget types need no migration; integrity depends entirely on the three mechanisms above staying in place.

---

## ADR-039: Player Character Claiming — Owner-Toggleable Addon + Per-Type Claimable Flag

**Status:** Accepted

**Context:** Chronicle needs bidirectional player-character binding for Foundry sync and campaign management: a GM must know which player owns which character, and a player must be able to claim an unclaimed one. The feature must be opt-in and extensible to more than one character-shaped type.

**Decision:** Three-part design:
1. **Owner-toggleable addon** (`player-character-claiming`): gates creating a "Player Character" sub-type (`preset_category == "player_character"`) and all claiming UI (claim button, owner roster, claimable toggle).
2. **Per-type claimable flag** (`entity_types.claimable BOOLEAN NULL`): when set, the Owner's choice is authoritative; when NULL, the legacy heuristic applies (`preset_category "character"` or slug `*-character`), so existing campaigns keep working without reconfiguration. New types default to claimable=true when the addon is on.
3. **Dedicated PC sub-type + legacy fallback**: new campaigns can use the explicit "Player Character" sub-type; existing campaigns keep claiming on their existing "Character" type via the heuristic. Neither path overwrites the other.

Distinct audit actions (`entity.claimed`, `entity.owner_changed`) record the claiming player and character name. The Foundry sync module, when it detects the addon is on, maps player-owned PC actors to the PC sub-type and auto-claims them without operator configuration.

**Consequences:**
- No migration is required for existing campaigns to keep working; per-type control avoids forcing every character-shaped entity (NPCs, companions) into claimability.

---

## ADR-040: Dynamic-surface frame — a system-agnostic Widget, not a hardcoded sheet

**Status:** Accepted

**Context:** The operator wants a dynamic UI — a mini surface that promotes into a full-screen sheet with expandable boxes, overlays and drill-downs — applied first to the character sheet, later the rulebook.

**Decision:** One reusable, system-agnostic frame in the Widget tier (`Chronicle.surface`, `static/js/widgets/dynamic_surface.js`): a motion-preset library, an overlay stack, an expand/collapse box, a memoized data provider, a mini→full `launch`, and a schema-driven `mount`. The frame owns motion and structure; a System supplies box bodies via `registerBox(name, fn)` and never writes animation code. Built on the existing motion tokens (`--ease/-dur/-elev-*`) plus a new `--surface-*` contract; all presets collapse to a fade under `prefers-reduced-motion`. No new tables — surfaces ride a declarative schema; per-user view-state uses localStorage.

**Consequences:**
- Chronicle owns the template; any System or plugin fills it, for any game system.

---

## ADR-041: `character_surface` as a layout BLOCK + the default for player-character types

**Status:** Accepted

**Context:** Player characters should open the dynamic sheet by default while staying editable in the existing layout customizer.

**Decision:** Register the surface as a normal entity-page layout block (`character_surface`, `Contexts:["template"]`, `Singleton`) whose renderer emits a `data-widget="dynamic-surface"` container with the entity's data seeded inline. `CharacterLayout()` (block + permissions) becomes the default layout for `isPlayerCharacterType` types in `CreateEntityType`, instead of `DefaultLayout()` — applied only to newly created PC types, never rewriting existing customized layouts. The description box mounts the same role-aware `editor` widget the standard `entry` block uses, so GM-only secrets aren't leaked by inlining `EntryHTML`.

**Consequences:**
- Because it's a registry-driven block, owners compose/rearrange it like any other block — no separate "sheet editor."

---

## ADR-042: Cross-plugin section injection — `NPCSectionProvider`

**Status:** Accepted

**Context:** The unified Characters page (core `entities` plugin) must render an NPCs/Monsters section owned by the `npcs` addon, without `entities` importing `npcs` (rule 8) or duplicating NPC logic.

**Decision:** `entities` defines an `NPCSectionProvider` interface returning a `templ.Component`; `npcs.Handler.NPCSection` structurally satisfies it and is injected via `entityHandler.SetNPCSectionProvider(npcHandler)` at app wiring. `npcs` renders its own section and `entities` slots it in when the `npcs` addon is on. The standalone `/npcs` gallery page now redirects into this.

**Consequences:**
- Keeps domain ownership in `npcs` and the dependency direction one-way (npcs→entities); any addon can contribute a section to a core page this way.

---

## ADR-043: Extension Settings / Onboarding framework (`SetupProvider`)

**Status:** Accepted

**Context:** Enabling an addon fired silent lifecycle hooks (`ApplySystemPresets`, `ApplyAddonEnableEffects`) that hid real decisions inside boot automation, producing the duplicate-player-character-category artifact (ADR-044). The owner wanted each extension to own a visible settings/onboarding page.

**Decision:** A Go `SetupProvider` interface + slug-keyed registry in the `addons` plugin, supplying `RunChecks` (health/QOL findings with a severity), `Questions` (onboarding inputs) and `Apply` (idempotent). A generic handler + three Templ components render any provider as a full page or a modal overlay. Concrete providers live in the app layer and are wired via `addonService.RegisterSetupProvider(...)` in `app/routes.go`, so `addons` never imports `entities`/`systems`. Per-campaign setup state (`{completed, dismissed, answers}`) persists under `campaign_addons.config_json["setup"]` — no new table. The Extensions hub shows a "Setup" badge + "Open setup" button when `NeedsSetup` is true.

**Consequences:**
- Enabling stays safe (idempotent effects like `EnsurePlayerCharacterType` still run) while destructive/ambiguous choices move into an owner-driven wizard; new providers need zero template changes.

---

## ADR-044: PC duplicate reconciliation moves from boot migration → owner-triggered Apply

**Status:** Accepted; amended after a production incident (see the migration-safety rule below); extended by ADR-045 and ADR-050.

**Context:** A campaign could end up with both a generic "Player Characters" type and a game system's own character type (e.g. Draw Steel's "Heroes") — an enable-ordering artifact. The one-time, guarded boot migration `000030_consolidate_player_character_duplicate` auto-merges the unambiguous case (exactly one of each) on deploy.

**Decision:** Keep migration `000030` permanently — it is applied in production and is idempotent/guarded. Additionally, an owner-triggered service method (`entities.MergeDuplicatePlayerCharacterType` + repo `MoveEntitiesAndDeleteType`, one transaction), surfaced on the player-character extension settings page (ADR-043), handles what the one-time migration doesn't: ambiguous campaigns (more than one of either type — returns a human-readable error) and duplicates that arise after the migration ran. It classifies the unambiguous pair by `preset_category`/`slug`/`is_default` only, moves the generic type's entities onto the system type (claims follow via `entities(id)`), and deletes the emptied generic type; the owner also picks the system name vs. a custom name in the same wizard.

**Migration safety — the incident.** An earlier draft of this ADR deleted migration `000030`. golang-migrate's `file://` source requires a migration file for every version up to the DB's recorded version; a production DB at `version=30` with no `000030` file fails unrecoverably (`no migration found for version 30`) — an infinite boot loop, since the runner only auto-recovers `ErrDirty`, not a missing-version source. **Rule: never delete or renumber a migration any live database has applied — keep it forever, even superseded.**

**Consequences:**
- Existing production duplicates heal automatically via `000030` (unambiguous case) and can be reconciled by the owner for any case; no migration is ever removed. The owner-merge is idempotent.

---

## ADR-045: Migration robustness — fail-safe boot, append-only guards, schema-only policy

**Status:** Accepted; extends ADR-044

**Context:** Deleting an applied migration crash-looped production, from three compounding causes: golang-migrate's `Up()` hard-errors when the DB version exceeds the on-disk source's highest version (true both for a deleted migration and an ordinary image rollback); `restart: unless-stopped` turned the fatal boot into a ~1/sec loop; and the pre-migration backup ran unconditionally on every boot, so each loop iteration wrote a full dump.

**Decision — three layers.**
1. **Runtime (boot fails safe).** `database.MigrateWithBackup` reads the DB version and highest on-disk migration once: if the DB is ahead of the build, it logs a warning and starts anyway (migrations are additive, so an older binary runs fine on a newer schema); if up to date, it skips both backup and `Up()`; if pending, it backs up then migrates. A dirty database now fails fast with restore guidance instead of auto-retrying `Force(v-1)` forever. `fatalBoot` sleeps `BOOT_FAIL_BACKOFF` (default 45s) before exit so unrecoverable errors retry at ~1/min instead of ~1/sec.
2. **CI guards.** `tools/check-migration-immutability.sh` fails any PR that deletes or edits a migration already on the base branch. `internal/database/migrate_test.go` pins the expected core migration version to the actual max, lints for idempotent DDL (grandfathering pre-existing non-idempotent files), and checks gapless numbering and plugin up/down-pair coverage.
3. **Visibility.** `/admin/database` is one tabbed page: **Migrations** (core version, dirty flag, pending count, a DB-ahead/downgrade banner, the per-plugin grid, Apply Pending, history), **Health** (the same `RunStartupHealthChecks` the boot path runs, so boot and the admin tab can never disagree — `GET /admin/database/status` exposes it as JSON), **Backups** (the backup/restore plugins, Auto vs. Manual badges), and **Schema** (the D3 diagram, mounted lazily so it reads a real container width). `admin` defines `HealthChecker`/`BackupLister` interfaces and the app layer injects adapters, so `admin` imports neither plugin.

**Policy — migrations are append-only and schema-only.** Never delete, edit, or renumber a migration any live DB may have applied. New DDL must be idempotent (`IF [NOT] EXISTS`). One-time data corrections do not go in migrations — use an idempotent reconciler (an `EnsureX`/`MergeX` service method run from a boot backfill, an addon-enable hook, or an owner-triggered `SetupProvider`).

**Consequences:**
- Upgrades and rollbacks either work or fail with a clear message; the whole incident class (delete/edit/renumber/gap/non-idempotent/version-drift) is blocked at PR time.

---

## ADR-047: World-state broadcasts are audience-SPLIT, not audience-filtered

**Status:** Moot. The code it governed was deleted in #595; the calendar is
being rebuilt, #741.

It fixed calendar world-state events (weather, celestial) that were silently
dropped before reaching any WebSocket client, by splitting each broadcast
into a player-safe payload and a separate DM-only payload instead of
omitting GM-only detail from one shared payload.

Full text: https://github.com/keyxmakerx/Chronicle/blob/dfc73c78/.ai/decisions.md

---

## ADR-046: Calendar events get first-class RSVPs, distinct from session attendance

**Status:** Moot. The code it governed was deleted in #595; the calendar is
being rebuilt, #741.

It gave calendar events (festivals, downtime windows, one-off scenes) their
own RSVP storage and service, separate from session attendance, gated by the
calendar's existing visibility check and kept out of campaign/AI export.

Full text: https://github.com/keyxmakerx/Chronicle/blob/dfc73c78/.ai/decisions.md

---

## ADR-048: calendar-v4 — the Block is the calendar, and its honesty states are load-bearing

**Status:** Moot. The code it governed was deleted in #595; the calendar is
being rebuilt, #741.

It replaced the month-grid calendar with a four-zone "Block" component and
ruled a set of "honesty states" — cases where the UI must show absence or
uncertainty rather than guess at missing data.

Sections 17 and 18 (the availability overlay's role vocabulary and
per-member timezone display) are still live; that guidance now lives in
`internal/plugins/sessions/.ai.md` under "Role and Zone Display Rules".

Full text: https://github.com/keyxmakerx/Chronicle/blob/dfc73c78/.ai/decisions.md

---

## ADR-049: "no authenticated user" and "trusted system caller" are two states, not one empty string

**Status:** Accepted

**Context:** The calendar and timeline visibility filters treated an empty `userID` as meaning "trusted system caller" — but an anonymous HTTP request on a public campaign also carries `userID == ""` (`auth.GetUserID(c)` returns `""` with no session). So the most privileged branch of the filter was the one logged-out traffic took: `dm_only` calendars/timelines and per-user-restricted events were served to a visitor who never logged in.

**Decision:** The two states get two representations, and the trusted one is unforgeable from request data. `internal/permissions/viewer.go` defines `Viewer` with an unexported `system` bool and exactly two constructors: `RequestViewer(role, userID)` (anything from HTTP; an empty `userID` means anonymous and can never produce a system viewer) and `SystemViewer(role)` (a trusted in-process caller, stated at the call site). Every visibility filter asks `Viewer.SkipsPerUserRules()` (`system || CanSeeDmOnly(role)`) instead of testing the user id itself, so an anonymous viewer falls to the least-privileged path by construction. An empty user id is an absent per-user layer — never a sentinel, never substituted with a synthesized identity.

**Consequences:**
- The timeline's create-or-pick picker and the campaign timeline export adapter pass `SystemViewer` explicitly, with unchanged shipped behavior.
- `TimelineService` methods take a `permissions.Viewer` instead of `(role, userID)`; exported service methods build a `RequestViewer` at their boundary.

**Amendment:** `SkipsPerUserRules` correctly sends an anonymous viewer through the same per-user check as everyone else — but every deny-list predicate that check reaches (maps' `VisibilityRules.Allows` and its SQL twins in `ListMarkers`/`ListDrawings`, the WebSocket hub's `Message.AudienceAllows`, timeline's `canUserView`) matched a `DeniedUsers` entry by literal userID equality, and an anonymous request's userID is always `""` — never equal to a real denied id — so a deny list alone did not exclude anonymous. A player denied an item could log out and see it. The invariant: **whenever an item carries a non-empty deny list, an anonymous viewer is denied by it too**, since an anonymous session can't be proven not to be the specific player named. `permissions.DeniesAnonymous(deniedUsers, userID)` states this once; all four predicates call it before testing membership.

---

## ADR-050: An immutable plugin migration is repaired by a reconciler, and a half-applied one resumes instead of replaying

**Status:** Accepted; extends ADR-044/045 and ADR-028/030

**Context:** Two failures were invisible to CI because nothing in CI ever migrated a genuinely empty database. First, `foundry_vtt` migration 001 (`RENAME TABLE foundry_module_campaign_tokens TO foundry_vtt_campaign_tokens`) crashed on every fresh install because its source table's plugin had already been deleted — and `runSinglePluginMigrations` stops at the first failed migration, so no later, fresh-DB-safe migration for that plugin could ever run. Second, a plugin migration that failed partway through left earlier statements' effects in the database with no record anything had happened (MariaDB has no transactional DDL), so the next boot replayed from statement one and hit "duplicate column name" — an artifact of the retry, not the real cause.

**Decision:**
1. **An immutable migration that cannot run is repaired by a Go-side reconciler plus a new append-only migration — never by editing the old one.** `foundry_vtt.ReconcileConsolidationState` records 001 as applied wherever its RENAME has no source table; a new idempotent migration `002_ensure_campaign_tokens` states the post-consolidation shape. 001 is untouched, so the immutability guard stands, and a database that still has the predecessor table is left alone — 001 runs there for real.
2. **A plugin migration gets a pre-flight applicability check and partial-progress recording, not a transaction.** Every statement is validated against the schema catalogue, simulated forward, before any statement runs; if any statement can't possibly succeed, the migration aborts having executed nothing. If a statement fails anyway, the count that DID apply is recorded in a new runtime table `plugin_migration_progress`, keyed by a sha256 of the migration text and honoured only on a byte-identical match, so the next boot resumes after them instead of replaying.
3. **Every uncertain answer degrades to the pre-existing behaviour.** The pre-flight fails open when `information_schema` can't be read; an unmatched or missing progress row replays from zero.
4. **The guard is a fresh-DB replay, and a skip is a failure.** `make test-freshdb` replays the real bootstrap against genuinely empty schemas (both a from-zero and a pre-consolidation shape) as its own CI job, which greps for PASS on each named test so a silent skip fails the job.

**Consequences:**
- A plugin whose old migration is unrunnable now has a sanctioned repair that never touches the immutability guard, at the cost of stating the canonical schema in two places.
- Boot-time recovery semantics changed for all plugins: a failing migration may now abort earlier (pre-flight) or resume later (progress) — both strictly safer than replay, both falling back to replay when unsure.
- `plugin_migration_progress` is created by the runner itself, not by a migration, since it must exist before any plugin migration runs.

---

## ADR-051: The server records its own recent errors in a bounded in-memory ring, not a table

**Status:** Accepted

**Context:** Chronicle had no visibility into its own running process — no build/commit/uptime info and no trace of recent errors reachable by an admin. Errors went to `slog` → stdout → the container log driver, so "what broke overnight?" required shell access. The audit plugin can't serve this: it's DB-backed, keyed by campaign and user, and has neither key for an anonymous request or a `/healthz` 500.

**Decision:** A new leaf package `internal/observability` holds a fixed 256-slot, mutex-guarded ring of recent server errors, allocated at package init so it records from the first request. Read through the diagnostics catalog as `host.errors`/`host.errors-summary`.
1. **What's recorded** — 5xx responses and recovered panics only, decided by a named function (`ShouldRecord`), because eviction (not volume) is the risk: a 404 storm could evict the one 500 that matters. Each entry holds time, status, method, route template, a `Kind` (`KindApp` for a deliberate `apperror.AppError`, `KindRaw` for an error that escaped a handler), and the error string.
2. **What's deliberately not recorded** — no headers, bodies, query strings, user or campaign id. `PathFor` stores the route template, never the concrete path, since Chronicle routes carry live tokens (`/rsvp/:token`, `/join/:code`); the error string is capped at 300 bytes and passes through the existing secret redaction.
3. **In-memory, not persisted** — a table would need a migration, a retention policy, and would fail exactly when the database is the thing that's broken. A restart empties the ring, and each replica keeps its own; this cost is stated in the output, not hidden.
4. **Three distinct renders** — "provider not wired", "wired and holding zero", and "the ring wrapped, N evicted" must never look alike; an unwired provider explicitly denies meaning "no errors occurred."
5. **Two write hooks** — `app.errorHandler` records then delegates unchanged; `middleware.Recovery` records the panic value separately, since a recovered panic never reaches the error handler.

**Consequences:**
- An admin answers "what broke overnight?" from the admin UI with no shell access.
- Errors outside the two HTTP hooks (a service's `slog.Error`, a background goroutine) are still invisible here — a `slog.Handler` tee was considered and deliberately not built.
- The ring is not an audit trail and must never be cited as one.

---

## ADR-052: The per-day moon discs get their own container-query threshold, not the named-event one

**Status:** Moot. The code it governed was deleted in #595; the calendar is
being rebuilt, #741.

It gave the per-day moon-disc row its own, lower container-query width
threshold, separate from the named-event threshold, after a census showed
the discs were unreachable on phones and on any long in-world week.

Full text: https://github.com/keyxmakerx/Chronicle/blob/dfc73c78/.ai/decisions.md

---

## ADR-053: The Sync API toggle refuses the Bearer key, not the route — and enabling it is a decision only a human or a key-creation makes

**Status:** Accepted

**Context:** The `sync-api` addon toggle did nothing: `/api/v1` mounted `RequireAuthOrAPIKey` with no addon check, so a valid Bearer key worked regardless of the toggle, on both REST and the WebSocket (`AuthenticateKeyForWS`). Only `calendar`/`maps` sub-groups were gated via `RequireAddonAPI`.

**Decision:**
1. `RequireSyncAPIAddon` gates both `/api/v1` groups but short-circuits for the synthetic key a browser session resolves to — `sync-api` is an integration toggle ("no outside clients"), and Chronicle's own widgets authenticate on the same routes by session cookie; reusing `RequireAddonAPI` (which gates every caller) would break them.
2. It answers `403 sync_api_disabled`, not 404 — a 404 makes the Foundry module treat the Chronicle instance as too old, hiding the real cause.
3. It is not folded into `AuthenticateKey`, which stays answering "is this token a live key?"; each transport shapes its own refusal.
4. The WebSocket is enforced at connect only — dropping live sockets would make the toggle stronger than key revocation itself, which also only takes effect at reconnect.
5. `syncAPIService.CreateKey` re-enables the addon for the campaign after the key row commits, best-effort, so a campaign minting its first key isn't left with a dead token until the next restart.
6. A boot reconciler (`syncapi.ReconcileAddonEnablement`) enables `sync-api` only for campaigns that own a key and have no `campaign_addons` row at all — never overriding a row an owner explicitly set to off.
7. An unwired gate refuses: if `SetAddonGate` is ever not injected, `AuthenticateKeyForWS` fails closed with a distinct internal error rather than assuming permission.

`/api/version` and the Foundry module manifest/zip routes (their own per-campaign signed token) stay ungated by design.

**Consequences:**
- The toggle actually cuts off external Bearer-key access while leaving first-party browser callers and the operator's own key-management path unaffected.

---

## ADR-054: The sync API is a second door to the same rooms, and it checks the same locks

**Status:** Accepted

**Context:** Six features across three plugins (fog of war, map layers, marker/drawing/token delete, Player Notes, Notes) enforced a role on the web route but only a coarse permission tier on the `/api/v1` twin, because `RequireAuthOrAPIKey` synthesises permissions from a campaign role (Owner: read/write/sync, Scribe: read/write, Player: read) and every syncapi route checked only that permission — so a Player's own session could read fog of war the web UI refuses them.

**Decision:**
1. **The invariant:** for every resource reachable through `/api/v1`, the API route's authorization is at least as strict as the web route's for the same action.
2. **The mechanism:** routes declare a role floor, not only a permission. The role comes from the session's live membership role for the synthetic key, and is fixed to Owner for a real stored Bearer key (keys are strictly Owner-minted, and the WebSocket path already granted any valid key Owner role) — deliberately decoupled from the creator's current membership, so a key outlives its creator losing access, with `flagIfKeyOwnerLostAccess` surfacing that loudly instead of degrading quietly.
3. **The guard:** each syncapi handler resolves one role (`resolveRole`, pinned by `resolve_role_test.go`) and hands it to the same service-layer filtering the web path uses, so the two surfaces can't drift onto different predicates for the same resource.

A fourth permission tier (`PermGM`) was rejected: it would have required re-minting every existing integration key before map sync worked again.

**Consequences:**
- The web and API surfaces share one predicate per resource instead of two that can silently diverge.

---

## ADR-055: Visibility stays per-subsystem; the read-path rule is the one thing they share

**Status:** Accepted

**Context:** Chronicle carries five visibility models for five kinds of content: entities (`is_private` + `custom` mode with per-subject grants), markers/drawings/timeline (`visibility` enum + `visibility_rules`), tokens (`is_hidden`), notes (owner/`is_shared`/`shared_with`), and maps/sessions (none — shared by nature). The campaign `DefaultVisibility` setting is scoped to entities only, everywhere it's read. A sweep found no creation-path inconsistency but four read-path leaks: a session page naming linked entities without checking privacy, fog of war and GM layers listed to any player via the API, and timeline `EventCount` leaking the existence of per-user-hidden events.

**Decision:**
1. **The models stay separate.** A note is personal by nature, a marker on a shared map is public by nature, an entity is DM-curated — they differ because the content differs. Unifying them is a rewrite on the scale the calendar shows the cost of.
2. **The campaign default stays entity-only.** If a DM wants a hidden marker, they hide the marker — one click on an object, not a campaign-wide policy.
3. **The one rule every subsystem shares is on the read side: any query that returns content to a non-Owner filters by that subsystem's own visibility model, server-side, so hidden content is absent — not greyed, not counted, not named, not ordered around.**
4. **The four leaks are fixed under rule 3**, each with its own model: session entity names, the timeline count, and fog of war/GM layers over the sync API (ADR-054).

**Consequences:**
- A campaign-wide visibility default for everything, and a creator-only "Private" entity mode, were both rejected (see ADR-056 for the latter).

**Amendment:** Decision 4 said the timeline `EventCount` leak was fixed; it wasn't — the fold only reached the `dm_only` base-visibility predicate in the COUNT subqueries, never the per-user `visibility_rules` (`allowed_users`/`denied_users`) that resolve in Go (`canUserView`). A Player excluded from an event only by rules still saw a count one higher than the events they could open, an existence oracle. Closed by reusing rule 3's own read-path filter instead of duplicating it in SQL: `timelineService.ListTimelines`/`ListTimelinesForCalendar` recount each timeline's `EventCount` with the same per-event filter (`filterEventLinksByUser`) `ListTimelineEvents` applies to its rows, for any viewer that doesn't skip the per-user layer (`SkipsPerUserRules`); Owners/co-DMs and system callers keep the cheap SQL count since they see every event regardless.

---

## ADR-056: A toggle says what it does — code where the label is a promise, copy where the label is a name

**Status:** Accepted

**Context:** A sweep of every addon toggle found three defect shapes: Sync API overpromised (labelled a cutoff, cut off nothing — ADR-053); Sessions was misnamed (assumed dead, gated one dashboard block while its real routes sat under Calendar); and 7 of 14 addons showed "Campaign extension." as their whole description on the enable/disable screen because `PluginHubAddon` had no `Description` field, though the real text existed in `builtinAddons`.

**Decision:** The test: if an owner would flip a toggle believing it protects or cuts something off, the gate must be real — code; if the label merely points at the wrong thing, the label moves — copy. Applied: Sync API and Player Notes (five ungated routes) and the floating Notes toggle got code fixes (routes gated on the addon for every caller, since these are feature toggles, not integration ones); Sessions, the Co-DM "control the live world-state" clause, the Calendar card, and Media Gallery's "upload" wording got copy fixes; the Default Visibility "Private" option was removed rather than built, since creator-only visibility already exists per-entity via `custom` mode and a campaign-wide creator-only default would hide a DM's work from their own co-DM; all 8 generic addon descriptions now thread `Description` through `PluginHubAddon`; the drifted, dead `settingsFeaturesTab` (~900 lines) was deleted.

Also under this ADR: the partial-update contract test only recognised structs named `Update*Input`, so every `Update*Request` — including `tags.UpdateTagRequest`, where a rename could turn off DM-only — was invisible to it. The scanner now widens to `*Request`.

**Consequences:**
- Campaigns already storing `"private"` keep behaving as `is_private=true`; the constant stays for compatibility but the radio button doesn't.

---

## ADR-057: One visibility glance, shown to those who can change it, edited only in edit mode

**Status:** Accepted; amended to Option B — editing opens from the glance icon (click, Owner only) as the widget's existing right-edge slide-in card, rather than a hover popover. Hover stays the read-only key; the edit form's old inline mount is retired.

**Context:** An operator asked for one visibility icon near the entity name that the DM team can glance at and click through to edit, replacing four separate implementations of the same three-glyph vocabulary (`fa-globe`/`fa-lock`/`fa-shield-halved`) — one of which (the "Details" card) showed even to Players, hard-coded a color no theme defines, and one of which (the "Permissions" row) was the editor bolted onto the read page. Two defects were found alongside: a Co-DM could not open a DM-only entity, because most `CheckEntityAccess` call sites passed raw `MemberRole` instead of `VisibilityRole()` (which promotes a DM grant to Owner); and a Scribe editing a DM-only entity was shown a wrong, default "Permissions · Everyone" because the widget swallowed a 403 on load and rendered its init defaults.

**Decision:**
1. **One component.** `visibilityGlance(state, viewer)` in the entities plugin replaces all four, keeping the same three glyphs, at the header position beside the name.
2. **Seen by the DM team, never by Players or visitors** — gated on `VisibilityRole() >= RoleScribe`. A badge visible to a Player would itself be evidence of hidden structure (ADR-055 rule 3).
3. **Colour from tokens** — `var(--color-accent)` for custom, `--color-fg-muted` otherwise; the hard-coded `#0d9488` is deleted from the tree and guarded by a render-contract test.
4. **Hover is a key, not a tooltip** — a portalled popover naming who has access (Everyone / DM team only / the actual custom grants), replacing native `title=`.
5. **Editing lives in edit mode only** — the auto-appended "Permissions" row and its heal goroutine are removed from the read page; the edit form's inline widget is the one editor.
6. **The two defects are fixed under this ADR, first**, needing no design: every `CheckEntityAccess` caller passes `VisibilityRole()`; the widget renders nothing on a failed load, never a default.
7. **`public` as a grant subject stays unfinished, on purpose, until wired as its own later slice** — migration `000028` added the enum value, but `ValidSubjectType` still refuses to write it. Half-wired is worse than absent.
8. **No "the party" audience** — a `role:1` grant ("every Player") is the working equivalent of a seeded Party group, which is not built.

**Consequences:**
- A single implementation and a single security predicate for entity visibility, instead of four independently-drifting ones.

---

## ADR-058: A picture inherits the permissions of the pages that use it

**Status:** Accepted; operator-ruled.

**Context:** A security audit found `checkMediaAccess` decided access from campaign membership alone, role-blind, never consulting the visibility of the entity a file hangs off (`media_files` carries no such reference) — so any campaign member could read any image, including DM-only artwork. Two facts shape the fix: a picture can be referenced by several entities (main image, cover, or inline in entry HTML), and Chronicle merges identical uploads by content hash, so a file can end up shared between pages nobody deliberately linked.

**Decision:**
1. **A picture is readable if at least one page using it is visible to the viewer** — not hidden if any page is hidden. The strict rule would break a public page with a dead image, which is itself a "something is being withheld" signal (ADR-055 rule 3), and is futile besides: the picture is already on screen wherever it's visible.
2. **This protects the picture, not the fact of reuse.** Putting a hidden page's artwork onto a visible page publishes that artwork; no access rule can undo it.
3. **Files no page references keep today's behaviour** — campaign membership at the existing threshold, since avatars, backdrops and freshly uploaded files have no owning entity by construction.
4. **The "where is this used" list becomes part of the permissions story** — `FindReferences` must be shown to an owner before publishing, not buried in one Owner-only fragment.
5. **Merging across different permission levels is refused, and the author is told** — a silent merge is how a secret map's artwork becomes reachable months later with nobody deciding anything.
6. **A signed URL is bound to the viewer** — today it's a bearer-token HMAC over `fileID:expires` valid an hour; binding identity in means a copied link is inert for anyone else. This doesn't replace decision 1 — a member can still mint their own link — it stops the link leaving.
7. **A public campaign stops serving every file unsigned** — unsigned anonymous access narrows to pictures used by pages an anonymous viewer may actually see (decision 1 with `RoleNone`), instead of every image in a public campaign.

**Consequences:**
- Access requires a lookup per image request, mitigated by caching the decision per (file, viewer); if the cache is unavailable the rule still applies, just slower.
- `FindReferences` must cover `cover_image_path` (not just `image_path`/`entry_html`) or the rule leaks through the missing column — this had to ship first, not later.
- Someone will lose access to an image they can see today; that's the point, and belongs in release notes, not a silent deploy.

---

## ADR-059: Chronicle stays separate from Grimoire; they integrate at the login and the link

**Status:** Accepted; operator-ruled. Open work: #634 (SSO/OIDC login, the first integration seam below).

**Context:** The operator asked whether Chronicle should merge into hunter-read/grimoire. Grimoire is a file-library manager (PDF indexing, battlemap images, tokens, audio, a flat markdown wiki for campaign tracking) with no entity, relation or timeline concept; Chronicle is a structured world (typed entities, a relations graph, a timeline, interactive maps, tag/group permissions, bidirectional Foundry sync). The stacks are disjoint (Python/FastAPI/SQLite/React vs. Go/Echo/MariaDB/HTMX) and grimoire has no plugin host to embed an application into.

**Decision:** Keep them separate, run both, and integrate at two seams:
1. **One login** — put both behind one OIDC identity provider (grimoire already speaks it; Chronicle doesn't), for one login and one user list.
2. **Cross-links, with a division of labour** — grimoire owns the library (PDF, battlemap image, audio track); Chronicle owns the world and links out to grimoire's resources via its published OpenAPI.

**Consequences:**
- Where both run, Chronicle's media scope may narrow to "pictures attached to entities," simplifying (not invalidating) `.ai/designs/2026-09-13-media-renovation.md`.
- Worth reimplementing from grimoire: OIDC, guest invite codes, a per-user revocable `.ics` session feed, a player-safe export, LegendKeeper import. Not worth reimplementing: PDF indexing, audio, 3D models, token/UVTT editors.

---

## ADR-060: A deleted world page is marked, not removed, until the Trash retention runs out

**Status:** Accepted; operator-approved design (page history, Trash, save clashes; #1006).

**Context:** Deleting a page ran `DELETE FROM entities`, which cascades into about twenty tables (relations, tags, permissions, map pins, calendar ties, session links, favourites, inventory). A Trash that copies the page out and re-inserts it on restore would have to know every one of those tables and keep up with each new one; missing one silently loses data on restore.

**Decision:**
1. Delete sets `entities.deleted_at`/`deleted_by` and stamps the page and its live sub-pages with one `trash_root_id`, so they restore together. Nothing that hangs off the page is touched.
2. Every read of `entities` excludes trashed rows. Inside the entities plugin `visibilityFilter` carries `liveOnly` for every role, owners included; reads that skip it add `liveOnly` (`FindByID`, `FindBySlug`, `ListByOwner`, sibling ordering). Other plugins add `deleted_at IS NULL` to their own joins (in the `ON` of a `LEFT JOIN`, so a row shows as if its page were gone).
3. Reads that must still see trashed pages say so: media "is this file used" checks (a picture on a trashed page must survive cleanup), the slug-uniqueness check, the sync-mapping list, and the armory ledger's historical names.
4. An hourly purge hard-deletes pages trashed longer than the site's `content.trash_retention_days` (30/60/90/180/365, default 30); the ordinary cascades then run.

**Consequences:**
- A new query against `entities` must exclude trashed rows, or a deleted page reappears in it. The Trash has no "delete forever" yet; the purge is the only hard delete.
- A trashed page still holds its slug, so a new page with the same name gets a suffixed slug.

---

## ADR-061: In the armory a DM grant acts as the Owner

**Status:** Accepted; operator ruling 2026-09-13 (#633).

**Context:** `VisibilityRole()` promotes a member the owner has given DM access to Owner, and is documented as a visibility rule. The armory's stash and shop handlers pass that promoted role into their services, so the grant also decides who may answer stash and purchase requests, open or close downtime and buy for any character. The operator ruled that such a member may buy on a player's behalf, not only see.

**Decision:** Armory handlers build their `Actor` from `VisibilityRole()`. Inside the armory, `Actor.IsOwner()` is true for the Owner and for members with DM access; Scribes stay below it. Other plugins keep `VisibilityRole()` for visibility only and use their own named checks (`CanControlWorldState`, `CanAuthorDmOnly`) for actions.

**Consequences:**
- Taking away a DM grant also takes away these armory actions on the next request.
- A new armory action that must be the Owner alone needs its own check on `MemberRole`; `IsOwner()` will not tell them apart.
