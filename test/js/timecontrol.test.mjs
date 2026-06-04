// timecontrol.test.mjs — Wave 3 runtime tests for the time-control verb layer
// (CATALOG Part 6, D&D narrative-chunk model). Booted with reduced-motion so
// the ~600ms/400ms tweens resolve to instant snaps (testable end-states). The
// shared-rAF tweening itself + reduced-motion snapping is what makes this work
// here; visual fidelity of the transitions is the operator's local gate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './harness.mjs';

function tc(w) { return w.__calTimeControl; }

test('TIME_CONTROL exposes the verb API', () => {
  const w = boot({ reduced: true });
  const T = tc(w);
  for (const k of ['advanceHours', 'advanceDays', 'longRest', 'setTime', 'stepBack', 'togglePause', 'setPaused', 'fillCap']) {
    assert.ok(k in T, 'has ' + k);
  }
  assert.equal(T.fillCap, 0.33);
});

test('+N hours advances timeOfDay and bumps timepieceFill', () => {
  const w = boot({ reduced: true });
  const from = w.__calWorldState.timeOfDay;          // 0.5 seed
  tc(w).advanceHours(1);                             // +1/24
  const exp = (from + 1 / 24) % 1;
  assert.ok(Math.abs(w.__calWorldState.timeOfDay - exp) < 1e-3, 'timeOfDay advanced ~1hr');
  assert.ok(w.__calWorldState.timepieceFill > 0, 'fill bumped');
  assert.ok(w.__calWorldState.timepieceFill <= 0.33 + 1e-9, 'fill never exceeds the cap');
});

test('fill caps at ~1/3 → period boundary resets it to 0', () => {
  const w = boot({ reduced: true });
  // a long rest (8h) on a 12h period → fill = 8/12*0.33 = 0.22 (below cap)
  tc(w).longRest();
  assert.ok(w.__calWorldState.timepieceFill > 0.2 && w.__calWorldState.timepieceFill < 0.33);
  // another big advance pushes past the cap → boundary fires, fill resets
  tc(w).advanceHours(8);
  assert.equal(w.__calWorldState.timepieceFill, 0, 'fill reset to 0 at the period boundary');
});

test('+1day moves the calendar cursor (day pipeline repaints)', () => {
  const w = boot({ reduced: true });
  const d0 = Object.assign({}, w.__calWorldState.date);
  tc(w).advanceDays(1);
  assert.equal(w.__calWorldState.date.day, d0.day + 1, 'date advanced one day');
});

test('set-time snaps timeOfDay + fill to the period fraction', () => {
  const w = boot({ reduced: true });
  tc(w).setTime(0.5);
  assert.ok(Math.abs(w.__calWorldState.timeOfDay - 0.5) < 1e-3);
});

test('step-back is a single undo restoring the prior state', () => {
  const w = boot({ reduced: true });
  const t0 = w.__calWorldState.timeOfDay, d0 = Object.assign({}, w.__calWorldState.date);
  tc(w).advanceHours(3);
  assert.ok(Math.abs(w.__calWorldState.timeOfDay - t0) > 1e-3, 'time moved');
  assert.equal(tc(w).stepBack(), true, 'undo available');
  assert.ok(Math.abs(w.__calWorldState.timeOfDay - t0) < 1e-3, 'timeOfDay restored');
  assert.equal(w.__calWorldState.date.day, d0.day, 'date restored');
  // nothing left to undo
  assert.equal(tc(w).stepBack(), false);
});

test('atmosphere-pause zeroes deltas (flag + engine paused), toggles back', () => {
  const w = boot({ reduced: true });
  tc(w).togglePause();
  assert.equal(w.__calWorldState.atmospherePaused, true);
  assert.equal(w.CalParticleEngine.paused(), true, 'engine loop paused');
  tc(w).togglePause();
  assert.equal(w.__calWorldState.atmospherePaused, false);
  assert.equal(w.CalParticleEngine.paused(), false, 'engine resumed');
});
