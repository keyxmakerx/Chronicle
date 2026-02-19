# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-19 -- Editor widget session (TipTap integration, boot.js, entry API)

## Current Phase
**Phase 1: Foundation** -- Core infrastructure, auth, campaigns, SMTP, admin,
entities plugins, UI layouts, and **editor widget** are built. App compiles
successfully. Next: @mentions, campaign selector, tests, deploy polish.

## Last Session Summary

### Completed
- **Editor widget (TipTap integration):**
  - `static/js/boot.js` -- Widget auto-mounter. Scans DOM for `data-widget`
    attributes, collects config from `data-*` attrs, and calls registered widget
    `init()`. Re-mounts after HTMX swaps via `htmx:afterSettle`. WeakMap prevents
    double-init. Destroys widgets before HTMX removes content.
  - `static/vendor/tiptap-bundle.min.js` -- Vendored TipTap 2.x bundle (345KB).
    Built via npm + esbuild from @tiptap/core, starter-kit, placeholder, link,
    underline. Exported as `window.TipTap` global.
  - `static/js/widgets/editor.js` -- Rich text editor widget. Registers via
    `Chronicle.register('editor', ...)`. Features: toolbar (bold, italic,
    underline, strike, H1-H3, lists, blockquote, code, hr, undo/redo, save),
    autosave (configurable interval, default 30s), status bar (saved/saving/
    unsaved/error), Ctrl+S keyboard shortcut, load/save via JSON API.
  - `static/css/input.css` -- Added editor widget CSS classes (chronicle-editor,
    toolbar, buttons, separator, content, status states).
  - Entry API endpoints: `GET /campaigns/:id/entities/:eid/entry` (load content),
    `PUT /campaigns/:id/entities/:eid/entry` (save content). JSON endpoints for
    the editor widget. Privacy-aware (Players get 404 for private entities).
  - `entities/service.go` -- Added `UpdateEntry` method for targeted entry saves.
  - `entities/repository.go` -- Added `UpdateEntry` SQL method.
  - `entities/handler.go` -- Added `GetEntry` and `UpdateEntryAPI` handlers.
  - `entities/routes.go` -- Registered entry API routes.
  - `entities/show.templ` -- Replaced static entry display with editor widget
    mount point (`data-widget="editor"` with endpoint, editable, autosave, csrf).
  - `layouts/base.templ` -- Enabled boot.js, added tiptap-bundle and editor.js
    script tags.
- **Build succeeds:** `go build ./...` and `go vet ./...` pass with zero errors

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Created This Session
- `static/js/boot.js` -- Widget auto-mounter
- `static/vendor/tiptap-bundle.min.js` -- Vendored TipTap bundle
- `static/js/widgets/editor.js` -- Rich text editor widget

### Files Modified This Session
- `static/css/input.css` -- Added editor widget CSS
- `internal/plugins/entities/service.go` -- Added UpdateEntry method
- `internal/plugins/entities/repository.go` -- Added UpdateEntry to interface + impl
- `internal/plugins/entities/handler.go` -- Added GetEntry + UpdateEntryAPI handlers
- `internal/plugins/entities/routes.go` -- Added entry API routes
- `internal/plugins/entities/show.templ` -- Editor widget mount point
- `internal/templates/layouts/base.templ` -- Enabled boot.js + vendor scripts
- `internal/plugins/entities/.ai.md` -- Updated routes + notes
- `.ai/todo.md` -- Marked editor widget items complete
- `.ai/api-routes.md` -- Added entry API routes
- `.ai/status.md` -- Updated for editor widget session

## Active Branch
`claude/setup-ai-project-docs-LhvVz`

## Next Session Should
1. **@mentions** -- Search entities, insert link, parse/render server-side
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
