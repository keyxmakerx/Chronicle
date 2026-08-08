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
- Entity show page reads layout_json to render the profile — wired via `BlockRegistry` + `DefaultLayout()`/`CharacterLayout()`, consumed by `show.templ`.

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
**Status:** Accepted — Layer 1 (declarative content packs) and Layer 3 (WASM logic
extensions) are both implemented in `internal/extensions/` (manifest/applier +
`wasm_host.go` on Extism/wazero, pinned in `go.mod`).

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

## ADR-037: Pre-migration backup symmetry with operator backups

**Status:** Accepted (2026-04-26).
**Refines:** ADR-035 (operator backup) and ADR-036 (admin UI for backup
and restore).

**Context:**
The original `PreMigrationBackup` (added in the ADR-035 era) captured
only the database — a single `chronicle_pre_migrate_<TS>.sql.gz` per
boot. Three gaps surfaced once the operator backup pipeline matured:

1. **No media or Redis snapshot.** A migration that changes how media
   IDs are encoded would leave the on-disk media tree out-of-sync
   with any restored DB. Same for Redis (sessions only, but
   recoverable).
2. **Fail-open on tool absence.** If `mysqldump` was missing from the
   image, the function logged a warning and returned. Migrations
   proceeded with no rollback. In production that's a silent
   data-loss risk hidden behind a green deploy.
3. **No version stamping.** The artifact filename was just a
   timestamp; the operator had to remember which schema version the
   DB was at when it was taken. Operator backups already embed
   `migration_version=<N>` in their manifest; pre-migration didn't.

**Decision:**
Extend `PreMigrationBackup` so its output is interchangeable with
`scripts/backup.sh` output:

- Same artifact prefixes
  (`chronicle_pre_migrate_db_*.sql.gz`,
  `chronicle_pre_migrate_media_*.tar.gz`,
  `chronicle_pre_migrate_redis_*.rdb`).
- Same manifest format (`chronicle_pre_migrate_manifest_*.txt`) with
  `chronicle_manifest_version=1`, `chronicle_version=`,
  `migration_version=`, plus per-artifact sha256 + size.
- One distinguishing line: `chronicle_pre_migrate=1` so restore
  tooling can label boot-time bundles separately from
  operator-triggered ones.

Add `BACKUP_REQUIRED=1` env var: when set, any artifact failure
aborts startup before migrations apply. Default remains fail-open
for backwards compatibility with development setups that lack
`mariadb-client`.

Three security/correctness defenses:

- **Atomic writes.** Each artifact written to `<file>.partial` and
  renamed only after sha256 + size verification. Half-written files
  never persist.
- **0600 file mode** on every artifact and the manifest. The dump
  contains all data; loose permissions on a multi-user host would
  leak it.
- **Zero-byte rejection.** Any artifact that ends up zero bytes is
  treated as a capture failure (covers silent `mysqldump` exit-zero
  on empty DB, `redis-cli` writing nothing on no-permission, etc.).

**Consequences:**

- Pre-migration snapshots become **first-class restorable artifacts**.
  `scripts/restore.sh --manifest chronicle_pre_migrate_manifest_<TS>.txt`
  works the same as it does for operator backups; the admin restore
  UI surfaces them in the same list.
- Production deployments can opt into fail-closed via
  `BACKUP_REQUIRED=1` for a real "no rollback story = no migration"
  guarantee. Existing deployments are unaffected (default still
  fail-open).
- The retention sweep was extended to glob the four new artifact
  families plus a backwards-compat pattern for legacy
  `chronicle_pre_migrate_<TS>.sql.gz` files (no `_db_` infix). 7-day
  retention applies to all five; legacy files time out and disappear
  on their own as the new format takes over.
- `redis-cli` is now a soft dependency: present → Redis snapshot
  included; absent → skipped with a debug log. Chronicle's only
  Redis state is sessions, so a missing snapshot only means "users
  get logged out on rollback" — not data loss. Production images
  should still include `redis-tools` for the safety net.
- Documentation: `docs/deployment.md` §5 gains `BACKUP_REQUIRED`,
  `BACKUP_SCRIPT_PATH`, `RESTORE_SCRIPT_PATH`, `CHRONICLE_VERSION`
  entries; §7 (Rollback / Scenario A) is rewritten to use
  `scripts/restore.sh --manifest` against pre-migration manifests.

## ADR-038: Widget bindings — polymorphic, FK-free association table

**Date:** 2026-06-07 · **Status:** Accepted · **Wave:** C-WIDGET-BINDING-P1-SPINE (E "real Wave-4")

**Context.** The widget-binding framework needs to map a *host*
(entity / entity-type / dashboard) to a *data instance* (a calendar / map /
timeline …) per *widget type*. `entities.map_id` is the existing hardcoded
special case (one entity → one map). We need the generic table.

**Decision.** `widget_bindings(id, campaign_id, host_type, host_id,
widget_type, instance_id, …)` is **polymorphic and FK-free** on both `host_id`
and `instance_id`. `host_type`/`widget_type` are an immutable, append-only
namespace validated **in app code, not a DB enum**.

**Why not the integrity-preserving alternatives** (the ones a DBA would reach
for first):
- *Exclusive-arc / nullable-FK-per-type* (`calendar_id`, `map_id`, … columns,
  each FK'd, with a CHECK that exactly one is set) and *join-table-per-type*
  (`entity_calendar_bindings`, `entity_map_bindings`, …) both **buy real
  referential integrity** — but at the cost of **per-widget-type schema churn**,
  which is exactly the hardcoding this framework exists to abolish (the
  "dynamic, not hardcoded" requirement). A new widget type would mean a
  migration every time.
- More decisively, a hard FK is **impossible here**: `instance_id` references a
  *different* table depending on `widget_type` (calendars **or** maps **or**
  timelines), and those are **plugin-owned** tables. Per the migration-ordering
  rule (`.ai/conventions.md` §Migration Safety — core runs before plugins, and
  a binding table referencing plugin tables would crash a fresh DB), the FK we'd
  want can't be collected anyway.

**Consequence / mitigation (this is load-bearing, not optional).** FK-free
means the *application* is the only integrity backstop (MariaDB has no RLS).
Integrity is enforced as an **AND** of three mechanisms — not "or":
1. **Per-plugin delete hook** — `Service.OnInstanceDeleted` (owning plugins
   call it when an instance is deleted).
2. **Always-on render-time orphan guard** — `Resolve` validates every candidate
   via `WidgetType.InstanceExists` (which also enforces campaign scope) and
   skips/sweeps dead bindings, falling through to the default.
3. **Periodic campaign integrity sweep** — `Service.Sweep`.
Campaign scope is pushed down to the repository signature (an unscoped read is
unrepresentable) and checked on **both** `host_id` and the resolved
`instance_id`. The table lives in the `widgetbindings` plugin; being FK-free,
its migration order vs calendar/maps/timeline is irrelevant.

**References.** `reports/chronicle/2026-06-07-widget-binding-framework-prep-audit.md`
(§3), `reports/chronicle/2026-06-07-widget-binding-precedent-research.md`
(polymorphic-association / multi-tenant-scoping precedent; Foundry #9818
cascade-direction bug → directional cascade test).

---

## ADR-039: Player Character Claiming — Owner-Toggleable Addon + Per-Type Claimable Flag

**Date:** 2026-06-19 · **Status:** Accepted · **Phase:** PC-CLAIM (Stages 1–3 merged)

**Context.** Chronicle needs bidirectional player-character binding for both
Foundry sync and internal campaign management. A GM must know which player owns
which character; a player must be able to claim an unclaimed character. The
feature must be optional (campaigns can opt-in) and extensible (not every
character-shaped type needs claiming).

**Decision.** Three-part design:

1. **Owner-toggleable addon** (`player-character-claiming`): GMs opt-in per
   campaign via the Addons panel. Creating a "Player Character" sub-type (a
   dedicated entity type with `preset_category == "player_character"`) is
   gated on the addon being enabled. UI surfaces (claim button, owner roster,
   claimable toggle) are all hidden when the addon is off.

2. **Per-type claimable flag** (`entity_types.claimable BOOLEAN NULL`): allows
   the Owner fine-grained control. When set (TRUE/FALSE), the Owner's choice
   is authoritative. When NULL (default/unset), the legacy heuristic applies
   (preset_category "character" or slug `*-character`). This allows existing
   campaigns to keep claiming on their "Character" type without manual
   re-configuration.

3. **"Player Character" sub-type + legacy fallback**: New campaigns can use
   the dedicated "Player Character" sub-type when the addon is on (explicit,
   separate from characters that might be NPCs). Existing campaigns keep
   claiming on their existing "Character" type (heuristic-based). Both paths
   are supported; neither overwrites the other.

**Claimable-by-default when addon is on:** When an Owner enables the addon
and creates a new type, the claimable flag defaults to true. This reflects
the mental model: "I turned on the feature" → "I want my character types to
be claimable." If the Owner wants a character-shaped type that is *not*
claimable (e.g., "NPC", "Companion"), they can toggle the flag to false.

**Why this design vs alternatives:**

- **Addon on/off (vs always-on):** Existing campaigns default off. Zero
  surprise. Opt-in ceremonies reduce feature cruft for campaigns that don't
  use the feature.

- **Per-type flag (vs all-or-nothing):** Not all character-shaped entities
  should be claimable. An NPC generator, a companion template, or an "Open
  Seat" character all have the same *shape* as a PC but aren't owned by
  players. Per-type control is finer-grained and avoids category-wide toggles.

- **Dedicated PC sub-type (vs hardcoding "Character"):** Separates the concerns.
  "Player Character" is a campaign-wide opt-in with the addon; "Character" is
  the general-purpose entity type, which may or may not be claimable. Foundry
  sync (Stage 4) can look for the PC sub-type specifically and auto-claim
  player-owned actors into it.

- **Heuristic fallback (vs migration-time decision):** Existing campaigns don't
  need a migration. The `claimable` column defaults NULL, and the service falls
  back to the existing heuristic (preset_category "character"). Campaigns can
  opt-in to explicit control by setting claimable on their types. Zero
  disruption.

**Audit trail:** New distinct audit actions (`entity.claimed` and
`entity.owner_changed`) make claiming and reassignment visible in the activity
log (Stage 1). The claiming player and the character's real name are recorded,
not opaque IDs.

**Stage 4 (pending):** When the Foundry sync module detects the addon, it maps
player-owned PC actors (by actor type + GM ownership) to the PC sub-type and
auto-claims them. This bridges Foundry and Chronicle without manual operator
configuration.

**References.** `internal/plugins/entities/.ai.md` §"Player Character Claiming",
`entities/{service.go, handler.go}` (isPlayerCharacterType, isClaimableType,
ClaimEntity, AssignOwner), `entities/{claim_banner.templ, claim_overview_test.go}`,
migration 000029.

## ADR-040: Dynamic-surface frame — a system-agnostic Widget, not a hardcoded sheet

**Status.** Accepted (2026-06-22). **Context.** The operator wants a dynamic UI: a
mini surface that promotes into a full-screen sheet with expandable boxes, action
overlays, and drill-downs — applied first to the character sheet, later the rulebook.

**Decision.** Build it as ONE reusable, **system-agnostic frame** in the Widget tier
(`Chronicle.surface`, `static/js/widgets/dynamic_surface.js`) — a motion-preset library,
an overlay stack, an expand/collapse box, a memoized data provider, a mini→full
`launch`, and a schema-driven `mount` — rather than a bespoke renderer per sheet. **The
frame owns motion + structure; a System supplies box BODIES** via `registerBox(name, fn)`.
A System never writes animation code; it names which preset fits each card.

**Why.** Chronicle is genre-agnostic; the same paradigm must serve any game system and
the rulebook. Separating the frame (Chronicle) from the content (System/plugin) keeps
"Chronicle owns the template; the system fills it." Built on existing motion tokens
(`--ease/-dur/-elev-*`) + a new `--surface-*` contract, so it stays theme-aware; all
presets collapse to a fade under `prefers-reduced-motion`. No new tables — surfaces ride
a declarative schema; per-user view-state rides localStorage.

**References.** `static/js/widgets/dynamic_surface.js` + `.ai.md`; the admin surface demo
(`/admin/design-lab`); Cordinator `plans/2026-06-21-dynamic-widget-ui-framework-design.md`.

## ADR-041: `character_surface` as a layout BLOCK + the default for player-character types

**Status.** Accepted (2026-06-22). **Context.** Player characters should open the dynamic
"big widget" sheet by default, yet stay editable in the existing layout customizer.

**Decision.** Register the surface as a normal entity-page **layout block**
(`character_surface`, `Contexts:["template"]`, `Singleton`) whose renderer emits a
`data-widget="dynamic-surface"` container with the entity's data **seeded inline**. Make
`CharacterLayout()` (the block + permissions) the default layout for
`isPlayerCharacterType` types in `CreateEntityType`, instead of `DefaultLayout()`.

**Why.** Because it's a registry-driven block, it appears in the layout-editor palette and
owners compose/rearrange it like any block — no separate "sheet editor." The default
applies only to NEWLY created PC types (we never rewrite existing customized layouts).
**Security:** the description box mounts the same role-aware `editor` widget the standard
`entry` block uses (so GM-only secrets aren't leaked) rather than inlining `EntryHTML`.

**References.** `entities/{character_surface.go, character_surface_block.templ,
block_registry_core.go, model.go:CharacterLayout, service.go}`,
`static/js/widgets/character_surface.js`; `entities/.ai.md` §`character_surface`.

## ADR-042: Cross-plugin section injection — `NPCSectionProvider`

**Status.** Accepted (2026-06-22). **Context.** The unified Characters page (in the core
`entities` plugin) must render an NPCs/Monsters section owned by the `npcs` addon, without
the core plugin importing the addon (rule 8) or duplicating NPC logic.

**Decision.** `entities` defines an `NPCSectionProvider` interface returning a
`templ.Component`; `npcs.Handler.NPCSection` **structurally** satisfies it and is injected
via `entityHandler.SetNPCSectionProvider(npcHandler)` at app wiring. The npcs plugin
renders its own section (featured tag-row + revealed list + reveal toggle, reusing
`NPCCardComponent`); entities just slots the component in when the `npcs` addon is on.

**Why.** Keeps domain ownership where it belongs (npcs owns NPC rendering), preserves the
dependency direction (npcs→entities, never the reverse — npcs needs no import of entities
since the interface is satisfied implicitly), and generalizes: any addon can contribute a
section to a core page this way. The standalone `/npcs` gallery page redirected into this.

**References.** `entities/handler.go` (NPCSectionProvider, Characters), `npcs/handler.go`
(NPCSection), `npcs/npc_section.templ`, `app/routes.go` (SetNPCSectionProvider).

---

## ADR-043: Extension Settings / Onboarding framework (`SetupProvider`)

**Status.** Accepted (2026-06-24).

**Context.** Enabling an addon (or a game system, which auto-registers as one per ADR-031)
fired SILENT lifecycle hooks (`ApplySystemPresets`, `ApplyAddonEnableEffects`). That hid
real decisions inside boot automation and produced the duplicate-player-character-category
artifact (see ADR-044). The owner wanted each extension to own a visible settings/onboarding
page — renderable as an integrated overlay — driven by a reusable framework, with a nudge on
enable.

**Decision.** A Go `SetupProvider` interface + slug-keyed registry lives in the `addons`
plugin (it already owns the toggle, the per-campaign list, and `config_json` persistence).
A provider supplies `RunChecks` (health/QOL findings with a severity), `Questions`
(onboarding inputs), and `Apply` (idempotent). A generic handler + three Templ components
(`extension_settings_page` / `_overlay` / `_fragment`) render ANY provider as a full page or
a modal overlay — so every extension gets a consistent settings surface for free. Concrete
providers live in the **app layer** (like `PresetApplier`) and are wired via
`addonService.RegisterSetupProvider(...)` in `app/routes.go`, so `addons` never imports
`entities`/`systems`. Per-campaign setup state (`{completed, dismissed, answers}`) persists
under **`campaign_addons.config_json["setup"]`** — no new table or migration. The Extensions
hub card carries a `NeedsSetup` flag (computed per-campaign in `addonListerAdapter`) that
shows a "Setup" badge + an "Open setup" button (`hx-get` the overlay into a page-level modal
container). On enable, the toggle handler co-emits a `chronicle:notify` toast alongside the
existing `extensions-hub-refresh` HX-Trigger.

**Alternatives considered.** (a) A declarative manifest-driven schema for checks — rejected
for now because the first provider's checks must inspect live campaign data (Go logic);
manifest-defined QOL notes can come later for external packs. (b) A new top-level settings
nav — rejected; the Extensions hub (ADR-029) is the established surface. (c) A new
`extension_setup` table — rejected; `config_json` already exists with `UpdateCampaignConfig`.

**Consequences.** Enabling stays safe (the idempotent `EnsurePlayerCharacterType` still runs)
while destructive/ambiguous choices move into the owner-driven wizard. New providers (e.g.
calendar, maps) register with zero template changes.

**References.** `addons/setup_provider.go` (interface + registry + state),
`addons/setup_handler.go` + `addons/routes.go` (3 owner-gated routes),
`addons/extension_settings_*.templ`, `campaigns/handler.go` (`PluginHubAddon.NeedsSetup/HasSetup`),
`app/routes.go` (`addonListerAdapter`, `RegisterSetupProvider`), `app/setup_pc.go`.

---

## ADR-044: PC duplicate reconciliation moves from boot migration → owner-triggered Apply

**Status.** Accepted (2026-06-24). **Amended same day after a production incident** — the
original draft proposed DELETING migration `000030`; that crash-looped prod and was reverted
(`9990134`). The migration is RETAINED; the owner-driven path is additive.

**Context.** A campaign could end up with BOTH a generic "Player Characters" type (holding the
claimed character entities) and a game system's own character type (e.g. Draw Steel's empty
"Heroes") — an enable-ordering artifact the non-destructive boot path nests but never merges.
The one-time, guarded boot migration `000030_consolidate_player_character_duplicate` auto-merges
the unambiguous case (exactly one of each) on deploy.

**Decision.** KEEP migration `000030` permanently (it is applied in production DBs and is
idempotent/guarded). ADDITIONALLY provide an owner-triggered, single-campaign service method
`entities.MergeDuplicatePlayerCharacterType` (+ repo `MoveEntitiesAndDeleteType`, one
transaction), surfaced as a check on the player-character extension settings page (ADR-043),
for cases the one-time migration does NOT cover: ambiguous campaigns (more than one of either
category → a human-readable `apperror`, closing the deferred PC-DUP-GUARD-2) and duplicates
that arise AFTER the migration ran. It classifies the unambiguous (generic → system) pair by
`preset_category`/`slug`/`is_default` only (no system names), moves the generic's entities onto
the system type (claims follow via `entities(id)`), and deletes the emptied generic. "Heroes
wins": the system's own type survives. The owner additionally chooses the system name vs a
custom name in the same wizard. The two mechanisms are complementary, not exclusive.

**Migration safety (the lesson — a real incident).** The original draft proposed DELETING
`000030`. That is UNSAFE and crash-looped production: golang-migrate's `file://` source
(`internal/database/migrate.go`, `m.Up()`) must contain a migration file for EVERY version up
to the DB's current recorded version. A prod DB already at `version=30` with no `000030` file
on disk fails with `no migration found for version 30: read down for version 30: file does not
exist` — an unrecoverable boot loop (the runner only auto-recovers `ErrDirty`, not a
missing-version source; the health-floor check never even runs). **RULE: never delete or
renumber a migration that any live database has applied — keep it forever, even when later
superseded.** (Matches CLAUDE.md "Never edit an applied migration" — extend it to "never
delete" one.)

**Incident-response lesson (the fix that didn't land).** The `000030` restore was committed to
the feature branch *after* PR #498 had already been **merged and closed** at the pre-restore
commit (`e71706f`). The fix therefore sat on the branch, **never reaching `main`** — and the
follow-up "correction" only edited the *already-merged* PR's body, which changes nothing in the
tree. `main` stayed broken (missing `000030`, no robustness) until a **fresh** hotfix PR (#499)
carried the restore in. **RULES:** (1) a post-merge fix needs a NEW PR — editing a merged PR is
inert; (2) after shipping any incident fix, VERIFY it is actually on `main`
(`git ls-tree origin/main -- db/migrations/` / check the merged SHA), don't assume the branch
state equals `main`; (3) prefer **squash-merge** so a "deleted-then-restored within the branch"
sequence can't merge at an intermediate broken commit.

**Consequences.** Existing prod duplicates are healed automatically by `000030` on deploy
(unambiguous case) AND can be reconciled by the owner from the settings page (any case, full
visibility). No migration is ever removed. The owner-merge is idempotent (once the generic is
gone, a re-run is a no-op success).

**References.** `entities/service.go` (`MergeDuplicatePlayerCharacterType`,
`PlayerCharacterSetupSnapshot`), `entities/repository.go` (`MoveEntitiesAndDeleteType`),
`app/setup_pc.go` (the provider), ADR-043, ADR-039 (PC claiming).

---

## ADR-045: Migration robustness — fail-safe boot, append-only guards, schema-only policy

**Status.** Accepted (2026-06-24). The durable fix for the `000030` incident (ADR-044).

**Context.** Deleting an applied migration crash-looped production. Root cause was THREE
things: (1) golang-migrate's `Up()` hard-errors when the DB version exceeds the on-disk
source's highest version — this fires on a deleted migration AND on a normal image rollback;
(2) `restart: unless-stopped` turns any fatal boot into a ~1/sec loop; (3) the pre-migration
backup ran unconditionally before every boot, so each loop iteration wrote a full dump (the
"6 backups/min" symptom). Audit also found a live `ExpectedMigrationVersion` drift (29 vs the
real max 30) and 15 historical migrations using non-idempotent `ADD COLUMN`.

**Decision — three layers.**

1. **Runtime (boot fails safe, never crash-loops).** `database.MigrateWithBackup`
   (`internal/database/migrate_state.go`) replaces the unconditional backup-then-migrate
   sequence. It reads the DB version + highest on-disk migration ONCE, then:
   - **DB ahead of the build** → log an actionable warning and **start anyway** (skip `Up()`).
     Migrations are additive, so an older binary runs fine on a newer schema; the startup
     health checks backstop a destructive rollback. Fixes the deletion case AND ordinary
     image rollbacks.
   - **up to date** → skip BOTH backup and `Up()` (ends the backup-on-every-restart storm).
   - **pending** → back up, then migrate.
   A **dirty** database now FAILS FAST with restore guidance (the old `Force(v-1)` auto-retry
   looped forever on non-idempotent migrations). `fatalBoot` (`cmd/server/main.go`) sleeps
   `BOOT_FAIL_BACKOFF` (default 45s) before exit so unrecoverable errors retry ~1/min.

2. **CI guards (prevent the mistake).** `tools/check-migration-immutability.sh` (CI step)
   fails any PR that deletes or edits a migration already on the base branch.
   `internal/database/migrate_test.go` gains: version-pin (`ExpectedCoreMigrationVersion ==
   max(core migration)`), idempotent-DDL lint (grandfathering the immutable historical files),
   gapless numbering, and plugin up/down-pair coverage.

3. **Visibility (admins see + act) — the unified Database page.** `/admin/database` is one
   tabbed control surface (Alpine `x-data` tabs, the `storage.templ` pattern) so an operator
   reasons about — and recovers from — the database from a page, not from crash logs:
   - **Migrations** — core schema version + dirty flag + pending count + a DB-ahead/downgrade
     banner (the runtime A3 state, made visible), plus the existing per-plugin grid + "Apply
     Pending" + history.
   - **Health** — the SAME `RunStartupHealthChecks` the boot path runs, rendered live with
     pass/warn/fail pills. The runner was split: `database.RunHealthChecks` returns a structured
     `HealthCheckResult` with no logging/exit, and `RunStartupHealthChecks` wraps it for boot.
     The check config was extracted to `app.StartupHealthCheckConfig(cfg)` so **boot and the
     admin tab share one definition and can never disagree.** `GET /admin/database/status`
     exposes core+plugin status as JSON for external monitoring.
   - **Backups** — the existing `backup`/`restore` plugins surfaced (no new engine): artifacts
     with an **Auto** (pre-migration) vs **Manual** badge, restorable snapshots with their
     Chronicle/schema versions, last-auto-backup recency, and create/download/restore actions.
   - **Schema** — the D3 diagram, lazily mounted on first tab activation so it reads a real
     container width instead of the hidden-tab zero-width fallback.

   **Cross-plugin wiring stays decoupled** (the established `DatabaseExplorer` / ADR-042
   `NPCSectionProvider` pattern): `admin` defines `HealthChecker` / `BackupLister` interfaces
   (`database_health.go`); the app layer injects adapters (`internal/app/admin_db_adapters.go`)
   over the boot health config and the backup/restore services, so `admin` imports neither.

**Policy — migrations are APPEND-ONLY and SCHEMA-ONLY.** Never delete, edit, or renumber a
migration that any live DB may have applied (the immutability guard enforces this). New DDL
must be idempotent (`IF [NOT] EXISTS`). One-time DATA corrections do NOT go in migrations —
use an idempotent reconciler (an `EnsureX`/`MergeX` service method run from a boot backfill,
an addon-enable hook, or an owner-triggered `SetupProvider`), as in `app/setup_pc.go` +
`entities.MergeDuplicatePlayerCharacterType`. Reconcilers are idempotent, handle cases that
arise later, and surface ambiguity to a human — none of which a one-shot data migration can do.

**Consequences.** Upgrades and rollbacks "just work" or fail with a clear message; the incident
class (delete/edit/renumber/gap/non-idempotent/version-drift) is blocked at PR time; admins
manage migration state from a page. The historical `000030` stays (it's applied; the
immutability guard enforces it can't be removed again).

**References.** `internal/database/migrate_state.go`, `internal/database/migrate.go` (dirty
fail-fast), `internal/database/healthcheck.go` (`RunHealthChecks` split), `cmd/server/main.go`
(`fatalBoot`), `internal/database/migrate_test.go` (guards), `tools/check-migration-immutability.sh`,
`internal/app/{health_config,admin_db_adapters}.go` (shared config + tab adapters),
`internal/plugins/admin/{database_service,database_health,handler,database.templ}`,
ADR-044, ADR-028/030 (plugin migrations), ADR-037 (pre-migration backup), ADR-042 (cross-plugin
injection pattern).


---

## ADR-047: World-state broadcasts are audience-SPLIT, not audience-filtered

**Status.** Accepted (2026-07-26, C-CAL-WORLDSTATE-WIRE). Numbered 047 because
ADR-046 is claimed by the in-flight RSVP branch (PR #566); the two are
independent.

**Context.** `calendar.worldstate.changed` and `calendar.weather.zones.changed`
were publisher-side dead letters: live emitters, no `case` in
`calendarEventPublisherAdapter.PublishCalendarEvent`, so both hit its
`default: return` and were discarded before the bus. The operator's meteors
and eclipses had never reached a WebSocket client, which is why "celestial
events unfindable/unsyncable" produced no trace to follow — there was no trace.

Routing them is one line each. The real decision is what the world-state
payload may carry. The original comment on `SetWorldState` justified a minimal
`{date, moodTint}` payload precisely on privacy grounds: *"so a player WS
subscriber never receives GM-only events through the change signal — clients
re-GET the seed with their own role."* That reasoning is sound and the
conclusion was still wrong in practice — a consumer that only learns "something
changed" cannot announce the meteor shower the GM just triggered, and the
"re-GET with your own role" escape hatch did not exist for token clients at all
(the world-state GET was session-auth only).

**Decision.** Enrich the payload, and get the privacy property from a SPLIT
rather than from omission.

`websocket.Message.RequiresDM` is a per-message flag consumed by the hub's
broadcast loop. One message therefore cannot be rich for the GM and redacted
for the table. So a world-state change emits:

1. **always** — a player-safe payload, celestial events filtered to
   `visibility="everyone"`, `RequiresDM` unset;
2. **only when the date carries dm_only rows** — a second payload with the full
   set, `RequiresDM` set, which the hub drops for non-DM clients.

The DM copy is published under an INTERNAL event name
(`calendar.worldstate.changed.dm`) that the adapter translates to the same
public `ws.MessageType` with the flag set. The suffix never crosses the wire.
It exists so the audience can be expressed without widening
`CalendarEventPublisher` — an interface with four implementations, one of them
a hand-written test mock — for a single caller. Same interface-churn trade-off
ADR-046's branch made when it declined to widen `CalendarService`.

Both payloads carry the STABLE `calendar_celestial_events` id. Without it a
consumer cannot tell a re-broadcast or a reconnect replay from a new event, so
it must either drop the feature or duplicate a note per delivery — which is
exactly why the Foundry module abstained from celestial notes (PR #82).

**Also decided: one shape, not two.** The broadcast reuses `WorldStateEvent` /
`WorldStateMoon` / `WorldStateWeather` verbatim from `WorldStateSeed`, so a
consumer parses the same celestial/moon/weather shape whether it arrived by
push or by a GET refetch. The weather path already shipped two shapes for one
concept (flat `WeatherInput` pushed, nested `Weather` returned) and the module
had to write a normalizer for it; not repeating that is worth more than
matching the DB column names on the wire.

**Also decided: enrichment is best-effort.** A failed celestial/moon/weather
load degrades to the minimal `{date, moodTint}` broadcast rather than
abandoning the publish. The write already succeeded, and a consumer that hears
nothing has no way to learn it is stale. Given the bug being fixed here is a
silent drop, "degrade loudly" beats "drop quietly" by a wide margin.

**Consequences.** Additive on the wire — `date` and `moodTint` keep their exact
paths, which matters because the Foundry module shipped
`formatWorldstateLine` against them while the event was still undeliverable. A
date with dm_only content costs two broadcasts instead of one; a date without
costs one. `default: return` in the calendar adapter is now pinned by a test
over the WHOLE mapping, so the next emitter added without a case fails CI
instead of a play session.

**References.** `internal/websocket/message.go` (types),
`internal/app/routes.go` (adapter + `routes_calendar_ws_test.go`),
`internal/plugins/calendar/worldstate.go` (`WorldStateChangePayload`,
`BuildWorldStateChangePayloads`), `worldstate_service.go`
(`publishWorldStateChange`), `worldstate_wire_test.go`,
`internal/plugins/syncapi/{calendar_api_handler,routes}.go` +
`calendar_worldstate_handler_test.go`, `internal/websocket/hub.go:159-167`
(the audience gate this relies on), Chronicle-Foundry-Module PR #82
(the trace), cordinator `dispatches/chronicle/C-CAL-WORLDSTATE-WIRE.md`.
## ADR-046: Calendar events get first-class RSVPs, distinct from session attendance

**Status.** Accepted (2026-07-25). Supersedes the in-code ruling at
`internal/plugins/calendar/calendar_v2.templ` (the disabled "Collect RSVPs" toggle), which said
RSVPs on a calendar event would require an event↔session link. Implements the operator's #1
calendar-remodel priority (cordinator `plans/2026-07-24-calendar-remodel-requirements.md` §1
item 6, §2 "RSVP is nearly free"); dispatch C-CAL-RSVP-P1.

**Context.** The V2 event drawer shipped a DISABLED "Collect RSVPs" switch with a comment ruling
that enabling it needed an event↔session link, because the only attendance storage in the
product was the sessions plugin's `session_attendees` (+ `session_rsvp_tokens`). The operator's
actual ask is Outlook/Google-style "who's coming" **on the calendar**, for calendar events —
festivals, downtime windows, one-off scenes — most of which are not sessions and never will be.
Routing them through a synthetic session would have created a session row per feast.

**Decision — calendar events own their RSVP state.**

1. **Separate storage, mirrored pattern.** `calendar_event_rsvps` (one row per event+user,
   `UNIQUE(event_id, user_id)` so re-answering updates in place) and
   `calendar_event_rsvp_tokens` (single-use, expiring, opaque `crypto/rand` tokens), both in the
   calendar plugin's own migration chain (013). We MIRROR the sessions token pattern into
   DISTINCT tables rather than reusing `session_rsvp_tokens` — the precedent
   `slot_proposal_tokens` set. Reusing sessions' table would put a calendar FK on a sessions row
   and couple two plugins' schemas.
2. **Separate service.** `RSVPService` / `RSVPRepository` are their own narrow types, NOT
   additions to `CalendarService` / `CalendarRepository` — those are wide interfaces mirrored by
   a hand-written mock and the syncapi stub, so widening them makes every concurrent calendar
   lane collide. The RSVP service reads the event aggregate through a two-method
   `rsvpEventLookup` that the existing repo already satisfies.
3. **Visibility is the existing predicate, reused.** Every RSVP path (write, read, email
   fan-out, token redemption) gates on the calendar's existing `canUserView`. A second, subtly
   different visibility path is exactly how the entity-ties leak happened
   (C-CAL-ENTITY-TIES-LEAK-FIX); there is now one predicate.
4. **Cross-plugin via narrow interfaces + post-construction setters** (rule 8, ADR-042). The
   calendar declares `MailSender`, `RSVPNotifier`, `AvailabilityExceptionWriter`, and
   `RSVPMemberDirectory`; `internal/app/routes.go` binds them to the smtp and sessions services.
   The calendar gains no import edge into sessions — `internal/wire/plugin_import_guard_test.go`
   forbids it — and every seam is nil-safe, so an instance with no SMTP and no scheduler still
   does in-app RSVP end to end. The generic `sessions.NotifyUsers` added here is the first
   external writer of the notifications store the store was always documented (T-B2) to allow.
5. **Emailed actions are per-action single-use tokens** at `/calendar-rsvp/:token`, distinct
   from sessions' `/rsvp/:token`, with the GET-confirm / POST-apply split (a mail scanner's
   prefetch records nothing) and the CSRF double-submit the global middleware requires.
   Membership AND visibility are re-checked at REDEMPTION, not just at mint time, so a link
   cannot outlive the access that justified it.
6. **"Suggest another time" does NOT mint a slot proposal.** It writes a note on the RSVP plus a
   notification to the event's owner. Proposal creation is Scribe+ by ruling
   (`sessions/routes.go`); a Player clicking a link in an email must not escalate into that
   capability.

**Policy — RSVP data never leaves via export.** Who is coming, who declined, and the free-text
suggestion notes stay out of the campaign export and the AI export. Pinned structurally by
`internal/plugins/calendar/rsvp_egress_test.go`, which walks `campaigns.CampaignExport`,
`calendar.ChronicleExport`, and `aiexport.AllCategories()`. Session attendance — a different,
pre-existing concept — keeps its existing export and is allowlisted by exact path.

**Consequences.** The drawer's disabled toggle becomes a live Scribe+ control whose ON state is
the invite moment (email + bell fan-out to members who can SEE the event). Players answer from
the quick-edit card, which every role receives and which month chips, ledger rows, and mobile
agenda cards all open — the drawer is Scribe+ and could never have been the player surface.
"Out this week" writes only the acting user's availability and skips days that already carry a
hand-authored exception, mirroring the scheduler's own client-side rule. Because availability
exceptions are real-world dates while a calendar event may be in fantasy reckoning, the blocked
week is the event's own week only when the calendar tracks real time; otherwise it falls back to
the week of redemption, and the resolved week is always named back to the member so the action
can never silently block the wrong one.

**References.** `internal/plugins/calendar/migrations/013_event_rsvps.up.sql`,
`internal/plugins/calendar/{rsvp_model,rsvp_repository,rsvp_service,rsvp_handler,rsvp_email}.go`,
`internal/plugins/calendar/routes.go` (`RegisterRSVPRoutes`), `internal/app/routes.go`
(`calendarRSVPNotifierAdapter`, `calendarAvailabilityAdapter`),
`internal/plugins/sessions/notifications_service.go` (`NotifyUsers`),
`internal/plugins/calendar/{rsvp_test,rsvp_email_test,rsvp_egress_test,event_scan_contract_test}.go`,
ADR-042 (cross-plugin injection), ADR-045 (migration safety), cordinator dispatch C-CAL-RSVP-P1.

### ADR-046 amendment (2026-07-26) — "suggest another time" becomes structured availability

**Status.** Accepted, amends ADR-046 above (C-CAL-RSVP-P2). Operator ask: "there needs to be a way
for players to give temporary availability."

**Context.** ADR-046 shipped an asymmetry. "Out this week" wrote structured *un*availability the
scheduler could aggregate; "suggest another time" wrote a free-text note the Director had to read
and re-key. Players could express when they COULDN'T play in a schedulable way, and when they COULD
only in prose. Meanwhile the storage for the answer already existed —
`availability_exceptions.state` has always accepted `available`/`preferred` per date, and
`effectiveBlocks` already projects exceptions over the recurring pattern into the DM overlay — so
what was missing was a safe WRITE path, not a schema.

**Decision.**

1. **Structured windows alongside the note.** The suggestion surfaces (in-app quick-edit card and
   the emailed token page) accept date + from + to rows plus an optional note; the server requires
   at least one of the two, so a member with a vague answer is never blocked. Windows become
   `available` exceptions and land in the overlay + computed best-window; the note carries nuance.
2. **The composition rule lives in SESSIONS, not calendar.** Exception rows REPLACE a date, so
   writing only the offered window would erase the member's usual hours for that day — the exact
   opposite of what they just said. `sessions.AddMyAvailableWindows` / `composeOfferedDay` composes
   the day, paints the offer on a minute canvas, and writes the merged set. It belongs there because
   replace-semantics is the scheduler's own invariant, and it is enforced SERVER-side because this
   write can arrive from an email link with no editor in front of it. `preferred` is never
   downgraded by a generic offer; `unavailable` is overwritten.
3. **`ApplyToken` no longer applies the richer actions.** It consumes the token and writes only the
   status the action implies (`suggest` carries none and returns early). Both richer actions need
   either the POST body or the scheduler, neither of which the service has an edge to, so the
   handler owns them — via one shared `applySuggestion` used by the in-app and emailed paths so the
   two cannot drift.
4. **Availability writing is best-effort.** The RSVP note and the owner notification are the promise
   this flow makes; a scheduler that is absent or erroring still records the answer and tells the
   member their times were not saved.

**Consequences.** A player who can't make the proposed slot now answers once and the Director sees
real, aggregatable availability instead of prose. No new tables and no new routes — the existing
`POST …/events/:eid/rsvp` body gained an optional `windows` array and the emailed suggest page gained
form rows. One existing test invariant was narrowed:
`TestEventQuickEditV2_PlayersReadOnly` asserted the player card contained no `<input>`/`<textarea>`
at all, which was a proxy for "a player cannot edit the event" and stopped being meaningful once
players legitimately type their OWN data into that card; it now pins the event-editing fields by
name (strictly narrower), with a companion test asserting the RSVP affordances render for
non-Scribes.

**References.** `internal/plugins/sessions/availability_service.go`
(`AddMyAvailableWindows`, `composeOfferedDay`, `paint`, `runsToBlocks`),
`internal/plugins/sessions/availability_offer_test.go`,
`internal/plugins/calendar/rsvp_handler.go` (`applySuggestion`, `parseOfferedWindows`),
`internal/plugins/calendar/rsvp_email.go` (`rsvpSuggestPage`),
`internal/app/routes.go` (`calendarAvailabilityAdapter.OfferAvailableWindows`), ADR-046.

---

## ADR-048: calendar-v4 — the Block is the calendar, and its honesty states are load-bearing

**Date:** 2026-07-28 · **Status:** Accepted · **Supersedes:** nothing; the
calendar-v2 depth ruling (cordinator `decisions/2026-07-23`) still governs V2.

**Number check:** `.ai/decisions.md` carried no calendar-v4 ADR at all when this
was written (zero hits for `calendar-v4|calv4`; the last numbers taken were 046
and 047). CLAUDE.md rule 6 has required one since the arc began; it was booked
at the C-CALV4-SEAM-P5 preflight and written by W-B, the slice that COMPLETES
the four-zone component and therefore the first slice able to describe a
finished thing.

### Context

The operator's directive was one sentence: *"just go with a full redo of the
calendar"* (2026-07-26). What came back from the judging round was not a new
skin on the V2 month grid but a different unit of composition — a **Block**: one
component, four zones (Nameplate / Instrument / docked Ledger / Shelf), dropped
anywhere a widget can mount, and a **Bench** that composes several of them.

The contract is `cordinator/mockups/calendar-v4.html`, signed 2026-07-25 (r43)
and IMMUTABLE. It is state-addressable by query param, and the `mockups/renders/
v4-*.png` stills gate fidelity. Eight canon amendments (A1–A8) and four guards
(B1–B4) were signed on top of it (`decisions/2026-07-26-calendar-v4-canon-
amendments.md`), and the pinned render contract `internal/widgets/calendar_block
/data.go` has been amended three times under numbered decisions: **r51**
(`2026-07-27-calv4-tie-mark-emission.md`), **r52**
(`2026-07-28-calv4-ledger-p6-pin-amendment.md`) and **r53**
(`2026-07-28-calv4-shelf-pin-amendment.md`).

### Decision

**1. The Block is a WIDGET, and it is plugin-agnostic by construction.**
`internal/widgets/calendar_block` imports nothing from `internal/plugins/**`,
exactly as `calendar_v2` does. Everything it draws arrives in one
fully-resolved `BlockData`; it performs no queries and reads no request state,
because bound blocks render with `context.Background()`.

That constraint is not tidiness — it is what forces every honesty question to be
answered at the producer, where the data is, instead of guessed at the renderer,
where it is not. Three fields exist only because of it: `Mark.Time` (the widget
has no clock, no calendar hour/minute geometry and no zone rules),
`Mark.AxisLabel` (a hue token cannot be mapped to a type name without campaign
data), and `MonthGeometry.MoonsDeclared` (the per-cell discs are already capped).

**2. Size class follows HOST width; density follows MEASURED COLUMN width; both
are CSS container queries.** The signed mockup computes them in JS. On this
platform that is broken by construction: `boot.js:163` sets
`htmx.config.allowScriptTags = false`, so a `<script>` inside an HTMX-swapped
fragment never runs and a JS-sized Block would silently render at the wrong
density after any swap. Separating the two is what lets a ten-column Harptos and
a seven-column Gregorian share one component and still be honest about what each
can afford at the same host width.

**3. There is NO JavaScript in the package, and every control is therefore CSS.**
The tie toggle is a hidden radio pair plus `:has()`. Day answering is a hidden
radio group plus a generated per-ordinal rule ladder. Both are pure functions of
the data, so two Blocks on one page cannot fight over one piece of state while
the same Block survives an HTMX binding swap with the viewer's choice intact.

**4. There is NO drawer. The Ledger IS the drawer, permanently docked.** Canon
A3 struck D9's 480px slide-in drawer clause outright; canon A4 rewrote D6's
resolution sentence to *"the panel is already there; choosing a day changes what
it says."* This is the decision the whole direction was judged on, and it has a
measurable consequence: **the Block's declared height must be identical selected
and unselected**, which is verified in a real engine rather than argued.

**5. Motion is a BUDGET of four items, not a permission.** Canon A5 mints
ANSWERED — *"it changes background-colour and ink hue only; it never gains or
loses shadow, never changes border thickness, never moves"* — and that is what
makes a docked panel that repaints legal under L1: the answer arrives as a
COLOUR change. The budget is (i) background-colour and ink hue on answering
surfaces, (ii) one `scaleX` on a Ledger row's rail and never on the gold GM
rail, (iii) the `m-latch` ring closing centre→corners on the viewer's own
explicit act, (iv) M5 non-target silence expressed as a selector, so adding
Blocks adds RESTING cost only. All inside `prefers-reduced-motion:
no-preference`, which is what proves ANSWERED is a state and not an animation.
`TestCSS_NoMotionAtAll` is an ALLOWLIST enforcing exactly this; the month grid
never moves (`decisions/2026-07-27-motion-policy.md`).

**6. HONESTY STATES ARE FIRST-CLASS, and they are the reason this arc is
different from its predecessors.** Four distinct idioms, deliberately not
interchangeable:

| Idiom | Means | Example |
|---|---|---|
| the FAULT | this thing cannot resolve, and it says so **where its value would go** | a calendar with no months prints the fault instead of the date, and emits no date element at all — not a zero, not an em dash |
| the `needs backend` chip | this SURFACE is designed and its store does not exist | the Shelf's Filters TAB, the Bench's horizon tile. It is **never rendered to a player** (`decisions/2026-07-27-needs-backend-audience.md`) |
| ABSENCE | this viewer is not entitled to it | a player receives no dm_only mark: no placeholder, no ghost, no "+1", and no hidden-count chip — **not even a zero** |
| OMISSION | the data exists nowhere yet, so the segment DROPS | the Ledger meta line's owner segment, `· RSVP n/m`, the mini foot's `next:` |

The rule that makes them work is that none of them is ever satisfied by a
`title` (WG-spec V18) or by a fabricated number.

**7. EVERY COUNT COMES FROM ONE VIEWER-FILTERED PASS**, and that is enforced
rather than intended. `filterEventsByUser` compacts in place, so a second
filtering pass reads a corrupted slice — but the real reason is stronger: two
counts on one screen, one computed pre-filter and one post, are an ORACLE whose
difference is exactly the number of events the viewer may not know about. The
signed mockup's own `tiedCount` is that shape and is deliberately not ported.
Two tests hold it: `TestBlockCountsAreNotAnOracle` and, since W-B,
`block_count_oracle_test.go`, which asserts that every number on screen is
independently reproducible from that viewer's own filtered set — not that the
numbers are right.

The structural half matters more than the tests: **the Ledger is reassembled
from the cells the grid already draws**, never from a parallel row list, so the
head count, the foot count and the day-cell total are three READINGS of one
number rather than three computations that happen to agree.

**8. The eight-key LAYER REGISTRY governs every layer-owned surface, and DEF is
`["moons"]`.** "The default surface is a month with its moon phases and nothing
else" (L-M2/A8 via L20/L26/L29). A HOST that wants more passes a set — which is
how the entity page and the Bench dock the Ledger — and that is a host decision,
not a DEF change (`decisions/2026-07-28-calv4-def-and-zone-chips-ruling.md` §1).
A zone's `NeedsBackend` flag belongs to the wave that FILLS it: one wave, one
field, one owner (§2 of the same ruling).

**9. The cross-slice contract is a PINNED FILE amended by numbered decisions.**
`data.go` is byte-copied from `dispatches/chronicle/C-CALV4-BLOCKDATA.go.txt`,
never `gofmt -w`-ed (the repo is not plain-gofmt-clean; a glob format churned
~24 unrelated files in a past retro). Changing a field is a STOP-AND-FLAG, not a
judgement call, because it desynchronises a parallel chat nobody can see. New
fields land in their own blank-line-separated paragraphs so gofmt's alignment
groups cannot force a realign of the existing block.

### Consequences

- **A seam-test category exists now and did not before.** Phase A shipped a
  producer and a renderer that were each green in isolation and composed into a
  Block that was visibly wrong five ways. `block_seam_test.go` pushes real
  producer output through the real renderer and reads the HTML; assertions about
  what the producer CHOSE live there, never in the widget package, where the
  test author writes the input and every such assertion is vacuous.
- **Three counters disagree about days** (leap-aware `Calendar.AbsoluteDay`,
  fixed-geometry `constLenDayIndex`, the legacy V1 path) and per-day moon discs
  mix the first two. Making `constLenDayIndex` intercalary-aware would shift the
  weekday column of every calendar in the operator's production database, so it
  is an operator-gated product decision, documented and pinned rather than
  "fixed".
- **What the model cannot express is booked, not faked:** composed
  tag+member audiences (W-G), a queryable knowledge horizon (W-F), per-calendar
  sync linkage, subordinate calendars, an owner/creator surface on `Event`, and
  era-relative year reckoning.
- **CSS-only controls have a ceiling.** Day answering needs one static rule set
  per day ordinal, so the bound is 40 + 8 keys, generated and diff-tested. Past
  it a day carries no control at all — the honest failure, not a dead one.

### Section: the Almanac, and why the grid's moon ceiling is legitimate (W-E, C-CALV4-SHELF-P7, 2026-07-28)

**Added as a SECTION and not as ADR-049, per the paragraph below.** W-F's layer
switchboard and preference store join it here when they land.

**10. THE GRID'S THREE-MOON CEILING IS ONLY LEGITIMATE BECAUSE THE ALMANAC
EXISTS.** L21 caps the month grid at `moonCap = 3` so *"the grid can never grow
with the fiction"* — and the second half of the same law is *"the Almanac
carries every declared body at full width"*. Wave 1 shipped the ceiling and the
nameplate badge that announces it (`MonthGeometry.MoonsDeclared`, r51) and
nothing shipped the place the overflow goes, so a builder who configured a
fourth moon had it silently drop out of the product. `BlockData` could not
express it either: `DayCell.Moons` is capped, so the fourth body had no
illumination data anywhere in the render contract at all.

r53 (`decisions/2026-07-28-calv4-shelf-pin-amendment.md`) adds
`MonthGeometry.Almanac []AlmanacMoon` — a per-month celestial register, produced
once, in the calendar's own declaration order — and it is **deliberately
UNCAPPED by the request's `MoonCap`**. That is the ONE sanctioned place a
host-passed parameter is non-authoritative for a zone, and it is signed once
here rather than discovered later: `MoonCap` governs the grid, and it does not
govern the surface the grid's ceiling points at. `AlmanacMoon.Drawn` is the only
place the renderer learns which bodies the ceiling excluded, so `moonCap`'s
arithmetic lives in exactly one place.

The consequence worth stating: **the nameplate badge's hover tail is now true
and is therefore printed** — *"N moons declared; the grid draws 3 — all of them
are in the Almanac"* — but only in renders where an Almanac is actually
reachable. A Block with the Shelf hidden (`noShelf`, the Bench's real-world
Block) or with the shelf layer off gets the sentence WITHOUT its tail. A title
that names an absent surface is the same lie class as a green sync pill with no
denominator, and SEAM-P5 withheld the tail for exactly that reason for one whole
wave.

**11. THE CELESTIAL SURFACE IS A READOUT, AND IT IS NOT A FIFTH AUTHORING
SURFACE.** Contract §8 item 2 (L5) is explicit: celestial events are an event
type, authored in the normal event editor, *"surfaced through a dedicated
filtered Almanac list in the shelf widget. No fifth authoring surface."* The
Shelf reads. It authors nothing and it configures nothing — the master plan's
*"owner/scribe-configurable"* clause is STRUCK for wave 2 and re-booked to
whichever wave ships a config store, because no signed artefact contains a Shelf
configuration surface and building one would be the surface L5 forbids by name.

The vetoed composite-brightness ribbon was RELOCATED into the Almanac's month
lane rather than resurrected: a magnitude as an explicit readout with its
percentage in reach, never a glanceable claim (L19/L24). Timepiece, skybox and
skypane stay PARKED (L12/D4) — the Shelf must not grow a pictorial sky.

**12. THE ARITHMETIC IS PRINTED SO IT CAN BE AUDITED, so none of it may be a
constant.** The Almanac's own signed footnote says *"no date in the register was
typed by hand"*, and the mockup hardcodes a thirty-day month in four places — in
a product whose whole thesis is ten-day weeks and arbitrary month lengths. All
four are fixed rather than ported: the footnote's column count, the drift
figure and the "N moons declared · none keeps the month" line are derived from
the month's REAL day count, and the fourth was fixture content and is not
printed at all. `TestCSS_NoLiteralWeekLength` is extended to the lane, which
takes its day columns from `grid-auto-flow` so that no literal and no variable
`repeat()` count for a month length exists anywhere in the sheet.

**13. THE CSS-ONLY CONTROL BUDGET IS NOW THREE RADIO GROUPS, AND IT HAS A SHAPE
IT CANNOT EXCEED.** The tie toggle, the day pick, the Shelf's three tabs and the
Almanac's three sub-tabs are all hidden radios plus `:has()`, all named by pure
functions of the data. The ceiling this ran into is worth recording: **a
server-rendered `checked` attribute cannot vary by container query**, so one
radio group cannot carry two tier-dependent defaults. That is why the signed std
construction — four tabs in the Ledger head, the Ledger's rows as the "Month"
panel — is not buildable while selection is CSS-only, and why the Shelf keeps
its own strip at every tier. It is a real bound on the no-JS discipline and the
next surface that wants a per-tier default will hit it too.

**14. A SHARED PRIMITIVE MAKES ONE RULE REACH TWO ZONES, TWICE NOW.** W-E's
Upcoming panel reuses the Ledger's row component VERBATIM — deliberately, so the
two zones cannot drift and the count oracle can be an equality — and the moment
it did, two of W-B's mechanisms reached into it: the day-pick ladder's row
FILTER (which made choosing a day reflow the Block by 86px) and, in W-B's own
fix round, its reveal rule. Both were unscoped selectors whose doc comments
already claimed the scope the code did not have. The rule that falls out:
**when a primitive is shared across zones, every rule that names it must name
its zone as well**, and the guard must check the SELECTOR and not only the
property. `.lrows .lrow`, `.lctx[…]`, `.lzero.lday[…]` — all three are pinned.

**15. A PREFLIGHT ERROR REACHED SIGNED HONESTY COPY, AND THE RETRACTION COMES
BEFORE THE FILL.** (C-CALV4-RSVP-P8 §2, wave 3 / W-G Part A.) Wave 1 shipped the
Bench RSVP panel as a header plus a `needs backend` chip, justified in code by
this, verbatim (`bench.go`, `type BenchRsvp`): *"there is no session entity, no
RSVP table and no per-member time zone."* **All three claims were false, and two
of them were false on the branch that wrote them:**

| Claimed absent | Actually shipped |
|---|---|
| session entity | `internal/plugins/sessions`, with `scheduled_date` + `scheduled_time` (`sessions/migrations/004`) |
| RSVP table | `calendar_event_rsvps` + `calendar_event_rsvp_tokens` + `calendar_events.collect_rsvps` (`calendar/migrations/013`) — and `calendar_v2.templ` already rendered against them on the same branch |
| per-member time zone | `users.timezone` since `db/migrations/000001_baseline`, with a live edit surface at `PUT /account/timezone` |
| (also) availability storage | `member_availability` + `availability_exceptions` (`sessions/migrations/002`), minute-accurate, DST-correct, additive-offer invariant enforced server-side |

The failure class is the one this whole discipline exists to prevent, inverted.
Every honesty state in calendar-v4 is a claim that Chronicle **cannot** compute
something; §7's chip audience rule, §13's ledger and the `.badge.need`
non-dilution rule all assume that claim is checked. **A `needs backend` chip
that is wrong is a fabricated ABSENCE — strictly worse than no chip**, because
it is read as a verified finding: it told the operator that his most-wanted
feature had no foundation when it had almost all of one, and it is the kind of
error that gets a shipped store re-built rather than used.

So the four sites were retired **as a correction, in the slice's first stage,
before any of the filling work** — `BenchRsvp`'s doc block, `benchRsvpPanel`'s
note, `benchSessionTile`'s qualifier and every inert control's title, and the
Bench page caption's design-ahead inventory. The rule that falls out and binds
every later wave: **a `needs backend` chip is a preflight FINDING, and a finding
names the migration or the route it checked.** Copy that asserts an absence
without naming what was looked at is not an honesty state, it is a guess with a
badge on it.

Three things in the panel genuinely were unbacked and kept their state, all
GM-tier so a player received none of them: the **propose** write
(`routes_snapshot.txt` carries no propose-from-window path), the
**reminder/nudge** endpoint (the fan-out fired only on the `collect_rsvps`
OFF→ON transition; booked as C-CALV4-RSVP-P8B, "the asking email"), and a
server-side **recommender** — which WG-3 retires by *deriving* the window
arithmetically from the overlay's own per-hour free counts rather than storing
it, under a permanent `derived · not stored` chip.

**AMENDED 2026-07-29, and the sentence above is left standing rather than
edited, because §15 is the RETRACTION section** — quietly rewriting it to hide
that one of the three has since been built would be the same class of error it
was written about. **The count is now TWO.** C-CALV4-RSVP-P8B shipped the
reminder endpoint (`POST /campaigns/:id/calendar/ask`), so the Nudge chip came
off and the control went live in one place; §25 below records what was built and
why. Propose and the recommender's status are unchanged.

**16. RSVP ANSWERS ARE PARTY-VISIBLE; AVAILABILITY LANES ARE NOT.** (WG-4 /
C-CALV4-RSVP-P8 §4.) Two artefacts openly disagreed — the unsigned W-G spec
gives a player only their own roster row; `mockups/v4-proposed/roles-and-rsvp.html`
makes every answer party-visible — and the SIGNED contract settles it, because
`rsvpPanel()` renders the lanes under `${GM ? lanes : ''}` but renders the full
`.mtable` of every member unconditionally, and `v4-bench-player-light.png` shows
a player receiving every name, role, zone chip, local clock and answer word. The
law, in one sentence:

> **RSVP answers, roles, zones and per-member local clocks are party-visible.
> Per-member availability LANES are owner / co-DM only. The aggregate density
> row is everyone's.**

Consequences that are structural rather than stylistic: detail is gated **in the
handler by role, never by route** (the shipped shape, `sessions/routes.go:29-46`),
so a player's payload does not contain another member's lane data at all —
absence is in the payload, not in the template; the anonymous aggregate is
post-filtered **server-side from the set the viewer is entitled to**, never
recomputed from the rows in the viewer's own DOM (which would flatten every
player's density to `1 of 1`); and the count lane's denominator is
`TotalMembers`, the Director included, so it is never labelled "of 5 players".

**17. THE ROLE VOCABULARY WAS TWO-VALUED ON A PERMISSION SURFACE, AND
`.badge.gm` GAINS A THIRD SIGNED STRING.** (WG-4.) `availability_overlay.go`'s
`roleLabel(isOwner)` derived from `Role >= RoleOwner` and ignored
`IsDmGranted` — so a **co-DM rendered as "player" while receiving full
owner-tier detail**, on the one surface whose entire subject is who-may-see-what.
It is retired: `campaigns.Role.DisplayName()` (Owner / Scribe / Player) is the
single lookup, printed through one shim so the later "role names come from the
installed game system" slice
(`decisions/2026-07-27-calendar-scope-and-roles.md` §4) stays a one-file change.

P6 ruled that `.badge.gm` carries exactly two strings, `"<N> hidden"` and
`"GM"`. **It gains a third, `co-DM`, ruled once here.** The two-string rule
exists to stop the GM badge accreting invented content in the Block's *grid and
Ledger*, where it marks event VISIBILITY; a roster's role column is a different
surface with a genuine third case. The alternatives are worse: a new badge class
is a new primitive in a slice meant to add none, and plain `.ro` text loses the
weight on the one surface where a mislabelled co-DM is a permission bug.

**18. ZONE ABBREVIATIONS STAY; THE HONEST RESIDUAL IS SMALLER AND DIFFERENT.**
(C-CALV4-RSVP-P8 §5.) The W-G spec forbids `CDT`/`EDT`, mandates the last IANA
path segment, and books a *"friendly zone names need a backend helper"* chip.
Overruled three ways: the signed render prints abbreviations directly; W-B
already ships them per-instant and DST-correct through the stdlib
(`Format("MST")` folded into `Mark.Time`, **pinned** in the BlockData pin, so
un-doing it is a pin amendment this slice does not have); and `EDT` is *more*
honest than `New_York`, because a location is not a clock. The spec's reasoning
was drawn from `internal/timeutil/zones.go` alone — true of `timeutil`,
irrelevant to the stdlib call that actually produces the string.

The real residual is narrower and is printed as a **caption, not a chip**:
`Format("MST")` degrades to a numeric offset (`+0545`) for zones with no
alphabetic abbreviation. The full IANA identifier rides in `title` on every
abbreviation, which is the Nameplate's existing shape one notch denser — **one
fact at two densities, not three conventions**.

And the state that has no clock at all: `users.timezone` is NULLABLE and both
viewer-zone resolvers silently fall back to `"UTC"`, so a rendered clock for a
zone-less member is **a guess presented as a fact**. The signed pair is
`.badge.warn "zone not set"` + `.btn.xs.ghost "Ask →"` and a **literally empty**
`.lt` — never `--:--`, never a dash, never a UTC guess. It is
`BlockData.Fault`'s fault-where-the-date-would-go idiom applied to a clock, and
the pair survives at every width, because the repair may not be the thing that
disappears on the screen a player is most likely holding.

**19. NO STORED RSVP AGGREGATE REACHES THE PANEL.** (C-CALV4-RSVP-P8 §6, the
slice's gate.) `EventRSVPSummary.Counts` is raw rows, while `decorateResponders`
drops ex-members from the named list — so a stored aggregate printed beside a
membership-filtered name list is a **counts-vs-names disagreement by
construction**, and the disagreement grows every time somebody leaves a
campaign. Killing it structurally is cheaper than asserting it: every number on
the panel is recomputed from the visible, membership-filtered rows, and the
`.caption` says why. The gate is a count-oracle test in the P6 shape — the
assertion is not "the numbers are right" but *"every number a viewer receives is
independently reproducible from that viewer's own visible set"* — over a fixture
whose load-bearing case is **one departed member holding a stored
`calendar_event_rsvps` row**, because that is the one case a screenshot cannot
reconstruct.

**20. A TOGGLE APPLIES BY WRITING A ROW AND RELOADING THE PAGE — AND THAT IS
THE SHAPE THAT KEEPS THE SECURITY REVIEW SMALL.** (C-CALV4-LAYERS-P9, wave 3 /
W-F, [LYR-1] SIGNED.)

Four facts fenced the mechanism before any of it was written, and all four are
properties of this package rather than preferences: there is **no JS and there
cannot be** (`boot.js` sets `allowScriptTags = false`, so a `<script>` inside an
HTMX-swapped fragment never runs, and `allowEval = false` kills `hx-on` too);
the Block renders under **`context.Background()`** and performs no queries, so a
preference can only be resolved by the PRODUCER and handed in on `BlockData`;
**CSS-only reveal was unavailable**; and a **server-rendered `checked` cannot
vary by container query** (§13).

The third deserves its own sentence, because it is the one a later hand will
want to undo. Rendering all eight layer surfaces and hiding seven with CSS fails
twice over, and either failure alone is fatal.
`TestSeam_EnabledLayerSetMatchesWhatRenders` asserts that an ABSENT layer key
produces an ABSENT surface — its header calls the alternative *"the
set-and-ignored defect class this file exists to kill"* — and a CSS reveal makes
that test unfalsifiable. And `needs backend` never renders to a player, so
emitting the horizon zone always and hiding it would put GM-facing honesty copy
into a player's markup. **Permission is ABSENCE, and `display: none` is not
absence.**

So the row is an `hx-post` that persists and answers **204 No Content with
`HX-Refresh: true`**, and the hosting page re-renders through its own handler.
No HTML crosses the route in either direction. The consequence that matters:
**a Block's host context is never client-supplied.** Its render depends on the
entity id, the bound calendar id, `MoonCap`, `LedgerHidden`/`ShelfHidden` and
the widget-binding resolution layer; under `HX-Refresh` the host page rebuilds
every Block through the handler that already owns those decisions, including
`requireVisibleCalendar` and the W5a `ListVisibleCalendars` split. A fragment
response would mean re-authorising every one of those fields inside a new
handler — a whole resolution layer reproduced, which is exactly where a leak
hides. The route therefore accepts a layer key list and nothing else: no
`calendar_id`, no `entity_id`, no host descriptor, no `user_id` (it comes from
the session), and there is no IDOR surface because the primary key it writes is
`(session user, authorised campaign)`.

**THE A8 READING THIS SHIPPED, stated so the next hand finds it argued rather
than assumed:** L-M2's *"a layer that changes the MONTH's geometry applies
instantly and silently"* is about **ANIMATION, not latency**. A server re-render
animates nothing at all — the month does not slide, grow, stagger or fade; it is
simply different. The honest cost is that the popover closes on each toggle, so
a viewer setting three layers re-opens the sheet three times. The switchboard is
a set-once control and that is survivable, but it is the downside and it is not
hidden.

And the branch Chronicle ships that the mockup does not: **no section animates
open, for anyone.** L-M1 permits it and the signed contract does it
(`.layer.opening` → `@keyframes m-open`), but the motion budget is four
properties and one keyframe inside a reduced-motion guard, and `height`,
`interpolate-size` and that keyframe are all outside it. The design notes
already wrote the branch — *"Under `prefers-reduced-motion` the section simply
exists"* — and Chronicle ships it for everyone rather than widening a budget
signed nine days earlier by the slice that rewrote its test. The only motion the
switchboard adds is the row's switch, and it lives inside the guard, which is
itself the proof that the switch is a STATE and not an animation.

**21. THE STORE IS ONE COLUMN ON A ROW THAT ALREADY EXISTED, AND `NULL` IS NOT
THE EMPTY SET.** (C-CALV4-LAYERS-P9, [LYR-3] SIGNED.)

Three waves ended by writing the same sentence into the same files: *"no
preference store of any kind exists in the repo."* It was true —
`db/migrations` ended at `000030`, the calendar plugin at `013`, and
`widgetbindings.WidgetBinding` had no config column and no user dimension.

Calendar migration `014` adds `block_layers VARCHAR(255) NULL` to
`calendar_active` rather than minting a table, and that is not a convenience:
migration `007` made the identical call under a live coordinator decision (PR
#368 stop-and-flag #3) — *"simpler; reuses the existing (user_id, campaign_id)
PK + FK cascade + index discipline"* — and an executor may not quietly reverse a
signed decision. The grain is therefore per-VIEWER per-CAMPAIGN: L20 describes a
viewer's preference for how they read calendars, and a viewer who turns eras off
almost certainly means "off, everywhere".

**The host's layer set became a SEED rather than a verdict.** With no stored row
the Block renders the host's set exactly as before — which is what kept every
wave-1/2 screenshot valid on day one, because a fresh viewer has no row. The
first switchboard write persists the viewer's set, and from then on the store
wins at every host in that campaign.

**`NULL` means "never chosen" and `''` means "chose nothing", and the difference
is load-bearing at every layer** — column, repository (`sql.NullString` → `nil`
vs an empty non-nil slice), service, producer and renderer. Collapse them and
the bare month becomes unreachable, which turns the default into a floor. *"Bare
mode is not a degraded view; for reading a month it is the better one"* is only
defensible if the viewer can leave it and have the leaving stick — so shipping
the mechanism to leave DEF is the opposite of changing DEF, and DEF is still
`["moons"]`.

`VARCHAR` and not JSON: the repo has no JSON-preference precedent, a
comma-joined key list stays greppable in a production incident, and the
validation belongs in Go beside the eight-key registry that defines the
vocabulary. It **rejects an unknown key with 400 on the way in** — a silently
dropped key half-applies a choice the viewer cannot audit — and **filters an
unknown key with a log line on the way out**, because a registry that shrank
after a write is a deploy rather than a caller and must not brick a calendar.

**22. THE SWITCHBOARD IS THE FIRST SURFACE WHERE ONE CONTROL PRODUCES BOTH
KINDS OF ABSENCE, AND THE TWO ARE DIFFERENT RULES.** (C-CALV4-LAYERS-P9 §8.4 /
§10.)

*Permission-is-absence* hides data that **exists** from a viewer not entitled to
it. The *needs-backend* omission hides data that exists **for nobody**. They are
identical in the DOM and they are governed by different rules, and until wave 3
nothing let a viewer produce both from one place.

`belowMonthLayers` rendered its `needs backend` chip unconditionally for three
waves, and that was safe **only because no player could enable the keys**: DEF
is moons-only and neither host seed carries `legend`, `horizon` or `moongraph`.
The switchboard makes every key reachable by everybody, so the passive rule
became an active gate — and the gate landed one commit BEFORE the reachability
rather than in the same breath as it, because a gate that lags its exposure by
one commit is a gate that was not there. **A player who toggles `horizon` gets
nothing: no zone, no greyed panel, no explanatory note.**

**The ROW stays in the sheet, though, and that is the other half of the
ruling** ([LYR-6a]). The denominator never varies by role — eight rows and "of
8" for owner, co-DM and player — because a layer is a per-viewer DISPLAY
PREFERENCE and not a promise of content, exactly as a calendar with no eras
gives an empty `eras` row. Filtering the row set by what a viewer would actually
get reintroduces a permission dimension inside a display-preference registry,
which the design review killed by name as *"a struct field invented to make a
narrative work"*.

**23. A PER-VIEWER STORE DOES NOT BEAT THE CONTAINER-QUERY BOUND, AND THAT IS A
FINDING.** (C-CALV4-LAYERS-P9 §11, [LYR-8] SIGNED.)

§13 records that *a server-rendered `checked` attribute cannot vary by container
query*. W-F was told it might re-open the `.ltabs` question **only with a
mechanism that beats that bound**, and the per-viewer store was the candidate,
on the theory that it moves the default out of the container query.

**It does not.** A store changes WHERE the single value comes from. It is still
one value, rendered once, into one `checked` attribute, for a Block that may be
at std on one host and full on another **in the same page render**. Nothing
about a store makes one attribute two. A second, independent reason landed after
the ADR: Chronicle's Block renders the Shelf zone at std as well, unlike the
signed std construction where `block()` emits no `.shelf` element at all — so
the three Shelf tabs already have exactly one home per tier, and a second copy
inside `.ltabs` would be two controls for one piece of state. The open item is
STRUCK rather than re-booked, because carrying it means three slices in a row
re-deriving the same answer.

**24. THE SCREENSHOT GATE FIRED TWICE, ON TWO THINGS NO ASSERTION COULD SEE.**
(C-CALV4-LAYERS-P9 §15 / [LYR-2].)

The sheet's placement and the below-month collision were both gated on renders
rather than on reasoning, and both gates caught a real defect on the first pass.

The popover was anchored to the ⋯ invoker, which is what the ruling's sketch
reads like. The ⋯ sits at the far right of the Nameplate, **directly above the
docked Ledger column**, so opening down-and-left from it covered the Ledger
entirely — its own "Event list" row included, which is a control hiding its own
target. The ruling's binding clause is *"over the month, never over the Ledger
column"*, so the anchor moved to the INSTRUMENT: it satisfies the clause by
construction at every tier instead of by an offset nudged until it looked right.

And the collision HOST-P3 predicted happened. That measurement was taken against
stubs, then against a filled Shelf, and never with a real legend and a real
three-lane illumination graph under the month — which is exactly what a
switchboard can now produce. At 420px and 358px the instrument pushed past the
Block's 520px ceiling and the docked Ledger's head drew over the Shelf's strip.

CTS-8's precedent supplied the fix and the bound on it: the answer is taken
**inside the Block's own std geometry**, the zone that can yield yields, and
**no layer key is dropped — the viewer asked for the key**. The body scrolls.
Three alternatives were built and measured before that one was kept, and each is
recorded in the stylesheet with its reason: shrinking the month was refused
because it inverts the Block's priorities AND changes every std Block nowhere
near the ceiling (a shrinkable instrument hands the Ledger space the month had);
clipping the Ledger was refused because a zone that silently vanishes is worse
than one that scrolls; and bounding the stack with a literal was refused because
at 420px any honest bound is near zero, so the sections the viewer just switched
on would render as a scrollbar. Scrolling is already this Block's idiom for a
zone that cannot fit — `.lrows` and the Shelf's panels both use it — so this is
CTS-8 applied one level up.

**25. THE ASKING EMAIL: FOUR DECISIONS, AND ONE OF THEM CORRECTS THE BOOKING.**
(C-CALV4-RSVP-P8B, wave 3 TAIL; rulings [PB-1] [PB-2] [PB-4] [PB-8].)

The operator asked, in his own words, for *"an email sent to them asking for
what their schedule is, having them set like their 'normal' hours … or they
could just type everything in."* **The second half was already built and nobody
had told him**: the shipped availability page is a painted 7×24 weekly grid
whose header says *"Paint the hours you can play — it repeats every week"*, with
a per-day exceptions editor beneath it. The missing piece was never an entry
surface. It was the sentence that asks.

**(a) The booking was two emails wearing one sentence, and it ships as one
([PB-1]).** `.ai/todo.md`'s line — *"one endpoint, one template, a rate limit
and a tokened link"* — silently merged the **schedule ask** (audience: every
member; landing: the grid; needs no session) with the **RSVP nudge** (audience:
members who have not answered a specific event; landing: the token pages; needs
a collecting event). Shipping only the first would not flip ledger item 10;
shipping only the second would not satisfy the directive. It ships as ONE email
with two sections: the schedule ask is the subject and the primary CTA, and the
five EXISTING RSVP action links ride along when a collecting session is
resolvable **and this recipient may see it**, re-minted through the unchanged
`MintActionTokens`. That flips item 10 truthfully, because the re-send makes
exactly the same promises with the same expiry and the same single-use property
as the OFF→ON fan-out — nothing new had to be invented for the nudge half. It
also matches what a Director actually does: *"we're picking a date — tell me
when you're free, and say yes or no to the one on the books"* is one message,
and two emails four seconds apart is the fastest way to teach a table to filter
Chronicle into a folder.

**(b) "A TOKENED LINK TO THE GRID" IS NOT BUILDABLE, AND THIS IS THE SECTION A
FUTURE SLICE WILL NEED MOST ([PB-2]).** The booking was written before anyone
had opened `availability.templ`. That file is an 82-line SHELL — a breadcrumb, a
heading, a tab pair and two empty `<section>`s. The grid is drawn client-side by
`static/js/availability.js` (1,318 lines) over **six authenticated JSON routes**
(`GET|PUT /availability/mine`, `GET|POST|PUT|DELETE /availability/exceptions`,
`GET /availability/overlay`). So "a tokened link to the grid" is not one tokened
page: it is token-authenticating six member-data endpoints, or minting a session
from an emailed link. That is the largest authorisation-surface change anyone
has proposed in calendar-v4, and it is emphatically not *"one endpoint, one
template, a rate limit"*.

It therefore ships as **a plain deep link plus destination preservation**, and
the directive's *"existing one-time-token pattern"* is honoured **elsewhere in
the same email**: the RSVP action links ARE that pattern, they DO work from a
logged-out inbox, and they are unchanged. The part that cannot use it is the
grid, because the grid is an authenticated SPA and always was. A token bridge
was considered and refused for a second reason worth recording: such a token
**cannot be single-use** — a mail scanner's prefetch would burn it, which is the
entire reason the RSVP flow has a GET-confirm / POST-apply split — so it would
be a *different* security shape from the one the directive names, bought for
click telemetry alone. Session-less access to the six endpoints was refused
outright.

What makes the deep link land is not a credential but a repair. Three defects
in `internal/plugins/auth` made a deep link useless from a cold inbox, and the
third was a latent open redirect: `handleUnauthenticated` threw the destination
away; `LoginPage`/`LoginForm_` took no redirect parameter, so `Login`'s
query-string read could never fire (register's own comment states the mechanism
— *"the query param is not present on the HTMX form POST"*); and `Login`
validated with `strings.HasPrefix(redir, "/")` while `sanitizeRedirect`, which
rejects `//` and `/\`, sat 120 lines above it. **The third was unreachable ONLY
because of the second**, so the slice that repairs the plumbing is the slice
that opens the hole — and it was fixed first, in its own commit, behind a fence
of destination preservation and sanitisation only.

**(c) THE RATE LIMIT IS PERSISTED, AND THAT BUYS THE READOUT ([PB-4]).** This
endpoint mails other people's inboxes on demand; it is the only control in
calendar-v4 whose abuse cannot be retracted. Three layers: a per-campaign
cooldown (6h) and a per-recipient floor (24h), both PERSISTED in
`calendar/migrations/015_schedule_asks`, plus a per-USER in-memory limiter
(10/h) on the route. In-memory-only was refused for two reasons and the second
is the one that matters: a cooldown that resets on every deploy is not a
cooldown, **and it costs the operator the "last asked" readout that is the only
thing making the control honest**. The Bench prints *"Asked 2 hours ago. You can
ask again in 4 hours"* and disables the button with that reason visible, because
a limit whose only expression is an error page is a limit the operator hits
blind. The gate and the readout are the SAME predicate, so the control and the
endpoint cannot disagree.

Two shapes fall out of the two persisted limits, and they are deliberately
different: the campaign cooldown REFUSES the whole send; the per-recipient floor
SKIPS that member and lets the rest proceed, so a legitimate second ask after
somebody joins mails the new member and nobody else. And a standing invariant:
**nothing is recorded when nothing was sent** — SMTP unconfigured, no address on
file, or a send error writes no row, because a cooldown must never lock out a
campaign that received no mail.

The per-user limiter was LIFTED rather than imported, and the reason is worth
recording because the obvious homes are both closed. `bestiary.UserRateLimit` is
the same algorithm, but importing it is a plugin-to-plugin edge
`internal/wire/plugin_import_guard_test.go` forbids outright. `internal/middleware`,
beside `RateLimit`, is closed for a harder reason than taste: a per-user limiter
must read the session, `auth.GetUserID` lives in the auth plugin, and
`auth/handler.go` already imports `internal/middleware` — it would be an import
cycle. So it lives in the plugin that needs it.

**(d) DEPLOYMENT STATUS IS NOT BUILD STATUS ([PB-8]).** The unconfigured-mail
state renders `.badge.warn` reading `email not configured`, NOT `.badge.need`.
WG-8 ruled that `.badge.need`'s text is always the literal `needs backend` and
that the class is not diluted; an unconfigured mail server is not a build gap —
the endpoint exists, the template exists, the operator has not configured SMTP.
That is the same register as the `zone not set` repair, and the fix is an admin
action rather than a Chronicle release. Using `.badge.need` here would leave a
`needs backend` chip over a backend that had just been built, which is the
inversion WG-8 retired. **For AUDIENCE purposes, though, deployment status IS
build status**: all three of the control's states are GM-tier and a player
receives none of them, because a player has no control to explain and telling
them this instance has no mail server is telling them about Chronicle's
operations rather than about their game.

One sentence is shared as a single package-level constant across the Bench and
the endpoint — *"Email is not configured on this server — answers still work
in-app; nobody was emailed."* — so a future edit cannot make the two surfaces
disagree about what an unconfigured mail server means. It is spec ledger item
11's own wording, and Part B's drawing renders it too: one fact, one sentence,
every surface.

**What this slice made necessary and did NOT build.** The asking email is the
first Chronicle email a member can receive **repeatedly because somebody else
pressed a button** — every prior one was transactional and self-triggered
(password reset, invite accept, "you were invited to this event"). Chronicle has
no notification preferences, no email opt-out and no unsubscribe anywhere
(`grep notification_pref|email_opt|unsubscribe` over `internal/` and `db/`
returns nothing). That is defensible at a 6-hour campaign cooldown and it is not
defensible forever, so it is booked as **C-NOTIFY-PREFS**. No unsubscribe link
was invented in the meantime: a dead link would be worse than the email's honest
*"you received this because you are a member of this campaign."*

### Section: round 2 — a surface is allowed to be closed (R2-1, C-CALV4-BENCH-R2, 2026-07-30)

Three calendar-v4 waves optimised the Bench's **content** — the ribbon's honesty
chips, the panel's audience law, the index's viewer-filtered counts — and not
one of them asked how much of it a person should meet at once. The operator used
the product on a live client on 2026-07-29 and named the result in three
sentences: *"so stretched out, especially the RSVP menu"*, *"where are the
menus" / "5-6 blocks of data before you get to the calendar"*, and *"on the
entity it scrolls"*. The page was correct at every line and exhausting as a
whole. Round 2 adds no data, no zone and no engine; it decides what a viewer
meets first, how wide it is allowed to be, and what a surface says when it is
closed.

**The disclosure mechanism, and why it is `<details>`.** Four Bench sections
collapse — ribbon (whole, not per tile), rsvp, nextup, rows — as native
`<details>`/`<summary>`. It is the only primitive that is honest with JavaScript
off: the section opens and closes natively and only the *remembering* needs
HTMX, so a viewer with a broken script tag gets a working page that forgets.
`aria-expanded`, the disclosure role, keyboard operation and screen-reader
announcement come free and correct — a hand-rolled `<div role="button">` would
reintroduce the class of defect WG-5 caught once already. And it sidesteps the
checkbox trap: copying the Block's hidden-radio + `:has()` shape would re-meet
§13's bound for no gain. The flip rides on the `<details>` **itself**, because
`toggle` does not bubble and a click handler on `<summary>` fires *before* the
state changes, which would invert every write.

**And because it rides there, everything it declares is INHERITED — the cost of
that primitive, found in review and stated here so it is not re-found.** htmx
resolves `hx-vals`, `hx-swap` and their siblings by walking ancestors (`bn()`
recurses to `parentElement` in `static/vendor/htmx.min.js`), not by reading the
element. So a `<details>` wrapping a section is an attribute broadcast over
every HTMX control inside it: shipped as `hx-vals`, the flip's `section=<key>`
was appended to the player's RSVP trio, the owner's `Ask →` form and all five
sort links — eight requests that never asked for it. Harmless on the day (their
handlers ignore unknown fields) and a trap immediately after, because the field
it broadcasts is one the same route rejects when paired with `layers=`. **The
general rule: a disclosure may not carry request state that its contents do not
share.** State that belongs to the disclosure alone goes on the request URL,
which is an element's own attribute and is inherited by nothing; state that must
be inherited (`hx-swap="none"`, so an error page is never swapped into the
section) is declared once and every control inside re-declares its own, asserted
rather than trusted. This is the same shape as the audience law — make it
structural, not remembered.

**§13, met a second time, which is what makes it a pattern rather than an
incident.** §13 says a server-rendered attribute cannot vary by viewport. It was
written about `checked`; it is equally true of `open`. LAYERS-P9 §11 already
tested a per-viewer store against that bound and found the store does not beat
it — it changes *where the single value comes from*, not how many values one
attribute can be. R2-1 met the identical wall from the other side: the
operator's instruction was "desktop defaults open, mobile defaults closed", and
one render cannot be both. **Twice is a pattern worth naming.** The way out is
never a smarter store; it is either accepting one value, or moving the decision
into CSS where viewport lives. R2-1 did both — one closed default at every
width, plus `order` lifting the calendar above the ribbon at ≤640px — and the
CSS half is the one that literally answered the request. The state half was
accepted with an honest summary line on every closed section, which is what
makes closed-by-default liveable rather than merely defensible.

**`calendar_active` grows a third column, and a table is refused a third time.**
`bench_sections VARCHAR(255) NULL` (migration 016) joins `sidebar_pinned` (007)
and `block_layers` (014). 007 made that choice under a live coordinator decision
(PR #368 stop-and-flag #3) — "simpler; reuses the existing (user_id,
campaign_id) PK + FK cascade + index discipline" — and 014 followed it. A prefs
table at this point would be an executor quietly reversing a signed decision
twice over. The grain is (viewer, campaign), the same grain a layer preference
has, for the same reason: these describe how a *person* reads calendars.

**The column stores the CLOSED set, not the open one**, and that is the
non-obvious half. Because the ruled default is closed, storing the *open* set
would make `''` mean "all four closed" — byte-identical to the default, and
therefore unable to record "I opened nothing on purpose". Storing the closed set
keeps `NULL` and `''` genuinely distinct, which is the entire point 014's header
argues at length. `NULL` is the ruled default; `''` is all four open; a list is
the sections that are shut.

**Zero new routes.** The existing `POST /campaigns/:id/calendar/prefs` grows one
optional field, `section=<key>`, drawn from a four-key closed registry and
rejected 400 when unknown. A sibling route would have had an identical shape and
an identical security review and cost a `routes_snapshot.txt` regeneration plus
a wire-contract event for nothing. **The two branches answer differently and the
difference is load-bearing:** `layers=` keeps `HX-Refresh: true` because a Block
genuinely must re-render for a layer change; `section=` returns 204 with **no**
refresh, because the `<details>` has already changed state client-side and a
page refresh per chevron would fight the disclosure animation, re-run every
Block's render, and visibly undo what the viewer just did. The next hand will
read that asymmetry as an oversight, so the handler says so in a comment.

**The third member of the "why is there less here" family.** Two distinctions
were already named in this ADR's neighbourhood: *permission-is-absence* (a
player's ribbon has three tiles because they were never sent three others — the
GM tiles are not in their DOM at all) and *needs-backend-omission* (a surface
prints a chip instead of a zero because Chronicle cannot compute the fact).
R2-1 adds *compactness-is-a-choice*: a GM's ribbon may be closed because they
closed it. **All three render as "less", and in a screenshot they are
indistinguishable.** They are told apart by their mechanism, and the product
keeps them apart in three different places: permission by *absence from the
payload*, needs-backend by a *visible `.badge.need` chip*, compactness by a
*summary line that states what is inside*. A closed section that said nothing
would collapse the third into the first, which is why the summary line is a
ruling and not a styling note, and why `.badge.need` is explicitly barred from a
summary — four visually identical grey chips meaning three different things was
caught once already.

**The entity seed drops two keys, and the seed rule is why that was cheap.** The
entity embed seeded five layer keys; it now seeds three. The two removed —
`ledger` and `shelf` — are exactly the two that add a **zone**; the three kept
are all inside the month. Under [LYR-3] the host's set is a SEED, not an
override, so this is a producer decision with a designed mechanism behind it and
**no widget file was edited**. The honest cost is stated rather than minimised:
opting `ledger` back on persists, and because the store's grain is (viewer,
campaign) it also turns the Ledger on for the Bench. That is the signed grain,
not a bug. Depth returns properly through R2-3's Block theater and R2-1 ships no
substitute for it, because a stopgap becomes the thing R2-3 has to delete.

**A three-wave-old warning was measured out rather than inherited.** The entity
producer warned that dropping `ledger` would break the full-tier column
arithmetic. Measured: `ColWidth` / `IsNamed` / `IsNamedCSS` have zero non-test
callers anywhere under `internal/` (the density flip is a
`@container cal-cell (min-width: 84px)` query against real layout), and the
full-tier body track is `minmax(0, 1fr) auto`, so an absent Ledger collapses its
own track rather than leaving a hole. The warning's premise was false in every
clause. The one real consequence is the opposite of the fear: without the Ledger
beside it the month's cells are wider at the same host width, so named columns
flip on at a **narrower** host — 1198px → 898px for a ten-day week, a shift of
exactly the dock's own 300px. The entity month became richer.

### Section: round 2 — the day answers its click (R2-2a, C-CALV4-DAYCARD, 2026-07-31)

**The complaint was two sentences and both were mechanically true.** *"I'm
unable to click and do anything, it just selects the date and nothing
happens."* Clicking a day checks a visually-hidden radio and the generated
ANSWER ladder filters the docked Ledger to that day — the one sanctioned
content change in the Block, deliberately quiet because L-M2 forbids the month
from moving, so the answer had to land somewhere else and landed in a column
the operator was not looking at. And where a viewer has switched the `ledger`
layer off, `dayPick` emits **no radio and no label at all**, so the click is not
quiet, it is absent. The second half was worse: in the whole of calendar-v4
there was no way to create or edit an event, because v4 replaced the *reading*
surface and never replaced the *writing* one.

**THE AGREEMENT LAW IS THE RULING, AND IT IS PINNED IN GO AT THE PRODUCER.**
For any day, the set of events the card lists is EXACTLY the set of `.lrow`
elements the ladder leaves visible for that day. Not a superset, not a subset.
The card's payload is built from the SAME viewer-filtered `BlockData` the Block
renders from — one pass, no second repository read, no second filter — which is
the discipline r52 §5 chose for the count oracle, for the same reason, and it is
why the two surfaces cannot drift. The assertion is **joined to
`block_count_oracle_test.go`** (GM / Nissa / Bryn on the signed fixture) rather
than forked, because a card that showed one more event than the Ledger would be
a permission leak wearing a UI change's clothes, and the difference between the
two sets is exactly the count of events the viewer is not allowed to know about.
Asserted in JS instead, it would have been a claim about DOM that — for a viewer
with the Ledger switched off, which is the viewer the card exists for — is not
there at all.

**THE PAYLOAD LAW IS A TYPE, NOT A CONVENTION.** The page attribute carries the
Ledger row's own field set and nothing more: id, the day's two keys, title,
time, the (axis, pattern, glyph) triple, the `dm_only` rail flag, the audience
label. `dayCardEvent` has no field for a description, for `visibility_rules` or
for recurrence — so the growth path is a code change with a failing test rather
than a judgement call. The cost of getting this wrong is specific: a payload
that grew a description body would put every event's prose into every viewer's
DOM for a card that never displays it. Those fields arrive instead over one new
`GET`, under the editor's own floor, for the one viewer who is about to write
them back.

**"READ AND LISTEN, NEVER MUTATE" IS THE MECHANICAL FORM OF THE BLOCK'S
INTERIOR LAW.** Round 2 works AROUND the Block; this slice opened no file in
`internal/widgets/calendar_block`. That is a claim about a diff, and
diff-shaped claims decay — the next hand adds "just a class" from the page and
nothing fails. So the rule is asserted at RUNTIME: a JS test boots the module
against a Block-shaped fixture and requires the host's serialised DOM to be
byte-identical before and after open + close. The one path that looks like an
exception is not: the `Open in the Ledger` door calls `.click()` on the day's
own radio, which changes CHECKEDNESS — IDL state, not a content attribute — so
the serialisation is unchanged, and the alternative would have been the module
simulating the browser instead of using it.

**THE CARD IS A PAGE-LEVEL SINGLETON IN THE TOP LAYER, AND BOTH HALVES ARE
LOAD-BEARING.** N Blocks would otherwise mean N popovers, N listeners and N
payload copies, and "the card and the Ledger cannot disagree" would stop being
checkable in one place. `[popover]` is what escapes `.cal-block-host .block`'s
own `overflow` clip WITHOUT touching the Block; CSS anchor positioning would
have needed a unique `anchor-name` on every day cell, i.e. an edit to
`instrument.templ`. It is `popover="manual"` rather than `auto` for a motion
reason: the UA's light-dismiss and Escape close a popover SYNCHRONOUSLY and the
hiding `beforetoggle` is not cancelable, so `auto` would have run the register's
160ms close on the button path and skipped it on every other one. One grammar
means one close, so the module owns dismissal.

**THE GUARD AMENDMENT, AND WHY IT IS AN AMENDMENT.**
`TestBenchCSS_NoMotionAtAll` was already an allowlist — R2-1 inverted it when
the register landed. This slice widened it by **zero bytes**: the card reuses
the same transitionable properties, the same two durations and the same easing,
inside the same single `prefers-reduced-motion: no-preference` wrapper. What was
added is the CLAIM that it does, failing in both directions — if the card's
rules vanish, and if they appear outside the wrapper. "The card consumes the
register" is exactly the kind of sentence that decays into a second grammar the
moment nothing checks it, and a second register section anywhere is laundering.
The card's own sheet gained its own monopoly guard for the same reason: a second
file, out of reach of the Bench guard, is where a second grammar would actually
grow.

**A ROUTE FLOOR IS NOT A SECURITY BOUNDARY, AND THE BODY IS GATED TWICE.** The
one new read is `RolePlayer` because a player may legitimately read an event
they can already see — the card lists it and the Ledger prints it. What gates
the body is the grid's own viewer filter, plus a second gate on the two audience
fields. Those two are gated TOGETHER at the Scribe floor rather than split
Scribe/Owner, and the reason is worth recording because the instinct is the
opposite: `PUT` re-writes the whole record, so withholding `visibility_rules`
from the person who is about to overwrite it does not protect the audience — it
DESTROYS it, silently, on the first save of a restricted event, with players
seeing something they should not as the only symptom, days later. The editor
round-trips every field it has no control for, and it can only do that with what
it was given.

**THE A11Y GAP IS STATED RATHER THAN PAPERED OVER.** Both openers are wired —
the cell's `click` and the day radio's `change` — so the keyboard path exists
wherever the radio does. Where the Ledger is not docked the radio does not
exist, and the card is pointer-only for that viewer. Injecting `tabindex` would
have been both the mutation the boundary forbids and a focusable control the
server never rendered, so the gap is BOOKED (**C-CALV4-DAYPICK-A11Y**) as the
widget change it actually is. A slice that quietly shipped a pointer-only
affordance and said nothing would be the failure mode this arc's honesty rules
exist to prevent.

**WHAT SPLIT OUT, AND THAT THE SPLIT WAS PRE-AUTHORISED.** Stages 1-2 — the card
and the editor's MECHANISM against the shipped API — answer the operator's
complaint on their own. Stages 3-4, the editor's full chrome pass and
drag-create, became **C-CALV4-EDITOR-R2b**, taken at a stage boundary with
everything green. The editor's visual gate was never the card's to wait on: the
mockup is mid-fix and un-re-reviewed, so it is a REFERENCE for fields, states
and vocabulary and NOT a fidelity gate, and only the chrome pass waits on the
operator's stills.

**AN ABSENT KEY CAN BE A WRITE, AND ROUND 2's VERIFIER FOUND WHERE.** The
round-trip discipline above was applied to `entity_id`, `description_html`,
`visibility` and `visibility_rules` — every one of them a POINTER on the shipped
`PUT`, where nil genuinely means "unchanged" because C-CAL-NULL-PRESERVE
nil-guards them. It was NOT applied to `is_recurring`, and that field is a
value-typed bool whose guard was left off ON PURPOSE: *"IsRecurring — bool:
false IS the value, not 'absent'."* On such a field there is no such thing as
saying nothing. The editor's body carried no key, the bind produced `false`, and
renaming a recurring event through the day card un-repeated it — while the
nil-guarded `recurrence_type`, `recurrence_interval` and `recurrence_end_*`
around it all survived, leaving precisely the half-state
C-CAL-RECURRING-PARTIAL-STATE-CLEANUP already had to clean up once. The module's
own comment asserted the opposite in so many words.

The lesson generalises past this slice and is why it is written here rather than
in a commit message: **"the client round-trips what it does not offer" is only
true field-by-field, and its truth depends on the field's TYPE on the wire.**
For a pointer, omission preserves; for a value, omission overwrites. A partial
editor is therefore lossless only against a request struct whose optional fields
are all pointers, and the two fixes are (a) the read route must HAND BACK
everything the write path will clobber — the record could not round-trip a field
it was never given — and (b) the guard must be a request-body assertion over a
stored record that HAS the field set, because a suite that only ever builds
bodies from empty prevs proves nothing about preservation. Both halves are now
pinned, in Go and on the JS wire, and the service's deliberate non-fix is pinned
too, in the voice `TestUpdateEvent_EntityIDStillClearsOnNil` uses, so a later
sweep reads why the client's round-trip exists before deleting it.

**A GUARD THAT ENUMERATES BY EXAMPLE ENUMERATES YESTERDAY.** Two of the slice's
own guards were found green-but-blind by the same pass, and both had the same
shape: they described the thing they guarded using a *sample* of it. The payload
law's inventory was taken by marshalling a hand-written `dayCardEvent` literal —
but every optional field is `omitempty`, so a ninth field would simply be absent
from that marshal and "want exactly these eight keys" would still pass. The
stylesheet's scope guard read only lines ENDING in `{`, so the sheet's 21
single-line rules were never examined at all. Neither was wrong today; both
would have gone on being green while the thing they claimed stopped being true.
The fixes are the same fix in two languages: **derive the inventory from the
DEFINITION, not from an instance of it** — reflection over the type's json tags,
and a brace-scanner over the sheet instead of a line filter. Where a guard's
subject can be enumerated mechanically, enumerating it by hand is a guard that
proves what its author remembered.

**A DODGE ON ONE AXIS IS A RULE ABOUT ONE LAYOUT.** [DC-3] signs that the card
may never occlude the Ledger, and the first two cuts implemented that as a
single left-clamp: when the card's box shared a Y band with the Ledger, keep it
entirely left of `ledger.left`. That is the correct answer, and it is complete
only while the Ledger IS a right-hand column. The Bench stacks it FULL-WIDTH
BELOW the grid at narrower sizes, and a band starting at x≈9 that spans the
whole Bench leaves no left to move to — the clamp computed a negative limit and
no-opped, the vertical branch above it only ever flipped for VIEWPORT room, and
a card that always fits below its day therefore landed on the band for every day
and every viewer between roughly 625px and 884px of `.cal-bench` content width.
The editor escaped only by accident: it is tall enough not to fit below, so its
viewport flip happened to clear the band — which is the proof that the missing
dodge was available all along.

Three things are worth recording past the fix itself.

**One: the exclusion zone is the Ledger's SETTLED RECT, not its role in the
layout.** Docked column and stacked band are the same obligation, and the rule
is now written that way — the placer asks "does the box intersect this rect",
never "is the Ledger to my right". A rule expressed in terms of one layout is a
rule that silently stops applying when the layout changes, and this arc changes
layout by breakpoint on purpose.

**Two: the degradation is an EXISTING signed treatment, not a new one.** The
order is below → above → the bottom sheet, and the sheet is [DC-3] bullet 4's
own answer. There is no third geometry and the card is never resized to fit: a
card that shrank to dodge would be a different card, and the sign-off covered
one card and one sheet. Reaching for a novel geometry under pressure is how a
product acquires a shape nobody signed.

**Three: retiring a flag with its harm un-signs it.** The desktop sheet fallback
means the geometry RAN OUT, which is the condition [DC-3] named a STOP-AND-FLAG;
the card simply no longer covers anything while it happens. So the placement
carries whether the sheet was a fallback or the layout, and the reporter speaks
for the first and stays silent for the second. `data-dc-clear` is written
honestly in every mode either way.

The reporting lesson is the one the arc should keep. The gate was measured at
1232px and at 390px and reported as "clear" and "the signed mobile treatment" —
both true, and the sizes between them were never named. **A geometry claim is a
claim about the widths it was taken at**, and the widths where a layout changes
shape are precisely the ones a two-point measurement omits. The mislabel
compounded it: the JS suite's one negative case called the stacked-Ledger
geometry "a pathological geometry", so two reviews read the product's own layout
as an impossible one. It is now a positive regression case at the ~884px and
~944px boundaries.

**A CANDIDATE THAT IS DROPPED IS A GEOMETRY THAT WAS NEVER ATTEMPTED, AND A
FALLBACK CANNOT TELL THE DIFFERENCE.** The fix above shipped with the ABOVE
candidate admitted only when it fitted the viewport outright, and everything
else falling to a clamp that pins the box to the BOTTOM of the viewport — which
is where the stacked band lives. So the fix closed the case it was written for,
the 227px card, and opened a larger one on the same code path: a box TALLER than
the room above its day never flipped at all, failed both candidates and took the
desktop sheet, covering 100% of the Ledger across the same band. It reached that
with no unusual data at either end — `+ New event` produces a 420x400 editor,
and twelve events on a festival day produce a ≈379px card — and the pre-fix
module had placed that same editor CLEAR, by accident, at the same widths. The
correction is a clamp on the existing flip rather than a new position: `above`
still means "start the box above the day", and a box too tall for the room above
its day starts at the viewport's top edge instead of off-screen. Two anchored
candidates and one sheet, unchanged.

Two further things are worth recording, and both are about what a fix costs
elsewhere.

**Four: a warning that fires on a fallback is asserting that the fallback was
necessary.** "This geometry cannot place the card clear of the Ledger" is a
claim about the world, and it was false for every one of those boxes — a 481px
card at top=8 ends at 489 above a band that starts at 595. A STOP-AND-FLAG that
can be raised by an un-attempted placement is worse than none, because it
retires the question. Point three above (retiring a flag with its harm un-signs
it) has a mirror: raising a flag in place of an attempt un-signs it too. The
module's own comment was the mechanism — "THERE IS NO THIRD GEOMETRY", written
to fence off invention, read by the next hand as a reason not to look at the
candidate set at all.

**Five: a stub's default is an assertion, and an all-zero rect asserts that
nothing can be occluded.** The editor half of this was invisible to the JS suite
for three rounds because `daycard_dom`'s rect defaults to zeros and the card is
the editor's ANCHOR — so every editor in the suite was placed at the viewport
origin. The card's rect is now DERIVED from the placement the module just wrote,
never assigned, so a test cannot pin it to a value the placer did not produce.
Where a harness supplies a value the real DOM computes, the harness is making a
claim, and it should be made where it can be seen.

### Section: W-G Part B — the page that answers "when do we play" (C-CALV4-RSVP-P8 Part B, 2026-08-01)

**The gate was the drawing, and the drawing changed the build.** Part B of
`C-CALV4-RSVP-P8` could not be pasted until `mockups/calendar-v4-schedule.html`
existed and its stills were signed — the spec's own condition, restated in the
dispatch (*"Do not build the Verdict, the Matrix, the Roster or the Painter from
§2–§3 prose"*). The operator signed the mockup on 2026-07-29
(`decisions/2026-07-29-schedule-mockup-signed.md`), and the drawing pass had by
then discovered ten resolutions the prose could not have: **the load-bearing one
is a cascade fact.** The sheet declares `@layer …, schedule, motion, block,
bench, …`, so `schedule` sorts BEFORE `bench` and a schedule rule can never
override a signed Bench rule at any specificity. Three places where the spec
reused a signed class became NEW classes — `.sc-why` (the reason sentence needs
`--text-secondary`; `.dt` is `--text-muted`, below the floor for a word read in
order to ACT), `.sc-foot`, `.sc-body`. **Production ships unlayered**, so the
guarantee is obtained by CONSTRUCTION instead: a Bench-owned subject needs an
`sc-` ancestor, and a test reads BOTH sheets, because "this file redefines
nothing that file defines" is a claim about a relationship and a relationship
asserted from one side is not asserted at all.

**ONE ROUTE IS THE WHOLE BUDGET.** `GET /campaigns/:id/schedule`, Player+, on
the identical `cg` stack every other calendar route rides;
`routes_snapshot.txt` 723 → 724, no migration. Everything this page WRITES rides
a route that already shipped: the scheduler's own availability PUTs
(`member_availability` for normal hours, `availability_exceptions` for a date
override — two endpoints because the `[This week only | Every week]` segment
means two different things and already has two tables), P8A's Player+ event RSVP
POST, and P8B's Scribe+ `/calendar/ask`. A second availability write path would
have been a second place to get the composition invariant wrong, and *"an offer
only ever adds, and never downgrades an hour already marked preferred"* is the
scheduler's own rule, enforced in the scheduler's own service.

**RANK 1 IS THE BENCH'S DERIVED WINDOW, BY CONSTRUCTION.** The Bench's RSVP
panel and this page's head candidate are one click apart and may not derive
"when to play" from two implementations of one idea, so `benchRsvpPeakRun` was
EXTRACTED and both callers read it; the oracle drives both public builders over
one `BenchAvailability` and asserts they name the same day and the same hour.
The Bench's printed sentence is byte-identical to what it was.

**PERMISSION IS ABSENCE, AND IT IS ASSERTED ABOUT THE PAYLOAD.** There is no
`if IsGM` in this page's markup at all: a player's `ScheduleData` carries no
lane, no out column, no other member's name, no awaiting-reply group and no
chip, so every loop simply has nothing to walk. The two role orders are SOURCE
order (Director: Verdict → Matrix → Roster · Painter → Answer; player: Answer ·
Painter → Verdict → Matrix), never CSS `order:` — a page whose tab sequence
disagrees with its reading sequence is unusable for exactly the people who most
need the reading sequence to be right.

**ONE NEW SEAM, BECAUSE IT ANSWERS A DIFFERENT QUESTION.**
`ScheduleOwnWeekReader` is the viewer's OWN composed week. It is deliberately
not a method on `BenchScheduleReader`: adding one would break every existing
implementation of a seam P8A shipped, and it would fuse two permission questions
into one contract. `BenchAvailability` answers *"may this viewer see EVERYONE'S
lanes"* and is gated by role; this answers *"may I read back what I saved"*,
whose answer is always yes. Without it a player's Painter would render an empty
grid over availability they had already entered.

**PROSE INHERITS PERMISSION, AND THAT IS WHERE IT IS EASIEST TO GET WRONG.** The
candidate cards' reason sentence computes clauses like *"N never answered"* and
*"X out"* from the lane map. A player has no lane map — so the clauses were
computing over an absent one and every member read as never-having-answered: the
card told a player that five of five people had ignored the question. The
clauses are now gated on the lane data EXISTING rather than on `IsGM`, and the
aggregates a player CAN state survive. A false sentence is worse than a missing
one, and prose is where a permission bug reads as innocuous.

**A NEVER-ANSWERED MEMBER IS NOT AN ABSENT ONE.** The honesty ledger's item 3
stays open (there is still no `HasPattern` signal), so it is enforced in the INK
instead: a known-busy member gets a filled swatch under `out`, a never-answered
member a hollow one under `no answer`, and an empty lane prints the sentence
saying exactly what is not known, in neutral ink — an unknown is not a fault.

**THE DOOR IS THE PANEL'S OWN TITLE.** The Bench's RSVP panel has been called
`RSVP · Schedule` since the signed contract drew it, so in Part B that title
becomes the link. It could not in Part A because the page 404'd. It is not in
the nav because WG-2's signed ruling keeps the nav pointing at `/availability`
and books the retirement as its own slice. All 23 stills from the Bench's own
shot harness are byte-identical before and after the link landed.

**THE FIDELITY GATE IS PIXELS, AND IT EARNED ITS COST TWICE.** Holding the built
page beside the signed stills found six defects across two passes that every
string assertion had passed: the player's false reason sentence and an invalid
`font:` shorthand copied off the mockup (`font: 500 12px/1.5 inherit` names
`inherit` as the FAMILY, which is invalid and dropped silently), then an
uppercased zone chip (`NEW_YORK` is not a member of `America/New_York` — a
`.badge` is uppercase because it is a LABEL vocabulary, and an identifier is not
a label), a candidate card whose zone wore the weight of a time, seven count-lane
numerals below the signed 24px pointer floor, and two heads that under-named the
slot. **Measure the page with a driver, never with `chrome --headless
--window-size`:** the CLI clamps the window to a 500px minimum, so a "390px"
shot is a 485px layout cropped to 390 and reads as a phone defect that does not
exist. And scroll a control into view before probing it — `elementFromPoint`
returns null outside the visual viewport, so a page-length surface probed in one
pass reports its own below-the-fold controls as unpadded.

**A THIRD PASS FOUND SIX MORE, AND FIVE OF THEM WERE ABOUT SHAPE RATHER THAN
WORDS.** Every caption on the surface shipped with the right sentences and none
of the drawing's emphasis — five bold lead-ins and its italic vocabulary words,
gone. That is not decoration: two of those lead-ins ARE this page's named
honesty claims (*"What the score cannot include"*, *"Fine and coarse disagree,
on purpose"*), and a claim nobody can find in a wall of grey prose is honesty as
decoration. Captions are therefore a MODEL now — `ScheduleCaption` is a slice of
`{Text, Em}` runs and the view draws `<b>` / `<i>` / bare text per run — never a
markup string, because five prose constants rendered raw would be five injection
sites the first time one of them takes a campaign's name. `Text()` joins them
back for every assertion about WORDING, so a test about what a caption says
never has to know where the eye is meant to land.

**FIX THE CLASS, NOT THE INSTANCES.** The verification pass named five missing
bold lead-ins; repairing exactly those five and stopping left three more on the
page, outside a `.caption` — the Painter's scope note (which bolds WHICH TABLE
the marks land in), its foot, and the suggest dock's note. The sweep that found
them is the one that should have followed the first finding: an inventory of
every `<b>` and `<i>` the mockup emits inside its SURFACE producers, checked
one by one against the build. A findings list is a sample, not the population.

**A COLOUR MIX NEEDS A HUE TO MIX WITH, AND `transparent` IS THE ONLY SAFE
SECOND TERM.** The drawing tints a pressed segment rung
`color-mix(in oklch, transparent 96%, var(--accent))`. The sheet shipped
`color-mix(in oklch, var(--surface-card) 88%, var(--accent))` — which reads as
the same idea said differently and is not. `--surface-card` is achromatic, and
oklch resolves a missing hue by the SHORT ARC: 0deg toward the accent's 270deg
travels 349.2deg the other way round the wheel and lands on PINK. Measured
`rgb(252,232,241)` (R>B>G) against the sealed still's `rgb(248,249,254)`
(B>G>R), on every segment on the page, at both widths and both themes, for four
stages before anyone looked. Mixing into `transparent` has no hue to interpolate
and cannot drift, which is exactly why the drawing does it. **Substituting a
surface for `transparent` in an oklch mix is a hue decision, not a syntax
preference** — and it is invisible in review because the diff looks like a
refactor. The one surface-toward-accent mix the sheet is allowed is `.surf.sel`,
which the drawing writes itself and annotates `D-T1: this tint drifts rose in
light. Booked upstream, not fixed here.` The pin therefore COUNTS the sanctioned
mix rather than forbidding the shape, so the drawn exception cannot be deleted
to make the rule pass.

**A NO-OP DECLARATION CAN BE LOAD-BEARING, AND "TIDYING" IT IS A CHANGE.** The
drawing's narrow rule writes `.sc-row .rs{width:auto}`. Nothing sets `width` at
base, so the declaration does nothing — and that is its entire function: it
occupies the slot without disturbing the base `min-width: 30px`, which is what
right-aligns the five answers into a column. It was transcribed as
`min-width: 0`, a different declaration with a real effect: the well collapses
30px → ~11px and `19:00` and `in` collide into `19:00 in` where the still spaces
them. **A declaration that appears to do nothing in a signed artifact is a
question, not a redundancy.** It is restored as written, with a comment saying
why, so the next reader does not tidy it away again.

**DIFF THE SEALED SHEET DECLARATION BY DECLARATION, NOT BY EYE.** Both defects
above, plus a missing `gap: 5px`, a missing `:hover` state (the segment gave a
pointer no feedback at all) and a `cursor: default` where the drawing writes
`not-allowed`, survived four fidelity passes and a targeted sweep that claimed
this exact rule block — the sweep fixed `padding: 0 10px` → `0 8px` two lines
above and stopped. What found them was a parser that normalises the
`.cal-schedule ` scoping and whitespace and then compares the drawing's `<style>`
to the shipped sheets declaration by declaration. The tell is positional: the
missing `.sc-row .say .badge` rule sits in the drawing BETWEEN two rules the
sheet carries in order, so it was dropped in transcription, not decided against.
**A rule missing from the middle of a carried run is a transcription defect, and
only a mechanical diff sees it** — screenshots show you a page that looks
plausible, because a 27% oversized chip looks like a chip.

**A STAND-IN SHELL MUST PAD LIKE THE SHELL IT STANDS IN FOR.** The fidelity
harness cannot render `layouts.App` (it needs a request), so it renders its own
wrapper — and that wrapper padded a flat 20px where the product's `<main
class="px-3 py-3 md:px-5 md:py-4">` pads 12px below 768. Eight pixels, and the
drawn narrow matrix has exactly eight to spare, so the phone shot clipped a
column the shipping page does not clip. A measuring instrument that differs from
its subject reports its own defects as the subject's. The paddings are named
constants now and a test asserts the harness's budget equals the product's.

**A DOCUMENT-LEVEL SCROLL CHECK CANNOT SEE A PANEL THAT DRAGS.** A nested
`overflow-x:auto` container never contributes to `documentElement.scrollWidth`,
so *"the page does not drag sideways in any role, theme or width"* was TRUE
while the matrix dragged inside its own panel. Any surface with a scroll
container inside it needs a per-element measurement, and this one's now records
every offender with its panel and its drag in pixels.

**A WIDTH-DEPENDENT STRING ON A SERVER-RENDERED PAGE IS TWO STRINGS.** The
mockup's producer re-runs on resize and simply swaps three sentences for shorter
ones at 640 (`who`, `free of 5`, `nothing saved`). Rendered once on the server,
the page emits both and one media query chooses. It is used ONLY where the two
forms say the same thing in a different number of words — both are in the
accessibility tree, so it can never be a way to hide a fact from one width — and
it was worth the duplication because the wide forms did not merely clip: the
empty lane's sentence clipped its own `no pattern` chip out of the row, and an
honesty chip that vanishes on a phone is the one thing this page forbids
everywhere it forbids anything.

**WHAT IS DELIBERATELY NOT BUILT, AND SAYS SO.** `Propose` and `Copy last week`
remain chipped Director-tier scaffolding — and are ABSENT from a player's DOM
rather than disabled, because the Director is the person being asked to sign off
on what is missing and a player is not. Conflict detection against booked events
(ledger #16) stays out of scope with the caption saying so permanently: this
grid shows availability only, and a window may collide with something already on
the calendar.

### §26 — W-H: the builder wizard, and why the preview is the Block

**C-CALV4-WIZARD-P13, wave 4.** The last wave of the remodel and the only one
whose stated gate is *finish* rather than capability. L6: "Builder wizard wraps,
it does not replace. New capability is explicitly not the point; style, animation
and finish are."

**THE PREVIEW IS THE SHIPPED BLOCK, AND A SECOND PRODUCER WAS REFUSED.** Wizard
form state becomes an in-memory `*Calendar` — never persisted — that is fed to
the EXISTING `projectBlock` and rendered by `calblock.Block`. Every link was
already pure: `Block` performs no queries and reads no request state,
`projectBlock` takes no context and no repository, and the only DB work in the
Block path (`requireVisibleCalendar` → `candidateEvents` → `tiedEventIDs` →
`CalendarLinkStatus`) happens BEFORE it and a draft skips all four. `projectBlock`'s
`*Calendar` had only ever come from the DB; W-H WIDENS ITS PROVENANCE, NOT ITS
CONTRACT.

Two alternatives were refused on evidence. A second pure `wizardState → BlockData`
producer would double the geometry surface, so the next geometry change — leap-aware
day counts, the five-column rule, the era-band Half/Edge semantics, the moon cap —
would have to be made twice. A client-side re-render would create a FOURTH counter
against `TestBlockCounterDivergencePin`, which is the exact class of bug the whole
spine exists to prevent.

Three consequences fall out for free and are the reason this is the cheap answer:
the draft carries `ID: "draft"` so DOM tokens and the identity triple are
deterministic and the fidelity gate is reproducible; `CampaignID` is empty so
`blockPrefsPath` returns `""` and the draft gets no switchboard, which makes r54's
`HasSwitchboard == (PersistURL != "")` hold BY CONSTRUCTION rather than by an
assignment somebody must remember; and a wizard with no months renders
`blockDateLine`'s OWN shipped fault text, so no bespoke "nothing to preview yet"
placeholder exists anywhere in the diff.

**THE LEAK GUARD, AND WHY THE STYLESHEET IS ITS OWN FILE.** The design asks for a
preview that cross-fades and the Block sits inside it, so this is where the
Block's austerity is most likely to be violated. Four mechanisms, each mechanical:
wizard motion lives in `static/css/calendar-builder.css` and nowhere else (the
Block's four-property `motionBudget` and the Bench's three-property register are
byte-unchanged); every wizard class is `wz-`-prefixed, because ROOT-SCOPING ALONE
DOES NOT STOP THE LEAK — `.badge`, `.field`, `.frow` and `.cell` are live product
classes and a descendant combinator reaches straight through the Block's own
scoping root; that scoping root's name appears in the wizard's sheet ZERO times;
and `builder_css_contract_test.go` asserts all three in one regex.

**THE BUDGET GUARD IS THE WAVE'S MOST DURABLE DELIVERABLE.** A wizard stylesheet
would have been governed by nothing — the shell guard does not police `static/css/`
at all. The motion policy asked for the property allowlist on 2026-07-27 ("this is
the durable part — everything above is advice until a guard enforces it") and it
was never built. It landed in stage 1, before a line of wizard CSS existed, so the
wizard was written INSIDE a guard rather than retro-fitted to one.

Its limits are also now known, and that is worth recording. A comment terminator
written by accident inside prose closed a comment early, CSS error recovery
discarded the token prelude that followed, and the ENTIRE motion register silently
stopped running — while all six assertion families stayed green, because they read
the sheet through the same comment regex a parser uses and every rule they check
was still present in the text. A browser probe found it in one reading. **A source-
text guard cannot see a cascade outcome; that is what the browser probes are for,
and it is why this wave's gate is not stills alone.**

**BUILD ON SUBMIT — NO DRAFT STORE, NO MIGRATION.** Draft state lives in the form
and the `?step=` URL; every preview POST rebuilds it from the posted body; the
terminal POST creates and applies atomically. It is the ONLY option under which
"nothing is written until Create" is literally true rather than advertised, and
the only one with no abandoned-draft cleanup story and no egress question — a
draft is not campaign content and never becomes a row, so it enters no export and
no AI-workspace DTO by construction. A `calendar_drafts` table would have bought a
migration, a TTL reconciler, an egress assertion and an IDOR review to solve a
problem the form already solves.

The pairing that makes it work is the carry/read pair, and its failure mode is
severe and silent: the month name list was initially split between the carry and
the owning station, which concatenated two groups in submission order instead of
interleaving them, so every month took its neighbour's name — the right month
count with the wrong month names, and nothing to notice it by. Either month
station now emits the WHOLE ordered list, hidden in place for the family it does
not show. A round-trip test over all nine stations is what caught it and what
keeps it caught.

**THE NEEDS-BACKEND AUDIENCE GATE ON THIS SURFACE IS THE ROLE FLOOR.** Every route
carries `RequireRole(RoleOwner)`, so every viewer of the builder is an owner and
the GM-facing chip can never reach a player — satisfied BY CONSTRUCTION rather
than by a branch. That is a finding rather than a free pass: if a later slice
lowers the floor to Player+ "so co-DMs can look", it ships four `needs backend`
chips into a player's markup. The reason is written into the handler's header and
the templ's, and the rule is that the floor and the chips are re-audited together.

**THE ERA GATE IS WIZARD-LOCAL POLICY, AND THE COPY SAYS SO.** Chronicle has no
era-relative year numbering — a zero-era calendar resolves "Deepwinter 14, 1523"
perfectly well — which is why wave 1 examined the era fault, refused to synthesise
it, and left a coordinator flag in `block_projection.go`. **That flag is closed
here.** The wizard may decline to CREATE what would read ambiguously, even where
the Block would render it; what it may not do is misdescribe why. So the copy
reads "this calendar has no era — Create waits", a claim about the wizard, and a
test asserts both that the copy is that claim and that the Block itself resolves a
zero-era date without a fault — which is the evidence the gate is policy and not
physics.

**THE CHOOSER IS RETIRED, AND THE ROUTE IS WHAT MOVED.** `GET /calendars/new` now
resolves to the wizard. A dozen surfaces link to that path and one of them is in
the frozen `calendar_v2` set, so re-pointing the handler lands every one of them —
and every external bookmark — on the designed surface with no href edited in a
file this wave does not own. The path is unchanged, so the snapshot is unchanged.
Three pins were refreshed rather than deleted; the banner pin's `t.Fatalf` fired
exactly as its dispatch predicted, which was the pin doing its job.

**THE FIX ROUND, AND THE THREE THINGS IT IS WORTH REMEMBERING FOR.** The wave's
first pass was rejected by adversarial verification, and every finding came from
a browser rather than from a suite that was, at the time, entirely green.

*A gate scoped to a subset states a fact about the subset.* The narrow-lane probe
walked four of the eleven station sheets, and the report stated its 24px floor as
if it held everywhere. The control that broke — a moon-name input at **0×26**,
its value in the DOM and nothing on the screen — lived on a station the probe
never visited. Two exemptions inside it made the same class invisible even where
it did look: the floor check skipped anything measuring zero wide, so the WORST
case was the one case it excused, and nothing measured whether a control could
show its own value, so a field clipping "Reckoning of Wards" to "Reckonir"
read as passing. The roster is now every shot key, the guard is gone, and the
probe compares `scrollWidth` against `clientWidth` on every text input.

*A derived readout must name the arithmetic it shares, not the field it hopes is
filled.* The Moons station read its turn marks out of `Block.Month.Almanac` with
a comment claiming they "come from the SAME Block data the grid drew" — but
`buildMonthGeometry` fills that register only when the Shelf zone is docked, and
the wizard docks neither. The register was empty for every moon of every preset,
so the station printed "no turn this month" for a 31.4-day moon in a 30-day
month, unconditionally, under a subtitle promising every turn is derived. The
comment was true about intent and false about mechanism. The fix calls
`blockAlmanacRegister` off `monthBaseAbsoluteDay` directly — the same shipped
functions, asked for rather than hoped for — and the test pins BOTH halves: the
Block's own register must stay empty, and the station's must carry turns.

*Reuse means inheriting the reused thing's limits, and inheriting them silently
is a lie.* Three features the signed stills draw prominently are absent from the
shipped preview, and all three follow from rulings: era bands ride the "eras"
layer and the wizard passes DEF `["moons"]` ([WZ-2c]); the phase marks and the
intercalary band row are the Block's full-tier treatment and this column resolves
to std. None of that is a defect. What was a defect is that the surface asserted
the opposite in three places at once while drawing none of it. The preview now
carries a note under the month naming all three absences — *"the preview
under-states the result; it never invents one"* — and a test pins the layer set,
the still-present band DATA and the note's words together, so the disclosure
cannot rot back into silence. **Left open for the coordinator, not resolved
here:** the signed stills draw the bands and [WZ-2c] signs DEF; reconciling them
is a host layer-set amendment, which is a coordinator act.

**THE SECOND FIX ROUND — three more, and the first is the worst thing this wave
did.** Adversarial verification rejected the first fix round too, on four
blocking findings and three non-blocking ones. All are closed and the lessons
are these.

*A role floor that lives on the route table is a property of the route table,
not of the handler.* §6.3 SIGNED says every viewer of the builder is an owner and
that the `needs backend` chips' audience gate on this surface IS that floor, and
the handler's own header asserted the rule was satisfied "BY CONSTRUCTION". It
was satisfied by three route registrations. Then a stage re-pointed `Index`'s
zero-calendar branch at `ShowBuilder` — and `Index` lives on the PUBLIC group
behind `RequireViewAccess`, which admits every role AND, on a public campaign,
authenticated non-members and logged-out visitors. Measured, not inferred: role
= player and role = NONE both rendered the whole wizard at 200, 58862 bytes, two
`wz-badge wz-need` chips and a live Create button at `?step=review`. The
capability was DISABLED (the POST 403s), not ABSENT, which is exactly the
distinction `decisions/2026-07-27-needs-backend-audience.md` exists to refuse.
The floor now travels with the handlers through one predicate byte-identical to
`RequireRole`'s own, so a Go call reaches the same gate a router does; `Index`
sends a non-owner to V2, whose `calendarV2EmptyState` is the DESIGNED non-owner
surface for a campaign with no calendar. **And the test named for the property
had never had one** — it asserted chip TEXT and exercised no role at all, which
is why the regression shipped green. A test whose NAME asserts a property its
BODY does not exercise is worse than no test: it occupies the slot.

*A composition ratified "as drawn" is a composition, not a headline.* [WZ-15]
item 5 ratified the fault sheet's TWO HONESTY STATES AT ONCE — the anchor
asserting what today resolves to, beside a Year length in warn ink and a grid
saying it has nothing to draw. The build replaced the anchor with a warn rail
reading "Cannot resolve a date" and drew an empty grid box, so the sheet printed
a false claim about the model a hundred pixels above the Block's own Nameplate
reading "Hammer 1, RoW 1523" — the very falsehood [WZ-3] was signed to remove
from the OTHER fault. The cause was scope: the wizard's anchor fault asked "is
anything in the declaration broken" where the drawing asks "does TODAY resolve".
Each question now goes to the thing that can answer it, and the Block is not
asked to draw a month with no days, because an empty grid shape is the
placeholder §6.2 refuses by name.

*Evidence claiming to be exhaustive is a claim, and it is gated on.* The
screenshot index listed nine data-forced differences and called them stated, not
hidden. The verifier found four more by eye in one pass — the month grid's lead
of six, which is real `monthBaseAbsoluteDay` arithmetic and the most visible
thing in the preview column; the resolved-date line's FORMAT as well as its day;
two-letter weekday headers; wrapped delete controls — and this round found
several more. The list is twenty-four now and says which are rulings, which are
arithmetic, and which are this slice's own judgement. **A gate that reads off a
list inherits that list's honesty.**

*And a motion deliverable cannot be gated on stills.* §12.1(ii) required a clip
per shipped pass and the evidence directory held zero; the still set was itself
non-deterministic, six of forty-two differing between runs purely on the frame
they landed. Stills are now captured at rest and reproduce **content-identically,
caption-glyph rasterisation aside** — 39 of 42 byte-identical across two full
regenerations, with `importer--dark`, `presets--mobile-light` and
`step-eras--dark` differing only inside rendered text runs and the membership of
that trio varying between run pairs. (This sentence read "byte-for-byte" for one
round longer than it was true; §26's tail corrects it below and this is the same
correction at the point of claim, because a fix that lives only in the retraction
leaves the assertion standing.) The motion is gated by five built clips — each
frame a separate headless load
with the register paused at an exact time through the Web Animations API, so the
film is reproducible and its time base is the register's own. Splitting the two
is the point: a still proves the resting state, a clip proves the transition,
and asking either to do the other's job is how a signed gate item quietly
becomes an executor's judgement call.

*One product bug came out of the evidence work, which is the argument for doing
it properly.* The importer parsed the dropped file and threw it away: the
detection line and the eight-row mapping table were built while the DRAFT was
left untouched, so the importer's own honesty mechanism reported the facts of
whatever was already on screen under the name of a file it had not adopted, and
"Continue to Review" carried the preset to Create. Nothing in the suite could see
it, because the still that would have shown it was photographed before the drop.
The file now BECOMES the draft through the same `builderDraftFromImport` a preset
goes through — §2.2's "one code path, two front doors", in code rather than in
prose — and is re-validated against §7.3's bounds, because an adopted file is
input like any other.

**THE THIRD FIX ROUND — five coordinator rulings, and the first is a rule that
existed only on paper.** Adversarial verification rejected the second fix round
on two blocking findings and three non-blocking ones. The coordinator ruled on
all five rather than leaving them to the executor, because two of them were
choices about what the product means and not about whether the code matches it.

*A rule that is present is not a mechanism that runs.* §5.2 pass 2 and [WZ-8]
SIGNED tabulate a delay ladder — "the station's content arrives in reading
order", `calc(min(--m-i, --m-cap) * --m-step)`, ≤132ms added — 133.3ms once
`--m-step` is divided rather than rounded (`--t-fast`/3 = 33.333ms × `--m-cap` 4),
which is what the sheet and the probe now both say. The rule was in
the sheet, in the one prelude it is legal in, composing the right tokens, and it
did nothing: it was declared BEFORE pass 2's `animation:` shorthand at identical
specificity, and a shorthand resets `animation-delay` to `0s`. Every static
guard passed, because every static guard asks whether the rule EXISTS. Measured
in Chromium, every `.wz-frow` computed `animation-delay: 0s` while `--m-i`
resolved 0,1,2,… correctly, and the clip evidence had been read as showing a
stagger when what it showed was `--t-base` plus one frame of encoder residue.
The repair is the declaration's position. **It deliberately diverges from the
sealed mockup**, which has the identical cascade defect and therefore lands its
own rows flat: the written signed mechanism outranks the drawing's accident
(coordinator ruling R1), the divergence is disclosed in the wave's evidence
index, and the mockup's one-line fix is BOOKED rather than taken, because a
sealed artifact is not edited from a build slice. Two pins now hold it — a
byte-ORDER assertion, because nothing in the file's shape tells a reader those
two rules are order-coupled, and a browser probe that measures the delays and
the reduced-motion silence in a real engine.

*An inherited divergence is disclosed and booked, not patched per surface.* The
filled CTA renders indigo where all twenty-two signed stills draw amber, because
both v4 sheets alias the contract's dedicated `--accent-action` to the campaign
accent. The exhaustive-differences list did not name it. The ruling (R2) is the
one the sibling wave already took a day earlier: it is product-wide and predates
the slice, so the remedy is a disclosure line, a comment at the aliasing lines,
and a real scope item on `C-CALV4-TOKENS-RESIGN` — where the question is not the
pixel but whether the primary action owns a hue independent of the campaign
accent. Patching it inside one surface is what has kept the other two token
defects alive for four waves.

*A host layer set is a coordinator act, and this is what it looks like when the
coordinator acts.* W-H shipped DEF `["moons"]` per [WZ-2c], disclosed on the
surface that the era bands the signed stills draw were therefore absent, and
booked the tension instead of turning a layer on to make a picture match. Ruling
R3 amends the host's own seed to `["moons", "eras"]` — DEF itself untouched, no
other producer's seed moved — and the preview's disclosure note SHRANK in the
same commit to the two absences that remain true. The layer set and the shrunk
sentence are pinned by one test, and the stale phrases are asserted GONE by
name: a note that keeps naming a fixed absence is as dishonest as one that hides
a real one. The Eras station's own copy, which told the author no band was
drawn, went with it.

*An acceptance item is either met or open; "neither" is the worst of the three.*
The list required each preset to round-trip against `export.go:BuildExport` and
no test referenced it. It does now, through the shipped hops with no restated
mapping — **and its first edition was a tautology**, which is the lesson worth
keeping. It compared `builderImportResult(d)` against `BuildExport(draftCalendar
(d))`: two derivations of ONE `*builderDraft`, so any payload change propagates
identically to both and they cannot disagree. Eleven distinct fields of
`presets/harptos.json` were mutated and it stayed green on every one. It was
green, it was honest about the export path, and it could not see the payload at
all.

A fourth round anchored it to hop 0 — the authored bytes — and two authored-data
drops came straight out: a moon's colour is not defaulted but **replaced**
(`builderMoon` has no colour field, so the wizard's front door discards colours
the plain importer preserves and stamps `#c0c0c0` over them), and an era's
**code** is dropped at Create (`builderDraftFromImport` reads it, the station
shows it, `builderImportResult` never writes it back). The second was invisible
because every preset's code happens to equal its epoch name, which does round
trip — a coincidence, now asserted by name so the day a payload separates the two
this reds. Six payload mutations were demonstrated red-then-green. **A round trip
compared only with itself proves the comparison, not the trip**; the anchor has to
be the thing nobody in the pipeline wrote.

*And the evidence's own arithmetic must describe the photograph.* Eighteen of
forty-two stills printed the 390px chain under a 500px capture; the tier
conclusion survived and the numbers did not, which is how a reader learns to
stop checking. The captions now state the photographed width with the 390
readings on a separate, attributed probe line. The "reproduces byte-for-byte"
claim became "content-identical, caption rasterisation aside", with the three
files named. **And the regeneration found one more, which is the argument for
re-measuring rather than re-asserting:** the two reduced-motion stills were
ordinary renders. The harness swaps its settle block for a
`@media (prefers-reduced-motion: reduce)` copy of the global guard, the capture
never asked Chromium for that preference, so the media query matched nothing and
the register ran — the pictures were evidence for the opposite of their own
caption, and they announced it by being the only two that failed to reproduce
between runs. The capture now passes `--force-prefers-reduced-motion`.

### Sections inside this ADR rather than beside it

W-F's layer switchboard and preference store became sections HERE when they
landed — §20-§24 above, W-G's tail is §25, and W-H's builder wizard is §26.
Round 2's reveal pass (R2-1) is a section here too, and R2-2…R2-5 will land the
same way.
There is no ADR-049. calendar-v4 is one architecture
decision; competing ADRs for its later waves would fragment the rationale that a
future re-litigation needs in one place. W-E followed this rule first (its
Almanac decisions are §10-§14) and W-G's RSVP decisions are §15-§19.

### §27 — R2-2b: the editor's chrome, the signed morph, and drag-create

**ADR-048 GROWS A SECTION; THERE IS NO ADR-049.** C-CALV4-EDITOR-R2b is the
second half of C-CALV4-DAYCARD, taken at the split point that slice's §11
pre-authorised, and every ruling below is a decision about the same surface.

**1. THE EDITOR-MORPH CARVE-OUT, AND WHAT IT LIFTS.** The operator signed it on
2026-08-01 (`decisions/2026-08-01-operator-signatures-wz1-sky-editor.md` §3):
the day card may visually morph into the editor **as its own named motion
signature**, the DAYCARD-era register-only constraint is lifted **for R2b only**,
and the morph must still be instant *and complete* under reduced motion and
never touch the Block's interior.

It lifts exactly one thing — [DC-7]'s "register-only in this slice" — and it does
not repeal the register. Clause 4 is what *creates* the category the morph
occupies: carve-outs are named signatures **on top of** the base. So the block
lives inside the ONE register section of `static/css/calendar-bench.css`,
immediately after the day card's, inside the SAME single
`prefers-reduced-motion: no-preference` wrapper. [DC-6]'s singularity clause
survives intact.

**THE NAMED CLASS IS `.edmorph`**, and it is present **only in flight** — added
to seed, removed when the geometry lands, re-added to reverse. A resting editor
therefore carries no transition at all, which is what keeps a later content
change (the audience roster opening under Restricted) from animating something
nobody signed.

**2. WHY THE MORPH IS A TRANSITION FAMILY AND NOT KEYFRAMES.** The five standing
refusals — `animation` · `@keyframes` · `will-change` · `@starting-style` ·
`view-transition` — survive the carve-out unchanged. **A signature is a grammar,
not a licence.** The register is a transition family; a morph built from
keyframes would be the second grammar [DC-6] refuses, and `view-transition` in
particular is also the one mechanism capable of animating across the Block's
subtree, which is precisely what the signature's last clause forbids.

The morph is **geometric and not a scale**: four properties and no fifth —
`inline-size` · `block-size` · `translate` · `opacity`. A FLIP scale is the cheap
way and it visibly squashes the text inside a growing box; this surface is a
*form*. `translate` is named on its own so the guard's allowlist can admit the
movement without admitting a scale, and `transform`/`scale` are asserted absent.
**No new token**: `--disc-open`, `--disc-close` and `--disc-ease` unchanged, two
durations product-wide, and close-faster-than-open is structural in the
register's own idiom — the base rule carries the close duration and the open
state overrides it.

**A DETAIL THAT LOOKS LIKE STYLE AND IS NOT.** The module writes the geometry
with the **logical** properties (`style.setProperty('inline-size', …)`), because
the carve-out's rule names them: writing `style.width` leaves the declared
`transition-property` matching nothing, so the box SNAPS while `opacity` alone
animates — which looks close enough to a morph in a still and is not one. It was
caught by parking the transitions at 0% and finding the editor already at full
size, and the same capture caught the deeper one: the morph's TARGET must come
from the size `placeCard` already measured, never from `getBoundingClientRect()`
on a box whose inner `.dcbox` is still collapsed to zero.

**3. THE RECURRENCE CORRECTION, IN THREE DIRECTIONS.** `recurrence_type` accepts
exactly `weekly` · `biweekly` · `monthly` · `custom` (`model.go:214-217`) and
`OccursOn` sends anything else to `default: return onBase` (`:305-309`). **A
wrong unit is not an error; it is a silent single occurrence.**

- **The week unit is NOT invention and its chip comes off.** Week-based
  recurrence strides `WeekLength() × recurrenceWeeks(...)`, so on a ten-day
  calendar `weekly` **means** every tenday. DAYCARD §5's "tenday is invention"
  line and the mockup's chip on that unit are **both wrong**, and the drawing
  lane's record is corrected so it does not re-teach it.
- **`year` IS invention and the drawing offers it UNCHIPPED.** There is no
  yearly type; it degrades silently. It does not ship at all — an unbacked unit
  in a picker is a trap, and the one thing worse than a missing option is one
  that quietly does nothing.
- **NEW, and this slice's own finding: `every N months` degrades the same way.**
  `OccursOn`'s monthly branch ignores `RecurrenceInterval` entirely, so `every 2
  months` would be stored, accepted, and then expanded **every** month. The
  interval control is therefore **absent** for the month unit rather than
  chipped: there is nothing there for a backend to add, the type simply has no
  interval.

**THE DAY-OF-WEEK MULTI-SELECT STAYS CHIPPED, and so would a single picker.**
`recurrence_day_of_week` exists as a column (migration 011) but **expansion is
base-anchored and ignores it** (`calendar/.ai.md:963`), and `eventEditorRecord`
does not carry it. DAYCARD §5's "Chronicle's model is `recurrence_type` plus a
single day-of-week" is wrong on main: the weekday is the base date's, full stop.

**THE UNIT LABEL IS DERIVED AND THE DERIVATION IS A NAMED DIVERGENCE.**
Chronicle's `Calendar` carries `Weekdays` and a `WeekLength()` and **no week
noun at all**, so "the calendar's own week noun" has nothing to read. The label
names the cycle's length (`10-day week`), which cannot lie about the stride the
way a bare "week" does on a ten-day calendar and cannot hardcode "tenday"
either. The week length itself is derived from the payload's own weekday names —
there is no literal `7`, no literal `10`, and no `% 7` in the module, the
template or the sheet.

**4. LOSSLESSNESS BECAME AN AUTHOR-vs-OMIT DISTINCTION.** The chrome now
*authors* recurrence, so the round trip is no longer pure. Three cases, all
pinned: **untouched** round-trips (a title-only save on an event whose stored
type Chronicle does not accept leaves that rule exactly as it found it),
**authored** sends the mapping, and **explicitly Once** clears the type and the
interval *together*. The clear uses `""` and not `null`, because
`service.UpdateEvent` guards the pointer siblings — a JSON `null` **preserves**
the column, which is exactly the half-state
`C-CAL-RECURRING-PARTIAL-STATE-CLEANUP` already had to clean up once. The
`RecurrenceEnd*` / `MaxOccurrences` fields cannot be reached from the client at
all, because neither shipped write binds them; that is carried, not papered over.

**5. [ER-3]: THE AUDIENCE ROSTER RIDES THE PAGE PAYLOAD, AND WHY THAT DOES NOT
BREACH [DC-1].** [DC-1]'s payload law governs the **card's per-event field set**
— "the Ledger row's own field set and NOTHING more". The roster is neither an
event nor a per-day field; it is **editor seed data on the same wrapper**,
exactly like the type palette, which rides here for the same shape of reason
([DC-8](c) option ii). `BenchScheduleReader.BenchRoster` is already called on
every Bench render, for every viewer, before any role is consulted — so this is
a re-serialisation of data the page already rendered and it costs **no new read
and no new route**.

The law is **re-stated rather than widened**: the wrapper's top-level inventory
is now pinned by name (`calendars` · `members`), the member row's own field set
beside it, and the identity pair asserted to be the RSVP panel's own index for
index — two surfaces that draw the same people must not draw them in two
colours. The gate is a **new explicit `DayCardMount.CanRestrict`**, never
`CanDelete` borrowed: both read `in.IsOwner` today, the routes behind them are
genuinely different, and two Owner floors that coincide today are one refactor
away from not.

**6. [ER-5]: THE EDITOR'S WIDTH IS MEASURED, AND THE MEASUREMENT DISAGREED WITH
THE PREDICTION.** The stills draw a two-column editor ~1008px wide. The sweep —
real page, real sheets, real module, 61 day cells × 11 viewport widths × 6
candidate widths, Ledger stacked and docked — found that **every candidate holds
0 px² of overlap**. The placement law is doing its job and the round-3 harm does
not recur at any width. What moves with width is how often the popover falls to
the signed desktop **sheet**, and at which viewports.

The shipped width is **760px**: the largest that never sheets at or above the
editor's own two-column breakpoint. Below that breakpoint `.ed-body` is one
column anyway and the narrow sheet is a treatment the shipped 420px box already
takes. **The divergence from the drawing is arithmetic, not taste**: `.cal-bench`
is 1180px at its measure, a docked Ledger column is 300px, and a 1008px editor
cannot sit beside that column at any viewport this product's measure produces.
`placeCard` was **not** re-opened — round 4's lesson was that the third geometry
was already one too many.

**6b. THE MEASUREMENT ABOVE WAS TRUE WHEN IT WAS TAKEN, FALSE WHEN THE MORPH
LANDED, AND IS TRUE AGAIN — and the two-stage gap is the durable lesson.** The
sweep was run at stage 2 and not re-run after stage 3 added the morph. From
stage 3 the same probe FAILED at every candidate width, up to 70,906 px² over a
docked Ledger, with the module's own occlusion report saying `clear=true`. Two
causes, and only one of them was the module's:

- **The module's.** `edClose` writes the reverse morph geometry as INLINE
  `inline-size`/`block-size`/`translate`/`opacity` — the card's rect — and
  `edHide`, the only thing that clears it, runs on a `--disc-close` timer that
  `edShow` cancels. Reopening inside that 160ms window handed `edPosition` the
  card's 420px for a box the sheet sizes at 760px: **placeCard reasoned about,
  and placed, a rectangle that does not render.** Fixed by clearing the morph
  geometry in `edShow` before the box is measured. `placeCard` still untouched.
- **The probe's.** It scored the popover's overlap and the SIGNED DESKTOP SHEET's
  overlap as the same number, and it measured every cell against the FIRST
  Ledger on the page rather than the one `ledgerRect(state.host)` actually
  excludes. Both are now separated, along the line `placeCard`'s own header
  already draws in words: *"A POPOVER over the Ledger is [DC-3]'s STOP-AND-FLAG
  and it speaks. A SHEET over the Ledger is [DC-3] bullet 4's own signed
  treatment… it is recorded and stays quiet."*

**The rule that comes out of it: a measurement is scoped to the commit it was
taken on.** A number quoted in a report is a claim about HEAD, and a slice that
changes the geometry has re-opened every geometric measurement it published.

**6c. A THIRD FINDING CAME OUT OF THE RE-MEASUREMENT, and it is a STOP-AND-FLAG
rather than a fix.** `placeCard` is handed ONE Ledger rect — the host Block's.
The Bench renders one Block per calendar and each carries its own Ledger, so a
card or editor opened from the real-world Block's grid can land on the PRIMARY
Block's Ledger. The probe now runs a **CARD control arm** beside the editor arm
so the ownership is not a matter of opinion: **the card hits it too** (2,221 px²
at viewport 720), and the card's box and `placeCard` are byte-unchanged by R2b.
The editor, being larger, reaches 72,600 px² — 100% of a docked 300×242 Ledger.
Widening the exclusion means re-opening `placeCard`, which [ER-5] SIGNED makes a
STOP-AND-FLAG rather than an edit, so it is measured, reported on every probe
run whether it fires or not, and booked as `C-CALV4-CARD-CROSSBLOCK-LEDGER`.

**7. [ER-2]: THE PLAYER'S CARD DOES NOT GROW A DETAIL PANEL.** The stills'
`.card-x` block prints an event's title, description prose, tie pill and date
pill. [DC-1] forbids description on the payload and §2 rule 3 forbids the card
rendering a field the Ledger row does not print. The 2026-08-01 signature
promotes the stills to a gate **"for the chrome stage"** — the *look* — and does
not restore the file's authority over *which fields exist*, which [DC-5] part 1
removed and nobody overturned. **The cheap-looking workaround is the expensive
one**: fetching the description on demand from the Player-floor
`GET …/events/:eid` needs no route and no payload change, and is refused — it
turns a read-only card into an authoring-adjacent fetch surface and puts a round
trip inside a 200ms open. If the operator wants the panel it is an amendment to
[DC-1] with its own signature: cheap to ask for, impossible to take by inference.

**8. [ER-8]: A DRAG PREVIEW THAT MAY NOT MARK A CELL.** The obvious
implementation adds a class to the cells under the pointer, and those cells are
inside `.cal-block-host` where §1 rule 1 forbids it byte for byte. [DC-11] term
2's "its own drag-highlight rules" reads like a licence to mark cells and is not
one. The span is therefore a **page-level overlay** the module creates beside the
card, positioned from the run's own rects, **one box per contiguous row** — a
union across two weeks would paint days that are not in the run, and a preview
that lies about what it is about to create is worse than no preview. It declares
**no motion**: a highlight that follows the pointer is a position update, not a
transition.

**9. A GUARD THAT CANNOT SEE THE THING IT CLAIMS TO CATCH.** The morph's close
must remove `.dcopen` **before** writing the reverse geometry, or leaving takes
exactly as long as arriving. Mutating that order left **every end-state
assertion green** — it is an ordering claim inside one task. The fixture's DOM
stub therefore grew an **operation log**, and the ordering is now a real
assertion rather than a comment. *A guard nudged until green stops proving
anything; a guard that was never able to see its subject never proved anything
at all.*

**10. THE SIGNED MORPH SHIPPED INERT ON OPEN, AND THE ORDERING RULE IS NOW
WRITTEN DOWN.** SEED → FLUSH → CLASS → FLUSH → FINAL WRITE. `edOpen` wrote the
seeded start geometry and added `.edmorph` in the SAME style recalc; per CSS
Transitions a transition starts from the **after-change** style, so that one
recalc started 160ms transitions running AWAY from the resting box and TOWARD
the seed. The seed never became the settled before-change style,
`getComputedStyle` answered the resting box, and the final write saw nothing to
change. The editor POPPED IN at full size and animated only on the way out —
`edClose` was correct by accident of shape, because it adds the class and
flushes BEFORE any value change. The rule is browser-general: it follows from
the after-change-style rule, not from anything Chromium does on its own.

**11. A GUARD THAT ASSERTS A STATE MACHINE HAS NOT ASSERTED AN ANIMATION.**
Four guards covered this morph — a layout-less DOM stub reading end-state inline
styles, a MutationObserver over the style attribute, and two CSS guards reading
the sheet — and **all four stayed green with the morph completely dead**. Two of
them said so in their own headers ("by the time this line runs the module has
already written the END state"; "DOES NOT PROVE that the compositor
interpolated") and the concession was read as a scope note rather than as a
hole. *When a guard's own comment tells you what it cannot see, that sentence is
the specification for the guard you are missing.* The answer is a browser-level,
rAF-sampled geometry probe in both directions, and it is deliberately NOT
env-gated.

**12. A RIG'S LIMIT IS A CLAIM, NOT A FACT, AND IT GETS CHECKED LIKE ONE.** The
evidence carried "this environment cannot photograph the morph… this is the
rig's limit, not the morph's" onto five images and into two documents. The
underlying measurement was true — under `--virtual-time-budget` the document's
rendering lifecycle is not run, so no transition is ever created to park — and
the conclusion drawn from it was false twice over: the environment can
photograph it (serve the page, hold the `load` event with a slow subresource,
freeze `setTimeout` at the click), and the thing it was failing to photograph
really was not moving. **A negative result about a tool is evidence about the
tool.** Reaching for a different tool is cheaper than the three rounds spent
explaining the first one.

**13. A DEAD ANIMATION HIDES EVERYTHING DOWNSTREAM OF IT.** The moment the morph
ran, three synchronous rigs turned out to have been measuring the box MID-FLIGHT
— the [ER-5] sweep read the card's 340px at every candidate width, the floors
probe found the date picker's day buttons at 15px under a 24px floor — and the
[ER-5] probe's own stale-geometry guard then caught a **pre-existing** placement
defect it had never been able to see: `applyPlacement` writes `.dcsheet` and its
`style.width` AFTER `edPosition` measures, and nothing cleared them, so a reopen
after a sheeted placement handed the placement law the full viewport width for a
box about to render at `--de-w`. Same shape as the round-3 blocker, arriving
through a class instead of an inline style. `edShow` clears both, through the
same writer `applyPlacement` uses; `placeCard` is still not touched. *A rig that
measures a surface no user sees will report a spotless result about it.*

### References

- Master plan: cordinator `plans/2026-07-26-calendar-v4-remodel-master-plan.md`
- Canon: `decisions/2026-07-26-calendar-v4-canon-amendments.md` (A1–A8, B1–B4)
- Pins: `decisions/2026-07-27-calv4-tie-mark-emission.md` (r51) ·
  `decisions/2026-07-28-calv4-ledger-p6-pin-amendment.md` (r52) ·
  `decisions/2026-07-28-calv4-shelf-pin-amendment.md` (r53, the Almanac register)
- Rulings: `2026-07-28-calv4-def-and-zone-chips-ruling.md` ·
  `2026-07-27-motion-policy.md` · `2026-07-27-needs-backend-audience.md` ·
  `2026-07-27-calendar-scope-and-roles.md`
- Wave reports: `reports/chronicle/2026-07-28-C-CALV4-{SEAM-P5,HOST-P3,BENCH-P4,
  LEDGER-P6,SHELF-P7}.md`
- Contract: `mockups/calendar-v4.html` + `mockups/renders/v4-*.png`

---

## ADR-049: "no authenticated user" and "trusted system caller" are two states, not one empty string

**Date:** 2026-08-07 · **Status:** Accepted · **Amends:** C-CAL-DASHBOARD-W5a
(the calendar visibility gate) and C-PERM-ANON-IDENTITY · **Consistent with:**
C-CALV4-V2SUNSET [VS-15] · **Origin:** C-AUTHZ-EMPTY-USERID, reproduced in
`reports/chronicle/2026-08-07-C-SWEEP-R3.md` §3 row 8.

### Context

The calendar and timeline visibility filters short-circuited on
`permissions.CanSeeDmOnly(role) || userID == ""` (calendar) and
`!CanSeeDmOnly(role) && userID != ""` (timeline). The empty user id was
documented in both places as "the system context" — a trusted in-process
caller with no request behind it.

**An anonymous HTTP request carries exactly that value.** `auth.GetUserID(c)`
returns `""` when there is no session, and on a PUBLIC campaign
`AllowPublicCampaignAccess` + `RequireViewAccess` let that request reach the
service. So the most privileged branch in the filter was the branch logged-out
internet traffic took: `dm_only` calendars, `dm_only` timelines and
per-user-restricted events and event links were served to a visitor who never
logged in — content a logged-in Player on the same campaign is correctly
denied.

This was not a missing check at a call site. It was **one representation
standing for two different states**, so every call site was correct and the
system was still wrong. It is also about to widen: R2-4 (C-CALV4-V2SUNSET)
moves `GET /apps/calendar` onto the public group.

### Decision

**The two states get two representations, and the trusted one is unforgeable
from request data.**

`internal/permissions/viewer.go` adds `Viewer`, with an **unexported** `system`
bool and exactly two constructors:

- `RequestViewer(role, userID)` — anything that came in over HTTP. An empty
  `userID` means ANONYMOUS: no user. It cannot produce a system viewer.
- `SystemViewer(role)` — a trusted in-process caller that has no request
  identity, stated at the call site.

Every visibility filter asks `Viewer.SkipsPerUserRules()` (`system ||
CanSeeDmOnly(role)`) instead of testing the user id itself. **An anonymous
viewer is neither**, so it falls to the least-privileged path by construction
rather than by each call site remembering.

Consistent with **[VS-15]**: an empty user id is an ABSENT per-user layer —
never a sentinel, never a lookup key, never substituted with a synthesised
identity (no `"anonymous"` user, no per-IP key).

### Consequences

- **Two trusted callers now say so.** `timeline_widget_type.go`'s
  create-or-pick picker (Scribe-gated at the route) and the campaign timeline
  export adapter pass `permissions.SystemViewer`. **Their shipped behaviour is
  unchanged** — the picker still lists allow-list-restricted timelines — which
  is the point: the trust was real, only its representation was shared with
  anonymous traffic.
- **Two green tests that pinned the bug as intended were INVERTED, not
  deleted**, each with a comment naming this ADR, and each kept a row asserting
  that the SYSTEM path still bypasses so the pair proves the distinction:
  `calendar_visibility_w5a_test.go`'s `TestCalendarVisibleTo` and
  `entity_ties_test.go`'s `…_OwnerUnfilteredAnonymousFiltered`.
- `TimelineService.ListTimelines` / `ListTimelinesForCalendar` /
  `ListTimelineEvents` take a `permissions.Viewer` instead of `(role, userID)`;
  the calendar's own filters are package-private and take one too, with the
  exported service methods building a `RequestViewer` at their boundary.
- **Not taken here:** a route-level middleware assertion that an anonymous
  request can never reach a filter with a trusted identity ([EU-5]). It touches
  every public-group route and is booked, not shipped.

### References

- Dispatch: `dispatches/chronicle/C-AUTHZ-EMPTY-USERID.md` ([EU-1] = explicit
  flag, [EU-2] = the picker becomes a declared system caller, [EU-3] = amend the
  pinned rows, [EU-4] = calendar + timeline in one slice, [EU-5] = booked)
- Pins: `internal/plugins/calendar/anonymous_visibility_test.go` ·
  `internal/plugins/timeline/anonymous_visibility_test.go`

---

## ADR-050: An immutable plugin migration is repaired by a reconciler, and a half-applied one resumes instead of replaying

**Date:** 2026-08-07 · **Status:** Accepted · **Extends:** ADR-044 / ADR-045
(migration robustness, the `000030` incident) and ADR-028/030 (plugin
migrations) · **Origin:** `C-PLUGIN-MIGRATION-RUNNER`, two defects reproduced in
`reports/chronicle/2026-08-07-C-SWEEP-R3.md` and closed by C-SWEEP-R4
stages 11–12.

### Context

Two failures in the same runner, both terminal, both invisible to CI.

1. **`foundry_vtt` migration 001 crashed on every brand-new database.** It is a
   consolidation migration — `RENAME TABLE foundry_module_campaign_tokens TO
   foundry_vtt_campaign_tokens` — and the plugin that created the source table
   was deleted in C-FMC-5c. A fresh install hit `Error 1146`, the plugin was
   marked DEGRADED, and it could never self-heal, because
   `runSinglePluginMigrations` returns on the first failed migration: no later
   migration for that plugin is reachable, so a fresh-DB-safe `002` would never
   run. `PreMigrationCheck` (PR #507) does not cover it — it refuses only when
   `foundry_module_versions` exists *and has rows*, and on a fresh database the
   table does not exist at all.
2. **A plugin migration that failed on its second statement was unrecoverable.**
   `execPluginMigration` splits on semicolons, runs statement by statement on a
   plain `*sql.DB`, and writes the `plugin_schema_versions` row only after the
   LAST statement succeeds. A mid-migration failure therefore leaves the earlier
   statements' effects in the database and *no record that anything happened*.
   The next boot replays from statement one and dies on "duplicate column name",
   because most plugin ALTERs are not idempotent — so the operator sees an
   artefact of the retry rather than the real cause, and fixing the real cause
   cannot help.

Both were invisible for the same reason: **nothing in CI ever migrated an empty
database.** `tools/restore-drill.sh` loads a dump of an already-migrated one and
every integration test assumes `make migrate-up` has run.

### Decision

**1. An immutable migration that cannot run is repaired by a Go-side reconciler
plus a new append-only migration — never by editing the old one.**
`foundry_vtt.ReconcileConsolidationState` records 001 as applied on any database
where its RENAME has no source table; new migration `002_ensure_campaign_tokens`
states the post-consolidation shape in idempotent DDL. 001 is untouched, so
`tools/check-migration-immutability.sh` and the `migrate_test.go:402` grandfather
stand exactly as they were, and a database that still HAS the predecessor table
is left alone — 001 runs there for real and carries its live token rows across.
Fresh install, completed upgrade, and an upgrade that died between 001's two
statements all converge on one schema.

This is the existing house rule ("one-time data fixes go in a reconciler, never a
migration") extended to its schema-bootstrap twin, and it deliberately rejects the
two alternatives: editing 001 is forbidden outright, and relaxing
`runSinglePluginMigrations` so an unapplied earlier version is not a hard stop
would change failure semantics for all nine registered plugins in order to fix
one.

**2. A plugin migration gets a pre-flight applicability check and partial-progress
recording. It does NOT get a transaction, and the errors say so.** MariaDB has no
transactional DDL: every CREATE / ALTER / DROP / RENAME commits implicitly and
cannot be rolled back. `internal/database/plugin_migration_safety.go` therefore
buys two specific things and claims nothing more:

- **Pre-flight.** Before the first statement runs, every statement is validated
  against the schema catalogue *as it will stand at that point in the migration*
  (the simulation moves forward, so the ordinary CREATE-then-ALTER shape is not
  falsely refused). If any statement cannot possibly succeed, the migration
  aborts having executed NOTHING — converting "half-applied and unrecoverable"
  into "nothing applied, actionable error", which is the closest thing to
  atomicity available here. Table granularity: it catches the failures that
  produce unrecoverable states, not every possible SQL error.
- **Partial-progress recording** in a new runtime table
  `plugin_migration_progress`. When a statement fails anyway, the number that DID
  apply is recorded first, so the next boot resumes after them. Keyed by a sha256
  of the migration text and honoured **only** on a byte-identical match —
  migrations are immutable so this should never diverge, but a resume that skipped
  the wrong statements would be far worse than the crash-loop it replaces, so it
  is checked rather than assumed.

**3. Every uncertain answer degrades to the pre-existing behaviour.** The
pre-flight **fails open** when `information_schema` cannot be read, and an
unmatched or missing progress row replays from zero. Refusing every plugin
migration over a metadata hiccup would be worse than the bug being guarded, and
a false abort of a real migration is the one outcome worse than the original
defect.

**4. The guard is a fresh-DB replay, and a SKIP is a failure.** `make test-freshdb`
/ `cmd/server/freshdb_migration_test.go` replay the real bootstrap against
genuinely empty MariaDB schemas — one from zero, one from the pre-consolidation
shape — wired into CI as its own `Fresh-DB Migration Replay` job. The job greps
for PASS on each named test, so a test that merely *skips* (the way this class
hid for as long as it did) fails the job. The three DB-backed
`TestPluginMigration_*` recovery regressions run in the same job and are named in
the same assertion. `TestFreshDatabase_EveryPluginSchemaApplies` continuing to
pass is what rules out the pre-flight falsely refusing a real migration.

### Consequences

- A plugin whose old migration is unrunnable now has a sanctioned repair that
  does not touch the immutability guard. The cost is that the canonical schema is
  stated in two places — the original migration and the idempotent `002` — which
  is the price of append-only.
- Boot-time recovery semantics changed for **all** plugins, not just
  `foundry_vtt`: a failing migration may now abort earlier (pre-flight) or resume
  later (progress). Both directions were chosen to be strictly safer than replay,
  and both fall back to replay when unsure.
- `plugin_migration_progress` is a runtime table created by the runner itself,
  not by a migration — it must exist before any plugin migration runs, so it
  cannot be one.
- The non-idempotent-DDL CI ratchet the original booking imagined was **not**
  built. All nine existing offenders are immutable, so a ratchet would have
  needed a grandfather allowlist and a house-law amendment; the pre-flight
  addresses the harm those offenders cause without requiring either.

### References

- `internal/database/plugin_migration_safety.go` (package doc states the
  non-transaction claim in full) · `internal/database/plugin_schema.go`
- `internal/plugins/foundry_vtt/reconcile_consolidation.go` +
  `migrations/002_ensure_campaign_tokens.{up,down}.sql`
- Pins: `cmd/server/freshdb_migration_test.go` ·
  `internal/database/plugin_migration_recovery_test.go` ·
  `internal/database/plugin_migration_safety_test.go` ·
  `internal/plugins/foundry_vtt/reconcile_consolidation_test.go`
- Booking: `.ai/todo.md` §"Booked by sweep R3" D · dispatch
  `dispatches/chronicle/C-PLUGIN-MIGRATION-RUNNER.md`
