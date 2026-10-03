# relation_graph.js

## Purpose

D3 force-directed graph of entity relationships in a campaign: nodes are
entities (coloured by entity type), edges are relations including @mentions.
Used full-page (with filters) and as small embeds, including a local
"ego graph" centred on one entity.

## Mount

`data-widget="relation-graph"`, loaded by a `<script defer>` tag in
`layouts/base.templ`. Mounted from `widgets/relations/graph.templ` (full page,
filters on), `plugins/entities/show.templ` (`blockLocalGraph`, focus mode) and
`plugins/campaigns/dashboard_blocks.templ` (two dashboard blocks).

| Attribute | Meaning |
|---|---|
| `data-campaign-id` | Campaign ID; used for node links |
| `data-api-url` | Graph endpoint, `/campaigns/:id/relations-graph` |
| `data-height` | Pixel height, default 500 |
| `data-entity-types` | JSON array (`slug`, `name`, `icon`, `color`) for the Types filter |
| `data-show-filters` | `"true"` renders the filter toolbar |
| `data-focus-entity`, `data-hops` | Ego-graph mode; hops default 2 |

## Data fetched

`GET <api-url>` with query params `types`, `search`, `include_mentions=false`,
`include_orphans=true`, `focus`, `hops`. The route and its `/page` sibling are
in `routes_snapshot.txt`. The response is `{nodes, edges}`. Filter changes
refetch (search is debounced 300 ms).

## D3 loading

D3 is not in `base.templ`; the widget injects `/static/vendor/d3.min.js` on
first mount and polls for it if another instance is already loading it. The
file header still says "loads from CDN"; the code loads the vendored copy.

## Behaviour

- Clicking a node navigates to `/campaigns/:id/entities/:nodeId`.
- Zoom in/out/reset buttons and PNG export (canvas) are built into the SVG
  overlay; export failure shows a `Chronicle.notify` error toast.
- A gentle per-type cluster force groups nodes of the same type.

## Events

None emitted or listened to; only DOM and `Chronicle.notify` on load errors.

## Gotchas

- Marker ids `arrowhead` and `arrowhead-mention` are global SVG ids; two
  graphs on one page share them.
- The filter dropdown registers a document click handler stored on the
  element; `destroy()` removes it.
- Filter reloads rebuild the toolbar from its saved HTML, so listeners are
  re-attached after each load.
