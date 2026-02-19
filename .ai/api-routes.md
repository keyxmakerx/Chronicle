# API Routes

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Complete map of all HTTP endpoints. Avoids reading each          -->
<!--          plugin's routes.go to understand the API surface.               -->
<!-- Update: Whenever a route is added, removed, or its handler changes.      -->
<!-- ====================================================================== -->

> **NOTE:** No routes exist yet. This is the PLANNED route design. Routes will
> be added here as they are implemented.

## Public Routes (No Auth Required)

| Method | Path | Plugin | Handler | Description |
|--------|------|--------|---------|-------------|
| GET | `/` | - | pages.Landing | Landing page |
| GET | `/login` | auth | LoginForm | Login form |
| POST | `/login` | auth | Login | Process login |
| GET | `/register` | auth | RegisterForm | Registration form |
| POST | `/register` | auth | Register | Process registration |
| POST | `/logout` | auth | Logout | Destroy session |
| GET | `/healthz` | - | Healthcheck | Health check endpoint |

## Authenticated Routes

### Campaign Management (Plugin: campaigns)

| Method | Path | Handler | HTMX? | Description |
|--------|------|---------|-------|-------------|
| GET | `/campaigns` | Index | Yes | List user's campaigns |
| GET | `/campaigns/new` | NewForm | Yes | Create campaign form |
| POST | `/campaigns` | Create | Yes | Create campaign |
| GET | `/campaigns/:id` | Show | Yes | Campaign dashboard |
| GET | `/campaigns/:id/edit` | EditForm | Yes | Edit campaign form |
| PUT | `/campaigns/:id` | Update | Yes | Update campaign |
| DELETE | `/campaigns/:id` | Delete | Yes | Delete campaign |
| GET | `/campaigns/:id/settings` | Settings | No | Campaign settings page |

### Entity Management (Plugin: entities)

| Method | Path | Handler | HTMX? | Description |
|--------|------|---------|-------|-------------|
| GET | `/campaigns/:cid/entities` | Index | Yes | List entities (filterable) |
| GET | `/campaigns/:cid/entities/new` | NewForm | Yes | Create entity form |
| POST | `/campaigns/:cid/entities` | Create | Yes | Create entity |
| GET | `/campaigns/:cid/entities/:eid` | Show | Yes | Entity profile page |
| GET | `/campaigns/:cid/entities/:eid/edit` | EditForm | Yes | Edit entity form |
| PUT | `/campaigns/:cid/entities/:eid` | Update | Yes | Update entity |
| DELETE | `/campaigns/:cid/entities/:eid` | Delete | Yes | Delete entity |

### Entity Shortcut Routes (by type)

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/campaigns/:cid/characters` | Index (type=character) | List characters |
| GET | `/campaigns/:cid/locations` | Index (type=location) | List locations |
| GET | `/campaigns/:cid/organizations` | Index (type=organization) | List orgs |
| GET | `/campaigns/:cid/items` | Index (type=item) | List items |

## REST API (for external clients like Foundry VTT)

### Authentication

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/token` | Get API token (Personal Access Token) |

### Campaigns

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/campaigns` | List campaigns |
| GET | `/api/v1/campaigns/:id` | Get campaign details |

### Entities

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/campaigns/:id/entities` | List entities (filter by type, tag) |
| GET | `/api/v1/campaigns/:id/entities/:eid` | Get entity with fields |
| GET | `/api/v1/campaigns/:id/entities/:eid/entry` | Get entry (JSON or HTML) |
| PUT | `/api/v1/campaigns/:id/entities/:eid/entry` | Update entry content |
| GET | `/api/v1/campaigns/:id/entity-types` | List entity types |
| GET | `/api/v1/campaigns/:id/tags` | List tags |

### Widget API Endpoints

| Method | Path | Widget | Description |
|--------|------|--------|-------------|
| GET | `/api/v1/search/entities` | mentions | Search entities for @mentions |
| GET | `/api/v1/search/tags` | tags | Search tags for tag picker |

### Module (Game System) Routes

| Method | Path | Module | Description |
|--------|------|--------|-------------|
| GET | `/ref/:system/` | * | Reference index page |
| GET | `/ref/:system/:category` | * | Category listing (spells, monsters) |
| GET | `/ref/:system/:category/:slug` | * | Individual entry page |
| GET | `/api/v1/ref/:system/lookup` | * | Tooltip API (query param: `q`) |
