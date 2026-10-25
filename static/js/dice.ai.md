# dice.js and dice/

Thrown 3D dice for any roll on the site (`window.Chronicle.dice`).

**Why it is split this way.** Rolling is a game rule; drawing is a show. `dice.js`
decides every value and every crit before anything is drawn, so a roll made with
motion Off, under Calm, or in a browser without WebGL gives the same numbers a
throw would show. `dice/engine.js` only shows them.

**Loading (ADR-063).** `boot.js` puts a stub `Chronicle.dice` on every page. The
first `roll()` or `catalogue()` fetches `dice.js` from the hashed URL on the
layout's `#chronicle-dice-src` element. `dice.js` then imports
`/static/js/dice/engine.js`, and three.js and cannon-es load on the first throw.
A set or crit file loads only when a roll needs it. Nothing else adds to the page
weight that `TestEveryPageScriptBudget` counts.

**Files**
- `dice.js`: the API. It decides values with `crypto.getRandomValues`, applies
  the crit rules, picks each die's look and announces the result to screen
  readers (`#chronicle-dice-live`). It waits up to 4 s for the engine, the
  libraries and the set and crit files; past that, the roll goes on without its
  dice.
- `dice/engine.js`: an ES module that holds the physics, the table and the
  Jewelled, Confetti glass and Acrylic sets. It also holds all six shows (the way
  a landed number lights up), the crit rules, quality and the catalogue.
- `dice/sets/<key>.js`: one file per set, for the other nine sets. Each calls
  `registerSet`.
- `dice/crits/<key>.js`: one file per crit. Each calls `registerCrit`.

**Module identity.** Set and crit files `import … from '../engine.js'`. Every
importer must name the engine by the same bare URL, or the browser runs two
copies with two registries. That is why `dice.js` imports it with no `?v=`.
Unversioned static files are always revalidated. Only keys in `SET_KEYS` and
`CRIT_KEYS` are ever imported, and library URLs must be same-origin `/static`
paths. Adding a set or crit means adding its key to that list in `engine.js`.

**API** (full detail in the header of `dice.js`)
- `roll(groups, opts)` resolves `[{ label, values, crits }]`.
  - It resolves when the dice land, or at once for Calm, still and undrawn rolls.
  - `onSettle(points)` fires at the same moment, with where each die rests.
  - `onEnd()` fires once the dice have gone.
- A die can carry its own `value`, `set`, `colour`, `succ`, `fail`, `crit` and
  `counts`.
- `opts.crits(group, values, index)` is the game system's crit rule. Without one,
  a natural 20 or 1 on a counting d20 crits.
- `catalogue()` lists every set with its colours (swatch and glow), every crit
  (`what`, `echoWhat`) and `defaults`, for pickers. `defaults` is also on the
  loaded `Chronicle.dice`; the stub only has `catalogue()`.
- `stop()` clears the table. Called before the files arrive, it still stops the
  rolls already asked for.
- `roll()` rejects only when a roll is refused (an unknown die, an impossible
  value) or `dice.js` itself can't load (the stub gives up after 10 s).

**Defaults:** Jewelled set, onyx colour, Phoenix crit success, Eldritch grasp
crit fail, quality Auto.

**Crits in one throw.** The grand plays on one crit die. A die that counts goes
before one that doesn't, a success before a failure, then the die nearest the
middle. Other crits of the same kind echo just after the grand. Crits of the other
kind play their table's echo after the grand's peak.

**Motion and skipping**
- `html[data-motion]` (set by `theme.js`) decides how much moves. Under `calm` the
  dice are set down with the decided face up and fade in, with no throw and no
  crits.
- `off`, or the device's reduce-motion setting, draws nothing.
- A press (pointer or key, never typing in a field) skips the animations, not the
  result: the dice stay 3 s with their numbers lit. A second press clears them.

**Quality**
- Auto starts from hints (a software renderer or small device is Basic) or the
  cached device test. It steps down one costly layer at a time when frames run
  long.
- The device test runs once, when the page is idle after the first throw. It is
  skipped where hints already say Basic, and stops on the first slow frame.
- `{ capBasic: true }` holds a table to Basic.

The canvas is fixed over the page at z-index 900, under menus and toasts. It has
`pointer-events: none` and `aria-hidden`. A frame that throws ends the roll and
clears the table.

**Memory.** Each throw gets its own renderer, freed when the dice go. Objects every
table reuses (face images, common shapes and materials) are made with `share()`.
three.js ties each object it draws to the renderer that drew it, so `disposeStage`
lets the shared ones go once no table is left, and the next table uploads them
again. The face-image cache keeps the 24 most recent.

**Changing the libraries:** `make dice-vendor` (versions pinned in `package.json`).
Tests: `test/js/dice.test.mjs` covers values, crit rules, numbering, symmetry and
landing. `test/js/dice_browser.test.mjs` runs Chromium: loading, throw, skip,
catalogue, and every set playing; `dice_crits_succ_browser.test.mjs` and
`dice_crits_fail_browser.test.mjs` play every crit (their own files so CI runs
them on runners of their own; they play in real time).
