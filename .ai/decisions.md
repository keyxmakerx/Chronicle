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

Three things in the panel genuinely are unbacked and keep their state, all
GM-tier so a player receives none of them: the **propose** write
(`routes_snapshot.txt` carries no propose-from-window path), the
**reminder/nudge** endpoint (the fan-out fires only on the `collect_rsvps`
OFF→ON transition; booked as C-CALV4-RSVP-P8B, "the asking email"), and a
server-side **recommender** — which WG-3 retires by *deriving* the window
arithmetically from the overlay's own per-hour free counts rather than storing
it, under a permanent `derived · not stored` chip.

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

### Sections inside this ADR rather than beside it

W-F's layer switchboard and preference store become sections HERE when they
land. calendar-v4 is one architecture decision; competing ADRs for its later
waves would fragment the rationale that a future re-litigation needs in one
place. W-E followed this rule: its Almanac decisions are §10-§14 above rather
than an ADR-049.

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
