// skybox_meteor_pacing.test.mjs — C-SKYBOX-WIDGET meteor pacing fix.
//
// Root cause (see cal-almanac.js mkMeteors/effectLayersFor comments): the
// operator's "one meteor then minutes of nothing" was layer-recreation
// churn, not spawn position — effectLayersFor() rebuilt every active SKY_FX
// factory from scratch on every refeedSky()/renderDayPipeline() call, which
// fires on every setWorldState({timeOfDay}) tick (a DM time-advance tween
// calls it on every rAF frame for ~600ms). Each rebuild discarded the
// meteor closure's live streak list + spawn accumulator.
//
// These tests cover the pure spawn-rate/intensity/viewport-bound helpers
// (peak / off-peak / none; viewport bounds) plus a regression check that
// the LAYER_CACHE fix actually reuses layer state across refeeds.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './harness.mjs';

test('meteorShowerIntensity: peak / off-peak / none', () => {
  const w = boot();
  const intensity = w.__calMeteorShowerIntensity;
  // No window data at all (most demo/legacy events have no duration) →
  // peak, i.e. today's unconditional-always-on behavior, unchanged.
  assert.equal(intensity(null), 1, 'no progress data → peak (unchanged default)');
  // Inside the window: ramps up, holds peak through the middle, tapers —
  // floored so "off-peak" never reads as fully dead.
  assert.equal(intensity(0.5), 1, 'mid-shower → peak');
  assert.equal(intensity(0.6), 1, 'still in the peak band');
  assert.ok(intensity(0.05) < 1 && intensity(0.05) > 0.3, 'ramping up → off-peak, not zero');
  assert.ok(intensity(0.95) < 1 && intensity(0.95) > 0.3, 'tapering down → off-peak, not zero');
  assert.ok(intensity(0.05) >= 0.35, 'off-peak never drops below the floor');
  // Concrete progress outside [0,1]: the clock has moved past/before the
  // shower's actual hours → genuinely silent.
  assert.equal(intensity(-0.1), 0, 'before the window → none');
  assert.equal(intensity(1.2), 0, 'after the window → none');
});

test('meteorShowerProgress: fraction through a {start_time,duration} window, wraps past midnight', () => {
  const w = boot();
  const progress = w.__calMeteorShowerProgress;
  // No duration data → null (caller then defaults to peak).
  assert.equal(progress({ start_time: 20 }, 0.9, 24), null, 'missing duration → null');
  assert.equal(progress(null, 0.9, 24), null, 'no window → null');
  // A 4-hour shower starting at hour 20 (0.833 of a 24h day): at hour 22
  // (0.9167) we're 2/4 of the way through.
  const win = { start_time: 20, duration: 4 };
  assert.ok(Math.abs(progress(win, 22 / 24, 24) - 0.5) < 1e-9, 'mid-window fraction');
  assert.ok(Math.abs(progress(win, 20 / 24, 24) - 0) < 1e-9, 'at the start → 0');
  // Past the window (hour 25 = hour 1 the next day, elapsed 5h of a 4h dur).
  assert.ok(progress(win, 1 / 24, 24) > 1, 'past the window → > 1 (intensity treats this as none)');
  // Wraps past midnight: start_time 23, duration 2 → active hours 23..25(=1).
  const wrapWin = { start_time: 23, duration: 2 };
  assert.ok(Math.abs(progress(wrapWin, 0 / 24, 24) - 0.5) < 1e-9, 'wraps past midnight correctly');
});

test('meteorSpawnRate: peak / off-peak / none scale the base rate', () => {
  const w = boot();
  const rate = w.__calMeteorSpawnRate;
  assert.equal(rate(2, 1), 2, 'peak (intensity 1) leaves baseRate unmodified');
  assert.equal(rate(2, 0), 0, 'none (intensity 0) is silent');
  assert.equal(rate(2, null), 2, 'undefined intensity defaults to peak');
  const offPeak = rate(2, 0.25);
  assert.ok(offPeak > 0 && offPeak < 2, 'off-peak is reduced but nonzero — "clearly alive", not paused');
  assert.ok(offPeak > 0.5, 'sqrt curve keeps a low-but-nonzero intensity visibly alive, not near-zero');
});

test('meteorSpawnPoint: spawn origin stays anchored to the actual rendered band (viewport bounds)', () => {
  const w = boot();
  const spawn = w.__calMeteorSpawnPoint;
  const W = 800, H = 200;
  for (let i = 0; i < 50; i++) {
    const fromTop = spawn(W, H, true);
    // A small (<=15%) edge overshoot is intentional (streaks enter already
    // in motion) — but never far outside, and Y starts just above the band.
    assert.ok(fromTop.x >= 0 && fromTop.x <= W * 1.15, 'fromTop x stays within the band + small overshoot');
    assert.ok(fromTop.y <= 0 && fromTop.y >= -H * 0.1, 'fromTop y starts just above the band, not off in space');

    const fromRight = spawn(W, H, false);
    assert.ok(fromRight.x >= W && fromRight.x <= W * 1.05, 'fromRight x hugs the right edge, not far beyond it');
    assert.ok(fromRight.y >= 0 && fromRight.y <= H * 0.4, 'fromRight y stays within the band vertically');
  }
});

test('LAYER_CACHE: an unchanged active meteor-shower reuses its layer state across refeeds', () => {
  const w = boot();
  const effectLayersFor = w.__calEffectLayersForRaw;
  const events = [{ type: 'meteor-shower', name: 'Test Shower', start_time: 20, duration: 4 }];
  const first = effectLayersFor('clear', events);
  const second = effectLayersFor('clear', events);
  // Same active set (same weather id, same event types) → the SAME layer
  // function instances come back, not fresh ones — this is the actual fix:
  // before it, every call tore down the meteor closure's live streaks +
  // spawn accumulator, which is what read as "one meteor then nothing".
  assert.equal(first.back.length, second.back.length, 'layer count stable across refeeds');
  // back[0] is the starfield underlay, which effectLayersFor intentionally
  // recreates every call (it's stateless) — the cache applies to weather/
  // event layers (index 1+), which is where the meteor closures live.
  for (let i = 1; i < first.back.length; i++) {
    assert.equal(first.back[i], second.back[i], 'back layer ' + i + ' is the SAME cached instance, not recreated');
  }
  // A genuine change (shower ends) drops the cached state instead of
  // leaking it forever. Layer order is [starfield, ...event layers (both
  // meteor-shower factories), ...weather layers (clear's wisps)] — index 1
  // is the meteor-shower's own first factory, which must NOT survive the
  // shower ending (unlike the weather wisps layer, which legitimately stays
  // cached throughout since "clear" weather never changed).
  const cleared = effectLayersFor('clear', []);
  assert.notEqual(cleared.back.length, 0, 'starfield still renders');
  const reactivated = effectLayersFor('clear', events);
  assert.notEqual(reactivated.back[1], first.back[1],
    'a shower that ended and restarted gets fresh state, not a stale leaked closure');
});

test('mkMeteors reduced-motion static frame (dt=0) never throws and draws no streak state', () => {
  const w = boot();
  const SKY_FX = w.__calSkyFx;
  const factories = [].concat(SKY_FX['meteor-shower'].back, SKY_FX['meteor-shower'].front);
  const grad = { addColorStop() {} };
  const ctx = {
    fillStyle: '', strokeStyle: '', lineWidth: 1,
    createLinearGradient: () => grad, createRadialGradient: () => grad,
    beginPath() {}, moveTo() {}, lineTo() {}, arc() {}, fill() {}, stroke() {},
  };
  assert.doesNotThrow(() => {
    for (const make of factories) {
      const frame = make();
      frame(ctx, 320, 200, 0, 0.1); // reduced-motion single static call
    }
  }, 'static dt=0 paint must not depend on canvas text APIs (fillText) or throw');
});
