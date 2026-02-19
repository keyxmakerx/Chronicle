# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Campaigns, SMTP, and Admin plugins implementation session

## Current Phase
**Phase 1: Foundation** -- Core infrastructure, auth plugin, campaigns plugin,
SMTP plugin, and admin plugin are built. App compiles successfully. Next: entities
plugin, then editor widget.

## Last Session Summary

### Completed
- **Campaigns plugin fully implemented:**
  - Migration 000002 (campaigns, campaign_members, ownership_transfers tables)
  - `model.go` -- Campaign, CampaignMember, Role system (Player=1, Scribe=2, Owner=3),
    CampaignContext with dual permission model, OwnershipTransfer, UserFinder interface,
    MailService interface, all DTOs, Slugify helper
  - `repository.go` -- Full CampaignRepository (CRUD, membership CRUD, transfer CRUD,
    TransferOwnership with atomic DB transaction, ForceTransferOwnership for admin)
  - `user_finder.go` -- UserFinderAdapter wrapping auth.UserRepository
  - `service.go` -- CampaignService with CRUD, slug generation with dedup, membership
    validation, ownership transfer flow (72h tokens, optional email), admin operations
  - `middleware.go` -- RequireCampaignAccess (resolves campaign + membership + admin),
    RequireRole (checks MemberRole, no admin bypass for content)
  - `handler.go` -- 16 thin handlers (Index, NewForm, Create, Show, EditForm, Update,
    Delete, Settings, Members, AddMember, RemoveMember, UpdateRole, TransferForm,
    Transfer, AcceptTransfer, CancelTransfer)
  - `routes.go` -- Route registration with middleware chains
  - Templ templates: index, campaign_card, form, show, settings, members
- **SMTP plugin fully implemented:**
  - Migration 000003 (smtp_settings singleton table)
  - `model.go` -- SMTPSettings (HasPassword bool, never exposes password), smtpRow, DTOs
  - `crypto.go` -- AES-256-GCM encrypt/decrypt with SHA-256(SECRET_KEY)
  - `repository.go` -- Get/Upsert singleton row
  - `service.go` -- MailService (SendMail, IsConfigured) + SMTPService (settings mgmt,
    test connection). Supports STARTTLS, SSL, and plain modes.
  - `handler.go` -- Settings GET/PUT, TestConnection POST
  - `settings.templ` -- SMTP form with password handling (never shows value)
  - `routes.go` -- Under /admin/smtp group
- **Admin plugin fully implemented:**
  - `handler.go` -- Dashboard, Users, ToggleAdmin, Campaigns, DeleteCampaign,
    JoinCampaign, LeaveCampaign
  - `routes.go` -- /admin group with auth + RequireSiteAdmin middleware
  - Templates: dashboard, users, campaigns
- **Auth plugin extended:**
  - `repository.go` -- Added ListUsers, UpdateIsAdmin, CountUsers for admin
  - `middleware.go` -- Added RequireSiteAdmin middleware
- **Route wiring:** All three new plugins wired in app/routes.go with full DI.
  Dashboard redirects to /campaigns. SMTP MailService injected into campaigns.
- **Build succeeds:** `go build ./...` and `go vet ./...` pass with zero errors

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Created This Session
- `db/migrations/000002_create_campaigns.up.sql`, `000002_create_campaigns.down.sql`
- `db/migrations/000003_create_smtp_settings.up.sql`, `000003_create_smtp_settings.down.sql`
- `internal/plugins/campaigns/` -- model.go, repository.go, user_finder.go, service.go,
  middleware.go, handler.go, routes.go, index.templ, campaign_card.templ, form.templ,
  show.templ, settings.templ, members.templ + generated _templ.go files
- `internal/plugins/smtp/` -- model.go, crypto.go, repository.go, service.go, handler.go,
  routes.go, settings.templ + generated _templ.go
- `internal/plugins/admin/` -- handler.go, routes.go, dashboard.templ, users.templ,
  campaigns.templ + generated _templ.go files

### Files Modified This Session
- `internal/app/routes.go` -- Wired campaigns, SMTP, admin plugins
- `internal/plugins/auth/repository.go` -- Added ListUsers, UpdateIsAdmin, CountUsers
- `internal/plugins/auth/middleware.go` -- Added RequireSiteAdmin

## Active Branch
`claude/setup-ai-project-docs-LhvVz`

## Next Session Should
1. **Entities plugin** -- implement entity types + entity CRUD:
   - Entity + EntityType models, migration 000004
   - Seed default entity types (Character, Location, Organization, Item, etc.)
   - CRUD + entity profile page with fields
   - Entity search (MariaDB FULLTEXT)
2. **Editor widget** -- TipTap integration:
   - TipTap vendored JS bundle
   - editor.js widget with Chronicle.register()
   - boot.js widget auto-mounter
   - Save/load entity entry content
3. **UI & Layouts** -- Authenticated app layout:
   - Sidebar navigation (campaign entities, collapsible)
   - Topbar (user menu, campaign selector, search)
   - Tailwind CSS styling
4. **Password reset** -- Wire auth password reset with SMTP when configured

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- HTMX and Alpine.js are loaded from CDN -- should be vendored for self-hosted
- Tailwind CSS output (`static/css/app.css`) doesn't exist yet -- needs
  `tailwindcss` binary to generate it
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- SMTP migration assumes smtp_settings table exists before first admin access
  (migration must be applied)

## Recently Completed Milestones
- 2026-02-19: Project scaffolding and three-tier AI documentation system
- 2026-02-19: Core infrastructure (config, database, middleware, app, server)
- 2026-02-19: Security middleware (proxy trust, CORS, CSRF, security headers)
- 2026-02-19: Auth plugin (register, login, logout, session management)
- 2026-02-19: Campaigns plugin (CRUD, roles, membership, ownership transfer)
- 2026-02-19: SMTP plugin (encrypted password, STARTTLS/SSL, test connection)
- 2026-02-19: Admin plugin (user management, campaign oversight, SMTP config)
