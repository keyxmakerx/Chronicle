# Chronicle Architecture

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Full system design document. Three-tier extension architecture,  -->
<!--          directory structure, request flow, dependency graph.             -->
<!-- Update: When major structural changes are made.                          -->
<!-- ====================================================================== -->

## System Overview

Chronicle is a monolithic Go application with a modular internal structure
organized into three extension tiers: **Plugins**, **Modules**, and **Widgets**.
The core handles bootstrapping, configuration, database connections, middleware,
and route aggregation. Everything else is a self-contained unit in one of the
three tiers.

## Three-Tier Extension Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                         CHRONICLE                             │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  CORE (always present)                                │    │
│  │  app/  config/  database/  middleware/  apperror/      │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  PLUGINS -- Feature Applications                      │    │
│  │  auth/  campaigns/  entities/  calendar/  maps/        │    │
│  │  timeline/  relations/  api/  tags/  posts/            │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  MODULES -- Game System Content Packs                  │    │
│  │  dnd5e/  pathfinder/  drawsteel/                       │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  WIDGETS -- Reusable UI Building Blocks                │    │
│  │  editor/  title/  tags/  attributes/  mentions/        │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  TEMPLATES -- Shared Templ Layouts & Components        │    │
│  │  layouts/  components/  pages/                         │    │
│  └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
```

### Tier Definitions

| Tier | What It Is | Has Backend? | Has Frontend? | Can Disable? |
|------|-----------|-------------|--------------|-------------|
| **Plugin** | Self-contained feature app with handler/service/repo/templates | Yes | Yes | Core: no. Optional: per-campaign |
| **Module** | Game system content pack. Reference data, tooltips, dedicated pages | Yes (data serving) | Yes (tooltips, pages) | Per-campaign |
| **Widget** | Reusable UI block. Mounts to DOM element, fetches own data | Minimal (API endpoints) | Primarily | Always available |

### How They Interact on a Page

```
Entity Profile Page Load:
  1. Plugin (entities) renders page skeleton via Templ
  2. Widget (title) renders the entity name field
  3. Widget (tags) renders the tag picker
  4. Widget (editor) mounts TipTap for entry content
  5. Widget (attributes) renders configurable entity fields
  6. Module (dnd5e) provides tooltip data when hovering @mentions
     that reference game content (spells, monsters, items, etc.)
```

**Communication rules:**
- Plugins talk to each other through **service interfaces** (never direct repo access)
- Widgets communicate via **DOM events** and **API endpoints**
- Modules are **read-only content providers** -- they never modify campaign state

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
│   ├── database/                     # CORE: Database connections
│   │   ├── mariadb.go                #   MariaDB connection pool
│   │   └── redis.go                  #   Redis client
│   │
│   ├── middleware/                    # CORE: HTTP middleware
│   │   ├── auth.go                   #   Session validation
│   │   ├── logging.go                #   Request logging
│   │   ├── recovery.go               #   Panic recovery
│   │   └── csrf.go                   #   CSRF protection
│   │
│   ├── apperror/                     # CORE: Domain error types
│   │   └── errors.go
│   │
│   ├── plugins/                      # PLUGINS: Feature applications
│   │   ├── auth/                     #   Authentication & user management
│   │   │   ├── .ai.md
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   ├── model.go
│   │   │   ├── routes.go
│   │   │   └── templates/
│   │   ├── campaigns/                #   Campaign/world management
│   │   └── entities/                 #   Entity CRUD & configurable types
│   │
│   ├── modules/                      # MODULES: Game system content packs
│   │   └── dnd5e/                    #   D&D 5th Edition (first module)
│   │       ├── .ai.md
│   │       ├── handler.go            #   Serves reference pages & tooltip API
│   │       ├── data/                 #   Static reference data (JSON/YAML)
│   │       ├── routes.go
│   │       └── templates/
│   │
│   ├── widgets/                      # WIDGETS: Reusable UI building blocks
│   │   ├── editor/                   #   TipTap rich text editor
│   │   │   ├── .ai.md
│   │   │   ├── handler.go            #   API: save/load content
│   │   │   └── templates/
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
│       │   ├── navbar.templ
│       │   ├── sidebar.templ
│       │   ├── flash.templ
│       │   └── pagination.templ
│       └── pages/
│           ├── landing.templ
│           └── error.templ
│
├── db/
│   ├── migrations/                   # Sequential SQL migration files
│   └── queries/                      # Raw SQL query files (reference)
│
├── static/
│   ├── css/
│   │   └── input.css                 # Tailwind input
│   ├── js/
│   │   ├── boot.js                   # Widget auto-mounter
│   │   └── widgets/
│   │       ├── editor.js             # TipTap wrapper
│   │       ├── attributes.js         # Dynamic field editor
│   │       ├── tags.js               # Tag picker
│   │       └── mentions.js           # @mention search
│   ├── vendor/                       # Vendored CDN libs
│   ├── fonts/
│   └── img/
│
├── .ai/                              # AI documentation
├── CLAUDE.md
├── .gitignore
├── Makefile
├── Dockerfile
├── docker-compose.yml
└── tailwind.config.js
```

## Plugin Internal Structure

Every plugin follows this exact structure. No exceptions.

```
internal/plugins/<name>/
  .ai.md              # Plugin-level AI documentation
  handler.go          # Echo handlers (thin: bind, call service, render)
  handler_test.go     # Handler tests (HTTP-level, mock service)
  service.go          # Business logic (never imports Echo types)
  service_test.go     # Service tests (unit, mocked repo)
  repository.go       # MariaDB queries (hand-written SQL)
  repository_test.go  # Repository tests (integration, real DB)
  model.go            # Domain models, DTOs, request/response structs
  routes.go           # Route registration function
  templates/          # Templ components for this plugin
    index.templ       #   List view
    show.templ        #   Detail view
    form.templ        #   Create/edit form
    partials/         #   HTMX fragments
      list_item.templ
      detail_panel.templ
```

## Module (Game System) Internal Structure

Modules are simpler than plugins -- primarily data serving.

```
internal/modules/<name>/
  .ai.md              # Module-level AI documentation
  handler.go          # Serves reference pages & tooltip API
  data/               # Static reference data (JSON/YAML files)
    spells.json
    monsters.json
    items.json
  routes.go           # Route registration
  templates/          # Templ components for reference pages
    index.templ       #   Category listing
    show.templ        #   Individual reference entry
    tooltip.templ     #   Hover tooltip fragment
```

## Widget Internal Structure

Widgets have minimal backend and primarily live in static/js/widgets/.

```
internal/widgets/<name>/
  .ai.md              # Widget-level AI documentation
  handler.go          # API endpoints (save/load/search) -- optional
  templates/          # Templ mount-point components
    mount.templ       #   Renders the data-widget div for auto-mounting

static/js/widgets/<name>.js   # Client-side JavaScript (the actual widget)
```

## Request Flow

1. HTTP request arrives at Echo router
2. Global middleware: logging -> recovery -> CSRF
3. Route middleware: auth session -> permissions
4. **Handler** binds request, validates, calls **Service**
5. **Service** applies business logic, calls **Repository**
6. **Repository** runs hand-written SQL against MariaDB
7. Handler checks `HX-Request` header:
   - HTMX: render Templ fragment
   - Full page: render Templ page in layout
8. Response sent to client

## Dependency Flow

```
cmd/server/main.go
  -> internal/app/app.go          (creates DB pool, Redis, config)
    -> each plugin's New()        (receives dependencies)
      -> handler                  (receives service interface)
        -> service                (receives repository interface)
          -> repository           (receives *sql.DB)
```

**Rules:**
- Handlers depend on service **interfaces** (not concrete types)
- Services depend on repository **interfaces** (not concrete types)
- Cross-plugin communication goes through service interfaces
- A plugin NEVER imports another plugin's internal types
- Modules NEVER write to the database
- Widgets are self-contained; backend is optional
