# craft_bench -- the crafting bench and how to build one

`data-widget="craft_bench"` mounts a crafting scene: a lit workbench where a
player makes something one step at a time (drag an ingredient into the mortar,
grind, tip it into the flask, pour to the line, keep the heat steady, strike at
the right moment), and the steps grade into a Draw Steel project roll. Build
mode is the same scene with an editor round it, for a Director to arrange the
room. The crafting app (`internal/plugins/crafting`) mounts build mode on a
Director's stored bench and saves what they arrange, and mounts play mode on
a player's project from the Crafting block, where the server makes the roll;
the admin Design Lab (`/admin/design-lab`) runs it against demo data and
saves nothing.

## The parts

A bench is five scripts, loaded together on sight (`coreWidgets` in
`internal/app/routes.go`):

| File | Owns | Global |
|---|---|---|
| `craft_scene.js` | Play: the steps, grading, the project roll, hover cards, the right-click menu, particles, sound | `CraftScene` |
| `craft_desk.js` | One skin: the candlelit desk's art, light, sky window and ambiance, and what each piece is | `CraftDesk` |
| `craft_journal.js` | The other skin: the crafter's paper journal open on a desk, its ink, quill, blots, page turn and wax seal | `CraftJournal` |
| `craft_bench_editor.js` | Build mode, for any skin that fills in the editor tables below | `CraftBenchEditor` |
| `craft_bench.js` | The widget glue: reads the mount's attributes, injects the bench's CSS once, hands each part its data | registers `craft_bench` |

Every skin plays the same rules, so a new look (a forge, a ritual circle) is
a new skin, never a new engine. The CSS lives in
`craft_bench.js` and is injected at mount, like the shop room's, so Tailwind's
purge never sees it.

Attributes: `data-mode` (`play`, the default, or `build`), `data-skin`
(`desk`, the default, or `journal`), `data-thumbs` (the folder of tray
pictures, `/static/img/crafting/desk/` or `/static/img/crafting/journal/`), and
`data-campaign-id` + `data-calendar-id` for the window's sky. Without both ids
the window shows the demo calendar. The Design Lab passes
`?campaign_id=<id>&calendar_id=<id>` through to them, so an admin can see a real
campaign's sky in the window.

The crafting app sets `el._craftApp` before mounting a build-mode bench:
`{state, recipes, items, recipe, status, onChange}`. `state` is the look's
saved arrangement (`{look, pieces}`, as `CraftBenchEditor`'s `state()` returns
it, or null for a fresh room), `recipes` and `items` are the campaign's own in
the engine's form (no recipes means the demo's), `status` is the bar's line,
and `onChange(state)` hears every change. The editor stores nothing itself:
saved pieces come back through `o.state`, the room's look through the skin's
own opts, and `ed.status(text)` rewrites the bar's line.

A play-mode bench takes `{state, recipes, items, recipe, settings, onWorked}`
the same way. `state` is the Director's arrangement, laid out with
`CraftBenchEditor.arrange(skin, svg, state)` (the editor's own placing, with
no editor round it); `settings` are the campaign's (difficulty, bane, skip);
`onWorked` takes over the roll, below.

## The engine

`CraftScene.mount(el, skin, {recipes, items, recipe, settings, onWorked})` returns
`{reset, settings, apply, sound, skip, retry, on, state, stage, svg, skin, rerender, destroy}`.

- **Recipes and items are data from anyone**, a Director or their AI, so the
  engine cleans them (`rules.cleanRecipes`, `rules.cleanItems`): strings become
  strings, numbers are clamped, a colour that is not a hex colour falls back.
  Every text that reaches `innerHTML` is escaped.
- **Steps**: `add` (`to: 'mortar'|'flask'`; anything else means the mortar),
  `grind` (`turns` 1-8), `tip`, `pour` (`item`), `simmer` (`secs` 2-20),
  `strike` (`n` 1-6). Unknown kinds and missing ingredients are dropped.
  `pour`, `simmer` and `strike` are graded perfect, good or miss.
- **The verdict follows Draw Steel**: no misses and at least one perfect earns
  an edge (+2 on the project roll); two or more misses earn a bane (−2) only
  when `settings.bane` is on; anything else is a plain roll. Skipping the
  steps (allowed when `settings.skip` is on) is a plain roll.
- **A host can make the roll.** With `onWorked`, the roll phase's button
  hands `onWorked(work, verdict)` how the steps went (`rules.work`: `clean`,
  `botched`, `steady` or `skipped`, botched counting two misses whether or
  not the campaign gives banes) and the scene waits in the `worked` phase
  instead of rolling its own dice. `retry()` offers the roll again when the
  host could not make it. Without `onWorked` the scene rolls for itself, as
  the Design Lab does.
- **Ids are scoped per scene.** Every id a skin draws, and every `url(#…)` and
  `href="#…"` pointing at one, is prefixed per mount (`rules.scoper`), so two
  benches on a page never share a gradient or filter. A skin writes plain ids.
- **One frame loop per scene**, stopped while the scene is off screen or the
  tab is hidden. Timers go through `later()` and die with the scene. Calm
  (reduce-motion, `html[data-cz-reduce]`, `data-view-motion="calm"`) is read
  per mount: tweens finish at once and shakes are skipped.
- **A Director's actions are data, never code.** A piece's right-click actions
  are `[label, name]` (a skin's own action, run by `skin.act`) or
  `{label, fx, sound, say}`, where `fx` is one of `CraftScene.effects`
  (`shake`, `sparkle`, `smoke`, `glow`) or `b:<name>` for one of the skin's
  built-in actions, `sound` is one of `CraftScene.sounds`, and `say` is a line
  of text shown over the piece.

## What a skin provides

`CraftDesk.create(opts)` is the model; a new skin copies its shape.

| Member | Used by | What it is |
|---|---|---|
| `label` | engine | The scene's accessible name |
| `layout` | engine | Where the flask, mortar, ingredient slots and gauges sit, in the 1200×700 viewBox |
| `defs()`, `background()`, `foreground()`, `shade()` | engine | SVG strings for the scene's layers |
| `art.{burner, flask, mortar, pestle, item, bottle, gaugeFrame, barFrame}` | engine | The tools and ingredients, drawn by the skin |
| `book(state)` | engine | The recipe book's pages, redrawn as the steps go |
| `fx.{drop, fizz, dust, steam, spark, smoke, bubbles, success}` | engine | Particle bursts for each moment of play |
| `ambient(P, dt, state, clock)`, `light(clock, state, svg)` | engine | Per-frame ambiance and the flicker |
| `info`, `act(target, name)` | engine | Each piece's hover name, lines and actions, and the skin's own actions |
| `itemSpot(key, i)`, `onStep`, `onDone` | engine | Optional: where an ingredient waits, and hooks on each step and the finish |
| `bind({scope, id, calm})` | engine | Called once at mount with the scene's id scoper |
| `sound` | engine | Which ambient sound set to play |
| `opts`, `setOpt(key, value)` | editor | The room's settings and the way to change one |
| `choices`, `tabs`, `looks`, `look(id)` | editor | The tray: each setting's values, the tabs they sit in, and whole-room presets |
| `pieces` | editor | Each piece: `kind` (tool or piece), `sel` (its SVG selector), `lock` (why it stays put), `fixed` (actions it always has), `uses`, `light`, `kit` |
| `kit`, `builtins`, `glows` | editor | Optional decor a Director can add, the skin's own actions offered for `b:`, and the glows that follow a dragged light |
| `thumb(key, value)` | editor | The URL of a tray picture |
| `destroy()` | engine | Releases the sky and any object URLs |
| `mounted(api)` | engine | Optional: called once the scene's elements exist, before the first recipe, for a skin with layers or drags of its own. `api` is `{P, svg, stage, root, calm, state, tween, later, say, sfx, rain, rerender}` |
| `down`, `move`, `up` `(e, pt)` | engine | Optional: a skin's own drag gets the pointer first and returns true when it took it (the journal's signet) |
| `onReset(S)`, `onFeedback(grade, x, y, S)` | engine | Optional: after a recipe is set up, and on every graded moment (the journal blots a miss) |
| `onDone(S, fin)` | engine | Optional: returning true means the skin finishes in its own time and calls `fin()` (the journal waits for its seal) |
| `turnTo(go)` | engine | Optional: animates a change of recipe and calls `go()` when the new one should appear (the journal turns the page) |

The skin dispatches `craft:redrawn` on the scene's `<svg>` after it redraws
the room, so the editor can re-find its pieces.

## The journal

`CraftJournal.create(opts)` takes the desk's `sky`, `thumbs` and `info`, plus
`look`, a saved set of its choices. What differs from the desk:

- **The page is baked as one picture.** Desk, binding, paper, wear and props
  are a standalone SVG document turned into an image (`data-bake="page"`), so
  it carries no fonts or CSS variables; ink, washes and moving parts are live
  over it, coloured by CSS variables on the stage (`--pp`, `--ink`, `--hand`,
  `--seal`, …). Paper, binding, wear and wax colour re-bake; ink and hand don't.
- **Light is one plain canvas over the scene**: ambient, the candle's pool and
  the window panes thrown across the left page, plus the glow. It is worked out
  as a multiply, then turned into a see-through shade that matches the multiply
  over the paper and the desk, because a blend mode over the whole scene costs
  a full extra pass every frame. It redraws at 30 a second, and the blurred
  window panes are drawn once per colour.
- **Writing.** Anything new on the page is written by the quill: `wr()` and
  `wtext()` wrap it in a clip that the quill reveals. What was on the page when
  it opened is simply there. Under Calm everything is simply there.
- **The calendar is read as players see it** (`?audience=players`, and moons
  with `hidden_from_players` are skipped anyway): it is a player's book, so a
  Director previewing it sees no secret moon. The date line, up to three moons
  and the window's weather and hour come from it.
- **Fonts** are self-hosted: Kalam and IM Fell from `static/fonts/customize/`,
  the other hands from `static/fonts/crafting/`.
- Its CSS is `JOURNAL_CSS` in `craft_bench.js`, every rule under `.crj` (the
  class the journal puts on its stage), resetting the desk's dark-room colours
  on the parts both skins share.

## The quality bar

The desk was signed off at this level; a new skin meets it before it ships.

- **One light.** Every highlight faces it and every shadow runs away from it.
  Its light falls off with distance into the room's ambient colour.
- **No point lights.** `fePointLight` and `feSpotLight` compute in half
  precision on many phone GPUs and overflow, so the lit layer renders black or
  as grey stripes. Light with `feDistantLight` instead, normalised so the
  surface under the light comes out at the same brightness, and draw the
  falloff as a radial gradient over the lit layer. Keep the lighting colour at
  half strength and double it afterwards, so nothing clips inside the filter.
  The desk's `sun`, `fall`, `half` and `sheen` helpers do all of this; the
  browser test fails if a point light appears.
- **Bake the expensive layers.** Plaster, wood and the book are lit by SVG
  filters once, then replaced by images (`[data-bake]`), so a frame costs only
  the moving parts. A setting that changes the light re-bakes.
- **Glass looks like glass**: a rim, a highlight facing the light, the liquid
  seen through it, a caustic on the bench.
- **The window is the calendar's sky** (`SkyPane.Scene`), painted from the
  scene's frame loop, so it stops off screen and holds still under Calm. Its
  paint time is measured, and a device too slow for the painted sky falls back
  to the basic one.
- **No blend modes over the whole scene.** A `mix-blend-mode` layer the size
  of the stage makes the browser composite the scene twice every frame; on the
  journal it cost a third of the frame rate. Blend inside a baked picture, or
  draw a plain see-through layer that looks the same.
- **Targets are what the hint names.** When the hint says "click the flask",
  anywhere on the flask counts (`onTarget`), not just the circle the engine
  aims at.
- **Phones**: the scene keeps its 12:7 shape and the build card drops below
  the scene under 720px.

## Tray pictures

Each tray picture is cut from the real scene with one choice made, so the tray
shows exactly what a choice does. After changing how a skin draws, redraw
them:

```
node tools/craft-thumbs.mjs                  # every desk picture
node tools/craft-thumbs.mjs cat look         # only the renders that cut those
node tools/craft-thumbs.mjs --skin=journal   # the journal's pictures
```

It needs playwright-core and Chromium, like the browser tests, and writes
`static/img/crafting/<skin>/<key>-<value>.jpg`, `kit-*.jpg` and `look-*.jpg`.
The journal's seal and signet pictures are cut from a recipe it has really
finished and sealed.

## Tests

- `test/js/craft_scene.test.mjs`: the engine's rules in a vm (steps, the
  verdict, cleaning, the scoper, the action lists).
- `test/js/craft_bench_browser.test.mjs`: the widget in Chromium (a step plays,
  two benches keep their ids apart, build mode and a look, every tray picture
  exists, destroy, no point lights), and the journal (a step plays beside a
  desk with ids apart, the players' calendar with no hidden moon, the seal, a
  blot staying with its recipe, its tray pictures). Shares `test/js/_craft_harness.mjs` with
  the picture tool.
