# layout_editor.js -- Unified Layout Editor Widget

## Purpose
Drag-and-drop layout editor used for campaign dashboards, owner dashboards,
and category dashboards (`data-context="dashboard"`). Mounted by the Layout
Studio (`layout_studio.js`) and by `customize.templ` / `entity_type_config.templ`
directly via `data-widget="layout-editor"`. Page templates still use the
separate `template_editor.js` widget (`data-context="template"` features in
this file describe that other widget's config surface, listed here for
comparison, not for this one).

## Mount
```html
<div data-widget="layout-editor"
     data-endpoint="/campaigns/:id/dashboard-layout"
     data-campaign-id="..."
     data-csrf-token="..."
     data-context="dashboard"
     data-features="roles"
     data-layout='{"rows":[...]}'
     data-block-types='[...]'
     data-fields='[...]'>
</div>
```

## Data Attributes
| Attribute | Required | Description |
|-----------|----------|-------------|
| `data-endpoint` | Yes | GET/PUT/DELETE endpoint for layout JSON |
| `data-campaign-id` | Yes | Campaign UUID |
| `data-csrf-token` | Yes | CSRF token |
| `data-context` | Yes | `"dashboard"` or `"template"` -- controls block type filtering |
| `data-features` | No | Comma-separated feature flags (see below) |
| `data-layout` | No | Initial layout JSON; if absent, fetched via GET from endpoint |
| `data-block-types` | No | Override palette block types JSON array |
| `data-fields` | No | Entity type field definitions for preview mockups |
| `data-role` | No | Role for dashboard layouts (default/player/scribe) |

## Feature Flags
| Flag | Dashboard | Template | Description |
|------|-----------|----------|-------------|
| `containers` | - | Yes | Enable container blocks (tabs, sections, 2-col, 3-col) |
| `visibility` | - | Yes | Per-block visibility (everyone/dm_only) |
| `height` | - | Yes | Per-block height presets |
| `presets` | - | Yes | Save/load layout presets |
| `preview` | - | Yes | Right-click block preview mockups |
| `roles` | Yes | - | Role-based layouts (default/player/scribe) |

## Block Types
Block types are fetched from `/campaigns/:id/entity-types/block-types?context=X`
where X is "dashboard" or "template". The API returns `BlockMeta` objects with
`config_fields` arrays that define the config dialog schema.

Falls back to built-in `DEFAULT_BLOCK_TYPES` if the API is unavailable.

## Config Dialog
When a block type has `config_fields`, clicking the gear icon opens a modal
overlay with auto-generated form fields:
- `number`: `<input type="number">` with min/max
- `text`: `<input type="text">`
- `textarea`: `<textarea>`
- `select`: `<select>` with predefined options
- `entity_type`: Dropdown populated from campaign entity types

## Architecture
```
layout_editor.js
├── Constants (COL_PRESETS, HEIGHT_PRESETS, VISIBILITY_OPTIONS, etc.)
├── Widget init (parse config, load block types, load layout)
├── Palette rendering (collapsible sections, addon badges)
├── Canvas rendering (rows, columns, blocks)
├── Block rendering (standard + container)
├── Container rendering (two_column, three_column, tabs, section)
├── Drag-and-drop engine (column drops + sub-zone drops)
├── Config dialog (modal builder from config_fields)
├── Block preview (right-click mockups)
├── Preset system (load/save layout presets)
└── Save/Load (PUT, GET, DELETE)
```

## Events
- Listens for `role-change` custom event (when `roles` feature enabled)
- Keyboard shortcut: Ctrl+S / Cmd+S to save

## Relationship to Other Files
- **layout_studio.js**: Orchestrator that mounts layout-editor with appropriate
  context/features for each navigation selection
- **template_editor.js**: Separate widget for page templates (`data-widget="template-editor"`,
  loaded directly in `base.templ`); layout_editor.js mounts with
  `data-context="dashboard"` (customize and entity-type config pages) or
  `data-context="template"` (mounted by layout_studio.js for page templates)
- **block_registry.go**: Server-side source of truth for block types + config fields
- **block_registry_core.go**: Registers core blocks with contexts + config fields
