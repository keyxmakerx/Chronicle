// calendar_place_weather.test.mjs — pins weather by place in the browser:
// the weather calendar rolls a place's days with the place's own climate as
// the generator's zone, reads and writes only that place's days, keeps its
// painted and pinned days, and will not switch place over an unsaved draft;
// the Director's day card lists every place's line with the party's marked,
// and a campaign with no place climates gets no list at all.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';
import { escapeHtml } from './_escape.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const widgets = path.join(here, '..', '..', 'static', 'js', 'widgets');
const cal = {
  name: 'Test', current_year: 100, current_month: 1, current_day: 1, leap_year_every: 0,
  months: ['Frost', 'Thaw', 'Bloom', 'Sun', 'Harvest', 'Dusk'].map((name, i) => ({ name, days: 30, sort_order: i })),
  weekdays: ['One', 'Two', 'Three', 'Four', 'Five', 'Six'].map((name, i) => ({ name, sort_order: i })),
  moons: [], seasons: [], eras: [],
};

function load() {
  const sandbox = {
    module: { exports: {} }, console,
    Chronicle: { escapeHtml, register() {} },
    document: { activeElement: null, querySelector: () => null, addEventListener() {} },
    window: {},
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(path.join(widgets, 'chronicle_gen.js'), 'utf8'), sandbox);
  vm.runInContext(readFileSync(path.join(widgets, 'calendar_view.js'), 'utf8'), sandbox);
  sandbox.viewExports = sandbox.module.exports;
  sandbox.module = { exports: {} };
  vm.runInContext(readFileSync(path.join(widgets, 'calendar_weather_sheet.js'), 'utf8'), sandbox);
  return sandbox;
}

const saltreach = { id: 'p-salt', name: 'Saltreach', climate: 'cold-coast', climate_name: 'Cold coast' };
const dunmoor = { id: 'p-dun', name: 'Dunmoor <b>', climate: 'desert', climate_name: 'Desert' };
const paintedAt = (day, preset = 'snow') => ({ year: 100, month: 2, day, source: 'manual', preset_id: preset, preset_label: preset, icon: preset, color: '#e8eef6' });

// panel builds the weather calendar over a world year and a places year, as
// the editor opens it for the Director.
function panel(sb, { world = {}, placeDays = {}, place = '' } = {}) {
  const { Panel } = sb.module.exports;
  const p = Object.create(Panel.prototype);
  p.view = {
    cal, weatherByYear: { 100: world }, placeWxByYear: { 100: { byDay: placeDays } },
    say() {}, _paintMonth() {}, wxPreview: null, mainMoon: () => null, eventsOnDay: () => [],
  };
  p._climate = 'temperate';
  p._continuity = 0.55;
  p._skyOwn = false;
  p._kinds = [];
  p._presets = {};
  sb.ChronicleGen.weather.presets([]).forEach((x) => { p._presets[x.id] = x; });
  p.state = { y: 100, m: 2, draft: {}, pins: {}, sel: {}, added: [], places: [saltreach, dunmoor], place };
  return p;
}
const day = (n) => '100_2_' + n;

test("a place's roll uses its own climate as the zone and says which place it saves to", () => {
  const sb = load();
  const p = panel(sb, { place: saltreach.id });
  assert.equal(p._recipe().details.climate, 'cold-coast');
  const out = p.roll('seed1');
  assert.equal(out.error, undefined, String(out.error));
  assert.equal(out.rolled.length, 30);
  const ch = p.changes();
  assert.equal(ch.place, saltreach.id);
  assert.ok(ch.put.every((d) => d.zone_id === saltreach.id && d.zone_name === 'Saltreach' && d.source === 'generated'));
});

test("a place reads only its own days: its painted day stays, the world's painted day does not count", () => {
  const sb = load();
  const p = panel(sb, { world: { '2_4': paintedAt(4) }, placeDays: { [saltreach.id]: { '2_3': paintedAt(3, 'fog') } }, place: saltreach.id });
  assert.equal(p._stored(day(3)).preset_id, 'fog');
  assert.equal(p._stored(day(4)), null);
  const out = p.roll('seed1');
  assert.ok(!out.rolled.includes(day(3)));
  assert.ok(out.rolled.includes(day(4)));
});

test('a pinned place day stays through a roll and is saved as a lock on that place', () => {
  const sb = load();
  const gen = { year: 100, month: 2, day: 6, source: 'generated', preset_id: 'rain', preset_label: 'Rain', icon: 'rain', color: '#88a' };
  const p = panel(sb, { placeDays: { [saltreach.id]: { '2_6': gen } }, place: saltreach.id });
  p.togglePins([day(6)]);
  const out = p.roll('seed1');
  assert.ok(!out.rolled.includes(day(6)));
  const ch = p.changes();
  assert.deepEqual([...ch.lock.map((d) => d.day)], [6]);
  assert.equal(ch.place, saltreach.id);
});

test("the same seed gives a place its own chain of weather, not the world's", () => {
  const sb = load();
  const world = panel(sb, { place: '' });
  world._climate = 'cold-coast';
  world.roll('same');
  const place = panel(sb, { place: saltreach.id });
  place.roll('same');
  const a = world.changes().put.map((d) => d.preset_id + d.temperature_celsius).join();
  const b = place.changes().put.map((d) => d.preset_id + d.temperature_celsius).join();
  assert.notEqual(a, b);
  assert.equal(world.changes().place, '');
});

test('switching place over an unsaved draft is refused and the chooser goes back', () => {
  const sb = load();
  const p = panel(sb, { place: '' });
  let said = '';
  p._say = (msg) => { said = msg; };
  p.paint('rain', [day(2)]);
  const sel = { value: saltreach.id };
  p._usePlace(saltreach.id, sel);
  assert.equal(p.state.place, '');
  assert.equal(sel.value, '');
  assert.match(said, /Save or discard/);
});

test('the party mark in force is the latest on or before the day', () => {
  const { partyMarkAt } = load().viewExports;
  const marks = [{ year: 100, month: 2, day: 5, region_id: 'p-salt' }, { year: 100, month: 3, day: 1, region_id: '' }];
  assert.equal(partyMarkAt(marks, 100, 2, 4), null);
  assert.equal(partyMarkAt(marks, 100, 2, 5).region_id, 'p-salt');
  assert.equal(partyMarkAt(marks, 100, 2, 30).region_id, 'p-salt');
  assert.equal(partyMarkAt(marks, 100, 3, 1).region_id, '');
  assert.equal(partyMarkAt(marks, 101, 1, 1).region_id, '');
  assert.equal(partyMarkAt([], 100, 2, 5), null);
});

test("the day's rows list each place, then everywhere else, with the party's line marked", () => {
  const { placeRowsFor, partyRowReading } = load().viewExports;
  const rain = { preset_label: 'Light rain', temperature_celsius: 6 };
  const sun = { preset_label: 'Clear', temperature_celsius: 20 };
  const pw = {
    world: { id: 'temperate', name: 'Temperate' }, places: [saltreach, dunmoor],
    party: [{ year: 100, month: 2, day: 5, region_id: 'p-salt' }], byDay: { 'p-salt': { '2_8': rain } },
  };
  const rows = placeRowsFor(pw, sun, 100, 2, 8);
  assert.deepEqual(rows.map((r) => r.name), ['Saltreach', 'Dunmoor <b>', 'Everywhere else']);
  assert.deepEqual(rows.map((r) => r.here), [true, false, false]);
  assert.equal(rows[0].climate, 'Cold coast');
  assert.equal(rows[2].climate, 'Temperate');
  assert.equal(rows[1].reading, null);
  assert.equal(partyRowReading(rows), rain);
  // Before the party's first mark they are everywhere else.
  assert.equal(partyRowReading(placeRowsFor(pw, sun, 100, 2, 1)), sun);
});

test('no place with a climate of its own means no list, so nothing changes', () => {
  const { placeRowsFor, placeListHTML } = load().viewExports;
  assert.deepEqual([...placeRowsFor({ world: { name: 'Temperate' }, places: [], party: [] }, null, 100, 2, 8)], []);
  assert.deepEqual([...placeRowsFor(undefined, null, 100, 2, 8)], []);
  assert.equal(placeListHTML([]), '');
});

test("the list escapes place names and tells the Director players see only the party's line", () => {
  const { placeRowsFor, placeListHTML } = load().viewExports;
  const pw = { world: { name: 'Temperate' }, places: [dunmoor], party: [{ year: 100, month: 1, day: 1, region_id: 'p-dun' }], byDay: {} };
  const html = placeListHTML(placeRowsFor(pw, null, 100, 2, 8));
  assert.ok(html.includes('Dunmoor &lt;b&gt;'));
  assert.ok(!html.includes('Dunmoor <b>'));
  assert.ok(html.includes('the party is here'));
  assert.ok(html.includes('No weather yet'));
  assert.ok(html.includes('Players see only the party’s line.'));
  assert.ok(html.includes('data-party-move'));
});
