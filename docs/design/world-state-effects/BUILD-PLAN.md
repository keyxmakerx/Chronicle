# C-CAL-WORLDSTATE-EFFECTS-SYSTEM — synced world-state animation (Almanac sky-band + hourglass time-piece)

**Target:** Chronicle AI (Chronicle Dev)
**Type:** Multi-wave implementation. Ports the design-session prototypes into the real `/demo` and builds the shared world-state animation system.
**Supersedes:** the AI-asset sun approach in `C-CAL-SHOWCASE-DESIGN-1-ALMANAC-FIDELITY-V5-SUN-PROTOTYPE.md` (see §SUPERSESSION).
**Builds on:** Almanac refinement v4 (the shared canvas particle engine) + v5 (3-layer sun composition), both already on `claude/setup-working-memory-vROh3` (commits `01c1acb`, `0330ccb`).

---

## ⚑ START HERE (read before building — these survive context compression)

Everything you need is committed in the repo at **`docs/design/world-state-effects/`**:
1. **`README.md`** — index + locked decisions + architecture note. Read first.
2. **`CATALOG.md`** — THE SPEC. The shared-registry model (Part 0), the `worldState` data shape (Part 8), the canonical weather taxonomy (Part 3), celestial events (Part 4), DM time-control behaviors (Part 6), and the ~140-effect library (Part 11). This is your source of truth for *what* to build.
3. **`prototypes/*.html`** — open these in a browser. They are working code you port from, not throwaway. Headed by **`hourglass-meteor-daynight-mockup.html`** — the architecture exemplar.

If anything in this dispatch and the committed `CATALOG.md` disagree, **CATALOG.md wins** (it's the operator-reviewed artifact).

---

## GOAL

One `worldState` object drives BOTH surfaces in sync:
- the calendar **sky-band** (wide panoramic — clouds, god-rays, sun arc, the rich one), and
- the standalone **hourglass time-piece** (two chambers + physics sand).

Change the world state once (a moon goes red, a meteor shower starts, the DM hits pause) → both surfaces react. That's the deliverable. The existing `WEATHER_EFFECTS`/`CELESTIAL_EFFECTS` registries (from refinement v2/v4) are the seed; this grows them into the unified world-state registry.

---

## SUPERSESSION — read carefully

The fidelity-v5 dispatch had the sun rendered from **AI-generated painted assets**. **That is abandoned** (operator budget + the icon path is better). The locked approach now:

- **Sun = `lorc/sun.svg`** from game-icons.net (CC-BY 3.0; one line in a `CREDITS.md` covers it). Recolor per-state via CSS `filter`; layer CSS-animated rays/glow + the canvas bloom on top. (See `prototypes/icon-curation-v5.5.html` — `lorc/sun.svg` is the locked pick; `lorc/eclipse.svg` for solar eclipse.)
- The v5 painted-`<picture>` machinery + placeholder WP2/PNG assets: **rip out**, replace with the inline SVG icon + CSS/canvas layers. Keep the v5 *state-resolution* logic (`resolveSunState`) and the `sun-bloom` canvas entry — those are good.
- Fetch the icon SVG once and **inline it** (or vendor it under `static/vendor/game-icons/`); do NOT hot-load from a CDN at runtime (page-separation + offline).

---

## ARCHITECTURE (build this spine first)

### The shared model
```
worldState = {
  timeOfDay: 0..1,
  season: 'spring'|'summer'|'autumn'|'winter',
  sun: { tint },
  moons: [ { namedPhase, cyclePct, tint, size, orbitSpeed, orbitOffset }, ... ],  // 0..N dynamic
  weather: { type, intensity },
  events: [ { type, params }, ... ],          // can stack
  moodTint: { color, intensity },
  timeControl: { direction:+1|0|-1, speed },  // the DM verb layer
}
```

### The registry
Each effect is one entry declaring optional per-surface renderers:
```
EFFECTS['blood-moon'] = {
  id, name, category, tier,
  skyBand:  fn,   // paint into the calendar sky canvas/SVG
  hgTop:    fn,   // hourglass top chamber
  hgBottom: fn,   // hourglass bottom chamber
  hgSand:   fn,   // returns/sets sand color
  timeline: fn,   // (future) Tuner axis overlay — leave as optional hook
}
```
Adding any of the ~140 catalog effects later = one entry + its renderFns. No refactor. That property is the whole point — preserve it.

### Render resolution order (back→front, both surfaces)
1. time-of-day base sky → 2. season palette → 3. celestial bodies (sun, moons[], stars) → 4. weather → 5. events → 6. mood-tint wash → 7. **time-control modifier** (direction/speed/pause-float) applied to every animation's time delta.

### Reuse, don't rebuild
- **Particle engine:** v4 shipped `window.CalParticleEngine` (single rAF, pooled, capped, reduced-motion static-frame, perf-budgeted). ALL particle effects (rain, snow, meteors, splash, sand grains) are emitters on THAT engine. Do not spin up parallel rAF loops.
- **Hourglass sand:** the cheap **slope-limited heightmap** model (column-height array + avalanche pass at ~30° angle of repose; falling grains are a tiny pooled particle set that deposit on landing). Working code in `prototypes/hourglass-meteor-daynight-mockup.html` and `hourglass-states-gallery.html` — port it.
- **Hourglass rendering pattern:** canvas-driven interior + SVG glass shell on top (frame, gloss, rim). See the mockup.

---

## WAVES

### WAVE 0 — Foundation (the spine)
**Do this before any effect.**
- Refactor the existing `WEATHER_EFFECTS` + `CELESTIAL_EFFECTS` into the unified `EFFECTS` registry with the per-surface renderer shape above.
- Introduce a single `worldState` object + a `setWorldState(patch)` that re-renders subscribers.
- Wire the Almanac sky-band AND a new hourglass module to both subscribe to `worldState`.
- Implement the layered render-resolution order.
**Gate:** changing `worldState.timeOfDay` (or a test weather) visibly updates BOTH surfaces from one call. No regressions to the shipped Almanac.

### WAVE 1 — Hourglass core + sun (the "v5.5")
- **Rip out** the AI-painted sun; install `lorc/sun.svg` + CSS recolor/rays/glow + `sun-bloom` canvas layer (keep `resolveSunState`). 5 states: default / dawn / dusk / eclipse(`lorc/eclipse.svg`) / special.
- **Port the glass hourglass** from the prototypes into the real Almanac time-piece slot: dark-wood frame (feTurbulence grain + feDiffuseLighting bevel), procedural glass (feSpecularLighting gloss, cyl edge-shading, rim, grit), horizontal **shelf** form factor (standing hourglass, time/date flanking — already the v4 shelf).
- **Port the heightmap sand** (canvas, fed by the neck stream, avalanche pile) + the dawn/dusk flip.
- **Bottom chamber = day/night** driven by `worldState.timeOfDay` (sun arcs + sets behind the sand horizon, stars emerge) — see the mockup.
**Gate:** hourglass renders live in `/demo/calendar/almanac` with working sand + sun + day/night, wired to `worldState`. Headless screenshots of: default, dawn, dusk, eclipse, special, sand mid-flow, reduced-motion static frame. Idle CPU < 5% (report the number).

### WAVE 2 — MUST effects across both surfaces (the "v6 core")
Implement these as registry entries with renderers for the relevant surfaces (see CATALOG Parts 3/4 + tier ⭐):
- **Time-of-day, sun (+tint), moons with named phases** (per-moon named-phase vocabulary — see CATALOG Part 2; mock Selûne's 8 / Shar's 3).
- **Weather:** clear, cloudy, rain, fog, thunderstorm (lightning flash), snow.
- **Celestial events:** meteor shower (+ hg sand-splash — already prototyped), solar eclipse (corona/diamond-ring/midday-dark/stars-emerge), lunar eclipse, blood moon (+ hg blood-drip), aurora (rippling curtains).
- **Player mood-tint** (global color wash, intensity).
**Gate:** the demo-controls panel (from v4) drives every MUST effect live on both surfaces. Headless screenshots per effect on each surface. Multi-effect stack proof (rain + meteor + mood-tint). CPU budget held.

### WAVE 3 — Time-control verb layer (CATALOG Part 6)
- `worldState.timeControl` modulates everything: **play** (forward), **pause** (sand grains freeze + gentle float-bob; sun/clouds freeze), **rewind** (sand falls UP / pile un-builds / sun reverses arc / clouds drift back), fast-forward, step.
- Implement as a global `{direction, speed}` multiplier on animation time deltas + a special pause-float mode for sand.
**Gate:** the demo controls expose ▶ ⏸ ◀ ⏩ and they visibly do the right thing on both surfaces. Reverse-sand + pause-float demonstrated. (Reverse-sand fidelity — see OPEN QUESTIONS; ship the simpler "stream reverses + pile shrinks" first if the full un-build is heavy, and flag.)

### WAVE 4 — SHOULD effects (CATALOG tier ✅)
Blizzard, sakura-bloom, ashfall, comet, supermoon, moon-special, volcanic, snow-flurries, hail, windy, god-rays, drifting-cloud parallax (sky-band), + the hg set-pieces (frost-crawl, ember-fall, petal-settle). Each = registry entry + renderFns. No architecture change.
**Gate:** each lands behind the demo-controls toggles with a screenshot.

### WAVE 5 — NICE/EXOTIC long tail (CATALOG tier ◐/✨, incremental, low priority)
Eldritch-ink/void set, divine/infernal, necromantic/plague(+ "gas"), planar/temporal, tornado/sand-tornado, optical phenomena (rainbow, sun-dogs), cosmic (milky-way, nebula), shattered-moon, etc. Pull from the catalog on demand. These are "nice to have," shipped opportunistically.

---

## BINDING CONVENTIONS (carry from the design canon — non-negotiable)
- **Self-contained CSS** under `.cal-*` prefixes in the existing demo CSS. No `@layer`, no `@apply`, no `--chronicle-*` token reads. Hardcoded OKLCH literals.
- **Externalized JS** in the existing demo JS files; INIT_BLOCKS try/catch with flag-after-success. No inline `<script>` logic (note the v5 fix: templ does NOT interpolate `{ }` inside `<script>` — use a `data-*` attribute + `getAttribute`).
- **One shared rAF** (the v4 particle engine). Pooled + capped particles. `IntersectionObserver` + `visibilitychange` pause. **`prefers-reduced-motion` → one static representative frame, no animation.** DPR clamp ≤2×. **Idle CPU > 5% on a mid laptop = cut the pool; report the measured number with screenshots.**
- **Page-separation:** `/demo/calendar/almanac` loads only its own assets. Demo-controls panel is showcase-only (never production templ).
- **Filters are static** (computed once) — only particles/positions animate per frame.
- **Mock-data only.** No backend/DB. World-state comes from a mock today; real campaign data is Wave-4-widget-framework territory (post-deadline) — do NOT wire it.
- **SVG filters** for glass/wood (feTurbulence/feDiffuseLighting/feSpecularLighting) per the glass-fidelity prototype — they're the realism levers, and cheap because static.

## VERIFICATION GATE (every wave)
Headless-browser screenshots delivered to operator in chat (GitHub API can't embed binaries in PR bodies). Plus the idle-CPU number. Plus reduced-motion proof. Existing tests green; `go test ./...`, `make lint`, `node --check`, `templ generate` all clean. New tests per wave (registry wiring, state resolution, reduced-motion, particle caps).

## MOCK DATA NEEDS
- `worldState` mock seed + a few authored days with varied weather/events/moons (reuse the `DayWeather` map pattern + `SpecialMoonDays` from prior dispatches).
- Per-moon named-phase tables (Selûne 8 / Shar 3 from the FR mock).
- Demo-controls panel extended to mutate `worldState` live: time-of-day scrubber, weather dropdown, event toggles, add/tint moons, mood-color picker, and the ▶⏸◀⏩ time-control buttons.

## OPEN QUESTIONS (gate parts of Waves 3-5; Waves 0-2 can proceed now)
These are pending operator answers — do NOT block Waves 0-2 on them:
1. **"Gas"** — likely = `plague`/`miasma` (green). Confirm before building a distinct one.
2. **Weather-vs-event overlap** — arcane/aurora exist as both a weather and an event in the catalog; confirm keep-both vs merge.
3. **Reverse-sand fidelity** (Wave 3) — full pile-un-build, or simpler stream-reverse + pile-shrink? Ship simpler first, flag.
4. **Pause-float intensity** — subtle suspended-bob vs dramatic zero-g drift.
5. **Moon max count** — perf/clutter cap, and default for a new world.
6. **MUST/SHOULD/NICE priority** — confirm the Wave 2 MUST set is the right first-ship.

## STOP-AND-FLAG
- If the worldState refactor risks regressing the shipped Almanac sky-band, flag before proceeding — Wave 0 must not break v4/v5.
- If reverse-sand or pause-float fights the heightmap model, flag with the tradeoff rather than forcing it.
- If CPU budget can't hold with multiple stacked effects, flag — we throttle pools, not drop reduced-motion safety.
- If a catalog effect needs a new engine capability (not just a renderFn), flag — the registry should stay "data + renderFn."

## SEQUENCING
Waves 0→1→2 are the spine and should land in order (each gated by screenshots → operator sign-off). Wave 3 (time-control) after 2. Waves 4-5 are incremental and can interleave / be pulled on demand. Each wave = its own PR against `main` (or stacked on the working branch), screenshots to operator, sign-off, next.

## CROSS-REFERENCES (in-repo, compression-proof)
- `docs/design/world-state-effects/README.md` + `CATALOG.md` + `prototypes/` — the source of truth.
- `C-CAL-SHOWCASE-DESIGN-1-ALMANAC-REFINEMENT-V4.md` — the particle engine + shelf timepiece + demo-controls this builds on.
- `C-CAL-SHOWCASE-DESIGN-1-ALMANAC-FIDELITY-V5-SUN-PROTOTYPE.md` — SUPERSEDED for the sun asset approach; state-resolution logic still valid.
- `C-TIMELINE-V2-DESIGN-1-TUNER.md` — will reuse this registry + engine for the timeline `timeline` surface hook (leave the optional hook in place).
