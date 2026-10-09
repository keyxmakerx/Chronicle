# Template Editor Widget

## Purpose

Visual drag-and-drop page template editor for entity type layouts. Defines how entity
profile pages are structured: which blocks appear (title, image, rich text, attributes,
tags, relations, etc.), their arrangement in rows and columns, visibility controls
(everyone vs DM-only), and height presets. Used by campaign owners in the Customization
Hub to design per-category page layouts.

## Widget Registration

```js
Chronicle.register('template-editor', { init, destroy, ... });
```

Mounts on: `data-widget="template-editor"`

## Configuration (data-* attributes)

| Attribute | Required | Description |
|-----------|----------|-------------|
| `data-endpoint` | Yes | PUT endpoint the layout is saved to |
| `data-layout` | Yes | Initial layout JSON string (an empty layout becomes the default) |
| `data-campaign-id` | Yes | Campaign id; used for the block-type and preset requests |
| `data-fields` | No | Entity type field definitions JSON string |
| `data-entity-type-name` | No | Display name of the entity type |
| `data-csrf-token` | No | CSRF token for mutations |

## Block Types

The palette comes from `GET /campaigns/:id/entity-types/block-types?context=template`
(server registry, filtered by the campaign's addons; dashboard-only blocks are
excluded). Each entry carries `type`, `label`, `icon`, `description`, `container`,
and optionally `addon` and `widget_slug`. If the request fails, returns nothing,
or there is no campaign id, the editor uses a built-in fallback palette:
title, image, entry (Rich Text), attributes, details, tags, relations, divider,
shop_inventory, posts, text_block, plus the containers below. The fallback has
no calendar block.

Container blocks (`two_column`, `three_column`, `tabs`, `section`) hold
sub-blocks in drop zones; a container never nests inside another container.
Container data:

| Type | Config |
|------|--------|
| `two_column` | `left_width`, `right_width` (out of 12), `left`, `right` block arrays |
| `three_column` | `widths`, `columns` (three block arrays) |
| `tabs` | `tabs`: `[{ label, blocks }]`; the active tab is a transient `_activeTab` |
| `section` | `title`, `collapsed`, `blocks` |

## Layout Presets

### Column Width Presets (for rows)

| Preset | Widths |
|--------|--------|
| 1 Column | [12] |
| 2 Columns | [6, 6] |
| Wide + Sidebar | [8, 4] |
| Sidebar + Wide | [4, 8] |
| 3 Columns | [4, 4, 4] |

### Two-Column Block Presets

| Preset | Left | Right |
|--------|------|-------|
| 50 / 50 | 6 | 6 |
| 33 / 67 | 4 | 8 |
| 67 / 33 | 8 | 4 |

### Height Presets (per block)

| Value | Label | Pixels |
|-------|-------|--------|
| `auto` | Auto | — |
| `sm` | Small | 150px |
| `md` | Medium | 300px |
| `lg` | Large | 500px |
| `xl` | X-Large | 700px |

### Visibility Options (per block)

| Value | Label | Icon |
|-------|-------|------|
| `everyone` | Everyone | fa-globe |
| `dm_only` | DM Only | fa-lock |

## Data Model (JSON)

```json
{
  "rows": [
    {
      "id": "row-a1b2c3",
      "columns": [
        {
          "id": "col-d4e5f6",
          "width": 8,
          "blocks": [
            { "id": "blk-g7h8i9", "type": "title", "config": {} },
            { "id": "blk-j0k1l2", "type": "entry", "config": {} }
          ]
        },
        {
          "id": "col-m3n4o5",
          "width": 4,
          "blocks": [
            { "id": "blk-p6q7r8", "type": "image", "config": {} },
            { "id": "blk-s9t0u1", "type": "attributes", "config": {} }
          ]
        }
      ]
    }
  ]
}
```

### Default Layout

If no layout exists, generates a default: one row with 8/4 split — left column has
title + rich text, right column has image + attributes + details.

### Block Config Schema

Each block has a `config` object with type-specific properties:

- `visibility` — `"everyone"` or `"dm_only"` (all blocks)
- `minHeight` — Height preset value: `"auto"`, `"sm"`, `"md"`, `"lg"`, `"xl"`
- Container blocks keep their sub-blocks inside `config` (see Block Types)
- Keys starting with `_` are transient UI state and are stripped on save (`cleanLayoutForSave`)

## Architecture

### State

- `layout` — Layout object with rows/columns/blocks
- `fields` — Entity type field definitions (passed from server)
- `dirty` — true if unsaved changes
- `dropIndicator` — Currently positioned drop indicator DOM element
- `dropTarget` — Current drop target (column/slot) info

### Key Methods

| Method | Description |
|--------|-------------|
| `init(el)` | Parse data-* attributes, start loading block types, render |
| `_loadBlockTypes()` | Fetch the palette from the API, fall back on failure |
| `defaultLayout()` | Starter layout (8/4 split: title + rich text / image + attributes + details) |
| `render()` / `renderCanvas()` | Full render of palette + canvas / the canvas only |
| `renderBlock(...)` | One block with visibility and height selects, drag handle, delete |
| `renderContainerBlock(...)` | Container block with its config and sub-block drop zones |
| `bindBlockDrag(...)` | Block drag source |
| `handleDrop(...)` / `handleSubBlockDrop(...)` | Drop into a column / into a container slot |
| `addRow` / `changeRowLayout` / `deleteRow` / `moveRow` | Row management with column-width presets |
| `markDirty()` / `bindSave()` / `save()` | Dirty tracking and `PUT` of `{ layout }` to `data-endpoint` |
| `cleanLayoutForSave(layout)` | Drops `_`-prefixed transient keys |
| `showLoadPresetMenu` / `loadPreset` / `saveAsPreset` | Layout presets (below) |

### Drag-and-Drop

Animated drop indicators appear between blocks during drag, showing exactly where a
block will be inserted. The system supports:
- Palette → column/slot (copy new block)
- Block → block position (move within or between columns)
- Block → container slot (nest inside container)

## Server Interaction

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/campaigns/:id/entity-types/block-types?context=template` | Block palette |
| PUT | `data-endpoint` | Save `{ layout }` |
| GET | `/campaigns/:id/layout-presets` | List saved layout presets (Load Preset menu) |
| POST | `/campaigns/:id/layout-presets` | Save the current layout as a preset |

The layout is read from `data-layout` (server-rendered), not fetched. Loading a
preset replaces the canvas after a confirm and still needs a save.

## Dependencies

- `Chronicle.register()` — Widget lifecycle
- `Chronicle.apiFetch()` — Requests with CSRF
- Font Awesome — Block type and control icons
- TailwindCSS — Utility classes

## Known limitations

Container nesting is limited to one level (no containers inside containers).
Tab labels and preset names use `prompt()` dialogs rather than a modal.
