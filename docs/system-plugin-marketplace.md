# System Plugin Marketplace - Architecture & Specification

## 1. Executive Summary

Game systems in Chronicle are **independently developed, submitted, reviewed, installed, and activated per-campaign**. Chronicle provides a generic runtime (system loader, tooltip renderer, entity presets, Foundry sync); all system-specific behavior lives in the system package itself.

System packages are distributed as GitHub repositories with tagged releases. Admins install them via the package manager. Campaign owners enable them as addons. The Foundry VTT module auto-detects the active system and syncs characters using field annotations from the manifest.

No custom Go code is required for any game system. Everything is manifest-driven.

## 2. System Package Format

A system package is a directory (or ZIP archive) containing:

```
<system-id>/
  manifest.json          # Required: system metadata, categories, entity presets
  data/
    <category-slug>.json # One file per category (array of ReferenceItem objects)
  widgets/
    <widget-slug>.js     # Optional: custom JS widgets
```

### manifest.json Schema

See `docs/repo-templates/chronicle-systems/schema/manifest.schema.json` for the full JSON Schema.

**Required fields:**
| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier (lowercase, no spaces) |
| `name` | string | Display name |
| `version` | string | Semver (e.g., "1.0.0") |
| `api_version` | string | Must be "1" |

**Optional fields:**
| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Short summary |
| `author` | string | Creator name |
| `license` | string | Content license (OGL-1.0a, CC-BY-4.0, ORC, etc.) |
| `icon` | string | Font Awesome icon class |
| `status` | string | "available" or "coming_soon" |
| `foundry_system_id` | string | Foundry VTT game.system.id for auto-detection |
| `tooltip_template` | string | Custom Go text/template for tooltips |
| `categories` | array | Reference content types with field schemas |
| `entity_presets` | array | Entity type templates |
| `relation_presets` | array | Relation type templates |

### Category Fields

Each category defines fields that control tooltip rendering and reference page display:

```json
{
  "slug": "spells",
  "name": "Spells",
  "icon": "fa-wand-sparkles",
  "fields": [
    { "key": "level", "label": "Level", "type": "number" },
    { "key": "school", "label": "School", "type": "string" }
  ]
}
```

Valid field types: `string`, `number`, `boolean`, `list`, `markdown`, `enum`, `url`.

### Entity Preset Fields with Foundry Annotations

Character presets can include `foundry_path` for automatic VTT sync:

```json
{
  "slug": "dnd5e-character",
  "foundry_actor_type": "character",
  "fields": [
    {
      "key": "str",
      "label": "Strength",
      "type": "number",
      "foundry_path": "system.abilities.str.value",
      "foundry_writable": true
    }
  ]
}
```

- `foundry_path`: Dot-notation path in Foundry Actor system data
- `foundry_writable`: Whether Chronicle can write this field back (default: true)

### Play Controls (`play`)

An entity preset field may carry an optional `play` object declaring that it can be changed during play, so Chronicle core never needs to know a system's field names. It applies to preset fields only (category fields are reference data) and is copied onto the entity type's stored field definition, the same way `gm_only` is.

```json
{
  "key": "stamina_current",
  "label": "Stamina",
  "type": "number",
  "play": { "edit": "owner", "kind": "counter", "min": 0, "max_field": "stamina_max", "step": 1 }
}
```

| Key | Meaning |
|-----|---------|
| `edit` (required) | Who may change it: `owner` (the claimed owner, or Scribe and above), `gm` (Scribe and above), `none` (nobody through play) |
| `kind` (required) | `counter`, `resource`, `conditions`, `choice` or `text` |
| `min`, `max` | Bounds for `counter` and `resource` |
| `max_field` | Key of another `number` field in the same preset whose value is the upper bound; wins over `max` |
| `step` | Size of one +/- nudge |
| `options` | Allowed values; required and non-empty for `conditions` and `choice` |
| `max_length` | Length bound for `text` and `choice` |
| `to_foundry` | Whether a play edit is written to Foundry; absent means follow the field's `foundry_writable` |
| `combat_authority` | `foundry` or `chronicle`: which side wins during combat; absent leaves it undecided |

A `play` block is dropped at load, and the field stays an ordinary field, when: `edit`, `kind` or `combat_authority` has an unknown value; `max_field` names a missing field, the field itself or a non-number field; `options` is empty for `conditions` or `choice`; `min` is greater than `max`; `step` or `max_length` is negative; or the field is `gm_only` and `edit` is `owner`. The rest of the system still loads. Each drop is logged and listed under the system's validation warnings (the diagnostics page and the install preview).

Declaring `play` changes no behaviour by itself; it is the declaration the play-edit guard and sheet controls read.

### Data Files

Each category has a corresponding `data/<slug>.json` file. Each item is keyed by `id` or `slug` (the loader fills `id` from `slug` when `id` is empty):

```json
[
  {
    "id": "fireball",
    "name": "Fireball",
    "summary": "20-foot radius sphere of flame.",
    "description": "Full markdown description...",
    "properties": { "level": 3, "school": "Evocation" },
    "tags": ["evocation", "damage", "fire"],
    "source": "SRD 5.1"
  }
]
```

### Validation Rules

- `id` in manifest must match directory name
- All categories must have matching data files
- Property keys in data items must match category field definitions
- Entity preset slugs must be unique
- Character preset slug must end with "-character"
- Max 20 categories, 100 fields per category, 10 entity presets, 100 fields per preset

## 3. Installation & Loading Flow

```
                 ┌─────────────┐
                 │  GitHub Repo │
                 │  (releases)  │
                 └──────┬──────┘
                        │ 1. Submit repo URL
                        v
                 ┌─────────────┐
                 │ Admin Review │  2. Approve/reject
                 │   Workflow   │
                 └──────┬──────┘
                        │ 3. Poll GitHub Releases API
                        v
                 ┌─────────────┐
                 │  Download &  │  4. Extract ZIP to
                 │   Extract    │     media/packages/systems/<slug>/<version>/
                 └──────┬──────┘
                        │ 5. Validate manifest
                        v
                 ┌─────────────┐
                 │  System      │  6. LoadAdditionalDir() discovers manifest
                 │  Registry    │     GenericSystem instantiated
                 └──────┬──────┘
                        │ 7. AddonInfos() auto-registers as addon
                        v
                 ┌─────────────┐
                 │  Campaign    │  8. Owner enables addon
                 │  Activation  │
                 └─────────────┘
```

### Key code paths:
- **Submission**: `PackageService.SubmitPackage()` in `internal/plugins/packages/service.go`
- **Review**: `PackageService.ReviewPackage()` — admin approves with notes
- **Version discovery**: `PackageService.CheckForUpdates()` — polls GitHub Releases API
- **Installation**: `PackageService.InstallVersion()` — downloads ZIP, extracts, validates
- **System loading**: `systems.LoadAdditionalDir()` in `internal/systems/registry.go`
- **Addon registration**: `systems.AddonInfos()` returns metadata for all discovered systems

## 4. Per-Campaign Activation

Systems are exposed to campaigns through the addon system:

1. **Auto-registration**: On startup, `AddonInfos()` returns all discovered systems. The addons plugin calls `RegisterSystemAddon()` to ensure each system has an addon database row.
2. **Campaign toggle**: Campaign owners enable/disable system addons via campaign settings.
3. **Middleware gating**: `requireSystemAddon()` middleware checks that the system addon is enabled before serving system routes.
4. **Entity presets**: When enabled, entity type templates from the manifest become available for creating new entities.

## 5. Character Sheet Rendering

Character sheets are rendered using Chronicle's entity system:

1. System manifest declares `entity_presets` with field definitions
2. When a campaign enables the system, entity types are created from presets
3. Fields render on entity pages via existing widgets (attributes, title, tags, etc.)
4. Custom widgets can be declared in the manifest's `widgets` array for richer rendering

### Custom Widgets

System packages can provide custom JS widgets:

```json
"widgets": [
  {
    "slug": "stat-block",
    "name": "Stat Block",
    "script_file": "widgets/stat-block.js",
    "mount": "entity-sidebar"
  }
]
```

`script_file` is the canonical key (`file` is accepted as an alias). Widgets are served from `/campaigns/:id/systems/:mod/widgets/:slug` and auto-mounted by `boot.js` via `data-widget` attributes.

## 6. Card Popup / Stat Block System

Reference items support hover tooltips and card popups:

### Tooltip Rendering

The `GenericTooltipRenderer` in `internal/systems/generic_tooltip.go` produces HTML tooltip fragments using the manifest's category field definitions. No custom Go code is needed.

**How it works:**
1. User hovers over an @mention referencing a system item
2. Handler calls `TooltipRenderer().RenderTooltip(item)`
3. GenericTooltipRenderer reads field definitions from the manifest's category
4. Renders: header (name + category badge), property rows, summary, source

### Custom Tooltip Templates

Systems can override the default tooltip with a Go `text/template` string:

```json
"tooltip_template": "<div class=\"tooltip\">{{.Name}} ({{.Properties.level}})</div>"
```

Systems needing richer, interactive stat-block popups can mount a custom JS widget in the entity sidebar or as an overlay card (see §5 Custom Widgets) rather than relying on the tooltip renderer.

## 7. VTT Sync Integration

The Foundry VTT module uses a single generic adapter for all systems:

### Detection Flow
1. On module load, `SyncManager._detectSystem()` queries `/systems` API
2. Each system's `foundry_system_id` is compared against `game.system.id`
3. Matched system stored in `detectedSystem` setting

### Field Sync Flow
1. `ActorSync._loadAdapter()` creates a generic adapter via `createGenericAdapter()`
2. Generic adapter fetches field definitions from `/systems/:id/character-fields` API
3. Fields with `foundry_path` annotations are auto-mapped
4. `toChronicleFields(actor)`: reads Foundry actor data at each `foundry_path`, maps to Chronicle field keys
5. `fromChronicleFields(entity)`: reads Chronicle field values, writes to Foundry actor at each `foundry_path` (respecting `foundry_writable`)

### Requirements for VTT Sync
- System manifest must include `foundry_system_id`
- Character entity preset must include `foundry_actor_type`
- Character fields must include `foundry_path` annotations
- Fields that are derived in Foundry should set `foundry_writable: false`

## 8. Rules Engine (WASM Extensions)

For systems that need computed fields, validation rules, or dice rolling, WASM extensions provide a sandboxed execution environment.

### Architecture
- `PluginManager` in `internal/extensions/wasm_manager.go` manages WASM lifecycles
- Plugins declare capabilities upfront; host only exposes matching functions
- Memory limits and execution timeouts enforced per plugin

### Capabilities Available
Each capability unlocks a group of host functions:
- `log` — `chronicle_log`
- `entity_read` — `get_entity`, `search_entities`, `list_entity_types`
- `entity_write` — `update_entity_fields`
- `calendar_read` — `get_calendar`, `list_events`
- `calendar_write` — `create_event`
- `tag_read` — `list_tags`, `get_entity_tags`
- `tag_write` — `set_entity_tags`
- `relation_write` — `create_relation`
- `kv_store` — `kv_get`, `kv_set`, `kv_delete` (per-plugin storage)
- `message` — `send_message` (plugin-to-plugin)

### Use Cases
- Auto-calculate derived stats (AC, spell save DC, carry capacity)
- Validate field constraints (ability scores 1-30, level 1-20)
- Dice rolling and random table lookups
- Session event logging

### WASM Plugin Manifest
```json
"contributes": {
  "wasm_plugins": [{
    "slug": "ac-calculator",
    "file": "plugins/ac-calculator.wasm",
    "capabilities": ["entity_read", "log"],
    "memory_limit_mb": 16,
    "timeout_secs": 5
  }]
}
```

## 9. Security Model

### Content Scanning
- ZIP extraction: path traversal prevention, symlink rejection
- File type allowlist (JSON, JS, CSS, SVG, PNG, WASM)
- File size limits per package
- SVG/CSS sanitization

### Manifest Validation
- Required fields enforced
- Slug format validation (lowercase, alphanumeric, hyphens)
- Content limits (max categories, fields, presets)
- HTML sanitization of manifest strings

### WASM Sandboxing
- Capability-based security: plugins declare needed capabilities
- Host only exposes functions matching declared capabilities
- Memory limits and execution timeouts
- No direct filesystem or network access

### Approval Workflow
- `owner_upload_policy`: auto_approve, require_approval, or disabled
- `repository_policy`: github_only, any_git, or allow_all
- Admin review with approve/reject and notes
- Package lifecycle: pending -> approved -> deprecated -> archived

## 10. API Endpoints

### System Reference
| Method | Path | Description |
|--------|------|-------------|
| GET | `/campaigns/:id/systems` | List available systems |
| GET | `/campaigns/:id/systems/:mod` | System reference home |
| GET | `/campaigns/:id/systems/:mod/search` | Full-text search |
| GET | `/campaigns/:id/systems/:mod/:cat` | Category listing |
| GET | `/campaigns/:id/systems/:mod/:cat/:item` | Item detail |
| GET | `/campaigns/:id/systems/:mod/:cat/:item/tooltip` | Tooltip HTML |
| GET | `/campaigns/:id/systems/:mod/widgets/:slug` | Widget JS file |

### System Management (Campaign Owner)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/campaigns/:id/systems/status` | System status |
| POST | `/campaigns/:id/systems/upload` | Upload custom system ZIP |
| POST | `/campaigns/:id/systems/preview` | Preview before install |
| DELETE | `/campaigns/:id/systems/custom` | Remove custom system |

### Character Fields API (for Foundry module)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/systems/:id/character-fields` | Character field definitions with foundry_path |
| GET | `/systems/:id/item-fields` | Item field definitions |

### Package Management (Admin)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/packages` | List all packages |
| POST | `/admin/packages` | Register new repo |
| DELETE | `/admin/packages/:id` | Remove package |
| PUT | `/admin/packages/:id/version` | Install specific version |
| PUT | `/admin/packages/:id/auto-update` | Set update policy |
| POST | `/admin/packages/:id/check` | Check for updates |
| GET | `/admin/packages/pending` | List pending submissions |
| POST | `/admin/packages/:id/review` | Approve/reject submission |

### Owner Submission
| Method | Path | Description |
|--------|------|-------------|
| GET | `/systems/browse` | Browse available systems |
| POST | `/systems/submit` | Submit repo for review |
| GET | `/systems/my-submissions` | View own submissions |

## 11. System Independence

No system-specific Go code lives in Chronicle core. D&D 5e and Draw Steel are
standalone repos; the Foundry module uses the generic adapter exclusively, and
system detection is fully API-driven.

Open work: #740.

### Adding a New Game System

No Chronicle code changes required. Just:

1. Create a GitHub repo with `manifest.json` + `data/*.json`
2. Tag a release (e.g., `v1.0.0`)
3. Submit the repo URL through Chronicle's package submission flow
4. Admin approves, package manager downloads and installs
5. Campaign owners enable the system addon

See `docs/repo-templates/chronicle-systems/.ai/creating-a-system.md` for the step-by-step guide.
