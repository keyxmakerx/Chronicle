# timeline_river.js

## Purpose

The time line page's River view: one line through history that drags, wheel-
and pinch-zooms, and coasts. The line is the accent up to today and quiet
after it, fading out at both edges, with a soft wash of the accent under the
history. Events are small beads coloured by kind, durations branch off the
line on curves, cause and effect are arcs under the line, crowded beads gather
into counted bubbles, the open event docks beside the line, and the calendar's
own month folds down under it. Styles are `../css/timeline_river.css`.

## Mount

`data-widget="timeline-river"` in `timelineShowContent` (`timeline.templ`),
loaded on sight by the timeline plugin's `Widgets` registration. Each mount
keeps its own state (boot.js shares one impl across mounts). The page also
links `calendar-view.css` for the folded month.

| Attribute | Meaning |
|---|---|
| `data-campaign-id` | Campaign ID, for links and the calendar calls |
| `data-api-url` | `/campaigns/:id/timelines/:tid/data` |
| `data-calendar-id` | Only when the time line follows a calendar; turns on real calendar dates, Today and the month fold |

The page's `?event=<source>:<id>` (for example `calendar:<event id>`) opens
that event once the line loads, when it is on this time line; the calendar's
page time line block links here that way.

## Data it calls

- `GET /campaigns/:id/timelines/:tid/data`: `events` (EventLink), `eras`
  (Owner/DM only) and `connections`. The server already filtered events and
  connections for this viewer; the widget draws only what it is given.
- `GET /campaigns/:id/calendars/:calid`: months, weekdays and today.
- `GET /campaigns/:id/calendars/:calid/events?year=&month=`: the folded
  month's calendar events, cached per month.

## How it works

- **Dates:** a date becomes a fractional year through `Chronicle.calendarDate`
  (loaded on every page by `calendar_view.js`), so days land where the
  calendar puts them. With no calendar, months are twelve thirty-day months.
- **Kinds:** an event's kind is its free-text category ("Event" when empty);
  colour is the link's `color_override` (plain hex only) or the next of the
  eight `--tlr-k0`…`--tlr-k7` palette colours, in the order kinds first appear
  (lighter in dark mode). Kind chips in the View menu hide and show kinds.
- **The bar** holds the range, Find (it opens on the search icon or `/`) and
  the View menu: All of it / Today, zoom, kinds, era steps, and the overview
  strip under the line, which is off until asked for.
- **The peek** after a 250ms rest on a bead shows its name and date. It is
  a small card, or the fading corner over the bead on a stem when the owner
  picked it (`<html data-cz-tlpop="corner">`, Customize › Hover cards ›
  Popups on time lines), flipping to the bead's left near the right end.
- **An open event** lights its span and draws a playhead under the line;
  everything outside its chain of causes and effects steps back.
- **The line's gradient** is set in pixels on every draw, so the blend at
  today and the edge fades stay in place however the line is panned. Gradient
  and mask ids are random per mount, since two lines can share a page.
- **The month fold** builds the calendar page's own grid markup
  (`.cal-v5 .cal`) and asks `calendarDate` which events fall on each day.
- **The month** is a drawer hanging from a tab right above it ("Show Ches 1492 on the calendar"), not a button.
- **Closing** is always on purpose: Close, Escape (the last thing opened
  first), or clicking the open event again.
- **Motion:** Full eases and coasts; Calm settles faster and drops travel;
  Off jumps. Read from `html[data-motion]` on every frame.

## Events

None emitted. Listens on `document` for `/`, Escape and arrow keys, and for
pointerdown to close the View menu, while mounted; `destroy()` removes those
listeners and the ResizeObserver.

## Gotchas

- The month fold follows the calendar page's look, which takes dark mode from
  the device setting, not the site's dark toggle.
- Any colour from data goes into a style attribute, so only plain hex passes.
