# Contributing to Chronicle

Thanks for helping. Chronicle is a self-hosted worldbuilding app for tabletop
RPGs: Go, Echo v4, Templ, HTMX, MariaDB and Redis. It runs in production for
real people, so changes are small, verified and reviewed.

## Reporting a security problem

Do not post details in a public issue. Use the **Report a vulnerability**
button on the repository's Security tab if it is there; otherwise open an issue
that only says you have a security report, and the maintainer will arrange a
private way to share it. There is no `SECURITY.md` and no published contact
address.

## Prerequisites

- Go 1.27 or newer (see `go.mod`)
- MariaDB 10.11+ and Redis 7+, from Docker (`make docker-up`) or installed locally
- The `templ` CLI (`go install github.com/a-h/templ/cmd/templ@v0.3.1070`, the
  version CI uses) and `air` for hot reload
- The Tailwind standalone CLI for `make tailwind`
- Node.js 20 for the JS tests (`make test-js`)
- `golangci-lint` for `make lint`

## Local setup

```bash
git clone https://github.com/keyxmakerx/chronicle.git && cd chronicle
cp .env.example .env     # edit database credentials and SECRET_KEY
make docker-up           # MariaDB + Redis in containers
make generate            # templ + tailwind; generated files are not committed
make dev                 # hot reload; migrations run on startup
```

No Docker daemon: `make test-db-up` starts a local MariaDB on port 13306 with
no containers (`make test-db-down` stops it). The first user to register
becomes the site admin.

To run the whole stack from your working tree, use `make docker-all-local`.
`make full-stack-check` builds it, waits for `/healthz`, and tears it down
again; CI runs the same command.

## Running tests

- `make verify` runs the same sequence as the CI "Build & Test" job: templ,
  build, vet, the repository guards, `go test ./... -short`, and the JS tests.
  Run it before you push.
- `make test-int-local` runs the database-backed tests against the local test
  MariaDB. Those tests skip when no database answers, so a plain `go test`
  can pass without running them.
- `make lint` runs golangci-lint; it is its own CI job and not part of `verify`.
- `make test-freshdb` replays every migration against an empty schema.
- Tests are table-driven, with interfaces at every service and repository
  boundary.

## Code layout

| Tier | Location | What it is |
|---|---|---|
| Plugin | `internal/plugins/<name>/` | A feature (auth, campaigns, entities, maps, calendar...) with handler, service, repository and templates |
| System | external repos | A game-system content pack, installed from Admin > Packages |
| Widget | `internal/widgets/<name>/` | A reusable UI block that mounts through `data-widget` and fetches its own data |

Shared code lives in `internal/middleware`, `internal/apperror`,
`internal/config`, `internal/database` and `internal/templates`. Core
migrations are in `db/migrations/`; each plugin's are in its own `migrations/`
directory. Read a plugin's or widget's `.ai.md` before changing it, and see
`.ai/README.md` for the index and `.ai/architecture.md` for the design.

## Conventions

The full list is in `CLAUDE.md` and `.ai/conventions.md`. The ones reviewers
check most:

- **Layering.** Handlers bind, call a service and render. Services own the
  business rules and never import Echo. Repositories own hand-written SQL.
  Plugins reach each other through service interfaces, never another plugin's
  repository. Errors are the domain types in `internal/apperror/`.
- **Migrations are append-only.** Never edit, delete or renumber a migration
  that has landed. New DDL is idempotent (`IF NOT EXISTS` / `IF EXISTS`). A
  migration changes schema only; a one-time data fix is an idempotent
  reconciler in code. Core migrations reference only core tables.
  `make verify` runs the immutability and numbering guards.
- **Comments say why, briefly.** Every package and exported type gets a doc
  comment. No history, task IDs, dates or `file:line` pointers in comments;
  put those in the PR description. Deferred work is `TODO(#issue)`.
- **UI follows `.ai/ui-standard.md`.** Any visible change starts from it.
- **Naming.** `snake_case.go` and `snake_case.templ` files, `PascalCase`
  exported types, `camelCase` JSON.
- **Do not run `gofmt -w` over globs.** The tree is not plain-gofmt clean, so
  that rewrites unrelated files. Edit in place.

## How work is tracked

Work lives in GitHub Issues. Start from an existing issue, or open one for
what you plan to change, before you write code. Bugs, features and follow-ups
you find along the way each get their own issue. Do not add backlog, status or
handoff markdown files.

## Pull requests

- Keep the change small and inside the issue's scope.
- Fill in `.github/pull_request_template.md`: a `Fixes #N` line, a security
  line, what changed and why, and a test plan.
- Say which commands you ran and their results. For changes that touch
  migrations, repositories or startup wiring, boot the app against a real
  database and say so (see "When a PR needs boot verification" in
  `.ai/conventions.md`).
- Update the docs that describe behavior you changed (the plugin's `.ai.md`,
  `.ai/`, or `docs/`) in the same PR.
- CI must be green.

Chronicle is licensed under the [GNU AGPL v3](LICENSE).
