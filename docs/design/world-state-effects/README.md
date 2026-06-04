# World-State Effects — design reference + working prototypes

Design artifacts from the operator↔coordinator session that designed the **synced world-state animation system**: one `worldState` object drives BOTH the calendar sky-band AND the standalone hourglass time-piece (and later the timeline Tuner). Committed so Chronicle Dev can build from working code instead of from scratch.

## Read first
- **`CATALOG.md`** — the full spec. The shared-registry model (Part 0), continuous states (Part 1), dynamic celestial bodies incl. named moon phases (Part 2), the canonical 11-type/4-category weather taxonomy (Part 3), celestial events (Part 4), player mood-tints (Part 5), the **DM time-control verb layer** — pause=floating sand, rewind=sand-falls-up (Part 6), surface-specific extras (Part 7), the `worldState` data shape (Part 8), build tiering (Part 9), open questions (Part 10), and the **~140-effect expanded library** (Part 11).

## Prototypes (open in a browser — all self-contained, no deps)
| File | What it proves |
|---|---|
| **`hourglass-meteor-daynight-mockup.html`** | The fullest one. Canvas-driven interior + SVG glass shell. Top chamber = meteor shower (glowing-trail meteors + sand-splash + impact flash). Bottom = independent day→sunset→night sky cycle (sun arcs down + sets behind the sand horizon, stars emerge). Neck stream feeds the live **slope-limited heightmap sand pile** (avalanche at angle of repose). This is the architecture: dynamic interior on canvas, glass/frame as SVG overlay. |
| `hourglass-states-gallery.html` | Parameterized generator stamping the hourglass across times-of-day / event themes / world-state sand colors / full scenarios. Shows the thematic-chamber system + the registry param shape. |
| `hourglass-poc-v5.html` | The thematic-chambers + glass-realism touches (grit, edge chromatic fringing, refraction of chamber backdrop) + slowed realistic sand. |
| `hourglass-glass-poc-v3.html` | Glass-material fidelity reference: `feTurbulence` wood grain + `feDiffuseLighting` bevel + `feSpecularLighting` glass gloss + funnel/cone sand. |
| `icon-curation-v5.5.html` | Sun-icon candidates. **Locked: `lorc/sun.svg`** (game-icons.net, CC-BY 3.0) for the default sun; `lorc/eclipse.svg` for solar eclipse. |

## Locked decisions
- **Aesthetic:** stylized fantasy (premium-stylized, NOT photoreal — inline-SVG glass has a ceiling; WebGL out of scope).
- **Sun:** `lorc/sun.svg` icon + CSS animation + canvas bloom (per fidelity-v5 dispatch).
- **Hourglass:** dark-wood frame + procedural glass (SVG filters) + canvas sand (heightmap+avalanche). Horizontal "shelf" form factor.
- **Sand:** slope-limited heightmap — the cheap "looks like physics" model (NOT a real granular sim). Falling grains are a tiny pooled particle set; the pile is a column-height array with an avalanche slope-limit pass.
- **Performance:** single shared rAF, pooled/capped particles, `IntersectionObserver`/visibility pause, `prefers-reduced-motion` → static frame, DPR clamp. >5% idle CPU = cut the pool.

## Architecture note (the sync)
Both surfaces subscribe to one `worldState` (see CATALOG Part 8). Change it once → both react. The existing `WEATHER_EFFECTS` + `CELESTIAL_EFFECTS` registries (from Almanac refinement v2/v4) are the seed; this catalog is the plan for growing them into the unified registry. Each effect declares per-surface renderers (`skyBand` / `hgTop` / `hgBottom` / `hgSand` / `timeline`); some are no-ops on a surface.

## Status
Design exploration — **not yet wired into `/demo`**. The coordinator will author the v5.5 (sun + hourglass) and v6 (effects) dispatches from this. These prototypes are the visual spec + starting code.
