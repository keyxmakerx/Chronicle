# Chronicle

Chronicle is a self-hosted TTRPG worldbuilding platform. Go backend with Echo v4 framework, Templ templates, HTMX for interactivity, MariaDB for persistence, Redis for caching/sessions. Frontend uses a **three-tier extension architecture** (Plugins, Systems, Widgets) — see "Architecture at a Glance" below.

## Quick Commands

```bash
make dev            # Dev server, hot reload (air)
make build          # Production binary
make templ          # Regen Templ .go from .templ
make tailwind       # Regen Tailwind CSS
make tailwind-watch # Watch mode
make test           # All tests
make test-unit      # Unit tests only
make test-int       # Integration tests (needs running DB)
make test-db-up     # Local MariaDB for tests, no Docker (port 13306)
make test-db-down   # Stop that DB
make test-int-local # Integration tests against it (starts it if needed)
make lint           # golangci-lint
make migrate-up     # Apply pending migrations
make migrate-down   # Rollback last migration
make migrate-create # New migration (NAME=description)
make docker-up      # MariaDB + Redis containers (needs Docker daemon)
make docker-down    # Stop containers
make clean          # Remove built artifacts
```

## Architecture at a Glance

Everything beyond core infrastructure is a Plugin, System, or Widget:

| Tier | Location | What It Is | Examples |
|------|----------|-----------|---------|
| **Plugin** | `internal/plugins/<name>/` | Feature app with handler/service/repo/templates | auth, campaigns, entities, maps, sessions, calendar (rebuilt in parts, #741) |
| **System** | External repos via package manager | Game system content pack (reference data, tooltips) | Installed via Admin > Packages |
| **Widget** | `internal/widgets/<name>/` | Reusable UI building block (mounts to DOM) | editor, title, tags, attributes, mentions |

**Request flow:** Router -> Middleware -> Handler -> Service -> Repository -> MariaDB. Handler calls a Templ component, returning a full page or an HTMX fragment (`middleware.IsHTMX(c)`, also checks `HX-Boosted`). Widgets are self-contained JS modules that mount via `data-widget` attributes, fetch their own data from the API, and render themselves; `boot.js` auto-mounts them.

See `.ai/architecture.md` for the full document.

## Code Conventions (Critical -- Read These)

- **Handlers:** bind, call service, render — no business logic.
- **Services:** own business logic; never import Echo types (those stay in handlers, routes and middleware).
- **Repositories:** own SQL, one per aggregate root, hand-written.
- **Cross-boundary:** plugins reach each other only via service interfaces, never direct repo access; systems are read-only; widgets talk via DOM events + API endpoints.
- **Templ:** one file per component; layouts in `internal/templates/layouts/`.
- **Errors:** domain types from `internal/apperror/`, never raw DB errors.
- **Tests:** table-driven; interfaces at every service/repo boundary.
- **Naming:** `snake_case.go` files, `PascalCase` exported Go types, `camelCase` JSON.
- **New plugin:** copy an existing plugin's structure and add its `.ai.md`.
- **Migrations:** append-only, schema-only, numbered SQL in `db/migrations/` (plugin tables in `internal/plugins/<slug>/migrations/`, run after core). Never edit/delete/renumber an applied migration (`tools/check-migration-immutability.sh`, CI-enforced); new DDL is idempotent; one-time data fixes are an idempotent reconciler, not a migration; core migrations reference only core tables. Rules: `.ai/conventions.md` §"Migration Safety Rules".
- **Comments:** every package, exported type, non-obvious block — why not what, briefly. No history, task IDs, dates or `file:line`; those go in the PR. Deferred work is `TODO(#issue)`. See `.ai/conventions.md` §Comment Conventions.
- **Database:** MariaDB via `database/sql` + `go-sql-driver/mysql`, no ORM.

## Where things live

Work is tracked in **GitHub Issues**, not markdown files. Every session starts from the issue for the task; open an issue for anything else found. Repo docs describe the system *now*, present tense — edit only when behavior changes, never append dated entries or "recent work". Pick a home before writing:

| What you have | Where it goes | Never |
|---|---|---|
| A bug, feature, follow-up or tech debt | An issue in the repo whose code changes | a backlog file, a new `.md`, a TODO comment |
| An operator-only action or decision | A `needs-operator` issue titled `Decide:`/`Check:`/`Do: …`, with your recommendation | chat only, a handoff doc |
| A multi-step effort | A parent issue with sub-issues | a "master plan" doc |
| How the system works | `.ai/architecture.md`, `conventions.md`, `data-model.md`, each plugin's `.ai.md`, `docs/` | dates, "recent work" sections |
| Why it is built this way | An ADR in `.ai/decisions.md` (append-only; code cites `ADR-NNN`) | an essay in a code comment |
| What happened | The commit message and PR description, with `Fixes #N` | status logs, dated docs, reports |
| Unfinished work at session end | An issue/PR comment: done, next, gotchas | a handoff file |

Labels: `needs-operator`, `security`, `priority: high`, `blocked`, `calendar-v5`, `documentation`, `good first issue`. Issue types (Bug/Feature/Task) say what kind of thing it is. A pre-September-2026 tracking ID (`C-…`, `FM-…`) is listed in the issue that replaced it.

**Unfixed security weaknesses never go in a public issue** — they're tracked in the private Cordinator repo until fixed; the fixing PR can then say "security fix". Cordinator is otherwise a frozen archive, still cited by code comments.

Full `.ai/` index with per-file "when to read": `.ai/README.md`. Each plugin and widget has its own `.ai.md`. `.ai/status.md` and `.ai/todo.md` are pointers to the issues; do not add to them.

## Working with this project

These rules come from the old coordination repo (Cordinator), which is now a
frozen archive. The same block is in the CLAUDE.md of Chronicle, the Foundry
module and the Draw Steel package; change all three together. The binding
tenets the PR templates name (T-B1 security first, T-B2 plugin isolation, T-B3
production-grade UI, T-B4 docs for humans and AI alike) are defined in
Cordinator's `decisions/2026-05-21-core-tenets.md`.

**With the operator** (the maintainer, who reviews and deploys):
- Explain things in plain language, without code. Give each trade-off in one sentence.
- Give live checks as click-paths: the exact URL, what to click, and what working
  and broken look like. Docker, OS and network commands are fine; never ask the
  operator to read code or run a test suite.
- The operator checks things later, not while you wait. Put checks in an issue
  labelled `needs-operator`, and when work is blocked on them, name the exact action.
- Decide and recommend. Don't offer a menu of options for things you can judge;
  ask only about real product, visual or scheduling choices.
- Stop at natural stopping points rather than interrupting with status questions.
- UI work gets a mockup first, and a mockup the operator signed stays the contract
  until they sign a new one. A decision about motion is shown as playable clips,
  never stills.

**Safety**
- Chronicle runs in production. Verify, then fix; back up before deploys; put
  anything risky behind an operator step. Security wins every tie.
- A merged PR is not a deployed fix. Deploy settings and gates are separate steps
  with their own checks.

**Verify before you claim**
- Read the source in the same turn before naming files, lines, identifiers or wire
  values. Verify a wire contract from the code that consumes it.
- Check any claim about state (open, merged, shipped, deployed) against git or
  GitHub first. A claim measured against another repo is true only on the day it
  was measured.
- A root cause is a guess until the code confirms it; a bug-fix PR says why the bug
  existed. When the scope is unclear, start by reading, not changing.
- CI red with local green on the same commit means an environment difference until
  proven otherwise.
- If a rule can't be followed or the task is wrong, stop and say so instead of
  pressing on.

**Scope and reporting**
- The PR description is what gets reviewed: what and why, the load-bearing lines,
  honest deviations, the exact test commands and their pass counts.
- Stay inside the task. Open an issue for anything else; ship the smallest useful
  change and split the follow-ups.

**Sessions**
- Big agent fleets are welcome for work that splits cleanly, but run them on a
  lighter model. Never fan a large fleet out on the most expensive model; keep
  that for the few agents that need it. Usage is a real limit.
- One session per piece of work, ended when it ships. Don't sit in a loop polling
  for CI or PR events.
- Work only on the branch you were given. Never push to another branch without
  explicit permission.
- File the issue before handing work on, and never point anyone at something that
  hasn't landed.
