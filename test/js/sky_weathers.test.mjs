// sky_weathers.test.mjs — the six weathers drawn with a strong sky shape of
// their own (hanging pouches, lens clouds, St Elmo's fire, ball lightning, an
// eruption, sky leviathans): each resolves from its built-in preset into what
// the painter draws, finite everywhere; the lenses and the volcano stand on
// the far range, the fire on the near wood's crowns, the balls low over the
// land; a change of weather grows them in without a jump; particle sets of
// different weathers never share a seed; and reduced motion holds them still.

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
    sessionStorage: { getItem: () => null, setItem() {} },
    addEventListener() {}, removeEventListener() {}, setTimeout, clearTimeout, performance: { now: () => 0 },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of ['sky_world.js', 'sky_looks.js', 'sky_moon.js', 'sky_events.js', 'sky_2d.js', 'gl_shared.js', 'sky_gl.js', 'sky_pane.js']) {
    vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox, { filename: f });
  }
  return sandbox;
}

const W = 1000, H = 250;
const CAL = {
  name: 'Harptos', current_year: 1491, current_month: 6, current_day: 12, current_hour: 21, current_minute: 0,
  months: Array.from({ length: 12 }, (_, i) => ({ name: 'M' + i, days: 30 })), weekdays: [{ name: 'One' }],
  hours_per_day: 24, minutes_per_hour: 60,
  moons: [{ id: 1, name: 'Selûne', cycle_days: 30, phase_offset: 0, color: '#d9e1ec', size: 1 }],
};
const cal = (preset, hour) => Object.assign({}, CAL, { weather: { preset_id: preset }, current_hour: Math.floor(hour), current_minute: Math.round((hour % 1) * 60) });
function model(env, c, extra = {}) {
  return Object.assign({ calendar: c, todayEvents: [], dayWeather: null, skym: env.SkyWorld.makeSkym(c), landSeed: 1, width: W, height: H, reduced: false }, extra);
}
const state = (env, preset, hour, t = 5, extra) => env.SkyPane.Scene.state(model(env, cal(preset, hour), extra), t, -1);

// Every number anywhere in a value is finite.
function finite(v, what, seen = new Set()) {
  if (typeof v === 'number') { assert.ok(Number.isFinite(v), what + ' is ' + v); return; }
  if (!v || typeof v !== 'object' || seen.has(v)) return;
  seen.add(v);
  for (const [k, x] of Object.entries(v)) finite(x, what + '.' + k, seen);
}

const NEW = {
  'mammatus-sky': { effect: 'mammatus', hour: 19.4 },
  lenticular: { effect: 'lenticular', hour: 18.9 },
  'st-elmos-fire': { effect: 'st-elmo', hour: 21.6 },
  'ball-lightning': { effect: 'ball-lightning', hour: 21.2 },
  eruption: { effect: 'volcano', hour: 21 },
  leviathans: { effect: 'leviathans', hour: 15 },
};
// What each weather hands the painter, from its frame state.
function drawn(st, preset) {
  const lk = st.look;
  if (preset === 'mammatus-sky') return lk.air.filter((a) => a.t === 'pouch');
  if (preset === 'lenticular') return st.fxItems.filter((f) => f.shape === 'lens');
  if (preset === 'st-elmos-fire') return lk.parts.filter((p) => p.mode === 'ridge');
  if (preset === 'ball-lightning') return lk.parts.filter((p) => p.p === 'ball');
  if (preset === 'eruption') return st.fxItems.filter((f) => f.shape === 'eruption');
  return st.fxItems.filter((f) => f.shape === 'leviathan');
}
const strength = (list) => list.reduce((s, x) => s + (x.k != null ? x.k : x.a), 0);

test('each new preset resolves to its effect and hands the painter only finite numbers, its uniforms included', () => {
  const env = load(), L = env.SkyLooks;
  assert.equal(L.AIR.pouch, 8);
  for (const [id, want] of Object.entries(NEW)) {
    assert.equal(L.PRESET_EFFECT[id], want.effect, id);
    assert.ok(L.VOCAB.includes(want.effect), want.effect + ' is in the look editor vocabulary');
    for (const hour of [want.hour, 12, 2]) {
      const st = state(env, id, hour);
      assert.ok(st.look.effects.includes(want.effect), id);
      assert.ok(drawn(st, id).length > 0, id + ' at ' + hour + ' draws its own shape');
      finite(st.fxItems, id + ' fxItems'); finite(st.look.air, id + ' air'); finite(st.look.parts, id + ' parts');
      const U = env.SkyGL._uniforms(st, { moonSlot: () => [0, 0, 1, 1] }, 1);
      finite(U, id + ' uniforms');
      if (id === 'mammatus-sky') assert.ok(U.uAirA.some((v, i) => i % 4 === 1 && v === 8), 'the pouch air reaches the shader as type 8');
    }
  }
});

test('lens clouds stand over true peaks of the far range, kept whole in a narrow pane, and the volcano on the far ridge', () => {
  const env = load(), FX = env.SkyFX;
  const st = state(env, 'lenticular', 18.9);
  const lenses = st.fxItems.filter((f) => f.shape === 'lens');
  assert.equal(lenses.length, 2);
  // A true peak: the top of the ridge for a twentieth of the width either side.
  const peakAt = (x) => { const y = FX.ridgeAt(0, x, W, H); for (let d = -W * .05; d <= W * .05; d += 1) if (FX.ridgeAt(0, x + d, W, H) < y - .5) return false; return true; };
  for (const l of lenses) assert.ok(peakAt(l.x), 'a lens is over a true peak at ' + l.x);
  assert.ok(FX.ridgeAt(0, lenses[0].x, W, H) <= FX.ridgeAt(0, lenses[1].x, W, H), 'the larger lens is over the higher peak');
  assert.ok(Math.abs(lenses[0].x - lenses[1].x) > W * .2, 'the peaks are apart');
  for (const l of lenses) {
    const peak = FX.ridgeAt(0, l.x, W, H);
    assert.ok(l.y < peak - H * .07 && l.y > peak - H * .15, 'a lens floats just over its peak');
    assert.equal(l.k, 1, 'at rest a lens is solid; cloud and fog haze it instead');
  }
  const narrow = env.SkyPane.Scene.state(model(env, cal('lenticular', 18.9), { width: 360, height: 160 }), 5, -1);
  for (const l of narrow.fxItems.filter((f) => f.shape === 'lens')) {
    const half = 160 * .31 * l.sz;
    assert.ok(l.x - half >= 0 && l.x + half <= 360, 'a lens stays whole in a phone-width pane');
  }
  const er = state(env, 'eruption', 21).fxItems.find((f) => f.shape === 'eruption');
  assert.ok(Math.abs(er.y - (FX.ridgeAt(0, er.x, W, H) + H * .02)) < 1e-6, 'the cone stands on the far ridge');
});

test("St Elmo's fire burns only on tree crowns of the near wood that stand clear of the foreground", () => {
  const env = load(), FX = env.SkyFX;
  const st = state(env, 'st-elmos-fire', 21.6), p = st.look.parts.find((q) => q.mode === 'ridge');
  const crowns = FX.crowns(W, H);
  assert.ok(crowns.length > 10, 'the wood has crowns to burn on');
  let lit = 0;
  for (let t = 0; t < 30; t += 1.7) for (let i = 0; i < p.n; i++) {
    const o = FX.particle(p, i, t, W, H, 1.6, .5, 3.1);
    if (o.a <= 0) continue;
    lit++;
    assert.ok(crowns.some((c) => c[0] === o.x && c[1] === o.y), 'on a crown');
    assert.equal(o.y, FX.ridgeAt(2, o.x, W, H));
    assert.ok(FX.ridgeAt(2, o.x, W, H, true) - o.y > 1, 'a tree stands there, not bare ground');
    assert.ok(FX.ridgeAt(3, o.x, W, H) > o.y, 'the foreground does not cover it');
    assert.deepEqual(FX.particle(p, i, t, W, H, 1.6, .5, 3.1), o, 'the same moment draws the same fire');
  }
  assert.ok(lit > 50);
});

test('ball lightning wanders low, in front of the middle range, and never floats far', () => {
  const env = load(), FX = env.SkyFX;
  const p = state(env, 'ball-lightning', 21.2).look.parts.find((q) => q.p === 'ball');
  for (const [w, h] of [[W, H], [360, 160]]) {
    let lo = Infinity, hi = -Infinity;
    for (let t = 0; t < 60; t += .5) for (let i = 0; i < p.n; i++) {
      const o = FX.particle(p, i, t, w, h, Math.max(.65, Math.min(1.6, h / 150)), .4, 9);
      if (o.x < 0 || o.x > w) continue;
      assert.ok(o.y >= FX.ridgeAt(1, o.x, w, h), 'below the middle range\'s line at ' + o.x);
      lo = Math.min(lo, o.y / h); hi = Math.max(hi, o.y / h);
    }
    assert.ok(lo > .6, 'never high in the sky (' + lo + ')');
  }
});

test("St Elmo's fire keeps its prototype size in a full pane and takes its floor only in a short one", () => {
  const env = load();
  const p = state(env, 'st-elmos-fire', 21.6).look.parts.find((q) => q.mode === 'ridge');
  assert.deepEqual(Array.from(p.s), [3.4, 5.6]);
  assert.ok(p.rmin > 0);
});

test('particle sets of different weathers never share a seed, even when their names are the same length', () => {
  const env = load(), L = env.SkyLooks, FX = env.SkyFX;
  for (const [a, b] of [['leaves', 'petals'], ['embers', 'sparks'], ['wisps', 'motes'], ['volcano', 'st-elmo']]) {
    if (!L.EFFECTS[a] || !L.EFFECTS[b]) continue;
    const R = L.resolve({ id: 'x', like: 'clear', look: { effect: a, accents: [b] } });
    const seeds = R.parts.map((p, i) => FX.seedOf(p, i));
    assert.equal(new Set(seeds).size, seeds.length, a + ' and ' + b + ' draw different particles');
  }
});

test('a change from clear to each new weather grows it in smoothly, with nothing jumping', () => {
  const env = load();
  for (const [id, want] of Object.entries(NEW)) {
    const m = model(env, cal('clear', want.hour));
    let st = env.SkyPane.Scene.state(m, 0, -1), t = 0;
    assert.equal(drawn(st, id).length, 0, id + ' is not there under a clear sky');
    m.calendar = cal(id, want.hour);
    let prev = 0, place = null, steps = 0;
    while (t < 8) {
      t += .1;
      st = env.SkyPane.Scene.state(m, t, .1);
      const k = strength(drawn(st, id));
      // The cover it fades with eases on its own, so a lens may settle by a hair as the clouds finish moving.
      assert.ok(k >= prev - .005, id + ' never fades back while it comes in');
      assert.ok(k - prev < .06 * Math.max(1, drawn(st, id).length), id + ' grows a little each frame, not at once (' + (k - prev) + ')');
      const where = drawn(st, id).filter((x) => x.x != null).map((x) => [x.x, x.y]);
      if (where.length && place) assert.deepEqual(where, place, id + ' keeps its place through the change');
      if (where.length) place = where;
      if (k > prev) steps++;
      prev = k;
    }
    assert.ok(steps > 20, id + ' comes in over many frames');
    const end = strength(drawn(state(env, id, want.hour, t), id));
    assert.ok(Math.abs(prev - end) < .01, id + ' ends at its own full strength');
  }
});

test('a particle set keeps its seed while another weather lays its sets before it', () => {
  const env = load(), L = env.SkyLooks;
  const ash = L.resolve(L.spec({ preset_id: 'eruption' })), elmo = L.resolve(L.spec({ preset_id: 'st-elmos-fire' }));
  const mid = L.blend(ash, elmo, .5);
  assert.equal(mid.parts[1].pi, elmo.parts[0].pi, 'the fire set is seeded by its place in its own effect');
});

test('the sun takes a smoky tint by degrees through a change of weather', () => {
  const env = load(), L = env.SkyLooks;
  const clear = L.resolve(L.spec({ preset_id: 'clear' })), er = L.resolve(L.spec({ preset_id: 'eruption' }));
  assert.equal(L.blend(clear, er, .3).sun.tk, .3);
  assert.equal(L.blend(clear, er, .3).sun.tint, er.sun.tint);
});

test('reduced motion holds every new weather still; otherwise the moving ones move and the lenses stand', () => {
  const env = load();
  for (const [id, want] of Object.entries(NEW)) {
    assert.equal(state(env, id, want.hour, 5, { reduced: true }).pace, 0, id);
    assert.equal(state(env, id, want.hour).pace, id === 'lenticular' ? 1 : 2, id);
  }
});
