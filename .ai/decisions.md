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
- **Systems** (`internal/systems/`): Game system content packs (Draw Steel, D&D 5e,
  Pathfinder 2e). Reference data, tooltips, dedicated pages. Read-only.
  Installed via package manager.
- **Widgets** (`internal/widgets/`): Reusable UI building blocks (editor, title,
  tags, attributes, mentions). Mount to DOM, fetch own data.

**Alternatives Considered:**
- Flat structure for everything: conflates apps with UI components
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

## ADR-023: Sessions-Calendar Integration and RSVP Email System

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
  query. Applied to calendar, maps, sessions, timeline, and media-gallery route groups.
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

1. **manifest.json** — Each system declares metadata in a JSON file: id, name,
   version, author, license, categories, API version, entity presets, etc.
2. **SystemLoader** — Scans `internal/systems/*/manifest.json` and package-installed
   systems at startup, validates required fields, logs warnings for invalid
   manifests without failing startup.
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

---

## ADR-022: WASM Runtime via Extism SDK + wazero

**Date:** 2026-03-06
**Status:** Accepted

**Context:** Layer 3 of ADR-021 called for WASM-sandboxed backend logic. With
Layers 1 (content) and 2 (widgets) proven, we need to implement the WASM runtime
to allow community-authored backend logic (custom validation, calculators,
automation) without giving extensions direct access to the database or filesystem.

**Decision:** Use the Extism Go SDK (v1.7.1) with wazero (v1.9.0) as the WASM
runtime. Key design choices:

1. **Capability-based security** — Plugins declare required capabilities in their
   manifest (`contributes.wasm_plugins[].capabilities`). The PluginManager only
   exposes host functions matching declared capabilities. Five capability groups:
   `log`, `entity_read`, `calendar_read`, `tag_read`, `kv_store`.

2. **Read-only host functions first** — Initial host functions are all read-only
   (get_entity, search_entities, list_entity_types, get_calendar, list_events,
   list_tags, kv_get/set/delete, chronicle_log). Write functions deferred to R-3.

3. **Per-plugin KV store via extension_data** — Reuses the existing `extension_data`
   table with namespace "wasm_kv" instead of creating new tables. Each plugin's
   data is scoped by campaign_id + extension_id.

4. **Async hook dispatch** — WASM plugins register for events via manifest `hooks`
   field. Events are dispatched fire-and-forget in goroutines. Plugin failures
   never affect the originating operation.

5. **Resource limits** — Default 16 MB memory, 30s timeout per call. Manifests
   can override up to 256 MB memory and 300s timeout. Fuel metering planned for R-2.

6. **Adapter interfaces** — EntityReader, CalendarReader, TagReader interfaces
   decouple WASM host functions from concrete plugin implementations, following
   the existing adapter pattern used throughout Chronicle.

**Alternatives Considered:**
- Direct wazero without Extism: More control but requires reimplementing plugin
  manifest handling, host function registration, and memory management that Extism
  provides out of the box.
- GopherLua: Lighter weight but Lua-only. WASM supports Rust, Go/TinyGo, JS,
  Python, and any language with a WASM target.
- gRPC subprocess model (HashiCorp go-plugin): Better for operator-installed
  plugins but too heavy for user-uploaded community extensions.

**Consequences:**
- WASM plugins are truly sandboxed: no filesystem, no network, no database access
  except through explicitly declared host functions.
- Community can author plugins in any language that compiles to WASM.
- Plugin lifecycle (load/unload/reload) managed centrally by PluginManager.
- Hook system enables reactive plugins without polling.
- KV store provides durable per-plugin state without new database tables.

## ADR-024: Extension Migration System (Dynamic Schema)

**Date:** 2026-03-08
**Status:** Accepted

**Context:** The current migration system (sequential numbered SQL files via
golang-migrate) works for core schema but cannot handle dynamic extensions. When
a user uploads an extension that needs its own tables, and later disables or
removes it, the core migration pipeline has no mechanism for this. Extensions
should not modify the core migration sequence.

**Decision:**
Extensions use a **separate, per-extension migration system** alongside core:

1. **Core migrations** — remain as-is (sequential `000NNN_*.sql` files). These
   define the platform schema and run on every startup.

2. **Extension migrations** — each extension's zip manifest declares a `migrations/`
   directory containing numbered SQL files scoped to that extension. When an
   extension is installed, its migrations run against a tracking table
   (`extension_schema_versions`) keyed by `(extension_id, version)`.

3. **Namespaced tables** — extension-created tables MUST be prefixed with `ext_`
   followed by the extension slug (e.g., `ext_knowledge_graph_nodes`). This
   prevents collisions with core tables and makes cleanup straightforward.

4. **Install/uninstall lifecycle**:
   - **Install**: Run extension's `up` migrations in order.
   - **Uninstall**: Run extension's `down` migrations in reverse, then delete
     tracking rows. All `ext_<slug>_*` tables are dropped.
   - **Disable**: Tables and data stay intact (campaign-level toggle only).
   - **Enable**: No migration action needed (data preserved).

5. **Campaign deletion**: When a campaign is deleted, extension data in
   `ext_*` tables is cleaned up via `ON DELETE CASCADE` foreign keys to
   `campaigns.id`. Extensions that create non-campaign-scoped data use the
   existing `extension_data` table (already cascaded).

6. **Validation**: Extension migrations are validated before execution:
   - Only `CREATE TABLE ext_<slug>_*` and `ALTER TABLE ext_<slug>_*` allowed
   - No `DROP TABLE` on core tables
   - No `ALTER TABLE` on core tables
   - SQL statements parsed and validated server-side

**Alternatives Considered:**
- Let extensions use only `extension_data` JSON blobs: simpler but doesn't
  support efficient queries, indexes, or foreign keys for complex extensions.
- Give extensions full migration access: too dangerous — a malicious extension
  could `DROP TABLE users`.
- Schema-per-extension (MySQL databases): MariaDB doesn't truly isolate, and
  cross-database JOINs are needed for host functions.

**Consequences:**
- Extensions can define proper relational schemas when JSON blobs aren't enough.
- Core migration system stays simple and predictable.
- Uninstalling an extension cleanly removes all its schema artifacts.
- The `ext_` prefix convention makes it trivial to audit what extensions own.

## ADR-025: Campaign Deletion Cascade and Cleanup

**Date:** 2026-03-08
**Status:** Accepted

**Context:** When a campaign is deleted, database CASCADE handles most rows, but
several gaps exist: media files are orphaned on disk (SET NULL, not CASCADE),
API keys lack foreign key constraints entirely, and extension-provisioned content
(entities, tags created by extensions) remains even after provenance records
are cascaded.

**Decision:**
Campaign deletion becomes a **multi-step service operation** instead of a single
SQL DELETE:

1. **Media file cleanup** — Before the SQL DELETE, query all `media_files` where
   `campaign_id = ?`, delete physical files from disk (main + thumbnails), then
   delete the DB rows. This replaces the current `ON DELETE SET NULL` behavior
   for campaign-scoped media. Avatars and backdrops (campaign_id IS NULL) are
   unaffected.

2. **API key cascade** — Add proper `FOREIGN KEY (campaign_id) REFERENCES
   campaigns(id) ON DELETE CASCADE` to `api_keys`. API request logs get
   `ON DELETE SET NULL` (retain for audit trail, but disassociate from campaign).

3. **Extension content cleanup** — Before delete, query `extension_provenance`
   for the campaign to find extension-created records (entity types, entities,
   tags, etc.). These are already CASCADE'd through their own campaign_id FKs,
   so no extra work needed. The provenance records themselves cascade.

4. **Extension table cleanup** — For extensions with `ext_*` tables, rows with
   the campaign_id are cleaned up via CASCADE FKs (required by ADR-024).

5. **WASM plugin state** — `extension_data` rows are already CASCADE'd. WASM
   plugins with in-memory state receive a `campaign.deleted` hook event so they
   can clean up caches.

6. **Non-default uploaded extensions** — When a campaign is deleted, uploaded
   extensions that are ONLY enabled for that campaign are flagged for cleanup.
   If no other campaign uses the extension, the extension zip and its `ext_*`
   tables can be uninstalled. This is a background job, not synchronous.

**Consequences:**
- Campaign deletion is slightly slower (disk I/O for media) but leaves no
  orphaned data.
- API keys are properly invalidated on campaign delete (security fix).
- Extensions can trust that campaign deletion is thorough.
- The media `CleanupOrphans()` method becomes a safety net, not the primary
  cleanup mechanism.

## ADR-026: Admin Data Hygiene Dashboard

**Date:** 2026-03-08
**Status:** Accepted

**Context:** Over time, the database accumulates orphaned data: media files
without campaigns, API keys pointing to deleted campaigns, extension records
with no parent, etc. Admins need visibility into this and tools to clean it up
safely — but also guardrails to prevent accidentally deleting data that active
campaigns still depend on.

**Decision:**
Add an admin "Data Hygiene" page at `/admin/data-hygiene` with read-only
diagnostics and guarded cleanup actions:

1. **Orphan detection queries** — Read-only scans that identify:
   - Media files with `campaign_id IS NULL` that aren't avatars/backdrops
     (orphaned by campaign deletion or SET NULL)
   - Media files on disk with no matching DB record (stale filesystem artifacts)
   - API keys referencing non-existent campaigns (pre-FK-fix orphans)
   - Extension provenance records pointing to deleted records
   - `ext_*` tables with no matching installed extension
   - Notes/note_versions for deleted campaigns (if any escaped CASCADE)
   - Users with no campaign memberships (not necessarily orphaned — could be new)

2. **Safety guardrails** — Cleanup actions are blocked when data is still
   referenced:
   - Cannot delete a media file that is referenced by any entity's `image_path`
     or `entry_html`
   - Cannot delete an extension that has campaigns with it enabled
   - Cannot purge API keys for campaigns that still exist
   - Each action shows a preview of what will be affected before confirming
   - All cleanup actions are logged to `security_events` for audit trail

3. **Cleanup actions** (admin-only, confirmation required):
   - "Purge orphaned media" — deletes files from disk + DB rows for
     campaign-less media not referenced by any entity
   - "Purge stale filesystem files" — deletes files on disk with no DB record
   - "Purge orphaned API keys" — deletes keys for non-existent campaigns
   - "Run media orphan scan" — invokes `CleanupOrphans()` with dry-run option

4. **Dashboard stats** — Summary cards showing:
   - Total disk usage vs DB-tracked usage (delta = stale files)
   - Orphaned media count + size
   - Orphaned API key count
   - Extension table count vs installed extension count

5. **No automated cleanup** — All actions are manual and admin-initiated.
   No cron jobs or background workers that silently delete data. The admin
   decides when to clean up and reviews what will be affected.

**Alternatives Considered:**
- Automated background cleanup on schedule: too risky — could delete data
  during a race condition (e.g., campaign being restored from backup).
- Per-campaign cleanup page: campaigns already cascade; the problem is
  cross-campaign orphans that only a site admin can see.

**Consequences:**
- Admins have full visibility into database/filesystem health.
- No data is ever deleted without explicit admin action + confirmation.
- Safety checks prevent accidental deletion of in-use data.
- Complements ADR-025 (campaign deletion cleanup) as a catch-all safety net.

---

## ADR-027: RequireAddon Middleware Fail-Open on DB Errors

**Date:** 2026-03-09
**Status:** Accepted

**Context:** The `RequireAddon` middleware checks whether an addon (calendar,
maps, timeline, sessions, etc.) is enabled for a campaign before allowing access
to its routes. When the database query fails, the middleware must decide whether
to block (fail-closed) or allow (fail-open) the request.

**Decision:**
`RequireAddon` fails open on DB errors — if the addon-check query fails, the
request is allowed through. Rationale:

1. If the database is down, nothing downstream works anyway (service calls,
   repo queries all fail). Blocking at the middleware level just changes the
   error from a 500 to a redirect/404, which is less informative.
2. Fail-open matches the principle of least surprise for self-hosted instances:
   a transient DB blip doesn't lock users out of features they have enabled.
3. The companion `RequireAddonAPI` middleware (for API v1 routes) uses
   fail-closed because API callers are programmatic and can handle 503 retries.

This convention is also used by `Handler.isAddonEnabled()` in the entity search
endpoint, which skips addon-specific search results on DB errors rather than
failing the entire search.

**Alternatives Considered:**
- Fail-closed everywhere: too disruptive for a self-hosted app where DB might
  have brief connectivity issues during backups or maintenance.
- Cache addon state in Redis: adds complexity; the DB query is a single indexed
  row lookup that takes <1ms.

**Consequences:**
- During DB outages, disabled addons may briefly appear enabled (routes accessible).
- This is acceptable because the underlying service calls will fail anyway.
- API routes use stricter fail-closed behavior (ADR-025 batch 24).

---

## ADR-028: Plugin-Isolated Database Schema Architecture

**Date:** 2026-03-09
**Status:** Accepted

**Context:** Chronicle had 63 sequential migration files mixing core tables with
plugin tables. A bad migration in any plugin (e.g., Error 1553 from migration
000063) crashed the entire app and left the DB in a dirty state requiring manual
recovery. Bandaid solutions (migrate_preflight.go, lint tests) caught some issues
but couldn't prevent all classes of failures. The goal: plugin failures should
never break the app, and user-installable extensions need safe schema isolation.

**Decision:** Two-tier schema system:
- **Tier 1 (Core):** Single baseline migration (`db/migrations/000001_baseline`)
  with all core tables. Runs via golang-migrate. Failure is fatal.
- **Tier 2 (Plugins):** Each built-in plugin has its own `migrations/` directory
  (`internal/plugins/<name>/migrations/`). Runs via custom `RunPluginMigrations()`
  after core migrations. Failure disables that plugin; app continues serving.

Plugin health tracked in `PluginHealthRegistry` (thread-safe in-memory). Routes
are conditionally registered based on `IsHealthy()`. Degraded plugins show a
"Feature unavailable" banner via `plugin_unavailable.templ`.

Version tracking uses `plugin_schema_versions` table (separate from
`extension_schema_versions` used by user-installed extensions). SQL validation
is skipped for trusted built-in plugins but enforced for user extensions via
`ValidateExtensionSQL()` + `ext_<slug>_` prefix requirement.

**Alternatives considered:**
- Keep all migrations together + better preflight checks: still single point of
  failure, doesn't scale to user-installable extensions.
- Per-plugin databases: too complex, cross-plugin FKs become impossible.
- Wrap each migration in a savepoint: MariaDB doesn't support transactional DDL.

**Consequences:**
- Plugin schema failures degrade gracefully instead of crashing the app.
- Each plugin's schema is independently versioned and can evolve separately.
- Cross-plugin FK dependencies require ordered plugin migration execution
  (calendar before sessions/timeline).
- Removed migrate_preflight.go and bandaid lint tests from migrate_test.go.
- Fresh DB only — no backward compatibility with the old 63-migration sequence.

---

## ADR-029: Features Page Consolidation (Plugin Hub + Addon Settings → Single Page)

**Date:** 2026-03-10
**Status:** Accepted

**Context:** Campaign feature management was split across two pages:
1. **Plugin Hub** (`/campaigns/:id/plugins`) — read-only card grid visible to all members.
2. **Addon Settings** (`/campaigns/:id/addons/settings`) — owner-only toggle list.

This created confusion: owners had two "features" pages with different layouts and
capabilities. Non-owners could see features but couldn't tell which were enabled.

**Decision:** Consolidate into a single Features page at `/campaigns/:id/plugins`.
- All members see the card grid with enable/disable status.
- Owners see inline toggle buttons on each card.
- The old `/addons/settings` route, handler, and full-page template are removed.
- The addons fragment route (`/addons/fragment`) remains for the Customization Hub.
- Toggle forms include `redirect_to=plugins` so the handler redirects back to the
  unified page after toggling.

**Alternatives considered:**
- Keep both pages with cross-links: still confusing, maintenance burden.
- Merge into the Customization Hub: too buried, features deserve top-level access.

**Consequences:**
- Single source of truth for feature management.
- Owners can manage features directly from the same page all members see.
- Future enhancements (per-addon entity usage, "offline" banners) have one target page.

---

## ADR-030: Embed Plugin Migrations via Go embed.FS

**Date:** 2026-03-11
**Status:** Accepted (amends ADR-028)

**Context:** ADR-028 introduced per-plugin migration directories at
`internal/plugins/<name>/migrations/`. The migration runner used `os.Stat` and
`os.ReadDir` with relative filesystem paths. This worked in development (CWD =
project root) but failed silently in Docker: the runtime image copies the binary
to `/app` but never copies plugin migration directories. Since `os.Stat` returned
`os.IsNotExist`, the runner treated each plugin as "healthy with 0 migrations"
— no tables were created, entity pages crashed, and the DB Explorer showed 0/0.

**Decision:** Embed plugin migration SQL files in the binary using Go's `embed.FS`:
- Each plugin package gets an `embed.go` that exports `MigrationsFS embed.FS`
  with `//go:embed migrations/*.sql`.
- `PluginSchema.MigrationsDir` (string) replaced with `MigrationsFS` (`fs.FS`).
- `parsePluginMigrations` and `LatestMigrationVersion` read from `fs.FS` instead
  of the real filesystem.
- `RegisteredPlugins()` moved from `database` package to `cmd/server/main.go`
  to avoid import cycles (database can't import plugin packages). Uses `fs.Sub`
  to strip the `migrations/` prefix from each embed.FS.
- `PluginSchemas` stored on `App` struct and passed to `DatabaseExplorer` for
  on-demand re-migration from the admin panel.

**Alternatives considered:**
- Copy plugin migration dirs to Docker runtime image: fragile, requires syncing
  Dockerfile whenever plugins are added/removed. Still fails if CWD changes.
- Centralise all plugin migrations in one directory: loses per-plugin isolation
  that ADR-028 established.

**Consequences:**
- Migrations work in any environment regardless of working directory.
- No Dockerfile changes needed when adding new plugins with migrations.
- Each plugin must have an `embed.go` exporting its `MigrationsFS`.
- `RegisteredPlugins()` now lives in `cmd/server/main.go` instead of `database`.

---

## ADR-031: Auto-Register Game Systems as Addons from Manifests

**Date:** 2026-03-19
**Status:** Accepted

**Context:** Game system addon definitions were hardcoded in the
`builtinAddons` array in `addons/service.go`. Adding a new game system
required two code changes: (1) the system's manifest.json + data files,
and (2) a matching `addonDef` entry in the addons package. This coupling
prevented truly self-service system creation — you couldn't just drop a
folder or install via the package manager and have it appear in the addon UI.

**Decision:** Auto-register game systems from the systems registry:
- `systems.AddonInfos()` returns addon metadata for all discovered systems
  with status "available" (name, description, version, icon, author from manifest).
- `addons.RegisterSystemAddon()` appends to `builtinAddons` and marks the
  slug as installed in `installedAddons`.
- App wiring calls these after `systems.Init()` but before `SeedInstalledAddons()`.
- The three hardcoded system entries (dnd5e, pathfinder2e, drawsteel) are
  removed from `builtinAddons`.

**Alternatives considered:**
- Have `addons` import `systems` directly: creates package coupling. The
  wiring layer in `app/routes.go` already imports both.
- Auto-discover from database only: doesn't help with initial registration
  of new systems before they're in the DB.

**Consequences:**
- New game systems appear as addons automatically with zero code changes.
- Systems from the package manager, custom uploads, or `internal/systems/`
  all register the same way.
- The blank import for dnd5e in `main.go` is still needed for its custom
  tooltip renderer factory — pure-data systems need no import.
- Campaign settings page falls back to "Campaign extension." for system
  descriptions (previously had hardcoded per-system strings).

---

## ADR-032: Sidebar Navigation Overhaul — Pure Folders & Unified Items

**Date:** 2026-03-19
**Status:** Accepted

**Context:** The sidebar navigation had several limitations:
1. Organizational folders were implemented as entities with `is_folder=TRUE`,
   which polluted entity search results and required filtering workarounds.
2. Addon links (Journal, NPCs) were hardcoded in `app.templ` — owners
   couldn't reorder them relative to categories or custom links.
3. No tag filtering, lazy loading, or bulk operations for large campaigns.
4. Favorites were localStorage-only (lost on device switch).

**Decision:** Four-sprint overhaul:

**Sprint N-1: Pure folders.** New `sidebar_nodes` table for organizational
folders with zero entity records. Entities gain `parent_node_id` (FK to
sidebar_nodes) as a mutually exclusive alternative to `parent_id`. The
`is_folder` column is removed from entities. Migration 000013 handles
data migration from `is_folder` entities to `sidebar_nodes` rows.

**Sprint N-2: DB-backed favorites.** New `entity_favorites` table replaces
localStorage. Per-user, per-campaign. Toggle/list API endpoints. The
`favorites.js` widget uses API calls with in-memory cache for instant UI.

**Sprint N-3: Unified sidebar model.** `SidebarItem` type added to
`SidebarConfig` with an `Items` array. All sidebar content (dashboard,
addons, categories, sections, links) unified as items. When `Items` is
present, the template renders in owner-defined order. When absent,
falls back to legacy format. New `sidebar_layout_editor.js` widget
replaces the separate category-order and custom-links editors.

**Sprint N-4: Large campaign support.** Tag filtering via `?tags=` query
param with AND-logic SQL subquery. Lazy loading at 50 entities per page
with IntersectionObserver. Multi-select bulk move with floating action
bar. Collapsible Manage section with localStorage persistence.

**Alternatives considered:**
- Keep `is_folder` on entities with filtering: still creates entity records,
  confusing conceptually, requires ongoing filtering in every query.
- New `sidebar_folders` table with `parent_type` discriminator: adds
  complexity for parent resolution. The simpler `parent_node_id` on
  entities avoids ambiguous joins.
- Separate `sidebar_items` database table: over-engineering for what
  is effectively a JSON config. The `SidebarConfig.Items` array in the
  existing JSON column is simpler and backward-compatible.

**Consequences:**
- Folders are true organizational containers — no entity records, no search pollution.
- Owners can reorder the entire sidebar (addons, categories, links).
- Large campaigns (500+ entities) load incrementally with tag filtering.
- Favorites persist across devices.
- Dual-parent model (`parent_id` vs `parent_node_id`) requires care in
  queries and the reorder service to keep them mutually exclusive.

---

## ADR-033: Startup Health Check System

**Date:** 2026-03-26
**Status:** Accepted

**Context:** Migration 000018 added `archived_at` and `join_code` columns to
campaigns, but wasn't applied in a dev environment. Repository queries already
referenced these columns, causing server errors on campaign pages. This revealed
that Chronicle had no proactive detection of schema drift, unapplied migrations,
or security misconfigurations at startup.

**Decision:** Comprehensive startup health check system in
`internal/database/healthcheck.go`. Runs after `RunMigrations()` but before
route registration. Five checks:

1. **Migration version** — Verifies DB is at expected version (currently v18).
   Detects dirty state and logs force-retry instructions.
2. **Critical columns** — Queries `information_schema.COLUMNS` for required
   table/column pairs. Catches schema drift from failed or skipped migrations.
3. **DB connectivity** — Pings with 5s timeout. Monitors connection pool
   utilization (warns at 80% capacity).
4. **Security audit** — Detects weak/default DB passwords in production, HTTP
   BaseURL (CSRF cookie vulnerability), overprivileged DB user grants (SUPER,
   FILE, PROCESS), and world-writable schema_migrations table.
5. **Pre-migration backup** — `PreMigrationBackup()` runs mysqldump with gzip
   before migrations. Auto-rotates old backups (configurable retention).
   Silently skips if mysqldump is unavailable.

Server exits with `os.Exit(1)` if any check fails. Configuration via
`HealthCheckConfig` struct in `cmd/server/main.go`.

**Alternatives Considered:**
- Runtime health endpoint: too late, server already accepts traffic with bad state.
- External monitoring (Prometheus/Grafana): doesn't prevent startup with broken schema.
- Manual `make migrate-up` before deploy: error-prone, forgotten in practice.

**Consequences:**
- Schema drift detected before first request is served.
- Database backed up before destructive migrations.
- Security baseline (strong password, HTTPS, least-privilege DB user) enforced
  on every start.
- Adds ~100ms to startup time (information_schema queries are fast).
- mysqldump dependency is optional — backup silently skipped if not installed.

---

## ADR-034: Asymmetric Corner Bleed CSS Effect System

**Date:** 2026-03-26
**Status:** Accepted

**Context:** Chronicle needed a cohesive visual language for interactive elements
(buttons, navigation items) that feels distinctive and polished, consistent
across the entire UI.

**Decision:** CSS pseudo-element (`::after` for buttons, `::before` for sidebar
nav) with stacked `linear-gradient` backgrounds creating an asymmetric glow
effect. Design principles:

- **Right edge strongest** — Gradients from right side have highest opacity.
- **Bottom heavier than top** — Bottom edge thicker (8px) vs top (4px).
- **Bottom-right corner heaviest** — 50% opacity at 8px, vs top-left at 15-20%.
- **Click/active state** — All corners expand to 100% width (full wrap-around)
  with 0.2-0.3s transition.
- **`.btn-pressed` JS class** — Added on `mousedown`, removed 300ms after
  `mouseup` for a tactile linger effect (`boot.js`).

Applied consistently across six button variants (primary, ghost, secondary,
danger, warning, success) in `static/css/components/buttons.css` and sidebar
navigation in `static/css/components/sidebar.css`.

Sidebar uses `::before` (not `::after`) to avoid conflicting with the icon-only
tooltip which uses `::after`. Glow is suppressed in icon-only mode (too narrow).

**Alternatives Considered:**
- `box-shadow`: symmetric only, can't create directional weighting.
- `border-image`: limited transition/animation support.
- SVG filters: performance overhead, harder to maintain.
- CSS `outline`: no gradient or directional control.

**Consequences:**
- Unified visual language across all interactive elements.
- Pure CSS except for the 300ms linger JavaScript (6 lines in boot.js).
- Two pseudo-elements needed per element (one for glow, tooltips need separate).
- Sidebar nav avoids `::after` conflict by using `::before` instead.

---

## ADR-035: Operator Backup as POSIX Shell Script + Make Target

**Date:** 2026-04-25
**Status:** Accepted

**Context:** Chronicle 0.0.1 needed an operator-runnable backup mechanism
plus a deployment runbook. The Go codebase already had
`PreMigrationBackup` (`internal/database/healthcheck.go:305-350`) — a
boot-time safety net invoked before migrations — but no on-demand path
for operators, no media or Redis coverage, and no manifest pairing.
Worse, the in-process backup was silently disabled in production
because the runtime image didn't ship `mysqldump`.

The choice was: build the operator backup as a Go subcommand of the
`chronicle` binary, or as a shell script under `scripts/`.

**Decision:** Shell script (`scripts/backup.sh`, `scripts/restore.sh`)
invoked via Make targets (`make backup`, `make restore`,
`make backup-check`, `make backup-list`). Inside the chronicle container
via `docker compose exec` for the compose path; same script runs
standalone on bare-metal hosts.

POSIX `sh` (Alpine `/bin/sh` is `ash`); no bashisms. `set -eu`. Exit
codes: `0` success, `1` operator error, `2` precondition failure,
`3` backend tool failure. Manifest pairs DB + media + redis artifacts
with sha256 + chronicle version + migration version so `restore.sh` can
refuse mismatched sets.

**Alternatives considered:**
- Go subcommand (`chronicle backup`, `chronicle restore`): would require
  rebuilding the image to update backup logic, adds a Cobra-style CLI
  surface to maintain, and still has to shell out to `mysqldump`
  internally — net loss vs. a shell script.
- Sidecar `chronicle-backup` service in compose: extra image, extra
  cron surface, more state to keep in sync. Rejected; the existing
  chronicle container already has the credentials, the volume, and
  (after this change) `mariadb-client`.
- Host-only script: forces every operator to install `mariadb-client`
  outside the container. Same script supports this case via env
  variables, but in-container is the documented primary path.

**Consequences:**
- Operators can update backup logic without rebuilding the image.
- `make backup-check` is a cheap CI surface for verifying that env vars
  and tool availability are correct.
- The `Dockerfile` runtime stage now installs `mariadb-client` and
  `gzip`. ~+15MB; this also lets the existing `PreMigrationBackup`
  actually function in production.
- Two retention systems coexist: `BackupMaxAge` (hardcoded 7d) for the
  in-process pre-migration files, and `BACKUP_RETENTION_DAYS` (default
  7d) for the operator-script artifacts. Filename prefix
  (`chronicle_pre_migrate_*` vs `chronicle_db_*`/`chronicle_media_*`)
  cleanly partitions which rotator owns which file. Future cleanup PR
  may unify; not a 0.0.1 blocker.
- Restore is a sysadmin operation only — no admin-UI restore path,
  intentionally. Documented in `docs/deployment.md` §9.
  *(Reversed by ADR-036 below.)*
- Documentation lives in `docs/deployment.md`; `scripts/README.md`
  documents the script convention itself.

## ADR-036: Admin UI for Backup and Restore

**Status:** Accepted (2026-04-26).
**Reverses:** ADR-035's "restore is sysadmin-only, no admin-UI path"
consequence.

**Context:**
ADR-035 deferred a web UI for both backup and restore on the grounds
that backup is rare enough for `make backup` from the host, and
restore is destructive enough that gating it behind a shell session
adds useful friction. A user request now makes that compromise the
bottleneck: operators who deploy chronicle to a VPS or container host
don't always have direct host shell access (or the muscle memory for
`make` invocations under their orchestrator), and "log in to the host
to recover from a backup" turns recovery from "click a button" into
"find the runbook, get an SSH key, hope BACKUP_DIR is mounted where I
think." For users running their own host, the UI is the only realistic
path.

**Decision:**
Two new admin-only plugins:

- `internal/plugins/backup` — `/admin/backup` page lists artifacts in
  `BACKUP_DIR` and exposes "Run backup now" + "Download artifact"
  buttons. Shells out to `scripts/backup.sh` synchronously under a
  20-minute timeout.
- `internal/plugins/restore` — `/admin/restore` page lists parsed
  manifests, with a per-row form requiring the operator to type the
  literal word `RESTORE` into a text field before the request is
  accepted. Shells out to `scripts/restore.sh --manifest <path>
  --yes --force` under a 30-minute timeout.

Security guarantees on top of the existing `RequireSiteAdmin` and
`CSRF` middleware:

- **Single-flight lock**: in-process mutex serializes both backup and
  restore against themselves. Concurrent requests get HTTP 409 with a
  clear message rather than spawning a second mysqldump or restore.
- **Rate limit**: per-IP sliding window — backup `2/hour`, downloads
  `20/hour`, restore `1/hour`. Bounds attack surface even when
  CSRF-protected.
- **Process group kills**: every shell-out runs with `Setpgid: true`;
  cancel sends `SIGKILL` to the negative PID so any descendants
  (mysqldump, tar, gzip, mariadb) die together.
- **Output cap**: stdout and stderr go through 64 KB ring buffers so
  a runaway script can't OOM the chronicle process.
- **Path safety**: every filename parameter is validated against
  `BACKUP_DIR` with both basename and prefix checks. Restore
  additionally requires the file to match `chronicle_manifest_*.txt`.
- **Typed-string confirmation**: restore requires `confirm=RESTORE`
  in the request body. Mirrors the shell script's interactive prompt
  so muscle memory transfers between the two surfaces.
- **No silent coalescing**: if a backup or restore is in flight,
  concurrent requests get 409, never "you joined the running one".

**Consequences:**
- Admins can recover without shell access. Big UX win for anyone
  running chronicle as a managed service.
- Restore from the UI is now possible — `docs/deployment.md` §9 must
  document this as the recommended path for VPS deployments while
  keeping the `make restore` flow as the in-shell escape hatch.
- The two plugins ship as independent PRs (backup #257, restore here).
  They duplicate small helpers (`capBuf`, basename validation) — once
  both land we can extract a shared internal package without churning
  the public surface.
- The 1/hour rate limit on restore is deliberately tighter than
  backup. Restore is at most an emergency operation and even one
  call/hour is generous.
- The "run backup before restore" advice in the UI's red banner is
  human-only safety; the system does NOT auto-snapshot before
  restoring. A future enhancement may add an opt-in pre-restore
  backup, but for now the operator owns that step (and the existing
  in-process pre-migration backup gives some protection too).
