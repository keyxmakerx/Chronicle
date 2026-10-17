# timeline_river.js

## Purpose

The time line page's River view: one line through history that drags, wheel-
and pinch-zooms, and coasts. Events are dots coloured by kind, cause and
effect are arcs under the line, crowded dots gather into counted bubbles, the
open event docks beside the line, and the calendar's own month folds down
under it. Styles are `../css/timeline_river.css`.

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
  colour is the link's `color_override` (plain hex only) or a hue hashed from
  the name, lighter in dark mode. Kind chips hide and show kinds.
- **The month fold** builds the calendar page's own grid markup
  (`.cal-v5 .cal`) and asks `calendarDate` which events fall on each day.
- **The month** is a drawer hanging from a tab right above it ("Show Ches 1492 on the calendar"), not a button.
- **Closing** is always on purpose: Close, Escape (the last thing opened
  first), or clicking the open event again.
- **Motion:** Full eases and coasts; Calm settles faster and drops travel;
  Off jumps. Read from `html[data-motion]` on every frame.

## Events

None emitted. Listens on `document` for `/`, Escape and arrow keys while
mounted; `destroy()` removes that listener and the ResizeObserver.

## Gotchas

- The month fold follows the calendar page's look, which takes dark mode from
  the device setting, not the site's dark toggle.
- Any colour from data goes into a style attribute, so only plain hex passes.
