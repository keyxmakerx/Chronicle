// worldstate.test.mjs — Wave 0 runtime test for cal-almanac.js.
//
// The almanac JS is a browser IIFE, so we run it inside a node `vm` context
// with a minimal DOM stub (every querySelector returns null → the render
// pipelines no-op safely; we only exercise the world-state pub/sub logic).
// This is the runtime complement to the Go static-assertion tests: it proves
// setWorldState's changedKeys diffing (incl. the no-op-patch guard) and the
// unified EFFECTS projection actually behave at runtime.
//
// Run: node --test test/js/   (or `make test-js`)

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'cal-almanac.js');

// Arrays returned from the vm context carry the sandbox realm's
// Array.prototype, which deepStrictEqual rejects. Copy to a host array of
// (primitive) strings before comparing.
const arr = (x) => Array.from(x);

// Minimal mock matching what the seed + lookups read.
const MOCK = {
  current_year: 1, current_month: 1, current_day: 1, sky_time: 0.5,
  seasons: [{ start: 1, name: 'Spring' }],
  calendar: { hours_per_day: 24 },
  moons: [{ id: 1, phase_offset: 0.1, size: 1 }],
  day_weather: {}, celestial_events: {},
  weather_types: [], weather_effects: [], celestial_effects: [],
};

// Build a fresh, fully-initialised sandbox (the IIFE auto-runs init()).
function boot() {
  const dataEl = {
    getAttribute: (n) => (n === 'data-cal-almanac-data' ? JSON.stringify(MOCK) : null),
    textContent: '',
  };
  const node = () => ({
    className: '', style: { setProperty() {} },
    setAttribute() {}, getAttribute() { return null; }, removeAttribute() {},
    appendChild() {}, replaceWith() {}, addEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    getBoundingClientRect() { return { left: 0, top: 0, width: 1080, height: 200 }; },
  });
  const sandbox = {
    console,
    navigator: { hardwareConcurrency: 8 },
    devicePixelRatio: 1,
    matchMedia: () => ({ matches: false }),
    requestAnimationFrame: () => 0,
    cancelAnimationFrame: () => {},
    setTimeout, clearTimeout,
    scrollX: 0, scrollY: 0,
    document: {
      readyState: 'complete',
      getElementById: (id) => (id === 'cal-almanac-data' ? dataEl : null),
      querySelector: () => null,
      querySelectorAll: () => [],
      addEventListener: () => {},
      createElement: () => node(),
    },
  };
  sandbox.window = sandbox;   // browser code references `window.…`
  sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'cal-almanac.js' });
  return sandbox.window;
}

test('all init blocks succeed under the DOM stub', () => {
  const w = boot();
  assert.ok(Array.isArray(w.__calAlmanacResults), 'init results recorded');
  const failed = arr(w.__calAlmanacResults).filter((r) => r.status !== 'OK');
  assert.equal(failed.length, 0, 'no init block failed: ' + JSON.stringify(failed));
});

test('worldState is seeded with the CATALOG Part 8 shape', () => {
  const w = boot();
  const ws = w.__calWorldState;
  assert.ok(ws, 'worldState seeded');
  for (const k of ['timeOfDay', 'season', 'date', 'sun', 'moons', 'weather', 'events', 'moodTint', 'timeControl']) {
    assert.ok(k in ws, 'worldState has key: ' + k);
  }
  assert.equal(ws.timeOfDay, 0.5);
  assert.equal(ws.weather.type, 'clear');
  assert.equal(ws.timeControl.direction, 1);
  assert.equal(ws.moons.length, 1);
});

test('unified EFFECTS projects both registries with per-surface fields', () => {
  const w = boot();
  const fx = w.__calEffects;
  assert.ok(fx, 'EFFECTS built');
  for (const id of ['clear', 'rain', 'snow', 'thunderstorm', 'fog', 'meteor-shower', 'eclipse-solar', 'eclipse-lunar']) {
    assert.ok(fx[id], 'EFFECTS has ' + id);
    assert.ok('skyBand' in fx[id], id + ' has skyBand');
    assert.ok('hgTop' in fx[id], id + ' has hgTop');
    assert.ok('hgBottom' in fx[id], id + ' has hgBottom');
    assert.equal(typeof fx[id].hgSand, 'function', id + ' has hgSand delegate');
    assert.ok('timeline' in fx[id], id + ' has timeline');
  }
  assert.equal(fx.rain.category, 'weather');
  assert.equal(fx['meteor-shower'].category, 'celestial');
});

test('setWorldState returns changedKeys and notifies subscribers', () => {
  const w = boot();
  const seen = [];
  w.__calSubscribeWorldState((st, changed) => seen.push(arr(changed)));
  const changed = arr(w.__calSetWorldState({ timeOfDay: 0.8 }));
  assert.deepEqual(changed, ['timeOfDay']);
  assert.equal(w.__calWorldState.timeOfDay, 0.8);
  assert.equal(seen.length, 1, 'subscriber fired once');
  assert.deepEqual(seen[0], ['timeOfDay']);
});

test('no-op patch (same value) changes nothing and notifies nobody', () => {
  const w = boot();
  w.__calSetWorldState({ timeOfDay: 0.8 });          // move once
  const seen = [];
  w.__calSubscribeWorldState((st, changed) => seen.push(arr(changed)));
  const a = arr(w.__calSetWorldState({ timeOfDay: 0.8 })); // same → no-op
  assert.deepEqual(a, [], 'timeOfDay no-op → empty changedKeys');
  // weather already 'clear' from seed → no-op even via a partial patch
  const b = arr(w.__calSetWorldState({ weather: { type: 'clear' } }));
  assert.deepEqual(b, [], 'weather no-op → empty changedKeys');
  // partial patch that does not move a field is still a no-op (field-merge)
  const c = arr(w.__calSetWorldState({ weather: { intensity: 1 } }));
  assert.deepEqual(c, [], 'weather intensity unchanged → empty changedKeys');
  assert.equal(seen.length, 0, 'no subscriber fired for any no-op');
});

test('resolveLayers returns active layers in back→front order', () => {
  const w = boot();
  assert.deepEqual(arr(w.__calLayerOrder),
    ['timeOfDay', 'season', 'celestial', 'weather', 'events', 'moodTint', 'timeControl']);
  // Seeded state: time + season + celestial(sun/moons) active; weather is
  // 'clear' (skipped), no events, no mood, default time-control.
  assert.deepEqual(arr(w.__calResolveLayers(w.__calWorldState)),
    ['timeOfDay', 'season', 'celestial']);
  // Turn weather on → weather layer appears, still in order.
  w.__calSetWorldState({ weather: { type: 'rain' } });
  assert.deepEqual(arr(w.__calResolveLayers(w.__calWorldState)),
    ['timeOfDay', 'season', 'celestial', 'weather']);
});

test('a real weather patch is detected exactly once', () => {
  const w = boot();
  const first = arr(w.__calSetWorldState({ weather: { type: 'rain' } }));
  assert.deepEqual(first, ['weather']);
  assert.equal(w.__calWorldState.weather.type, 'rain');
  assert.equal(w.__calWorldState.weather.intensity, 1, 'untouched field preserved');
  const again = arr(w.__calSetWorldState({ weather: { type: 'rain' } }));
  assert.deepEqual(again, [], 'second identical patch is a no-op');
});
