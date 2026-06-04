// mood.test.mjs — Wave 2 runtime tests for the player mood-tint wash
// (CATALOG Part 5; resolution step 6). The renderer composites over both
// surfaces; here we pin the preset palette, the step-6 ordering, the
// intensity-0 no-op, and that applyMoodTint runs without throwing. Visual
// fidelity (the wash over each scene) is the operator's local gate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot, arr } from './harness.mjs';

const PRESETS = ['ominous-red', 'eerie-green', 'melancholy-blue', 'festive-gold', 'cursed-violet', 'holy-white', 'void-black', 'frostbite-cyan'];

test('MOOD_PRESETS has the 8 named presets with {color,intensity}', () => {
  const w = boot();
  const P = w.__calMoodPresets;
  assert.ok(P, 'MOOD_PRESETS exposed');
  for (const id of PRESETS) {
    assert.ok(P[id], 'has preset ' + id);
    assert.equal(typeof P[id].color, 'string', id + ' color');
    assert.ok(P[id].intensity > 0 && P[id].intensity <= 1, id + ' intensity in (0,1]');
  }
});

test('mood-tint is resolution step 6 — after events, before time-control', () => {
  const w = boot();
  const order = arr(w.__calLayerOrder);
  assert.ok(order.indexOf('moodTint') > order.indexOf('events'), 'moodTint after events');
  assert.ok(order.indexOf('moodTint') < order.indexOf('timeControl'), 'moodTint before timeControl');
  // becomes an active layer once a color + intensity are set
  w.__calSetWorldState({ moodTint: { color: 'oklch(0.55 0.22 25)', intensity: 0.5 } });
  assert.ok(arr(w.__calResolveLayers(w.__calWorldState)).includes('moodTint'), 'active when set');
});

test('intensity 0 / no color is a no-op (not an active layer, no change from seed)', () => {
  const w = boot();
  const changed = arr(w.__calSetWorldState({ moodTint: { intensity: 0 } })); // seed is {color:null,intensity:0}
  assert.deepEqual(changed, [], 'no change from the transparent seed');
  assert.ok(!arr(w.__calResolveLayers(w.__calWorldState)).includes('moodTint'), 'not active at 0');
});

test('applyMoodTint runs without throwing and is idempotent', () => {
  const w = boot();
  w.__calSetWorldState({ moodTint: { color: 'oklch(0.55 0.22 25)', intensity: 0.5 } });
  assert.doesNotThrow(() => { w.__calApplyMoodTint(); w.__calApplyMoodTint(); });
  // clearing back to 0 is also safe
  w.__calSetWorldState({ moodTint: { intensity: 0 } });
  assert.doesNotThrow(() => w.__calApplyMoodTint());
});
