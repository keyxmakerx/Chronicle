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
| Leaflet.js | 1.9.x | Interactive maps (vendored in `static/vendor/`) |
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
| `tailwindcss` | Generate CSS (standalone binary) |
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
