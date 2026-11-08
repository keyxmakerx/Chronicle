# Motion craft

General practice from well-known sources, applied to Chronicle. The moves,
timings and tokens themselves are in `.ai/ui-standard.md` (**Motion**,
**Every interaction**); this file explains why they are shaped that way and
how to build them so they hold up.

## What motion is for

Motion earns its place only when it does one of these jobs (Material Design
and Apple's Human Interface Guidelines say the same):

- **Shows where something came from and went** (a menu grows from its
  button, so the eye keeps its place).
- **Shows that the app heard you** (a press, a fill, a spinner).
- **Shows a change of state** (a switch's knob, a count growing in).
- **Points at what changed** (a Glow on a new row).

If a move does none of these, leave it out. Decoration that repeats on every
visit becomes a wait.

## Timing

- People read a response under about 0.1 s as instant, under 1 s as
  uninterrupted, and lose focus past about 10 s (Nielsen's response-time
  limits). So: feedback starts at once; the move can then take its time.
- Most interface moves sit between 100 and 400 ms. Small feedback (a toggle)
  is about 100 ms, a dialog 200 to 300 ms, and 400 ms is for large travel on
  a big screen; near 500 ms feels slow (Nielsen Norman Group, "Executing UX
  Animations: Duration and Motion Characteristics").
- **Distance and size set the duration.** A small thing moving a short way
  is quick; a whole page is slower. Don't give everything one duration.
- **Exits are quicker than entrances.** The person has already decided to
  close it, so it gets out of the way.
- **Stagger sparingly.** If rows enter one after another, keep the step
  around 20 to 40 ms and stop after the first handful, or the last row
  waits too long.

## Easing

- **Arrivals decelerate** (fast start, slow stop): the thing feels like it
  was thrown into place and settled. Chronicle: `--ease-out`, `--ease-slide`.
- **Exits accelerate** (slow start, fast finish): it leaves the stage.
  Chronicle: `--ease-in`.
- **Moves on screen from one place to another** ease both ways
  (`--ease-standard`).
- **Linear is for things with no physical mass**: a progress line, a
  rotating spinner, a fade on its own.
- Overshoot and bounce read as playful and draw the eye; Chronicle keeps
  them out except where the standard signs one.

## Building it so it holds up

- **Animate `transform` and `opacity` only.** The browser can run them on
  the compositor without recalculating layout or repainting; `width`,
  `height`, `top`, `left`, `margin` and `box-shadow` animate on the main
  thread and stutter on weak devices (web.dev, "How to create
  high-performance CSS animations"). To animate a shadow, fade a
  pseudo-element that already has the larger shadow.
- **Size changes:** measure (`scrollHeight`), animate to the number, then
  release to `auto`. For a layout jump, use FLIP: record the first position,
  make the change, invert it with a transform, then play the transform to
  zero.
- **Interruptible.** A second click mid-move must reverse from where the
  thing is, not snap to the end and replay. CSS transitions do this for
  free; keyframe animations and scripted sequences do not. Prefer
  transitions, or the Web Animations API (`element.animate`), whose
  animations can be reversed and cancelled.
- **Capture before you change.** View transitions (`document.startViewTransition`,
  and the cross-document `@view-transition`) take a picture of the old
  state; anything that scrolls or reflows before the capture shows as a
  jump. Always provide the plain path for browsers without them.
- **`will-change` sparingly**, set just before a move and removed after; on
  many elements it costs memory and can blur text.
- **Never wait on an animation to finish a task.** Save, navigate and focus
  happen on the real event; the motion only shows them. Off must still work
  because nothing waited on `transitionend`.
- **Move focus with the content.** When a panel opens, focus goes into it;
  when it closes, focus returns to what opened it.

## Reduced motion and accessibility

- Motion triggers dizziness and nausea for people with vestibular
  disorders. The worst offenders are large travel across the screen,
  zooming, parallax, spinning and anything that moves with scrolling.
- **WCAG 2.2, 2.3.3 Animation from Interactions (AAA):** motion started by
  an interaction can be turned off unless it is essential. Chronicle meets
  it with Calm and Off; a device's `prefers-reduced-motion: reduce` is
  always Off.
- **WCAG 2.2, 2.2.2 Pause, Stop, Hide (A):** anything that moves or blinks
  by itself for more than 5 seconds needs a way to stop it. Chronicle's
  loops rest when the person steps away (`MotionRest`); a long ambient loop
  also needs a visible stop.
- **WCAG 2.2, 2.3.1 Three Flashes (A):** nothing flashes more than three
  times a second.
- Reduced motion means **less travel, not less feedback**: keep fades,
  colour changes and focus rings so the person still sees what happened.

## Sources

- Material Design 3, Motion: https://m3.material.io/styles/motion/overview
- Apple Human Interface Guidelines, Motion:
  https://developer.apple.com/design/human-interface-guidelines/motion
- Nielsen Norman Group, animation duration:
  https://www.nngroup.com/articles/animation-duration/
- Nielsen Norman Group, response-time limits:
  https://www.nngroup.com/articles/response-times-3-important-limits/
- web.dev, high-performance CSS animations:
  https://web.dev/articles/animations-guide
- web.dev, `prefers-reduced-motion`:
  https://web.dev/articles/prefers-reduced-motion
- MDN, View Transition API:
  https://developer.mozilla.org/en-US/docs/Web/API/View_Transition_API
- WCAG 2.2: https://www.w3.org/TR/WCAG22/
