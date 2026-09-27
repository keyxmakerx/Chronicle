// sky_moon.test.mjs — coverage for the moon-library mapping and the
// deterministic-body seeding in static/js/widgets/sky_moon.js (issue #763).
//
// Scoped to the parts that need no canvas: MOONR.rng/makeNoise/fbm/body (pure
// math) and SkyMoon.specFor's mapping from a real calendar_moons row onto a
// MOONR spec. MOONR.shade/sprite are NOT exercised here — they allocate a
// `document`-backed canvas/ImageData this harness doesn't provide (no DOM is
// loaded; see the file header comment on why sky_world's tests can skip it
// too). Those two are exercised indirectly by the Playwright coverage
// (sky_pane.spec — asserts the canvas actually paints non-blank pixels).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const worldPath = join(here, '..', '..', 'static', 'js', 'widgets', 'sky_world.js');
const moonPath = join(here, '..', '..', 'static', 'js', 'widgets', 'sky_moon.js');

// sky_moon.js does not itself depend on SkyWorld, but is loaded into the same
// shape of sandbox (a self-referencing `window`) every sky_*.js file expects,
// so a future dependency added to either doesn't silently pass here while
// failing in the browser's real load order.
function loadSandbox() {
  const sandbox = {};
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(worldPath, 'utf8'), sandbox, { filename: 'sky_world.js' });
  vm.runInContext(readFileSync(moonPath, 'utf8'), sandbox, { filename: 'sky_moon.js' });
  return sandbox;
}

const { MOONR, SkyMoon } = loadSandbox();

test('MOONR.rng is a deterministic, seeded PRNG (same seed -> same sequence)', () => {
  const a = MOONR.rng(12345);
  const b = MOONR.rng(12345);
  const seqA = [a(), a(), a()];
  const seqB = [b(), b(), b()];
  assert.deepEqual(seqA, seqB);
  for (const v of seqA) assert.ok(v >= 0 && v < 1, `rng() output ${v} out of [0,1)`);
});

test('MOONR.rng gives different sequences for different seeds', () => {
  const a = MOONR.rng(1)();
  const b = MOONR.rng(2)();
  assert.notEqual(a, b);
});

test('specFor maps a hand-authored BaseDesign to its named library spec', () => {
  const moon = { base_design: 'moon-realistic-selene', color: '#d9e1ec', tint: 0.4 };
  const spec = SkyMoon.specFor(moon);
  assert.equal(spec.seed, SkyMoon.library['moon-realistic-selene'].seed);
  assert.equal(spec.color, '#d9e1ec');
  assert.equal(spec.tint, 0.4);
});

test('specFor falls back to a deterministic generated body for an unknown design', () => {
  const moon = { base_design: 'some-custom-system-moon', color: '#ffaa00' };
  const specA = SkyMoon.specFor(moon);
  const specB = SkyMoon.specFor({ base_design: 'some-custom-system-moon', color: '#ffaa00' });
  // Same key -> same body, forever (the file's own contract): re-deriving it
  // must not silently reroll a different-looking moon on a page reload.
  assert.equal(specA.seed, specB.seed);
  assert.equal(specA.basins, specB.basins);
  assert.ok(specA.craters > 0 && specA.rMax > 0);
});

test('specFor rejects a malformed color and falls back to the default swatch', () => {
  const spec = SkyMoon.specFor({ base_design: 'moon-realistic-selene', color: 'not-a-color' });
  assert.equal(spec.color, '#d9e1ec');
});

test('specFor clamps an out-of-range tint to the 0.4 default', () => {
  const spec = SkyMoon.specFor({ base_design: 'moon-realistic-selene', color: '#fff', tint: 5 });
  assert.equal(spec.tint, 0.4);
});

test('specFor accepts the mockup-shaped {baseDesign, Color} casing too', () => {
  const spec = SkyMoon.specFor({ baseDesign: 'moon-realistic-umbra', Color: '#abcabc' });
  assert.equal(spec.seed, SkyMoon.library['moon-realistic-umbra'].seed);
  assert.equal(spec.color, '#abcabc');
});

test('MOONR.body builds a stable procedural body for a spec (same spec -> same body identity)', () => {
  const spec = SkyMoon.specFor({ base_design: 'moon-realistic-selene', color: '#d9e1ec' });
  const bodyA = MOONR.body(spec);
  const bodyB = MOONR.body(spec);
  // body() is cached by a structural key derived from the spec — calling it
  // twice for the same spec must return the SAME object, not rebuild it.
  assert.strictEqual(bodyA, bodyB);
  assert.ok(bodyA.craters.length > 0, 'a hand-authored spec should place craters');
});
