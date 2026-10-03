# notifications.js

## Purpose

Global toast system. Defines `Chronicle.notify(message, type, opts)` and
turns server `HX-Trigger` events and HTMX request failures into toasts. It is
unrelated to the in-app notification inbox served by the sessions plugin
(`/notifications`).

## Loading

A `<script defer>` tag in `layouts/base.templ`, right after htmx and before
`boot.js` and Alpine so `Chronicle.notify` exists when widgets initialise.
It creates `window.Chronicle` if it does not exist yet.

## API

`Chronicle.notify(message, type = 'info', opts)`

| Argument | Meaning |
|---|---|
| `type` | `success`, `error`, `info`, `warning`; unknown falls back to `info` |
| `opts.duration` | Auto-dismiss ms, default 4000; `0` keeps it until closed |
| `opts.html` | Treat `message` as HTML; trusted content only |

A repeat of an on-screen toast (same message and type) does not stack: it adds
a `×N` badge and restarts the timer. The badge bump animation is skipped under
`prefers-reduced-motion`.

## Events listened to

| Event | Effect |
|---|---|
| `chronicle:notify` | `detail` `{message, type, duration}` becomes a toast. Servers send it as `HX-Trigger: {"chronicle:notify": {...}}` (for example `internal/app/app.go`, armory and addons handlers) |
| `htmx:responseError` | Generic toast by status (400, 403, 404, 5xx, else "Request failed"); skipped when the response already carries a `chronicle:notify` trigger or `Chronicle.isReauthResponse(xhr)` is true |
| `htmx:sendError` | "Connection error" toast, 6 s |

It emits no events of its own.

## Who uses it

About 40 files across `static/js` and templates call `Chronicle.notify`
(widgets, editor, admin pages).

## Gotchas

- Toasts live in `#chronicle-toasts` (fixed, top right, `z-index: 10000`,
  `pointer-events: none` on the container, enabled per toast).
- Styles are inline; colours fall back to hard-coded values when the theme
  variables `--color-card-bg`, `--color-border`, `--color-text-body` are
  absent, and the dark-mode text colours in `typeConfig` are defined but not
  applied.
- Plain `fetch` failures are not covered; only HTMX errors are.
