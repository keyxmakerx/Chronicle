# Technology Stack

<!-- ====================================================================== -->
<!-- Category: STATIC                                                         -->
<!-- Purpose: Quick reference for exact versions and why each tech was chosen. -->
<!-- Update: Only when a technology is added, removed, or upgraded.            -->
<!-- ====================================================================== -->

## Core

| Technology | Version | Role | Why |
|-----------|---------|------|-----|
| Go | 1.27+ | Backend language | Fast, single binary, strong typing |
| Echo | v4 | HTTP framework | Mature middleware, validation, Templ-friendly |
| Templ | version in `go.mod` (CI installs the same) | HTML templating | Type-safe, compiles to Go, component model |
| HTMX | 2.x | Frontend interactivity | Server-driven partials, no SPA, no Node |
| Alpine.js | 3.x | Client-side reactivity | Dropdowns, modals, toggles |
| MariaDB | latest (Docker image tag) | Primary database | User infrastructure requirement |
| Redis | latest/alpine (Docker image tag) | Sessions & cache | Session storage, rate limiting, caching |
| Tailwind CSS | 3.x (standalone CLI) | CSS framework | Utility-first, no Node needed |

## Frontend (Mostly Vendored; Node Only for the Editor Bundle)

| Library | Version | Role |
|---------|---------|------|
| TipTap | 3.x | Rich text editor widget (bundled via esbuild, see `static/vendor/tiptap-bundle.src.js`) |
| Mermaid | 11.17.2 | Diagrams in pages (vendored single-file build in `static/vendor/mermaid.min.js`, loaded only when a diagram is drawn; `make mermaid-vendor`) |
| three.js | 0.160.0 | Draws the 3D dice (`static/js/dice.js`, `static/js/dice/`); three.js and the post-processing add-ons the dice use, bundled as one ES module `static/vendor/three-dice.min.js` from `static/vendor/three-dice.src.js`, loaded on the first throw; `make dice-vendor` |
| cannon-es | 0.20.0 | Physics for the dice throw; vendored ES module `static/vendor/cannon-es.js`, loaded with three.js; `make dice-vendor` |
| Leaflet.js | 1.9.x | Interactive maps (vendored in `static/vendor/`) |
| D3 | 7.9 | Timeline, relation graph and database explorer (vendored `static/vendor/d3.min.js`) |
| Font Awesome | 6 Free | UI icons (loaded from cdnjs in `base.templ`) |
| Inter | latest | UI font (Google Fonts, loaded in `base.templ`) |

## Go Dependencies

| Package | Role |
|---------|------|
| `github.com/labstack/echo/v4` | HTTP framework |
| `github.com/a-h/templ` | Template engine |
| `github.com/go-sql-driver/mysql` | MariaDB driver |
| `github.com/redis/go-redis/v9` | Redis client |
| `github.com/google/uuid` | UUID generation |
| `github.com/golang-migrate/migrate/v4` | DB migrations |
| `golang.org/x/crypto/argon2` | Password hashing (argon2id) |
| `github.com/microcosm-cc/bluemonday` | HTML sanitization |
| `github.com/extism/go-sdk` + `github.com/tetratelabs/wazero` | WASM plugin runtime (Extism host SDK on the wazero engine) |

## Dev Tools

| Tool | Purpose |
|------|---------|
| `air` | Hot reload for Go dev server |
| `templ` | Generate Go from .templ files |
| `tailwindcss` | Generate CSS (standalone binary pinned in `tools/tailwind.sh`, fetched into `bin/` by `make tailwind`) |
| `golangci-lint` | Linting |
| `gosec` | Security static analysis |
| `migrate` | CLI for running migrations |

## Docker Services

| Service | Image | Role |
|---------|-------|------|
| `chronicle` | Custom multi-stage | Go binary serves HTTP directly |
| `chronicle-db` | `mariadb:latest` | Database (persistent volume) |
| `chronicle-redis` | `redis:alpine` | Cache/sessions (128MB, allkeys-lru) |

## Environment Variables

See `internal/config/config.go` (`Load()`) for the authoritative list and `docs/deployment.md` for the variables an operator sets, with defaults.
