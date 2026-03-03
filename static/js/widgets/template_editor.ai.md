# Template Editor Widget

## Purpose

Visual drag-and-drop page template editor for entity type layouts. Defines how entity
profile pages are structured: which blocks appear (title, image, rich text, attributes,
tags, relations, etc.), their arrangement in rows and columns, visibility controls
(everyone vs DM-only), and height presets. Used by campaign owners in the Customization
Hub to design per-category page layouts.

## Widget Registration

```js
Chronicle.register('template-editor', { init, render, bindSave, destroy });
```

Mounts on: `data-widget="template-editor"`

## Configuration (data-* attributes)

| Attribute | Required | Description |
|-----------|----------|-------------|
| `data-endpoint` | Yes | GET/PUT endpoint for layout JSON |
| `data-layout` | Yes | Initial layout JSON string |
| `data-fields` | Yes | Entity type field definitions JSON string |
| `data-entity-type-name` | Yes | Display name of the entity type |
| `data-csrf-token` | Yes | CSRF token for mutations |

## Block Types

13 block types, including 4 container types that hold sub-blocks:

### Content Blocks

| Type | Label | Icon | Description |
|------|-------|------|-------------|
| `title` | Title | fa-heading | Entity name and actions |
| `image` | Image | fa-image | Header image with upload |
| `entry` | Rich Text | fa-align-left | Main content editor (TipTap) |
| `attributes` | Attributes | fa-list | Custom field values |
| `details` | Details | fa-info-circle | Metadata and dates |
| `tags` | Tags | fa-tags | Tag picker widget |
| `relations` | Relations | fa-link | Entity relation links |
| `divider` | Divider | fa-minus | Horizontal separator |
| `calendar` | Calendar | fa-calendar-days | Entity calendar events |

### Container Blocks

| Type | Label | Icon | Description |
|------|-------|------|-------------|
| `two_column` | 2 Columns | fa-columns | Side-by-side columns with width presets |
| `three_column` | 3 Columns | fa-table-columns | Three equal columns |
| `tabs` | Tabs | fa-folder | Tabbed content sections |
| `section` | Section | fa-caret-down | Collapsible accordion |

Container blocks (`two_column`, `three_column`, `tabs`, `section`) can hold other
blocks inside their slots. They are rendered with sub-block drop zones.

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
- Container blocks: `slots` array with sub-block arrays
- `tabs`: `config.tabs` array with `{ label, blocks }` entries
- `two_column`: `config.left` (width), sub-blocks in slots

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
| `init(el)` | Parse data-* attributes, validate JSON, render |
| `defaultLayout()` | Generate starter layout (8/4 title+text / image+attrs) |
| `render()` | Full re-render of palette + canvas |
| `renderRow(row, ri)` | Single row with columns and controls |
| `renderBlock(block, path)` | Block with settings panel, drag handle, sub-blocks |
| `renderContainerSlots(block, path)` | Drop zones for container sub-blocks |
| `renderSettingsPanel(block, path)` | Visibility, height, type-specific config |
| `bindDrag()` | Animated drop indicators between blocks |
| `bindSave()` | Attach save button click handler |
| `save()` | PUT layout to endpoint |
| `addRow(widths)` | Add new row with column presets |
| `deleteRow(ri)` | Remove row |
| `moveRow(ri, dir)` | Reorder row up/down |
| `addBlock(path, type)` | Add block at path (row/col or container slot) |
| `deleteBlock(path)` | Remove block at path |
| `moveBlock(from, to)` | Drag block between positions |
| `uid(prefix)` | Generate unique ID with prefix |
| `destroy()` | Clear innerHTML |

### Drag-and-Drop

Animated drop indicators appear between blocks during drag, showing exactly where a
block will be inserted. The system supports:
- Palette → column/slot (copy new block)
- Block → block position (move within or between columns)
- Block → container slot (nest inside container)

## Server Interaction

| Method | Endpoint | Description |
|--------|----------|-------------|
| PUT | `data-endpoint` | Save layout JSON (X-CSRF-Token header) |

Layout is loaded from `data-layout` attribute (server-rendered), not fetched via GET.

## Dependencies

- `Chronicle.register()` — Widget lifecycle
- `Chronicle.notify()` — Toast notifications
- Fetch API — Save to server
- Font Awesome — Block type and control icons
- TailwindCSS — Utility classes

## Known Limitations & Future Work

- No live preview of how the template will look with real entity data
- No undo/redo for layout changes
- Tab configuration uses prompt() dialogs
- No drag handle visual affordance on container sub-blocks
- Container nesting is limited to one level (no containers inside containers)
