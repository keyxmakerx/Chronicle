// engine.test.mjs — pre-port hardening tests (§8): the pure bits that had zero
// coverage + the rAF resilience BLOCKER (§1). Engine-mode boot gives the
// particle engine a mock 2D context + a manually-pumped rAF so we can drive the
// live loop and assert a throwing spec does NOT kill it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './harness.mjs';

test('parseTime handles 12am/12pm + out-of-range edges (24h)', () => {
  const w = boot();
  const pt = w.__calParseTime;
  assert.ok(Math.abs(pt('12:00 am', 24) - 0) < 1e-9, '12:00am → 00:00');
  assert.ok(Math.abs(pt('12:00 pm', 24) - 0.5) < 1e-9, '12:00pm → noon');
  assert.ok(Math.abs(pt('6:00', 24) - 0.25) < 1e-9, '06:00 → 0.25');
  assert.ok(Math.abs(pt('11:30 pm', 24) - (23 * 60 + 30) / 1440) < 1e-9, '11:30pm');
  assert.equal(pt('25:00', 24), null, 'hour out of range → null');
  assert.equal(pt('6:75', 24), null, 'minute out of range → null');
  assert.equal(pt('not a time', 24), null, 'unparseable → null');
});

test('reduced-motion gate is honored at boot', () => {
  assert.equal(boot({ reduced: true }).CalParticleEngine.reduced(), true);
  assert.equal(boot({ reduced: false }).CalParticleEngine.reduced(), false);
});

test('setProfile trims live particles to the new cap', () => {
  const w = boot({ engine: true });
  const eng = w.CalParticleEngine, sky = w.__calSkyEngine;
  assert.ok(sky, 'sky surface created under engine mode');
  sky.setEmitters([{ shape: 'dot', color: '#fff', sizeRange: [1, 2], velocity: { x: [0, 0], y: [10, 20] }, spawnRate: 2000, maxAlive: 80 }]);
  for (let i = 0; i < 6; i++) w.__pump(1);     // spawn up to the normal cap (40)
  assert.ok(eng.live() > 20, 'spawned past the low cap, got ' + eng.live());
  eng.setProfile('low');                        // cap → 20
  assert.ok(eng.live() <= 20, 'trimmed to the low cap, got ' + eng.live());
  assert.equal(eng.cap(), 20);
});

test('§1 BLOCKER — a throwing particleSpec does NOT kill the shared rAF', () => {
  const w = boot({ engine: true });
  const sky = w.__calSkyEngine;
  // a spec whose color getter throws every time spawn() reads it
  let reads = 0;
  sky.setEmitters([{ shape: 'dot', sizeRange: [1, 2], velocity: { x: [0, 0], y: [10, 20] }, spawnRate: 2000, maxAlive: 10, get color() { reads++; throw new Error('bad spec'); } }]);
  const before = w.__rafCount || 0;
  for (let i = 0; i < 6; i++) w.__pump(1);
  assert.ok(!w.__pumpThrew, 'step() never threw out to the rAF caller');
  assert.ok((w.__rafCount || 0) > before, 'the loop kept rescheduling frames');
  assert.ok(reads > 0, 'the bad spec was actually exercised');
  // a healthy second surface/emitter still runs (engine not globally dead)
  assert.equal(w.CalParticleEngine.paused(), false);
});
