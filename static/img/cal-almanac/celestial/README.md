# Almanac celestial assets — sun (v5 prototype)

These are the painted celestial bodies that replace the v4 hand-rolled
SVG primitives. See
`dispatches/chronicle/C-CAL-SHOWCASE-DESIGN-1-ALMANAC-FIDELITY-V5-SUN-PROTOTYPE.md`
for the binding asset spec + prompt set.

## ⚠ Current state — PLACEHOLDERS

The committed `.webp` / `.png` files in this directory are **procedurally-
generated radial-gradient stand-ins** (Pillow), watermarked
"PLACEHOLDER — replace with painted AI asset". They exist so the v5
wiring (templ + CSS + JS + tests + headless gate) is reviewable
end-to-end before the painted assets land. **They are not the final art.**

## Replacement procedure — for the operator

1. Generate the 5 sun states per the dispatch's **Prompt set** (Midjourney
   / DALL-E / Sora), following the dispatch's iteration guidance.
2. **Save as standard WebP** (NOT WebP 2 / WP2).
   - The asset you uploaded `2026-06-03` was actually a `WebP 2` file
     (`.wp2`, Google's experimental successor) — magic bytes `f4 ff 6f`.
     **Browsers do not support WP2 yet.** Re-export from your tool as
     standard WebP or PNG (`squoosh.app` → WebP @ q85 + PNG @ q90).
3. Run through `squoosh.app` or `cwebp -q 85 / -q 90` for optimization.
   Target file sizes: 80-150 KB each (300 KB hard cap, per dispatch
   stop-and-flag).
4. Commit both formats for each of the 5 states:
   - `sun-default.webp` + `sun-default.png`
   - `sun-dawn.webp` + `sun-dawn.png`
   - `sun-dusk.webp` + `sun-dusk.png`
   - `sun-eclipse.webp` + `sun-eclipse.png`
   - `sun-special.webp` + `sun-special.png`
5. Push. The `<picture>` element switches via `data-cal-sun-state`; no
   wiring changes needed once the file paths exist.

## Why two formats

`<picture>` serves WebP to modern browsers (~30 % smaller) and falls
back to PNG for environments that don't support WebP (e.g. older
in-app webviews). Both ship.

## Test gate behaviour

`TestCalAlmanacSun_AssetsReferenced` checks the markup references all
5 paths. `TestCalAlmanacSun_PaintedAssetsExist` checks the files
actually exist and are within size budget. Placeholders pass both gates
so the PR is mergeable; real-asset replacement is a follow-up commit.
