# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Audit wiring, storage limit enforcement, tag picker widget

## Current Phase
**Phase 2: Media & UI** -- Building on the Phase 1 foundation. Media plugin
for file uploads, security hardening, dynamic sidebar, entity image upload,
UI quality improvements, sidebar customization, visual template editor for
entity type page layouts, public campaign support, dark mode, tags, and audit logging.

## Last Session Summary

### Completed
- **Wired audit logging into handlers:** Entity, campaign, and tag handlers now
  emit audit events after successful mutations. Entity handler fires
  `entity.created`, `entity.updated`, `entity.deleted` on CRUD and entry saves.
  Campaign handler fires `campaign.updated`, `member.joined`, `member.left`,
  `member.role_changed`. Tag handler fires `tag.created`, `tag.deleted`.
  Uses fire-and-forget pattern: audit failures are logged but never block.
  Campaign handler uses `AuditLogger` interface + adapter to avoid circular import
  (audit → campaigns import cycle).
- **Wired storage limit enforcement in media uploads:** Media service now checks
  dynamic storage limits from the settings plugin at upload time. Three checks:
  per-file size limit (from settings), campaign total storage quota, and campaign
  file count limit. New `StorageLimiter` interface in media package, implemented
  via adapter wrapping `settings.SettingsService` in routes.go. New
  `GetCampaignUsage()` repo method for quota checks. Graceful degradation: if
  settings lookup fails, upload is allowed (logged warning).
- **Tag picker widget:** New `static/js/widgets/tag_picker.js` with full
  tag management UI. Shows entity tags as colored chips with remove buttons.
  Dropdown with search/filter to add existing tags or create new ones inline.
  Auto-mounted via boot.js on `data-widget="tag-picker"` elements. Added
  `blockTags` component to entity show template (both in block dispatcher and
  fallback layout). Tags block also available in template editor palette.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

### Files Modified This Session
- `internal/plugins/entities/handler.go` -- Added auditSvc field, logAudit helper, audit calls after CRUD
- `internal/plugins/campaigns/handler.go` -- Added AuditLogger interface, auditLogger field, audit calls
- `internal/widgets/tags/handler.go` -- Added auditSvc field, logAudit helper, audit calls
- `internal/plugins/media/service.go` -- Added StorageLimiter interface, checkQuotas method, SetStorageLimiter
- `internal/plugins/media/repository.go` -- Added GetCampaignUsage method
- `internal/plugins/entities/model.go` -- Added "tags" to valid block types comment
- `internal/app/routes.go` -- Added campaignAuditAdapter, storageLimiterAdapter, wired all audit+limits
- `internal/plugins/entities/show.templ` -- Added blockTags component, "tags" block type in dispatcher
- `internal/templates/layouts/base.templ` -- Added tag_picker.js script tag
- `static/js/widgets/tag_picker.js` -- New: tag picker widget (search, create, assign, remove)
- `static/js/widgets/template_editor.js` -- Added "tags" block type to palette

## Active Branch
`claude/resume-previous-work-YqXiG`

## Next Session Should
1. **@mentions** -- Search entities in editor, insert link, parse/render server-side
2. **Password reset** -- Wire auth password reset with SMTP when configured
3. **Entity relations** -- Bi-directional entity linking
4. **Entity type CRUD** -- Let campaign owners add/edit/remove entity types
5. **Regenerate Tailwind CSS** -- Run `make tailwind` to include new dark: and component classes
6. **Tag display on entity list cards** -- Show tag chips on entity cards in list view

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
- 2026-02-20: Dark mode toggle, collapsible admin sidebar, theme.js
- 2026-02-20: Tags widget plugin (campaign-scoped entity tagging)
- 2026-02-20: Audit log plugin (campaign activity timeline, stats)
- 2026-02-20: Site settings plugin (editable storage limits, per-user/campaign overrides)
- 2026-02-20: Public landing page with discoverable campaign cards
- 2026-02-20: Enhanced template editor (two-column, three-column, tabs, sections)
- 2026-02-20: Wired audit logging into entity, campaign, and tag mutation handlers
- 2026-02-20: Wired storage limit enforcement into media upload handler
- 2026-02-20: Tag picker widget (search, create, assign tags on entity profile pages)
