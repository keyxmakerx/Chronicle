# Technology Stack

<!-- ====================================================================== -->
<!-- Category: STATIC                                                         -->
<!-- Purpose: Quick reference for exact versions and why each tech was chosen. -->
<!-- Update: Only when a technology is added, removed, or upgraded.            -->
<!-- ====================================================================== -->

## Core

| Technology | Version | Role | Why |
|-----------|---------|------|-----|
| Go | 1.24+ | Backend language | Fast, single binary, strong typing |
| Echo | v4 | HTTP framework | Mature middleware, validation, Templ-friendly |
| Templ | latest | HTML templating | Type-safe, compiles to Go, component model |
| HTMX | 2.x | Frontend interactivity | Server-driven partials, no SPA, no Node |
| Alpine.js | 3.x | Client-side reactivity | Dropdowns, modals, toggles |
| MariaDB | 10.11+ | Primary database | User infrastructure requirement |
| Redis | 7.x | Sessions & cache | Session storage, rate limiting, caching |
| Tailwind CSS | 3.x (standalone CLI) | CSS framework | Utility-first, no Node needed |

## Frontend (Vendored, No Node.js)

| Library | Version | Role |
|---------|---------|------|
| TipTap | 2.x | Rich text editor widget |
| Leaflet.js | 1.9.x | Interactive maps (Phase 2) |
| Font Awesome | 6 Free | UI icons |
| RPG Awesome | latest | TTRPG-themed icons |
| Inter | latest | UI font (self-hosted) |

## Go Dependencies

| Package | Role |
|---------|------|
| `github.com/labstack/echo/v4` | HTTP framework |
| `github.com/a-h/templ` | Template engine |
| `github.com/go-sql-driver/mysql` | MariaDB driver |
| `github.com/redis/go-redis/v9` | Redis client |
| `github.com/google/uuid` | UUID generation |
| `github.com/golang-migrate/migrate/v4` | DB migrations |
| `github.com/alexedwards/argon2id` | Password hashing |
| `github.com/vk-rv/pvx` | PASETO v4 tokens |
| `github.com/go-playground/validator/v10` | Input validation |
| `github.com/microcosm-cc/bluemonday` | HTML sanitization |

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
| `chronicle-db` | `mariadb:10.11` | Database (persistent volume) |
| `chronicle-redis` | `redis:7-alpine` | Cache/sessions (128MB, allkeys-lru) |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | (required) | `user:pass@tcp(host:3306)/chronicle?parseTime=true` |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection |
| `SECRET_KEY` | (required) | PASETO signing key (32+ bytes, base64) |
| `BASE_URL` | `http://localhost:8080` | Public URL |
| `ENV` | `development` | `development` or `production` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `MAX_UPLOAD_SIZE` | `10MB` | Max file upload |
| `SESSION_TTL` | `720h` | Session duration (30 days) |
