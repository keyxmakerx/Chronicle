# Chronicle

Chronicle is a self-hosted TTRPG worldbuilding platform. Go backend with Echo v4
framework, Templ templates, HTMX for interactivity, MariaDB for persistence, Redis
for caching/sessions. Frontend uses a **three-tier extension architecture**:
Plugins (feature apps), Modules (game system content), and Widgets (reusable UI blocks).

## Quick Commands

```bash
make dev            # Start dev server with hot reload (air)
make build          # Production binary build
make templ          # Regenerate Templ .go files from .templ sources
make tailwind       # Regenerate Tailwind CSS
make tailwind-watch # Watch mode for Tailwind CSS
make test           # Run all tests
make test-unit      # Unit tests only
make test-int       # Integration tests (requires running DB)
make lint           # Run golangci-lint
make migrate-up     # Apply all pending migrations
make migrate-down   # Rollback last migration
make migrate-create # Create new migration (NAME=description)
make seed           # Seed dev database with sample data
make docker-up      # Start MariaDB + Redis containers
make docker-down    # Stop containers
make clean          # Remove built artifacts
```

## Architecture at a Glance

**Three-tier extension architecture.** Everything beyond core infrastructure is
a Plugin, Module, or Widget:

| Tier | Location | What It Is | Examples |
|------|----------|-----------|---------|
| **Plugin** | `internal/plugins/<name>/` | Feature app with handler/service/repo/templates | auth, campaigns, entities, calendar, maps |
| **Module** | `internal/modules/<name>/` | Game system content pack (reference data, tooltips) | dnd5e, pathfinder, drawsteel |
| **Widget** | `internal/widgets/<name>/` | Reusable UI building block (mounts to DOM) | editor, title, tags, attributes, mentions |

**Request flow:**
Router -> Middleware -> Handler -> Service -> Repository -> MariaDB

**Templates:**
Handler calls Templ component -> returns full page OR HTMX fragment (based on
`HX-Request` header).

**Widgets:**
Self-contained JS modules that mount to a DOM element via `data-widget` attributes,
fetch their own data from the API, and render themselves. Auto-mounted by `boot.js`.

See `.ai/architecture.md` for the full architecture document.

## Code Conventions (Critical -- Read These)

- **Handlers are thin:** bind request, call service, render response. NO business logic.
- **Services own business logic.** Services NEVER import Echo types.
- **Repositories own SQL.** One repository per aggregate root. Hand-written SQL.
- **Templ components:** one file per visual component. Layouts in `internal/templates/layouts/`.
- **HTMX detection:** check `c.Request().Header.Get("HX-Request")` to return fragment vs full page.
- **Errors:** use domain error types from `internal/apperror/`. Never return raw DB errors.
- **Tests:** table-driven tests. Interfaces for all service/repo boundaries.
- **Naming:** `snake_case.go` for files, `PascalCase` for exported Go types, `camelCase` for JSON.
- **Migrations:** sequential numbered SQL files in `db/migrations/`. Never edit an applied migration.
- **Comments:** every package, every exported type, every non-obvious block. WHY not WHAT.
- **Database:** MariaDB. Use `database/sql` + `go-sql-driver/mysql`. No ORM.

## AI Documentation System

All AI context files live in `.ai/` at the project root. Read `.ai/README.md` for
the full index. Key files:

| File | When to Read |
|------|-------------|
| `.ai/status.md` | **Every session start** -- current state and next priorities |
| `.ai/todo.md` | When planning work -- prioritized backlog |
| `.ai/architecture.md` | When designing new features or modules |
| `.ai/conventions.md` | When writing any code -- patterns with examples |
| `.ai/decisions.md` | When questioning a design choice -- ADRs with rationale |
| `.ai/data-model.md` | When writing queries or migrations |

Each plugin/module/widget has its own `.ai.md` in its directory.

## IMPORTANT RULES

1. **ALWAYS** read `.ai/status.md` before starting work.
2. **ALWAYS** update `.ai/status.md` and `.ai/todo.md` after completing work.
3. **NEVER** add business logic to handlers.
4. **NEVER** import Echo types outside of handler files.
5. When creating a new plugin, copy structure from an existing one and create its `.ai.md`.
6. When making an architecture decision, record it in `.ai/decisions.md`.
7. Add comments to every package, exported type, and non-obvious code block.
8. Plugins talk to each other via **service interfaces**, never direct repo access.
9. Modules are **read-only** -- they serve reference content but never modify campaign data.
10. Widgets communicate via **DOM events** and **API endpoints**.
