# Timeline Visualization Widget

## Purpose

D3.js-powered interactive SVG timeline chart for visualizing events across time.
Supports zoom/pan, event clustering at high zoom, era bars, range events, category
icons, detail panel, and a minimap overview strip.

## Widget Registration

```js
Chronicle.register('timeline-viz', { init, destroy });
```

Mounts on: `data-widget="timeline-viz"`

## Configuration (data-* attributes)

| Attribute | Required | Description |
|-----------|----------|-------------|
| `data-events-endpoint` | Yes | JSON endpoint returning timeline events |
| `data-eras-endpoint` | No | JSON endpoint returning calendar eras |
| `data-editable` | No | "true" enables edit controls |
| `data-csrf-token` | No | CSRF token for mutations |

## Architecture

### Rendering Pipeline

1. `_render()` — Main entry: creates SVG, sets up scales, calls sub-renderers
2. `_drawGrid()` — Alternating column bands + major/minor grid lines
3. `_drawRuler()` — Center spine horizontal ruler with 3-tier ticks
4. `_drawEraBands()` — Compact 16px bars at top of SVG with truncated labels
5. `_drawEvents()` — Event dots/icons positioned on timeline, click for detail
6. `_drawRangeEvents()` — Horizontal colored bars for multi-day events
7. `_drawMinimap()` — 36px overview strip below SVG with viewport indicator

### Zoom System

- 5 zoom levels: `era`, `century`, `decade`, `year`, `month`
- Zoom buttons (+/-) in toolbar
- Click-to-jump on minimap
- At era/century zoom: events cluster into count badges

### Event Clustering

At high zoom levels (`era`, `century`), overlapping events collapse into circular
count badges showing the number of events in that region. Click to zoom in.

### Category Icons

Events with a category display the category's Font Awesome icon instead of a
plain dot. Icon mapping from event's `category` field.

### Detail Panel

Clicking an event opens a slide-in panel showing: title, date, category, linked
entity, description. Edit/delete buttons for Scribe+ role.

## CSS Classes

- `.timeline-viz-svg` — Main SVG container
- `.timeline-era-band` — Era background bar (opacity 0.25, rounded corners)
- `.timeline-era-label` — Era text label (monospace, 10px)
- `.timeline-event-dot` — Individual event marker
- `.timeline-range-bar` — Range event horizontal bar
- `.timeline-cluster` — Clustered event count badge

## Dependencies

- D3.js (loaded via vendored `d3.min.js` or CDN)
- Timeline plugin provides data API
- Calendar plugin provides era data (optional)
