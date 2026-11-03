# craft_paint -- painted crafting benches

`data-widget="craft_paint"` mounts a painted bench: a crafting room drawn on a
canvas (the forge first), finished with lighting effects at the level the
device can hold. It is the look the Director signed for the benches; the
play rules stay in `CraftScene` (`craft_bench.ai.md`), and hooking a painted
scene in as a bench skin is separate work. Today the admin Design Lab
(`/admin/design-lab`) is the only page that mounts it.

## The parts

Loaded together on sight (`coreWidgets` in `internal/app/routes.go`), in this
order:

| File | Owns | Global |
|---|---|---|
| `craft_paint.js` | The toolkit every painted scene shares: seeded noise, baked textures (wood, stone, flags, plaster, slate), light (pools, several lights, glows, contact shadows), grain and vignette, a particle list, the baked flipbook fire, and `define(name, make)` | `CraftPaint` |
| `craft_paint_forge.js` | The forge scene: hearth, bellows, anvil, the hammer's strikes, quench tub, rack, window | `CraftPaint.scenes.forge` |
| `craft_paint_stage.js` | Runs a scene: the effect levels, stepping down on slow frames, one stage at a time, the window's sky, the Design Lab controls, and the widget glue | `CraftStage`, registers `craft_paint` |

The graphics-card level paints on the page's one WebGL2 context,
`SharedGL` (`gl_shared.js`, loaded globally before the sky in `base.templ`).
The skies use the same context and the same slow-device verdict, so a page
never holds two contexts and a device found slow is slow for both.

## Effect levels

| Level | What it adds over the scene |
|---|---|
| `gpu` (Graphics card) | One shader pass: heat haze over the fire, a shockwave off each blow, colour fringes on the brightest light, bloom, rays toward the window's light. The window's sky is the painted sky |
| `canvas` (Canvas glow) | A small blurred copy of the bright parts laid back as bloom, rays, a streak on each blow |
| `basic` (Plain canvas) | Nothing: the scene alone, its baked fire and light |
| `still` (Still frame) | One frame and a Play button |

- **Start:** `gpu`, or `canvas` when the device reports little memory (4 GB
  or less), few cores (4 or fewer) or data saver, or when the page already
  found it slow (`SharedGL.isSlow()`), or when there is no shared context
  (no WebGL2, or a software renderer).
- **Step down:** after 30 unjudged frames, frames averaging slower than
  24 ms for about two seconds drop one level (`governor()`); a short stutter
  drains away. Reaching `still` pauses with "Paused to keep this page smooth.
  Play anyway", which runs `basic` and stops judging.
- **One verdict:** leaving `gpu` for slowness calls `SharedGL.markSlow()`,
  which turns the skies basic for the session; a sky that marks the device
  slow moves an adjusting stage off `gpu`. The Design Lab's pretend-slow
  switch never marks the page slow.
- **A failed copy** (a blank first copy out of the shared context) or a lost
  context moves the stage to `canvas`; it never shows a black box.
- The readout (Design Lab only) lists each step and why.

## Motion and the page

- Only one stage animates at a time; Play on another pauses the first.
  A stage leaving the view pauses, and the first stage in page order still
  in view (some of it showing) takes over after the reports settle. The
  pause button pauses for good until Play.
- A hidden tab draws nothing. Drawing is capped at twice the screen's pixel
  density; the scene itself is drawn at up to 1.5x.
- Calm (reduce-motion, `data-motion="calm"`, `data-view-motion="calm"`,
  `data-cz-reduce`) waits for Play. Off (`data-motion="off"`) shows the
  still frame with no Play button.
- Clicking a playing scene calls its `poke` (the forge starts a fresh run of
  strikes); it never pauses.

## The window's sky

With `data-campaign-id` and `data-calendar-id`, the window shows that
calendar's sky as players see it: the calendar is read with
`?audience=players`, hidden moons are dropped, and the weather comes from
`/weather/days?…&audience=players`. Without them it shows a demo calendar.
The sky repaints 10 times a second into the window, with the painted sky on
`gpu` and the basic sky (`Sky2D`) below it, and its light tints the room.

## Mount

```html
<div data-widget="craft_paint" data-scene="forge" data-name="the forge"
     data-campaign-id="…" data-calendar-id="…"></div>
```

| Attribute | Description |
|---|---|
| `data-scene` | A `CraftPaint.scenes` name; `forge` by default |
| `data-name` | Said on the Play button and the pause button's label ("Play the forge") |
| `data-level` | Hold one level (`gpu`, `canvas`, `basic`, `still`); omitted means adjust |
| `data-lab` | Show the level picker, pretend-slow switch and readout |
| `data-campaign-id`, `data-calendar-id` | The window's calendar, as above |

`host._paint` is the handle: `level()`, `setLevel(name | 'auto')`, `running()`,
`play()`, `pause()`, `poke()`, `steps()`, `destroy()`. Destroying empties the
mount and stops the scene.

## A new scene

`CraftPaint.define('name', function (g, canvas) { … return v; })`. `g` is a 2D
context already scaled to the logical size, 640x360. The scene object:

| Field | Set by | Meaning |
|---|---|---|
| `frame(t, dt)` | scene | Draw one frame at time `t` |
| `look` | scene | Where the effects aim: `window` `[x, y, w, h]` for the sky, `hit` `[x, y]` for the blow's shockwave, `light` `[x, y]` for the rays, `haze` up to two `[x0, y0, x1, y1]` boxes |
| `label` | scene | The canvas's accessible name |
| `poke()` | scene | What a click does |
| `hits` | scene | A counter bumped on each blow; the levels flash on a change |
| `rich` | stage | True on `gpu` and `canvas`: draw the costlier extras (the forge's rising smoke) |
| `sky`, `skyLight` | stage | The window's sky canvas and its light colour, set before each frame |

Every level draws the scene's own fire and light, so a scene must look
finished on `basic`; the levels only add.

A scene never draws its canvas onto itself in a loop: each such call copies
the whole canvas, so the forge's heat haze (34 strips) cost 47 ms a frame on a
2x screen without a graphics card. Copy the region once a frame into a small
canvas and draw the strips from that copy, as the forge's `shimmer` does.

## Tests

- `test/js/craft_paint.test.mjs`: the rules in a vm (the governor's step
  down, warm-up and drain, the weak-device start, motion modes, level names,
  the forge registering, seeded noise, particles).
- `test/js/craft_paint_browser.test.mjs`: the widget in Chromium (every
  level paints the lit forge and the richer ones add light, a few-cores start
  and slow frames stepping down to the still frame, one stage at a time and
  pausing off screen, Calm, Off, the players' calendar and a device already
  found slow, destroy).
