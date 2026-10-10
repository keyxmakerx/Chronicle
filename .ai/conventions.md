# Coding Conventions

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Concrete code patterns with examples. Every pattern the AI      -->
<!--          should follow when writing code for Chronicle.                  -->
<!-- Update: When a new pattern is established or an existing one changes.    -->
<!-- ====================================================================== -->

## Handler Pattern

Handlers are **thin**: bind request, call service, render response. No business logic, no direct repo calls, no SQL, no side effects.

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

Services own **all business logic**. They accept and return domain types only, and NEVER import `echo` or HTTP types.

```go
type CampaignService interface {
    Create(ctx context.Context, userID string, input CreateCampaignInput) (*Campaign, error)
}
```

## Repository Pattern

Repositories own **all SQL**, one per aggregate root, hand-written with `database/sql` + `go-sql-driver/mysql`. Use `?` placeholders, not `$1`.

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

Use the shared middleware helpers, never local copies (see the Handler Pattern example). `middleware.IsHTMX(c)` checks both `HX-Request == "true"` and `HX-Boosted != "true"`, so boosted navigation still gets a full page. `middleware.Render(c, status, component)` sets Content-Type and writes the Templ component.

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

The contract, the same everywhere: **an ABSENT key preserves. An EXPLICIT `null` clears. A present value replaces.**

A plain pointer (`*string`, `*int`, …) collapses "absent" and "explicit null" at the JSON bind layer, so it cannot express this. Use `internal/patch`'s `Field[T]` instead — it records presence in `UnmarshalJSON`, since `encoding/json` only calls that for keys the body actually carries.

```go
stored.Summary = input.Summary.Ptr(stored.Summary) // nullable column
stored.Status  = input.Status.Val(stored.Status)   // NOT NULL column
```

- `Ptr(cur)` (nullable field): absent → `cur`, null → `nil` (cleared), value → pointer to it.
- `Val(cur)` (non-nullable field): absent → `cur`, value → the value; explicit null also preserves, since a NOT NULL column has no cleared state and writing its zero silently is the data loss.
- Validators read the MERGED value, not the raw input — an absent name is not an empty name.
- A refused write drops to ABSENT, not null: denying a field is not authority to erase it.
- For a broad surface (many optional fields), load-merge-write beats a wall of nil-guards.
- Every governed endpoint is pinned in all three directions, documented where it's described (`API-CONTRACT.md` for the Foundry wire; the plugin's `.ai.md` for web routes).
- `internal/patch/partial_update_contract_test.go` pins the precondition: every field of a contract-governed `Update*Input` must be `patch.Field[T]`, a pointer, a map, or a slice; the whole-tree inventory is frozen, so a new one must be classified out loud (named exceptions carry a reason). Three classes: `contractGoverned` (partial, presence-aware), `fullReplaceByDesign` (audited: a form that posts every field, or a one-field body — the reason must be a fact about the callers), and `notYetSwept` (not audited; empty today, so a new input should not land there).

## Create Endpoints — visibility comes from the campaign, not from a zero value

An absent `is_private` on create must defer to the campaign's `DefaultVisibility` setting, not default to public — a value-typed `bool` can't tell "omitted" from "sent false", so binding one straight answers "public" either way.

Resolve it through `campaigns.CampaignSettings.ResolveNewEntityPrivacy`, the single implementation — don't re-derive the rule inline.

```go
input := entities.CreateEntityInput{
    Name:      req.Name,
    IsPrivate: settings.ResolveNewEntityPrivacy(req.IsPrivate), // req.IsPrivate is patch.Field[bool]
}
```

- Absent and explicit-false stay different; the campaign default fills only the first. A path with no client input at all (the bestiary import) is always the absent case and can call `DefaultsToPrivate()` directly.
- An unreadable campaign fails CLOSED: private, with a loud log — never public. Read the setting at most once per request (a sync batch may carry 2000 changes; the default can't change mid-request).

Pinned by `default_visibility_test.go` (campaigns), `default_visibility_create_test.go` (entities), `create_default_visibility_test.go` (syncapi), `bestiary_import_visibility_test.go` (app).

## Test Pattern (Table-Driven)

```go
tests := []struct {
    name    string
    input   CreateCampaignInput
    wantErr bool
}{
    {name: "creates campaign successfully", input: CreateCampaignInput{Name: "Eldoria"}},
    {name: "fails with empty name", input: CreateCampaignInput{Name: ""}, wantErr: true},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        _, err := svc.Create(context.Background(), "user-1", tt.input)
        if tt.wantErr { assert.Error(t, err) } else { assert.NoError(t, err) }
    })
}
```

## Widget Registration (Frontend JS)

Mounts to `data-widget="<slug>"` elements, fetches its own data, renders itself.

```javascript
Chronicle.register('editor', {
    init(el, config) { /* Mount, fetch from config.endpoint */ },
    destroy(el) { /* Cleanup */ }
});
```

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

Every package gets a doc comment (`// Package auth handles ...`). Every exported type gets a doc comment starting with its name. Non-obvious logic gets a short WHY, not a restatement of WHAT:

```go
// Check ownership before cascade delete because MariaDB FK constraints
// alone don't prevent cross-user deletion via direct ID manipulation.
if campaign.CreatedBy != userID {
    return apperror.NewForbidden("you do not own this campaign")
}
```

May point at one stable place for more: an ADR (`ADR-058`), a test name, or an issue/PR (`#613`, or `owner/repo#12` across repos).

Never: the story of how a bug was found, "before this fix"/"this used to", task or dispatch IDs (`C-…`), `cordinator/` paths, "this PR", dates as provenance, or `file:line` pointers (they drift). Those belong in the commit message and PR description. A comment describing code that no longer exists is a bug — fix it on sight.

Avoid: restating the code (`// Set name to the request name`), unexplained commented-out code, and comments stating the obvious (`// Delete deletes a campaign`).

`tools/check-comment-clutter.sh` (CI) fails a branch whose new comments carry an old tracking ID, a `cordinator/` path, "this PR", a `file:line` pointer, a pointer to `.ai/todo.md`, or a date outside tests. Existing comments are grandfathered; a line that truly needs one ends with `clutter-ok`.

**TODO format** names the tracking issue (open one first if none exists): `// TODO(#613): stop echoing untouched fields back on update`, or cross-repo `// TODO(keyxmakerx/Chronicle-Foundry-Module#95): ...`.

## Schema, Migrations and Permissions

### Two-Tier Schema System (ADR-028)

**Core** (`db/migrations/`): all core tables, runs via golang-migrate on startup; failure is fatal. **Plugin** (`internal/plugins/<name>/migrations/`): each built-in plugin's numbered migrations are **embedded in the binary** via `embed.FS` (ADR-030) in an `embed.go`, run via `RunPluginMigrations()` after core; failure disables that plugin, the app keeps serving. `RegisteredPlugins()` lives in `cmd/server/main.go`, not the database package, to avoid import cycles.

### Migration Safety Rules

1. **ENUM values**: add a new ENUM value via `ALTER TABLE` in the same or an earlier migration before using it in an INSERT/UPDATE — never assume it exists from a different, unapplied migration.
2. **Seed data conflicts**: check whether seed data for a slug/key already exists; use UPDATE or `INSERT ... ON DUPLICATE KEY UPDATE`, not a bare INSERT.
3. **Down migrations**: if the up migration UPDATEs a row, the down migration reverts it to its original values, not DELETE; only DELETE rows the same migration INSERTed.
4. **ENUM in down migrations**: revert all rows using an added ENUM value before removing that value from the ENUM.
5. `internal/database/migrate_test.go` validates ENUM values in migration SQL — update its valid sets when adding new ones.
6. **Plugin tables**: belong in `internal/plugins/<name>/migrations/`, not `db/migrations/`; failures degrade gracefully (ADR-028). A new plugin with migrations needs an `embed.go` and a registration in `registeredPlugins()` in `cmd/server/main.go`.
7. **Migration layering**: core migrations reference ONLY core schema; plugin migrations own their tables and any backfills touching them. Core runs before plugins, so a core migration referencing a plugin-owned table (`api_keys`, `maps`, `calendars`, etc.) crashes on a fresh DB — split a cross-layer fix: core part in `db/migrations/`, plugin part in that plugin's `migrations/`.
8. **Idempotent DDL (enforced)**: `ADD COLUMN`/`CREATE TABLE` use `IF NOT EXISTS`; `DROP COLUMN`/`DROP TABLE`/`DROP INDEX` use `IF EXISTS` — bare DDL fails if a partially-applied migration re-runs. `TestMigrations_IdempotentDDL` enforces this on new migrations (historical files are grandfathered, per #9).
9. **Append-only / immutability**: NEVER delete, edit, or renumber a migration any live database may have applied — golang-migrate needs a file for every version up to the DB's recorded one; removing one crash-loops boot (ADR-044/045). `tools/check-migration-immutability.sh` (CI) fails a PR that deletes or edits an existing migration file.
10. **Schema-only**: a one-time DATA correction belongs in an IDEMPOTENT reconciler — an `EnsureX`/`MergeX` service method run from a boot backfill, an addon-enable hook, or an owner-triggered `SetupProvider` (see `app/setup_pc.go` + `entities.MergeDuplicatePlayerCharacterType`) — never a migration.
11. **Boot runtime contract** (`database.MigrateWithBackup`, ADR-045): the pre-migration backup runs only when a migration is pending; a DB AHEAD of the build logs a warning and boots anyway; a DIRTY DB fails fast with restore guidance; `fatalBoot` backs off (`BOOT_FAIL_BACKOFF`, 45s). Keep `ExpectedCoreMigrationVersion` (`migrate_state.go`) equal to the highest migration — `TestExpectedCoreMigrationVersion_MatchesMax` enforces it.

### Permission Model

`internal/permissions` provides shared role constants (`RoleOwner`, `RoleScribe`, `RolePlayer`) and helpers (`CanSeeDmOnly`, `CanSetDmOnly`) for services/repos that can't import `campaigns` (circular deps).

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

**dm_only rules:** only Owners can create or toggle dm_only on any resource; only Owners can see dm_only content (default; per-campaign config is a later phase). Handlers silently strip dm_only from non-Owner requests (not a 403). Use `permissions.CanSeeDmOnly(role)` / `permissions.CanSetDmOnly(role)`.

## Formatting — do NOT `gofmt -w` over globs

This repo is **not plain-`gofmt`-clean** (many committed files predate or differ from the local `gofmt` version's alignment), so `gofmt -w` over a package or `*.go` glob reformats unrelated files and pollutes the diff. **Edit in place** instead — Edit preserves surrounding formatting, and `make`/`templ generate` handle codegen. If you must format, scope it to the exact files you authored.

## CI tenet-enforcement guards

`make verify` runs the full local CI sequence, guards included; `.github/workflows/ci.yml` and the `verify` target in the `Makefile` are the source of truth for the list. The guards enforce the binding tenets (T-B1 security, T-B2 plugin isolation, T-O2/T-O3 verification) from `cordinator/decisions/2026-05-21-core-tenets.md`. The table below covers the ones that need explanation.

| Guard | File | Mode | Enforces |
|---|---|---|---|
| Plugin isolation grep | `tools/check-plugin-isolation.sh` | diff-scoped FAIL | T-B2: no new `foundry-vtt`/`foundry-module`/`foundry_vtt` literals outside `internal/plugins/foundry_vtt/*` |
| Motion discipline | `tools/check-v2-motion-discipline.sh` | diff-scoped FAIL | No new `transition: all`/`transition-all` in calendar/timeline/ai_workspace/campaigns sources; opt out per line with `/* OK exempt: … */` |
| Wire-contract conformance | `internal/wire/wire_contract_test.go` + `routes_snapshot.txt` | FAIL (snapshot) | T-O2: every Echo route registration is in the curated snapshot |
| Foundry public rate-limit pin | `internal/wire/foundry_public_ratelimit_test.go` | FAIL | T-B1: two AST assertions pin the Foundry public manifest rate-limit wiring (`g.Use(rateLimit)` in `foundry_vtt.RegisterPublicRoutes`) and call site (`middleware.RateLimit(...)` in `app.RegisterRoutes`) |
| Sanitize-on-write invariant | `internal/sanitize/invariant_test.go` + snapshot | FAIL (snapshot + invariant) | T-B1: every `internal/plugins/*/service.go` (+ widgets) declaring HTML-typed inputs must call `sanitize.HTML` |
| Decision-citations | `tools/check-decision-citations.sh` | WARN (exit 0, `continue-on-error`) | T-O3: warns about a `cordinator/decisions/*.md` that nothing references; with no Cordinator checkout it has nothing to check |

### Pre-merge rules that no guard checks

Check these by hand: a PR adding an `hx-get` fragment endpoint lists every consumer (each templ file or fetch call embedding it), and one deleting an endpoint shows every consumer moved elsewhere; a PR changing how a URL is served checks that on-disk artifacts (extracted zips, cached files, generated manifests) carry the new URL too, not just runtime rewriting; click handlers inside HTMX fragments use the inline-IIFE `onclick` pattern (Go-side builders like `foundry_vtt/onclick_handlers.go`), never `templ script` helpers or delegated `document.addEventListener`, since a templ script tag isn't reliably run before a swapped-in button can be clicked (`onclick_handlers_test.go` enforces this for `foundry_vtt` only); keep `foundry_vtt/errors.go`, its `.ai.md` catalog table, and `error-catalog.json` in step (`foundry_vtt/errors_test.go`).

### Extending the guards

- **Wire-contract snapshot:** on an intentional route add/remove/change, run `UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...` and commit the regenerated `internal/wire/routes_snapshot.txt`, explaining the change (especially for the auth surfaces below). Limitations: the snapshot captures `(method, path, file)` via static AST extraction — it doesn't resolve group prefixes (an `e.Group("/admin")` rename), classify the auth surface, capture programmatic registration (loops/builders), or catch per-route middleware removal in general (#697 is the open work), except where a focused AST assertion pins one invariant by hand, as `internal/wire/foundry_public_ratelimit_test.go` does for the Foundry rate limit — copy its shape for a new one: locate the function wiring the middleware and assert its `*.Use(...)` call, then the call site supplying the argument and assert it names the middleware.
- **Plugin-isolation guard:** targets `foundry-vtt` strings only today; other plugin names aren't checked. Joins its tokens from fragments at runtime so it can scan its own tree without false-positiving on itself.
- **Sanitize-invariant snapshot:** on a new plugin's `service.go`, or added/removed HTML-typed inputs, run `UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...` and commit the regenerated snapshot; cite the audit in the PR if it's a new sanitize surface.

## Cross-plugin import discipline

Plugins are physically isolated under `internal/plugins/<slug>/`. Cross-plugin communication is **always** mediated by an exported Go interface (a service or middleware) defined on the providing plugin (CLAUDE.md rule 8). Importing another plugin's repository, store, internal struct, or `_test.go` helpers is a layering violation.

```go
// In plugin-A: define the interface you need from plugin-B; plugin-B implements
// it and exposes it via NewService(...) campaigns.CampaignService; plugin-A's
// wiring accepts the interface, not the concrete type.
type CampaignService interface {
    Get(ctx context.Context, id string) (*Campaign, error)
}
```

- Importing a concrete type from another plugin is a violation; the imported plugin must expose an interface, or a middleware constructor returning `echo.MiddlewareFunc`.
- `internal/app/routes.go` is the ONLY package that imports every plugin's package; new plugins register there.
- `foundry_vtt` is imported by `internal/websocket/{auth,client,hub}.go` only for the `foundry_vtt.ModuleSource` const — a thin const-usage import, not behavioral coupling, so it's fine.

**Regression-prevention:** `tools/check-plugin-isolation.sh` (CI, diff-scoped FAIL) catches new `foundry-vtt`/`foundry-module` magic-string literals outside `internal/plugins/foundry_vtt/` (targets that plugin only today); the wire-contract conformance test catches new routes outside the curated snapshot; code review catches `import ".../internal/plugins/<X>/<subpkg>"` paths beyond `internal/plugins/<X>` itself (e.g. `.../repository`), the canonical "bypassed the interface" smell.

## Security

Per `cordinator/decisions/2026-05-21-core-tenets.md §T-B1`, security is the highest-priority tenet. A PR touching an auth surface, sanitization site, signed URL, or SQL identifier interpolation states the security implication in its description; the CI guards below are load-bearing where one exists.

### "The route is authorized" is not "the object is authorized"

Campaign middleware proves the caller belongs to the campaign in the URL. It proves nothing about an object a handler then loads **by its own id**. When addressing an object by id: resolve it, 404 if its campaign isn't the route's campaign, run the plugin's canonical visibility gate (next section), and only then consult a cache — a cache hit must never skip the gate.

### Visibility filters take a `permissions.Viewer`, never a bare `(role, userID)` (ADR-049)

**An empty user id means ANONYMOUS.** It has never meant "trusted", and must never be used as a lookup key.

- Take a `permissions.Viewer`; bypass only on `v.SkipsPerUserRules()` (`system || CanSeeDmOnly(role)`). Never test the user id yourself.
- Build it with `permissions.RequestViewer(role, userID)` at the handler/service boundary — it cannot produce a trusted viewer (`system` is unexported).
- A genuinely trusted caller (an export walking its own rows, an already-authorized picker) uses `permissions.SystemViewer(role)` at that call site, with a comment justifying the trust.
- **"Every caller" means every path to the rule, not every grep hit.** When a change or audit says it covers every caller of a gate, search for the behaviour (which pages and lists reach the same visibility rule, including through helpers such as child lists, graph and dashboard queries) as well as for the gate's name. A list built from one function's name misses the paths that reach the rule another way.
- Never synthesise an identity for an anonymous request (no `"anonymous"` user, no per-IP key) — a shared anonymous identity is a shared write target.

### Auth surfaces — five canonical shapes

Conflating these is the risk this table and the wire-contract test guard against.

| Surface | Mounting | Middleware | Consumers |
|---|---|---|---|
| **Session-cookie (web UI)** | `internal/app/routes.go` (campaigns, entities, maps, plugin UI routes) | `auth.RequireAuth(authSvc)` + `campaigns.RequireCampaignAccess(campaignSvc)` | Browser users, `chronicle_session` cookie |
| **Per-campaign-token (legacy public API)** | `foundry_vtt/routes.go::RegisterPublicRoutes` | Per-campaign signed manifest token (`foundry_vtt/token.go`) | Foundry module manifest + download fetch |
| **Session-OR-Bearer (syncapi JSON)** | `syncapi/routes.go::RegisterAPIRoutes` `v1` group | `RequireAuthOrAPIKey` + `RateLimit` + `RequireJSONContentType` (state-changing methods get 415 without `Content-Type: application/json`) | Foundry sync REST API + in-app widgets |
| **Session-OR-Bearer (syncapi multipart)** | same file, `v1Multipart` group | `RequireAuthOrAPIKey` + `RateLimit` (skips `RequireJSONContentType`) | `POST /api/v1/campaigns/:id/media`, the only multipart endpoint under `/api/v1/*` (a sub-group that skips the JSON content-type check) |
| **Admin-session (site admin UI)** | `internal/app/routes.go` (admin group) | `auth.RequireAuth` + `auth.RequireSiteAdmin` + optional `auth.RequireReauth` | Site admin browser users |

**Regression-prevention:** the wire-contract conformance test pins every Echo route registration, but only `(method, path, file)` — not per-route middleware (see "Extending the guards" above).

### CSRF

Double-submit cookie via `internal/middleware/csrf.go` (`__Host-` prefix on HTTPS), applied to every session-cookie-authed write endpoint. Bearer-authed endpoints (syncapi) skip it since cross-origin Bearer callers don't carry the cookie.

CSP allows `'unsafe-inline'` + `'unsafe-eval'` (the Alpine.js trade-off), mitigated by `sanitize.HTML` on every user-controlled HTML field (see "Sanitization invariant" below).

### Signed URLs — HMAC-SHA256 with `crypto/subtle`

Two families:

- **`/media/...`** (`internal/plugins/media/handler.go`) — per ADR-058 decision 6, the signature is HMAC-SHA256 over `fileID:viewer:expires`, not just `fileID:expires`, so a copied link is inert for anyone else. `currentViewerIdentity` resolves the viewer from the session cookie or `ViewerAnonymous`; `Verify` decides whether an anonymous presenter may still satisfy a link minted for `ViewerAPIKey` (cross-origin, cookie-less Foundry `<img>`).
- **`/foundry-vtt/...`** (`internal/plugins/foundry_vtt/token.go`) — per-campaign signed manifest URLs; `tokenDomain = "foundry-vtt"` scopes the HMAC so a media-signed URL can't replay as a manifest URL.

Both use `hmac.Equal` (constant-time), never `==`/`bytes.Equal`; verification checks expiry, and replayed/expired URLs get 403. A picture is readable if at least one page using it is visible to the viewer (ADR-058 decision 1); an unreferenced file falls back to campaign membership (decision 3).

### Sanitization invariant — bluemonday UGCPolicy on every HTML write

Every plugin's `Service.Create*`/`Update*` accepting an HTML-typed field calls `sanitize.HTML(...)` (`internal/sanitize/sanitize.go`) before persisting: `internal/plugins/{entities,sessions,timeline,campaigns}/service.go`, `internal/widgets/{notes,posts,entity_notes}/service.go`. `internal/sanitize/invariant_test.go` + `sanitize_invariant_snapshot.txt` pin this: any `service.go` declaring HTML-typed inputs needs ≥1 `sanitize.HTML` call (regenerate via `UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...`).

**Egress:** the Foundry-bound `/api/v1/*` GET handlers emitting user HTML (`GetEntity`, `ListEntities`, `GetNote`, `ListNotes`) re-sanitize via `internal/plugins/syncapi/egress_sanitize.go` helpers before `c.JSON` (`sanitize.HTMLPtr` is the nullable companion); `TestEgressSanitize_HandlersInvokeHelpers` pins the wiring. Backup/restore export stays unsanitized (lossless) — don't touch `internal/app/export_adapters.go` or `internal/plugins/campaigns/export_handler.go`. The Foundry-bound calendar `GetEvent`/`ListEvents` handlers (`internal/plugins/syncapi/calendar_api_handler.go`) re-sanitize `description_html` through `sanitizeCalendarEventHTMLForEgress` / `sanitizeCalendarEventsHTMLForEgress`.

### `SafeIdent` convention — DDL identifier interpolation

Every SQL DDL statement interpolating a table/column name MUST pass it through `internal/database/safeident.go::SafeIdent` (backtick-quotes it, or errors unless it matches `^[a-zA-Z_][a-zA-Z0-9_]*$`).

```go
quoted, err := database.SafeIdent(tableName)
```

Callers include `internal/extensions/migration_runner.go::DropExtensionTables` and `internal/database/icon_reconcile.go`. Future DDL identifier interpolation MUST use this helper, even for "trusted" input (e.g. from `SHOW TABLES`).

Debug logs must never emit a raw email (log access becomes an enumeration oracle) — hash via `internal/plugins/auth/loghash.go::hashEmail()` (SHA-256 hex prefix), pinned by `loghash_test.go`.

`internal/plugins/foundry_vtt/descriptor_fallback_test.go` pins the fallback `defaultDescriptor()` field-by-field against `testdata/chronicle-package.json`; keep both in step.

Open work: full middleware-chain capture for every route (#697), method-level sanitize invariant with flow analysis (#696).

**Reading order for a security-touching PR:** this section → `cordinator/decisions/2026-05-21-core-tenets.md §T-B1` → the plugin's `.ai.md` → the relevant CI guard's source.

## Production safety system

`cmd/server/main.go` runs three startup layers before serving traffic. Touch any DB-adjacent surface and you intersect this system.

| Layer | Purpose |
|---|---|
| `database.PreMigrationBackup(cfg)` | `mysqldump` + gzip before any migration applies; silently skips when `mysqldump` is absent (`BACKUP_REQUIRED=1` flips that to fail-loud, per `docs/deployment.md`) |
| `database.RunMigrations(db, cfg)` | golang-migrate, auto-Up, dirty-state retry |
| `database.RunStartupHealthChecks(db, cfg)` | Fail-fast validation (`os.Exit(1)` on failure): migration version, critical-column inventory, DB connectivity, security audit (weak passwords, HTTP `BaseURL`, overprivileged grants, world-writable `schema_migrations`), and per-plugin smoke tests (e.g. `campaigns.ScanSmokeTest` runs a real `SELECT + Scan` to validate the column list matches the `Campaign` struct) |

See `internal/database/.ai.md §Startup Health Check System` for the full breakdown and how to extend a smoke test.

### When a PR needs boot verification

Boot verification (a real `make docker-up && go run ./cmd/server`, checking for `smoketest passed` on each plugin touched) is required when a PR touches: plugin `*.go` files containing `SELECT`+`Scan` patterns, plugin `repository.go` files, routes affecting the wire snapshot, migration files, the `HealthCheckConfig.CriticalColumns` map, `cmd/server/main.go` startup wiring, a refactor removing code an existing smoke test references, or a refactor removing handler setters/adapters wired at app startup.

Typically not needed for templ-only changes, handler logic without DB scan patterns, pure documentation, or test files.

### Substitute pattern (docker-unavailable sandboxes)

When `make docker-up` isn't available (common in cloud/AI sandboxes), run these static substitutes and say so in the PR description: grep `cmd/server/main.go` for `RunStartupHealthChecks` + the touched smoke tests (wiring intact); grep `cmd/server/*.go` for every removed/renamed symbol, expecting zero hits (no symbol leakage); `go build ./cmd/server/` succeeds (clean binary); `go run ./cmd/server` emits `starting Chronicle` and enters the MariaDB retry loop, proving static init completes without panic (reaches DB layer).

The real check transfers to the operator as a pre-merge gate: `make docker-up && go run ./cmd/server 2>&1 | head -50`, looking for `smoketest passed` on each touched plugin.

## CSS sub-layer naming — hyphen-vs-underscore

Plugin CSS sub-layers under `@layer plugins` use **hyphens** matching the public plugin slug, e.g. `@layer plugins { @layer foundry-vtt, calendar, maps, packages, settings }` — not the Go package's underscore (`internal/plugins/foundry_vtt/`, since Go disallows hyphens in identifiers). Register a new plugin's sub-layer with its hyphen slug; keep the Go package underscored.

## Tailwind JIT safelist for runtime-injected classes

Tailwind's JIT only emits classes found in source files, so classes added at runtime by JS or HTMX (e.g. `.htmx-added`, applied during the settle phase) won't be in the compiled CSS unless referenced: define base styling directly in `static/css/input.css` under an inline `@layer` rule (see the `.htmx-added` + `@starting-style` block there), or add a runtime Tailwind-utility class to the safelist in `tailwind.config.js`. Prefer the inline `@layer` for HTMX/Alpine-injected classes, since the JIT can't see them.

## Plugin registration + per-plugin static assets

Per `cordinator/decisions/2026-05-23-plugin-registration.md`, plugins self-describe via a `PluginRegistration` value (slug, optional `embed.FS` for migrations, optional `embed.FS` for static assets, optional smoke test); the registry is `internal/app/plugins.go`. Only `ai_workspace`, `foundry_vtt` and `smtp` have a `registration.go`; the other plugins register in `registeredPlugins()` in `cmd/server/main.go`.

Per `cordinator/decisions/2026-05-25-plugin-static-assets.md`, each plugin's static assets (JS/CSS under `static/`) embed via `embed.FS` and mount through the registry — no app-level static-route enumeration. Not every plugin has migrated; check a plugin's `registration.go` for a `StaticFS` entry.

A plugin's widget scripts load on sight (ADR-063): list them in `PluginRegistration.Widgets` as `{Name: "<data-widget name>", Scripts: []string{"js/helper.js", "js/widget.js"}}`, registering script last. `boot.js` fetches them the first time that mount appears on a page, including after boosted navigation. Don't add a feature script to `base.templ`; that list is for the shell and for scripts Alpine needs before it starts.

## Static asset URLs go through `layouts.AssetURL`

**Never write a bare `src="/static/…"` in a templ file** — `TestTemplatesUseAssetURL` (`internal/templates/layouts/assets_test.go`) walks every `.templ` and fails on one.

```templ
<script src={ layouts.AssetURL("/static/js/boot.js") } defer></script>
```

`?v=<content digest>` busts the cache on deploy (inside package `layouts` itself, call `AssetURL(...)` unqualified). This matters because Echo's `e.Static` emits `Last-Modified` but no `Cache-Control`, so browsers use heuristic freshness — a deploy adding a Tailwind utility can half-land: HTML names a class the cached CSS never heard of, and the element silently takes its unstyled (usually `hidden`) branch. `middleware.StaticCache` (global, `app.New`) sets `immutable, max-age=1y` for `?v=`-carrying requests and `max-age=0, must-revalidate` otherwise, so a missed conversion degrades to "revalidated every use", never "silently stale". Plugin `embed.FS` assets are content-hashed too once registered with `layouts.RegisterAssetFS` (automatic for every `PluginRegistration.StaticFS`); anything unresolvable falls back to a per-build token, which still busts on deploy.
