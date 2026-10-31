# Chronicle Architecture

Chronicle is a monolithic Go app. Core handles bootstrapping, config, DB
connections, middleware and route aggregation; everything else is a Plugin,
System or Widget (tier definitions and cross-tier rules: root `CLAUDE.md`).
The plugin and widget lists are in `.ai/README.md`; each has its own `.ai.md`.

## Tiers

| Tier | Where | Backend | Frontend | Can disable |
|------|-------|---------|----------|-------------|
| **Plugin** | `internal/plugins/<name>/` | Yes | Yes | Core: no. Optional: per campaign |
| **System** | External package; loader in `internal/systems/` | Data serving | Tooltips, pages | Per campaign |
| **Widget** | `internal/widgets/<name>/` + `static/js/widgets/<name>.js` | Minimal (API endpoints) | Mainly | Always available |

Core lives under `internal/`: `app/` (App struct, DI, route aggregation),
`config/`, `database/` (pools, migration runners, plugin health),
`middleware/`, `apperror/`, `sanitize/`. Shared Templ is in
`internal/templates/` (`layouts/`, `components/`, `pages/`). `cmd/server/main.go`
is the entry point; `db/migrations/` is the core schema.

Entity page load: the `entities` plugin renders the skeleton via Templ; widgets
(title, tags, editor, attributes) render their own fields; an installed system
supplies tooltip data for `@mentions` of game content.

**Plugins**, feature applications in `internal/plugins/`:
- `addons/`
- `admin/`
- `ai_workspace/`
- `armory/`
- `audit/`
- `auth/`
- `backup/`
- `bestiary/`
- `calendar/`
- `campaigns/`
- `crafting/`
- `designlab/`
- `dmscreen/`
- `encounters/`
- `entities/`
- `fieldrules/`
- `foundry_vtt/`
- `homebrew/`
- `maps/`
- `media/`
- `npcs/`
- `packages/`
- `pageblocks/`
- `pagetests/`
- `quests/`
- `restore/`
- `rolls/`
- `rolltables/`
- `rumours/`
- `sessions/`
- `settings/`
- `smtp/`
- `syncapi/`
- `systemstate/`
- `timeline/`
- `trackers/`
- `vault_import/`
- `widgetbindings/`

## Plugin structure

```
internal/plugins/<name>/
  .ai.md         # purpose, files, routes, rules, footguns
  embed.go       # optional: go:embed for migrations and/or static assets (ADR-030)
  handler.go     # Echo handlers: bind, call service, render
  service.go     # business logic; never imports Echo types
  repository.go  # hand-written SQL
  model.go       # domain models, DTOs, request/response structs
  routes.go      # route registration
  migrations/    # plugin schema, embedded in the binary
  templates/     # Templ components (some plugins keep .templ files top-level)
```

Test files and `.templ` placement vary per plugin.

## Systems

Systems are external packages, not in-repo directories. `internal/systems/` is
one flat package (loader, manifest parser, registry, generic content handler);
there are no per-system subdirectories. A package (manifest plus data files)
is installed per campaign via Admin > Packages; its manifest declares
categories, fields and metadata, and its data lives in the package's own
on-disk version directory. Custom tooltip formatting registers through
`systems.RegisterFactory()` from an `init()`.

## Widgets

```
internal/widgets/<name>/
  .ai.md       # widget docs
  handler.go   # optional API endpoints (save/load/search)
  *.templ      # optional markup, top-level

static/js/widgets/<name>.js   # the widget; mounts to a `data-widget` element
```

`static/js/boot.js` auto-mounts widgets. Other global scripts sit in
`static/js/`; a plugin's own scripts sit in `internal/plugins/<slug>/static/js/`.

## Dependency flow

Each layer has its own constructor (`NewUserRepository`, `NewAuthService`,
`NewHandler`, ...), wired bottom-up in `internal/app/routes.go`:

```
cmd/server/main.go
  -> internal/app/app.go      (DB pool, Redis, config)
    -> internal/app/routes.go (repo -> service -> handler per plugin, routes)
```

Handlers depend on service interfaces and services on repository interfaces,
never concrete types, so a plugin's internal types stay unimported outside it.

## Other top-level directories

`docs/` (operator and API docs), `extensions/` (example extensions), `sdk/`
(Go SDK for extension authors), `scripts/` (`backup.sh`, `restore.sh`),
`test/` (JS and e2e suites), `testdata/` (fixtures), `tools/` (CI guard
scripts, see `.ai/conventions.md`), `.claude/` (agent configs).
