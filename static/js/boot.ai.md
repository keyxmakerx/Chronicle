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
   for new widgets (and after `htmx:historyRestore`, scans the whole body). Before `htmx:beforeSwap`, destroys outgoing widgets.
4. **Config parsing:** Collects `data-*` attributes from mount elements,
   converts kebab-case to camelCase, auto-parses booleans and numbers.
5. **Loading on sight:** a `[data-widget]` mount whose widget isn't registered
   yet gets its scripts from the page's `#chronicle-widget-scripts` map (built
   by `buildWidgetManifest` in `internal/app/plugins.go` from each plugin's
   `Widgets`; ADR-063), appended to `<head>` once per URL per page, then
   mounts when they register.

## Cross-Cutting Features

| Feature | Mechanism |
|---------|-----------|
| CSRF tokens | Reads the `__Host-chronicle_csrf` cookie (HTTPS), falling back to `chronicle_csrf` (HTTP dev), and attaches `X-CSRF-Token` on all HTMX requests |
| Loading indicator | Tracks active HTMX requests, toggles `body.htmx-request` class |
| Unsaved changes | `Chronicle.markDirty(id)` / `Chronicle.markClean(id)` with `beforeunload` prompt; a boosted GET link or form is held on `htmx:confirm` while dirty and asked about with `Chronicle.askSlip` on the dirty form (a full page load, so the browser asks, when no such form is on screen); `Chronicle.discardChanges()` after the person chose to leave without saving |
| Where you were | A boosted swap into `#main-content` scrolls it to the top; each page's `#main-content` offset is kept by address (sessionStorage) and put back on Back |
| Back closes a sheet | `Chronicle.backCloses(name, close)` returns `release()`; on phones and touch tablets it pushes a same-address history step so Back closes the sheet. Used by the page peek and the phone sidebar drawer. A boosted link followed from inside an open sheet first steps back off the sheet's step (closing the sheet) and then goes, so the new page takes that step |
| Form tracking | Forms with `data-track-changes="<id>"` auto-mark dirty on input |
| Sidebar highlighting | After `htmx:pushedIntoHistory`, updates sidebar `.active` via longest-prefix-match |
| Form validation | Inline error hints below invalid `.input` fields |

## Shared Utilities

| Function | Description |
|----------|-------------|
| `Chronicle.escapeHtml(str)` | HTML entity escaping |
| `Chronicle.escapeAttr(str)` | Attribute value escaping (`Chronicle.escapeHtml` is the same function) |
| `Chronicle.getCsrf()` | Returns current CSRF token string |
| `Chronicle.apiFetch(url, opts)` | Fetch wrapper with CSRF header injection. Inside an outside app's frame (`Chronicle.embed` set by `notes_embed.js`) a `/campaigns/...` URL is rewritten to `/api/notes-app/campaigns/...` with the embed's bearer token |
| `Chronicle.embed` | Set by `notes_embed.js` in an outside app's frame: `{ token, go }`; absent on the site |
| `Chronicle.go(url)` | Navigates to a Chronicle address; in an embed frame `Chronicle.embed.go` decides (page in a new tab, Journal note in the app's notebook) |
| `Chronicle.openJournalNote(id)` | Set by the Journal widget while mounted; opens a note in it and returns whether it did. The Journal loads on sight, so it fires `chronicle:journal-ready` (window) once mounted for callers holding a note |
| `Chronicle.cameFrom()` | The address of the page the reader was on before this one. A boosted link leaves `document.referrer` on the last page loaded in full, so boot.js records the address just before htmx pushes the new one; a page loaded in full answers the referrer, and one restored by Back or Forward answers `''`. The book's way back reads it. Test: `test/js/boot_came_from_browser.test.mjs` |
| `Chronicle.isReauthResponse(xhr)` | True for a 403 carrying `HX-Trigger: reauth-required` (the password re-confirm flow) |
| `Chronicle.tokenMaker.open(o)` | A stub that loads the token maker (`token_maker.js`, from `#chronicle-token-maker-src`) on its first open; a click on any `[data-token-maker]` control opens it for that page. See `token_maker.ai.md` |

## DOM Events

| Event | Direction | Description |
|-------|-----------|-------------|
| `chronicle:navigated` | Emits (window) | Fired after `htmx:pushedIntoHistory`; listened to by several scripts (sidebar filters and peek, command palette, quick capture, search) |
| `DOMContentLoaded` | Listens | Initial widget mount scan |
| `htmx:afterSettle` | Listens | Re-scan for new widgets after swap |
| `htmx:beforeSwap` | Listens | Destroy outgoing widgets |
| `htmx:historyRestore` | Listens | Back/Forward swaps in a whole body with no afterSettle, so mount widgets across `document.body` |
| `htmx:pushedIntoHistory` | Listens | Sidebar highlight + navigation event |
| `htmx:configRequest` | Listens | CSRF token injection |

## Widget Lifecycle

```
Load:     unregistered mount → its scripts from #chronicle-widget-scripts
     ↓
Register: Chronicle.register('my-widget', { init, destroy })
     ↓
Mount:    boot.js finds <div data-widget="my-widget"> → calls init(el, config)
     ↓
Re-mount: htmx:afterSettle → scan new content → init() for uninitialized elements
     ↓
Destroy:  htmx:beforeSwap → calls destroy(el) for elements being removed
```
