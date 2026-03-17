# Chronicle API Contract

This document defines every Chronicle REST API endpoint and WebSocket message
that the Foundry module depends on. A new AI working on this module MUST
understand this contract to avoid breaking changes.

## Authentication

All REST requests include a Bearer token:
```
Authorization: Bearer <api-key>
```

WebSocket connections authenticate via query parameter:
```
wss://chronicle.example.com/ws?token=<api-key>
```

API keys are scoped to a single campaign. The key determines:
- Which campaign's data is accessible
- Permission level: `read`, `write`, `sync`
- Rate limit: 60 requests/minute (default)

## Base URL Pattern

All REST endpoints are prefixed with:
```
{chronicleUrl}/api/v1/campaigns/{campaignId}
```

The module's `api-client.mjs` constructs this from settings:
```javascript
const baseUrl = getSetting('apiUrl');     // e.g., "https://chronicle.example.com"
const campaignId = getSetting('campaignId'); // UUID
// Requests go to: baseUrl + "/api/v1/campaigns/" + campaignId + path
```

## Error Response Format

All error responses follow:
```json
{
  "error": "Human-readable error message"
}
```

HTTP status codes:
- `400` — Bad request (invalid input)
- `401` — Unauthorized (missing/invalid API key)
- `403` — Forbidden (insufficient permissions)
- `404` — Not found
- `429` — Rate limited
- `500` — Server error

---

## REST Endpoints

### Systems

#### GET /systems
Lists all available game systems for this campaign.

**Used by:** `sync-manager.mjs` → `_detectSystem()`

**Response:**
```json
{
  "data": [
    {
      "id": "dnd5e",
      "name": "D&D 5th Edition",
      "status": "available",
      "enabled": true,
      "has_character_fields": true,
      "has_item_fields": true,
      "foundry_system_id": "dnd5e"
    }
  ]
}
```

#### GET /systems/:systemId/character-fields
Returns character preset field definitions with Foundry annotations.

**Used by:** `adapters/generic-adapter.mjs` → `createGenericAdapter()`

**Response:**
```json
{
  "system_id": "drawsteel",
  "preset_slug": "drawsteel-character",
  "preset_name": "Draw Steel Hero",
  "foundry_system_id": "draw-steel",
  "foundry_actor_type": "hero",
  "fields": [
    {
      "key": "might",
      "label": "Might",
      "type": "number",
      "foundry_path": "system.characteristics.might.value",
      "foundry_writable": true
    },
    {
      "key": "stamina_max",
      "label": "Stamina (Max)",
      "type": "number",
      "foundry_path": "system.stamina.max",
      "foundry_writable": false
    }
  ]
}
```

**Key fields:**
- `foundry_actor_type` — Actor type to create/filter (e.g., "character", "hero")
- `foundry_path` — Dot-notation path on `actor.system` (e.g., "system.abilities.str.value")
- `foundry_writable` — Whether this field can be written back to Foundry (false = read-only from Foundry)

#### GET /systems/:systemId/item-fields
Returns item preset field definitions. Same shape as character-fields.

**Used by:** `item-sync.mjs`

---

### Entities

#### GET /entities
Lists entities in the campaign. Supports pagination and filtering.

**Used by:** `journal-sync.mjs` → initial sync

**Query params:** `?page=1&per_page=50&type_id=X&updated_since=ISO`

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "name": "Entity Name",
      "content": "<p>HTML content</p>",
      "summary": "Short text",
      "entity_type_id": 1,
      "fields_data": { "hp_current": 45, "str": 18 },
      "tags": ["npc", "villain"],
      "visibility": "public",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-15T12:00:00Z"
    }
  ],
  "pagination": { "page": 1, "per_page": 50, "total": 120 }
}
```

#### POST /entities
Creates a new entity.

**Used by:** `journal-sync.mjs` → push new journal to Chronicle

**Request:**
```json
{
  "name": "Entity Name",
  "content": "<p>HTML content</p>",
  "entity_type_id": 1,
  "visibility": "public"
}
```

**Response:** The created entity object (same shape as GET).

#### GET /entities/:entityId
Returns a single entity with full content.

#### PUT /entities/:entityId
Updates an entity.

**Request:** Same shape as POST (partial updates supported).

#### DELETE /entities/:entityId
Deletes an entity.

#### PUT /entities/:entityId/fields
Updates only the `fields_data` on an entity.

**Used by:** `actor-sync.mjs` → push character stats to Chronicle

**Request:**
```json
{
  "fields_data": {
    "hp_current": 45,
    "str": 18,
    "level": 5
  }
}
```

#### GET /entities/:entityId/permissions
Returns entity permission/visibility settings.

#### PUT /entities/:entityId/permissions
Updates entity permissions.

**Request:**
```json
{
  "visibility": "public"
}
```

#### POST /entities/:entityId/reveal
Toggles entity reveal state (NPC reveal to players).

**Used by:** `actor-sync.mjs`

---

### Entity Types

#### GET /entity-types
Lists all entity types in the campaign.

**Response:**
```json
{
  "data": [
    {
      "id": 1,
      "name": "Character",
      "slug": "dnd5e-character",
      "icon": "fa-user",
      "color": "#7C3AED"
    }
  ]
}
```

#### GET /entity-types/:typeId
Returns a single entity type with field definitions.

---

### Sync Mappings

#### GET /sync/mappings
Lists all sync mappings for the campaign.

**Used by:** `sync-manager.mjs` → initial sync setup

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "chronicle_id": "entity-uuid",
      "foundry_id": "foundry-doc-id",
      "type": "entity",
      "last_synced": "2026-01-15T12:00:00Z"
    }
  ]
}
```

#### POST /sync/mappings
Creates a new sync mapping.

**Request:**
```json
{
  "chronicle_id": "entity-uuid",
  "foundry_id": "foundry-doc-id",
  "type": "entity"
}
```

#### DELETE /sync/mappings/:mappingId
Removes a sync mapping.

#### GET /sync/lookup
Looks up a mapping by Foundry ID.

**Used by:** All sync modules to find existing mappings

**Query:** `?foundry_id=abc123&type=entity`

**Response:**
```json
{
  "chronicle_id": "entity-uuid",
  "foundry_id": "abc123",
  "type": "entity"
}
```
Returns 404 if no mapping exists.

#### GET /sync/pull
Pulls all changes since a timestamp.

**Used by:** `sync-manager.mjs` → initial sync

**Query:** `?since=2026-01-01T00:00:00Z`

**Response:**
```json
{
  "entities": [ /* updated/created entities */ ],
  "deleted_entities": [ "uuid1", "uuid2" ],
  "drawings": [ /* map drawings */ ],
  "tokens": [ /* map tokens */ ],
  "calendar_events": [ /* events */ ]
}
```

#### POST /sync
Generic sync endpoint for batch operations.

---

### Maps

#### GET /maps
Lists all maps in the campaign.

#### GET /maps/:mapId/drawings
Lists drawings for a map.

#### POST /maps/:mapId/drawings
Creates a new drawing.

#### PUT /maps/:mapId/drawings/:drawingId
Updates a drawing.

#### DELETE /maps/:mapId/drawings/:drawingId
Deletes a drawing.

#### GET /maps/:mapId/tokens
Lists tokens for a map.

#### POST /maps/:mapId/tokens
Creates a token.

#### PATCH /maps/:mapId/tokens/:tokenId/position
Moves a token (x, y only).

#### PUT /maps/:mapId/tokens/:tokenId
Full token update.

#### DELETE /maps/:mapId/tokens/:tokenId
Deletes a token.

#### GET /maps/:mapId/fog
Gets fog of war data.

#### PUT /maps/:mapId/fog
Updates fog of war data.

#### GET /maps/:mapId/layers
Lists map layers.

---

### Calendar

All calendar endpoints require the calendar addon to be enabled.

#### GET /calendar
Returns calendar configuration (months, weekdays, moons, eras, current date).

#### GET /calendar/date
Returns current calendar date.

#### PUT /calendar/date
Sets current calendar date.

#### POST /calendar/advance
Advances the calendar by days.

#### POST /calendar/advance-time
Advances the calendar by hours/minutes.

#### GET /calendar/events
Lists calendar events.

#### POST /calendar/events
Creates a calendar event.

#### PUT /calendar/events/:eventId
Updates a calendar event.

#### DELETE /calendar/events/:eventId
Deletes a calendar event.

#### PUT /calendar/settings
Updates calendar configuration.

#### PUT /calendar/months
Updates month definitions.

#### PUT /calendar/weekdays
Updates weekday definitions.

#### PUT /calendar/moons
Updates moon definitions.

#### PUT /calendar/eras
Updates era definitions.

---

### Media

#### POST /media/upload
Uploads a media file (image, etc.).

**Used by:** `api-client.mjs` for image sync

**Request:** Multipart form data with `file` field.

**Response:**
```json
{
  "id": "media-uuid",
  "url": "/media/media-uuid.png",
  "filename": "map-background.png",
  "content_type": "image/png",
  "size": 1048576
}
```

#### GET /media/:mediaId
Returns media metadata.

#### DELETE /media/:mediaId
Deletes a media file.

---

### Relations (Shops/Inventory)

#### GET /entities/:entityId/relations
Lists relations for an entity (used for shop inventory).

**Response:**
```json
{
  "data": [
    {
      "id": "relation-uuid",
      "source_id": "shop-entity-uuid",
      "target_id": "item-entity-uuid",
      "relation_type_id": 1,
      "metadata": { "quantity": 5, "equipped": false },
      "target": { "id": "item-uuid", "name": "Longsword", "fields_data": {...} }
    }
  ]
}
```

---

## WebSocket Protocol

### Connection
```
GET /ws?token=<api-key>
Upgrade: websocket
```

### Message Format (Server → Client)
```json
{
  "type": "entity.updated",
  "data": { /* payload */ }
}
```

### Message Types

| Type | Data Payload | Description |
|------|-------------|-------------|
| `entity.created` | Full entity object | New entity created in Chronicle |
| `entity.updated` | Full entity object | Entity modified in Chronicle |
| `entity.deleted` | `{ id: "uuid" }` | Entity deleted in Chronicle |
| `drawing.created` | Full drawing object | Map drawing created |
| `drawing.updated` | Full drawing object | Map drawing modified |
| `drawing.deleted` | `{ id: "uuid", map_id: "uuid" }` | Map drawing deleted |
| `token.created` | Full token object | Map token created |
| `token.moved` | `{ id, map_id, x, y }` | Map token moved |
| `token.updated` | Full token object | Map token modified |
| `token.deleted` | `{ id: "uuid", map_id: "uuid" }` | Map token deleted |
| `fog.updated` | `{ map_id, fog_data }` | Fog of war changed |
| `calendar.event.created` | Full event object | Calendar event created |
| `calendar.event.updated` | Full event object | Calendar event modified |
| `calendar.event.deleted` | `{ id: "uuid" }` | Calendar event deleted |
| `calendar.date.advanced` | `{ current_date }` | Calendar date changed |
| `sync.status` | `{ connected: bool }` | Connection state change |

### Reconnection

The API client automatically reconnects on WebSocket disconnection:
- Initial retry delay: 2 seconds
- Max retry delay: 30 seconds (exponential backoff)
- Infinite retries (never gives up)
- Queued messages are replayed on reconnection

---

## CORS Requirements

Chronicle must whitelist the Foundry VTT server's origin in its CORS configuration.
The module makes cross-origin requests from the Foundry server (typically
`http://localhost:30000` or a custom domain) to the Chronicle server.

CORS origins are managed in Chronicle's admin panel:
**Admin > API Settings > CORS Origin Whitelist**

Required CORS headers from Chronicle:
```
Access-Control-Allow-Origin: <foundry-origin>
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
Access-Control-Allow-Headers: Authorization, Content-Type
Access-Control-Allow-Credentials: true
```
