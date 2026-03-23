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
Returns the full calendar with all sub-resources eager-loaded: months, weekdays,
moons, seasons, eras, event_categories, cycles, festivals.

**Response:**
```json
{
  "id": "uuid",
  "campaign_id": "uuid",
  "mode": "fantasy",
  "name": "Calendar of Harptos",
  "description": "...",
  "epoch_name": "DR",
  "current_year": 1492,
  "current_month": 1,
  "current_day": 15,
  "current_hour": 14,
  "current_minute": 30,
  "hours_per_day": 24,
  "minutes_per_hour": 60,
  "seconds_per_minute": 60,
  "leap_year_every": 4,
  "leap_year_offset": 0,
  "months": [{ "id": 1, "name": "Hammer", "days": 30, "sort_order": 0, "is_intercalary": false, "leap_year_days": 0 }],
  "weekdays": [{ "id": 1, "name": "First Day", "sort_order": 0, "is_rest_day": false }],
  "moons": [{ "id": 1, "name": "Selûne", "cycle_days": 30.0, "phase_offset": 0.0, "color": "#c0c0ff" }],
  "seasons": [{ "id": 1, "name": "Winter", "start_month": 11, "start_day": 1, "end_month": 2, "end_day": 28, "color": "#a0c4ff" }],
  "eras": [{ "id": 1, "name": "Dale Reckoning", "start_year": 1, "end_year": null, "color": "#6366f1", "sort_order": 0 }],
  "event_categories": [{ "id": 1, "slug": "holiday", "name": "Holiday", "icon": "⭐", "color": "#f59e0b", "sort_order": 0 }],
  "cycles": [{ "id": 1, "name": "Zodiac", "cycle_length": 12, "type": "yearly", "sort_order": 0, "entries": [...] }],
  "festivals": [{ "id": 1, "name": "Midsummer", "month": 7, "day": null, "after_month": 7, "sort_order": 0 }]
}
```

#### GET /calendar/date
Returns current date/time with computed state: current season, moon phases, era, weather.

**Used by:** `calendar-sync.mjs` → poll current state

**Response:**
```json
{
  "mode": "fantasy",
  "year": 1492,
  "month": 1,
  "day": 15,
  "hour": 14,
  "minute": 30,
  "current_season": {
    "id": 1,
    "name": "Winter",
    "color": "#a0c4ff"
  },
  "current_moon_phases": [
    {
      "moon_id": 1,
      "moon_name": "Selûne",
      "phase_name": "Full Moon",
      "phase_position": 0.5,
      "phase_icon": "moon"
    }
  ],
  "current_era": {
    "id": 1,
    "name": "Dale Reckoning",
    "start_year": 1,
    "color": "#6366f1"
  },
  "current_weather": {
    "preset_id": "rain",
    "preset_label": "Rain",
    "icon": "cloud-rain",
    "color": "#6b9bd2",
    "temperature_celsius": 12.0,
    "wind": { "speed_kph": 25.0, "speed_tier": "moderate", "direction": "NW", "direction_degrees": 315 },
    "precipitation": { "type": "rain", "intensity": 0.6 },
    "zone_id": "temperate",
    "zone_name": "Temperate",
    "description": "Steady rainfall"
  }
}
```

**Key:** `current_season`, `current_moon_phases`, `current_era`, and `current_weather` are
computed server-side. They may be `null`/absent if no data is configured.

#### PUT /calendar/date
Sets current calendar date/time to an absolute value.

**Request:**
```json
{ "year": 1492, "month": 3, "day": 1, "hour": 8, "minute": 0 }
```

#### POST /calendar/advance
Advances the calendar by N days (1–3650).

**Request:** `{ "days": 7 }`

#### POST /calendar/advance-time
Advances time by hours/minutes (rolls over into days).

**Request:** `{ "hours": 2, "minutes": 30 }`

---

#### GET /calendar/seasons
Returns all season definitions.

**Response:**
```json
{
  "data": [
    {
      "id": 1, "name": "Winter",
      "start_month": 11, "start_day": 1, "end_month": 2, "end_day": 28,
      "description": "The cold months", "color": "#a0c4ff", "weather_effect": "snow"
    }
  ]
}
```

#### PUT /calendar/seasons
Replaces all season definitions (bulk replace).

**Request:** Array of season objects (same shape as response, without `id`/`calendar_id`).

#### GET /calendar/moons
Returns all moon definitions.

**Response:**
```json
{
  "data": [
    { "id": 1, "name": "Selûne", "cycle_days": 30.0, "phase_offset": 0.0, "color": "#c0c0ff" }
  ]
}
```

#### PUT /calendar/moons
Replaces all moon definitions.

**Request:** `[{ "name": "Selûne", "cycle_days": 30.0, "phase_offset": 0.0, "color": "#c0c0ff" }]`

#### GET /calendar/eras
Returns all era definitions.

**Response:**
```json
{
  "data": [
    { "id": 1, "name": "Dale Reckoning", "start_year": 1, "end_year": null, "description": "...", "color": "#6366f1", "sort_order": 0 }
  ]
}
```

#### PUT /calendar/eras
Replaces all era definitions.

#### GET /calendar/event-categories
Returns all event category definitions.

**Response:**
```json
{
  "data": [
    { "id": 1, "slug": "holiday", "name": "Holiday", "icon": "⭐", "color": "#f59e0b", "sort_order": 0 }
  ]
}
```

#### PUT /calendar/event-categories
Replaces all event categories.

**Request:** `[{ "slug": "holiday", "name": "Holiday", "icon": "⭐", "color": "#f59e0b", "sort_order": 0 }]`

#### GET /calendar/structure
Returns calendar structure in Calendaria-compatible format.

**Response:**
```json
{
  "id": "uuid",
  "name": "Calendar of Harptos",
  "mode": "fantasy",
  "hours_per_day": 24,
  "minutes_per_hour": 60,
  "seconds_per_minute": 60,
  "epoch_name": "DR",
  "months": [{ "id": 1, "name": "Hammer", "days": 30, "sort_order": 0, "is_intercalary": false, "leap_year_days": 0 }],
  "weekdays": [{ "id": 1, "name": "First Day", "sort_order": 0, "is_rest_day": false }],
  "leap_year": { "every": 4, "offset": 0 }
}
```

#### GET /calendar/weather
Returns current weather state, or `{}` if none set.

**Response:** Weather object (see GET /calendar/date response for shape).

#### PUT /calendar/weather
Sets current weather state (GM override).

**Request:**
```json
{
  "preset_id": "thunderstorm",
  "preset_label": "Thunderstorm",
  "icon": "cloud-bolt",
  "color": "#4a5568",
  "temperature_celsius": 18.0,
  "wind_speed_kph": 60.0,
  "wind_speed_tier": "strong",
  "wind_direction": "S",
  "wind_direction_degrees": 180,
  "precipitation_type": "rain",
  "precipitation_intensity": 0.9,
  "zone_id": "temperate",
  "zone_name": "Temperate",
  "description": "A violent thunderstorm rolls in"
}
```

#### GET /calendar/cycles
Returns zodiac/elemental cycle definitions with entries.

**Response:**
```json
{
  "data": [
    {
      "id": 1, "name": "Zodiac", "cycle_length": 12, "type": "yearly", "sort_order": 0,
      "entries": [
        { "id": 1, "name": "The Warrior", "icon": "shield", "year_offset": 0, "sort_order": 0 },
        { "id": 2, "name": "The Mage", "icon": "hat-wizard", "year_offset": 1, "sort_order": 1 }
      ]
    }
  ]
}
```

#### PUT /calendar/cycles
Replaces all cycle definitions (including entries).

**Request:**
```json
[
  {
    "name": "Zodiac", "cycle_length": 12, "type": "yearly", "sort_order": 0,
    "entries": [
      { "name": "The Warrior", "icon": "shield", "year_offset": 0, "sort_order": 0 }
    ]
  }
]
```

#### GET /calendar/festivals
Returns fixed calendar festival entries.

**Response:**
```json
{
  "data": [
    {
      "id": 1, "name": "Midsummer", "month": null, "day": null, "after_month": 7,
      "description": "The grand festival", "color": "#ffd700", "icon": "star", "sort_order": 0
    }
  ]
}
```

#### PUT /calendar/festivals
Replaces all festival definitions.

**Request:** `[{ "name": "Midsummer", "after_month": 7, "description": "...", "color": "#ffd700", "icon": "star", "sort_order": 0 }]`

---

#### Calendar Events

#### GET /calendar/events
Lists events for a month. Query: `?year=1492&month=3` or `?entity_id=uuid`.

#### POST /calendar/events
Creates a calendar event.

**Request:**
```json
{
  "name": "Festival of the Moon",
  "description": "ProseMirror JSON or plain text",
  "description_html": "<p>Rendered HTML</p>",
  "entity_id": "optional-entity-uuid",
  "year": 1492, "month": 11, "day": 30,
  "start_hour": 8, "start_minute": 0,
  "end_year": 1492, "end_month": 12, "end_day": 1,
  "end_hour": 23, "end_minute": 59,
  "is_recurring": true,
  "recurrence_type": "yearly",
  "recurrence_interval": 1,
  "recurrence_end_year": null, "recurrence_end_month": null, "recurrence_end_day": null,
  "recurrence_max_occurrences": null,
  "visibility": "everyone",
  "category": "festival",
  "color": "#ffd700",
  "icon": "star",
  "all_day": true
}
```

**New fields (Calendaria parity):**
- `color` — Hex color for calendar display
- `icon` — Icon identifier (FontAwesome or custom)
- `all_day` — Whether event spans entire day(s) vs. specific times
- `recurrence_interval` — How many periods between recurrences (e.g., every 2 years)
- `recurrence_end_year/month/day` — When recurrence stops
- `recurrence_max_occurrences` — Maximum number of recurrences

#### PUT /calendar/events/:eventId
Updates a calendar event. Same fields as POST.

#### DELETE /calendar/events/:eventId
Deletes a calendar event.

#### GET /calendar/events/:eventId
Returns a single event by ID.

---

#### Calendar Settings & Structure

#### PUT /calendar/settings
Updates calendar name, time system, leap year, current date/time.

#### PUT /calendar/months
Replaces all month definitions. `[{ "name": "Hammer", "days": 30, "sort_order": 0, "is_intercalary": false, "leap_year_days": 0 }]`

#### PUT /calendar/weekdays
Replaces all weekday definitions. `[{ "name": "First Day", "sort_order": 0, "is_rest_day": false }]`

#### GET /calendar/export
Exports the full calendar as Chronicle JSON. Add `?events=true` to include events.

#### POST /calendar/import
Imports a calendar from JSON (Chronicle, Simple Calendar, Calendaria, Fantasy-Calendar formats).

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
| `entity_type.created` | Full entity type object | Entity type created |
| `entity_type.updated` | Full entity type object | Entity type modified |
| `entity_type.deleted` | `{ id: "uuid" }` | Entity type deleted |
| `drawing.created` | Full drawing object | Map drawing created |
| `drawing.updated` | Full drawing object | Map drawing modified |
| `drawing.deleted` | `{ id: "uuid", map_id: "uuid" }` | Map drawing deleted |
| `token.created` | Full token object | Map token created |
| `token.moved` | `{ id, map_id, x, y }` | Map token moved |
| `token.updated` | Full token object | Map token modified |
| `token.deleted` | `{ id: "uuid", map_id: "uuid" }` | Map token deleted |
| `marker.created` | Full marker object | Map marker created |
| `marker.updated` | Full marker object | Map marker modified |
| `marker.deleted` | `{ id: "uuid" }` | Map marker deleted |
| `fog.updated` | `{ map_id, fog_data }` | Fog of war changed |
| `layer.updated` | Full layer object | Map layer changed |
| `note.created` | Full note object | Note created |
| `note.updated` | Full note object | Note modified |
| `note.deleted` | `{ id: "uuid" }` | Note deleted |
| `calendar.event.created` | Full event object | Calendar event created |
| `calendar.event.updated` | Full event object | Calendar event modified |
| `calendar.event.deleted` | `{ id: "uuid" }` | Calendar event deleted |
| `calendar.date.advanced` | `{ year, month, day, hour, minute }` | Calendar date/time changed |
| `calendar.season.changed` | `{ id, name, color }` | Season boundary crossed |
| `calendar.moon.phase_changed` | `{ moon_id, moon_name, phase_name, phase_position }` | Moon phase changed |
| `calendar.weather.changed` | Weather input object | Weather set or generated |
| `calendar.structure.updated` | `null` | Calendar structure modified (months, weekdays, etc.) |
| `calendar.era.changed` | `{ id, name, color }` | Era boundary crossed |
| `sync.status` | `{ connected: bool }` | Connection state change |
| `sync.error` | `{ message: "..." }` | Synchronization error |
| `sync.conflict` | Conflict details | Data conflict detected |

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
