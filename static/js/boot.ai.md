# boot.js -- Widget Auto-Mounter & Chronicle Namespace

## Purpose

Global initialization script that creates the `window.Chronicle` namespace,
provides the widget registration/lifecycle system, and handles cross-cutting
concerns: CSRF token injection, HTMX integration, unsaved changes tracking,
sidebar active link highlighting, and shared utilities.

## How It Works

1. **Widget registration:** `Chronicle.register(name, impl)` registers a widget
   with `init(el, config)` and optional `destroy(el)` methods.
2. **Auto-mounting:** On `DOMContentLoaded`, scans DOM for `[data-widget]`
   elements and calls the matching widget's `init()`. Uses a `WeakMap` to
   prevent double-initialization.
3. **HTMX integration:** After `htmx:afterSettle`, re-scans the swapped target
   for new widgets. Before `htmx:beforeSwap`, destroys outgoing widgets.
4. **Config parsing:** Collects `data-*` attributes from mount elements,
   converts kebab-case to camelCase, auto-parses booleans and numbers.

## Cross-Cutting Features

| Feature | Mechanism |
|---------|-----------|
| CSRF tokens | Reads `chronicle_csrf` cookie, attaches `X-CSRF-Token` on all HTMX requests |
| Loading indicator | Tracks active HTMX requests, toggles `body.htmx-request` class |
| Unsaved changes | `Chronicle.markDirty(id)` / `Chronicle.markClean(id)` with `beforeunload` prompt |
| Form tracking | Forms with `data-track-changes="<id>"` auto-mark dirty on input |
| Sidebar highlighting | After `htmx:pushedIntoHistory`, updates sidebar `.active` via longest-prefix-match |
| Form validation | Inline error hints below invalid `.input` fields |

## Shared Utilities

| Function | Description |
|----------|-------------|
| `Chronicle.escapeHtml(str)` | HTML entity escaping |
| `Chronicle.escapeAttr(str)` | Attribute value escaping |
| `Chronicle.getCsrf()` | Returns current CSRF token string |
| `Chronicle.apiFetch(url, opts)` | Fetch wrapper with CSRF header injection |

## DOM Events

| Event | Direction | Description |
|-------|-----------|-------------|
| `chronicle:navigated` | Emits (window) | Fired after `htmx:pushedIntoHistory`, used by notes widget |
| `DOMContentLoaded` | Listens | Initial widget mount scan |
| `htmx:afterSettle` | Listens | Re-scan for new widgets after swap |
| `htmx:beforeSwap` | Listens | Destroy outgoing widgets |
| `htmx:pushedIntoHistory` | Listens | Sidebar highlight + navigation event |
| `htmx:configRequest` | Listens | CSRF token injection |

## Widget Lifecycle

```
Register: Chronicle.register('my-widget', { init, destroy })
     ↓
Mount:    boot.js finds <div data-widget="my-widget"> → calls init(el, config)
     ↓
Re-mount: htmx:afterSettle → scan new content → init() for uninitialized elements
     ↓
Destroy:  htmx:beforeSwap → calls destroy(el) for elements being removed
```
