# Dashboard Editor Widget

## Purpose

Visual drag-and-drop dashboard layout editor for campaigns and per-category pages.
Owners can add rows with configurable column widths, drag blocks from a palette into
columns, reorder rows and blocks, configure block options, save to server, and reset
to defaults. Uses a 12-column CSS grid system with rows → columns → blocks hierarchy.

## Widget Registration

```js
Chronicle.register('dashboard-editor', { init, load, save, render, destroy });
```

Mounts on: `data-widget="dashboard-editor"`

## Configuration (data-* attributes)

| Attribute | Required | Description |
|-----------|----------|-------------|
| `data-endpoint` | Yes | GET/PUT/DELETE endpoint for dashboard layout JSON |
| `data-campaign-id` | Yes | Campaign UUID |
| `data-csrf-token` | Yes | CSRF token for mutations |
| `data-block-types` | No | JSON array of block type objects to override default palette |

## Block Types

8 default block types (overridable via `data-block-types` for category dashboards):

| Type | Label | Icon | Description |
|------|-------|------|-------------|
| `welcome_banner` | Welcome Banner | fa-flag | Campaign name & description |
| `category_grid` | Category Grid | fa-grid-2 | Quick-nav entity type grid |
| `recent_pages` | Recent Pages | fa-clock | Recently updated entities |
| `entity_list` | Entity List | fa-list | Filtered list by category |
| `text_block` | Text Block | fa-align-left | Custom rich text / HTML |
| `pinned_pages` | Pinned Pages | fa-thumbtack | Hand-picked entity cards |
| `calendar_preview` | Calendar | fa-calendar-days | Upcoming calendar events |
| `timeline_preview` | Timeline | fa-timeline | Timeline list with event counts |

### Block Configuration Options

- `recent_pages` — `limit` (4-12, default 8)
- `category_grid` — `columns` (2-6, default 4)
- `text_block` — `content` (HTML string)
- `entity_list` — `entity_type_id`, `limit` (4-20, default 8)
- `entity_grid` — `columns` (2-6, default 4)
- `calendar_preview` — `limit` (1-20, default 5)
- `timeline_preview` — `limit` (1-20, default 5)

## Column Layout Presets

5 presets for adding new rows:

| Preset | Column Widths |
|--------|---------------|
| Full Width | [12] |
| 2 Equal Columns | [6, 6] |
| Wide + Sidebar | [8, 4] |
| Sidebar + Wide | [4, 8] |
| 3 Equal Columns | [4, 4, 4] |

## Data Model (JSON)

```json
{
  "rows": [
    {
      "id": "db_a1b2c3d4",
      "columns": [
        {
          "id": "db_e5f6g7h8",
          "width": 6,
          "blocks": [
            { "id": "db_i9j0k1l2", "type": "welcome_banner", "config": {} }
          ]
        }
      ]
    }
  ]
}
```

- `layout = null` means the default server-side layout is in use
- IDs are generated client-side with `db_` prefix + 8 random alphanumeric chars

## Architecture

### State

- `layout` — null (default) or layout object with rows/columns/blocks
- `dirty` — true if unsaved changes exist
- `dragState` — `{ type: 'palette', blockType }` or `{ type: 'move', rowIdx, colIdx, blockIdx }`
- `blockTypes` — Array of block type definitions (default or custom)

### Key Methods

| Method | Description |
|--------|-------------|
| `init(el, config)` | Parse config from data-* attributes, load layout |
| `load()` | GET layout JSON from endpoint |
| `save(callback)` | PUT layout JSON to endpoint (skips if null/default) |
| `resetLayout()` | DELETE custom layout, revert to server defaults |
| `ensureLayout()` | Create empty `{ rows: [] }` if layout is null |
| `render()` | Full re-render: toolbar + palette + canvas |
| `renderRow(row, rowIdx)` | Single row with column grid and controls |
| `renderBlock(block, rowIdx, colIdx, blockIdx)` | Single block with config/delete buttons |
| `bindEvents()` | Attach all event listeners after render |
| `addRow(widths)` | Append new row with given column widths |
| `deleteRow(rowIdx)` | Remove row by index |
| `moveRow(rowIdx, direction)` | Swap row up (-1) or down (+1) |
| `addBlock(rowIdx, colIdx, blockType)` | Append block to column |
| `deleteBlock(rowIdx, colIdx, blockIdx)` | Remove block from column |
| `moveBlock(from, to)` | Move block between columns |
| `configBlock(rowIdx, colIdx, blockIdx)` | Prompt-based config per block type |
| `clearDropHighlights()` | Remove visual drag-over feedback |
| `destroy()` | Clear innerHTML |

### Drag-and-Drop

Two drag operations:
1. **Palette → Column** (copy): Drag a block type from the left palette into a column
2. **Block → Column** (move): Drag an existing block to a different column

Drop targets are columns (`.dash-col`), highlighted with accent border on dragover.

## Server Interaction

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `data-endpoint` | Load existing layout (returns JSON or null) |
| PUT | `data-endpoint` | Save layout (JSON body, X-CSRF-Token header) |
| DELETE | `data-endpoint` | Reset to default layout |

## Dependencies

- `Chronicle.register()` — Widget lifecycle
- `Chronicle.notify()` — Toast notifications
- Fetch API — Server communication
- Font Awesome — Block type icons
- TailwindCSS — Utility classes for layout

## CSS Classes

- `.palette-block` — Draggable block in the left palette
- `.dash-row` — Row container on the canvas
- `.dash-col` — Column drop target within a row
- `.dash-block` — Individual block within a column
- `.btn-save` / `.btn-reset` — Toolbar action buttons
- `.add-row-btn` — Row preset buttons in palette
- `.move-row-btn` / `.delete-row-btn` — Row management controls
- `.config-block-btn` / `.delete-block-btn` — Block management controls

## Known Limitations & Future Work

- Block configuration uses `prompt()` dialogs — should be replaced with proper modals
- No drag-to-reorder within the same column (only move between columns)
- No undo/redo for layout changes
- No preview of how dashboard will look with real data
