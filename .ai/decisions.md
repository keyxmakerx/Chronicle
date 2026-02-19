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
