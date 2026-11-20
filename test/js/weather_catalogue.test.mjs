// weather_catalogue.test.mjs — the built-in weathers are one catalogue kept in
// two scripts: chronicle_gen.js decides when a weather happens and how a day
// reads, sky_looks.js decides how it is drawn. A weather added to one and not
// the other is rolled but drawn as a clear sky, or drawn but never rolled, and
// neither shows up as an error. These tests fail instead.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const dir = join(here, '..', '..', 'static', 'js', 'widgets');
const genSrc = readFileSync(join(dir, 'chronicle_gen.js'), 'utf8');

function loadGen() {
  const sandbox = { module: { exports: {} }, console };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(genSrc, sandbox, { filename: 'chronicle_gen.js' });
  return sandbox.ChronicleGen;
}
function loadSky() {
  const sandbox = { console, Math, JSON, Object, Array, Number, String, Float32Array, Uint8Array, Uint8ClampedArray, Error };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of ['sky_world.js', 'sky_looks.js', 'sky_events.js']) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox, { filename: f });
  return sandbox;
}
// The generator's preset table as written, for the columns its API does not hand out.
function presetRows() {
  const block = genSrc.slice(genSrc.indexOf('var PRESET_ROWS = {') + 18, genSrc.indexOf('\n};', genSrc.indexOf('var PRESET_ROWS = {')) + 2);
  return vm.runInNewContext('(' + block + ')');
}
const cal = {
  name: 'Test', current_year: 100, current_month: 1, current_day: 1, leap_year_every: 0,
  months: Array.from({ length: 12 }, (_, i) => ({ name: 'M' + i, days: 30, sort_order: i })),
  weekdays: Array.from({ length: 7 }, (_, i) => ({ name: 'D' + i, sort_order: i })),
  moons: [], seasons: [], eras: [],
};

test('every built-in weather is drawn by the effect the generator names for it', () => {
  const G = loadGen(), { SkyLooks } = loadSky();
  const presets = G.weather.presets();
  assert.equal(Object.keys(SkyLooks.PRESET_EFFECT).sort().join(','), presets.map((p) => p.id).sort().join(','));
  for (const p of presets) {
    assert.equal(SkyLooks.PRESET_EFFECT[p.id], p.effect, p.id);
    assert.ok(SkyLooks.EFFECTS[p.effect], p.id + ' draws with an effect the sky has: ' + p.effect);
  }
});

test("the sky's cloud, wetness and rain for a weather are the generator's", () => {
  const rows = presetRows(), { SkyLooks } = loadSky();
  for (const id of Object.keys(rows)) {
    const r = rows[id];
    assert.deepEqual(Array.from(SkyLooks.PHYS[id] || []), [r[4], r[5], r[6], r[8], r[9]], id);
  }
});

test("the sky's effect list is the generator's, name for name", () => {
  const G = loadGen(), { SkyLooks } = loadSky();
  const list = G.weather.effects().list;
  assert.equal(list.map((e) => e.id).sort().join(','), SkyLooks.VOCAB.slice().sort().join(','));
  // Calendaria's effects keep its names in the generator; Chronicle's own are named once.
  for (const e of list) if (!e.calendaria && !['motes', 'sparks', 'sigils'].includes(e.id)) assert.equal(SkyLooks.EFFECTS[e.id].n, e.label, e.id);
  const groups = SkyLooks.GROUPS.map((g) => g[0]);
  for (const e of list) assert.ok(groups.includes(e.group), e.id + ' is in a group the look editor shows: ' + e.group);
});

test('every climate compiles, and every built-in weather can happen somewhere', () => {
  const G = loadGen();
  const seen = new Set();
  for (const c of G.weather.climates()) {
    const z = G.weather.zonePayload(c.id);
    assert.ok(z.payload.presets.length > 4, c.id + ' has weather');
    z.payload.presets.forEach((p) => seen.add(p.id));
  }
  const never = G.weather.presets().map((p) => p.id).filter((id) => !seen.has(id));
  assert.equal(never.join(', '), '', 'weathers no climate or spell of weather can bring');
});

test('every built-in weather that turns up reads in its own words, short enough for a day cell', () => {
  const G = loadGen();
  const days = [];
  for (let y = 100; y <= 102; y++) for (let m = 1; m <= 12; m++) for (let d = 1; d <= 30; d++) days.push({ year: y, month: m, day: d });
  const labels = {}, seen = new Set();
  G.weather.presets().forEach((p) => { labels[p.id] = p.label; });
  for (const c of G.weather.climates()) {
    const out = G.run('weather', { calendar: cal, seed: 'words', recipe: { generator: 'weather', details: { climate: c.id } }, scope: { days } });
    for (const d of out.days) {
      seen.add(d.preset_id);
      assert.ok(d.description && d.description.length <= 34, c.id + ' ' + d.preset_id + ': "' + d.description + '"');
      assert.notEqual(d.description, labels[d.preset_id], d.preset_id + ' has no reading of its own');
    }
  }
  assert.ok(seen.size >= 70, 'three years in every climate bring most of the catalogue, not ' + seen.size + ' weathers');
});

test('the weather that needs cold or warmth keeps to it', () => {
  const G = loadGen();
  const days = [];
  for (let m = 1; m <= 12; m++) for (let d = 1; d <= 30; d++) days.push({ year: 100, month: m, day: d });
  const cold = { 'hard-frost': 2.5, 'bitter-cold': -5, flurries: 3, thundersnow: 2, glimmersnow: 3 };
  const warm = { 'firefly-dusk': 10, 'butterfly-days': 10, 'pollen-haze': 10, 'locust-swarm': 10 };
  for (const climate of ['temperate', 'taiga', 'frostfell', 'fey-wilds', 'steppe', 'bayou']) {
    for (const seed of ['a', 'b']) {
      const out = G.run('weather', { calendar: cal, seed, recipe: { generator: 'weather', details: { climate } }, scope: { days } });
      for (const d of out.days) {
        const T = d.temperature_celsius, where = climate + ' ' + d.month + '/' + d.day + ' ' + d.preset_id + ' at ' + T;
        if (cold[d.preset_id] != null) assert.ok(T <= cold[d.preset_id] + 0.5, where);
        if (warm[d.preset_id] != null) assert.ok(T >= warm[d.preset_id] - 0.5, where);
      }
    }
  }
});

// A sky with the sun at a given height, as much of the frame state as placing a look's shapes reads.
function skyAt(sandbox, look, sunAltDeg, dark) {
  const a = sunAltDeg * Math.PI / 180, az = Math.PI * .75;
  return {
    W: 600, H: 150, L: { hor: 120, top: 0 }, look, dark, wx: { cloud: .3, fog: 0 }, t: 3, rm: false, moons: [],
    sl: { night: dark, col: [1, 1, 1], amb: .3 }, sun: { alt: a, az, v: [Math.sin(az) * Math.cos(a), Math.sin(a), Math.cos(az) * Math.cos(a)] },
  };
}
function shapesFor(sandbox, id, sunAltDeg, dark) {
  const { SkyLooks, SkyEvents } = sandbox;
  const st = skyAt(sandbox, SkyLooks.resolve(SkyLooks.spec({ preset_id: id, preset_label: id })), sunAltDeg, dark);
  SkyEvents.apply(st, []);
  return st.fxItems;
}

test('a rainbow needs a low sun, and a halo needs something to ring', () => {
  const sky = loadSky();
  assert.equal(shapesFor(sky, 'rainbow', 15, 0).filter((f) => f.shape === 'rainbow').length, 1, 'a bow with the sun low');
  assert.equal(shapesFor(sky, 'rainbow', 60, 0).filter((f) => f.shape === 'rainbow').length, 0, 'none under a high sun');
  assert.equal(shapesFor(sky, 'rainbow', -20, 1).filter((f) => f.shape === 'rainbow').length, 0, 'none at night');
  assert.equal(shapesFor(sky, 'sun-halo', 40, 0).filter((f) => f.shape === 'sunring').length, 1, 'a ring round the sun');
  assert.equal(shapesFor(sky, 'sun-halo', -30, 1).filter((f) => f.shape === 'sunring').length, 0, 'no ring with no sun or moon up');
  assert.ok(shapesFor(sky, 'night-clouds', -12, 1).some((f) => f.shape === 'streak' && f.k > .1), 'night-shining clouds after dark');
  assert.ok(shapesFor(sky, 'night-clouds', 40, 0).every((f) => !(f.shape === 'streak' && f.k > .01)), 'and none by day');
});

test('every look resolves to numbers the painter can use', () => {
  const sky = loadSky(), { SkyLooks } = sky;
  for (const id of SkyLooks.VOCAB) {
    const look = SkyLooks.resolve({ id: 'x-' + id, like: 'clear', builtin: false, label: id, look: { effect: id, strength: 1 } });
    for (const key of ['cloud', 'dark', 'fog', 'rain', 'snow', 'hail', 'storm', 'wind']) {
      if (look[key] != null) assert.ok(Number.isFinite(look[key]), id + '.' + key + ' is ' + look[key]);
    }
    for (const list of ['air', 'parts', 'shapes']) assert.ok(Array.isArray(look[list]), id + ' has a list of ' + list);
    look.shapes.forEach((q) => assert.ok(Number.isFinite(q.k), id + ' shape strength'));
  }
});
