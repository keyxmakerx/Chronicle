# Chronicle Sync Module Architecture

## Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                   Foundry VTT (Client)                       │
│                                                              │
│  module.mjs (entry point)                                    │
│    │                                                         │
│    ├── settings.mjs ── 18 world-scoped settings              │
│    │                                                         │
│    └── SyncManager (sync-manager.mjs)                        │
│          │                                                   │
│          ├── ChronicleAPI (api-client.mjs)                    │
│          │     ├── REST client (fetch + Bearer auth)          │
│          │     └── WebSocket client (auto-reconnect)          │
│          │                                                   │
│          ├── JournalSync (journal-sync.mjs)                   │
│          ├── MapSync (map-sync.mjs)                           │
│          ├── CalendarSync (calendar-sync.mjs)                 │
│          ├── ActorSync (actor-sync.mjs)                       │
│          │     └── System Adapter (adapters/*.mjs)            │
│          ├── ItemSync (item-sync.mjs)                         │
│          └── ShopWidget (shop-widget.mjs)                     │
│                                                              │
│  SyncDashboard (sync-dashboard.mjs) ── standalone UI         │
└─────────────────────────────────────────────────────────────┘
         │                    ▲
         │ REST + WebSocket   │
         ▼                    │
┌─────────────────────────────────────────────────────────────┐
│               Chronicle Server (Go backend)                  │
│  /api/v1/campaigns/:id/*  (REST)                             │
│  /ws?token=<key>          (WebSocket)                        │
└─────────────────────────────────────────────────────────────┘
```

## Module Initialization Sequence

```
1. Foundry fires 'init' hook
   └── module.mjs calls registerSettings()
       └── settings.mjs registers 18 settings with game.settings

2. Foundry fires 'ready' hook (only GM proceeds)
   └── module.mjs creates SyncManager, calls start()
       ├── Validates configuration (URL, key, campaign ID)
       ├── Creates ChronicleAPI (REST + WebSocket)
       ├── Calls _detectSystem()
       │   ├── GET /systems → match foundry_system_id to game.system.id
       │   └── Falls back to SYSTEM_MAP_FALLBACK if no API match
       ├── Initializes sync modules:
       │   ├── JournalSync.init(api, syncManager)
       │   ├── MapSync.init(api, syncManager)
       │   ├── CalendarSync.init(api, syncManager)
       │   ├── ActorSync.init(api, syncManager, matchedSystem)
       │   │   └── _loadAdapter(matchedSystem)
       │   │       ├── Try built-in: dnd5e-adapter.mjs / pf2e-adapter.mjs
       │   │       └── Fall back to: createGenericAdapter(api, systemId)
       │   ├── ItemSync.init(api, syncManager)
       │   └── ShopWidget.init(api, syncManager)
       ├── Connects WebSocket
       └── Performs initial sync (GET /sync/pull?since=lastSyncTime)
```

## Data Flow Per Sync Type

### Journal Sync (journal-sync.mjs)
```
Chronicle Entity ←→ Foundry JournalEntry

Foundry → Chronicle:
  Hook: createJournalEntry / updateJournalEntry / deleteJournalEntry
  → journalSync.onFoundryChange(journal)
  → POST /entities or PUT /entities/:id (with HTML content)

Chronicle → Foundry:
  WebSocket: entity.created / entity.updated / entity.deleted
  → journalSync.onChronicleChange(entityData)
  → JournalEntry.create() or journal.update()

Permission Mapping:
  Chronicle visibility → Foundry ownership levels
  "public" → OBSERVER, "private" → NONE, etc.
  Configurable via syncPermissions and defaultOwnership settings
```

### Map Sync (map-sync.mjs)
```
Chronicle Map ←→ Foundry Scene

Three sub-channels:
1. Drawings: Chronicle drawing shapes ←→ Foundry Drawing documents
2. Tokens: Chronicle map tokens ←→ Foundry Token documents
3. Fog: Chronicle fog overlay ←→ Foundry fog exploration data

Coordinate Conversion:
  Chronicle uses percentage coordinates (0-100)
  Foundry uses pixel coordinates (0-sceneWidth/Height)
  Conversion: pixel = percentage * sceneDimension / 100
```

### Calendar Sync (calendar-sync.mjs)
```
Chronicle Calendar ←→ Foundry Calendar Module

Adapter pattern supports multiple Foundry calendar modules:
  - Calendaria: game.modules.get("calendaria")
  - Simple Calendar: SimpleCalendar.api

Date Indexing:
  Chronicle uses 1-indexed months/days
  Some Foundry calendars use 0-indexed
  Adapter handles conversion automatically

Events:
  POST/PUT/DELETE /calendar/events ←→ calendar module event API
```

### Character Sync (actor-sync.mjs)
```
Chronicle Character Entity ←→ Foundry Actor

System adapter converts between field formats:
  toChronicleFields(actor) → Chronicle fields_data object
  fromChronicleFields(entity) → Foundry actor.update() data

Actor Type:
  adapter.actorType determines which Actor type to create/filter
  D&D 5e: "character", Draw Steel: "hero", PF2e: "character"

Sync Direction:
  Foundry → Chronicle: Hook on updateActor → push fields_data
  Chronicle → Foundry: WebSocket entity.updated → apply actor.update()
```

## WebSocket Message Types

The module receives these message types from Chronicle via WebSocket:

| Type | Routed To | Payload |
|------|-----------|---------|
| `entity.created` | JournalSync | `{ id, name, content, entity_type_id, fields_data, ... }` |
| `entity.updated` | JournalSync, ActorSync | `{ id, name, content, fields_data, ... }` |
| `entity.deleted` | JournalSync | `{ id }` |
| `drawing.created` | MapSync | `{ id, map_id, shape, coordinates, ... }` |
| `drawing.updated` | MapSync | `{ id, map_id, ... }` |
| `drawing.deleted` | MapSync | `{ id, map_id }` |
| `token.created` | MapSync | `{ id, map_id, x, y, ... }` |
| `token.moved` | MapSync | `{ id, map_id, x, y }` |
| `token.updated` | MapSync | `{ id, map_id, ... }` |
| `token.deleted` | MapSync | `{ id, map_id }` |
| `fog.updated` | MapSync | `{ map_id, fog_data }` |
| `calendar.event.created` | CalendarSync | `{ id, title, date, ... }` |
| `calendar.event.updated` | CalendarSync | `{ id, title, date, ... }` |
| `calendar.event.deleted` | CalendarSync | `{ id }` |
| `calendar.date.advanced` | CalendarSync | `{ current_date }` |
| `sync.status` | SyncManager | `{ connected: bool }` |

## Sync Guard Pattern

Every sync operation uses a `_chronicleSyncing` flag to prevent infinite echo loops:

```javascript
// When applying changes FROM Chronicle TO Foundry:
this._chronicleSyncing = true;
try {
  await actor.update(updateData);
} finally {
  this._chronicleSyncing = false;
}

// In Foundry hook handlers, skip if we're currently syncing:
Hooks.on('updateActor', (actor, changes, options) => {
  if (this._chronicleSyncing) return; // Skip — this is our own change
  this._pushToChronicle(actor);
});
```

This pattern is used in JournalSync, MapSync, CalendarSync, and ActorSync.

## Sync Mapping System

The module tracks which Chronicle entities map to which Foundry documents using
the `/sync/mappings` API. Each mapping is:

```json
{
  "chronicle_id": "uuid-of-entity",
  "foundry_id": "foundry-document-id",
  "type": "entity|drawing|token|calendar_event",
  "last_synced": "2026-01-01T00:00:00Z"
}
```

The module uses `GET /sync/lookup?foundry_id=X` to find existing mappings and
`POST /sync/mappings` to create new ones during initial sync.

## Settings System

18 Foundry world-scoped settings, divided into:

### User-Configurable (shown in Foundry settings dialog)
| Key | Type | Default | Purpose |
|-----|------|---------|---------|
| `apiUrl` | String | `""` | Chronicle instance URL |
| `apiKey` | String | `""` | API key (masked as password input) |
| `campaignId` | String | `""` | Campaign UUID |
| `syncEnabled` | Boolean | `true` | Master sync toggle |
| `syncJournals` | Boolean | `true` | Journal sync toggle |
| `syncMaps` | Boolean | `true` | Map sync toggle |
| `syncCalendar` | Boolean | `false` | Calendar sync toggle |
| `syncCharacters` | Boolean | `false` | Character sync toggle |

### Internal (managed via dashboard UI or programmatically)
| Key | Type | Default | Purpose |
|-----|------|---------|---------|
| `detectedSystem` | String | `""` | Matched Chronicle system ID |
| `lastSyncTime` | String | `""` | ISO timestamp of last sync |
| `syncExclusions` | String (JSON) | `{"excludedTypes":[],"excludedEntities":[]}` | Excluded types/entities |
| `syncDirections` | String (JSON) | `{"journals":"both",...}` | Per-type sync direction |
| `syncPermissions` | Boolean | `true` | Sync Chronicle visibility → Foundry ownership |
| `defaultOwnership` | Number | `0` | Default Foundry ownership for new docs |
| `dmOnlyHidden` | Boolean | `true` | Hide DM-only entities in Foundry |
| `conflictResolution` | String | `"chronicle"` | Conflict strategy: chronicle/foundry/newest |
| `autoSync` | Boolean | `true` | Auto-sync on change vs manual-only |
| `excludedTags` | String (JSON) | `[]` | Tag-based exclusion list |
| `excludedNamePattern` | String | `""` | Name pattern exclusion substring |

## System Detection (SYSTEM_MAP_FALLBACK)

The module matches Foundry's `game.system.id` to Chronicle systems in two ways:

1. **API-driven (primary):** Query `/systems`, find system where `foundry_system_id === game.system.id`
2. **Hardcoded fallback (legacy):** `SYSTEM_MAP_FALLBACK` map in `sync-manager.mjs`:

```javascript
const SYSTEM_MAP_FALLBACK = {
  dnd5e: 'dnd5e',
  pf2e: 'pathfinder2e',
  'draw-steel': 'drawsteel',
};
```

The API-driven approach is preferred because it works for custom-uploaded systems
without any module code changes. The fallback exists only for backwards compatibility.
