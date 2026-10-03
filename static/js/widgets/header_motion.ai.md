# header_motion.js -- the header's moving background

Mounts on `data-widget="header-motion"` (rendered by `Topbar()` in
`internal/templates/layouts/app.templ` when the header background is
"moving"). The element is an over-wide strip of the owner's two gradient
colours (`data-axis` "x" or "y"); the widget slides it one bar-length in a
slow loop with the Web Animations API (transform only), its playback rate
following the shared `MotionRest` clock and pausing once that is still.
`Chronicle.headerMotion.drive` is the same slide for the Customize page's
example header. Under
prefers-reduced-motion or `html[data-cz-reduce]` it never plays.

The Customize page re-renders `#topbar-bg` after a Save and re-mounts the
widget (`customize_look.js`, `swapFrom`), so `destroy` cancels the animation
and its wake hook.
