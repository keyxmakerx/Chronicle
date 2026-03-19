# sidebar_reorg.js

## Purpose

Controls the sidebar reorg (reorganization) mode — a toggle that enables
drag-and-drop reordering at two levels: categories (entity type icons)
and entities (within a drilled category).

## Architecture

**IIFE module** — manages a toggle button that switches between category
reorg and entity reorg based on the current drill state.

### Two Levels

1. **Category level** (not drilled in): drag to reorder entity type icons,
   toggle visibility of entire categories. Saves to `PUT /sidebar-config`.
2. **Entity level** (drilled into a category): signals `sidebar_tree.js`
   via `chronicle:reorg-changed` event to enable drag handles and
   visibility toggles on tree nodes.

## State

- `active` — whether reorg mode is on
- `level` — "categories" or "entities"
- `sidebarConfig` — cached sidebar config from API (for category order + hidden IDs)
- `body.sidebar-reorg-active` — CSS class for global state detection

## Key Functions

| Function | Purpose |
|----------|---------|
| `toggle()` | Activate/deactivate reorg mode |
| `activateCategoryReorg()` | Fetch config, render drag handles + eye toggles on category links |
| `activateEntityReorg()` | Dispatch `chronicle:reorg-changed` to sidebar_tree.js |
| `saveSidebarConfig()` | PUT updated config (type order, hidden IDs, entity IDs) |
| `toggleEntityVisibility(entityId)` | Add/remove from hidden_entity_ids |

## Events

| Event | Direction | Purpose |
|-------|-----------|---------|
| `chronicle:toggle-reorg` | Listen | Alternative toggle trigger (from drill panel button) |
| `chronicle:reorg-changed` | Dispatch | Tell sidebar_tree.js to enable/disable D&D |
| `chronicle:toggle-entity-visibility` | Listen | Request from tree to hide/show entity |
| `chronicle:entity-visibility-changed` | Dispatch | Tell tree about new hidden state |
| `chronicle:navigated` | Listen | Exit reorg on navigation |

## Touch Support

Full touch drag-and-drop for mobile category reordering:
- `touchstart` → record start position
- `touchmove` → create ghost element after 10px threshold
- `touchend` → determine drop target, reorder in DOM, save

## MutationObserver

Observes `#sidebar-category` class changes to detect drill-in/drill-out.
On drill-in while reorg is active, transitions from category reorg to
entity reorg automatically.
