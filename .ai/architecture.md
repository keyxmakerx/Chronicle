# Chronicle Architecture

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Full system design document. Three-tier extension architecture,  -->
<!--          directory structure, request flow, dependency graph.             -->
<!-- Update: When major structural changes are made.                          -->
<!-- ====================================================================== -->

Chronicle is a monolithic Go app; core handles bootstrapping, config, DB
connections, middleware and route aggregation, everything else is a Plugin,
System or Widget (see root `CLAUDE.md` for what each tier is).

## Three-Tier Extension Architecture

```
CHRONICLE
  CORE (always present): app/ config/ database/ middleware/ apperror/

  PLUGINS -- Feature Applications (24): auth/ campaigns/ entities/
    calendar/ maps/ admin/ addons/ syncapi/ media/ audit/ settings/
    timeline/ sessions/ packages/ smtp/ armory/ bestiary/ designlab/
    npcs/ ai_workspace/ backup/ foundry_vtt/ restore/ widgetbindings/

  SYSTEMS -- External packages via package manager (generic loader +
    GenericTooltipRenderer)

  WIDGETS -- Reusable UI Building Blocks: editor/ title/ tags/
    attributes/ mentions/ notes/ relations/ posts/ entity_notes/

  TEMPLATES -- Shared Templ Layouts & Components: layouts/ components/
    pages/
```

### Tier Definitions

| Tier | What It Is | Has Backend? | Has Frontend? | Can Disable? |
|------|-----------|-------------|--------------|-------------|
| **Plugin** | Self-contained feature app: handler/service/repo/templates | Yes | Yes | Core: no. Optional: per-campaign |
| **System** | Game system content pack: reference data, tooltips, pages | Yes (data serving) | Yes (tooltips, pages) | Per-campaign |
| **Widget** | Reusable UI block: mounts to a DOM element, fetches own data | Minimal (API endpoints) | Primarily | Always available |

`calendar` is the one exception to the plugin shape above: its UI, routes and
handler were deleted for a ground-up rebuild (V5, #741); only its domain
layer and migrations remain. `syncapi`'s calendar routes answer `503
calendar_rebuilding` (`.ai/plugin-development.md`); re-wiring points
elsewhere are tagged `CALV5-PLACEHOLDER:`.

### How They Interact on a Page

```
Entity Profile Page Load:
  1. Plugin (entities) renders page skeleton via Templ
  2. Widgets (title, tags, editor, attributes) render their own fields
  3. System (installed via package manager) supplies tooltip data for
     @mentions that reference game content
```

Cross-tier communication rules: root `CLAUDE.md` → Code Conventions.

## Directory Structure

```
chronicle/
├── cmd/
│   └── server/
│       └── main.go                   # Entry point, wires everything
│
├── internal/
│   ├── app/                          # CORE: App struct, DI, route aggregation
│   │   ├── app.go
│   │   └── routes.go
│   │
│   ├── config/                       # CORE: Configuration loading (env vars)
│   │   └── config.go
│   │
│   ├── database/                     # CORE: connections, migrations, backup/recovery
│   │   ├── mariadb.go / redis.go     #   Connection pools
│   │   ├── migrate.go                #   Core migration runner
│   │   ├── plugin_schema.go          #   Plugin migration runner (reads embed.FS)
│   │   └── plugin_health.go          #   Plugin health registry
│   │
│   ├── middleware/                    # CORE: HTTP middleware (logging, recovery, CSRF, CORS,
│   │                                  #   rate limiting, IDOR, security headers, static caching;
│   │                                  #   session validation: internal/plugins/auth/middleware.go)
│   │
│   ├── apperror/                     # CORE: Domain error types
│   │   └── errors.go
│   │
│   ├── sanitize/                     # CORE: HTML sanitization (bluemonday)
│   │   └── sanitize.go              #   HTML(), StripSecretsHTML(), StripSecretsJSON()
│   │
│   ├── plugins/                      # PLUGINS: Feature applications
│   │   ├── auth/                     #   Authentication & user management
│   │   │                             #   (standard plugin shape, see below)
│   │   ├── campaigns/                #   Campaign/world management
│   │   ├── entities/                 #   Entity CRUD & configurable types
│   │   ├── calendar/                 #   Domain layer + migrations only (mid-rebuild, see below)
│   │   │   ├── model.go             #   Calendar, Month, Weekday, Moon, Season, Event
│   │   │   ├── calendar.go          #   Calendar math (dates, recurrence)
│   │   │   ├── import.go / export.go
│   │   │   ├── presets/             #   Built-in calendar presets (JSON)
│   │   │   └── migrations/
│   │   └── maps/                     #   Interactive Leaflet.js maps + markers
│   │       ├── .ai.md
│   │       ├── model.go             #   Map, Marker + DTOs
│   │       ├── repository.go
│   │       ├── service.go
│   │       ├── handler.go
│   │       ├── routes.go
│   │       └── maps.templ
│   │
│   ├── systems/                      # SYSTEMS: Generic system infrastructure
│   │   ├── registry.go               #   System registry + factory pattern
│   │   ├── loader.go                 #   Discover manifests from directories
│   │   ├── generic_system.go         #   Fallback for manifest-only systems
│   │   ├── generic_tooltip.go        #   Manifest-driven tooltip renderer
│   │   └── handler.go                #   System reference page handlers
│   │
│   ├── widgets/                      # WIDGETS: Reusable UI building blocks
│   │   ├── editor/                   #   TipTap rich text editor (no backend; loads/saves via the entity API)
│   │   │   ├── .ai.md
│   │   │   └── templates/
│   │   ├── notes/                    #   Floating notes panel (full backend)
│   │   │   ├── .ai.md
│   │   │   ├── model.go              #   Note, NoteVersion, Block structs
│   │   │   ├── repository.go         #   CRUD + locking + versions SQL
│   │   │   ├── service.go            #   Business logic + snapshots
│   │   │   ├── handler.go            #   HTTP endpoints
│   │   │   └── routes.go
│   │   ├── title/                    #   Page title component
│   │   ├── tags/                     #   Tag picker/display
│   │   ├── attributes/               #   Dynamic key-value field editor
│   │   └── mentions/                 #   @mention search & insert
│   │
│   └── templates/                    # SHARED: Templ layouts & components
│       ├── layouts/
│       │   ├── base.templ
│       │   └── app.templ
│       ├── components/
│       │   ├── breadcrumbs.templ
│       │   ├── pagination.templ
│       │   ├── plugin_unavailable.templ
│       │   └── rebuilding.templ
│       └── pages/
│           ├── landing.templ
│           └── error.templ
│
├── db/
│   ├── migrations/                   # Core schema baseline (fatal on failure)
│   └── queries/                      # Raw SQL query files (reference)
│
├── static/
│   ├── css/
│   │   └── input.css                 # Tailwind input
│   ├── js/                           # Global scripts (boot.js is the widget auto-mounter,
│   │   │                             #   plus keyboard_shortcuts.js and other UI helpers)
│   │   └── widgets/                  # One file per widget (editor.js, attributes.js, etc.)
│   ├── vendor/                       # Vendored CDN libs
│   ├── fonts/
│   └── img/
│
├── docs/                              # Operator + API docs (deployment, restore drills, api/openapi.yaml)
├── extensions/                        # Example/reference extensions (dice-roller, wasm, harptos-calendar)
├── scripts/                           # Shell scripts (backup.sh, restore.sh)
├── sdk/                               # Go SDK for extension authors
├── test/                              # JS test suites
├── testdata/                          # Fixtures (e.g. restore-drill)
├── tools/                             # CI guard scripts (see .ai/conventions.md)
├── .ai/                               # AI documentation
├── .claude/                           # Claude Code agent configs
├── CLAUDE.md
├── .gitignore
├── Makefile
├── Dockerfile
├── docker-compose.yml
└── tailwind.config.js
```

## Plugin Internal Structure

Every plugin has this common shape (test-file location and where `.templ`
lives vary per plugin; not every plugin embeds something, and the file that
does may hold migrations, static assets, or both — ADR-030):

```
internal/plugins/<name>/
  .ai.md              # Plugin-level AI documentation
  embed.go            # Optional: go:embed for migrations/static assets
  handler.go          # Echo handlers (conventions: root CLAUDE.md)
  service.go          # Business logic
  repository.go       # MariaDB queries (hand-written SQL)
  model.go            # Domain models, DTOs, request/response structs
  routes.go           # Route registration function
  migrations/         # Plugin schema migrations, if any (embedded in binary)
  templates/          # Templ components (some plugins keep top-level
                       #   .templ files instead of this subdirectory)
```

## System (Game System) Internal Structure

Systems are **external packages**, not in-repo directories: `internal/systems/` is a
flat Go package (loader, manifest parser, generic content-serving handler, registry),
with no `internal/systems/<name>/` subdirectories. A game system content pack (D&D 5e,
Draw Steel, Pathfinder 2e, etc.) is a manifest + data-files bundle installed per-campaign
via Admin > Packages; its manifest declares categories/fields/metadata, and its data
lives under the installed package's own on-disk version directory, outside the Go source
tree.

For custom tooltip formatting, a system package registers a Go file with `init()` calling
`systems.RegisterFactory()` (e.g. a stat-block formatter) — generic registration, so no
system gets a bespoke in-repo package.

## Widget Internal Structure

Widgets have minimal backend and primarily live in static/js/widgets/.

```
internal/widgets/<name>/
  .ai.md              # Widget-level AI documentation
  handler.go          # API endpoints (save/load/search) -- optional
  *.templ             # Optional: own markup, kept top-level (e.g. relations/graph.templ)

static/js/widgets/<name>.js   # The actual widget; mounts to a `data-widget` element
```

Request flow and cross-boundary rules: see root `CLAUDE.md`.

## Dependency Flow

Each layer has its own constructor (`NewUserRepository`, `NewAuthService`,
`NewHandler`, ...), wired bottom-up in `internal/app/routes.go`:

```
cmd/server/main.go
  -> internal/app/app.go          (creates DB pool, Redis, config)
    -> internal/app/routes.go     (constructs repo -> service -> handler per plugin,
                                    registers routes)
```

Handlers depend on service interfaces, services on repository interfaces —
never concrete types — so a plugin's internal types stay unimported outside it.
