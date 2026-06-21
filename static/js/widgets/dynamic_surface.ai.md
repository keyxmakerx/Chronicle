# dynamic_surface.js — the dynamic-surface frame (`Chronicle.surface`)

Wave-1 of the dynamic-surface vision (Cordinator `plans/2026-06-21-dynamic-widget-ui-framework-design.md`).
A frame-owned, system-agnostic toolkit for building dynamic sheets: a motion-preset
library, an overlay stack, an expand/collapse box primitive, a shared data provider,
a mini→full launch, and a schema-driven mount. Vanilla browser JS, loaded after
`boot.js` via `base.templ`. **No node runtime**; verified with esbuild + the browser.

## API — `Chronicle.surface`

| Member | Purpose |
|---|---|
| `play(name, el, opts)` | Run a named transition preset on `el` (returns the WAAPI Animation). |
| `transitions` | The preset map (Systems may read/extend). |
| `overlay.push(content, opts)` / `pop()` / `popAll()` | Stacking dialogs. `content` = HTML string or element. `opts`: `transition` (default `scale-fade`), `origin`, `fromRect`, `panelClass`, `label`, `dismissable`. Escape/backdrop pop; focus trapped in the top layer; prior focus restored. |
| `launch(fromEl, content, opts)` | mini→full: opens `content` as an overlay that grows from `fromEl`'s rect via `container-transform` (full-width panel by default). |
| `box(el)` | Enhance a `[data-box-toggle]`+`[data-box-body]` element into an expand/collapse box. (Also auto-mounted as `data-widget="surface-box"`.) |
| `provider(key, fetcher, opts)` | Shared, memoized fetch keyed by `key`: one `fetcher()` fans to all subscribers. `.subscribe(fn)`, `.push(data)` (live update), `.current()`, `.refresh()`, `.onError(fn)`. `opts.seed` adopts a server payload (zero network). Self-destroys when the last subscriber leaves. |
| `mount(container, schema)` | Build a sheet from a schema (below). Returns the provider. |
| `registerBox(name, fn)` | A **System** supplies a box-body renderer: `fn(boxDef, providerData) → html|element`. The frame mounts; the system renders. |
| `reducedMotion()` / `cssVar(name, fb)` | Helpers. Every preset collapses to a quick fade under `prefers-reduced-motion`. |

## Motion preset menu (pick per card type)
`container-transform` (default open/expand, FLIP) · `scale-fade` (default overlays) ·
`lift` · `slide` (`opts.from`) · `fade` · `flip` · `deal` · `expand` · `none`.
Built on the existing `--ease-*`/`--dur-*`/`--elev-*` tokens + the `--surface-*` contract,
so they're theme-aware. A System names which fits each card; it never writes animation code.

## Mount schema (rides the entity-type layout — no new schema store)
```jsonc
{
  "provider": { "key": "entity:123", "endpoint": "/api/v1/.../entities/123" },
  "rows": [
    { "columns": [
      { "width": 8, "boxes": [
        { "id": "vitals", "title": "Vitals", "block": "ds-vitals", "expand": "expanded",
          "transition": "container-transform",
          "actions": [ { "label": "Roll", "on": "overlay", "endpoint": "/.../roll", "transition": "scale-fade" } ] }
      ]},
      { "width": 4, "boxes": [
        { "id": "inv", "title": "Inventory", "block": "ds-inventory", "expand": "collapsed", "lazy": true, "endpoint": "/.../inventory" }
      ]}
    ]}
  ]
}
```
- A System `registerBox('ds-vitals', fn)`s its box renderers, then the frame mounts the schema
  (via `Chronicle.surface.mount` or `data-widget="dynamic-surface"` reading the schema from a
  `data-surface-schema` attribute / inline `<script type="application/json" data-surface-schema>`).
- `actions[].on`: `overlay` (push `endpoint` HTML / `html`) or `api` (`target` = `"POST /url"`, then `provider.refresh()`).
- Box view-state (expanded/collapsed) persists in localStorage when `id` is set.

## Verification
Browser JS; **esbuild** transform-validated (es2015); the `templ`+`go build` wiring is green.
`app.css`/`*_templ.go` are gitignored, rebuilt at deploy. **Look-and-feel is browser-verified by the
operator** — no node/jsdom test harness (project is node-free apart from build tooling).

## Not yet (next)
A System adopter (the Draw Steel character sheet registering its box renderers + emitting the schema),
and the server-side surface schema authoring (extend `layout_json`). The frame is complete and adopter-ready.
