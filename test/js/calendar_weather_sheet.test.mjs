// calendar_weather_sheet.test.mjs — pins the Generate sheet's rules in
// calendar_weather_sheet.js, driven through its own methods with the real
// generator (chronicle_gen.js) and calendar maths (calendar_view.js):
// painted days are never in what Apply saves and never previewed over,
// kept days survive "Reroll the rest", the same seed gives the same
// weather, Apply's write is the per-day API's shape marked generated, and
// the sheet starts from the calendar's stored climate and own kinds.

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

const fire = { id: 'fire-rain', name: 'Fire rain', icon: 'rain', color: '#e2552b', like: 'rain', seasons: { winter: 'often', spring: 'often', summer: 'often', autumn: 'often' } };

test("the calendar's climate becomes the sheet's starting point", () => {
  const s = sheet(load());
  s._useSettings({ climate: 'ashlands', continuity: 0.9, kinds: [] });
  assert.equal(s._climate, 'ashlands');
  assert.equal(s._continuity, 0.9);
});

test('a climate the owner picked here is kept; own kinds still arrive', () => {
  const s = sheet(load());
  s._picked = true;
  s._useSettings({ climate: 'ashlands', continuity: 0.9, kinds: [fire] });
  assert.equal(s._climate, 'temperate');
  assert.equal(s._kinds.length, 1);
});

test('an unknown climate or no settings keeps the defaults', () => {
  const s = sheet(load());
  s._useSettings({ climate: 'moon', continuity: 7 });
  assert.equal(s._climate, 'temperate');
  assert.equal(s._continuity, 0.55);
  s._useSettings(null);
  assert.equal(s._climate, 'temperate');
});

test("the owner's own kinds are generated, kept when painted, and a bad one is dropped", () => {
  const sb = load();
  const { ownKinds } = sb.module.exports;
  const bad = { id: 'Bad Id', name: '', icon: 'lava', color: 'red', like: 'nope' };
  assert.equal(ownKinds([fire, bad]).length, 1);
  assert.equal(ownKinds('nope').length, 0);

  const s = sheet(sb);
  s._useSettings({ climate: 'ashlands', continuity: 0.2, kinds: [fire, bad] });
  s._generate();
  assert.equal(s.state.error, null, String(s.state.error));
  // Every season is "often", so a fortnight-plus run is sure to see it.
  const long = Array.from({ length: 30 }, (_, i) => ({ year: 100, month: 3, day: i + 1 }));
  s.state.dates = long;
  s._generate();
  const days = Object.values(s.state.days);
  const own = days.filter((d) => d.preset_id === 'fire-rain');
  assert.ok(own.length > 0, 'no fire rain in 30 days: ' + days.map((d) => d.preset_id).join(','));
  assert.equal(own[0].color, '#e2552b');
  assert.equal(own[0].preset_label, 'Fire rain');

  // A day painted with an own kind stays a lock rather than being dropped.
  s.state.stored = { '100_3_5': { year: 100, month: 3, day: 5, source: 'manual', preset_id: 'fire-rain', preset_label: 'Fire rain', icon: 'rain', color: '#e2552b' } };
  assert.ok(s._locks(false).some((d) => d.preset_id === 'fire-rain'));
});

test("the settings form's climates are the generator's climates", () => {
  const go = readFileSync(path.join(root, 'internal', 'plugins', 'calendar', 'weather_climates.go'), 'utf8');
  const fromGo = [...go.matchAll(/\{ID: "([^"]+)", Name: "([^"]+)"(, Magic: true)?\}/g)].map((m) => `${m[1]}|${m[2]}|${!!m[3]}`);
  const fromJS = load().ChronicleGen.weather.climates().map((c) => `${c.id}|${c.name}|${c.magic}`);
  assert.ok(fromGo.length > 0);
  assert.equal(fromGo.join(','), fromJS.join(','));
});

// The server checks an own kind's "behaves like" and sky effect against its
// own copies of the generator's lists; they must stay the same lists.
function goList(src, name) {
  const block = src.slice(src.indexOf('var ' + name + ' = '), src.indexOf('\n}\n', src.indexOf('var ' + name + ' = ')));
  return [...block.matchAll(/\{ID: "([^"]+)", Label: "([^"]+)"\}/g)].map((m) => m[1] + '|' + m[2]);
}

test("the server's built-in weathers and sky effects are the generator's", () => {
  const go = readFileSync(path.join(root, 'internal', 'plugins', 'calendar', 'weather_kinds.go'), 'utf8');
  const G = load().ChronicleGen;
  const presets = goList(go, 'weatherPresets'), effects = goList(go, 'weatherEffects');
  assert.ok(presets.length > 0 && effects.length > 0);
  assert.equal(presets.join(','), G.weather.presets().map((p) => p.id + '|' + p.label).join(','));
  assert.equal(effects.join(','), G.weather.effects().list.map((e) => e.id + '|' + e.label).join(','));
});
