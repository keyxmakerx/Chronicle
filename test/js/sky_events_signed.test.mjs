// sky_events_signed.test.mjs — the signed sky events beyond the moon nights
// (comets, meteor showers, falling stars, aurora nights, solar eclipses, blue
// and hunter's moons, dark nights, an owner's shapes) placed from an event's
// payload, its days and its start hour (static/js/widgets/sky_events.js), and
// carried through the pane's state (sky_pane.js buildState).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const dir = join(here, '..', '..', 'static', 'js', 'widgets');

function load() {
  const sandbox = {
    console, Math, JSON, Object, Array, Number, String, Float32Array, Uint8Array, Uint8ClampedArray, Error, Date, isFinite,
    Chronicle: { register() {} },
    document: { documentElement: { classList: { contains: () => false }, hasAttribute: () => false, getAttribute: () => null } },
    matchMedia: () => ({ matches: false, addEventListener() {} }),
    addEventListener() {}, removeEventListener() {}, setTimeout, clearTimeout,
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of ['sky_world.js', 'sky_looks.js', 'sky_moon.js', 'sky_events.js', 'sky_2d.js', 'sky_pane.js']) {
    vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox, { filename: f });
  }
  return sandbox;
}

const CAL = {
  name: 'Harptos', current_year: 1491, current_month: 3, current_day: 10, current_hour: 22, current_minute: 0,
  months: [{ name: 'Hammer', days: 30 }, { name: 'Alturiak', days: 30 }, { name: 'Ches', days: 30 }], weekdays: [{ name: 'One' }],
  hours_per_day: 24, minutes_per_hour: 60,
  moons: [{ id: 7, name: 'Selûne', cycle_days: 30.4375, phase_offset: 0, color: '#d9e1ec', size: 1 }],
};
const at = (h, d = 10, m = 3) => Object.assign({}, CAL, { current_hour: h, current_day: d, current_month: m });
const ev = (payload, extra = {}) => Object.assign({ id: 1, year: 1491, month: 3, day: 10, payload: JSON.stringify(payload) }, extra);

function state(env, cal, events, t = 3) {
  return env.SkyPane.Scene.state({ calendar: cal, todayEvents: events, dayWeather: null, skym: env.SkyWorld.makeSkym(cal), landSeed: 1, width: 640, height: 160, reduced: false }, t, -1);
}
function finite(list, what) {
  (list || []).forEach((x) => (Array.isArray(x) ? x : Object.values(x)).forEach((v, i) => {
    if (typeof v === 'number') assert.ok(Number.isFinite(v), `${what} value ${i} is ${v}`);
  }));
}

test('the signed types are read from a payload; colours that are not #rrggbb and unknown types are dropped', () => {
  const { SkyEvents } = load();
  const got = SkyEvents.parseDayEvents([
    ev({ type: 'comet', color: 'red; background:url(x)', direction: { compass: 'W', altitude: 'low' }, tail_degrees: 300 }),
    ev({ type: 'aurora', colors: ['#45ffa0', 'nope', '#c35cff', '#ffffff'] }),
    ev({ type: 'party' }),
    { id: 9, payload: 'not json' },
  ]);
  assert.equal(got.length, 2);
  assert.equal(got[0].type, 'comet');
  assert.equal(got[0].sky.color, null);
  assert.equal(got[0].sky.tail, 90, 'a tail is held to 90 degrees');
  assert.deepEqual({ ...got[0].sky.direction }, { compass: 'W', altitude: 'low' });
  assert.deepEqual([...got[1].sky.colors], ['#45ffa0', '#c35cff']);
});

test('a comet on its days is drawn with a tail; before its first day it is not', () => {
  const env = load();
  const comet = ev({ type: 'comet', direction: { compass: 'SW', altitude: 'mid' }, tail_degrees: 30 }, { day: 8, end_year: 1491, end_month: 3, end_day: 20 });
  const st = state(env, at(20), [comet]);
  assert.equal(st.comets.length, 1, 'the comet shows on a night inside its run');
  finite(st.comets, 'comet');
  assert.ok(st.comets[0].k > .05 && st.comets[0].len > 10);
  const before = state(env, at(20, 1), [comet]);
  assert.equal(before.comets.length, 0, 'nothing before its first day');
});

test('a meteor shower falls on its own evening, over the odd shooting star any clear night has, and never hands the painter a non-finite meteor', () => {
  const env = load();
  // Meteors summed over 40 seconds, so the night's own shooting stars are the baseline.
  const count = (cal, events) => { let n = 0; for (let t = 0; t < 40; t += .1) { const st = state(env, cal, events, t); finite(st.met, 'meteor'); n += st.met.length; } return n; };
  const plain = count(at(23), []);
  for (const extra of [{}, { start_hour: 23 }, { start_hour: 22 }]) {
    const shower = ev({ type: 'meteor-shower', direction: { compass: 'NE', altitude: 'high' }, rate_per_hour: 120, color: '#9fd8ff' }, extra);
    const got = count(at(23), [shower]);
    assert.ok(got > plain * 2 + 20, `the shower is falling at 23:00 (${JSON.stringify(extra)}): ${got} meteors against ${plain} without it`);
  }
  const shower = ev({ type: 'meteor-shower', direction: { compass: 'NE', altitude: 'high' }, rate_per_hour: 120 }, { start_hour: 23 });
  assert.equal(state(env, at(12), [shower], 5).met.length, 0, 'no meteors in daylight');
});

test('a total solar eclipse covers the sun at its hour, darkens the sky and moves the moon over it', () => {
  const env = load();
  const ecl = ev({ type: 'eclipse-solar', subtype: 'total', moons: [7], duration_minutes: 4 }, { start_hour: 12, start_minute: 0 });
  const peak = state(env, at(12), [ecl]);
  assert.ok(peak.ecl > .98, 'the sun is covered at totality, got ' + peak.ecl);
  const plain = state(env, at(12), []);
  assert.ok(peak.dark > plain.dark + .5, `the sky darkens (${plain.dark} to ${peak.dark})`);
  const M = peak.moons[0];
  assert.ok(Math.hypot(M.x - peak.sp.x, M.y - peak.sp.y) < peak.R, 'the moon stands on the sun');
  assert.equal(M.m.lit, 0, 'the moon shows its unlit side');
  const later = state(env, at(15), [ecl]);
  assert.equal(later.ecl, 0, 'over after the crossing');
  const annular = state(env, at(12), [ev({ type: 'eclipse-solar', subtype: 'annular', moons: [7] }, { start_hour: 12 })]);
  assert.ok(annular.ecl > .5 && annular.ecl < .9, 'an annular one leaves a ring, got ' + annular.ecl);
});

test('a solar eclipse whose moon is not in this sky (hidden from players) leaves the sun and the sky alone', () => {
  const env = load();
  const ecl = ev({ type: 'eclipse-solar', subtype: 'total', moons: [99] }, { start_hour: 12 });
  const st = state(env, at(12), [ecl]), plain = state(env, at(12), []);
  assert.equal(st.ecl, 0);
  assert.equal(st.dark, plain.dark, 'the noon sky is as bright as without it');
  const moonless = state(env, Object.assign(at(12), { moons: [] }), [ev({ type: 'eclipse-solar', subtype: 'total' }, { start_hour: 12 })]);
  assert.equal(moonless.ecl, 0, 'no moon, no eclipse');
});

test('an untimed wandering star stands near its compass point, open, all day', () => {
  const env = load();
  const st = state(env, at(22), [ev({ type: 'custom', shape: 'star', size: 'large', direction: { compass: 'S', altitude: 'mid' } })]);
  const it = st.fxItems.find((f) => f.shape === 'star');
  assert.ok(it, 'the star is drawn');
  assert.ok(it.elapsed <= 1, 'it has wandered an hour at most, got ' + it.elapsed);
  assert.equal(it.open, 1);
});

test('an aurora night hangs over its compass point and turns the view toward it', () => {
  const env = load();
  const a = ev({ type: 'aurora', direction: { compass: 'N', altitude: 'low' }, colors: ['#45ffa0', '#c35cff'] });
  const st = state(env, at(23), [a]);
  assert.equal(st.aur.length, 1);
  assert.equal(st.aur[0].az, -180, 'north, as degrees from south');
  assert.notEqual(st.cam, 0, 'the view turns away from south');
});

test('a falling star with an hour shows only around that hour; without one it holds all its night', () => {
  const env = load();
  const timed = ev({ type: 'star-fall', direction: { compass: 'E', altitude: 'mid' } }, { start_hour: 21 });
  // Its own trail of embers, not the odd shooting star any clear night has.
  const seen = (cal, e) => { for (let t = 0; t < 16; t += .1) { const st = state(env, cal, [e], t); if (st.fxItems.some((f) => f.shape === 'embers' || f.shape === 'landing')) return true; } return false; };
  assert.ok(seen(at(21), timed));
  assert.ok(!seen(at(23), timed), 'gone two hours later');
  assert.ok(seen(at(23), ev({ type: 'star-fall' })), 'an all-day one shows late at night');
});

test('blue moon, hunter\'s moon and dark night each change the sky', () => {
  const env = load();
  const cal = at(23, 25);
  const blue = state(env, cal, [ev({ type: 'blue-moon', moons: [7] }, { day: 25 })]);
  if (blue.moons[0].up && blue.moons[0].m.alt > 0) assert.ok(blue.fxItems.some((f) => f.shape === 'halo'), 'a halo round the moon');
  const hunt = state(env, cal, [ev({ type: 'hunters-moon', moons: [7] }, { day: 25 })]);
  if (hunt.moons[0].up) assert.ok(hunt.moons[0].warm, 'the hunter\'s moon is warm');
  const dark = state(env, cal, [ev({ type: 'dark-night', intensity: 1 }, { day: 25 })]);
  assert.ok(dark.darkNight > .5);
});

test('an owner\'s shapes become the painter\'s items, comets, curtains and veils', () => {
  const env = load();
  const mk = (shape) => state(env, at(22), [ev({ type: 'custom', shape, size: 'large', color: '#ff9ad6', direction: { compass: 'S', altitude: 'mid' } })]);
  assert.ok(mk('ring').fxItems.some((f) => f.shape === 'ring'));
  assert.ok(mk('doorway').fxItems.some((f) => f.shape === 'doorway'));
  assert.equal(mk('comet').comets.length, 1);
  assert.equal(mk('curtain').aur.length, 1);
  assert.equal(mk('veil').airs.length, 1);
});

test('a one-off event whose days run over today counts as today\'s; a repeat is left to the calendar page', () => {
  const env = load();
  const cal = at(21, 15);
  const list = [
    ev({ type: 'comet' }, { id: 1, day: 8, end_year: 1491, end_month: 3, end_day: 20 }),
    ev({ type: 'comet' }, { id: 2, day: 1, end_year: 1491, end_month: 3, end_day: 5 }),
    ev({ type: 'aurora' }, { id: 3, day: 15 }),
  ];
  assert.deepEqual(env.SkyPane.Scene.eventsToday(cal, list).map((e) => e.id), [1, 3]);
});
