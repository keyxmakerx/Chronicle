# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Phase 2: Media plugin, security hardening, dynamic sidebar, UI upgrade

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads and image management, security hardening (IDOR fixes, HSTS,
rate limiting, magic byte validation), dynamic sidebar from DB, entity image
upload, and UI quality improvements with Font Awesome icons. All tests pass.

## Last Session Summary

### Completed
- **Media plugin:** Full implementation with model, repository, service, handler,
  routes, and migration 000005. Upload with magic byte validation (JPEG/PNG/WebP/GIF),
  UUID filenames in YYYY/MM/ directory structure, thumbnail generation at 300px + 800px
  using golang.org/x/image (Catmull-Rom interpolation). Serve with Cache-Control:
  immutable. Rate-limited uploads (30/min per IP).
- **Rate limiting middleware:** Per-IP sliding window counter with background cleanup.
  Applied to auth routes (login: 10/min, register: 5/min) and media upload.
- **IDOR security fixes:** All entity handlers (Show, EditForm, Update, Delete,
  GetEntry, UpdateEntryAPI, UpdateImageAPI) now verify entity.CampaignID matches
  the campaign from the URL before proceeding.
- **HSTS header:** Added Strict-Transport-Security (1 year, includeSubDomains)
  to security middleware.
- **Editor save button:** Made prominent with bg-gray-200 default, accent color
  highlight via .has-changes class when content is modified.
- **Entity image upload:** Full pipeline: UpdateImage in repository/service/handler,
  PUT /campaigns/:id/entities/:eid/image route, image_upload.js widget that
  uploads to media then sets path on entity. Show page displays image with
  hover overlay for editors.
- **Dynamic sidebar:** Entity types loaded from DB into layout context
  (SidebarEntityType struct in data.go, LayoutInjector populates from
  entityService.GetEntityTypes). Sidebar renders Font Awesome icons and entity
  count badges per type.
- **UI quality upgrade:** Entity cards show image thumbnails (or type icon
  placeholder), Font Awesome icons on type badges, smooth hover transitions.
  Index page type filter uses FA icons. Improved empty state. Breadcrumbs
  on entity show page include type link.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `db/migrations/000005_create_media.up.sql` -- New: media_files table + campaigns.backdrop_path
- `db/migrations/000005_create_media.down.sql` -- New: rollback
- `internal/plugins/media/model.go` -- New: MediaFile, UploadInput, AllowedMimeTypes
- `internal/plugins/media/repository.go` -- New: CRUD with JSON thumbnail paths
- `internal/plugins/media/service.go` -- New: upload, validate, thumbnails, delete
- `internal/plugins/media/handler.go` -- New: Upload, Serve, ServeThumbnail, Info, Delete
- `internal/plugins/media/routes.go` -- New: media routes with auth + rate limiting
- `internal/middleware/ratelimit.go` -- New: per-IP rate limiter
- `internal/middleware/security.go` -- Added HSTS header
- `internal/config/config.go` -- Added MediaPath to UploadConfig
- `internal/app/routes.go` -- Wired media plugin, entity types in LayoutInjector
- `internal/plugins/auth/routes.go` -- Rate limiting on login/register
- `internal/plugins/entities/model.go` -- ImagePath in UpdateEntityInput
- `internal/plugins/entities/repository.go` -- UpdateImage method
- `internal/plugins/entities/service.go` -- UpdateImage method
- `internal/plugins/entities/handler.go` -- IDOR on all handlers + UpdateImageAPI
- `internal/plugins/entities/routes.go` -- Image API route
- `internal/plugins/entities/service_test.go` -- UpdateImage in mock
- `internal/plugins/entities/show.templ` -- Image display + upload widget + breadcrumbs
- `internal/plugins/entities/entity_card.templ` -- Image thumbnails + FA icons
- `internal/plugins/entities/index.templ` -- FA icons in type filter + empty state
- `internal/templates/layouts/data.go` -- SidebarEntityType, entity types/counts context
- `internal/templates/layouts/app.templ` -- Dynamic sidebar with FA icons + count badges
- `internal/templates/layouts/base.templ` -- image_upload.js script tag
- `static/css/input.css` -- Editor save button styles
- `static/js/widgets/image_upload.js` -- New: image upload widget
- `go.mod` / `go.sum` -- golang.org/x/image dependency

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **@mentions** -- Search entities in editor, insert link, parse/render server-side
2. **Password reset** -- Wire auth password reset with SMTP when configured
3. **Entity relations** -- Bi-directional entity linking
4. **Sidebar customization** -- Campaign-level sidebar config (migration 000006)
5. **Layout builder** -- Entity type layout_json for custom profile layouts (migration 000007)

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
