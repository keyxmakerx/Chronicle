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
