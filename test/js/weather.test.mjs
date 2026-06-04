// weather.test.mjs — Wave 2 runtime tests for the weather/celestial sky-band
// renderers (CATALOG §12.2). Verifies all 10 effects are registered, each
// factory yields a frame(ctx,w,h,dt) that runs without throwing against a mock
// 2D context (live + static dt=0), and the EFFECTS entries carry the
// per-surface shape (skyBand renderer + hgSand color + timeline glyph). Visual
// fidelity is the operator's local gate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './harness.mjs';

const IDS = [
  'weather-clear', 'weather-cloudy', 'weather-rain', 'weather-thunderstorm',
  'weather-snow', 'weather-fog', 'weather-tornado', 'weather-ashfall',
  'celestial-meteor-shower', 'celestial-aurora',
];

// Minimal canvas 2D context stub: records nothing, just satisfies the calls.
function mockCtx() {
  const grad = { addColorStop() {} };
  return {
    fillStyle: '', strokeStyle: '', lineWidth: 1, globalAlpha: 1, globalCompositeOperation: 'source-over',
    createLinearGradient: () => grad, createRadialGradient: () => grad,
    fillRect() {}, strokeRect() {}, clearRect() {},
    beginPath() {}, closePath() {}, moveTo() {}, lineTo() {}, arc() {}, ellipse() {},
    fill() {}, stroke() {}, save() {}, restore() {}, translate() {}, rotate() {}, scale() {}, clip() {},
  };
}

test('all 10 weather/celestial renderers are registered', () => {
  const w = boot();
  const R = w.__calWeatherRenderers;
  assert.ok(R, 'WEATHER_RENDERERS exposed');
  for (const id of IDS) assert.equal(typeof R[id], 'function', 'factory for ' + id);
  assert.equal(Object.keys(R).length, 10, 'exactly 10 renderers');
});

test('each renderer factory yields a frame that runs (live + static)', () => {
  const w = boot();
  const R = w.__calWeatherRenderers;
  for (const id of IDS) {
    const frame = R[id]();
    assert.equal(typeof frame, 'function', id + ' factory returns a frame fn');
    const ctx = mockCtx();
    // a few live frames then a static (dt=0) frame — none should throw
    assert.doesNotThrow(() => { for (let i = 0; i < 5; i++) frame(ctx, 320, 200, 0.016); frame(ctx, 320, 200, 0); }, id + ' frame threw');
  }
});

test('EFFECTS carries the Wave 2 entries with per-surface shape', () => {
  const w = boot();
  const fx = w.__calEffects;
  for (const id of IDS) {
    assert.ok(fx[id], 'EFFECTS has ' + id);
    assert.equal(typeof fx[id].skyBand, 'function', id + ' has a skyBand renderer factory');
    assert.ok(fx[id].hgSand && typeof fx[id].hgSand.color === 'string', id + ' has hgSand.color');
    assert.ok(typeof fx[id].timeline === 'string' && fx[id].timeline.length > 0, id + ' has a timeline glyph');
    assert.equal(fx[id].tier, 'must');
    assert.ok(fx[id].category, id + ' has a category');
  }
});
