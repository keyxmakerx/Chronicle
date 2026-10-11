# header_sky.js -- the header's Sky background

Mounts on `data-widget="header-sky"` (rendered by `Topbar()` in
`internal/templates/layouts/app.templ` when the header background is "sky"
and the viewer can see a campaign calendar). It reads that calendar, the
day's events and the day's own weather, and paints the same sky the
calendar shows over the month, using `SkyPane.Scene` from `sky_pane.js`
(the painted sky where the device can, the basic sky otherwise). The
canvas fades in over the still night colours the server puts behind it;
with no calendar, or when a read fails, those colours are all there is.

Time is the shared `MotionRest` clock: every frame while something visibly
moves, 12 a second while only clouds drift and stars twinkle, nothing once
the page is at rest, and again on wake. Under prefers-reduced-motion,
`html[data-cz-reduce]` or `html[data-view-motion="calm"]` it paints once and
holds. The header outlives boosted navigation, so the calendar is read
again after five minutes on the next wake.

`Chronicle.headerSky.mount(el, {campaignId, calendarId, still})` is the
same sky for the Customize page's example header. The Customize page
re-renders `#topbar-bg` after a Save and re-mounts the widget, so `destroy`
stops its frames and its wake hook. Tests: `test/js/header_sky.test.mjs`.
