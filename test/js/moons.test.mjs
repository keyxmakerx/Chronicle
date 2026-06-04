// moons.test.mjs — Wave 2 runtime tests for the moon library (CATALOG §12.1):
// the MOON_DESIGNS registry (12 procedural + 2 emoji families), phase-index
// mapping, the named-phase vocabulary walk, and the worldState.moons shape.
// Visual fidelity (the rendered discs) is the operator's local gate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './harness.mjs';

const PROCEDURALS = [
  'moon-watercolor', 'moon-holographic', 'moon-etched', 'moon-constellation',
  'moon-realistic-selene', 'moon-realistic-silver', 'moon-realistic-warm',
  'moon-realistic-full', 'moon-realistic-eclipse', 'moon-realistic-ancient',
  'moon-realistic-icy', 'moon-realistic-volcanic',
];

test('MOON_DESIGNS has all 12 procedurals + 2 emoji families', () => {
  const w = boot();
  const D = w.__calMoonDesigns;
  assert.ok(D, 'MOON_DESIGNS exposed');
  for (const id of PROCEDURALS) {
    assert.ok(D[id], 'has ' + id);
    assert.equal(D[id].phaseSource, 'css-clip', id + ' is css-clip');
  }
  assert.equal(D.noto.phaseSource, 'noto');
  assert.equal(D.twemoji.phaseSource, 'twemoji');
  assert.equal(D['moon-holographic'].animated, true, 'holographic is the animated one');
  assert.equal(Object.keys(D).length, 14);
});

test('phase index maps cycle position → 0..7 (new…full…waning)', () => {
  const w = boot();
  const pi = w.__calMoonSim.phaseIndex;
  assert.equal(pi(0.0), 0, 'new at 0');
  assert.equal(pi(0.5), 4, 'full at half-cycle');
  assert.equal(pi(0.25), 2, 'first quarter');
  assert.equal(pi(0.75), 6, 'last quarter');
  assert.equal(pi(1.0), 0, 'wraps to new');
  // 8 emoji codes + 8 phase classes line up with the index range.
  assert.equal(w.__calMoonSim.emojiCodes.length, 8);
  assert.equal(w.__calMoonSim.phaseClasses.length, 8);
});

test('named-phase walk prefers the per-moon vocabulary, falls back procedurally', () => {
  const w = boot();
  const np = w.__calMoonSim.namedPhase;
  const moon = { namedPhases: [{ start_pct: 50, end_pct: 70, name: 'The Silver Crown' }] };
  assert.equal(np(moon, 0.56), 'The Silver Crown', 'inside the span → named');
  assert.match(np(moon, 0.10), /New|Crescent|Quarter|Gibbous|Full/, 'outside any span → procedural name');
  // wrap-around span (e.g. 90→10) covers the seam.
  const wrap = { namedPhases: [{ start_pct: 90, end_pct: 10, name: 'The Dark Sister' }] };
  assert.equal(np(wrap, 0.97), 'The Dark Sister');
  assert.equal(np(wrap, 0.03), 'The Dark Sister');
});

test('worldState.moons carries the §12.1 shape (design/phaseSource/tint)', () => {
  const w = boot();
  const moons = w.__calWorldState.moons;
  assert.ok(moons.length >= 1);
  for (const m of moons) {
    for (const k of ['baseDesign', 'phaseSource', 'size', 'orbitSpeed', 'orbitOffset', 'namedPhases']) {
      assert.ok(k in m, 'moon has ' + k);
    }
    assert.ok(w.__calMoonDesigns[m.baseDesign] || m.phaseSource, 'design resolvable: ' + m.baseDesign);
  }
});
