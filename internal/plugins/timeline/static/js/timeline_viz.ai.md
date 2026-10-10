# Timeline Visualization Widget

## Purpose

D3.js-powered interactive SVG timeline: a zoomable spine ruler, event markers
with clustering, range bars, era bands, a mini-map, entity swim-lanes,
search/filter, connections between events, and double-click to create an event
at a date. Source: `internal/plugins/timeline/static/js/timeline_viz.js`, loaded on sight by the timeline plugin's `Widgets` registration (ADR-063).

## Widget Registration

```js
Chronicle.register('timeline-viz', { init, destroy });
```

Mounts on `data-widget="timeline-viz"` (timeline page `timeline.templ`, and the
embed/dashboard blocks in `blocks.templ`). Needs D3 v7; the widget loads
`/static/vendor/d3.min.js` itself when D3 is missing.

## Configuration (data-* attributes)

| Attribute | Used | Description |
|-----------|------|-------------|
| `data-api-url` | Yes (required) | `GET` endpoint returning the timeline data (`/campaigns/:id/timelines/:tid/data`) |
| `data-timeline-color` | Yes | Accent colour for the timeline (default indigo) |
| `data-campaign-id`, `data-timeline-id` | Passed by mounts | Parsed into the config but not read by the widget |
| `data-height`, `data-compact` | Passed by the embed blocks | Parsed into the config but not read by the widget |

## Data

The response holds `timeline`, `groups` (swim-lane membership by `entity_id`),
`eras`, `connections` and `events`. Eras come from `data.eras`; there is no
separate eras endpoint. Events with a missing or NaN `event_year` are dropped;
a missing month or day defaults to 1.

## Architecture

### Rendering pipeline

`_render()` builds the SVG and calls, in order: `_drawEraBands()`, `_drawGrid()`,
`_drawRulerTicks()` (three tiers of ticks on the spine), `_drawEvents()`,
`_drawConnections()`, `_drawMinimap()`. `_onZoom()` redraws the ruler, grid, era
bands, events (when the zoom level changes) and connections.

### Zoom

Six levels: `era`, `century`, `decade`, `year`, `month`, `day`, picked from the
d3-zoom scale factor against `zoomThresholds`. The toolbar has zoom in/out, fit,
a level button per level, a "go to year" box, and the mini-map jumps on click.

### Events

- At `era` and `century` zoom, nearby events collapse into count badges
  (`timeline-cluster-badge` with a `timeline-cluster-label`); click zooms in.
- A range event also draws `timeline-event-range-bar` and
  `timeline-event-range-end`. Events with a category show its icon in place of
  the dot.
- Hover shows a tooltip; click opens the detail panel.
- Swim-lanes group events by entity (`_buildLanes`, `_assignLanes`).
- The toolbar filter box highlights matching events (`_applySearchFilter`).
- Double-clicking empty space fills the date fields of `#standalone-event-modal`
  (when the page has it) and opens it.

## CSS Classes

`.timeline-viz-svg`, `.timeline-era-band` / `.timeline-era-label`,
`.timeline-event` (group) with `.timeline-event-dot`, `.timeline-cluster-badge`,
`.timeline-connection-line`, `.timeline-minimap-viewport`; toolbar and panel use
the `.timeline-viz-*` prefix.

## Dependencies

- D3.js v7
- Timeline plugin provides the data API
