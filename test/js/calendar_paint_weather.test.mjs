// calendar_paint_weather.test.mjs — pins the weather calendar's palette in
// calendar_weather_sheet.js: its six common kinds and the More weather
// groups both match the generator's own weather list (the owner's own kinds
// first), the list's search and escaping; and, in calendar_editor.js, a
// stored reading flattens back to the write shape Undo sends (source kept).

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
  const editor = sandbox.module.exports;
  sandbox.module = { exports: {} };
  vm.runInContext(readFileSync(path.join(widgets, 'calendar_weather_sheet.js'), 'utf8'), sandbox);
  return { ...editor, ...sandbox.module.exports };
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
  assert.equal(PAINT_GROUPS.map((g) => g[1]).join(','), 'Yours,Common,Stormy,Nature,Magic');
});

test("the owner's own kinds lead the More weather list, in their own colour", () => {
  const { moreListHTML: paintListHTML } = load();
  const fire = { id: 'fire-rain', name: 'Fire rain', icon: 'rain', color: '#e2552b', like: 'rain', seasons: { summer: 'often' } };
  const mine = Object.fromEntries(G.weather.presets([fire]).map((p) => [p.id, p]));
  const html = paintListHTML(mine, '');
  assert.ok(html.indexOf('>Yours<') >= 0 && html.indexOf('>Yours<') < html.indexOf('>Common<'));
  assert.match(html, /data-wx-pick="fire-rain" style="--wc:#e2552b"/);
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
  const { moreListHTML: paintListHTML } = load();
  const all = paintListHTML(byId, '');
  for (const name of ['Common', 'Stormy', 'Nature', 'Magic']) assert.match(all, new RegExp('class="wxgt">' + name + '<'));
  const rain = paintListHTML(byId, 'RAIN');
  assert.match(rain, /data-wx-pick="rain"/);
  assert.doesNotMatch(rain, /data-wx-pick="snow"/);
  assert.match(paintListHTML(byId, 'zzz<b>'), /No weather matches “zzz&lt;b&gt;”/);
  const magic = Object.values(byId).find((p) => p.category === 'Fantasy');
  assert.match(paintListHTML(byId, magic.label), /class="wmg">Magic</);
});

test('a palette button is a pressed toggle in the kind colour', () => {
  const { paletteChip } = load();
  const html = paletteChip(byId.rain, true, false);
  assert.match(html, /data-wx="rain" class="" aria-pressed="true" style="--wc:#/);
  assert.match(html, /fa-cloud-rain/);
});

test('weather writes go out in batches of at most 1000 days, in order', async () => {
  const { weatherBatches, WEATHER_BATCH } = load();
  assert.equal(WEATHER_BATCH, 1000);
  const cases = [
    { name: 'empty list sends nothing', n: 0, calls: [] },
    { name: 'exactly one batch', n: 1000, calls: [1000] },
    { name: 'one over splits', n: 1001, calls: [1000, 1] },
    { name: 'a large undo splits in three', n: 2500, calls: [1000, 1000, 500] },
  ];
  for (const tc of cases) {
    const sent = [];
    const apiFetch = async (url, opts) => { sent.push({ url, opts }); return { ok: true }; };
    const list = Array.from({ length: tc.n }, (_, i) => i);
    await weatherBatches(apiFetch, '/api', '/weather/days/lock', 'POST', list, { locked: true });
    assert.deepEqual(sent.map((s) => s.opts.body.days.length), tc.calls, tc.name);
    assert.deepEqual(sent.flatMap((s) => s.opts.body.days), list, tc.name);
    for (const s of sent) {
      assert.equal(s.url, '/api/weather/days/lock');
      assert.equal(s.opts.method, 'POST');
      assert.equal(s.opts.body.locked, true);
    }
  }
});

test('a refused batch stops the rest and rejects', async () => {
  const { weatherBatches } = load();
  let n = 0;
  const apiFetch = async () => ({ ok: ++n < 2 });
  await assert.rejects(weatherBatches(apiFetch, '/api', '/weather/days', 'PUT', new Array(3000).fill(0)));
  assert.equal(n, 2);
});
