# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Fix template editor save, drop indicators, admin storage page

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization, visual template editor for
entity type page layouts, and public campaign support.

## Last Session Summary

### Completed
- **Fixed campaigns 500 error:** `ListByUser` and `ListAll` in
  `campaigns/repository.go` selected `is_public` but were missing
  `&c.IsPublic` in their `Scan()` calls (11 columns, 10 destinations).
- **Fixed template editor save:** Added missing `X-CSRF-Token` header to
  the save `fetch()` call in `template_editor.js`. The CSRF middleware was
  rejecting the PUT request with 403 because no token was sent.
- **Added drag-and-drop indicators:** Template editor now shows an animated
  indigo line between blocks when dragging, indicating exactly where the
  block will be inserted. Uses `te-block` class for position tracking,
  calculates insertion index from mouse Y position vs block midpoints, and
  correctly adjusts the index when reordering within the same column.
- **Admin storage management page:** New `/admin/storage` page with:
  - Overview stats (total storage, file count, per-file upload limit)
  - Usage type breakdown (entity images, attachments, avatars, backdrops)
  - Rate limits & restrictions info panel
  - Paginated file table with thumbnails, uploader name, delete action
  - New `GetStorageStats()` and `ListAll()` repository methods with JOINs
- **Storage in admin sidebar:** Added "Storage" link with hard-drive icon
  to the admin section of the left sidebar, between "All Campaigns" and
  "SMTP Settings".
- **Dashboard storage card:** Admin dashboard now shows a 4th stat card
  with total storage used and file count, linking to `/admin/storage`.
- **Moved admin nav to main sidebar:** (from previous session) Admin links
  appear directly in the far-left sidebar under an "Admin" section.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `internal/plugins/campaigns/repository.go` -- Added missing IsPublic to Scan calls
- `static/js/widgets/template_editor.js` -- CSRF header, drop indicators, position-aware insertion
- `internal/plugins/media/repository.go` -- StorageStats, AdminMediaFile types, GetStorageStats(), ListAll()
- `internal/plugins/admin/handler.go` -- SetMediaDeps(), Storage(), DeleteMedia() handlers
- `internal/plugins/admin/storage.templ` -- New storage management admin page
- `internal/plugins/admin/dashboard.templ` -- Added storage stat card, 4-column grid
- `internal/plugins/admin/routes.go` -- /admin/storage and /admin/media/:fileID routes
- `internal/templates/layouts/app.templ` -- Storage link in admin sidebar
- `internal/app/routes.go` -- Wire SetMediaDeps on admin handler

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **@mentions** -- Search entities in editor, insert link, parse/render server-side
2. **Password reset** -- Wire auth password reset with SMTP when configured
3. **Entity relations** -- Bi-directional entity linking
4. **Entity type CRUD** -- Let campaign owners add/edit/remove entity types
5. **Game system modules** -- Implement module registry, D&D 5e module, admin module settings
6. **Regenerate Tailwind CSS** -- Run `make tailwind` to include new safelist classes

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`

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
- 2026-02-19: CI/CD pipeline (GitHub Actions: build, test, Docker push to GHCR)
- 2026-02-19: Production deployment hardening (retry logic, real healthcheck, credential sync)
- 2026-02-19: Auto-migrations on startup, first-user-is-admin, /health alias
- 2026-02-19: Media plugin (upload, thumbnails, magic byte validation, rate limiting)
- 2026-02-19: Security hardening (IDOR fixes, HSTS, rate limiting on auth)
- 2026-02-19: Dynamic sidebar with entity types from DB + count badges
- 2026-02-19: Entity image upload pipeline + UI quality upgrade
- 2026-02-19: Sidebar customization (drag-to-reorder, hide/show entity types)
- 2026-02-19: Layout builder scaffold (two-column entity profile layout editor)
- 2026-02-19: Comprehensive security audit (14 vulnerability fixes across 14 files)
- 2026-02-19: Unified entity type config, color picker, public campaigns
- 2026-02-19: Visual template editor, layout-driven entity pages, admin nav with modules
- 2026-02-20: Fix campaigns 500 error, move admin nav to sidebar
- 2026-02-20: Fix template editor save, drop indicators, admin storage management page
