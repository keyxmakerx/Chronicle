// worldstate.test.mjs — Wave 0 runtime tests for cal-almanac.js. Proves
// setWorldState's changedKeys diffing (incl. the no-op-patch guard), the
// unified EFFECTS projection, and the resolveLayers ordering actually behave
// at runtime. Harness (node vm + DOM stub) lives in ./harness.mjs.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot, arr } from './harness.mjs';

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
  const b = arr(w.__calSetWorldState({ weather: { type: 'clear' } }));
  assert.deepEqual(b, [], 'weather no-op → empty changedKeys');
  const c = arr(w.__calSetWorldState({ weather: { intensity: 1 } }));
  assert.deepEqual(c, [], 'weather intensity unchanged → empty changedKeys');
  assert.equal(seen.length, 0, 'no subscriber fired for any no-op');
});

test('resolveLayers returns active layers in back→front order', () => {
  const w = boot();
  assert.deepEqual(arr(w.__calLayerOrder),
    ['timeOfDay', 'season', 'celestial', 'weather', 'events', 'moodTint', 'timeControl']);
  assert.deepEqual(arr(w.__calResolveLayers(w.__calWorldState)),
    ['timeOfDay', 'season', 'celestial']);
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
