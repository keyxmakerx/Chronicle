# Chronicle

**A self-hosted worldbuilding platform for tabletop RPG campaigns.**

Chronicle gives game masters and players a shared space to build worlds, track lore, and run campaigns — all on your own server, with no paywall, no forced public content, and full control over your data.

---

## Why Chronicle?

Chronicle is purpose-built for tabletop RPGs, open source, and designed to be self-hosted from day one. No paywalls, no forced public content, no vendor lock-in — your world, your server, your data.

---

## Features

### Worldbuilding
- **Pages & Categories** — Create any content type (characters, locations, factions, items, etc.) with custom categories and dynamic field schemas
- **Rich Text Editor** — TipTap-powered WYSIWYG with @mentions, backlinks, GM secrets, and an insert menu
- **Relations** — Bi-directional entity relationships with typed connections ("is spouse of", "leads", "enemy of")
- **Entity Hierarchy** — Parent/child nesting with tree views and breadcrumb navigation
- **Tags** — Color-coded tags with DM-only visibility controls
- **Custom Attributes** — Per-category field templates (text, number, select, checkbox, URL) with per-entity overrides
- **Drag-and-Drop Page Layouts** — Visual layout editor for entity profile pages

### Calendar
- **Custom Calendars** — Months, weekdays, moons, seasons and eras of your own, or the real-world calendar; import from Calendaria, Simple Calendar and Fantasy Calendar
- **Events** — Kinds of event, repeats by rule with skipped or moved dates, and pages tied to events and eras
- **Weather** — Climate-based generation, locked days and an optional player forecast
- **Game Nights** — Plan a game night in the calendar, with a "Who's free" view of when everyone is available

### Timeline
- **Interactive D3 Visualization** — SVG-rendered timeline with zoom, pan, and minimap
- **Standalone Events** — Calendar-free events, organized into swim-lanes by entity group
- **Event Clustering** — Overlapping events at low zoom automatically group into count badges for readability

### Maps
- **Leaflet.js Maps** — Upload custom map images and place interactive markers
- **Entity-Linked Markers** — Pin entities to map locations with click-through navigation
- **DM-Only Markers** — Hide map pins from players

### Armory, Shops & Stashes
- **Armory** — Item galleries with tag filters, collections and character money history
- **Shops** — A Shop block you add to a page, opening onto a walk-in shop room; players buy as their character, or ask the GM to when downtime is closed
- **Stashes** — Shared hoards of items and money, with GM-approved moves and a downtime switch

### DM Screen
- **GM Control Panel** — A three-leaf screen for owners and scribes: party at a glance, the world's date and weather, conditions and NPC reveals

### Game Sessions
- **Session Scheduling** — Plan game nights with date, location, and status tracking
- **RSVP** — Going / Maybe / Can't buttons with attendee tracking
- **Entity Linking** — Tag which pages were relevant to each session

### Campaign Management
- **Roles** — Owner (GM), Scribe, and Player roles with granular permissions
- **Customizable Dashboards** — Drag-and-drop dashboard blocks (recent pages, maps, stats)
- **Site and Campaign Look** — Admin > Site look sets the name, logo and sign-in background; each person's My view sets light/dark, calmer motion, text size and contrast
- **Customizable Sidebar** — Reorder, rename, and add custom navigation links
- **Category Dashboards** — Per-category landing pages with their own layouts
- **Public Campaigns** — Optionally make campaigns publicly viewable
- **"View as Player"** — Toggle to see your campaign as players see it

### Player Notes
- **Per-Entity Notes** — Private notes attached to any page
- **Shared Notes** — Share notes with the campaign (with edit locking)
- **Version History** — View and restore previous note versions
- **Page History and Trash** — Every page keeps its text history; deleted pages wait in Trash and save clashes are caught instead of overwritten
- **Checklists** — Quick checklist blocks within notes

### REST API
- **API v1** — Full CRUD for entities, entity types, tags, relations, maps, drawings, tokens, layers, fog, media, and notes
- **API Key Auth** — Per-campaign API keys with read/write/sync permissions and device fingerprint binding
- **Sync History** — One record of what synced in both directions, with who, when and any failure (Manage › Sync history, for the owner and members with DM access); the Foundry module's History tab reads the same list
- **Addon Discovery** — External tools can detect which features are enabled per campaign
- **Bulk Operations** — Bulk tag assignment and entity type reassignment (up to 200 per request)
- **Sync Protocol** — Sync mappings, WebSocket real-time events, and bidirectional Foundry VTT integration through the Chronicle Sync module (journals, characters, maps, shops, stashes, player notes)

### Admin & Security
- **Startup Health Checks** — Automatic migration validation, schema verification, DB connectivity checks, and security audit on every server start
- **Pre-Migration Backups** — Optional mysqldump with gzip before schema changes, with automatic rotation
- **User Management** — Admin dashboard for users, campaigns, storage, and security
- **Audit Logging** — Full activity trail for all campaign mutations
- **Rate Limiting** — Per-route rate limits on auth and upload endpoints
- **Session Security** — Redis-backed sessions with force-logout and session termination
- **Database TLS** — Configurable TLS encryption for database connections (`DB_TLS_MODE`)
- **IDOR Protection** — Campaign-scoped access checks on every route
- **Argon2id** — Password hashing with modern algorithm

---

## Screenshots

> Screenshots coming soon — see the feature list above for what Chronicle offers today.

---

## Quick Start

> For production deployment, backups, upgrades, and rollback procedures, see [`docs/deployment.md`](docs/deployment.md).

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

**Prerequisites:** Go 1.27+, Node.js (only for `make tiptap-bundle`), MariaDB 10.11+, Redis 7+

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
  systems/           # Game system content packs (installed via package manager)
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

Chronicle has three kinds of extension: plugins (feature apps), systems (game system content packs installed from Admin > Packages) and widgets (reusable UI blocks).

See [.ai/architecture.md](.ai/architecture.md) for the full architecture document.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| **Backend** | Go 1.27, [Echo v4](https://echo.labstack.com/) |
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

Chronicle was developed with reference to several existing worldbuilding and note-taking platforms in the TTRPG space. We're grateful to the broader community for establishing patterns and conventions that inform what users expect from tools like these.

Notable platforms we studied during development include [World Anvil](https://www.worldanvil.com/), [Kanka](https://kanka.io/), [LegendKeeper](https://www.legendkeeper.com/), and [Obsidian](https://obsidian.md/). All design and code in Chronicle is original work.

---

## Contributing

Chronicle is in active early development (pre-alpha). Contribution guidelines will be established as the project matures. In the meantime, feel free to open issues for bug reports or feature suggestions.

---

## License

This project is licensed under the [GNU Affero General Public License v3.0](LICENSE).
