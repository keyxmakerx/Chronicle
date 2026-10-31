# System Plugin Marketplace - Architecture & Specification

## 1. Summary

Game systems are developed, submitted, reviewed, installed and activated per
campaign independently of Chronicle core. Core provides a generic runtime
(system loader, tooltip renderer, entity presets, Foundry sync); all
system-specific behavior lives in the package, driven by its manifest. No
custom Go code is needed for any system.

Packages are GitHub repositories with tagged releases. Admins install them via
the package manager, campaign owners enable them as addons, and the Foundry
module detects the active system and syncs characters from the manifest's field
annotations.

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
| `api_version` | string | Must be "1" |

**Optional fields:**
| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Leave it out: the package manager sets it from the GitHub release tag on install (a manifest without one loads as "0.0.0") |
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
| `widgets` | array | JS widgets the package ships (`slug`, `name`, `script_file`) |
| `text_renderers` | array | JS text renderers the package ships |
| `renderers` | array | Binds one of this manifest's widgets to one of its entity presets (see `docs/system-package-rendering.md`) |
| `dm_screen` | object | Which character fields the DM Screen shows for each hero |
| `entity_panels` | array | Extra panels one of this manifest's widgets adds to NPC pages |
| `director_data` | array | Data file names with no category (for example `"role-templates.json"`) that the raw data route keeps to the campaign's Directors |

### Category Fields

Category fields control tooltip rendering and reference page display:

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

A category with `"director": true` is Director-only: its reference pages, search hits, tooltips and raw `data/<slug>.json` answer 404 below Director. Use it for content the system's book keeps in a Director-only chapter.

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
- `format` (optional, any preset field): how the value is written when it is not plain text or a number, such as the shape of one entry in a JSON list. AI Import's brief shows it beside the field

### Play Controls (`play`)

An entity preset field may carry a `play` object declaring it can be changed during play, so core never needs a system's field names. It applies to preset fields only (category fields are reference data) and is copied onto the entity type's stored field definition, like `gm_only`.

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
| `options` | Allowed values, at most 50, each 1 to 64 characters; required and non-empty for `conditions` and `choice` |
| `max_length` | Length bound for `text` and `choice` |
| `to_foundry` | Whether a play edit is written to Foundry; absent means follow the field's `foundry_writable` |
| `combat_authority` | `foundry` or `chronicle`: which side wins during combat; absent leaves it undecided |

A `play` block is dropped at load, leaving an ordinary field, when: `edit`, `kind` or `combat_authority` is unknown; `max_field` names a missing field, the field itself or a non-number field; `options` is empty for `conditions`/`choice`, has over 50 entries, or holds an empty or over-64-character value; `min` > `max`; `step` or `max_length` is negative; or the field is `gm_only` with `edit: owner`. The rest of the system still loads. Each drop is logged and listed in the system's validation warnings (diagnostics page, install preview).

Entity types do not record their source system, so when two installed systems declare the same category and field key with different `play` blocks, neither is copied onto existing types.

`play` is only a declaration; the play-edit guard and sheet controls read it.

### Crafting (`crafting`)

An optional top-level block saying how the game rolls and counts a crafting project in the Crafting app. Recipes, benches and steps are each campaign's own and work under any system; this is only the arithmetic and the words.

```json
"crafting": {
  "dice": "2d10",
  "characteristics": [{"key": "might", "label": "Might"}, {"key": "reason", "label": "Reason"}],
  "edge": 2,
  "bane": -2,
  "tiers": [11, 16],
  "labels": {"goal": "Project goal", "prerequisite": "Item prerequisite", "source": "Project source"}
}
```

| Key | Meaning |
|-----|---------|
| `dice` | The project roll: one to four d4, d6, d8, d10, d12 or d20, like `2d10` |
| `characteristics` | Up to 12 sheet fields a recipe may add to the roll, each a field `key` and the `label` the editor shows. None means the Director writes one in |
| `edge`, `bane` | What clean and botched work add to the roll, -10 to 10 |
| `tiers` | The highest totals of the first and second outcome of a tiered test such as harvesting, rising: `[11, 16]` makes 11 or less the first tier, 12 to 16 the second, 17 or more the third |
| `labels` | The game's words for a project's `goal`, the item it needs first (`prerequisite`) and where the knowledge comes from (`source`), each up to 40 plain characters |

Every key is optional; a missing one, or a system with no block, takes the default (2d10, no characteristic, +2 and -2, `[11, 16]`, "Goal", "Item prerequisite", "Project source"). A block that breaks these rules fails manifest validation, so the system does not load.

### Data Files

Each category has a `data/<slug>.json` file. Items are keyed by `id` or `slug` (the loader fills `id` from `slug`):

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

`ValidateManifest` in `internal/systems/manifest.go`:

- `id`, `name` and `api_version` are required. `id`, category, preset, widget and text-renderer slugs match `^[a-z0-9][a-z0-9_-]*$`. `status` is `available` or `coming_soon` (default `coming_soon`).
- Limits: 20 categories, 100 fields per category, 10 entity presets, 100 fields per preset, 20 relation presets, 10 widgets, 5 text renderers, 10 renderers, 10 entity panels.
- `script_file` and text-renderer `file` end in `.js` and contain no path traversal. Field types are limited to the list above.
- Warnings, not errors: no categories, no entity presets, no preset slug ending in `-character` (Foundry character sync won't work), no `foundry_system_id`, no field with `foundry_path`.

## 3. Installation & Loading Flow

1. Submit a GitHub repo URL: `PackageService.SubmitPackage()` in `internal/plugins/packages/service.go`.
2. Admin approves or rejects with notes: `ReviewPackage()`.
3. `CheckForUpdates()` polls the GitHub Releases API.
4. `InstallVersion()` downloads the ZIP, extracts it to `media/packages/systems/<slug>/<version>/` and validates the manifest.
5. `systems.LoadAdditionalDir()` (`internal/systems/registry.go`) discovers the manifest and instantiates a `GenericSystem`.
6. `systems.AddonInfos()` returns every discovered system so it registers as an addon.
7. A campaign owner enables the addon.

## 4. Per-Campaign Activation

Systems are exposed to campaigns through the addon system:

1. **Auto-registration**: at startup the addons plugin calls `RegisterSystemAddon()` for each system from `AddonInfos()`, so each has an addon row.
2. **Campaign toggle**: Campaign owners enable/disable system addons via campaign settings.
3. **Middleware gating**: `requireSystemAddon()` middleware checks that the system addon is enabled before serving system routes.
4. **Entity presets**: once enabled, the manifest's entity type templates are available for new entities.

## 5. Character Sheet Rendering

Character sheets use the entity system:

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

`GenericTooltipRenderer` (`internal/systems/generic_tooltip.go`) renders tooltip HTML from the manifest's category field definitions.

1. User hovers over an @mention referencing a system item
2. Handler calls `TooltipRenderer().RenderTooltip(item)`
3. GenericTooltipRenderer reads field definitions from the manifest's category
4. Renders: header (name + category badge), property rows, summary, source

### Custom Tooltip Templates

Systems can override the default tooltip with a Go `text/template` string:

```json
"tooltip_template": "<div class=\"tooltip\">{{.Name}} ({{.Properties.level}})</div>"
```

For interactive stat-block popups, mount a custom widget in the entity sidebar or as an overlay card (§5) instead.

## 7. VTT Sync Integration

The Foundry module uses one generic adapter for all systems.

### Detection Flow
1. On module load, `SyncManager._detectSystem()` queries `/systems` API
2. Each system's `foundry_system_id` is compared against `game.system.id`
3. Matched system stored in `detectedSystem` setting

### Field Sync Flow
1. `ActorSync._loadAdapter()` creates a generic adapter via `createGenericAdapter()`
2. The adapter fetches field definitions from the `character-fields` endpoint (§10)
3. Fields with `foundry_path` annotations are auto-mapped
4. `toChronicleFields(actor)` reads actor data at each `foundry_path` into Chronicle field keys.
5. `fromChronicleFields(entity)` writes Chronicle values to the actor at each `foundry_path`, respecting `foundry_writable`.

### Requirements for VTT Sync
- System manifest must include `foundry_system_id`
- Character entity preset must include `foundry_actor_type`
- Character fields must include `foundry_path` annotations
- Fields that are derived in Foundry should set `foundry_writable: false`

## 8. Rules Engine (WASM Extensions)

WASM extensions give a sandboxed environment for computed fields, validation and dice rolling.

### Architecture
- `PluginManager` (`internal/extensions/wasm_manager.go`) manages WASM lifecycles.
- Plugins declare capabilities upfront; the host exposes only matching functions.
- Memory limits and execution timeouts are enforced per plugin.

### Capabilities
Each unlocks a group of host functions:
- `log` — `chronicle_log`
- `entity_read` — `get_entity`, `search_entities`, `list_entity_types`
- `entity_write` — `update_entity_fields`
- `calendar_read` — `get_calendar`, `list_events`
- `calendar_write` — `create_event` (the calendar functions always error until the rebuild lands, #778)
- `tag_read` — `list_tags`
- `tag_write` — `set_entity_tags`, `get_entity_tags`
- `relation_write` — `create_relation`
- `kv_store` — `kv_get`, `kv_set`, `kv_delete` (per-plugin storage)
- `message` — `send_message` (plugin-to-plugin)

### WASM plugin manifest
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
- ZIP extraction rejects path traversal and symlinks.
- File type allowlist: JSON, JS, CSS, SVG, PNG, WASM.
- Per-package file size limits; SVG and CSS are sanitized.

### Manifest Validation
- Required fields, slug format and content limits (see Validation Rules).
- Manifest strings are HTML-sanitized.

### WASM Sandboxing
- Host functions are limited to the declared capabilities.
- Memory limits and execution timeouts.
- No direct filesystem or network access.

### Approval Workflow
- `owner_upload_policy`: auto_approve, require_approval, or disabled
- `repository_policy`: github_only, any_git, or allow_all
- Admin review with approve/reject and notes
- Package lifecycle: `pending`, `approved`, `rejected`, `deprecated`, `archived`

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
| GET | `/campaigns/:id/systems/:mod/rules-glossary` | Raw rules glossary, for client-side reference renderers |
| GET | `/campaigns/:id/systems/:mod/data/:file` | A bundled `data/<file>.json`, served verbatim for widgets |

### Rulebook book
Every route sits under `/campaigns/:id/systems/:mod`, behind the same campaign
access and system addon checks as the reference routes. Reads are sliced to the
viewer's own book; every editing route checks the editor rule itself (campaign
owner or co-Director). Format: `docs/system-rulebook-book.md`.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/book` | The book, filtered for this viewer |
| GET | `/book/index/:cat`, `/book/find` | Rules-index pages and book search |
| GET | `/book/edit` | The book editor page (editors only) |
| GET | `/book/source` | The editor's starting state: the campaign's edition with each page's state (editors only) |
| GET | `/book/export` | Download the campaign's edition in the package's own book format (editors only) |
| POST, PUT, DELETE | `/book/chapters[/:chapter]` | Add, change or remove a chapter (editors only) |
| POST, PUT, DELETE | `/book/chapters/:chapter/pages[/:key]` | Add, save or remove a page (editors only) |
| POST | `/book/chapters/:chapter/pages/:key/keep` | Keep the campaign's copy of a page the package changed after it was edited (editors only) |

### System Management (Campaign Owner)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/campaigns/:id/systems/status` | System status |
| POST | `/campaigns/:id/systems/upload` | Upload custom system ZIP |
| POST | `/campaigns/:id/systems/preview` | Preview before install |
| DELETE | `/campaigns/:id/systems/custom` | Remove custom system |

### Character Fields API (Foundry module)
Registered in `internal/plugins/syncapi/routes.go` under `/api/v1/campaigns/:id`, with read permission. Paths below are relative to that.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/systems/:systemId/character-fields` | Character field definitions with foundry_path |
| GET | `/systems/:systemId/item-fields` | Item field definitions |
| GET | `/systems/:systemId/creature-fields` | Creature field definitions, with the fields Foundry may send back |

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

Other `/admin/packages` routes (versions, pin, retention, usage, per-campaign hold/move, prune, deprecate, archive, settings) are in `internal/plugins/packages/routes.go`.

### Owner Submission
| Method | Path | Description |
|--------|------|-------------|
| GET | `/systems/browse` | Browse available systems |
| POST | `/systems/submit` | Submit repo for review |
| GET | `/systems/my-submissions` | View own submissions |

## 11. System Independence

No system-specific Go code lives in core. D&D 5e and Draw Steel are standalone
repos; the Foundry module uses the generic adapter only, and system detection is
API-driven. Open work: #740.

### Adding a New Game System

No core changes required:

1. Create a GitHub repo with `manifest.json` + `data/*.json`
2. Tag a release (e.g., `v1.0.0`)
3. Submit the repo URL through Chronicle's package submission flow
4. Admin approves, package manager downloads and installs
5. Campaign owners enable the system addon

See `docs/repo-templates/chronicle-systems/.ai/creating-a-system.md` for the step-by-step guide.
