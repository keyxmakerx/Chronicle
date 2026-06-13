// weather_catalog_completeness.test.mjs — pins the JS half of the 3-way weather
// catalog sync (C-CAL-PARITY-W0W1): SKY_FX (renderers) ↔ SKY_FX_META (display
// meta) ↔ the weather-id contract. The Go half (gmWeatherTypes) is pinned by
// internal/plugins/calendar/weather_catalog_test.go against the SAME id set, so
// a new weather type cannot half-land (a renderer with no meta, a console tile
// with no renderer, etc.). If you add a preset, update both sides.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './harness.mjs';

// The 42 Calendaria preset ids (the sync contract) + the 7 Chronicle-native
// extras Chronicle keeps. MUST match gmWeatherTypes() / the Go contract test.
const CALENDARIA_42 = [
  // Standard (13)
  'clear', 'partly-cloudy', 'cloudy', 'overcast', 'drizzle', 'rain', 'fog', 'mist',
  'windy', 'sunshower', 'snow', 'sleet', 'heat-wave',
  // Severe (7)
  'thunderstorm', 'blizzard', 'hail', 'tornado', 'hurricane', 'ice-storm', 'monsoon',
  // Environmental (8)
  'ashfall', 'sandstorm', 'luminous-sky', 'sakura-bloom', 'autumn-leaves', 'rolling-fog',
  'wildfire-smoke', 'dust-devil',
  // Fantasy (14)
  'black-sun', 'ley-surge', 'aether-haze', 'nullfront', 'permafrost-surge', 'gravewind',
  'veilfall', 'arcane-winds', 'acid-rain', 'blood-rain', 'meteor-shower', 'spore-cloud',
  'divine-light', 'plague-miasma',
];
const CHRONICLE_EXTRAS = [
  'heavy-rain', 'snow-flurries', 'ember-rain', 'falling-leaves', 'pollen-drift',
  'fireflies', 'miasma',
];
const WEATHER_IDS = [...CALENDARIA_42, ...CHRONICLE_EXTRAS];

const WEATHER_CATEGORIES = new Set([
  'standard-weather', 'severe-weather', 'environmental-weather', 'fantasy-weather',
]);

function mockCtx() {
  const grad = { addColorStop() {} };
  return {
    fillStyle: '', strokeStyle: '', lineWidth: 1, lineCap: 'butt', globalAlpha: 1,
    globalCompositeOperation: 'source-over', shadowBlur: 0, shadowColor: '', filter: 'none',
    imageSmoothingEnabled: true,
    createLinearGradient: () => grad, createRadialGradient: () => grad,
    fillRect() {}, strokeRect() {}, clearRect() {}, drawImage() {},
    beginPath() {}, closePath() {}, moveTo() {}, lineTo() {}, arc() {}, ellipse() {},
    fill() {}, stroke() {}, save() {}, restore() {}, translate() {}, rotate() {}, scale() {}, clip() {},
    getContext: () => mockCtx(),
  };
}

test('exactly 42 Calendaria ids + Chronicle extras, no overlap', () => {
  assert.equal(CALENDARIA_42.length, 42, 'the contract is 42 Calendaria presets');
  const set = new Set(WEATHER_IDS);
  assert.equal(set.size, WEATHER_IDS.length, 'no duplicate ids across contract + extras');
  for (const x of CHRONICLE_EXTRAS) {
    assert.ok(!CALENDARIA_42.includes(x), x + ' is a Chronicle extra, not a Calendaria id');
  }
});

test('every weather id has BOTH a SKY_FX renderer and SKY_FX_META', () => {
  const w = boot();
  const SKY_FX = w.__calSkyFx, META = w.__calSkyFxMeta;
  assert.ok(SKY_FX && META, 'SKY_FX + SKY_FX_META exposed');
  for (const id of WEATHER_IDS) {
    assert.ok(SKY_FX[id], 'SKY_FX missing renderer for ' + id);
    assert.ok(Array.isArray(SKY_FX[id].back) && Array.isArray(SKY_FX[id].front), id + ' renderer has back/front layer arrays');
    assert.ok(META[id], 'SKY_FX_META missing entry for ' + id);
    assert.ok(META[id].name && META[id].category && META[id].sand && META[id].glyph, id + ' meta has name/category/sand/glyph');
  }
});

test('every weather-category SKY_FX_META id is in the contract (no stray weather meta)', () => {
  const w = boot();
  const META = w.__calSkyFxMeta;
  const contract = new Set(WEATHER_IDS);
  for (const id of Object.keys(META)) {
    if (WEATHER_CATEGORIES.has(META[id].category)) {
      assert.ok(contract.has(id), 'SKY_FX_META weather id ' + id + ' (' + META[id].category + ') is not in the weather contract');
    }
  }
});

test('every weather-category SKY_FX_META id has a SKY_FX renderer and vice-versa', () => {
  const w = boot();
  const SKY_FX = w.__calSkyFx, META = w.__calSkyFxMeta;
  // meta(weather) → renderer
  for (const id of Object.keys(META)) {
    if (WEATHER_CATEGORIES.has(META[id].category)) {
      assert.ok(SKY_FX[id], 'weather meta ' + id + ' has no SKY_FX renderer');
    }
  }
  // renderer → meta (every contract renderer has meta; checked above) — also
  // guard that no contract id renders without meta via the EFFECTS registry.
  const fx = w.__calEffects;
  for (const id of WEATHER_IDS) {
    assert.ok(fx[id], 'EFFECTS missing ' + id);
    assert.equal(typeof fx[id].skyBand, 'function', id + ' has a composed skyBand factory');
    assert.ok(fx[id].hgSand && typeof fx[id].hgSand.color === 'string', id + ' has hgSand.color');
  }
});

test('every weather renderer composes frames that run (live + static dt=0)', () => {
  const w = boot();
  const SKY_FX = w.__calSkyFx;
  for (const id of WEATHER_IDS) {
    const fx = SKY_FX[id];
    const factories = [].concat(fx.back, fx.front);
    assert.doesNotThrow(() => {
      for (const make of factories) {
        const frame = make();
        const ctx = mockCtx();
        for (let i = 0; i < 4; i++) frame(ctx, 320, 200, 0.016, i * 0.016);
        frame(ctx, 320, 200, 0, 0.1); // static / reduced-motion frame
      }
    }, id + ' renderer threw');
  }
});
