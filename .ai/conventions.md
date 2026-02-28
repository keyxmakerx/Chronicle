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

```go
// isHTMX returns true if this request was initiated by HTMX.
func isHTMX(c echo.Context) bool {
    return c.Request().Header.Get("HX-Request") == "true"
}

// render writes a Templ component to the response with the given status code.
func render(c echo.Context, status int, component templ.Component) error {
    c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
    c.Response().WriteHeader(status)
    return component.Render(c.Request().Context(), c.Response().Writer)
}
```

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

### SQL Migration Headers

```sql
-- Migration: 000002_create_campaigns
-- Description: Creates campaigns table for worldbuilding containers.
-- Related: ADR-001

CREATE TABLE campaigns ( ... );
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
