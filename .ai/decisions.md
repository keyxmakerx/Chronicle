# Architecture Decision Records

<!-- ====================================================================== -->
<!-- Category: Semi-static (APPEND-ONLY)                                      -->
<!-- Purpose: Records WHY decisions were made. Prevents revisiting settled     -->
<!--          questions. Existing records are NEVER modified except to         -->
<!--          change status to "Superseded by ADR-NNN".                       -->
<!-- Update: Append a new record when a significant decision is made.         -->
<!-- Template: See .ai/templates/decision-record.md.tmpl                      -->
<!-- ====================================================================== -->

---

## ADR-001: Three-Tier Extension Architecture (Plugins, Modules, Widgets)

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Chronicle needs complete compartmentalization. Every feature should
be its own self-contained unit. But there are fundamentally different kinds of
extensions: full feature apps, game system content packs, and reusable UI pieces.

**Decision:** Three tiers:
- **Plugins** (`internal/plugins/`): Feature apps with handler/service/repo/templates.
  Core plugins (auth, campaigns, entities) always enabled. Optional plugins
  (calendar, maps, timeline) enabled per-campaign.
- **Modules** (`internal/modules/`): Game system content packs (D&D 5e, Pathfinder,
  Draw Steel). Reference data, tooltips, dedicated pages. Read-only.
- **Widgets** (`internal/widgets/`): Reusable UI building blocks (editor, title,
  tags, attributes, mentions). Mount to DOM, fetch own data.

**Alternatives Considered:**
- Flat `internal/modules/` for everything: conflates apps with UI components
  and content packs. Naming becomes ambiguous.
- Plugin-only: widgets and modules have fundamentally different structures.

**Consequences:**
- Clear separation of concerns per tier.
- Each tier has its own directory structure template.
- Cross-tier deps flow downward: Plugins may use Widgets. Modules may use
  Widgets. Widgets are self-contained.

---

## ADR-002: MariaDB Over PostgreSQL

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Original spec called for PostgreSQL, but deployment target (Cosmos
Cloud) and user infrastructure use MariaDB.

**Decision:** MariaDB with `database/sql` + `go-sql-driver/mysql`. No ORM.

**Alternatives Considered:**
- PostgreSQL: richer features (JSONB, tsvector) but doesn't match user infra.
- SQLite: doesn't support concurrent writes for multi-user web app.

**Consequences:**
- No JSONB -- use MariaDB `JSON` columns (validated on write).
- No `tsvector` -- use MariaDB `FULLTEXT` indexes.
- No `gen_random_uuid()` -- generate UUIDs in Go (`uuid.New()`).
- Use `?` placeholders instead of `$1` in SQL.

---

## ADR-003: Hand-Written SQL Over ORM or sqlc

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Need a SQL layer. Options: ORM (GORM), code generator (sqlc),
hand-written.

**Decision:** Hand-written SQL in repository files.

**Alternatives Considered:**
- GORM: magic behavior, N+1 queries, hard to optimize.
- sqlc: excellent for Postgres but MySQL support is immature.

**Consequences:**
- Full control over query performance.
- More verbose but explicit.
- Each repository is self-contained.

---

## ADR-004: HTMX + Templ Over SPA Framework

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Frontend needs interactivity without Node.js build chain.

**Decision:** Server-side rendering with Templ + HTMX. Alpine.js for
client-only interactions.

**Alternatives Considered:**
- React/Vue SPA: requires Node.js build pipeline.
- Go html/template: no type safety, no components.

**Consequences:**
- No JSON API needed for UI (HTMX speaks HTML).
- Simpler build pipeline.
- Every handler checks `HX-Request` for fragment vs full page.

---

## ADR-005: PASETO v4 Over JWT

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Need secure tokens for sessions and API auth.

**Decision:** PASETO v4 for all tokens.

**Alternatives Considered:**
- JWT: algorithm confusion attacks, `none` algorithm, key confusion.

**Consequences:**
- No algorithm confusion attacks (PASETO mandates algorithms per version).
- Less library support than JWT, but Go has solid PASETO libs.

---

## ADR-006: Go Binary Serves HTTP Directly (No Nginx)

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Cosmos Cloud provides its own reverse proxy.

**Decision:** Echo serves HTTP directly. No nginx/caddy in container. Cosmos
handles TLS, domain routing, DDoS.

**Consequences:**
- Single-process container (just Go binary).
- Simpler Dockerfile, faster startup.
- No exposed ports in docker-compose -- Cosmos routes internally.

---

## ADR-007: Configurable Entity Types with JSON Fields

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Kanka has fixed entity types. Users want custom types and fields.

**Decision:** Entity types stored in DB with `fields` JSON column defining
field definitions. Drives both edit forms and profile display dynamically.

**Consequences:**
- GMs can add/remove/reorder fields per entity type per campaign.
- New entity types without code changes.
- JSON queries less performant but entity type defs are small and cached.

---

## ADR-008: Game Systems as Read-Only Modules

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Users want D&D 5e, Pathfinder, Draw Steel reference content
available as tooltips and pages.

**Decision:** Game systems are "Modules" -- separate tier from Plugins.
Ship static data, provide tooltip API, render reference pages. Read-only.
Enabled/disabled per campaign.

**Alternatives Considered:**
- Embed in entities system: conflates user content with reference data.
- External API calls: adds latency and external deps for self-hosted.

**Consequences:**
- Reference data ships with Docker image.
- Simpler structure than plugins (no service/repo).
- @mentions can reference both campaign entities AND module content.
- Must only include SRD/OGL content (legal).

---

## ADR-009: Dual Permission Model (Action vs Content Visibility)

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Site admins need to manage campaigns (delete, force-transfer) without
necessarily seeing all campaign content. A site admin who is also a player in a
campaign shouldn't be spoiled by seeing GM-only content.

**Decision:** Two distinct permission concepts:
1. **Action permissions** -- "can this user perform admin actions?" Checks
   `users.is_admin` flag. Admin actions go through `/admin` routes.
2. **Content visibility** -- "what content can this user see?" Uses the actual
   `campaign_members.role` value. No admin bypass for content.

An admin joining as Player sees only Player-visible content. An admin who hasn't
joined has `MemberRole=RoleNone` (no content access) but can still perform admin
actions via the admin panel.

**Role levels:** Player (1) < Scribe (2) < Owner (3). Admin is site-wide, not a
campaign role. `RequireRole(min)` checks `MemberRole >= min`.

**Alternatives Considered:**
- Single permission model with admin override: admins would always see everything,
  ruining the player experience for admin-players.
- Separate admin accounts: inconvenient for small servers where the admin is also
  a player.

**Consequences:**
- Admins can enjoy campaigns as players without spoilers.
- Admin operations are cleanly separated into `/admin` routes.
- Campaign routes never check `is_admin` -- only membership role.
- Future entity permissions (is_private) will respect MemberRole, not admin flag.

---

## ADR-010: SMTP Password Encryption with AES-256-GCM

**Date:** 2026-02-19
**Status:** Accepted

**Context:** SMTP settings include a password that must be stored securely. The
password must be encrypted at rest and NEVER returned to the UI.

**Decision:** AES-256-GCM encryption with key derived from `SHA-256(SECRET_KEY)`.
Nonce prepended to ciphertext. Password decrypted only at send time, never cached.
UI shows `HasPassword: bool` only.

Empty password on update = keep existing. SECRET_KEY rotation makes stored password
unrecoverable -- admin must re-enter.

**Alternatives Considered:**
- Bcrypt/argon2id hash: can't decrypt to use for SMTP auth.
- Environment variable only: less flexible for web-based management.
- Reversible encryption with separate key: unnecessary complexity.

**Consequences:**
- Password encrypted at rest using app's SECRET_KEY.
- No password recovery -- by design. Admin re-enters on key rotation.
- Single encryption key (SECRET_KEY) for simplicity.
- If SECRET_KEY leaked, SMTP password is compromised (acceptable tradeoff
  for self-hosted). Document key management best practices.

---

## ADR-011: Sidebar Customization via Campaign JSON Column

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Campaign owners want to reorder and hide entity types in the sidebar
to match their campaign's focus (e.g., hide "Events" if not used, promote
"Characters" to the top).

**Decision:** Store sidebar configuration as JSON in `campaigns.sidebar_config`
column (migration 000006). Config contains `entity_type_order` (ordered list
of type IDs) and `hidden_type_ids`. LayoutInjector applies the config before
rendering. Client-side drag-to-reorder widget with auto-save via PUT API.

**Alternatives Considered:**
- Separate `sidebar_order` table: more normalized but overkill for a simple
  ordered list. One campaign has at most ~20 entity types.
- Store order in `entity_types.sort_order`: sort_order is type-global, not
  per-campaign. Two campaigns sharing the same type definitions would conflict.

**Consequences:**
- Simple single-column storage, no joins needed.
- Config parsed on every page render (small JSON, negligible overhead).
- Graceful degradation: malformed JSON falls back to default sort_order.
- Owner-only access -- players cannot customize the sidebar.

---

## ADR-012: Entity Type Layout Builder with JSON Column

**Date:** 2026-02-19
**Status:** Accepted

**Context:** Entity profile pages need customizable layouts -- different entity
types should display their sections in different arrangements (e.g., Characters
might want "Basics" fields in a left sidebar with the entry in the main column).

**Decision:** Store layout configuration as JSON in `entity_types.layout_json`
column (migration 000007). Layout defines sections with key/label/type/column
properties. "column" is either "left" (sidebar) or "right" (main). Section types
are "fields", "entry", or "posts". Client-side two-column drag-and-drop widget.

**Alternatives Considered:**
- Separate layout_sections table: over-normalized for what is always read as a
  unit. The JSON blob is never queried individually.
- Hardcoded layouts per entity type: inflexible, defeats the purpose.

**Consequences:**
- Layout config read with entity type, no additional query.
- Sections validated server-side (valid types, valid columns, unique keys).
- Default layout auto-generated from field definitions when empty.
- Entity show page must read layout_json to render the profile (not yet wired).

---

## ADR-013: Pessimistic Locking for Shared Notes

**Date:** 2026-02-24
**Status:** Accepted

**Context:** Shared notes can be edited by any campaign member. Without
concurrency control, two users editing the same note simultaneously would
overwrite each other's changes (last-write-wins).

**Decision:** Pessimistic edit locking with 5-minute auto-expiry. When a user
starts editing a shared note, the client acquires a lock via `POST /lock`.
While held, the lock is kept alive with a 2-minute heartbeat interval. Stale
locks (older than 5 minutes without heartbeat) are automatically reclaimed
by the lock acquisition query. Campaign owners can force-unlock any note.

**Alternatives Considered:**
- Optimistic concurrency (version counter + conflict detection): more complex
  client-side merge resolution. Notes panel is a lightweight widget, not a
  full collaborative editor -- pessimistic locking is simpler and sufficient.
- Real-time collaborative editing (CRDT/OT): massive complexity for a notes
  sidebar. This is Google Docs-level infra; overkill for a notes widget.
- No locking: acceptable for private notes (single user), but shared notes
  need protection against concurrent edits.

**Consequences:**
- Only one user can edit a shared note at a time.
- Lock state stored in the notes table itself (locked_by, locked_at columns).
- Stale locks self-heal via age check in the acquisition query.
- Private (non-shared) notes skip locking entirely -- only the owner edits them.
- 5-minute timeout is generous enough for slow typists but prevents abandoned locks.

---

## ADR-014: Snapshot-on-Save Version History for Notes

**Date:** 2026-02-24
**Status:** Accepted

**Context:** Users need to recover previous versions of notes, especially
when shared notes are edited by multiple people.

**Decision:** Create a version snapshot before every content-changing operation
(Update and RestoreVersion). Snapshots store title, content blocks, entry JSON,
and entry HTML. Maximum 50 versions per note, oldest auto-pruned. Version
creation errors are swallowed -- version tracking is non-critical.

**Alternatives Considered:**
- Changelog-style diffs: more storage-efficient but requires complex diff/merge
  to reconstruct a version. Snapshots are simpler and notes are small.
- Event sourcing: overkill. Notes are not high-frequency write targets.
- No version history: risky with shared editing. Users expect undo capability.

**Consequences:**
- Every update creates a version row -- storage grows linearly but is bounded at 50.
- Auto-pruning runs after every version creation (DELETE subquery).
- Restore is a two-step operation: snapshot current state, then apply old version.
- Version errors don't block the save operation (swallowed with `_ = err`).

---

## ADR-015: Maps with Percentage Coordinates and Leaflet CRS.Simple

**Date:** 2026-02-28
**Status:** Accepted

**Context:** Maps plugin needs to display pin markers on uploaded background images.
Markers must be positioned relative to the image, independent of actual pixel resolution.

**Decision:** Store marker coordinates as percentages (0-100 for both X and Y).
Use Leaflet.js with CRS.Simple to create a non-geographic coordinate system where
the image is overlaid. Leaflet converts percentage coords to pixel space at render
time based on image dimensions stored on the map record.

Multiple maps per campaign (unlike calendar's 1:1). Maps are listed on an index page.

**Alternatives Considered:**
- Pixel coordinates: breaks when image is resized or replaced with different resolution.
- Geographic coordinates (lat/lng): adds complexity for fantasy maps with no real-world
  mapping. CRS.Simple avoids this entirely.
- Canvas-based rendering: more work, less accessible, no built-in panning/zooming.

**Consequences:**
- Markers are resolution-independent. Image can be swapped with different sizes.
- Leaflet loaded from CDN per-page (not globally) to avoid loading JS on non-map pages.
- Image dimensions (width/height) must be stored on the map record for coordinate space.
- Draggable markers use silent PUT on dragend -- no save button needed.

---

## ADR-016: Inline Secrets via TipTap Mark Extension

**Date:** 2026-02-28
**Status:** Accepted

**Context:** GMs need to write inline secret text within entity entries that only
they and scribes can see. Players should never receive the secret content -- it must
be stripped server-side, not just hidden with CSS.

**Decision:** Create a TipTap `secret` mark that renders as
`<span data-secret="true" class="chronicle-secret">`. Since the vendored TipTap
bundle doesn't export the raw `Mark` class, extend `TipTap.Underline` (which IS a
Mark subclass) and override name, parseHTML, renderHTML, commands, and shortcuts.

Server-side stripping in `internal/sanitize/`:
- `StripSecretsHTML()` -- regex strips `<span data-secret>...</span>` from HTML.
- `StripSecretsJSON()` -- recursive tree walk removes text nodes with `secret` mark
  from ProseMirror JSON.

Applied in `GetEntry` handler when `role < RoleScribe`.

**Alternatives Considered:**
- CSS-only hiding: insecure -- HTML still sent to client, visible in DevTools.
- Separate "GM notes" field: less flexible than inline secrets mixed with regular text.
- Build custom TipTap bundle with Mark export: adds Node.js build step, breaks
  vendored-only constraint.

**Consequences:**
- Secret content never reaches players (server-stripped from both JSON and HTML).
- Mark extension uses Underline.extend() hack -- works but is coupled to Underline
  being present in the bundle.
- Bluemonday whitelist updated to allow `data-secret` on `<span>`.
- CSS shows amber background + eye-slash indicator for owners/scribes in edit mode.

## ADR-017: Add 'plugin' to Addon Category ENUM

**Date:** 2026-02-28
**Status:** Accepted

**Context:** The `addons.category` ENUM had three values: `module`, `widget`,
`integration`. Calendar and Maps are architecturally Plugins (full feature apps with
handler/service/repo/templates), not Widgets. The original migration 000015 seed data
miscategorized them as `widget` because the Plugin tier hadn't been reflected in the
database schema. Migrations 000027 and 000029 attempted to INSERT with
`category='plugin'`, causing a MariaDB "Data truncated" error (Error 1265). A
secondary duplicate slug conflict also existed since the rows were already seeded.

**Decision:** Add `plugin` as a fourth ENUM value via ALTER TABLE in migration 000027.
Use UPDATE instead of INSERT to fix existing seed data rows. Add `CategoryPlugin`
constant to Go code and validation. Add migration SQL validation tests as a safeguard.

**Alternatives Considered:**
- Keep only three categories and map plugins to `widget`: semantically wrong. Plugins
  are full feature apps, not reusable UI blocks.
- Change the column from ENUM to VARCHAR: loses the schema-level validation benefit
  of ENUM. The four-value ENUM is small and stable.

**Consequences:**
- The category ENUM now has four values: `plugin`, `module`, `widget`, `integration`.
- All future plugin registrations should use `category='plugin'`.
- Down migration for 000027 must revert the ENUM (requires no rows use `plugin`).
- Migration validation test in `internal/database/migrate_test.go` catches invalid
  ENUM values at `make test` time.

---

## ADR-018: D3.js for Timeline Visualization

**Date:** 2026-03-02
**Status:** Accepted
**Context:** The timeline plugin needs an interactive visualization with zoom/pan/drag,
time scales, and entity group swim-lanes. We already use Leaflet.js for the maps plugin.

**Decision:** Use D3.js v7 for the timeline visualization. Load from CDN per-page
(matching Leaflet pattern), not bundled globally. D3 provides SVG-based rendering,
`d3.zoom` for pan/drag, `d3.scaleLinear` for time axes, and transitions. Leaflet.js
is designed for geographic tile-based rendering and is unsuitable for time-axis layouts.

**Alternatives Considered:**
- Leaflet.js: Already in the project for maps, but fundamentally geographic. Would
  require fighting the library's coordinate system and tile-based assumptions.
- vis-timeline: Purpose-built timeline library, but opinionated about styling and
  harder to customize for swim-lanes, fantasy calendars, and Chronicle's dark theme.
- Canvas-based rendering: Better performance for very large datasets, but loses SVG's
  accessibility, CSS styling integration, and text rendering quality.

**Consequences:**
- D3 v7 (~90KB gzipped) loaded only on timeline detail pages, no impact on other pages.
- SVG rendering gives full CSS control, accessibility, and crisp text at all zoom levels.
- Swim-lanes, zoom levels, and entity grouping can be implemented incrementally.
- Fantasy calendar dates (arbitrary year/month/day systems) work naturally with
  `d3.scaleLinear` since we convert to fractional years for positioning.

---

## ADR-012: Sessions-Calendar Integration and RSVP Email System

**Date:** 2026-03-04
**Status:** Accepted

**Context:** Sessions were a standalone plugin with their own sidebar link and
addon toggle. Users expected sessions to appear on the calendar (especially
real-life mode calendars) and wanted RSVP from the calendar UI. The separate
sidebar link was confusing — sessions are fundamentally a calendar feature.

**Decision:**
- **Sessions require the calendar addon** — no separate "sessions" addon toggle.
  The sidebar link for sessions is removed; sessions are accessed via the
  calendar's dice icon and Sessions button in the calendar header.
- **Sessions display on real-life calendar grids** as purple chips with a dice
  icon. Clicking opens an inline modal with RSVP controls (Going/Maybe/Can't).
- **Recurring sessions** supported: weekly, biweekly, monthly, and custom N-week
  intervals. Stored on the sessions table with recurrence_type/interval fields.
- **RSVP via email**: SMTP SendHTMLMail added for multipart/alternative emails.
  Each invitation generates single-use tokens (7-day expiry) for one-click
  accept/decline links without requiring login.
- **RequireAddon middleware**: Route-level addon gating via AddonService.IsEnabledForCampaign
  query. Applied to calendar, maps, sessions, and timeline route groups.
- **Date formatting**: Session dates use `FormatScheduledDate()` returning
  "Mon, Jan 2, 2006" instead of raw ISO 8601.

**Alternatives considered:**
- Merging sessions into the calendar_events table: Rejected because sessions have
  attendees, RSVP tracking, entity linking, and notes — fundamentally different
  from calendar events. Keeping separate tables is cleaner.
- JWT-based RSVP tokens: Rejected for simplicity. Random tokens with DB lookup
  are simpler, revocable, and auditable.

**Future: Discord Bot Integration**
- Plan: A `discord` integration plugin that sends session invites to a configured
  Discord channel with reaction-based RSVP (✅/❌ emoji reactions).
- Architecture: `internal/plugins/discord/` plugin with bot token configuration
  (admin settings), webhook for outbound notifications, and a listener for
  reaction events that calls SessionService.UpdateRSVP.
- The Discord bot will reuse the same SessionService interface — no session-specific
  code changes needed. Just a new notification channel alongside SMTP email.

**Consequences:**
- Sessions sidebar link removed — users navigate via calendar.
- Disabled addons now return 404/redirect at the route level, not just hidden sidebar links.
- SMTP service supports both plain text and HTML email variants.
- Session RSVP tokens stored in session_rsvp_tokens table with FK cascade.

---

## ADR-019: Manifest-Driven Module Framework

**Date:** 2026-03-05
**Status:** Accepted

**Context:** The module system had a static hardcoded registry listing three
coming-soon modules with no runtime infrastructure. We need a framework that
supports auto-discovery, validation, and a sandboxed interface for modules
to implement without accessing the database or Echo router.

**Decision:** Replace the static registry with a manifest-driven framework:

1. **manifest.json** — Each module declares metadata in a JSON file: id, name,
   version, author, license, categories, API version, entity presets, etc.
2. **ModuleLoader** — Scans `internal/modules/*/manifest.json` at startup,
   validates required fields, logs warnings for invalid manifests without
   failing startup.
3. **Module interface** — Sandboxed: `Info() *ModuleManifest`,
   `DataProvider() DataProvider`, `TooltipRenderer() TooltipRenderer`.
   Modules can only serve data through these interfaces.
4. **DataProvider interface** — `List(category)`, `Get(category, id)`,
   `Search(query)`, `Categories()` returning `ReferenceItem` structs.
5. **Global Init()** — Called once at startup, populates the singleton registry.

**Alternatives Considered:**
- Database-stored manifests: adds unnecessary complexity for static content packs.
- Go struct registration (current approach): no separation of metadata from code,
  no validation, no path to external module loading.

**Consequences:**
- Modules are self-describing via manifest.json (human-readable, validatable).
- Auto-discovery eliminates manual registry maintenance.
- Sandboxed interfaces prevent modules from accessing infrastructure directly.
- Admin modules page shows manifest metadata (author, license, API version).
- Module slugs added to installedAddons for per-campaign enable/disable.
- K-4 will build HTTP handlers and DataProvider implementations on this foundation.

---

## ADR-020: JSON-File DataProvider with Factory Registry

**Date:** 2026-03-05
**Status:** Accepted

**Context:** Sprint K-3 delivered the Module/DataProvider/TooltipRenderer interfaces
and auto-discovery. K-4 needs a concrete DataProvider implementation, the first
module (D&D 5e), and HTTP handlers. The challenge: module subpackages (dnd5e/)
import the parent modules package for interfaces, but the loader in modules/
cannot import subpackages without creating circular imports.

**Decision:** Three key design choices:

1. **JSONProvider** — Generic DataProvider implementation that loads `data/*.json`
   files from a module's directory. Filename stem becomes the category slug.
   Items loaded into memory at startup. Case-insensitive search across Name,
   Summary, and Tags.

2. **Factory Registry** — Modules register factory functions via
   `modules.RegisterFactory(id, fn)` in their package `init()` functions.
   The loader calls registered factories during DiscoverAll() for modules
   with status "available". This avoids circular imports: the parent package
   holds the factory map, subpackages register themselves, and `app/routes.go`
   uses blank imports (`_ "modules/dnd5e"`) to trigger init().

3. **Dynamic Addon Middleware** — Module routes use `/campaigns/:id/modules/:mod`
   with middleware that reads the `:mod` param and checks `addonSvc.IsEnabledForCampaign()`
   dynamically, rather than requiring a separate route group per module.

**Alternatives Considered:**
- Direct import of dnd5e in loader.go: creates circular import.
- Plugin-style registration in app/routes.go: too much wiring code, doesn't scale.
- Separate handler per module: unnecessary duplication since all modules share the
  same reference page structure.

**Consequences:**
- Adding a new module requires: manifest.json, data/*.json, a Go file with init()
  factory registration, and a blank import in app/routes.go.
- Module reference pages are generic (same Templ templates for all modules).
- Module content appears in entity @mention search when the module addon is enabled.
- TooltipAPI returns module-specific HTML via the TooltipRenderer interface.

---

## ADR-021: Layered Third-Party Extension Strategy

**Date:** 2026-03-06
**Status:** Proposed

**Context:** Chronicle's three-tier architecture (Plugins, Modules, Widgets) is
currently internal-only — all extensions ship with the Go binary. Users and the
community want to create and share content packs, custom widgets, and eventually
custom backend logic without forking the codebase. Research was conducted across
WordPress, Grafana, Discourse, Obsidian, Foundry VTT, and Shopify, plus Go-specific
approaches (HashiCorp go-plugin, WASM/Extism/wazero, GopherLua).

**Key findings from research:**
- No mainstream self-hosted platform truly sandboxes plugins except Grafana
  (subprocess isolation via gRPC) and Shopify (restricted Liquid rendering).
- WordPress, Discourse, Obsidian, and Foundry VTT all run plugins in-process
  with full access — security relies entirely on trust and code review.
- WASM (via Extism + wazero) is the most promising approach for a Go backend
  wanting user-uploadable extensions with real sandboxing: memory-safe isolation,
  capability-based security, language-agnostic authoring, pure Go runtime.
- Foundry VTT's patterns are directly relevant as a TTRPG competitor: manifest
  format, Flags storage, hook-based events, manifest URL updates.

**Decision:** Three layers of third-party extensibility, implemented incrementally:

### Layer 1: Content Extensions (Manifest-Only, No Code)
Declarative content packs distributed as zip archives containing a `manifest.json`
plus static assets (JSON data files, images, CSS). No executable code. Examples:
monster packs, map tile sets, pre-built entity templates, custom field definitions,
calendar presets, theme variants.

- **Manifest**: JSON declaring id, name, version, author, compatibility, contents.
- **Installation**: Upload zip via admin UI or place in `extensions/` directory.
- **Storage**: Extension data stored in DB via a generic extension data table.
  Inspired by Foundry VTT's Flags system (namespaced key-value on documents).
- **Security**: No code execution. Manifest validated, file types allowlisted.
- **Covers**: ~60% of what TTRPG users actually want to share.

### Layer 2: Widget Extensions (Browser-Sandboxed JS)
Custom widgets that self-register via `Chronicle.registerWidget()` and mount to
DOM elements. They run in the browser, can only hit existing API endpoints, and
are naturally sandboxed by the browser same-origin policy.

- **Distribution**: Bundled in content extension zips (a JS file in the package).
- **API**: `Chronicle.registerWidget(name, { mount, unmount, config })`.
- **Security**: Browser sandbox. Widgets use Chronicle.apiFetch() which includes
  CSRF tokens. Cannot access server filesystem or database directly.
- **Covers**: Custom UI blocks, visualization widgets, interactive tools.

### Layer 3: Logic Extensions (WASM-Sandboxed Backend, Future)
Custom backend logic compiled to WebAssembly and executed via Extism + wazero.
Plugins are `.wasm` files with capability-based security: no filesystem, no
network, no database unless the host explicitly grants it through defined
host functions.

- **Runtime**: wazero (pure Go, zero CGO) via Extism SDK.
- **Host functions**: Chronicle exposes specific APIs (read entity, list tags,
  create event) as host functions. Plugins can only call what's exposed.
- **Distribution**: `.wasm` files in extension packages, hash-verified.
- **Use cases**: Custom validation rules, automated entity generation,
  game-system-specific calculators, webhook processors.
- **Deferred**: This layer is complex and should only be built when Layers 1-2
  prove insufficient for user needs.

**Alternatives Considered:**
- HashiCorp go-plugin (gRPC subprocess per plugin): Battle-tested by Terraform
  and Grafana but designed for operator-installed compiled binaries, not
  user-uploaded extensions. Per-process overhead is heavy for many small TTRPG
  extensions.
- GopherLua (embedded Lua VM): Lightweight and familiar to game/modding
  communities. Could serve as intermediate between Layers 2 and 3 for simple
  automation/macros. May be added as Layer 2.5 if demand warrants.
- No sandboxing (WordPress/Foundry model): Unacceptable for a self-hosted
  platform where users upload community content. Security-by-trust doesn't scale.
- Signing-only (Grafana model): Good defense-in-depth but insufficient alone.
  Chronicle should implement SHA-256 manifest signing regardless of sandbox choice.

**Implementation order:**
1. Layer 1 first (content extensions) — highest value, lowest risk.
2. Layer 2 second (widget extensions) — builds on existing widget infrastructure.
3. Layer 3 only when needed — complex, can be deferred indefinitely.

**Consequences:**
- Content extensions cover the majority of community sharing needs without code.
- Widget extensions leverage the existing boot.js auto-mounter and apiFetch infrastructure.
- WASM layer provides a future path for backend extensibility with real security.
- Each layer can be shipped independently; later layers don't block earlier ones.
- Manifest format and extension installer are shared infrastructure across all layers.
- Extension signing (SHA-256 checksums in signed manifest, inspired by Grafana)
  should be implemented for all layers as defense-in-depth.
