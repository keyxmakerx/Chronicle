# Coding Conventions

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Concrete code patterns with examples. Every pattern the AI      -->
<!--          should follow when writing code for Chronicle.                  -->
<!-- Update: When a new pattern is established or an existing one changes.    -->
<!-- ====================================================================== -->

## Handler Pattern

Handlers are **thin**. Bind request, call service, render response. NO business logic.

```go
// GOOD -- handler is thin, delegates to service
func (h *CampaignHandler) Create(c echo.Context) error {
    var req CreateCampaignRequest
    if err := c.Bind(&req); err != nil {
        return apperror.NewBadRequest("invalid request body")
    }
    if err := c.Validate(req); err != nil {
        return err
    }

    userID := middleware.GetUserID(c)

    campaign, err := h.service.Create(c.Request().Context(), userID, req.ToInput())
    if err != nil {
        return err
    }

    if isHTMX(c) {
        return render(c, http.StatusCreated, templates.CampaignCard(campaign))
    }
    return render(c, http.StatusCreated, templates.CampaignShow(campaign))
}

// BAD -- business logic in handler
func (h *CampaignHandler) Create(c echo.Context) error {
    // DO NOT: validate business rules here
    // DO NOT: call repository directly
    // DO NOT: construct SQL here
    // DO NOT: send emails or trigger side effects here
}
```

## Service Pattern

Services own **all business logic**. They accept and return domain types only.
They NEVER import `echo` or HTTP types.

```go
// CampaignService handles business logic for campaign operations.
type CampaignService interface {
    Create(ctx context.Context, userID string, input CreateCampaignInput) (*Campaign, error)
    GetByID(ctx context.Context, id string) (*Campaign, error)
    List(ctx context.Context, userID string, opts ListOptions) ([]Campaign, error)
    Update(ctx context.Context, id string, userID string, input UpdateCampaignInput) (*Campaign, error)
    Delete(ctx context.Context, id string, userID string) error
}

type campaignService struct {
    repo  CampaignRepository
    cache *redis.Client
}

func NewCampaignService(repo CampaignRepository, cache *redis.Client) CampaignService {
    return &campaignService{repo: repo, cache: cache}
}
```

## Repository Pattern

Repositories own **all SQL**. One per aggregate root. Hand-written SQL with
`database/sql` + `go-sql-driver/mysql`. Use `?` placeholders (not `$1`).

```go
// CampaignRepository defines the data access contract for campaigns.
type CampaignRepository interface {
    Create(ctx context.Context, campaign *Campaign) error
    FindByID(ctx context.Context, id string) (*Campaign, error)
}

func (r *campaignRepository) FindByID(ctx context.Context, id string) (*Campaign, error) {
    query := `SELECT id, name, slug, description, created_by, created_at, updated_at
              FROM campaigns WHERE id = ?`

    var c Campaign
    err := r.db.QueryRowContext(ctx, query, id).Scan(
        &c.ID, &c.Name, &c.Slug, &c.Description,
        &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
    )
    if errors.Is(err, sql.ErrNoRows) {
        return nil, apperror.NewNotFound("campaign not found")
    }
    if err != nil {
        return nil, fmt.Errorf("querying campaign by id: %w", err)
    }
    return &c, nil
}
```

## Templ Component Pattern

One component per file. File name matches component name. Props as function args.

```go
// CampaignCard renders a summary card for the campaign listing.
templ CampaignCard(campaign *model.Campaign) {
    <div class="card" id={ fmt.Sprintf("campaign-%s", campaign.ID) }>
        <h3>{ campaign.Name }</h3>
        <p>{ campaign.Description }</p>
        <button
            hx-get={ fmt.Sprintf("/campaigns/%s", campaign.ID) }
            hx-target="#detail-panel"
            hx-swap="innerHTML"
        >View Details</button>
    </div>
}
```

## HTMX Fragment Detection

Use the shared middleware helpers — not local copies:

```go
// Check if request is HTMX (also rejects HX-Boosted to avoid fragments on navigation).
if middleware.IsHTMX(c) {
    return middleware.Render(c, http.StatusOK, MyFragment(data))
}
return middleware.Render(c, http.StatusOK, MyFullPage(data))
```

`middleware.IsHTMX(c)` checks both `HX-Request == "true"` and `HX-Boosted != "true"`.
`middleware.Render(c, status, component)` sets Content-Type and writes the Templ component.

## Error Handling

Domain errors from `internal/apperror/`. Never expose raw DB errors.

```go
apperror.NewNotFound("campaign not found")
apperror.NewBadRequest("name is required")
apperror.NewForbidden("you do not own this campaign")
apperror.NewInternal("unexpected error")     // Logs real error, returns generic
apperror.NewConflict("slug already exists")
apperror.NewUnauthorized("invalid session")
```

## Partial-Update Endpoints (nil-preserve semantics)

For Update handlers that accept a payload describing one or more rows, **prefer
explicit nil-preserve guards or load-merge-write over unconditional field
assignment.** Pointer-typed input fields (`*string`, `*int`, `*bool`, etc.)
collapse "absent" and "explicit null" at the JSON bind layer — nil at the
service is the signal for "the caller didn't send this field; keep current
value", not "overwrite to NULL".

```go
// ❌ Wrong — partial-save silently blanks Description if absent from request.
cal.Description = input.Description

// ✓ Right — nil-guard preserves the existing value.
if input.Description != nil {
    cal.Description = input.Description
}
```

For broader surfaces (e.g. weather, where 14+ pointer fields can each be
absent), `load-merge-write` is cleaner than a wall of nil-guards: load the
existing row, overlay the non-nil input fields, write the merged result.

Canonical precedent: chronicle#318 (`UpdateEntityInput.IsPrivate *bool`) for
single-field nil-guards; chronicle PR for C-CAL-NULL-PRESERVE (`SetWeather`
load-merge-write) for the multi-field merge pattern. Audit context:
`cordinator/reports/chronicle/2026-05-19-c-cal-null-preserve-audit.md`.

**Trade-off to acknowledge:** nil-preserve guards make it harder to clear a
field by sending explicit null. If "clear by null" is a real use case for a
field, ship a dedicated endpoint or escape-hatch flag rather than mixing
preserve-and-clear into one input — the two semantics interfere.

## Test Pattern (Table-Driven)

```go
func TestCampaignService_Create(t *testing.T) {
    tests := []struct {
        name    string
        input   CreateCampaignInput
        setup   func(*mockCampaignRepo)
        wantErr bool
    }{
        {
            name:  "creates campaign successfully",
            input: CreateCampaignInput{Name: "Eldoria"},
            setup: func(m *mockCampaignRepo) {
                m.createFn = func(ctx context.Context, c *Campaign) error { return nil }
            },
        },
        {
            name:    "fails with empty name",
            input:   CreateCampaignInput{Name: ""},
            wantErr: true,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            repo := &mockCampaignRepo{}
            if tt.setup != nil { tt.setup(repo) }
            svc := NewCampaignService(repo, nil)
            _, err := svc.Create(context.Background(), "user-1", tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

## Widget Registration (Frontend JS)

```javascript
/**
 * @module editor - TipTap rich text editor widget.
 * Mounts to any element with data-widget="editor".
 * Data attrs: data-endpoint (API URL), data-editable ("true"/"false")
 */
Chronicle.register('editor', {
    init(el, config) { /* Mount TipTap, fetch from config.endpoint */ },
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

### Every Package

```go
// Package auth handles user authentication, session management, and
// password hashing for Chronicle.
package auth
```

### Every Exported Type

```go
// Campaign represents a top-level worldbuilding container.
type Campaign struct { ... }
```

### Non-Obvious Logic (WHY, not WHAT)

```go
// Check ownership before cascade delete because MariaDB FK constraints
// alone don't prevent cross-user deletion via direct ID manipulation.
if campaign.CreatedBy != userID {
    return apperror.NewForbidden("you do not own this campaign")
}
```

### TODO Format

```go
// TODO: Add soft-delete with 30-day recovery window
// TODO(auth): Implement login rate limiting
```

### Two-Tier Schema System (ADR-028)

Chronicle uses a **plugin-isolated database schema architecture**:

- **Core schema** (`db/migrations/`): Single baseline migration with all core tables.
  Runs via golang-migrate on startup. Failure is fatal.
- **Plugin schema** (`internal/plugins/<name>/migrations/`): Each built-in plugin
  has its own numbered migration files **embedded in the binary** via Go's `embed.FS`
  (ADR-030). Each plugin has an `embed.go` exporting `MigrationsFS`. Runs via
  `RunPluginMigrations()` after core migrations. Failure disables that plugin; app
  continues serving. `RegisteredPlugins()` lives in `cmd/server/main.go` (not in
  the database package) to avoid import cycles.

```sql
-- Core migration example: db/migrations/000001_baseline.up.sql
CREATE TABLE IF NOT EXISTS campaigns ( ... );

-- Plugin migration example: internal/plugins/calendar/migrations/001_calendar_tables.up.sql
CREATE TABLE IF NOT EXISTS calendars ( ... );
```

### Migration Safety Rules

1. **ENUM values**: Before using a new ENUM value in an INSERT or UPDATE, the
   same migration (or an earlier one) must ALTER TABLE to add that value. Never
   assume ENUM values exist from a different, unapplied migration.
2. **Seed data conflicts**: Check if seed data for a slug/key already exists from
   an earlier migration. Use UPDATE or INSERT ON DUPLICATE KEY UPDATE, not INSERT.
3. **Down migrations**: If the up migration UPDATEs an existing row, the down
   migration should revert it to its original values, not DELETE it. Only DELETE
   rows that were INSERTed by the same migration.
4. **ENUM in down migrations**: If the up migration adds an ENUM value, the down
   migration must revert all rows using that value BEFORE removing it from the ENUM.
5. **Validation tests**: `internal/database/migrate_test.go` validates ENUM values
   in migration SQL. Update the valid sets there when adding new ENUM values.
6. **Plugin tables**: Plugin tables belong in `internal/plugins/<name>/migrations/`,
   not in `db/migrations/`. Plugin schema failures degrade gracefully (ADR-028).
   Migrations are embedded in the binary via `embed.FS` (ADR-030). When adding a
   new plugin with migrations, create an `embed.go` in the plugin package and
   register it in `registeredPlugins()` in `cmd/server/main.go`.
7. **Migration layering**: Core migrations (`db/migrations/`) may reference ONLY
   core schema. Plugin migrations (`internal/plugins/<slug>/migrations/`) own
   their tables and any data backfills/heals that touch them. Core runs before
   plugins, so a core migration that references a plugin-owned table (`api_keys`,
   `maps`, `calendars`, etc.) crashes on a fresh DB. If a single data fix needs
   to span both layers, split it: the core part stays in `db/migrations/`, the
   plugin part moves to that plugin's `migrations/` directory.

### Permission Model

Chronicle uses a hierarchical role system. The `internal/permissions` package
provides shared constants (`RoleOwner`, `RoleScribe`, `RolePlayer`) and
helper functions (`CanSeeDmOnly`, `CanSetDmOnly`) for services/repos that
cannot import `campaigns` due to circular deps.

**Role hierarchy:** Admin (site) > Owner (campaign) > Scribe > Player > Public

**Permission matrix:**

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
\+ Notes: own notes only; shared notes visible to all campaign members.

**dm_only rules:**
- Only Owners can create or toggle dm_only on any resource
- Only Owners can see dm_only content (default; Phase 2 adds per-campaign config)
- Handlers silently strip dm_only from non-Owner requests (not a 403)
- Use `permissions.CanSeeDmOnly(role)` / `permissions.CanSetDmOnly(role)` for checks

### Anti-Patterns (AVOID)

```go
// BAD: Restating the code
// Set name to the request name
c.Name = req.Name

// BAD: Commented-out code without explanation
// c.Status = "draft"

// BAD: Obvious comment
// Delete deletes a campaign
func (s *service) Delete(...) error
```

## CI tenet-enforcement guards

Per `C-CI-GUARDS-PHASE-2` (lands four mechanisms enforcing tenets from
`cordinator/decisions/2026-05-21-core-tenets.md`). Each guard runs in
`.github/workflows/ci.yml`. Extending the guards is part of normal PR
work — coordinator updates the dispatch + audit citations.

| Guard | File | Mode | Enforces |
|---|---|---|---|
| Plugin isolation grep | `tools/check-plugin-isolation.sh` | **diff-scoped FAIL** | T-B2: no new `foundry-vtt` / `foundry-module` / `foundry_vtt` literals outside `internal/plugins/foundry_vtt/*` |
| Templ drift | `tools/check-templ-drift.sh` | **FAIL** | hygiene-audit §0.5 D6: generated `.templ.go` files always match their `.templ` source |
| Wire-contract conformance | `internal/wire/wire_contract_test.go` + `internal/wire/routes_snapshot.txt` | **FAIL** (snapshot test) | T-O2 + hygiene-audit §5: every Echo route registration is in the curated snapshot. Drift triggers manual coordinator review. |
| Foundry public rate-limit pin | `internal/wire/foundry_public_ratelimit_test.go` | **FAIL** | T-B1 + security-audit §2 M-3: the Foundry public manifest endpoint MUST be rate-limited. Two AST assertions pin the wiring (`g.Use(rateLimit)` in `foundry_vtt.RegisterPublicRoutes`) and the call site (`middleware.RateLimit(...)` argument in `app.RegisterRoutes`). |
| Decision-citations | `tools/check-decision-citations.sh` | **WARN** (always exit 0) | T-O3 + meta-audit Phase 2: every `cordinator/decisions/*.md` is referenced from at least one piece of code, dispatch, report, or other decision |

### Extending the wire-contract snapshot

When a PR intentionally adds, removes, or changes Echo routes:

```bash
UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...
```

Commit the regenerated `internal/wire/routes_snapshot.txt` in the same PR.
The PR description must cite the decision/audit that motivated the route
change, especially when it touches one of the four auth surfaces named in
`cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md` §5.1.

### Extending the plugin-isolation guard

Today's guard targets `foundry-vtt` strings. After NW-2.1 plugin-isolation
audit lands and NW-2.2 cleanup removes the 161 existing violations, the
guard's scope generalizes to every plugin's name + the WARN-not-FAIL mode
for non-foundry paths flips to FAIL. The fragment-join token pattern (same
as `tools/check-no-instance-hostname.sh`) lets the script scan its own
directory tree without false-positiving on itself.

### Phase 2B follow-ups for wire-contract test

The Phase 2A snapshot captures `(method, path, file)` tuples via static AST
extraction. Documented limitations (will lift in a future Phase 2C with
`golang.org/x/tools/go/packages`):

1. Group prefix not resolved — `e.Group("/admin")` rename doesn't trigger drift.
2. Auth surface not classified — dispatch spec'd `(method, path, auth)`; we
   ship `(method, path, file)` for Phase 2A.
3. Programmatic registration (loops, builders) not captured.
4. Middleware chain NOT captured per-route — Phase 2A pins the curated
   list of registered routes; it doesn't catch silent removal of middleware
   from existing routes. C-SEC-CHUNK-2 (Phase 2B) closes this hole for the
   specific Foundry public manifest invariant (M-3) via a focused AST
   assertion at `internal/wire/foundry_public_ratelimit_test.go`; full
   per-route middleware capture is deferred to Phase 2C.

### Adding a focused middleware-pin test

When a security finding requires that a specific route's middleware never
silently disappear (e.g. rate-limit on a public endpoint, auth on an
admin-only route), prefer a focused AST assertion over a full type-resolved
walk. The Foundry public rate-limit pin (NW-2.2 era, C-SEC Chunk 2) is the
reference implementation; copy its shape:

1. Locate the function declaration that wires the middleware
2. AST-walk its body asserting the `*.Use(...)` (or per-route equivalent)
   call exists
3. Locate the call site that supplies the middleware argument
4. AST-walk asserting the argument is a non-nil call expression containing
   the expected middleware name

Two small assertions, no new dependencies, pin the invariant end-to-end.

## Cross-plugin import discipline

### For humans

Plugins are physically isolated under `internal/plugins/<slug>/`. Cross-plugin
communication is **always** mediated by an exported Go interface (a service or a
middleware) defined on the providing plugin. Importing another plugin's
repository, store, internal struct, or `_test.go` helpers is a layering
violation.

The shape that works:

```go
// In plugin-A's package: define the interface YOU need from plugin-B.
type CampaignService interface {
    Get(ctx context.Context, id string) (*Campaign, error)
}

// In plugin-B: implement it and expose via NewService(...) campaigns.CampaignService.

// In plugin-A's wiring: accept the interface, not the concrete type.
type Handler struct {
    campaignSvc campaigns.CampaignService
}
```

This pattern is already CLAUDE.md rule 8: *"Plugins talk to each other via
service interfaces, never direct repo access."* This section formalizes it as
the architectural-enforcement convention for Pillar 2 (`decisions/2026-05-21-four-pillars.md`).

### For AI sessions

**Verified clean.** The plugin-isolation audit (`cordinator/reports/chronicle/2026-05-23-c-plugin-isolation-audit.md §1.2`) walked every cross-plugin `import` in `internal/` and found **zero** suspicious-internal or suspicious-cross-plugin imports. All ~157 cross-plugin imports are exactly the legit-service / legit-middleware pattern above. The `internal/app/routes.go` is the registrar — the one place where imports of every plugin's package are expected and correct.

**Implications:**

1. Adding a new cross-plugin import requires the imported plugin to expose an interface (or a middleware constructor returning `echo.MiddlewareFunc`). Importing a concrete type from another plugin is a violation.
2. `internal/app/routes.go` is the ONLY package that imports every plugin's package. New plugins register here.
3. `foundry_vtt` is imported by `internal/websocket/{auth,client,hub}.go` for `foundry_vtt.ModuleSource` (NW-2.2 Chunk B). This is a thin const-usage import, not a cross-plugin "talk to service" import — analogous to how `packages.PackageTypeFoundryModule` is referenced from plugins that need to dispatch on package type. The "service-interface-only" rule is about behavioral coupling (calling methods); const sharing is acceptable.

**Regression-prevention mechanisms:**

| Mechanism | Catches |
|---|---|
| `tools/check-plugin-isolation.sh` (CI, diff-scoped FAIL) | New `foundry-vtt` / `foundry-module` magic-string literals outside `internal/plugins/foundry_vtt/`. Scope generalizes to other plugins per NW-2.3. |
| Wire-contract conformance test (`internal/wire/wire_contract_test.go`) | New routes outside the curated snapshot. Forces dispatch citation when a new cross-plugin route lands. |
| Code review | New `import "github.com/keyxmakerx/chronicle/internal/plugins/<X>/<subpkg>"` paths — anything beyond `internal/plugins/<X>` itself (i.e. importing `internal/plugins/<X>/repository`) is the canonical "you bypassed the interface" smell. |

**Reference:** `cordinator/reports/chronicle/2026-05-23-c-plugin-isolation-audit.md §1.2, §3 Chunk C` (the audit that verified this convention is already honored — this section is the regression-prevention documentation that locks in the verified state).

## Security

### For humans

Chronicle's security posture is the consolidated state after the C-SECURITY-AUDIT (2026-05-22) and the C-SEC chunks 1-7 that implemented its findings. This section is the standing reference every PR that touches an auth surface, a sanitization site, a signed URL, or a SQL identifier interpolation should read FIRST.

Per `cordinator/decisions/2026-05-21-core-tenets.md §T-B1`, security is the highest-priority tenet — every dispatch, audit chunk, and implementation PR considers security first. The mechanisms below are the operational consequences of that tenet.

### For AI sessions

When a PR touches any of the surfaces in this section, the PR description MUST include a Security-implication line per the audit's discipline. If the surface change is a regression risk, the corresponding CI guard (listed throughout this section) catches it; the guard is the load-bearing mechanism.

### Auth surfaces — four canonical shapes

Chronicle exposes **four distinct auth surfaces**. Conflating them is the chronicle#323 risk pattern that motivated the wire-contract conformance test (PR #330). Full inventory in `cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §5.1`.

| Surface | Mounting | Middleware | Consumers |
|---|---|---|---|
| **Session-cookie (web UI)** | `internal/app/routes.go` (campaigns, entities, calendar UI routes) | `auth.RequireAuth(authSvc)` + `campaigns.RequireCampaignAccess(campaignSvc)` | Browser users with `chronicle_session` cookie |
| **Per-campaign-token (legacy public API)** | `internal/plugins/calendar/api_routes.go` + `internal/plugins/foundry_vtt/routes.go::RegisterPublicRoutes` | Token validation via signed URL `?token=` param | Foundry module manifest fetch + calendar public surface |
| **Session-OR-Bearer (syncapi surface)** | `internal/plugins/syncapi/routes.go::RegisterAPIRoutes` | `syncapi.RequireAuthOrAPIKey` — accepts EITHER `chronicle_session` cookie OR `Authorization: Bearer <apiKey>` | Foundry sync REST API + in-app browser widgets |
| **Admin-session (site admin UI)** | `internal/app/routes.go` (admin group) | `auth.RequireAuth` + `auth.RequireSiteAdmin` + optional `auth.RequireReauth` for sensitive actions | Site admin browser users |

**Regression-prevention:** the wire-contract conformance test (`internal/wire/wire_contract_test.go` + `routes_snapshot.txt`) pins every Echo route registration. Adding a new route forces a snapshot regen + PR-description citation. Phase 2A captures `(method, path, file)`; per-route middleware capture is deferred to a Phase 2C upgrade (see "Phase 2B follow-ups for wire-contract test" above).

### CSRF

Double-submit cookie pattern via `internal/middleware/csrf.go`. On HTTPS the cookie uses the `__Host-` prefix for hardening. Applied to every session-cookie-authed write endpoint; Bearer-authed endpoints (syncapi) skip CSRF because cross-origin Bearer callers don't carry the cookie.

CSP allows `'unsafe-inline'` + `'unsafe-eval'` — the Alpine.js trade-off — mitigated by the server-side `sanitize.HTML` bluemonday wrapper on every user-controlled HTML field (see "Sanitization invariant" below).

Per `cordinator/reports/chronicle/2026-05-22-c-security-audit.md §2 G-CSRF` for the full inventory.

### Signed URLs — HMAC-SHA256 with `crypto/subtle`

Two signed-URL families:

- **`/media/...` (media plugin)** — `internal/plugins/media/handler.go` mints + verifies signed URLs for protected media. Uses a shared instance secret. Domain-prefix scoping per the `C-MEDIA-SIGNED-URL-TRUST` (chronicle#322) decision: signed URLs ARE the auth proof for cross-origin consumers.
- **`/foundry-vtt/...` (foundry_vtt plugin)** — `internal/plugins/foundry_vtt/token.go` mints per-campaign signed manifest URLs. `tokenDomain = "foundry-vtt"` const scopes the HMAC so a media-signed URL can't be replayed as a manifest URL (different domain prefix).

Both use `hmac.Equal` (constant-time comparison via `crypto/subtle`) — never `==` or `bytes.Equal`. Verification includes expiry checks; replayed/expired URLs reject with 403.

### Sanitization invariant — bluemonday UGCPolicy on every HTML write

Every plugin's `Service.Create*` / `Service.Update*` method that accepts an HTML-typed field calls `sanitize.HTML(...)` (defined in `internal/sanitize/sanitize.go`) before persisting. Per audit §1.3, 8 plugins/widgets follow this convention today:

- `internal/plugins/{entities,calendar,sessions,timeline,campaigns}/service.go`
- `internal/widgets/{notes,posts,entity_notes}/service.go`

**Regression-prevention (Chunk 7, PR #340):** `internal/sanitize/invariant_test.go` + `sanitize_invariant_snapshot.txt` pin a file-level invariant — any `service.go` (+ its sibling `model.go`) that declares HTML-typed inputs MUST have at least one `sanitize.HTML` call. The snapshot inventories all 25 plugin/widget service.go files; regenerate via `UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...`.

**Egress side:** per the §0.5 D4=(c) decision the backup/restore round-trip is intentionally lossless (no re-sanitization on export). The Foundry-bound egress paths are not yet re-sanitized — the audit deferred this to a separate chunk per the C-SEC-CHUNK-6 stop-and-flag (the actual Foundry-bound egress lives in `/api/v1/` entity/note handlers, not in `export_adapters.go` as the audit initially located).

### `SafeIdent` convention — DDL identifier interpolation

Per `cordinator/reports/chronicle/2026-05-22-c-security-audit.md §2 M-2` (Chunk 5, PR #331), every SQL DDL statement that interpolates a table/column name into the SQL string MUST pass the identifier through `internal/database/safeident.go::SafeIdent`. Returns the identifier backtick-quoted, or an error if it doesn't match the conservative regex `^[a-zA-Z_][a-zA-Z0-9_]*$`.

```go
quoted, err := database.SafeIdent(tableName)
if err != nil { return err }
_, err = db.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoted)
```

Today's only live caller is `internal/extensions/migration_runner.go::DropExtensionTables`. Future callers that need DDL identifier interpolation MUST use this helper rather than raw concat — even when the input is "trusted" (e.g. from `SHOW TABLES`). The helper's job is to make the safety mechanical rather than convention-only.

### Historical footguns — the four medium findings

Documented for future contributors who might re-introduce these patterns. Each finding's mitigation is now load-bearing in CI or in convention; the historical context explains WHY the mechanism exists.

| Finding | Original gap | Mitigation | PR |
|---|---|---|---|
| **M-1** | Password-reset Debug logs emitted raw email as `slog.String("email", email)` — email enumeration via log access | `internal/plugins/auth/loghash.go::hashEmail()` — SHA-256 hex prefix; regression-pinned by `loghash_test.go` | #331 (Chunk 1) |
| **M-2** | `DropExtensionTables` interpolated `table` name into `DROP TABLE` via raw concat | `internal/database/safeident.go::SafeIdent` — regex-validating identifier helper; convention enforced going forward | #331 (Chunk 5) |
| **M-3** | Foundry public manifest endpoint's rate-limit middleware was optional in registration signature; silent removal possible | `internal/wire/foundry_public_ratelimit_test.go` — two focused AST assertions pin the wiring + the call site | #339 (Chunk 2) |
| **M-4** | Sanitization on ingress but not on Foundry-bound egress | Egress side partially deferred per Chunk 6 stop-and-flag; the audit's cited sites were all backup-bound (lossless per D4=(c)); the actual Foundry-bound egress in syncapi handlers awaits a follow-up dispatch | (deferred — see `cordinator/reports/chronicle/2026-05-23-c-sec-chunk-6.md`) |

### Deferred Wave 2 follow-ups (not yet shipped)

For completeness, the security work that's authored but pending:

- **C-SEC-CHUNK-3** (Content-Type enforcement on JSON APIs) — deferred; multipart `POST /api/v1/.../media` is inside the `/api/v1/*` group, contradicting the dispatch's "Out of scope" assumption. Operator decision required (D-C3.1 per `cordinator/reports/chronicle/2026-05-23-c-sec-chunk-3.md`).
- **C-SEC-CHUNK-4** (loadDescriptor fallback decision doc + pinning test) — authored but not yet executed; cross-repo (Chronicle + Cordinator + Foundry-module comment).
- **C-SEC-CHUNK-6** (selective Foundry-egress sanitization) — deferred; audit's cited sites were all backup-bound. Operator decision required (D-C6.1 per `cordinator/reports/chronicle/2026-05-23-c-sec-chunk-6.md`).
- **C-SEC-CHUNK-2-PHASE-2C** — full middleware-chain capture for every route via `golang.org/x/tools/go/packages`. Deferred from PR #339's reshape.
- **C-SEC-CHUNK-7-PHASE-2** — method-level sanitize invariant with flow analysis + helper tracing. Deferred from PR #340's reshape.

### Reading order for a security-touching PR

1. This section (`.ai/conventions.md` §Security) — start here
2. `cordinator/decisions/2026-05-21-core-tenets.md §T-B1` — the binding tenet
3. `cordinator/reports/chronicle/2026-05-22-c-security-audit.md` — full audit (§1.3 sanitization inventory + §2 findings + §3 guardrails + §5 chunk roadmap)
4. `cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §5.1` — auth surfaces detail
5. The plugin's `.ai.md` for plugin-specific footguns
6. The relevant CI guard's source (`tools/check-plugin-isolation.sh`, `internal/wire/wire_contract_test.go`, `internal/sanitize/invariant_test.go`, etc.) to understand what the guard catches
