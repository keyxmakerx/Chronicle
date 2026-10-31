# Coding Conventions

## Handler Pattern

Handlers are thin: bind, call service, render. No business logic, repo calls, SQL or side effects.

```go
func (h *CampaignHandler) Create(c echo.Context) error {
    var req CreateCampaignRequest
    if err := c.Bind(&req); err != nil { return apperror.NewBadRequest("invalid request body") }
    if err := c.Validate(req); err != nil { return err }
    campaign, err := h.service.Create(c.Request().Context(), middleware.GetUserID(c), req.ToInput())
    if err != nil { return err }
    if middleware.IsHTMX(c) { return middleware.Render(c, http.StatusCreated, templates.CampaignCard(campaign)) }
    return middleware.Render(c, http.StatusCreated, templates.CampaignShow(campaign))
}
```

## Service Pattern

Services own all business logic, take and return domain types, and never import `echo` or HTTP types.

```go
type CampaignService interface {
    Create(ctx context.Context, userID string, input CreateCampaignInput) (*Campaign, error)
}
```

## Repository Pattern

Repositories own all SQL, one per aggregate root, hand-written with `database/sql` + `go-sql-driver/mysql`, `?` placeholders.

```go
func (r *campaignRepository) FindByID(ctx context.Context, id string) (*Campaign, error) {
    var c Campaign
    err := r.db.QueryRowContext(ctx,
        `SELECT id, name FROM campaigns WHERE id = ?`, id).Scan(&c.ID, &c.Name)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, apperror.NewNotFound("campaign not found")
    }
    return &c, err
}
```

## Templ Component Pattern

One component per file; file name matches component name; props as function args.

```go
templ CampaignCard(campaign *model.Campaign) {
    <div class="card" id={ fmt.Sprintf("campaign-%s", campaign.ID) }>{ campaign.Name }</div>
}
```

## HTMX Fragment Detection

Use `middleware.IsHTMX(c)`, never a local copy: it is true for `HX-Request: true` without `HX-Boosted`, so boosted navigation gets a full page. `middleware.Render(c, status, component)` sets Content-Type and writes the component.

## Error Handling

Domain errors from `internal/apperror/`. Never expose raw DB errors.

```go
apperror.NewNotFound("campaign not found")
apperror.NewBadRequest("name is required")
apperror.NewForbidden("you do not own this campaign")
apperror.NewInternal("unexpected error")     // logs real error, returns generic
apperror.NewConflict("slug already exists")
apperror.NewUnauthorized("invalid session")
```

## Partial-Update Endpoints (nil-preserve semantics)

The contract, everywhere: an absent key preserves, an explicit `null` clears, a present value replaces.

A plain pointer collapses "absent" and "null" at JSON bind, so use `internal/patch`'s `Field[T]`, which records presence in `UnmarshalJSON` (called only for keys the body carries).

```go
stored.Summary = input.Summary.Ptr(stored.Summary) // nullable column
stored.Status  = input.Status.Val(stored.Status)   // NOT NULL column
```

- `Ptr(cur)` (nullable field): absent → `cur`, null → `nil` (cleared), value → pointer to it.
- `Val(cur)` (non-nullable field): absent → `cur`, value → the value; null also preserves, since a NOT NULL column has no cleared state and writing its zero would lose data.
- Validators read the merged value, not the raw input: an absent name is not an empty name.
- A refused write drops to absent, not null: denying a field is not authority to erase it.
- For a broad surface (many optional fields), load-merge-write beats a wall of nil-guards.
- Every governed endpoint is pinned in all three directions and documented where it is described (`API-CONTRACT.md` for the Foundry wire; the plugin's `.ai.md` for web routes).
- `internal/patch/partial_update_contract_test.go`: every field of a contract-governed `Update*Input` must be `patch.Field[T]`, a pointer, a map or a slice. The inventory is frozen; a new input is classified `contractGoverned` (partial, presence-aware), `fullReplaceByDesign` (a form that posts every field, or a one-field body; the reason is a fact about the callers) or `notYetSwept` (unaudited; should stay empty).

## Create Endpoints: visibility comes from the campaign

An absent `is_private` on create defers to the campaign's `DefaultVisibility`; a value-typed `bool` cannot tell "omitted" from "false" and would answer public. Resolve it through `campaigns.CampaignSettings.ResolveNewEntityPrivacy`, the single implementation.

```go
input := entities.CreateEntityInput{
    Name:      req.Name,
    IsPrivate: settings.ResolveNewEntityPrivacy(req.IsPrivate), // req.IsPrivate is patch.Field[bool]
}
```

- Absent and explicit false stay different; the campaign default fills only the first. A path with no client input (the bestiary import) is always absent and calls `DefaultsToPrivate()`.
- An unreadable campaign fails closed: private, with a loud log. Read the setting at most once per request (a sync batch can carry 2000 changes).

Pinned by `default_visibility_test.go` (campaigns), `default_visibility_create_test.go` (entities), `create_default_visibility_test.go` (syncapi), `bestiary_import_visibility_test.go` (app).

## Tests

Table-driven (`tests := []struct{name string; ...}` with `t.Run`), with interfaces at every service and repository boundary. DB-backed Go tests get a throwaway migrated schema from `dbtest.Open` (`internal/database/dbtest`), which reads `CHRONICLE_TEST_DB_DSN` and drops the schema afterwards (`make test-int-local` sets the DSN).

## Widget Registration (Frontend JS)

A widget mounts to `data-widget="<slug>"`, fetches its own data and renders itself: `Chronicle.register('editor', { init(el, config) {}, destroy(el) {} })`.

## File Naming

| Context | Convention | Example |
|---------|-----------|---------|
| Go source | `snake_case.go` | `campaign_handler.go` |
| Templ | `snake_case.templ` | `campaign_card.templ` |
| Tests | `<file>_test.go` (colocated) | `campaign_service_test.go` |
| Migrations | `NNNNNN_description.up.sql` | `000001_create_users.up.sql` |
| JS widgets | `snake_case.js` | `editor.js` |
| AI docs | `.ai.md` (in tier root) | `internal/plugins/auth/.ai.md` |

## Comment Conventions

Every package gets a doc comment (`// Package auth handles ...`); every exported type one starting with its name. Non-obvious logic gets a short why, not a restatement of what:

```go
// Check ownership before cascade delete because MariaDB FK constraints
// alone don't prevent cross-user deletion via direct ID manipulation.
if campaign.CreatedBy != userID {
    return apperror.NewForbidden("you do not own this campaign")
}
```

A comment may point at one stable place: an ADR (`ADR-058`), a test name, or an issue (`#613`, `owner/repo#12` across repos).

Never: how a bug was found, "before this fix"/"this used to", task or dispatch IDs (`C-…`), `cordinator/` paths, "this PR", dates as provenance, `file:line` pointers. Those belong in the commit message and PR description. A comment describing code that no longer exists is a bug.

Avoid restating the code, unexplained commented-out code, and the obvious (`// Delete deletes a campaign`).

`tools/check-comment-clutter.sh` (CI) fails a branch whose new comments carry any of the above, a pointer to `.ai/todo.md`, or a date outside tests. Existing comments are grandfathered; a line that needs one ends with `clutter-ok`.

**TODO** names its issue (open one first): `// TODO(#613): stop echoing untouched fields back on update`, or cross-repo `// TODO(keyxmakerx/Chronicle-Foundry-Module#95): ...`.

## Schema, Migrations and Permissions

### Two-Tier Schema System (ADR-028)

**Core** (`db/migrations/`): core tables, run by golang-migrate on startup; failure is fatal. **Plugin** (`internal/plugins/<name>/migrations/`): numbered migrations embedded in the binary via `embed.FS` (ADR-030, in `embed.go`), run by `RunPluginMigrations()` after core; failure disables that plugin and the app keeps serving. `registeredPlugins()` is in `cmd/server/plugin_schemas.go`, not the database package, to avoid import cycles.

### Migration Safety Rules

1. **ENUM values**: add a new value via `ALTER TABLE` in the same or an earlier migration before an INSERT/UPDATE uses it.
2. **Seed data conflicts**: check whether seed data for a slug/key already exists; use UPDATE or `INSERT ... ON DUPLICATE KEY UPDATE`, not a bare INSERT.
3. **Down migrations**: if up UPDATEs a row, down reverts it, not DELETEs it; DELETE only rows the same migration INSERTed.
4. **ENUM in down migrations**: revert rows using an added value before removing it from the ENUM.
5. `internal/database/migrate_test.go` validates ENUM values in migration SQL; update its valid sets for new ones.
6. **Plugin tables** belong in `internal/plugins/<name>/migrations/`, not `db/migrations/` (ADR-028). A new plugin with migrations needs an `embed.go` and its own file in `cmd/server` whose `init` calls `addPluginSchema(rank, slug, fs)`; rank sets run order, so a plugin whose tables point at another plugin's ranks after it.
7. **Layering**: core migrations reference only core schema; plugin migrations own their tables and any backfills touching them. Core runs first, so a core migration referencing a plugin table (`api_keys`, `maps`, `calendars`, ...) crashes a fresh DB. Split a cross-layer fix: core part in `db/migrations/`, plugin part in the plugin's `migrations/`.
8. **Idempotent DDL**: `ADD COLUMN`/`CREATE TABLE` use `IF NOT EXISTS`; `DROP COLUMN`/`DROP TABLE`/`DROP INDEX` use `IF EXISTS`, since a partially applied migration re-runs. `TestMigrations_IdempotentDDL` enforces it on new migrations; historical files are grandfathered (rule 9).
9. **Append-only**: never delete, edit or renumber a migration a live database may have applied. golang-migrate needs a file for every version up to the DB's recorded one, so removing one crash-loops boot (ADR-044/045). `tools/check-migration-immutability.sh` (CI) fails a PR that deletes or edits an existing migration.
10. **Schema-only**: a one-time data fix is an idempotent reconciler (an `EnsureX`/`MergeX` service method run from a boot backfill, an addon-enable hook or an owner-triggered `SetupProvider`; see `app/setup_pc.go`), never a migration.
11. **Boot contract** (`database.MigrateWithBackup`, ADR-045): the pre-migration backup runs only when a migration is pending; a DB ahead of the build warns and boots; a dirty DB fails fast with restore guidance; `fatalBoot` backs off (`BOOT_FAIL_BACKOFF`, default 45s). `ExpectedCoreMigrationVersion` (`migrate_state.go`) must equal the highest core migration (`TestExpectedCoreMigrationVersion_MatchesMax`).
12. **Unique numbers**: one version per directory. `tools/check-migration-numbers.sh` (CI) fails a migration sharing a number with one in its tree or the base branch's tip. When two open PRs collide, the second to merge renumbers its own unapplied files and `ExpectedCoreMigrationVersion`.

### Permission Model

`internal/permissions` holds the role constants (`RoleOwner`, `RoleScribe`, `RolePlayer`) and `CanSeeDmOnly` for services and repos that cannot import `campaigns` (circular).

**Role hierarchy:** Admin (site) > Owner (campaign) > Scribe > Player > Public

| Resource | View | Create | Edit | Delete | Toggle dm_only |
|----------|------|--------|------|--------|----------------|
| Campaign | Player | (site) | Owner | Owner | -- |
| Entity types | Player | Owner | Owner | Owner | -- |
| Entities | Player* | Scribe | Scribe | Owner | Owner |
| Entity permissions | Owner | Owner | Owner | Owner | -- |
| Tags | Player | Scribe | Scribe | Scribe | Owner |
| Relations | Player | Scribe | Scribe | Scribe | Owner |
| Calendar | Player | Owner | Owner | Owner | -- |
| Calendar events | Player* | Scribe | Scribe | Owner | Owner |
| Timeline | Player | Owner | Owner | Owner | Owner |
| Timeline events | Player* | Scribe | Scribe | Scribe | Owner |
| Maps | Player | Owner | Owner | Owner | -- |
| Markers | Player* | Scribe | Scribe | Owner | Owner |
| Drawings | Player* | Scribe | Scribe | Owner | -- |
| Tokens | Player* | Scribe | Scribe | Owner | -- |
| Layers | Player | Owner | Owner | Owner | -- |
| Fog of war | Owner | Owner | -- | Owner | -- |
| Sessions | Player | Scribe | Scribe | Owner | -- |
| Notes | Player+ | Player+ | Player+ | Player+ | -- |
| Groups | Owner | Owner | Owner | Owner | -- |

\* Player sees content unless dm_only or custom permissions restrict it.
\+ Notes: own notes, plus notes shared with the party, with you by name, or (for the Owner and members with DM access) with the GM. A private note is private from the GM too.

**dm_only:** Owners, and co-DMs granted through `CampaignSettings.DmGrantIDs`, see and author dm_only content: `permissions.CanSeeDmOnly(role, granted)` to read, `CampaignContext.CanAuthorDmOnly()` to write. Everyone else never sees it.

## Formatting: no `gofmt -w` over globs

The tree is not plain-`gofmt` clean, so `gofmt -w` over a package or glob reformats unrelated files. Edit in place; if you must format, scope it to files you authored.

## CI tenet-enforcement guards

`make verify` runs the local CI sequence, guards included; `.github/workflows/ci.yml` and the Makefile `verify` target list them all. They enforce the binding tenets (T-B1 security, T-B2 plugin isolation, T-O2/T-O3 verification) defined in Cordinator's `decisions/2026-05-21-core-tenets.md`. The table covers the ones that need explanation.

| Guard | File | Mode | Enforces |
|---|---|---|---|
| Plugin isolation grep | `tools/check-plugin-isolation.sh` | diff-scoped FAIL | T-B2: no new `foundry-vtt`/`foundry-module`/`foundry_vtt` literals outside `internal/plugins/foundry_vtt/*` |
| Motion discipline | `tools/check-v2-motion-discipline.sh` | diff-scoped FAIL | No new `transition: all`/`transition-all` in calendar/timeline/ai_workspace/campaigns sources; opt out per line with `/* OK exempt: … */` |
| Wire-contract conformance | `internal/wire/wire_contract_test.go` + `routes_snapshot.txt` | FAIL (snapshot) | T-O2: every route the real `RegisterRoutes` registers is in the curated snapshot with its resolved path, auth class, handler and middleware chain |
| Foundry public rate-limit pin | `internal/wire/foundry_public_ratelimit_test.go` | FAIL | T-B1: two AST assertions pin the Foundry public manifest rate-limit wiring (`g.Use(rateLimit)` in `foundry_vtt.RegisterPublicRoutes`) and call site (`middleware.RateLimit(...)` in `app.RegisterRoutes`) |
| Sanitize-on-write invariant | `internal/sanitize/invariant_test.go` + snapshot | FAIL (snapshot + invariant) | T-B1: every `internal/plugins/*/service.go` (+ widgets) declaring HTML-typed inputs must call `sanitize.HTML` |
| CSS emissions | `tools/check-css-emissions.sh` + `tools/css-emissions.txt` | FAIL (CI and `make verify`) | Every selector listed in `css-emissions.txt` survives into the built stylesheet; Tailwind's purge drops rules it thinks are unused (JS-built class names) with no build error |
| Page-script ratchet | `tools/check-page-scripts.sh` + `tools/page-script-allowlist.txt` | FAIL (whole tree) | A page templ's `<script src>` is stripped on hx-boosted nav; scripts go through `pluginBodyScripts` in `internal/app/routes.go`. The allowlist may only shrink |
| Dead widget mount | `tools/check-widget-mounts.sh` + `tools/widget-mount-allowlist.txt` | FAIL (whole tree) | A widget JS file must be listed in a plugin's `Widgets` registration (or `base.templ`), or its `data-widget` div renders empty |
| Customize discipline | `tools/check-customize-discipline.sh` | FAIL (fixed file list) | Customize > Appearance: no JS-built topbar controls, no `setTimeout` page reloads, no hand-built JSON strings in templ |
| Templ packages | `tools/check-templ-packages.sh` | FAIL | Every directory of `.templ` files has a hand-written `.go` file (a `doc.go` is enough), so dependency updates on a checkout without generated code still resolve |
| Decision-citations | `tools/check-decision-citations.sh` | WARN (`continue-on-error`) | T-O3: warns about a Cordinator decision nothing cites; no-op without a Cordinator checkout |

### Pre-merge rules that no guard checks

Check by hand:

- A PR adding an `hx-get` fragment endpoint lists every consumer (each templ file or fetch call embedding it); one deleting an endpoint shows every consumer moved.
- A PR changing how a URL is served checks that on-disk artifacts (extracted zips, cached files, generated manifests) carry the new URL, not just runtime rewriting.
- Click handlers inside HTMX fragments use the inline-IIFE `onclick` pattern (Go-side builders like `foundry_vtt/onclick_handlers.go`), never `templ script` helpers or delegated `document.addEventListener`: a templ script tag is not reliably run before a swapped-in button is clicked. `onclick_handlers_test.go` enforces this for `foundry_vtt` only.
- Keep `foundry_vtt/errors.go`, its `.ai.md` catalog table and `error-catalog.json` in step (`errors_test.go`).

### Extending the guards

- **Wire-contract snapshot:** on an intentional route or middleware change, run `UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...` (after `templ generate`) and commit the regenerated `internal/wire/routes_snapshot.txt` with an explanation, especially for the auth surfaces below. The test builds the real `App` against an unreachable database and in-memory Redis, runs the real `RegisterRoutes`, and records each route through Echo's `OnAddRouteHandler`: resolved path, method, auth class, handler and middleware chain in execution order, including routes added by loops. The auth class comes from the chain's middleware names (`authClassRules`), so a route losing `RequireAuth` shows as class `none`; a new auth middleware needs a rule there. Limits: middleware is identified by function name, not arguments (`RequireRole(RoleOwner)` becoming `RequireRole(RoleScribe)` does not change the line); global `Echo.Use` middleware is not in any chain; routes registered during `App` construction (the `/static` mount) are not recorded; inline closures are named after their builder. `foundry_public_ratelimit_test.go` pins the Foundry rate limit by AST for what the snapshot cannot see.
- **Plugin-isolation guard:** checks `foundry-vtt` strings only; other plugin names are not checked.
- **Sanitize-invariant snapshot:** on a new plugin `service.go` or added/removed HTML-typed inputs, run `UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...` and commit the snapshot; cite the audit in the PR for a new sanitize surface.

## Cross-plugin import discipline

Cross-plugin communication always goes through an exported Go interface (a service or middleware) on the providing plugin. Importing another plugin's repository, store, internal struct or `_test.go` helpers is a layering violation.

```go
// In plugin-A: define the interface you need from plugin-B; plugin-B implements
// it and exposes it via NewService(...) campaigns.CampaignService; plugin-A's
// wiring accepts the interface, not the concrete type.
type CampaignService interface {
    Get(ctx context.Context, id string) (*Campaign, error)
}
```

- A plugin exposes an interface, or a middleware constructor returning `echo.MiddlewareFunc`, never a concrete type.
- `internal/app/routes.go` is the only package that imports every plugin; new plugins register there.
- `internal/websocket/{auth,client,hub}.go` import `foundry_vtt` only for the `ModuleSource` const; that is allowed.

**Enforcement:** `tools/check-plugin-isolation.sh` (diff-scoped) catches new `foundry-vtt`/`foundry-module` literals outside `internal/plugins/foundry_vtt/`; the wire-contract test catches routes outside the snapshot; review catches imports of `internal/plugins/<X>/<subpkg>` (e.g. `.../repository`), the usual sign the interface was bypassed.

## Security

Security is the highest-priority tenet (T-B1). A PR touching an auth surface, sanitization site, signed URL or SQL identifier interpolation states the security implication in its description.

### "The route is authorized" is not "the object is authorized"

Campaign middleware proves the caller belongs to the campaign in the URL, nothing about an object a handler loads by its own id. For such an object: resolve it, 404 if its campaign is not the route's, run the plugin's canonical visibility gate (next section), and only then consult a cache. A cache hit must never skip the gate.

### Visibility filters take a `permissions.Viewer`, never a bare `(role, userID)` (ADR-049)

**An empty user id means anonymous.** It never means "trusted" and is never a lookup key.

- Take a `permissions.Viewer`; bypass only on `v.SkipsPerUserRules()` (`system || CanSeeDmOnly(role)`). Never test the user id yourself.
- Build it with `permissions.RequestViewer(role, userID)` at the handler/service boundary; it cannot produce a trusted viewer (`system` is unexported).
- A genuinely trusted caller (an export walking its own rows, an already-authorized picker) uses `permissions.SystemViewer(role)` at that call site, with a comment justifying the trust.
- "Every caller" means every path to the rule, not every grep hit: search for the behaviour (pages and lists reaching the same visibility rule, including child lists, graph and dashboard queries) as well as the gate's name.
- Never synthesise an identity for an anonymous request (no `"anonymous"` user, no per-IP key): a shared identity is a shared write target.

### Auth surfaces — five canonical shapes

| Surface | Mounting | Middleware | Consumers |
|---|---|---|---|
| **Session-cookie (web UI)** | `internal/app/routes.go` (campaigns, entities, maps, plugin UI routes) | `auth.RequireAuth(authSvc)` + `campaigns.RequireCampaignAccess(campaignSvc)` | Browser users, `chronicle_session` cookie |
| **Per-campaign-token (legacy public API)** | `foundry_vtt/routes.go::RegisterPublicRoutes` | Per-campaign signed manifest token (`foundry_vtt/token.go`) | Foundry module manifest + download fetch |
| **Session-OR-Bearer (syncapi JSON)** | `syncapi/routes.go::RegisterAPIRoutes` `v1` group | `RequireAuthOrAPIKey` + `RateLimit` + `RequireJSONContentType` (state-changing methods get 415 without `Content-Type: application/json`) | Foundry sync REST API + in-app widgets |
| **Session-OR-Bearer (syncapi multipart)** | same file, `v1Multipart` group | `RequireAuthOrAPIKey` + `RateLimit` (skips `RequireJSONContentType`) | `POST /api/v1/campaigns/:id/media`, the only multipart endpoint under `/api/v1/*` (a sub-group that skips the JSON content-type check) |
| **Admin-session (site admin UI)** | `internal/app/routes.go` (admin group) | `auth.RequireAuth` + `auth.RequireSiteAdmin` + optional `auth.RequireReauth` | Site admin browser users |

The wire-contract test pins every route's auth class and middleware chain, so an added or removed middleware fails naming the route; it does not see middleware arguments such as a role level (see "Extending the guards").

### CSRF

Double-submit cookie via `internal/middleware/csrf.go` (`__Host-` prefix on HTTPS) on every session-cookie-authed write endpoint. Bearer-authed (syncapi) endpoints skip it; their callers carry no cookie.

CSP allows `'unsafe-inline'` + `'unsafe-eval'` (the Alpine.js trade-off), mitigated by `sanitize.HTML` on every user-controlled HTML field (see "Sanitization invariant" below).

A second, report-only policy (`script-src 'self'`, no inline code or eval) rides on every page; browsers report what it would block to `POST /csp-report`, which logs each distinct violation once (`internal/middleware/csp_report.go`). New code should not add inline scripts or handlers, so the switch to enforcing it (#1907) gets smaller, not larger.

### Signed URLs — HMAC-SHA256 with `crypto/subtle`

Two families:

- **`/media/...`** (`internal/plugins/media/handler.go`): HMAC-SHA256 over `fileID:viewer:expires` (ADR-058 decision 6), so a copied link is inert for anyone else. `currentViewerIdentity` resolves the viewer from the session cookie or `ViewerAnonymous`; `Verify` decides whether an anonymous presenter may satisfy a link minted for `ViewerAPIKey` (cookie-less Foundry `<img>`).
- **`/foundry-vtt/...`** (`internal/plugins/foundry_vtt/token.go`): per-campaign manifest URLs; `tokenDomain = "foundry-vtt"` scopes the HMAC so a media URL cannot replay as a manifest URL.

Both use `hmac.Equal`, never `==`/`bytes.Equal`, and check expiry; expired or replayed URLs get 403. A picture is readable if at least one page using it is visible to the viewer (ADR-058 decision 1); an unreferenced file falls back to campaign membership (decision 3).

### Sanitization invariant — bluemonday UGCPolicy on every HTML write

Every `Service.Create*`/`Update*` accepting an HTML-typed field calls `sanitize.HTML(...)` (`internal/sanitize/sanitize.go`) before persisting. `internal/sanitize/invariant_test.go` + `sanitize_invariant_snapshot.txt` pin it: any `service.go` declaring HTML-typed inputs needs at least one `sanitize.HTML` call (regenerate: `UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...`).

**Egress:** the `/api/v1/*` GET handlers emitting user HTML (`GetEntity`, `ListEntities`, `GetNote`, `ListNotes`) re-sanitize via `internal/plugins/syncapi/egress_sanitize.go` before `c.JSON` (`sanitize.HTMLPtr` for nullable fields); `TestEgressSanitize_HandlersInvokeHelpers` pins it. The calendar `GetEvent`/`ListEvents` handlers (`calendar_api_handler.go`) re-sanitize `description_html` with `sanitizeCalendarEventHTMLForEgress` / `sanitizeCalendarEventsHTMLForEgress`. Backup/restore export stays unsanitized (lossless): leave `internal/app/export_adapters.go` and `internal/plugins/campaigns/export_handler.go` alone.

### `SafeIdent` convention — DDL identifier interpolation

Every DDL statement interpolating a table or column name must pass it through `database.SafeIdent` (`internal/database/safeident.go`), which backtick-quotes it or errors unless it matches `^[a-zA-Z_][a-zA-Z0-9_]*$`.

```go
quoted, err := database.SafeIdent(tableName)
```

Callers: `DropExtensionTables` (`internal/extensions/migration_runner.go`) and `internal/database/icon_reconcile.go`. Applies even to "trusted" input such as `SHOW TABLES` output.

Logs never emit a raw email (an enumeration oracle): hash with `hashEmail()` (`internal/plugins/auth/loghash.go`, SHA-256 hex prefix; `loghash_test.go`).

`internal/plugins/foundry_vtt/descriptor_fallback_test.go` pins `defaultDescriptor()` field by field against `testdata/chronicle-package.json`; keep both in step.

Open: full middleware-chain capture for every route (#697); method-level sanitize invariant with flow analysis (#696).

## Production safety system

`cmd/server/main.go` runs three startup layers before serving:

| Layer | Purpose |
|---|---|
| `database.PreMigrationBackup` | `mysqldump` + gzip before any migration applies; skips when `mysqldump` is absent (`BACKUP_REQUIRED=1` makes that fatal, `docs/deployment.md`) |
| `database.MigrateWithBackup` | backup, then golang-migrate Up (rule 11 above) |
| `database.RunStartupHealthChecks` | Fail-fast (`os.Exit(1)`): migration version, critical-column inventory, connectivity, security audit (weak passwords, HTTP `BaseURL`, overprivileged grants, world-writable `schema_migrations`), and per-plugin smoke tests (e.g. `campaigns.ScanSmokeTest` runs a real `SELECT + Scan` to catch column-vs-struct drift) |

Details and how to add a smoke test: `internal/database/.ai.md` §Startup Health Check System.

### When a PR needs boot verification

Boot verification (`make docker-up && go run ./cmd/server`, checking the `smoke_tests` health check passes) is required when a PR touches plugin code with `SELECT`+`Scan`, plugin `repository.go` files, routes in the wire snapshot, migrations, `HealthCheckConfig.CriticalColumns`, `cmd/server/main.go` wiring, code an existing smoke test references, or handler setters/adapters wired at startup. It is not needed for templ-only changes, handler logic without scans, docs or tests.

### Substitute pattern (docker-unavailable sandboxes)

Without Docker (cloud and AI sandboxes), run these and say so in the PR description: grep `cmd/server/main.go` for `RunStartupHealthChecks` and the touched smoke tests (wiring intact); grep `cmd/server/*.go` for every removed or renamed symbol (zero hits); `go build ./cmd/server/`; `go run ./cmd/server` logs `starting Chronicle` and enters the MariaDB retry loop (static init did not panic). The real check is then an operator pre-merge gate: `make docker-up && go run ./cmd/server 2>&1 | head -50`, looking for `health check passed` with `check=smoke_tests`.

## CSS sub-layer naming — hyphen-vs-underscore

Plugin CSS sub-layers under `@layer plugins` use hyphens matching the public slug (`@layer plugins { @layer foundry-vtt, calendar, maps, packages, settings }`), while the Go package stays underscored (`foundry_vtt`).

## Tailwind JIT safelist for runtime-injected classes

Tailwind's JIT emits only classes found in source, so classes added at runtime by JS or HTMX (e.g. `.htmx-added`) are missing unless defined in `static/css/input.css` under an inline `@layer` rule (preferred; see the `.htmx-added` block) or safelisted in `tailwind.config.js`. Add the selector of any hand-written rule that must survive to `tools/css-emissions.txt`; `tools/check-css-emissions.sh` fails when the built stylesheet lacks it.

## Plugin registration + per-plugin static assets

A plugin self-describes with a `PluginRegistration` (slug, optional `embed.FS` for migrations, optional `embed.FS` for static assets, optional smoke test); the registry is `internal/app/plugins.go`. Only `ai_workspace`, `foundry_vtt` and `smtp` have a `registration.go`; the others register in `registeredPlugins()` (`cmd/server/plugin_schemas.go`).

A plugin's static assets (JS/CSS under `static/`) embed via `embed.FS` and mount through the registry, with no app-level static-route list. Not every plugin has moved; a `StaticFS` entry in its `registration.go` marks one that has.

Widget scripts load on sight (ADR-063): list them in `PluginRegistration.Widgets` as `{Name: "<data-widget name>", Scripts: []string{"js/helper.js", "js/widget.js"}}`, registering script last. A widget whose scripts live under `static/js/widgets/` goes in `coreWidgets` (`internal/app/routes.go`) with site paths. `boot.js` fetches them the first time the mount appears, including after boosted navigation. Do not add a feature script to `base.templ`; that list is for the shell and scripts Alpine needs before it starts. `TestEveryPageScriptBudget` (`internal/app/script_budget_test.go`) caps the count and gzip size of the scripts every page loads; a change past the cap raises it in the same PR with a reason, and a move out of the list lowers it.

## Static asset URLs go through `layouts.AssetURL`

Never write a bare `src="/static/…"` in a templ file; `TestTemplatesUseAssetURL` (`internal/templates/layouts/assets_test.go`) fails on one.

```templ
<script src={ layouts.AssetURL("/static/js/boot.js") } defer></script>
```

`?v=<content digest>` busts the cache on deploy (inside package `layouts`, call `AssetURL(...)` unqualified). Echo's `e.Static` sends `Last-Modified` but no `Cache-Control`, so browsers use heuristic freshness and a deploy can half-land: HTML names a Tailwind class the cached CSS lacks, and the element silently takes its unstyled (often `hidden`) branch. `middleware.StaticCache` (global, `app.New`) sets `immutable, max-age=1y` for `?v=` requests and `max-age=0, must-revalidate` otherwise, so a missed conversion is revalidated, not stale. Plugin `embed.FS` assets are content-hashed once registered with `layouts.RegisterAssetFS` (automatic for every `PluginRegistration.StaticFS`); unresolvable ones fall back to a per-build token.
