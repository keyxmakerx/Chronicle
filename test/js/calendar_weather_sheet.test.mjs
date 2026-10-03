// calendar_weather_sheet.test.mjs — pins the Generate sheet's rules in
// calendar_weather_sheet.js, driven through its own methods with the real
// generator (chronicle_gen.js) and calendar maths (calendar_view.js):
// painted days are never in what Apply saves and never previewed over,
// kept days survive "Reroll the rest", the same seed gives the same
// weather, Apply's write is the per-day API's shape marked generated, and
// the sheet starts from the calendar's stored climate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, '..', '..');
const widgets = path.join(root, 'static', 'js', 'widgets');
// A calendar in the API's own shape (what view.cal holds).
const cal = {
  name: 'Test', current_year: 100, current_month: 1, current_day: 1, leap_year_every: 0,
  months: ['Frost', 'Thaw', 'Bloom', 'Sun', 'Harvest', 'Dusk'].map((name, i) => ({ name, days: 30, sort_order: i })),
  weekdays: ['One', 'Two', 'Three', 'Four', 'Five', 'Six'].map((name, i) => ({ name, sort_order: i })),
  moons: [], seasons: [], eras: [],
};

function load() {
  const sandbox = {
    module: { exports: {} }, console,
    Chronicle: { register() {} },
    document: { activeElement: null, querySelector: () => null, addEventListener() {} },
    window: {},
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(path.join(widgets, 'chronicle_gen.js'), 'utf8'), sandbox);
  vm.runInContext(readFileSync(path.join(widgets, 'calendar_view.js'), 'utf8'), sandbox);
  sandbox.module = { exports: {} };
  vm.runInContext(readFileSync(path.join(widgets, 'calendar_weather_sheet.js'), 'utf8'), sandbox);
  return sandbox;
}

const dates = [1, 2, 3, 4, 5, 6].map((d) => ({ year: 100, month: 2, day: d }));
const painted = { year: 100, month: 2, day: 3, source: 'manual', preset_id: 'snow', preset_label: 'Snow', icon: 'snow', color: '#e8eef6' };

function sheet(sb, seed = 'pinned') {
  const { Sheet } = sb.module.exports;
  const s = Object.create(Sheet.prototype);
  s.view = { cal, say() {}, _paintMonth() {}, wxPreview: null };
  s.el = { innerHTML: '', contains: () => false };
  s._climate = 'temperate';
  s._continuity = 0.55;
  s.state = { dates, stored: { '100_2_3': painted }, context: [], seed, nonce: 0, kept: {}, days: {}, summary: '', warnings: [], error: null, showAll: false, busy: false };
  s._generate();
  return s;
}

test('a painted day is kept out of the preview and out of Apply', () => {
  const s = sheet(load());
  assert.equal(s.state.error, null, String(s.state.error));
  assert.ok(!('100_2_3' in s.view.wxPreview));
  assert.equal(Object.keys(s.view.wxPreview).length, 5);
  const days = s._applyDays();
  assert.equal(days.length, 5);
  assert.ok(days.every((d) => d.day !== 3 && d.source === 'generated'));
});

test('Apply sends the per-day write shape', () => {
  const d = sheet(load())._applyDays()[0];
  for (const k of ['year', 'month', 'day', 'source', 'preset_id', 'preset_label', 'icon', 'color', 'temperature_celsius', 'wind_speed_kph', 'wind_direction', 'zone_id', 'description']) assert.ok(k in d, k);
  assert.equal(d.gen, undefined);
});

test('the same seed gives the same weather', () => {
  const a = sheet(load())._applyDays(), b = sheet(load())._applyDays();
  assert.equal(JSON.stringify(a), JSON.stringify(b));
});

test('kept days survive Reroll the rest; the others are redrawn', () => {
  const s = sheet(load());
  const before = JSON.stringify(s.state.days);
  s.state.kept['100_2_1'] = true;
  const kept = JSON.stringify(s.state.days['100_2_1']);
  for (let i = 0; i < 4; i++) s._reroll();
  assert.equal(JSON.stringify(s.state.days['100_2_1']), kept);
  assert.notEqual(JSON.stringify(s.state.days), before);
  assert.equal(s.state.nonce, 4);
  assert.ok(!('100_2_3' in s.view.wxPreview));
});

test('a single-day reroll touches only that day', () => {
  const s = sheet(load());
  const others = ['100_2_1', '100_2_2', '100_2_4', '100_2_6'].map((k) => JSON.stringify(s.state.days[k]));
  for (let i = 0; i < 3; i++) s._reroll(['100_2_5']);
  assert.deepEqual(['100_2_1', '100_2_2', '100_2_4', '100_2_6'].map((k) => JSON.stringify(s.state.days[k])), others);
});

test('the sheet renders cards, marks the painted day and escapes text', () => {
  const s = sheet(load());
  s.state.stored['100_2_3'] = Object.assign({}, painted, { preset_label: '<b>x</b>' });
  s._render();
  const h = s.el.innerHTML;
  assert.match(h, /Generate weather/);
  assert.match(h, /Apply to 5 days/);
  assert.match(h, /painted by hand, stays/);
  assert.match(h, /&lt;b&gt;x&lt;\/b&gt;/);
  assert.equal((h.match(/data-rc="keep"/g) || []).length, 5);
  assert.match(h, /<optgroup label="Magic">.*Ashlands/);
});

test('closing clears the grid preview', () => {
  const s = sheet(load());
  s.el.classList = { contains: () => true, remove() {} };
  s.view.dockEl = { classList: { remove() {} } };
  s.close(true);
  assert.equal(s.view.wxPreview, null);
});

function withSettings(sb, body, ok = true) {
  sb.Chronicle.apiFetch = (url) => {
    sb.lastURL = url;
    return Promise.resolve({ ok, json: () => Promise.resolve(body) });
  };
  const s = sheet(sb);
  s.view.apiBase = '/api/c';
  s._render = () => {};
  s._settingsAsked = false;
  s._picked = false;
  return s;
}
const settle = () => new Promise((r) => setTimeout(r, 0));

test("the calendar's climate becomes the sheet's starting point", async () => {
  const sb = load();
  const s = withSettings(sb, { climate: 'ashlands', continuity: 0.9 });
  const before = JSON.stringify(s.state.days);
  s._loadSettings();
  await settle();
  assert.equal(sb.lastURL, '/api/c/weather/settings');
  assert.equal(s._climate, 'ashlands');
  assert.equal(s._continuity, 0.9);
  assert.notEqual(JSON.stringify(s.state.days), before);
});

test('a climate the owner picked here, or kept days, are not overridden', async () => {
  const sb = load();
  const s = withSettings(sb, { climate: 'ashlands', continuity: 0.9 });
  s._picked = true;
  s._loadSettings();
  await settle();
  assert.equal(s._climate, 'temperate');

  const k = withSettings(load(), { climate: 'desert', continuity: 0.2 });
  k.state.kept['100_2_1'] = true;
  const before = JSON.stringify(k.state.days);
  k._loadSettings();
  await settle();
  assert.equal(k._climate, 'desert');
  assert.equal(JSON.stringify(k.state.days), before);
});

test('an unknown climate or a failed read keeps the defaults', async () => {
  const s = withSettings(load(), { climate: 'moon', continuity: 7 });
  s._loadSettings();
  await settle();
  assert.equal(s._climate, 'temperate');
  assert.equal(s._continuity, 0.55);
  const f = withSettings(load(), null, false);
  f._loadSettings();
  await settle();
  assert.equal(f._climate, 'temperate');
});

test("the settings form's climates are the generator's climates", () => {
  const go = readFileSync(path.join(root, 'internal', 'plugins', 'calendar', 'weather_climates.go'), 'utf8');
  const fromGo = [...go.matchAll(/\{ID: "([^"]+)", Name: "([^"]+)"(, Magic: true)?\}/g)].map((m) => `${m[1]}|${m[2]}|${!!m[3]}`);
  const fromJS = load().ChronicleGen.weather.climates().map((c) => `${c.id}|${c.name}|${c.magic}`);
  assert.ok(fromGo.length > 0);
  assert.equal(fromGo.join(','), fromJS.join(','));
});
