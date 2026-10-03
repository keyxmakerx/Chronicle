# rest_wake.js -- Chronicle.restWake

## Purpose

Shared helper that eases a looping effect to a stop when nobody is using the
page and back up when they return, so effects cost nothing while resting and
need no rest logic of their own. First user: the header's moving background
(`widgets/header_motion.js`, and the Customize example's copy of it).

## How it works

`Chronicle.restWake.create(opts)` returns a controller with a level from 0
(resting) to 1 (full speed). The level eases down over `downMs` (2000) and up
over `upMs` (1000), smoothed. It rests on: tab hidden, window blur, pointer
leaving the window, and `idleMs` (8000) with no pointer/key/wheel/touch input.
Any of that input wakes it.

| Option | Meaning |
|---|---|
| `reduced` | boolean or function; when true the helper is inert (no listeners, no frames, level 0) |
| `onLevel(level, dt)` | each frame while the level changes |
| `onState('active' \| 'resting')` | when the level leaves 0 / returns to 0; start and stop the effect's own loop here |
| `idleMs`, `downMs`, `upMs` | timings |

Controller: `level()`, `isResting()`, `wake()`, `rest()`, `suppress(ms)`,
`destroy()`.

## Rules

- Only transform and opacity may move; an effect draws nothing on `'resting'`.
- Pass the campaign's reduce switch in `reduced` as well as the device setting.
- Tested by `test/js/rest_wake.test.mjs` (fake clock, no browser).
