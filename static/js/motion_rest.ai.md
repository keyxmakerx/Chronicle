# motion_rest.js

The rest clock shared by every looping animation on the page (`window.MotionRest`).

**Why it exists.** Ongoing animations should behave the same way when the
viewer steps away. They ease to a standstill instead of running on, which
saves battery, and they come back together when the viewer returns (#935).
One-shot transitions do not use it: drawers, the sky's fold, card openings.

**When the viewer counts as away**
- the tab is hidden: rests at once, since nothing is drawn anyway
- the window loses focus
- the mouse leaves the window: a lifted finger does not count
- 8 seconds pass with no pointer, key, wheel, touch or scroll input

Speed eases from 1 to 0 over 2 seconds. Any input eases it back to 1 over
1 second.

**API**
- `now()`: rest time in seconds, which a loop uses as its clock instead of
  `performance.now() / 1000`. It runs at `speed()`, so motion slows smoothly
  and resumes exactly where it stopped, with no jump.
- `speed()`: 1 normally, 0 at rest, in between while easing.
- `still()`: true once fully at rest. A loop stops requesting frames then and
  leaves its last frame up.
- `onWake(fn)` / `offWake(fn)`: `fn` runs when motion starts again; use it to
  restart a stopped loop.
- `simulate(away)`: for mockups and tests. `true` rests now; `false` hands
  back to the real signals.

Loaded in `base.templ` before anything that loops. Users: the sky pane
(`internal/widgets/sky/.ai.md`, "Rest"). Tests: `test/js/motion_rest.test.mjs`.
