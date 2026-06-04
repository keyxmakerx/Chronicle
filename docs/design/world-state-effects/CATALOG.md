# World-State Effects Catalog — Calendar Sky-band + Hourglass Time-piece (shared registry)

**Purpose:** one registry of world-state effects drives BOTH surfaces — the calendar's panoramic sky-band AND the standalone hourglass time-piece (and, later, the timeline). Each effect declares how it renders on each surface; some are no-ops on a given surface. This is the "sync" — change the world state once, both surfaces react.

---

## PART 0 — The shared model

**Surfaces** (render targets an effect can paint into):
- `skyBand` — the calendar's wide panoramic sky (has clouds, god-rays, horizon — the rich one)
- `hgTop` — hourglass top chamber (the "events / incoming" scene)
- `hgBottom` — hourglass bottom chamber (the "time of day / now" scene)
- `hgSand` — the sand color/material itself (drains from top to bottom)
- `timeline` — (future) the Tuner axis overlay

**Each registry entry declares optional renderers per surface**, e.g.:
```
'blood-moon': {
  skyBand:  drawRedMoonInSky,
  hgTop:    drawBloodSeepFromTop,
  hgBottom: null,            // no-op here
  hgSand:   tintSandCrimson,
  intensity: 0..1,
}
```

**Render resolution order (precedence, back→front):**
1. Time-of-day base (sky gradient, sun arc, star density)
2. Season palette shift
3. Celestial bodies (sun, moons[], stars) — *dynamic count*
4. Weather layer
5. Celestial / special events
6. Player mood-tint overlay
7. **Time-control modifier** (direction / speed / pause-float) — modulates everything above

---

## PART 1 — Continuous states (always have a value)

| State | Dynamic? | skyBand | hgBottom | hgSand | Notes |
|---|---|---|---|---|---|
| **Time of day** (0–1) | yes (clock) | full sky gradient + sun arc across width | bottom-chamber sky cycle + setting/rising sun | — | dawn→morning→midday→afternoon→dusk→twilight→night→deep-night |
| **Season** | yes | palette warmth, daylight length, sun height | subtle palette tint | — | spring/summer/autumn/winter |
| **Stars / constellations** | yes (by darkness) | fade in at night, twinkle, optional constellations | fade in at night in bottom chamber | — | density scales with darkness |

---

## PART 2 — Celestial bodies (DYNAMIC — variable count + per-body params)

> These are the big "dynamic" ones you flagged. The world can have **any number of moons**, each independently parameterized. Players/DMs add/configure them.

**Sun** — one. Params: `{ tint }`. Normal gold; can be tinted (red sun, pale winter sun, sickly green). Position derived from time-of-day.

**Moons[]** — array, **0..N**. Each moon: `{ phase, namedPhase, tint, size, orbitSpeed, orbitOffset, cyclePct }`.
- skyBand: each moon drawn at its phase + tint, moving on its own arc
- hgBottom: moons can appear in the night-sky portion of the bottom chamber
- A **tinted moon** (e.g., red) is just a moon with `tint` set — not a separate event. (Your "red mood / tinted moon" = a moon param.)
- **NAMED PHASE VOCABULARY (dynamic, per-moon)** — restored from canon. Phases aren't just geometric; each moon has worldbuilding-named spans keyed by `start_pct`/`end_pct` (0–100). The day-popover reads *"The Silver Crown — 56%"*, not a raw number. Example from the FR mock: **Selûne has 8 named phases** ("The Dark Sister", "The Growing — early/middle/late", "The Silver Crown", "The Fading — early/middle/late"); **Shar has 3**. `moonGlyphFor()` walks named spans first, falls back to procedural only if none covers the day. So a "moon" entry also carries its own named-phase table — fully data-driven per world.

**Player mood-tint** — see Part 5 (global, not a body, but lumped near here because "red mood" came up).

---

## PART 3 — Weather (CANONICAL: 11 named types in 4 categories + proposed additions)

> Restored from canon: weather is **operator-authored per day** via a `DayWeather` map keyed "Y-M-D" (not procedural — DM assigns specific weather to specific days; procedural pip fallback only for unauthored days). Each type carries `id / name / category / icon (inline SVG) / color (OKLCH) / temp_c`. **Chip styling differs per category** so they're distinct at a glance: Standard = solid color · Severe = glow-shadow · Environmental = dashed border · Fantasy = solid accent border.

### Canonical 11 (the locked vocabulary)

| Category | Type | skyBand (rich) | hgTop/hgBottom | hgSand |
|---|---|---|---|---|
| **Standard** | Clear | open sky, drifting wisps | clean | golden |
| | Cloudy | parallax cloud layers + shadows | dimmer | grey-gold |
| | Rain | rain across band, dim sky | faint rain | blue droplets |
| | Fog | low haze, muted sun | grey haze tint | grey, low-opacity |
| **Severe** | Thunderstorm | rain + **lightning flashes** + dark clouds | flash sync | blue + flash |
| | Blizzard | heavy snow + wind streaks | dense white | white, fast |
| **Environmental** | Sakura Bloom 🌸 | drifting pink petals, soft warm sky | petals in chambers | pink-tinged |
| | Ashfall | grey falling ash, dim red sky | grey-red flecks | grey embers |
| **Fantasy** | Arcane Winds | shifting prismatic streaks in air | prismatic drift | prismatic shift |
| | Ley Surge | ground-up pulsing glow | pulsing glow | glowing |
| | Acid Rain | green-tinted rain, eerie sky | green droplets | green |

### Proposed additions (beyond the canonical 11 — confirm if wanted)

| Type | Notes |
|---|---|
| **Snow** (light) | distinct from Blizzard — gentle drifting snow (was in MUST renderFn list) |
| **Overcast** | flat grey full-cover (between Cloudy and storm) |
| **Windy** | fast clouds, bent god-rays, debris motes |
| **Hail** | fast white pellets |
| **Sandstorm** | brown haze sweeping |
| **Gas / Miasma** ⚠️ | YOUR "gas" — drifting toxic haze + rising bubbles. *Confirm intent: poison/plague-green? volcanic sulfur-yellow? swamp? This may belong with the Plague event instead — see Part 4.* |
| **Heatwave** | shimmer + bleached sky (summer) |

> **skyBand-only weather extras:** multi-layer drifting clouds + shadows, god-rays/crepuscular sunbeams, rainbow after rain, heat shimmer. Hourglass renders weather mostly as **sand tint + a few in-chamber particles**.

---

## PART 4 — Celestial / special EVENTS (transient, overlay on top)

> Restored from canon (`CELESTIAL_EFFECTS` registry). **MUST tier** = meteor shower + eclipse. **Canonical TBD set** (architectural hooks now, full visuals later) = volcanic / ice age / plague / arcane events / moon-special / aurora / comet. Operator additions this session = blood moon / supermoon / conjunction / shooting star.

| Event | Tier | skyBand | hgTop (dramatic chamber scene) | hgSand |
|---|---|---|---|---|
| **Meteor shower** | MUST ✅ | streaks raking the sky | meteors fall + **splash into sand** + flash (BUILT) | golden + streaks |
| **Solar eclipse** | MUST | sun darkens, corona ring, sky → dusk | dark disc, dimmed chamber | silver-dark |
| **Lunar eclipse** | MUST | moon reddens/darkens | red moon glow in bottom | dim |
| **Volcanic** 🌋 | TBD-canon | red-orange glow + ash plume | ember glow, ash flecks | red-orange embers |
| **Ice Age** ❄️ | TBD-canon | frosted overlay + colder palette | frost creep on glass, blue wash | pale frost-blue *(note: climate-scale, may persist across many days)* |
| **Plague** 🦠 | TBD-canon | green-tinted mist over sky | **green seep/miasma in chamber** | sickly green *(your "gas" may live here)* |
| **Arcane events** ✨ | TBD-canon | shifting prismatic streaks | prismatic shimmer / reality-warp | prismatic shift |
| **Moon-special** 🌙 | TBD-canon | per-moon highlight/halo (a specific moon's special night) | that moon featured in bottom | tint to moon color |
| **Aurora** | TBD-canon | **rippling color curtains** (green/violet) along upper sky | shimmering curtains in chambers | prismatic shimmer |
| **Comet** | TBD-canon | slow bright comet + long tail, multi-day | comet arcs through top | golden |
| **Blood moon** 🔴 | operator-new | large red moon, red-tinted sky | **blood seeps/drips from top of chamber** (your idea) | crimson |
| **Supermoon** | operator-new | oversized bright moon | bright moon, silver wash | bright silver |
| **Conjunction** | operator-new | ≥2 moons align | aligned moons in bottom | — |
| **Shooting star** | operator-new | single occasional streak | single meteor | — |
| **Planar rift / Wild magic** | NICE | tear of swirling color, distortion | reality-warp, color static | prismatic chaos |

> **Overlap note:** "Arcane Winds / Ley Surge / Acid Rain" live in WEATHER (Part 3, Fantasy category); "Arcane events / Aurora" live here in EVENTS. They're related but distinct registry entries (a persistent weather vs a transient event). Worth confirming we want both, or merging some.
> **Stacking:** events can stack with weather (rain + meteor). Default precedence: **event wins the dramatic layer; weather still tints sand; mood-tint washes over all.** (Confirm.)

---

## PART 5 — Player mood-tint (DYNAMIC, arbitrary color)

A global color overlay both surfaces, `{ color, intensity }`. This is your "add a red mood" — the DM picks a color (or a named preset) and the whole scene tints toward it.

- Presets: ominous red · eerie green · melancholy blue · festive gold · cursed violet · holy white
- Applies as a screen/multiply wash over skyBand + both hg chambers + a shift on sand
- Independent of weather/events (you can have a clear day with a "dread red" mood)
- Intensity slider (subtle → heavy)

---

## PART 6 — DM TIME-CONTROL behaviors (the verb layer — your floating/reverse idea) ⏯️

> These modulate **everything** globally. They're tied to the DM's playback controls (play / pause / rewind / fast-forward), same controls that scrub the calendar.

| Control | Hourglass behavior | Sky-band behavior |
|---|---|---|
| **▶ Play (forward)** | sand falls normally, pile builds | time advances, sun/clouds/moons move forward |
| **⏸ Pause** | **sand FREEZES & floats** — mid-air grains hover, top sand lifts slightly, stream suspended, gentle bob/shimmer ("suspended in amber") | sun/clouds/moons freeze; subtle ambient float only |
| **◀ Rewind / reverse** | **sand falls UPWARD** — grains flow back up through neck, pile un-piles into top, stream reverses | sun reverses arc (sets in east), clouds drift backward, star/event anims reverse |
| **⏩ Fast-forward** | sand accelerates, pile builds fast | time + all motion accelerate |
| **⏭ Step** | jump by increment (day/hour) | snap to next/prev step |
| **■ Stop / reset** | sand resets to start of period | clock resets to period start |

**Implementation note:** this is a global `{ direction: +1/0/-1, speed }` multiplier on every animation's time delta, plus a special **pause-float mode** (grains get tiny buoyancy + bob instead of gravity). Reverse = negate velocity / run the heightmap deposit in reverse (un-deposit from pile back into stream going up).

---

## PART 7 — Surface-specific extras

**skyBand ONLY (the subtle animated differences you mentioned):**
- **Drifting clouds** — multiple parallax layers, with cloud shadows
- **Crepuscular rays / god-rays / sunbeams** — animated shafts through clouds (you called this out)
- **Horizon glow / landscape silhouette**
- **Birds / flocks** (optional ambient)
- **Rainbow** after rain; **heat shimmer** in summer-clear
- Sun/moons travel the **full width**; richer, larger canvas

**Hourglass ONLY:**
- **Two-chamber split**: top = events scene, bottom = time-of-day scene
- **Sand color + physics** (fall, pile, avalanche) — the medium carrying world-state
- **The flip** at period boundary (dawn/dusk) — re-orients chambers
- **Glass caustics / refraction** of the chamber content
- The dramatic per-event scenes (meteor splash, blood seep) live here

---

## PART 8 — Dynamic parameters summary (the data shape)

```
worldState = {
  timeOfDay: 0.0–1.0,
  season: 'spring'|'summer'|'autumn'|'winter',
  sun: { tint },
  moons: [ { phase, tint, size, orbitSpeed, orbitOffset }, ... ],   // 0..N — DYNAMIC
  weather: { type, intensity },
  events: [ { type, params, startsAt, duration }, ... ],            // can stack
  moodTint: { color, intensity },                                   // player overlay
  timeControl: { direction: +1|0|-1, speed },                       // DM playback
}
```
Both surfaces subscribe to `worldState`; when it changes, both re-render. That's the sync.

---

## PART 9 — Build tiering (proposed — needs your priority pass)

> Reconciled with canon. The original MUST/TBD split for the *registry stubs* still holds; this tiering is about *full animated visuals* for the synced hourglass+sky-band system.

**MUST (first build):** time-of-day, sun (+tint), 1+ moons w/ named-phase + tint, the 4 Standard weathers (clear/cloudy/rain/fog) + Thunderstorm + Snow, meteor shower, solar+lunar eclipse, blood moon, aurora, mood-tint, **play/pause/rewind** (the verb layer). Proves the whole system + your specific asks.

**SHOULD:** Blizzard, Sakura Bloom, Ashfall, comet, supermoon, moon-special, volcanic, drifting-cloud parallax, god-rays, fast-forward/step.

**NICE / later:** Arcane Winds, Ley Surge, Acid Rain, plague/gas, ice age, conjunction, planar rift, wild magic, windy, hail, sandstorm, heatwave, birds, rainbow, heat shimmer.

---

## PART 10 — Open questions for you

1. **"Gas"** — confirm what you mean: it may already be covered by **Plague** (green miasma, Part 4) or **Volcanic** (sulfur gas). Or is it a distinct toxic/swamp weather? (affects color + behavior)
0. **Weather vs event overlap** — canon has Arcane Winds/Ley Surge/Acid Rain as *weather* AND Arcane events/Aurora as *events*. Keep both as distinct entries, or merge? (e.g., is "aurora" a weather or an event in your head?)
0. **Named moon phases** — confirm we keep the per-moon named-phase vocabulary (Selûne's 8, Shar's 3 in the FR mock) as the model — i.e., each world authors its own moon names + phase spans.
2. **Stacking rules** — when an event + weather + mood-tint are all active, is "event wins the dramatic layer, weather tints sand, mood washes everything" right? Any combos that should be exclusive?
3. **Moons** — is there a sensible **max** (perf + visual clutter), or truly unlimited? Default count for a new world?
4. **Reverse sand** — when rewinding, should the pile physically un-build (grains lift from pile back up the stream), or is a simpler "stream reverses + pile shrinks" enough?
5. **Pause float** — how floaty? Subtle suspended-bob, or dramatic zero-gravity drift?
6. **Where does the calendar get its world-state?** Today it's mock. Eventually: real campaign data (current date → events on that date → weather → moon phases computed). That's the Wave-4 wiring; the registry is built mock-first.
7. **Priority pass** — agree with the MUST/SHOULD/NICE tiering, or move things?

---

---

# PART 11 — EXPANDED EFFECT LIBRARY (the big brainstorm)

> The full menu — canonical + a wide brainstorm so the registry has a deep catalog to draw from. Format: **`id`** label — short visual `[surfaces]` **tier**.
> Surfaces: `SB`=sky-band · `HT`=hg top chamber · `HB`=hg bottom · `SAND`=sand color. Tier: ⭐MUST · ✅SHOULD · ◐NICE · ✨EXOTIC(later).
> Nothing here is locked — it's the palette. We pick what ships per tier; the registry makes adding any of them "data + a renderFn."

### 11.1 — Clouds & precipitation (mundane)
- **`clear`** Clear — open sky, faint wisps `[SB·HB]` ⭐
- **`partly-cloudy`** Partly Cloudy — scattered drifting clouds `[SB]` ⭐
- **`cloudy`** Cloudy — parallax cloud layers + shadows `[SB·SAND]` ⭐
- **`overcast`** Overcast — flat full-grey cover `[SB]` ✅
- **`fog`** Fog — low haze, muted sun `[SB·SAND]` ⭐
- **`mist`** Mist — thin ground haze `[SB]` ✅
- **`haze`** Haze — soft atmospheric blur `[SB]` ◐
- **`drizzle`** Drizzle — fine sparse rain `[SB·SAND]` ✅
- **`rain`** Rain — steady rain across band `[SB·SAND]` ⭐
- **`heavy-rain`** Heavy Rain — dense rain, dark sky `[SB·SAND]` ✅
- **`downpour`** Downpour — sheeting rain + splash `[SB]` ◐
- **`thunderstorm`** Thunderstorm — rain + lightning flashes `[SB·HT·SAND]` ⭐
- **`lightning-storm`** Lightning Storm — frequent strikes, no rain `[SB]` ◐
- **`supercell`** Supercell — towering wall cloud, green sky `[SB]` ✨

### 11.2 — Cold & winter
- **`snow-flurries`** Snow Flurries — light sparse drifting flakes `[SB·SAND]` ✅ *(your ask)*
- **`snow`** Snow — steady gentle snowfall `[SB·SAND]` ⭐ *(the "boring" general one)*
- **`heavy-snow`** Heavy Snow — dense fall, accumulating `[SB·SAND]` ✅
- **`blizzard`** Blizzard — heavy snow + wind streaks + whiteout `[SB·SAND]` ✅
- **`whiteout`** Whiteout — near-total white obscurement `[SB]` ◐
- **`sleet`** Sleet — mixed rain/ice `[SB]` ◐
- **`freezing-rain`** Freezing Rain — glassy ice coating, sheen `[SB]` ◐
- **`hail`** Hail — fast white pellets, bounce `[SB·SAND]` ✅
- **`ice-storm`** Ice Storm — crystalline glaze + shimmer `[SB]` ◐
- **`frost`** Frost — creeping frost on edges/glass `[SB·HT]` ✅ *(frost crawls up the glass — great hg effect)*
- **`cold-snap`** Cold Snap — pale palette, breath fog, sharp light `[SB]` ◐

### 11.3 — Wind & violent storms
- **`breezy`** Breezy — gentle motion in clouds/leaves `[SB]` ◐
- **`windy`** Windy — fast clouds, bent god-rays, debris motes `[SB]` ✅
- **`gale`** Gale — strong wind streaks `[SB]` ◐
- **`tornado`** Tornado — funnel cloud, swirling debris, dark sky `[SB·HT]` ✅ *(your ask — and a sand-tornado in the hg top is sick)*
- **`hurricane`** Hurricane / Cyclone — spiral cloud band, driving rain `[SB]` ✨
- **`dust-devil`** Dust Devil — small spinning dust column `[SB]` ◐
- **`waterspout`** Waterspout — tornado over water `[SB]` ✨
- **`sandstorm`** Sandstorm — sweeping brown haze wall `[SB·SAND]` ✅
- **`haboob`** Haboob — massive dust wall `[SB]` ✨
- **`derecho`** Derecho — straight-line wind front `[SB]` ✨

### 11.4 — Optical / atmospheric phenomena
- **`rainbow`** Rainbow — arc after rain `[SB]` ✅
- **`double-rainbow`** Double Rainbow — twin arcs `[SB]` ◐
- **`fogbow`** Fogbow — pale white bow `[SB]` ✨
- **`sun-halo`** Sun Halo — ring around sun `[SB]` ◐
- **`moon-halo`** Moon Halo — ring around moon `[SB]` ◐
- **`sun-dogs`** Sun Dogs (parhelia) — bright spots flanking sun `[SB]` ✨
- **`light-pillars`** Light Pillars — vertical light shafts `[SB]` ✨
- **`crepuscular-rays`** God-rays — sun shafts through cloud `[SB]` ✅ *(the sunbeams you mentioned)*
- **`mirage`** Mirage — heat-shimmer false horizon `[SB]` ◐
- **`glory`** Glory — halo opposite sun `[SB]` ✨

### 11.5 — Environmental / biological
- **`sakura-bloom`** Sakura Bloom — drifting pink petals 🌸 `[SB·HT·SAND]` ✅
- **`blossom-fall`** Blossom Fall — white/colored petals `[SB]` ◐
- **`falling-leaves`** Falling Leaves — autumn leaves drift 🍂 `[SB·HT]` ✅
- **`pollen-drift`** Pollen Drift — golden floating motes `[SB]` ◐
- **`seed-drift`** Seed Drift — dandelion fluff `[SB]` ◐
- **`spore-cloud`** Spore Cloud — drifting fungal spores `[SB]` ◐
- **`fireflies`** Fireflies — glowing night motes ✨ `[SB·HB]` ✅
- **`locust-swarm`** Locust Swarm — dark insect cloud `[SB]` ◐ *(biblical/ominous)*
- **`bird-flocks`** Bird Flocks — V-formations, murmurations `[SB]` ◐
- **`bat-swarm`** Bat Swarm — dusk bat cloud `[SB]` ◐
- **`butterfly-drift`** Butterfly Drift — colorful flutter `[SB]` ✨

### 11.6 — Earth & fire
- **`ashfall`** Ashfall — grey falling ash, dim red sky `[SB·HT·SAND]` ✅
- **`volcanic`** Volcanic — red-orange glow + ash plume 🌋 `[SB·HT·SAND]` ✅
- **`ember-rain`** Ember Rain — drifting glowing embers `[SB·HT]` ✅
- **`wildfire-glow`** Wildfire Glow — orange smoke horizon `[SB]` ◐
- **`smoke`** Smoke — drifting grey haze `[SB]` ◐
- **`geyser-steam`** Steam — rising white vapor `[SB]` ✨
- **`earthquake`** Earthquake — screen shimmer/shake, dust `[SB·HT]` ✨

### 11.7 — Sun states
- **`sunny`** Sunny — bright clear sun `[SB·HB]` ⭐
- **`golden-hour`** Golden Hour — warm low-angle glow `[SB·HB]` ✅
- **`blue-hour`** Blue Hour — cool pre-dawn/post-dusk `[SB·HB]` ✅
- **`hazy-sun`** Hazy Sun — diffuse veiled sun `[SB]` ◐
- **`blood-sun`** Blood Sun — deep red disc, ominous `[SB·SAND]` ✅
- **`pale-sun`** Pale Sun — weak winter sun `[SB]` ◐
- **`green-sun`** Green Sun — sickly/cursed `[SB]` ✨
- **`twin-suns`** Twin Suns — two suns (alien world) `[SB]` ✨
- **`dying-sun`** Dying Sun — swollen red giant `[SB]` ✨

### 11.8 — Moon states (per-moon, dynamic)
- **`moon-phase`** Phases — new→crescent→half→gibbous→full (named per moon) `[SB·HB]` ⭐
- **`blood-moon`** Blood Moon — red moon + blood-seep in hg top 🔴 `[SB·HT·SAND]` ⭐
- **`blue-moon`** Blue Moon — cool blue tint `[SB]` ✅
- **`harvest-moon`** Harvest Moon — large orange autumn moon `[SB]` ✅
- **`supermoon`** Supermoon — oversized bright moon `[SB]` ✅
- **`micro-moon`** Micro Moon — small distant moon `[SB]` ◐
- **`twin-moons`** Twin/Many Moons — multiple visible `[SB·HB]` ✅
- **`moonbow`** Moonbow — faint night rainbow `[SB]` ✨
- **`ringed-moon`** Ringed Moon — Saturn-like ring (fantasy) `[SB]` ✨
- **`shattered-moon`** Shattered Moon — broken moon + debris ring `[SB]` ✨ *(Destiny/WoW vibe)*

### 11.9 — Eclipses (the "cool effect" — done right)
- **`solar-eclipse-total`** Total Solar — corona + diamond-ring flash, midday→dark, **stars emerge mid-day**, temp-drop tint `[SB·HT·SAND]` ⭐ *(the dramatic one)*
- **`solar-eclipse-annular`** Annular — "ring of fire" `[SB]` ✅
- **`solar-eclipse-partial`** Partial — bite out of sun `[SB]` ◐
- **`lunar-eclipse`** Lunar — moon reddens (blood) `[SB·HB]` ⭐
- **`double-eclipse`** Twin-Moon Eclipse — both moons cross (fantasy) `[SB]` ✨
- **`demon-eclipse`** Demon Eclipse — black sun + red corona, dread `[SB·HT]` ✨

### 11.10 — Meteors, comets & falling sky
- **`shooting-star`** Shooting Star — single occasional streak `[SB·HT]` ✅
- **`meteor-shower`** Meteor Shower — many streaks + hg splash (BUILT) `[SB·HT·SAND]` ⭐
- **`meteor-storm`** Meteor Storm — intense barrage `[SB·HT]` ✅
- **`bolide`** Fireball/Bolide — bright slow fireball + boom flash `[SB]` ◐
- **`comet`** Comet — slow bright head + long tail, multi-day ☄️ `[SB·HT]` ✅
- **`great-comet`** Great Comet — enormous tail spanning sky `[SB]` ✨
- **`stardust-fall`** Stardust — slow glittering descent `[SB·HT]` ◐
- **`debris-rain`** Debris Rain — burning fragments (shattered moon) `[SB]` ✨

### 11.11 — Cosmic / aurora
- **`aurora`** Aurora — rippling green/violet curtains `[SB·HT·SAND]` ✅ *(your arcane idea)*
- **`arcane-aurora`** Arcane Aurora — prismatic magical curtains `[SB·HT]` ✅
- **`milky-way`** Galaxy Band — Milky Way arc at night `[SB]` ◐
- **`nebula-glow`** Nebula — colored cosmic cloud `[SB]` ✨
- **`starfield-bright`** Bright Starfield — dense twinkling stars `[SB·HB]` ✅
- **`zodiac-reveal`** Constellations — traced constellation lines `[SB]` ◐
- **`cosmic-storm`** Cosmic Storm — swirling space tempest `[SB]` ✨
- **`star-fall`** Star Fall — mass falling stars (omen) `[SB·HT]` ✨

### 11.12 — Arcane / magical
- **`arcane-winds`** Arcane Winds — shifting prismatic streaks `[SB·SAND]` ✅
- **`ley-surge`** Ley Surge — ground-up pulsing glow `[SB·SAND]` ✅
- **`mana-storm`** Mana Storm — chaotic magical tempest `[SB·HT]` ◐
- **`wild-magic`** Wild Magic — random color bursts, distortion `[SB·HT]` ◐
- **`rune-rain`** Rune Rain — falling glowing glyphs `[SB·HT]` ◐
- **`prismatic-veil`** Prismatic Veil — rainbow shimmer curtain `[SB]` ◐
- **`fae-bloom`** Fae Bloom — oversaturated dreamlike haze `[SB·SAND]` ◐
- **`glamour-haze`** Glamour Haze — shimmering enchantment `[SB]` ✨
- **`levitation-motes`** Floating Motes — drifting light particles `[SB·HT]` ◐

### 11.13 — Eldritch / void (your "inky" idea) 🐙
- **`eldritch-ink`** Eldritch Ink — black tendrils creep in from edges `[SB·HT·SAND]` ✅ *(your idea — inky seep in the hg top)*
- **`the-watching-eye`** The Watching Eye — vast eye opens in the sky `[SB]` ✨
- **`void-seep`** Void Seep — reality darkens/drains to black `[SB·HT]` ✅
- **`tentacle-shadows`** Tentacle Shadows — writhing dark silhouettes `[SB]` ✨
- **`madness-warp`** Madness Warp — non-euclidean distortion shimmer `[SB·HT]` ✨
- **`star-spawn-fall`** Star-Spawn Fall — wrong-colored meteors `[SB]` ✨
- **`the-black-tide`** The Black Tide — inky flood rising `[SB·HT·SAND]` ✨
- **`whispering-dark`** Whispering Dark — desaturated dread, drifting murk `[SB·SAND]` ◐

### 11.14 — Divine / infernal
- **`divine-radiance`** Divine Radiance — holy light + god-rays from above `[SB·HT]` ✅
- **`ascension-beam`** Ascension Beam — column of light to heavens `[SB]` ◐
- **`celestial-glow`** Celestial Glow — soft holy ambiance `[SB·HB]` ◐
- **`infernal-sky`** Infernal Sky — red sky + brimstone embers `[SB·HT·SAND]` ✅
- **`hellfire-rain`** Hellfire Rain — falling fire `[SB·HT]` ◐
- **`sulfur-haze`** Sulfur Haze — yellow-green toxic air `[SB·SAND]` ◐ *(possible "gas")*
- **`judgment-light`** Judgment Light — harsh white pillar `[SB]` ✨

### 11.15 — Necromantic / blight / plague
- **`plague`** Plague — green-tinted miasma 🦠 `[SB·HT·SAND]` ✅ *(possible "gas")*
- **`miasma`** Miasma / Gas — drifting toxic haze + bubbles `[SB·HT·SAND]` ✅ *(your "gas")*
- **`necrotic-pall`** Necrotic Pall — green-black death shroud `[SB·SAND]` ◐
- **`grave-mist`** Grave Mist — low spectral fog `[SB]` ◐
- **`will-o-wisps`** Will-o'-Wisps — drifting spirit lights `[SB·HB]` ◐
- **`spirit-aurora`** Spirit Aurora — ghostly soul-lights rising `[SB·HT]` ✨
- **`blight-spread`** Blight — sickly desaturation creeping `[SB·SAND]` ✨
- **`bone-dust`** Bone Dust — pale grey grit fall `[SB·SAND]` ✨

### 11.16 — Planar / temporal / elemental
- **`planar-rift`** Planar Rift — tear of swirling color `[SB·HT]` ◐
- **`portal-tear`** Portal Tear — glowing dimensional gate `[SB]` ✨
- **`shadowfell-gloom`** Shadowfell Gloom — desaturated, cold, dim `[SB·SAND]` ◐
- **`feywild-bleed`** Feywild Bleed — hypersaturated dream-color `[SB·SAND]` ◐
- **`astral-shimmer`** Astral Shimmer — silvery ethereal haze `[SB]` ◐
- **`elemental-surge`** Elemental Surge — fire/water/air/earth tint floods `[SB·SAND]` ◐
- **`chronal-storm`** Chronal Storm — time distortion ripples `[SB·HT]` ✨
- **`frozen-moment`** Frozen Moment — everything stops + crystalline shimmer `[SB·HT]` ✨ *(ties to time-control pause)*

### 11.17 — Hourglass-only dramatic scenes (top chamber set-pieces)
- **`hg-meteor-splash`** meteors land in sand + splash (BUILT) ⭐
- **`hg-blood-drip`** blood seeps/drips down from the top ✅
- **`hg-ink-seep`** eldritch ink bleeds in from the cap ✅
- **`hg-frost-crawl`** frost crystals crawl up the glass walls ✅
- **`hg-petal-settle`** sakura petals drift down onto the sand ◐
- **`hg-ember-fall`** embers drift + glow, settle as ash ◐
- **`hg-sand-tornado`** the sand itself swirls into a vortex (tornado) ◐
- **`hg-souls-rising`** wisps rise from the sand (necromantic) ✨
- **`hg-lightning`** a tiny storm flashes inside the glass ✨
- **`hg-bubbling`** plague/gas bubbles rise through the sand ◐
- **`hg-gold-dust`** festive golden sparkle in the sand (celebration) ◐

### 11.18 — Player mood-tints (global color wash, any color + intensity)
ominous-red · dread-crimson · eerie-green · sickly-green · melancholy-blue · twilight-blue · festive-gold · holy-white · cursed-violet · infernal-orange · fae-pink · void-black · nostalgic-sepia · frostbite-cyan · royal-purple · **+ custom (color picker)**

### 11.19 — Time-control verbs (the playback layer)
play (forward) · **pause (float)** · **rewind (sand falls up)** · fast-forward · step · stop/reset

---

**Tally:** ~140 labeled effects across 19 categories. The MUST set (~20) proves the synced system + every specific thing you've asked for; everything else is registry-ready data the DM/world can switch on. Adding any of them later = one entry + one renderFn, no refactor — that's the whole point of the shared registry.

---

*This catalog becomes the spec for expanding the existing `WEATHER_EFFECTS` + `CELESTIAL_EFFECTS` registries (from Almanac refinement v2/v4) into the unified world-state registry that drives both the sky-band and the hourglass. It folds into the v5.5 (sun + hourglass) and v6 (full effects) dispatches.*
