// calendar_weather_sheet.test.mjs — pins the weather calendar's rules in
// calendar_weather_sheet.js, driven through its own methods with the real
// generator (chronicle_gen.js) and calendar maths (calendar_view.js): a roll
// leaves painted and pinned days alone and touches only the chosen days, the
// same seed gives the same weather, Save's writes are the per-day API's
// shape, a sky event brings its own weather, moon marks sit on the turning
// days, and the panel starts from the calendar's stored climate and kinds.

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

const painted = { year: 100, month: 2, day: 3, source: 'manual', preset_id: 'snow', preset_label: 'Snow', icon: 'snow', color: '#e8eef6' };
const eclipse = { id: 9, name: 'The Dark Noon', year: 100, month: 2, day: 10, payload: '{"type":"eclipse","moons":[1]}' };

function panel(sb, stored = { '2_3': painted }, events = []) {
  const { Panel } = sb.module.exports;
  const p = Object.create(Panel.prototype);
  p.view = {
    cal, weatherByYear: { 100: stored }, say() {}, _paintMonth() {}, wxPreview: null, mainMoon: () => null,
    eventsOnDay: (y, m, d) => events.filter((e) => e.year === y && e.month === m && e.day === d),
  };
  p._climate = 'temperate';
  p._continuity = 0.55;
  p._skyOwn = true;
  p._kinds = [];
  p._presets = {};
  sb.ChronicleGen.weather.presets([]).forEach((x) => { p._presets[x.id] = x; });
  p.state = { y: 100, m: 2, draft: {}, pins: {}, sel: {}, added: [] };
  return p;
}
const day = (n) => '100_2_' + n;

test('a roll leaves the painted day alone and drafts the rest as generated', () => {
  const p = panel(load());
  const out = p.roll('pinned');
  assert.equal(out.error, undefined, String(out.error));
  assert.equal(out.rolled.length, 29);
  assert.ok(!(day(3) in p.state.draft));
  const ch = p.changes();
  assert.equal(ch.put.length, 29);
  assert.ok(ch.put.every((d) => d.day !== 3 && d.source === 'generated'));
  assert.equal(ch.clear.length + ch.lock.length + ch.unlock.length, 0);
});

test("Save sends the per-day write shape, without the generator's notes", () => {
  const p = panel(load());
  p.roll('pinned');
  const d = p.changes().put[0];
  for (const k of ['year', 'month', 'day', 'source', 'preset_id', 'preset_label', 'icon', 'color', 'temperature_celsius', 'wind_speed_kph', 'wind_direction', 'zone_id', 'description']) assert.ok(k in d, k);
  assert.equal(d.gen, undefined);
});

test('the same seed gives the same weather', () => {
  const a = panel(load()), b = panel(load());
  a.roll('pinned');
  b.roll('pinned');
  assert.equal(JSON.stringify(a.changes()), JSON.stringify(b.changes()));
});

test('only the chosen days roll, and a pinned one among them stays', () => {
  const p = panel(load(), { '2_3': painted, '2_6': { ...painted, day: 6, source: 'generated' } });
  [5, 6, 7, 8].forEach((n) => { p.state.sel[day(n)] = true; });
  p.togglePins([day(6)]);
  const out = p.roll('pinned');
  assert.deepEqual([...out.rolled].sort(), [day(5), day(7), day(8)]);
  assert.equal(out.left, 1);
  assert.deepEqual(Object.keys(p.state.draft).sort(), [day(5), day(7), day(8)]);
});

test('a pinned day with weather is saved as a lock', () => {
  const p = panel(load());
  p.roll('pinned');
  p.togglePins([day(8)]);
  const ch = p.changes();
  assert.deepEqual([...ch.lock.map((d) => d.day)], [8]);
});

test('rolling over an unpinned locked day or an erased painted day clears it first, so the write lands', () => {
  const locked = { year: 100, month: 2, day: 5, source: 'generated', locked: true, preset_id: 'snow', preset_label: 'Snow', icon: 'snow', color: '#e8eef6' };
  const p = panel(load(), { '2_3': painted, '2_5': locked });
  p.state.sel[day(5)] = true;
  p.togglePins([day(5)]);
  p.roll('pinned');
  let ch = p.changes();
  assert.deepEqual([...ch.put.map((d) => d.day)], [5]);
  assert.deepEqual([...ch.clear.map((d) => d.day)], [5]);
  assert.equal(ch.lock.length + ch.unlock.length, 0);

  const q = panel(load());
  q.state.sel[day(3)] = true;
  q.clear([day(3)]);
  q.roll('pinned');
  ch = q.changes();
  assert.equal(ch.put[0].source, 'generated');
  assert.deepEqual([...ch.clear.map((d) => d.day)], [3]);
});

test('only days with weather can be pinned', () => {
  const p = panel(load(), {});
  assert.equal(p.togglePins([day(8)]), null);
  assert.equal(p.dirty(), false);
  p.paint('rain', [day(9)]);
  assert.equal(p.togglePins([day(8), day(9)]), true);
  assert.equal(p._pinned(day(8)), false);
  assert.equal(p._pinned(day(9)), true);
});

test('painting drafts a hand-painted reading, and clearing only clears stored weather', () => {
  const p = panel(load());
  assert.equal(p.paint('rain', [day(1), day(2)]), 2);
  assert.equal(p.state.draft[day(1)].w.source, 'manual');
  assert.equal(p.clear([day(1), day(3), day(4)]), 2);
  const ch = p.changes();
  assert.deepEqual([...ch.put.map((d) => d.day)], [2]);
  assert.equal(ch.put[0].preset_id, 'rain');
  assert.deepEqual([...ch.clear.map((d) => d.day)], [3]);
  assert.ok(!(day(4) in p.state.draft));
});

test("a sky event brings its own weather unless that is switched off; a painted day keeps its own", () => {
  const sb = load();
  const p = panel(sb, { '2_3': painted }, [eclipse, { ...eclipse, day: 3 }]);
  p.roll('pinned');
  assert.equal(p.state.draft[day(10)].w.preset_id, 'black-sun');
  assert.ok(!(day(3) in p.state.draft));
  const q = panel(sb, { '2_3': painted }, [eclipse]);
  q._skyOwn = false;
  q.roll('pinned');
  assert.notEqual(q.state.draft[day(10)].w.preset_id, 'black-sun');
});

test('moon marks sit on the one day nearest each full and new moon', () => {
  const { moonPeaks } = load().module.exports;
  const cycle = 28, phase = (a) => { const r = a / cycle; return r - Math.floor(r); };
  const abs = Array.from({ length: 56 }, (_, i) => i + 1);
  const peaks = moonPeaks(phase, abs, cycle);
  const marks = Object.keys(peaks).map((i) => abs[i] + ':' + peaks[i]);
  assert.deepEqual(marks, ['14:full', '28:new', '42:full', '56:new']);
});

test('the sky marks the strongest moon night of a day and ignores other payloads', () => {
  const p = panel(load(), {}, [eclipse, { ...eclipse, payload: '{"type":"harvest"}' }, { ...eclipse, day: 11, payload: 'not json' }, { ...eclipse, day: 12, payload: '{"type":"party"}' }]);
  const sky = p._sky();
  assert.equal(sky[day(10)].type, 'eclipse');
  assert.equal(sky[day(10)].name, 'The Dark Noon');
  assert.equal(sky[day(11)], undefined);
  assert.equal(sky[day(12)], undefined);
});

test('a day cell names its weather, season, sky and pin, escaped', () => {
  const p = panel(load(), { '2_3': { ...painted, preset_label: '<b>x</b>', locked: true } }, [{ ...eclipse, day: 3, name: '<i>y</i>' }]);
  p._sky();
  const h = p._cellHTML(day(3), day(3));
  assert.match(h, /&lt;b&gt;x&lt;\/b&gt;, painted by hand/);
  assert.match(h, /Eclipse: &lt;i&gt;y&lt;\/i&gt;/);
  assert.match(h, /class="wxc-d kept/);
  assert.match(h, /tabindex="0"/);
  assert.match(h, /fa-circle-half-stroke/);
  assert.doesNotMatch(h, /<b>x/);
});

test('the calendar behind previews the draft; nothing unsaved means not dirty', () => {
  const p = panel(load());
  assert.equal(p.dirty(), false);
  p.paint('fog', [day(7)]);
  assert.equal(p.dirty(), true);
  p._preview();
  assert.equal(p.view.wxPreview[day(7)].preset_id, 'fog');
  p.state.draft = {};
  assert.equal(p.dirty(), false);
});

const fire = { id: 'fire-rain', name: 'Fire rain', icon: 'rain', color: '#e2552b', like: 'rain', seasons: { winter: 'often', spring: 'often', summer: 'often', autumn: 'often' } };

test("the calendar's climate becomes the sheet's starting point", () => {
  const s = panel(load());
  s._useSettings({ climate: 'ashlands', continuity: 0.9, kinds: [] });
  assert.equal(s._climate, 'ashlands');
  assert.equal(s._continuity, 0.9);
});

test('a climate the owner picked here is kept; own kinds still arrive', () => {
  const s = panel(load());
  s._picked = true;
  s._useSettings({ climate: 'ashlands', continuity: 0.9, kinds: [fire] });
  assert.equal(s._climate, 'temperate');
  assert.equal(s._kinds.length, 1);
});

test('an unknown climate or no settings keeps the defaults', () => {
  const s = panel(load());
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

  const s = panel(sb, {});
  s._useSettings({ climate: 'ashlands', continuity: 0.2, kinds: [fire, bad] });
  sb.ChronicleGen.weather.presets(s._kinds).forEach((x) => { s._presets[x.id] = x; });
  // Every season is "often", so a month-long run is sure to see it.
  const out = s.roll('pinned');
  assert.equal(out.error, undefined, String(out.error));
  const days = Object.values(s.state.draft).map((d) => d.w);
  const own = days.filter((d) => d.preset_id === 'fire-rain');
  assert.ok(own.length > 0, 'no fire rain in 30 days: ' + days.map((d) => d.preset_id).join(','));
  assert.equal(own[0].color, '#e2552b');
  assert.equal(own[0].preset_label, 'Fire rain');

  // A day painted with an own kind can be painted from the palette.
  assert.equal(s.paint('fire-rain', [day(5)]), 1);
  assert.equal(s.state.draft[day(5)].w.preset_id, 'fire-rain');
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
