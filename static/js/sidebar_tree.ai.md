# sidebar_tree.js

## Purpose

Transforms the flat entity list rendered by `SidebarEntityList` into a
collapsible tree, sorted per list, with moves for lists kept by hand. This is
the core interactive component of the sidebar drill panel.

`base.templ` loads it with `sidebar_drill.js`. A category row in the
campaign sidebar (`layouts/nav.templ`, `data-drill-open`) slides the drill
panel open, and the panel's fragment carries the entity list this script
turns into a tree. The row menu (`sidebar_row_menu.js`) acts on the rows
through `Chronicle.sidebarTree`.

## Architecture

**IIFE module**, self-initializing. Listens for HTMX swaps and DOM ready to
call `initTree()`. Exposes `Chronicle.sidebarTree = { rename, refresh, step,
canStep, listOrder, setOrder }`.

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

## Order

Each list sorts by its own order: the category's top level reads
`data-root-order` on `#sidebar-entity-tree`, a folder or page reads
`data-child-order` on its row (copied to its children container as
`data-order`); none means A to Z. `sortChildren(nodes, mode)` always puts
folders first, then: `az` by name, `hand` by `data-sort-order`, `new` by
`data-created`, newest first.

In a list kept by hand, someone who may edit (`data-can-edit`) gets up and
down arrows (`.tree-step`) left of a row's dots, and Alt+Up/Down on a focused
row. `stepRow` moves the neighbour round the row (so focus stays where it
is), turns off arrows at the ends, and saves through
`PUT /sidebar-order/move` naming the neighbour; moves are queued so quick
clicks save in order. A failed save says so and redraws the saved order. A
folder steps among folders and a page among pages; extra listings never move.

Someone who may edit can also drag a row (`draggable` on the row, never on
an extra listing): in a list kept by hand, dropping on the top or bottom half
of a sibling of the same kind places it there (the same save as the arrows);
dropping on the middle of any folder files the row at the end of that folder
(`PUT .../entities/:eid/reorder` or `.../sidebar-nodes/:nid/reorder`) and
redraws. A list kept A to Z or newest first takes only the folder drop. Touch
uses the arrows and the row menu, since a long press opens the menu.

`setOrder(kind, id, mode, then)` saves a list's order and redraws. The
category's picker (`[data-tree-order]`, drawn by the server above the tree
for a Scribe or the owner) and the row menu's "Order inside…" use it.

## Key Functions

| Function | Purpose |
|----------|---------|
| `initTree()` | Builds tree from flat list, sorts each list, renders nodes |
| `renderNode(node, depth, listMode)` | One node: indentation, toggle, count, dots, arrows |
| `sortChildren(nodes, mode)` | Folders first, then the list's order |
| `stepRow(wrapper, dir, focusEl)` | Moves a row one place in a list kept by hand |
| `setOrder(kind, id, mode, then)` | Saves how a list is ordered, then redraws |
| `startInlineRename(link, nodeId, campaignId, entityId)` | Rename in place |
| `refreshSidebarTree()` | Re-fetches entity list via HTMX |
| `observeLoadMore()` | IntersectionObserver for lazy pagination |

## Lazy Loading

Initial load fetches 50 entities. A "Load more" sentinel at the bottom
triggers the next page via IntersectionObserver or manual click. New items
are appended and the tree is re-initialized.

## Persistence

- **Collapse state**: localStorage `chronicle-tree-collapsed-{campaignId}`
- **Order**: on the server, `sidebar_child_orders` and each row's `sort_order`

## Events

| Event | Direction | Purpose |
|-------|-----------|---------|
| `chronicle:sidebar-tree` | Dispatch | The tree has been drawn |
| `sidebar:load-more` | Listen | Trigger next-page fetch |
| `htmx:afterSwap` | Listen | Re-initialize tree after content refresh |

## API Endpoints Used

- `PUT /campaigns/:id/sidebar-order` — a list's order `{kind, id, mode}`
- `PUT /campaigns/:id/sidebar-order/move` — `{kind, id, next_to, after}`
- `PUT /campaigns/:id/entities/:eid/reorder` and
  `PUT /campaigns/:id/sidebar-nodes/:nid/reorder` — a drop onto a folder
- `PUT /campaigns/:id/entities/:eid/metadata` and
  `PUT /campaigns/:id/sidebar-nodes/:nid` — rename
- `GET /campaigns/:id/entities/search?sidebar=1&type=N&page=P` — entity list

## Extra listings (data-place-key rows)

`SidebarEntityList` also draws a page listed under a second parent as a leaf
row keyed `page~parent` (`data-place-key`, `data-place-entity`) with a link
icon and an "also here" label. It has no `data-entity-id`, never moves on its
own and is not counted in its folder's page count. A listing is added from
the page's own "Add a place" picker.
