# Chronicle System Pack Architecture

## Overview

A Chronicle system pack is a directory containing a `manifest.json` file and a
`data/` directory with one JSON file per reference content category. Chronicle
loads these packs to provide browsable reference pages, searchable content,
hover tooltips, entity type presets, and Foundry VTT character sync mappings.

## How Chronicle Loads Systems

1. **Discovery:** Chronicle scans a directory for subdirectories containing `manifest.json`
2. **Validation:** `ValidateManifest()` checks required fields (id, name, version, api_version)
3. **Data loading:** `JSONProvider` reads `data/*.json`, maps filename stem to category slug
4. **Registration:** System is registered in the global registry by its `id`
5. **Serving:** HTTP handlers serve reference pages, search API, and tooltip API

Systems loaded via ZIP upload go through the same validation but are stored in
`media/systems/<campaignID>/` instead of the built-in directory.

## ZIP Package Format

Chronicle's upload endpoint expects:

```
my-system.zip
├── manifest.json          # REQUIRED: System metadata
└── data/                  # REQUIRED: At least one data file
    ├── spells.json
    ├── monsters.json
    └── ...
```

**Validation rules applied during upload:**
- ZIP must contain `manifest.json` at the root
- ZIP must contain at least one `data/*.json` file
- Each data file must parse as a JSON array of ReferenceItem objects
- Each data file must contain at least one item
- ZIP size limit: 50 MB
- Individual data file limit: 10 MB
- Path traversal (`..`) is rejected

---

## manifest.json Schema

### SystemManifest (top-level object)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique machine-readable identifier (e.g., `"dnd5e"`, `"drawsteel"`) |
| `name` | string | Yes | Human-readable display name (e.g., `"D&D 5th Edition"`) |
| `description` | string | No | Short summary of what the system provides |
| `version` | string | Yes | Semantic version (e.g., `"1.0.0"`) |
| `author` | string | No | Creator name or organization |
| `license` | string | No | Content license identifier (e.g., `"OGL-1.0a"`, `"ORC"`, `"CC-BY-4.0"`) |
| `icon` | string | No | Font Awesome icon class (e.g., `"fa-dragon"`, `"fa-dice-d20"`) |
| `api_version` | string | Yes | System framework version, must be `"1"` |
| `status` | string | No | `"available"` or `"coming_soon"` (default: `"coming_soon"`) |
| `categories` | CategoryDef[] | No | Reference content categories |
| `entity_presets` | EntityPresetDef[] | No | Entity type templates for campaigns |
| `relation_presets` | RelationPresetDef[] | No | Relation type templates |
| `foundry_system_id` | string | No | Foundry VTT `game.system.id` (e.g., `"dnd5e"`, `"draw-steel"`, `"pf2e"`) |
| `tooltip_template` | string | No | Custom HTML template (Go text/template syntax) |

### CategoryDef

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `slug` | string | Yes | URL-safe identifier (e.g., `"spells"`, `"monsters"`) |
| `name` | string | Yes | Display name (e.g., `"Spells"`, `"Monsters"`) |
| `icon` | string | No | Font Awesome icon class |
| `fields` | FieldDef[] | No | Schema for Properties keys on items in this category |

The `slug` must match the filename stem of the corresponding data file:
`categories[].slug = "spells"` → `data/spells.json`

### FieldDef

Used in both category fields (for reference item display) and entity preset fields
(for character/creature stat tracking with optional Foundry sync).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Property map key (e.g., `"level"`, `"school"`, `"cr"`) |
| `label` | string | Yes | Human-readable name (e.g., `"Spell Level"`, `"School"`) |
| `type` | string | Yes | Data type: `"string"`, `"number"`, `"list"`, `"markdown"` |
| `foundry_path` | string | No | Dot-notation path in Foundry Actor (e.g., `"system.abilities.str.value"`) |
| `foundry_writable` | boolean | No | Whether Chronicle can write this field back to Foundry. Defaults to `true` when `foundry_path` is set. |

**Field types:**
- `"string"` — plain text value
- `"number"` — numeric value (integer or float)
- `"list"` — comma-separated list displayed as tags
- `"markdown"` — rich text content

### EntityPresetDef

Entity presets create entity types when a campaign enables the system. For example,
enabling D&D 5e creates "D&D Character", "D&D Creature", and "D&D Item" entity types.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `slug` | string | Yes | URL-safe identifier (e.g., `"dnd5e-character"`) |
| `name` | string | Yes | Display name (e.g., `"D&D Character"`) |
| `name_plural` | string | No | Plural form (e.g., `"D&D Characters"`) |
| `icon` | string | No | Font Awesome icon class |
| `color` | string | No | Hex color for badge (e.g., `"#7C3AED"`) |
| `category` | string | No | Feature category: `"character"`, `"creature"`, `"item"` |
| `foundry_actor_type` | string | No | Foundry Actor type: `"character"`, `"hero"`, `"npc"`. Defaults to `"character"`. |
| `fields` | FieldDef[] | No | Default field definitions for entities of this type |

**Important conventions:**
- Character preset slugs MUST end with `"-character"` (e.g., `"dnd5e-character"`)
- Chronicle uses `CharacterPreset()` which finds the first preset with slug ending in `"-character"`
- Item presets MUST have `category: "item"`
- The `foundry_actor_type` tells the Foundry module which Actor type to create/filter

### RelationPresetDef

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `slug` | string | Yes | URL-safe identifier (e.g., `"has-item"`) |
| `name` | string | Yes | Display name (e.g., `"Has Item"`) |
| `reverse_name` | string | No | Reverse direction label (e.g., `"In Inventory Of"`) |
| `metadata_schema` | object | No | Map of field name → `{ type, default }` for relation metadata |

---

## ReferenceItem Schema (data files)

Each `data/<category>.json` file contains a JSON array of ReferenceItem objects:

```json
[
  {
    "id": "acid-splash",
    "name": "Acid Splash",
    "summary": "You hurl a bubble of acid at a creature.",
    "description": "Full spell text with markdown formatting...",
    "properties": {
      "level": 0,
      "school": "Conjuration",
      "casting_time": "1 action",
      "range": "60 feet",
      "components": "V, S",
      "duration": "Instantaneous"
    },
    "tags": ["cantrip", "conjuration", "acid"],
    "source": "SRD 5.1"
  }
]
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique within category. URL-safe slug (e.g., `"acid-splash"`) |
| `name` | string | Yes | Human-readable display name |
| `summary` | string | No | One-line description for lists and search results |
| `description` | string | No | Full reference text (supports markdown) |
| `properties` | object | No | Key-value map of structured data. Keys should match `FieldDef.key` in the manifest |
| `tags` | string[] | No | Searchable labels/categories |
| `source` | string | No | Attribution (e.g., `"SRD 5.1"`, `"ORC"`, `"Arcana Archive"`) |

**Note:** `category` and `system_id` are auto-populated by Chronicle at load time.
Do NOT include them in data files.

---

## Foundry VTT Integration

Systems can enable automatic character sync with Foundry VTT by providing:

1. **`foundry_system_id`** on the manifest — tells the Foundry module which game system this matches
2. **`foundry_actor_type`** on entity presets — tells the module which Actor type to create
3. **`foundry_path`** on entity preset fields — maps Chronicle fields to Foundry Actor data paths
4. **`foundry_writable`** on entity preset fields — controls sync direction

### How It Works

The Foundry module's generic adapter:
1. Queries `GET /systems/:id/character-fields` on the Chronicle API
2. Gets back field definitions with `foundry_path` annotations
3. Auto-generates bidirectional sync mappings
4. No Foundry module code changes needed for new systems

### Foundry Path Examples

| Chronicle Field | Foundry Path | System |
|----------------|-------------|--------|
| `str` | `system.abilities.str.value` | D&D 5e |
| `hp_current` | `system.attributes.hp.value` | D&D 5e |
| `might` | `system.characteristics.might.value` | Draw Steel |
| `stamina_current` | `system.stamina.value` | Draw Steel |
| `level` | `system.details.level.value` | PF2e |

### Read-Only Fields

Set `foundry_writable: false` for fields that are calculated/derived in Foundry
(e.g., AC, proficiency bonus, speed). These fields are read FROM Foundry but
never written back, preventing Chronicle from overwriting Foundry's calculations.

---

## API Endpoints That Serve System Data

When loaded into Chronicle, systems are accessible via these endpoints:

### Web UI Endpoints (HTML/HTMX)

| Endpoint | Description |
|----------|-------------|
| `GET /campaigns/:id/systems/:mod` | System index page (category list) |
| `GET /campaigns/:id/systems/:mod/:cat` | Category item list |
| `GET /campaigns/:id/systems/:mod/:cat/:item` | Item detail page |
| `GET /campaigns/:id/systems/:mod/search?q=X` | Search API (JSON) |
| `GET /campaigns/:id/systems/:mod/:cat/:item/tooltip` | Tooltip API (JSON) |

### REST API Endpoints (JSON, for Foundry module)

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/campaigns/:id/systems` | List all systems (with `foundry_system_id`) |
| `GET /api/v1/campaigns/:id/systems/:id/character-fields` | Character field definitions with Foundry annotations |
| `GET /api/v1/campaigns/:id/systems/:id/item-fields` | Item field definitions with Foundry annotations |

### Tooltip API Response

```json
{
  "name": "Fireball",
  "category": "Spells",
  "summary": "A bright streak flashes...",
  "properties": { "level": 3, "school": "Evocation" },
  "tags": ["evocation", "fire"],
  "source": "SRD 5.1",
  "tooltip_html": "<div class='tooltip'>...</div>"
}
```

### Search API Response

```json
[
  {
    "id": "fireball",
    "name": "Fireball",
    "category": "spells",
    "summary": "A bright streak flashes...",
    "system_id": "dnd5e",
    "url": "/campaigns/uuid/systems/dnd5e/spells/fireball"
  }
]
```

---

## Validation Report

When a system is uploaded, Chronicle generates a `ValidationReport`:

```json
{
  "category_count": 6,
  "total_fields": 25,
  "preset_count": 3,
  "has_character_preset": true,
  "character_field_count": 15,
  "has_item_preset": true,
  "item_field_count": 4,
  "foundry_compatible": true,
  "foundry_system_id": "dnd5e",
  "foundry_mapped_fields": 15,
  "foundry_writable_fields": 8,
  "warnings": []
}
```

Common warnings:
- "No reference data categories defined"
- "No entity presets defined"
- "No character preset found (slug ending in '-character')"
- "foundry_system_id is set but no fields have foundry_path"
