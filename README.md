# Chronicle

**A self-hosted worldbuilding platform for tabletop RPG campaigns.**

Chronicle gives game masters and players a shared space to build worlds, track lore, manage calendars, visualize timelines, and run campaigns — all on your own server, with no paywall, no forced public content, and full control over your data.

<!-- TODO: Add hero screenshot -->

---

## Why Chronicle?

Existing tools either lock features behind paid tiers (World Anvil), rely on outdated editors (Kanka's Summernote), require cobbling together plugins for basic TTRPG features (Obsidian), or are closed-source (LegendKeeper). Chronicle is purpose-built for tabletop RPGs, open source, and designed to be self-hosted from day one.

**Inspired by** [World Anvil](https://www.worldanvil.com/), [Kanka](https://kanka.io/), [LegendKeeper](https://www.legendkeeper.com/), and [Obsidian](https://obsidian.md/) — taking the best ideas from each while avoiding their limitations.

---

## Features

### Worldbuilding
- **Pages & Categories** — Create any content type (characters, locations, factions, items, etc.) with custom categories and dynamic field schemas
- **Rich Text Editor** — TipTap-powered WYSIWYG with @mentions, backlinks, GM secrets, and an insert menu
- **Relations** — Bi-directional entity relationships with typed connections ("is spouse of", "leads", "enemy of")
- **Entity Hierarchy** — Parent/child nesting with tree views and breadcrumb navigation
- **Tags** — Color-coded tags with DM-only visibility controls
- **Custom Attributes** — Per-category field templates (text, number, select, checkbox, URL) with per-entity overrides
- **Drag-and-Drop Page Layouts** — Visual layout editor for entity profile pages — no other tool has this

### Calendar & Time
- **Custom Calendars** — Define months, weekdays, moons, seasons, eras, and leap year rules for any fantasy calendar system
- **Real-Life Sync** — Optional Gregorian calendar mode synced to real-world dates
- **Events** — Single-day, multi-day, and recurring events with visibility controls, entity linking, and category icons
- **Import/Export** — Import from Simple Calendar, Calendaria, Fantasy-Calendar, or Chronicle JSON

### Timeline
- **Interactive D3 Visualization** — SVG-rendered timeline with zoom, pan, and minimap
- **Eras** — Named time periods with colored bars spanning year ranges
- **Standalone Events** — Calendar-free events for campaigns without formal calendars
- **Event Clustering** — Overlapping events automatically group for readability

### Maps
- **Leaflet.js Maps** — Upload custom map images and place interactive markers
- **Entity-Linked Markers** — Pin entities to map locations with click-through navigation
- **DM-Only Markers** — Hide map pins from players

### Game Sessions
- **Session Scheduling** — Plan game nights with date, location, and status tracking
- **RSVP** — Going / Maybe / Can't buttons with attendee tracking
- **Entity Linking** — Tag which pages were relevant to each session

### Campaign Management
- **Roles** — Owner (GM), Scribe (co-GM), and Player roles with granular permissions
- **Customizable Dashboards** — Drag-and-drop dashboard blocks (recent pages, calendar preview, timeline, maps, stats)
- **Customizable Sidebar** — Reorder, rename, and add custom navigation links
- **Category Dashboards** — Per-category landing pages with their own layouts
- **Public Campaigns** — Optionally make campaigns publicly viewable
- **"View as Player"** — Toggle to see your campaign as players see it

### Player Notes
- **Per-Entity Notes** — Private notes attached to any page
- **Shared Notes** — Share notes with the campaign (with edit locking)
- **Version History** — View and restore previous note versions
- **Checklists** — Quick checklist blocks within notes

### REST API
- **API v1** — Full CRUD for entities, calendar, and events
- **API Key Auth** — Per-campaign API keys with read/write/sync permissions
- **Sync Protocol** — Device fingerprinting for external tool integration (Foundry VTT planned)

### Admin & Security
- **User Management** — Admin dashboard for users, campaigns, storage, and security
- **Audit Logging** — Full activity trail for all campaign mutations
- **Rate Limiting** — Per-route rate limits on auth and upload endpoints
- **Session Security** — Redis-backed sessions with force-logout and session termination
- **IDOR Protection** — Campaign-scoped access checks on every route
- **Argon2id** — Password hashing with modern algorithm

---

## Planned Features

- Per-entity permissions (view/edit per role)
- Media browser for campaign owners
- Map layers, marker groups, and nested maps
- Relations graph visualization
- Editor tables and callout blocks
- Game system modules (D&D 5e SRD, Pathfinder 2e, Draw Steel)
- Campaign export/import
- Foundry VTT sync
- Auto-linking (LegendKeeper-style entity name detection)

See [.ai/todo.md](.ai/todo.md) for the full backlog.

---

## Screenshots

<!-- TODO: Add screenshots for each feature area -->
<!-- Suggested: dashboard, entity page, calendar, timeline, map, editor, notes -->

---

## Quick Start

### Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/keyxmakerx/chronicle.git
cd chronicle

# Set required secrets
export SECRET_KEY=$(openssl rand -base64 32)
export DB_PASSWORD=your-secure-password
export MYSQL_ROOT_PASSWORD=your-root-password

# Start the full stack
docker compose up -d

# Chronicle is now running at http://localhost:8080
# The first user to register becomes the site admin.
```

### From Source

**Prerequisites:** Go 1.24+, Node.js (for Tailwind), MariaDB 10.11+, Redis 7+

```bash
# Clone and setup
git clone https://github.com/keyxmakerx/chronicle.git
cd chronicle
cp .env.example .env       # Edit with your database credentials

# Start dependencies
make docker-up              # MariaDB + Redis via Docker

# Generate templates and CSS
make generate               # Runs templ generate + tailwindcss

# Run the server
make dev                    # Hot reload with air
# or
make run                    # Direct run
```

Database migrations run automatically on startup. The first user to register becomes the site admin.

---

## Development

```bash
make help            # Show all available commands
make dev             # Start dev server with hot reload (air)
make build           # Production binary build
make test            # Run all tests
make test-unit       # Unit tests only
make lint            # Run golangci-lint
make generate        # Regenerate Templ + Tailwind
make docker-up       # Start MariaDB + Redis
make docker-down     # Stop containers
```

### Project Structure

```
cmd/server/          # Application entrypoint
internal/
  plugins/           # Feature apps (auth, campaigns, entities, calendar, ...)
  modules/           # Game system content packs (dnd5e)
  widgets/           # Reusable UI components (editor, tags, relations, notes, ...)
  templates/         # Templ layouts and shared components
  middleware/        # HTTP middleware (auth, CSRF, logging, recovery)
  apperror/          # Domain error types
  config/            # Environment configuration
  database/          # Database connection and helpers
static/
  js/                # Client-side JavaScript (boot.js, widgets, search, shortcuts)
  css/               # Tailwind input + compiled output
  img/               # Static assets
db/migrations/       # Sequential SQL migration files
```

### Architecture

Chronicle uses a **three-tier extension architecture**:

| Tier | Purpose | Example |
|------|---------|---------|
| **Plugin** | Feature app with handler/service/repo/templates | auth, campaigns, entities, calendar, maps |
| **Module** | Game system content pack (read-only reference data) | dnd5e, pathfinder |
| **Widget** | Reusable UI component (self-contained JS + API) | editor, tags, relations, notes |

Request flow: `Router → Middleware → Handler → Service → Repository → MariaDB`

See [.ai/architecture.md](.ai/architecture.md) for the full architecture document.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| **Backend** | Go 1.24, [Echo v4](https://echo.labstack.com/) |
| **Templates** | [Templ](https://templ.guide/) (type-safe Go templates) |
| **Frontend** | [HTMX](https://htmx.org/), [Alpine.js](https://alpinejs.dev/) |
| **Editor** | [TipTap](https://tiptap.dev/) (ProseMirror-based) |
| **CSS** | [Tailwind CSS](https://tailwindcss.com/) |
| **Timeline** | [D3.js](https://d3js.org/) |
| **Maps** | [Leaflet.js](https://leafletjs.com/) |
| **Database** | MariaDB 10.11 |
| **Cache/Sessions** | Redis 7 |
| **Deployment** | Docker, multi-stage builds |

---

## Inspiration & Credits

Chronicle draws inspiration from the best in the TTRPG worldbuilding space:

- **[World Anvil](https://www.worldanvil.com/)** — Guided worldbuilding prompts, deep secrets system, 45+ RPG system support. Chronicle aims for similar breadth with a cleaner UI and no paywall.
- **[Kanka](https://kanka.io/)** — Generous free tier, deep permissions, best-in-class calendar. Chronicle's calendar and permission model are influenced by Kanka's approach.
- **[LegendKeeper](https://www.legendkeeper.com/)** — Auto-linking, speed-first philosophy, beautiful maps. Chronicle aspires to LegendKeeper's polish and interaction depth.
- **[Obsidian](https://obsidian.md/)** — Plugin ecosystem, graph visualization, local-first philosophy. Chronicle's addon system aims to enable similar extensibility for TTRPG-specific features.

---

## Contributing

Chronicle is in active early development (pre-alpha). Contribution guidelines will be established as the project matures. In the meantime, feel free to open issues for bug reports or feature suggestions.

---

## License

<!-- TODO: Choose and add a license (AGPL-3.0 recommended for self-hosted open source) -->

License TBD. See [LICENSE](LICENSE) when available.
