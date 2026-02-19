# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Entities plugin implementation session

## Current Phase
**Phase 1: Foundation** -- Core infrastructure, auth, campaigns, SMTP, admin,
and **entities** plugins are built. App compiles successfully. Next: editor widget,
then UI/layouts polish.

## Last Session Summary

### Completed
- **Entities plugin fully implemented:**
  - Migration 000004 (entity_types + entities tables with FULLTEXT index)
  - `model.go` -- Entity, EntityType, FieldDefinition structs, CreateEntityRequest,
    UpdateEntityRequest, CreateEntityInput, UpdateEntityInput DTOs, ListOptions,
    Slugify helper
  - `repository.go` -- EntityTypeRepository (CRUD, SeedDefaults with 6 default types
    in transaction) + EntityRepository (CRUD, ListByCampaign with privacy filtering,
    Search with FULLTEXT + LIKE fallback, CountByType for sidebar badges, SlugExists)
  - `service.go` -- EntityService (CRUD with slug gen, privacy enforcement, search,
    entity type management, SeedDefaults). Also satisfies campaigns.EntityTypeSeeder.
  - `handler.go` -- 8 thin handlers (Index, NewForm, Create, Show, EditForm, Update,
    Delete, SearchAPI) with dynamic field parsing from form params (field_<key>)
  - `routes.go` -- Campaign-scoped routes with RequireCampaignAccess + RequireRole
    middleware. Shortcut routes: /characters, /locations, /organizations, /items,
    /notes, /events
  - Templ templates: index (type filter sidebar + grid), entity_card (type badge +
    privacy), show (profile with sidebar fields + main content), form (create/edit
    with dynamic fields), search_results (HTMX fragment)
- **Campaigns plugin extended:**
  - `model.go` -- Added EntityTypeSeeder interface (like UserFinder, MailService)
  - `service.go` -- Added `seeder EntityTypeSeeder` field, updated NewCampaignService
    to accept seeder, Create() now calls SeedDefaults after adding owner (non-fatal)
  - `show.templ` -- Updated Entities card from "Coming soon" to link to entities list
- **Route wiring updated:**
  - `app/routes.go` -- Entities repos/service created BEFORE campaigns service (DI order
    change). EntityService passed as EntityTypeSeeder to campaigns. Entity routes
    registered with campaign middleware.
- **Build succeeds:** `go build ./...` and `go vet ./...` pass with zero errors

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Created This Session
- `db/migrations/000004_create_entities.up.sql`, `000004_create_entities.down.sql`
- `internal/plugins/entities/` -- model.go, repository.go, service.go, handler.go,
  routes.go, index.templ, entity_card.templ, show.templ, form.templ,
  search_results.templ + generated _templ.go files

### Files Modified This Session
- `internal/plugins/campaigns/model.go` -- Added EntityTypeSeeder interface
- `internal/plugins/campaigns/service.go` -- Added seeder field, updated constructor,
  seed defaults in Create()
- `internal/plugins/campaigns/show.templ` -- Updated Entities card to link to entities
- `internal/app/routes.go` -- Wired entities plugin, reordered DI

## Active Branch
`claude/setup-ai-project-docs-LhvVz`

## Next Session Should
1. **Editor widget** -- TipTap integration:
   - TipTap vendored JS bundle
   - editor.js widget with Chronicle.register()
   - boot.js widget auto-mounter
   - Save/load entity entry content
2. **UI & Layouts** -- Authenticated app layout:
   - Dynamic sidebar navigation (entity types from campaign)
   - Topbar (user menu, campaign selector, search)
   - Tailwind CSS styling
3. **Password reset** -- Wire auth password reset with SMTP when configured
4. **Tests** -- Unit tests for entities service and repository

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- HTMX and Alpine.js are loaded from CDN -- should be vendored for self-hosted
- Tailwind CSS output (`static/css/app.css`) doesn't exist yet -- needs
  `tailwindcss` binary to generate it
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- SMTP migration assumes smtp_settings table exists before first admin access
  (migration must be applied)
- Entity entry content is currently stored/displayed as plain text -- needs
  TipTap editor widget for rich text

## Recently Completed Milestones
- 2026-02-19: Project scaffolding and three-tier AI documentation system
- 2026-02-19: Core infrastructure (config, database, middleware, app, server)
- 2026-02-19: Security middleware (proxy trust, CORS, CSRF, security headers)
- 2026-02-19: Auth plugin (register, login, logout, session management)
- 2026-02-19: Campaigns plugin (CRUD, roles, membership, ownership transfer)
- 2026-02-19: SMTP plugin (encrypted password, STARTTLS/SSL, test connection)
- 2026-02-19: Admin plugin (user management, campaign oversight, SMTP config)
- 2026-02-19: Entities plugin (CRUD, entity types, FULLTEXT search, privacy, dynamic fields)
