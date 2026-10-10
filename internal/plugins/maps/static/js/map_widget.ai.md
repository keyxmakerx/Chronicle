# map_widget.js

## Purpose

Compact, read-only Leaflet map embed for dashboards, category dashboards and
entity pages. Shows the map image with its markers (and optionally drawings
and tokens); full editing stays on the dedicated map page. Each instance
fetches its own data; nothing is shared between instances.

## Mount

`data-widget="map-widget"` (registered as `map-widget`). Source:
`internal/plugins/maps/static/js/map_widget.js`, loaded on sight after
`map_annotations.js` by the maps plugin's `Widgets` registration (ADR-063).
Mounted from
`plugins/maps/blocks.templ`, `plugins/campaigns/dashboard_blocks.templ`
(two places) and `plugins/entities/category_blocks.templ`.

| Attribute | Meaning |
|---|---|
| `data-campaign-id` | Campaign ID (required) |
| `data-map-id` | Map to show; empty shows a static "Configure a map" prompt |
| `data-height` | Pixel height, default 250 |
| `data-show-drawings`, `data-show-tokens` | `"true"` to also load drawings / tokens |

## Data fetched

All paths are relative to `/campaigns/:id`, via `Chronicle.apiFetch`:

- `GET /maps/:mid/meta` — image id, dimensions and visibility-filtered
  markers. The web route is used (not `/api/v1`) because it works for public
  campaigns.
- `GET /maps/:mid/drawings`, `GET /maps/:mid/tokens` — only when the matching
  `data-show-*` flag is set; failures are swallowed.
- `GET /maps` — only in the no-map-id case; the response body is discarded and
  the widget shows a link to the maps page.
- Image from `/media/:image_id`.

All of these exist in `internal/wire/routes_snapshot.txt`.

## Leaflet loading

If `L` is undefined the widget injects `/static/vendor/leaflet.css`,
`MarkerCluster.css`, `leaflet.js` and `leaflet.markercluster.js` itself;
`base.templ` does not load them. Markers cluster only above 5 markers.

## Events

None emitted or listened to.

## Gotchas

- Marker, drawing and token positions are percentages (0-100) from the
  top-left; the widget flips Y for Leaflet's bottom-left `CRS.Simple`.
- Tokens flagged `is_hidden` are skipped client-side as well as server-side.
- Ellipse drawings are approximated as a circle sized from the x-extent.
- `destroy()` removes the Leaflet map.
- The map-picker branch is a placeholder: it does not list maps.
