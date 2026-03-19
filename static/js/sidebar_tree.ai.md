# sidebar_tree.js

## Purpose

Transforms the flat entity list rendered by `SidebarEntityList` into a
collapsible tree with drag-and-drop reordering and reparenting. This is
the core interactive component of the sidebar drill panel.

## Architecture

**IIFE module** — no exports, self-initializing. Listens for HTMX swaps
and DOM ready to call `initTree()`.

### Data Sources

The tree merges two types of items from the server-rendered HTML:
- **Entities** (`data-entity-id`) — navigable `<a>` links to entity pages
- **Sidebar folder nodes** (`data-node-id`, `data-is-folder`) — non-navigable
  `<div>` organizational containers from the `sidebar_nodes` table

### Parent Resolution

An item's parent is determined by checking (in order):
1. `data-parent-node-id` — entity parented under a sidebar folder node
2. `data-parent-id` — entity parented under another entity, or folder
   node parented under another node

This dual-parent model (entity `parent_id` vs `parent_node_id`) is
reflected in the reorder API calls.

## Key Functions

| Function | Purpose |
|----------|---------|
| `initTree()` | Builds tree from flat list, renders nodes, sets up D&D |
| `renderNode(node, depth)` | Renders a single tree node with indentation, icons, toggle |
| `setupDragAndDrop(container, campaignId)` | Attaches drag/drop handlers for reorder + reparent |
| `reorderEntity(campaignId, entityId, parentId, sortOrder, parentNodeId)` | Calls PUT reorder API |
| `showReparentMenu(x, y, droppedId, targetNode, campaignId)` | Context menu for leaf→leaf drops |
| `createGroupFolder(campaignId, droppedId, targetNode, name, isPureFolder)` | Creates folder + reparents both items |
| `refreshSidebarTree()` | Re-fetches entity list via HTMX |
| `observeLoadMore()` | IntersectionObserver for lazy pagination |

## Drag-and-Drop Zones

```
┌─────────────────────────────┐
│      Top 1/3 of Node        │ → REORDER (insert before)
├─────────────────────────────┤
│      Bottom 2/3 of Node     │ → REPARENT (nest inside)
└─────────────────────────────┘
```

- **Drop onto existing folder** (has children or `data-is-folder`): adds as child
- **Drop onto leaf node**: shows reparent menu with two options:
  - "New page as folder" — creates entity via QuickCreateAPI
  - "New empty folder" — creates sidebar node via POST /sidebar-nodes

## Reparent Menu

When a leaf entity is dropped onto another leaf, a floating context menu
appears with two options. The "empty folder" option creates a `sidebar_nodes`
record (no entity), while "page as folder" creates a real entity.

## Lazy Loading

Initial load fetches 50 entities. A "Load more" sentinel at the bottom
triggers the next page via IntersectionObserver or manual click. New items
are appended and the tree is re-initialized.

## Multi-Select (Reorg Mode)

Ctrl/Cmd+click in reorg mode toggles selection on tree nodes. Selected
nodes get `.sidebar-selected` class. A floating action bar appears with
"Move to folder" (calls POST /entities/bulk-move) and "Clear" buttons.

## Persistence

- **Collapse state**: localStorage `chronicle-tree-collapsed-{campaignId}`
- **Reorg mode**: body class `sidebar-reorg-active` (set by sidebar_reorg.js)

## Events

| Event | Direction | Purpose |
|-------|-----------|---------|
| `chronicle:reorg-changed` | Listen | Enable/disable drag handles + visibility toggles |
| `chronicle:entity-visibility-changed` | Listen | Update eye icon on individual entities |
| `sidebar:load-more` | Listen | Trigger next-page fetch |
| `htmx:afterSwap` | Listen | Re-initialize tree after content refresh |

## API Endpoints Used

- `PUT /campaigns/:id/entities/:eid/reorder` — reorder/reparent entity
- `POST /campaigns/:id/entities/quick-create` — create page-as-folder
- `POST /campaigns/:id/sidebar-nodes` — create pure folder
- `PUT /campaigns/:id/sidebar-nodes/:nid/reorder` — reorder/reparent folder
- `POST /campaigns/:id/entities/bulk-move` — multi-select bulk reparent
- `GET /campaigns/:id/entities/search?sidebar=1&type=N&page=P` — entity list
