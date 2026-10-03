# timeline_widget.js

## Purpose

Adds a quick-preview button to each timeline link in a server-rendered
timeline summary card. Clicking it opens a small popup listing the first five
events; full viewing and editing stay on the timeline page.

## Mount

`data-widget="timeline-widget"`, loaded by a `<script defer>` tag in
`layouts/base.templ`. Mounted from `plugins/timeline/blocks.templ` (when no
timeline is bound), `plugins/entities/category_blocks.templ` and
`plugins/campaigns/dashboard_blocks.templ`.

| Attribute | Meaning |
|---|---|
| `data-campaign-id` | Campaign ID (stored, not otherwise used) |
| `data-limit` | Stored but unused by the script; the list length comes from the server-side `?limit=` on the HTMX request |

The card's list is not rendered by this widget: the mount contains an
`hx-get` of `/campaigns/:id/timelines/preview?limit=N` with
`hx-trigger="intersect once"`.

## How it works

A `MutationObserver` watches the mount for `a[href*="/timelines/"]` links
(they arrive via the HTMX swap) and inserts an eye button after each. The
button opens a popup inside the mount element.

## Data it calls

`GET <timeline-link-href>/data`, which is `/campaigns/:id/timelines/:tid/data`
(in `routes_snapshot.txt`). Reads `events[].name` and `events[].display_date`.

## Events

None emitted or listened to. `destroy()` disconnects the observer.

## Gotchas

- The popup is positioned absolutely at the centre of the mount; the script
  forces `position: relative` on it.
- Only one popup exists at a time; opening another removes the first.
- Any link whose href contains `/timelines/` inside the mount gets a button,
  so a link added to the card by a future template change would get one too.
