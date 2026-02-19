# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- UI & Layouts session (dynamic sidebar, topbar, pagination, flash messages)

## Current Phase
**Phase 1: Foundation** -- Core infrastructure, auth, campaigns, SMTP, admin,
entities plugins, and **UI layouts** are built. App compiles successfully.
Next: editor widget (TipTap), then remaining polish.

## Last Session Summary

### Completed
- **UI & Layouts overhaul:**
  - `layouts/data.go` -- Layout data context helpers (typed setters/getters for user,
    campaign, CSRF, flash messages, active path) using Go's `context.Context`
  - `layouts/app.templ` -- Fully redesigned:
    - **Dynamic sidebar:** Shows campaign entity type links (Characters, Locations,
      Organizations, Items, Notes, Events) with colored dot indicators when in
      campaign context. Shows generic "My Campaigns" nav when not in a campaign.
      Includes Members, Settings links, "All Campaigns" back link, and Admin link
      (for site admins). Active state highlighting via path matching.
    - **Dynamic topbar:** User avatar with initials, user name, admin link (if admin),
      campaign-scoped entity search (HTMX), CSRF-protected logout form.
    - **Flash messages:** Success/error flash messages that auto-dismiss via Alpine.js
      (5s for success, 8s for error) with manual close button.
  - `middleware/helpers.go` -- Added `LayoutInjector` callback pattern to `Render()`.
    Before every template render, copies auth session and campaign context data from
    Echo's context into Go's `context.Context` for Templ to read.
  - `app/routes.go` -- Registered the LayoutInjector callback that copies: user name,
    email, admin status, campaign ID/name/role, CSRF token, and active path.
  - `components/pagination.templ` -- Shared pagination component with `PaginationData`
    struct. Supports HTMX partial swap via optional `HTMXTarget`. Shows prev/next
    buttons with page N of M indicator. Used by campaigns and entities list pages.
  - Updated `campaigns/index.templ` and `entities/index.templ` to use shared pagination
  - **Error page improvements (previous session):** Contextual error pages with
    color-coded badges, type-specific titles, smart navigation links, HTMX support
- **Doc fixes:** Updated admin/.ai.md and smtp/.ai.md integration checkboxes
- **Build succeeds:** `go build ./...` and `go vet ./...` pass with zero errors

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Created This Session
- `internal/templates/layouts/data.go` -- Context helpers for layout data
- `internal/templates/components/pagination.templ` -- Shared pagination component

### Files Modified This Session
- `internal/templates/layouts/app.templ` -- Complete redesign with dynamic sidebar/topbar
- `internal/middleware/helpers.go` -- LayoutInjector callback in Render
- `internal/app/routes.go` -- Layout injector registration, layouts import
- `internal/plugins/entities/index.templ` -- Replaced inline pagination with component
- `internal/plugins/campaigns/index.templ` -- Replaced inline pagination with component
- `internal/plugins/admin/.ai.md` -- Fixed integration checkbox
- `internal/plugins/smtp/.ai.md` -- Fixed integration checkbox
- `.ai/todo.md` -- Updated UI & Layouts items

## Active Branch
`claude/setup-ai-project-docs-LhvVz`

## Next Session Should
1. **Editor widget** -- TipTap integration:
   - TipTap vendored JS bundle
   - editor.js widget with Chronicle.register()
   - boot.js widget auto-mounter
   - Save/load entity entry content via API
2. **Campaign selector** -- Dropdown in topbar to switch between campaigns
3. **Password reset** -- Wire auth password reset with SMTP when configured
4. **Tests** -- Unit tests for entities service and repository
5. **Tailwind CSS** -- Generate app.css (requires tailwindcss binary)
6. **Vendor libraries** -- HTMX + Alpine.js (currently CDN)

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
- 2026-02-19: UI & Layouts (dynamic sidebar, topbar, pagination, flash messages, error pages)
