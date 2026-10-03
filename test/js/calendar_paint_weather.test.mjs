// calendar_paint_weather.test.mjs — pins the paint tray's pure pieces in
// calendar_editor.js: the palette's six common kinds and the More weather
// groups both match the generator's own weather list, a stored reading
// flattens back to the write shape Undo sends (source kept), and the list's
// search and escaping.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);
const widgets = path.join(here, '..', '..', 'static', 'js', 'widgets');
const G = require(path.join(widgets, 'chronicle_gen.js'));

function load() {
  const view = { module: { exports: {} }, Chronicle: { register() {} }, console };
  vm.createContext(view);
  vm.runInContext(readFileSync(path.join(widgets, 'calendar_view.js'), 'utf8'), view);
  const sandbox = {
    module: { exports: {} },
    Chronicle: view.Chronicle,
    document: { querySelector: () => null, addEventListener() {} },
    window: {},
    console,
  };
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(path.join(widgets, 'calendar_editor.js'), 'utf8'), sandbox);
  return sandbox.module.exports;
}

const byId = Object.fromEntries(G.weather.presets().map((p) => [p.id, p]));

test('the six common kinds are all in the generator list', () => {
  const { PAINT_COMMON } = load();
  assert.equal(PAINT_COMMON.length, 6);
  for (const id of PAINT_COMMON) assert.ok(byId[id], id);
});

test('every generator category has a More weather group', () => {
  const { PAINT_GROUPS } = load();
  const groups = new Set(PAINT_GROUPS.map((g) => g[0]));
  for (const p of Object.values(byId)) assert.ok(groups.has(p.category), p.id + ' ' + p.category);
  assert.equal(PAINT_GROUPS.slice(0, 4).map((g) => g[1]).join(','), 'Common,Stormy,Nature,Magic');
});

test('a stored reading flattens to the write shape, keeping its source', () => {
  const { dayInput } = load();
  const got = dayInput({
    year: 1, month: 2, day: 3, source: 'generated', preset_id: 'rain', preset_label: 'Rain', icon: 'rain', color: '#5b8fc7',
    temperature_celsius: 0, wind: { speed_kph: 12, speed_tier: 'light', direction: 'W', direction_degrees: 270 },
    precipitation: { type: 'rain', intensity: 0.4 },
  });
  assert.equal(got.source, 'generated');
  assert.equal(got.temperature_celsius, 0);
  assert.equal(got.wind_speed_kph, 12);
  assert.equal(got.wind_direction_degrees, 270);
  assert.equal(got.precipitation_type, 'rain');
  assert.equal(got.precipitation_intensity, 0.4);
  assert.equal(got.zone_id, null);
  assert.equal(dayInput({ year: 1, month: 1, day: 1 }).source, 'manual');
});

test('the list groups kinds, searches labels and marks magic', () => {
  const { paintListHTML } = load();
  const all = paintListHTML(byId, '');
  for (const name of ['Common', 'Stormy', 'Nature', 'Magic']) assert.match(all, new RegExp('class="wxgt">' + name + '<'));
  const rain = paintListHTML(byId, 'RAIN');
  assert.match(rain, /data-wx="rain"/);
  assert.doesNotMatch(rain, /data-wx="snow"/);
  assert.match(paintListHTML(byId, 'zzz<b>'), /No weather matches “zzz&lt;b&gt;”/);
  const magic = Object.values(byId).find((p) => p.category === 'Fantasy');
  assert.match(paintListHTML(byId, magic.label), /class="wmg">Magic</);
});

test('a palette button is a radio in the kind colour', () => {
  const { paintButtonHTML } = load();
  const html = paintButtonHTML(byId.rain, true, false);
  assert.match(html, /role="radio" aria-checked="true" data-wx="rain"/);
  assert.match(html, /--wxc:#/);
  assert.match(html, /fa-cloud-rain/);
});
