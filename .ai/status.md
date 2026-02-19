# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- UI polish, unit tests, Dockerfile fix session

## Current Phase
**Phase 1: Foundation** -- Core infrastructure, auth, campaigns, SMTP, admin,
entities plugins, UI layouts, editor widget, and **UI polish** are built. App
compiles and tests pass. Tailwind CSS generated. Dockerfile ready for production.
Next: @mentions, password reset, CI, final deploy testing.

## Last Session Summary

### Completed
- **SMTP settings page converted from dark to light theme:**
  - `smtp/settings.templ` -- Rewrote from dark (bg-gray-800) to light theme
    using `layouts.App()` wrapper, `card` component, `input` classes, `btn-primary`
    and `btn-secondary` buttons. Consistent with admin pages.
- **Landing page redesigned:**
  - `pages/landing.templ` -- Added Font Awesome book icon, larger heading (text-5xl),
    extended description, refined CTA buttons, self-hosted tagline footer.
- **Auth templates polished:**
  - `auth/login.templ` + `auth/register.templ` -- Replaced hardcoded `text-indigo-600`
    links with `text-accent` / `text-accent-hover` to use the theme system.
- **CSS component library expanded (`input.css`):**
  - Added `.link`, `.link-muted`, `.link-danger` link styles
  - Added `.badge`, `.badge-primary`, `.badge-gray`, `.badge-green`, `.badge-amber`,
    `.badge-red`, `.badge-blue` badge components
  - Added `.alert`, `.alert-error`, `.alert-success`, `.alert-warning`, `.alert-info`
    alert components
  - Added `.empty-state`, `.empty-state__icon`, `.empty-state__title`,
    `.empty-state__description` empty state component
  - Added `.table-header`, `.table-row` table component utilities
  - Added `.htmx-indicator` HTMX loading indicator utility
  - Enhanced `.btn` with `focus:ring-2 focus:ring-offset-2` focus states
  - Enhanced `.input` with `bg-white text-gray-900 placeholder-gray-400` + focus ring
  - Added `.btn:disabled` and `.input:disabled` states
- **Tailwind CSS regenerated:**
  - Generated app.css with all new component classes included
- **Entity service unit tests (30 tests, all passing):**
  - `entities/service_test.go` -- Full mock-based test suite covering:
    - Create: success, empty name, whitespace, too long, invalid type, wrong campaign,
      slug dedup (-2/-3), name trimming, nil FieldsData defaults
    - Update: success, empty name, slug regen on name change, slug preserved when
      unchanged, TypeLabel set/clear
    - UpdateEntry: success, empty content, whitespace only, repo error propagation
    - Delete: success, repo error propagation
    - List: default pagination, per-page clamping
    - Search: minimum query length, trimming, valid query delegation
    - Entity types: delegation, SeedDefaults delegation
    - Slugify: 9 table-driven cases (simple, spaces, special chars, unicode, empty)
    - ListOptions: Offset calculation for 5 cases (page 1, 2, 3, 0, negative)
- **Dockerfile fixed for production build:**
  - Upgraded Go build stage from 1.22 to 1.24 (matches go.mod 1.24.7)
  - Upgraded Alpine from 3.19 to 3.20
  - Pinned Tailwind CSS to v3.4.17 (was `latest`, which can break)
- **Build verified:** `go build ./...`, `go vet ./...`, `go test ./...` all pass

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Created This Session
- `internal/plugins/entities/service_test.go` -- 30 unit tests for entity service

### Files Modified This Session
- `internal/plugins/smtp/settings.templ` -- Dark → light theme rewrite
- `internal/templates/pages/landing.templ` -- Visual hierarchy improvements
- `internal/plugins/auth/login.templ` -- accent color for Register link
- `internal/plugins/auth/register.templ` -- accent color for Sign in link
- `static/css/input.css` -- Expanded CSS component library
- `static/css/app.css` -- Regenerated Tailwind output
- `Dockerfile` -- Go 1.24, Alpine 3.20, pinned Tailwind version
- `.ai/status.md` -- Updated for UI polish session
- `.ai/todo.md` -- Marked items complete

## Active Branch
`claude/setup-ai-project-docs-LhvVz`

## Next Session Should
1. **@mentions** -- Search entities, insert link, parse/render server-side
2. **Password reset** -- Wire auth password reset with SMTP when configured
3. **CI pipeline** -- GitHub Actions for build, lint, test
4. **Deploy testing** -- Verify Docker Compose full stack works end-to-end

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
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
- 2026-02-19: Entities plugin (CRUD, entity types, FULLTEXT search, privacy, dynamic fields)
- 2026-02-19: UI & Layouts (dynamic sidebar, topbar, pagination, flash messages, error pages)
- 2026-02-19: Editor widget (TipTap integration, boot.js auto-mounter, entry API)
- 2026-02-19: Vendor HTMX + Alpine.js, campaign selector dropdown
- 2026-02-19: UI polish (light theme unification, CSS component library, landing page)
- 2026-02-19: Entity service unit tests (30 tests passing)
- 2026-02-19: Dockerfile fixed for production (Go 1.24, pinned Tailwind)
