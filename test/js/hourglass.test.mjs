// hourglass.test.mjs — Wave 1b runtime tests for the hourglass interior sim's
// PURE math (window.__calHgSim): slope-limited avalanche, day/night sky
// keyframe interpolation, sun arc/visibility, star fade. Drawing is canvas and
// not verifiable headless — the operator's local visual gate covers that.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './harness.mjs';

test('__calHgSim is exposed after init', () => {
  const w = boot();
  assert.ok(w.__calHgSim, 'hg sim helpers exposed');
  for (const k of ['avalanche', 'sky', 'sun', 'starFade']) {
    assert.equal(typeof w.__calHgSim[k], 'function', 'has ' + k);
  }
});

test('avalanche conserves total sand mass', () => {
  const w = boot();
  // A single tall spike should spread but not change the total.
  const col = new Float64Array(10);
  col[0] = 100;
  const before = col.reduce((a, b) => a + b, 0);
  w.__calHgSim.avalanche(col, 1.0, 8);
  const after = Array.from(col).reduce((a, b) => a + b, 0);
  assert.ok(Math.abs(before - after) < 1e-6, `mass conserved: ${before} → ${after}`);
});

test('avalanche reduces the max slope toward the repose limit', () => {
  const w = boot();
  const col = new Float64Array(8);
  col[0] = 50; // steep spike against flat neighbours
  const repose = 2.0;
  const slopeOf = (c) => { let m = 0; for (let i = 0; i < c.length - 1; i++) m = Math.max(m, Math.abs(c[i] - c[i + 1])); return m; };
  const oneePass = new Float64Array(col); w.__calHgSim.avalanche(oneePass, repose, 1);
  // A single pass already reduces the spike's slope...
  assert.ok(slopeOf(oneePass) < 50, 'one pass reduces the slope');
  // ...and enough passes settle every adjacent pair to ≤ the repose slope.
  w.__calHgSim.avalanche(col, repose, 500);
  assert.ok(slopeOf(col) <= repose + 1e-3, `max slope ${slopeOf(col)} settled to ≤ repose ${repose}`);
});

test('sky interpolation hits the keyframe colors and stays mid-range between', () => {
  const w = boot();
  const midday = w.__calHgSim.sky(0.5);   // [top, bot]
  assert.equal(midday[1], 'rgb(191,224,255)'); // #bfe0ff bottom at the 0.50 keyframe
  const night = w.__calHgSim.sky(0.0);
  assert.equal(night[0], 'rgb(4,6,15)');       // #04060f top at deep night
  // A point between keyframes is a valid rgb() string, not a keyframe literal.
  const dawnish = w.__calHgSim.sky(0.24);
  assert.match(dawnish[0], /^rgb\(\d+,\d+,\d+\)$/);
});

test('sun is visible across the day window and hidden at night', () => {
  const w = boot();
  assert.equal(w.__calHgSim.sun(0.05).visible, false, 'deep night → no sun');
  assert.equal(w.__calHgSim.sun(0.95).visible, false, 'deep night → no sun');
  const noon = w.__calHgSim.sun(0.5);
  assert.equal(noon.visible, true);
  assert.ok(noon.y > 0.9, 'sun near zenith at midday');
  assert.ok(noon.alpha > 0.9, 'fully bright at midday');
  // Lower and fading as it approaches dusk.
  const dusk = w.__calHgSim.sun(0.78);
  assert.ok(dusk.visible && dusk.y < noon.y, 'lower toward dusk');
});

test('star fade is 0 by day and 1 deep at night', () => {
  const w = boot();
  assert.equal(w.__calHgSim.starFade(0.5), 0, 'no stars at midday');
  assert.equal(w.__calHgSim.starFade(0.0), 1, 'full stars deep at night');
  const dusk = w.__calHgSim.starFade(0.78);
  assert.ok(dusk > 0 && dusk <= 1, 'stars fading in at dusk');
});
