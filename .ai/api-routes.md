# API Routes

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Complete map of all HTTP endpoints. Avoids reading each          -->
<!--          plugin's routes.go to understand the API surface.               -->
<!-- Update: Whenever a route is added, removed, or its handler changes.      -->
<!-- ====================================================================== -->

> Routes marked with **(implemented)** have working handlers. Others are planned.

## Public Routes (No Auth Required) -- implemented

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

### Campaign Management (Plugin: campaigns) -- implemented

| Method | Path | Handler | Min Role | Description |
|--------|------|---------|----------|-------------|
| GET | `/campaigns` | Index | Auth only | List user's campaigns |
| GET | `/campaigns/new` | NewForm | Auth only | Create campaign form |
| POST | `/campaigns` | Create | Auth only | Create campaign |
| GET | `/campaigns/:id` | Show | Player | Campaign dashboard |
| GET | `/campaigns/:id/edit` | EditForm | Owner | Edit campaign form |
| PUT | `/campaigns/:id` | Update | Owner | Update campaign |
| DELETE | `/campaigns/:id` | Delete | Owner | Delete campaign |
| GET | `/campaigns/:id/settings` | Settings | Owner | Campaign settings page |
| GET | `/campaigns/:id/members` | Members | Player | Member list page |
| POST | `/campaigns/:id/members` | AddMember | Owner | Add member by email |
| DELETE | `/campaigns/:id/members/:uid` | RemoveMember | Owner | Remove member |
| PUT | `/campaigns/:id/members/:uid/role` | UpdateRole | Owner | Change member role |
| GET | `/campaigns/:id/transfer` | TransferForm | Owner | Transfer ownership form |
| POST | `/campaigns/:id/transfer` | Transfer | Owner | Initiate transfer |
| GET | `/campaigns/:id/accept-transfer` | AcceptTransfer | Auth only | Accept transfer (token) |
| POST | `/campaigns/:id/cancel-transfer` | CancelTransfer | Owner | Cancel pending transfer |

### Dashboard Redirect -- implemented

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/dashboard` | redirect | Redirects to `/campaigns` |

### Admin Panel (Plugin: admin) -- implemented

All routes require `auth.RequireAuth` + `auth.RequireSiteAdmin`.

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/admin` | Dashboard | Overview stats (users, campaigns, SMTP) |
| GET | `/admin/users` | Users | User management list |
| PUT | `/admin/users/:id/admin` | ToggleAdmin | Toggle user's admin flag |
| GET | `/admin/campaigns` | Campaigns | All campaigns list |
| DELETE | `/admin/campaigns/:id` | DeleteCampaign | Force-delete campaign |
| POST | `/admin/campaigns/:id/join` | JoinCampaign | Admin joins with role |
| DELETE | `/admin/campaigns/:id/leave` | LeaveCampaign | Admin leaves campaign |

### SMTP Settings (Plugin: smtp, under admin) -- implemented

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/admin/smtp` | Settings | SMTP settings form |
| PUT | `/admin/smtp` | UpdateSettings | Save SMTP settings |
| POST | `/admin/smtp/test` | TestConnection | Test SMTP connectivity |

### Entity Management (Plugin: entities) -- planned

| Method | Path | Handler | HTMX? | Description |
|--------|------|---------|-------|-------------|
| GET | `/campaigns/:cid/entities` | Index | Yes | List entities (filterable) |
| GET | `/campaigns/:cid/entities/new` | NewForm | Yes | Create entity form |
| POST | `/campaigns/:cid/entities` | Create | Yes | Create entity |
| GET | `/campaigns/:cid/entities/:eid` | Show | Yes | Entity profile page |
| GET | `/campaigns/:cid/entities/:eid/edit` | EditForm | Yes | Edit entity form |
| PUT | `/campaigns/:cid/entities/:eid` | Update | Yes | Update entity |
| DELETE | `/campaigns/:cid/entities/:eid` | Delete | Yes | Delete entity |

### Entity Shortcut Routes (by type) -- planned

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/campaigns/:cid/characters` | Index (type=character) | List characters |
| GET | `/campaigns/:cid/locations` | Index (type=location) | List locations |
| GET | `/campaigns/:cid/organizations` | Index (type=organization) | List orgs |
| GET | `/campaigns/:cid/items` | Index (type=item) | List items |

## REST API (for external clients like Foundry VTT) -- planned

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

### Widget API Endpoints -- planned

| Method | Path | Widget | Description |
|--------|------|--------|-------------|
| GET | `/api/v1/search/entities` | mentions | Search entities for @mentions |
| GET | `/api/v1/search/tags` | tags | Search tags for tag picker |

### Module (Game System) Routes -- planned

| Method | Path | Module | Description |
|--------|------|--------|-------------|
| GET | `/ref/:system/` | * | Reference index page |
| GET | `/ref/:system/:category` | * | Category listing (spells, monsters) |
| GET | `/ref/:system/:category/:slug` | * | Individual entry page |
| GET | `/api/v1/ref/:system/lookup` | * | Tooltip API (query param: `q`) |
