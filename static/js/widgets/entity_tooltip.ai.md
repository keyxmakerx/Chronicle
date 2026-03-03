# Entity Tooltip Widget

## Purpose

Provides hover preview cards for entity references throughout the app. When the user
hovers over any element with a `data-entity-preview` attribute, a floating card shows
the entity's image, type badge, name, attributes, and entry excerpt. Uses event
delegation on the document for efficiency — works with dynamically added elements
without re-initialization.

## Widget Registration

```js
Chronicle.register('entity-tooltip', { init, destroy });
```

Mounts on: `data-widget="entity-tooltip"` (minimal — event delegation handles behavior)

## Two Usage Modes

### 1. Auto-mounted (boot.js)

Place `data-widget="entity-tooltip"` on a container element. Children with
`data-entity-preview="URL"` automatically get hover behavior via document-level
event delegation.

### 2. Global Helper API

Other widgets can programmatically attach tooltip behavior:

```js
Chronicle.tooltip.attach(element, previewURL)  // Add tooltip to element
Chronicle.tooltip.detach(element)              // Remove tooltip from element
Chronicle.tooltip.show(element, previewURL)    // Manually show
Chronicle.tooltip.hide()                       // Manually hide
Chronicle.tooltip.clearCache()                 // Clear LRU cache (after edits)
```

## Configuration

| Attribute | On | Description |
|-----------|-----|-------------|
| `data-widget="entity-tooltip"` | Container | Registers with boot.js lifecycle |
| `data-entity-preview` | Trigger element | Preview API URL; hover shows tooltip |

## Tooltip Content

The tooltip renders (in order):

1. **Image** (optional) — gradient-bordered (entity type color → purple), 76×76px
2. **Type badge** — colored pill with icon and type name
3. **Type label** (optional) — descriptor text next to badge
4. **Privacy indicator** — lock icon if entity is private
5. **Entity name** — heading
6. **Attributes** — up to 5 key-value pairs from custom fields
7. **Entry excerpt** — first ~150 chars, 3-line clamp
8. **Footer** — "Click to view →" hint

Content controlled per-entity via `popup_config`:
- `showImage` — include/exclude image
- `showAttributes` — include/exclude attribute pairs
- `showEntry` — include/exclude entry excerpt

## API Response Schema

```json
{
  "name": "Gandalf",
  "type_name": "NPC",
  "type_icon": "fa-solid fa-person",
  "type_color": "#6366f1",
  "type_label": "Wizard",
  "image_path": "/uploads/abc123.jpg",
  "is_private": false,
  "attributes": [
    { "label": "Age", "value": "Unknown" },
    { "label": "Race", "value": "Maiar" }
  ],
  "entry_excerpt": "A wandering wizard who...",
  "popup_config": {
    "showImage": true,
    "showAttributes": true,
    "showEntry": true
  }
}
```

## Architecture

### Interaction Timings

| Event | Delay | Purpose |
|-------|-------|---------|
| Hover → show | 300ms | Debounce to avoid API spam |
| Mouseout → hide | 100ms | Allows mouse to move to tooltip |
| Touch → show | 500ms | Long-press on mobile |
| Fade-out transition | 150ms | Smooth disappearance |
| Hide cleanup | 160ms | Wait for fade before `display: none` |

### LRU Cache

- Max 100 entries
- On hit: entry promoted to most-recently-used
- On capacity: least-recently-used entry evicted
- `clearCache()` exposed via global API (call after entity edits)

### Smart Positioning

- Prefers **below** the trigger element with 8px gap
- Flips **above** if insufficient space below viewport edge
- Horizontally centered on trigger, clamped to viewport (8px margin)
- Uses `requestAnimationFrame` for positioning after render

### Event Delegation

All mouse/touch events use document-level delegation:
- `mouseover` on document → find nearest `[data-entity-preview]` ancestor
- `mouseout` on document → schedule hide with delay
- `touchstart` on document → long-press timer or dismiss
- `touchend` / `touchmove` → cancel long-press
- `keydown` (Escape) → dismiss
- `scroll` on window (capture phase) → dismiss on any scroll

### Singleton Tooltip Element

One tooltip DOM element (`<div class="et-tooltip">`) created lazily and reused.
Hovering over the tooltip itself cancels the hide timer.

## CSS Classes

Inline styles injected once via `<style id="entity-tooltip-styles">`:

| Class | Description |
|-------|-------------|
| `.et-tooltip` | Main container (fixed position, z-9999) |
| `.et-tooltip--visible` | Visible state (opacity 1, pointer-events auto) |
| `.et-tooltip--above` | Positioned above target |
| `.et-tooltip__content` | Flex row: image + info side-by-side |
| `.et-tooltip__content--no-image` | Block layout when no image |
| `.et-tooltip__image-wrap` | Gradient-bordered image container |
| `.et-tooltip__image` | Image element (cover fit) |
| `.et-tooltip__info` | Info column (name, badge, attrs) |
| `.et-tooltip__type-row` | Badge + label row |
| `.et-tooltip__badge` | Type badge pill (dynamic bg color) |
| `.et-tooltip__name` | Entity name heading |
| `.et-tooltip__attrs` | Attributes key-value list |
| `.et-tooltip__attr` | Single key-value pair |
| `.et-tooltip__excerpt` | Entry excerpt (3-line clamp) |
| `.et-tooltip__footer` | Bottom hint text |
| `.et-tooltip__loading` | Loading state |

All classes have `.dark` variants for dark mode support.

## Accessibility

- Tooltip element has `role="tooltip"`
- Trigger elements get `aria-describedby` pointing to tooltip ID
- `aria-describedby` removed on hide
- Escape key dismisses tooltip
- Unique ID per tooltip instance (`entity-tooltip-N`)

## Utility Functions

- `escapeHtml(str)` — Prevent XSS in rendered content
- `escapeAttr(str)` — Escape HTML attributes
- `contrastTextColor(hex)` — ITU-R BT.709 perceived brightness for badge text color

## Dependencies

- `Chronicle.register()` — Widget lifecycle
- Fetch API — Preview endpoint requests
- No external CSS (all styles inline)
- No TailwindCSS dependency
- Font Awesome — Type icons and lock icon

## Known Limitations & Future Work

- No keyboard-only trigger (hover/touch only)
- Cache not persisted across page loads
- No prefetching on link proximity
- Tooltip width fixed at 320px (max 90vw on mobile)
- No support for rich HTML in entry excerpt (plain text only)
