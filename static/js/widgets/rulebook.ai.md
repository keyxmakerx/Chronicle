# Rulebook Widget

## Purpose

The **Rulebook** flagship — the dynamic-surface paradigm (`Chronicle.surface`)
pointed at reference content. It turns a game system's authored
`data/rules-glossary.json` into an interactive browser: a category nav beside a
column of **expanding rule boxes**, with client-side **search** and stackable
**`{@term}` cross-reference overlays** (read a referenced rule without losing
your place — Back/Escape pops the stack).

It is a thin **adopter** of the shared surface frame
(`dynamic_surface.js`): the frame owns box chrome, motion and the overlay
stack; this widget only supplies the data, the rule-body renderer, and the
nav/search shell.

## Widget Registration

```js
Chronicle.register('rulebook', { init, destroy });
```

Mounts on: `data-widget="rulebook"`. Auto-mounted by `boot.js`. The systems
reference index (`internal/systems/system_pages.templ` → `SystemIndexContent`)
emits the mount div above the category grid.

## Configuration

| Attribute | On | Description |
|-----------|-----|-------------|
| `data-widget="rulebook"` | Mount div | Registers with boot.js lifecycle |
| `data-campaign-id` | Mount div | Campaign ID (for the glossary fetch URL) |
| `data-mod` | Mount div | System slug — the `:mod` route param (`manifest.ID`) |

## How it works

1. `init` reads `data-campaign-id` + `data-mod`, then fetches
   `GET /campaigns/:id/systems/:mod/rules-glossary` (the existing
   `SystemHandler.RulesGlossaryAPI`, served raw from the system's
   `data/rules-glossary.json`).
2. **Degrades invisibly:** if the system ships no glossary (empty `[]`), the
   mount div is left empty — only the category grid shows. So adding the mount
   to every system index is safe.
3. Entries are grouped by `properties.category` (friendly labels + a stable
   order for the known rule categories; unknown categories title-cased and
   appended). Each rule becomes a collapsed surface box (`block: rulebook-rule`,
   `transition: expand`); the per-rule data rides on the box def (`def.rule`),
   so no provider/refetch is needed.
4. **Search** (debounced) matches name + description across *all* categories and
   re-mounts the surface with the flat result set; clearing it (or clicking a
   category) returns to the active-category view.
5. **`{@category term|display}`** tokens in descriptions render as `.rb-ref`
   spans; clicking one pushes the referenced rule as a `deal`-transition
   overlay. The same handler is attached to each overlay panel, so nested refs
   stack.

State lives on `root._rb` (not on the registered object), so multiple mounts on
a page are independent; `destroy` tears down the surface, listeners and timer.

## Read-only

Serves reference content only — never mutates campaign data (Chronicle systems
are read-only).

## Dependencies

- `Chronicle.surface` (`dynamic_surface.js`) — `mount`, `registerBox`,
  `overlay.push`. Loaded before this widget in `base.templ`.
- `Chronicle.apiFetch`, `Chronicle.escapeHtml` (`boot.js`).

## Related

- Design: Cordinator `plans/2026-06-21-dynamic-widget-ui-framework-design.md` §5
  "Flagship #2 — Rulebook" + mockup `renders/2026-06-21-dynamic-ui/04_rulebook.png`.
- Frame: `dynamic_surface.ai.md`. Glossary route: `internal/systems/.ai.md`.
