# Chronicle Backlog

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Single source of truth for what needs to be done, priorities,    -->
<!--          and what has been completed.                                     -->
<!-- Update: At the start of a session (to understand priorities), during      -->
<!--         work (to mark progress), and at session end (to reflect).        -->
<!-- Legend: [ ] Not started  [~] In progress  [x] Complete  [!] Blocked      -->
<!-- ====================================================================== -->

## Current Sprint: Phase 1 -- Foundation

**Target:** Working CRUD app with auth, campaign management, entity editor with
rich text, and basic Kanka-inspired UI. Deployable via Docker.

### Priority 1 -- Core Infrastructure (Must Do First)
- [x] Initialize Go module and install core dependencies
- [x] Create `cmd/server/main.go` entry point with Echo server
- [x] Create `internal/app/` -- App struct, dependency injection, route aggregation
- [x] Create `internal/config/` -- ENV loading with sensible defaults
- [x] Create `internal/database/` -- MariaDB connection pool + Redis client
- [x] Create `internal/middleware/` -- logging, recovery, helpers (IsHTMX, Render)
- [x] Create `internal/apperror/` -- domain error types
- [x] Create base Templ layouts (base, app, landing, error)
- [x] Create migration 000001_create_users
- [ ] Set up `air` hot reload for dev workflow
- [x] Vendor HTMX + Alpine.js (static/vendor/htmx.min.js, alpine.min.js)
- [ ] Verify `make docker-up` -> `make dev` works end-to-end

### Priority 2 -- Auth Plugin (Must Do)
- [x] User MariaDB table (migration 000001)
- [x] User model in `internal/plugins/auth/model.go`
- [x] Registration handler + service + repository
- [x] Login handler with argon2id password verification
- [x] Session tokens stored in Redis (random hex tokens)
- [x] Auth middleware (session validation, user context injection)
- [x] Login/Register Templ pages with HTMX support
- [x] Logout handler (destroy session)
- [x] Security middleware (CSP, proxy trust, CORS, CSRF)
- [x] RequireSiteAdmin middleware for admin routes
- [x] ListUsers, UpdateIsAdmin, CountUsers for admin panel

### Priority 3 -- Campaigns Plugin (Must Do)
- [x] Campaign model and MariaDB tables (migration 000002)
- [x] Campaign CRUD (create, list, show, edit, delete)
- [x] Campaign membership with roles (Owner, Scribe, Player)
- [x] RequireCampaignAccess + RequireRole middleware
- [x] Ownership transfer flow (72h token, optional email)
- [x] Campaign Templ pages (index, show, form, settings, members)
- [x] Campaign sidebar navigation in app layout (entity types, members, settings)
- [x] Campaign selector dropdown in topbar

### Priority 3.5 -- SMTP Plugin
- [x] SMTP settings singleton table (migration 000003)
- [x] AES-256-GCM password encryption
- [x] MailService interface (SendMail, IsConfigured)
- [x] SMTP settings admin page with test connection
- [x] SMTP password never returned to UI

### Priority 3.5 -- Admin Plugin
- [x] Admin dashboard with stats
- [x] User management (list, toggle admin)
- [x] Campaign management (list all, delete, join as role, leave)
- [x] SMTP settings integration
- [ ] Password reset flow (requires SMTP, wire into auth)

### Priority 4 -- Entities Plugin (Must Do)
- [x] Entity types table with configurable fields (migration 000004)
- [x] Default entity types seeded (Character, Location, Organization, Item, Note, Event)
- [x] Entity CRUD (create, list, show, edit, delete)
- [x] Entity list view (grid layout with type filter sidebar)
- [x] Entity profile page (sidebar with fields + main content area)
- [x] Entity search (MariaDB FULLTEXT + LIKE fallback)
- [x] Shortcut routes (/characters, /locations, /organizations, /items, /notes, /events)
- [x] Privacy filtering (is_private at SQL level, 404 for Players)
- [x] EntityTypeSeeder wired into campaigns (seeds on campaign create)

### Priority 5 -- Editor Widget (Must Do)
- [x] TipTap vendored JS bundle (static/vendor/tiptap-bundle.min.js)
- [x] editor.js widget with Chronicle.register() (static/js/widgets/editor.js)
- [x] boot.js widget auto-mounter (static/js/boot.js)
- [x] Save/load entity entry content via API (GET/PUT entry endpoints)
- [ ] @mention system (search entities, insert link)
- [ ] Entity mention parsing and rendering server-side

### Priority 6 -- UI & Layouts (Must Do)
- [x] Base Templ layout (HTML shell, head, scripts, styles)
- [x] App layout (authenticated -- sidebar + topbar + content area)
- [x] Sidebar navigation (campaign entities, dynamic via context injection)
- [x] Topbar (user avatar, admin link, campaign-scoped search, CSRF logout)
- [x] Tailwind CSS config (dark sidebar, light content -- Kanka-inspired)
- [x] Flash messages component (Alpine.js auto-dismiss, success/error)
- [x] Pagination component (shared, HTMX-aware, used by campaigns + entities)
- [x] LayoutInjector pattern (middleware.Render copies auth/campaign data to Go context)
- [x] Campaign selector dropdown in topbar (HTMX lazy-loaded, Alpine.js toggle)
- [x] Generate Tailwind CSS output (tailwindcss v3.4.17 standalone binary)

### Priority 7 -- Build & Deploy (Should Do)
- [x] Dockerfile builds successfully (multi-stage, Go 1.24, pinned Tailwind)
- [ ] docker-compose.yml works for full stack (app + MariaDB + Redis)
- [ ] GitHub Actions CI (build, lint, test)
- [x] Basic health check endpoint (`/healthz`)

### Priority 8 -- Nice to Have (Phase 1)
- [ ] Tags widget (tag CRUD, entity tagging)
- [ ] Entity nesting (parent/child relationships)
- [ ] Entity posts (additional sections on entity profile)
- [x] Image upload for entity headers
- [ ] Dark mode toggle

---

## Future Sprints (Not Yet Planned in Detail)

### Phase 2 -- Maps & Media
- [ ] Leaflet.js map viewer widget
- [ ] Map pin CRUD with entity linking
- [x] Image upload system with thumbnails (media plugin)
- [ ] Entity relations plugin (bi-directional)
- [ ] REST API plugin with PASETO token auth
- [x] Rate limiting middleware (auth + uploads)
- [x] IDOR protection on all entity endpoints
- [x] HSTS security header
- [x] Dynamic sidebar with entity types from DB
- [x] Entity image upload pipeline
- [x] UI upgrade with Font Awesome icons
- [ ] Sidebar customization (campaign-level config)
- [ ] Layout builder (entity type profile customization)

### Phase 3 -- Permissions & Advanced Multi-User
- [ ] Per-entity permissions (view/edit per role/user)
- [ ] Invite system (email invitations for campaigns)
- [ ] 2FA/TOTP support
- [ ] Audit log
- [ ] Entity type layout editor (drag-drop field customization)

### Phase 4 -- Game Systems & Advanced
- [ ] D&D 5e module (SRD reference data, tooltips, pages)
- [ ] Pathfinder module
- [ ] Draw Steel module
- [ ] Calendar plugin (custom months, days, moons)
- [ ] Timeline plugin (eras, events, zoomable)
- [ ] Relations graph visualization widget
- [ ] Foundry VTT sync module
- [ ] Campaign export/import
- [ ] AI integration endpoint

---

## Completed Sprints

### Phase 0: Project Scaffolding (2026-02-19)
- [x] Create AI documentation system (`.ai/` directory, 13 files)
- [x] Create `CLAUDE.md` root context file (three-tier architecture)
- [x] Create project directory skeleton (plugins, modules, widgets)
- [x] Create plugin `.ai.md` files (auth, campaigns, entities)
- [x] Create module `.ai.md` file (dnd5e)
- [x] Create widget `.ai.md` file (editor)
- [x] Create build configuration (Makefile, Dockerfile, docker-compose)
- [x] Create `.gitignore` and `.env.example`
- [x] Create `tailwind.config.js` and `static/css/input.css`
- [x] Establish coding conventions and 8 architecture decisions
