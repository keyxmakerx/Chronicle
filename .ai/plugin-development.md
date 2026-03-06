# Chronicle WASM Plugin Development Guide

## Overview

Chronicle supports server-side logic extensions via WebAssembly (WASM) plugins.
Plugins run in a sandboxed environment powered by Extism/wazero, with
capability-based security controlling which host functions are available.

Plugins can:
- React to entity/calendar/tag events via hooks
- Read and write entities, tags, calendar events, and relations
- Store per-plugin data in a key-value store
- Send messages to other plugins
- Export custom functions callable via the API

## Quick Start

1. Create an extension directory under `extensions/your-plugin-name/`
2. Write a `manifest.json` declaring your WASM plugin
3. Implement your plugin in Rust, Go, or any language that compiles to WASM
4. Build the `.wasm` binary
5. Install the extension via the admin UI

## Manifest Structure

```json
{
  "manifest_version": 1,
  "id": "my-plugin",
  "name": "My Plugin",
  "version": "1.0.0",
  "description": "What it does.",
  "author": { "name": "Your Name" },
  "license": "MIT",
  "contributes": {
    "wasm_plugins": [
      {
        "slug": "my-logic",
        "name": "My Logic",
        "description": "Detailed description.",
        "file": "dist/my_plugin.wasm",
        "capabilities": ["log", "entity_read", "kv_store"],
        "hooks": ["entity.created"],
        "config": [
          { "key": "setting1", "label": "Setting", "type": "string" }
        ],
        "memory_limit_mb": 16,
        "timeout_secs": 30
      }
    ]
  }
}
```

## Capabilities

Plugins must declare required capabilities. Only matching host functions are
exposed to the WASM runtime.

| Capability | Host Functions | Description |
|---|---|---|
| `log` | `chronicle_log` | Server-side logging |
| `entity_read` | `get_entity`, `search_entities`, `list_entity_types` | Read entity data |
| `entity_write` | `update_entity_fields` | Modify entity custom fields |
| `calendar_read` | `get_calendar`, `list_events` | Read calendar data |
| `calendar_write` | `create_event` | Create calendar events |
| `tag_read` | `list_tags` | Read campaign tags |
| `tag_write` | `set_entity_tags`, `get_entity_tags` | Read/write entity tags |
| `relation_write` | `create_relation` | Create entity relations |
| `kv_store` | `kv_get`, `kv_set`, `kv_delete` | Per-plugin key-value storage |
| `message` | `send_message` | Plugin-to-plugin messaging |

## Host Function Reference

### chronicle_log
Log a message to the Chronicle server log.
- Input: `string` (max 4096 chars, truncated if longer)
- Output: `"ok"`

### get_entity
Get entity details by ID.
- Input: `{"entity_id": "..."}`
- Output: Entity JSON

### search_entities
Search entities in a campaign.
- Input: `{"campaign_id": "...", "query": "...", "limit": 10}`
- Output: JSON array of matching entities

### list_entity_types
List entity types for a campaign.
- Input: `{"campaign_id": "..."}`
- Output: JSON array of entity types

### update_entity_fields
Update custom fields on an entity.
- Input: `{"entity_id": "...", "fields": {"key": "value", ...}}`
- Output: `{"ok": true}`
- Max fields size: 256 KB

### get_calendar
Get calendar configuration for a campaign.
- Input: `{"campaign_id": "..."}`
- Output: Calendar config JSON

### list_events
List upcoming events for a campaign's calendar.
- Input: `{"campaign_id": "...", "limit": 50}`
- Output: JSON array of events

### create_event
Create a calendar event.
- Input: `{"name": "...", "year": 1492, "month": 6, "day": 15, ...}`
- Output: Created event JSON with ID
- Max input: 64 KB

### list_tags
List all tags for a campaign.
- Input: `{"campaign_id": "..."}`
- Output: JSON array of tags

### set_entity_tags
Replace all tags on an entity.
- Input: `{"entity_id": "...", "tag_ids": [1, 2, 3]}`
- Output: `{"ok": true}`

### get_entity_tags
Get tags currently on an entity.
- Input: `{"entity_id": "..."}`
- Output: JSON array of tags

### create_relation
Create a relation between two entities.
- Input: `{"source_entity_id": "...", "target_entity_id": "...", "relation_type": "...", ...}`
- Output: Created relation JSON with ID
- Max input: 64 KB

### kv_get
Read a value from the plugin's KV store.
- Input: `{"key": "..."}`
- Output: `{"value": "..."}` or `{"value": null}` if not found

### kv_set
Write a value to the plugin's KV store.
- Input: `{"key": "...", "value": "..."}`
- Output: `{"ok": true}`
- Max value: 64 KB

### kv_delete
Delete a key from the plugin's KV store.
- Input: `{"key": "..."}`
- Output: `{"ok": true}`

### send_message
Send an async message to another plugin.
- Input: `{"target_ext_id": "...", "target_slug": "...", "payload": {...}}`
- Output: `{"ok": true}` (delivery is fire-and-forget)
- Max payload: 64 KB
- The target receives the call on its `on_message` export

## Hooks

Plugins can subscribe to events by listing hook types in their manifest.
When an event fires, Chronicle calls the plugin's `on_hook` export with:

```json
{
  "type": "entity.created",
  "entity_id": "abc-123",
  "campaign_id": "camp-456",
  "data": { ... }
}
```

| Hook | Fires When |
|---|---|
| `entity.created` | New entity created |
| `entity.updated` | Entity modified |
| `entity.deleted` | Entity deleted |
| `calendar.event_created` | Calendar event created |
| `calendar.event_updated` | Calendar event modified |
| `calendar.event_deleted` | Calendar event deleted |
| `tag.added` | Tag applied to entity |
| `tag.removed` | Tag removed from entity |

Hook dispatch is async (fire-and-forget). The hook handler's return value is
logged but does not affect the triggering operation.

## Building Plugins

### Rust

```toml
# Cargo.toml
[lib]
crate-type = ["cdylib"]

[dependencies]
extism-pdk = "1.4"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
```

```bash
# .cargo/config.toml
[build]
target = "wasm32-unknown-unknown"
```

```bash
cargo build --release
```

See `extensions/example-wasm-rust/` for a complete example.

### Go / TinyGo

```
# go.mod
require github.com/extism/go-pdk v1.1.3
```

```bash
# TinyGo (smaller binaries):
tinygo build -o plugin.wasm -target wasip1 main.go

# Go 1.24+ (native):
GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm main.go
```

See `extensions/example-wasm-go/` for a complete example.

### Other Languages

Any language with an Extism PDK can build Chronicle plugins:
- AssemblyScript: `@nicholasgasior/extism-pdk`
- C: `extism/c-pdk`
- Haskell: `extism/haskell-pdk`
- Zig: `extism/zig-pdk`

## Testing Locally

### Go SDK Mock

The `sdk/go/chronicle` package provides a `MockHost` for testing plugin logic
without the full Chronicle runtime:

```go
import "github.com/keyxmakerx/chronicle/sdk/go/chronicle"

func TestMyPlugin(t *testing.T) {
    mock := chronicle.NewMockHost()
    mock.AddEntity("ent-1", map[string]any{"name": "Gandalf", "type": "npc"})
    mock.AddTag("camp-1", chronicle.Tag{ID: 1, Name: "NPC"})

    // Test your plugin logic using mock.GetEntity(), mock.ListTags(), etc.
    // Check results with mock.Logs(), mock.Events(), mock.EntityTagIDs(), etc.
}
```

### Extism CLI

The [Extism CLI](https://github.com/extism/cli) can invoke WASM plugins directly:

```bash
extism call dist/plugin.wasm roll --input '{"expression":"2d6+3"}'
```

Note: Host functions won't be available when testing with the CLI directly.
Use the Go SDK mock for integration testing of host function calls.

## Limits

| Resource | Default | Maximum |
|---|---|---|
| Memory | 16 MB | 256 MB |
| Timeout | 30s | 300s |
| KV value size | — | 64 KB |
| Log message | — | 4096 chars |
| Write payloads | — | 64-256 KB |

## API Endpoints

### Admin
- `GET /admin/extensions/wasm/plugins` — list all loaded WASM plugins
- `GET /admin/extensions/wasm/plugins/:extID/:slug` — get plugin info
- `POST /admin/extensions/wasm/plugins/:extID/:slug/reload` — reload plugin
- `POST /admin/extensions/wasm/plugins/:extID/:slug/stop` — stop plugin

### Campaign (Scribe+ role required)
- `GET /campaigns/:id/extensions/wasm/plugins` — list campaign WASM plugins
- `POST /campaigns/:id/extensions/wasm/call/:extID/:slug` — call plugin function

Call endpoint body:
```json
{
  "function": "roll",
  "input": {"expression": "2d6+3"}
}
```

Response:
```json
{
  "output": {"expression": "2d6+3", "rolls": [4, 2], "modifier": 3, "total": 9},
  "logs": ["chronicle_log output..."]
}
```
