// sky_dock.test.mjs — the sky docked over the calendar's month (#830):
// SkyPane.Dock in static/js/widgets/sky_pane.js.
//
// Pins the signed fold: the pane's room changes at once, the card's top edge
// stays put on every frame while it travels, the rest state lands with no
// transform or clip left behind, reduced motion never moves the card, and
// nothing asks for frames once the sky is at rest. The sky scripts run in a
// vm context with small fakes for the DOM and canvas they touch.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const widgets = join(root, 'static', 'js', 'widgets');

// A 2D context that accepts every call Sky2D and MOONR make.
function fakeContext() {
  const gradient = { addColorStop() {} };
  return new Proxy({}, {
    get(t, k) {
      if (k in t) return t[k];
      if (k === 'createLinearGradient' || k === 'createRadialGradient') return () => gradient;
      if (k === 'getImageData') return (x, y, w, h) => ({ data: new Uint8ClampedArray(w * h * 4) });
      return () => {};
    },
    set(t, k, v) { t[k] = v; return true; },
  });
}

function fakeEl(extra = {}) {
  const attrs = {}, classes = new Set(), props = {}, listeners = {};
  const el = {
    style: new Proxy(props, {
      get(t, k) {
        if (k === 'setProperty') return (n, v) => { t[n] = v; };
        if (k === 'getPropertyValue') return (n) => t[n] || '';
        return t[k] === undefined ? '' : t[k];
      },
      set(t, k, v) { t[k] = v; return true; },
    }),
    classList: {
      add: (c) => classes.add(c), remove: (c) => classes.delete(c), contains: (c) => classes.has(c),
      toggle: (c, on) => { if (on === undefined ? !classes.has(c) : on) classes.add(c); else classes.delete(c); },
    },
    setAttribute: (k, v) => { attrs[k] = String(v); },
    getAttribute: (k) => (k in attrs ? attrs[k] : null),
    addEventListener: (type, fn) => { (listeners[type] = listeners[type] || []).push(fn); },
    removeEventListener: (type, fn) => { listeners[type] = (listeners[type] || []).filter((f) => f !== fn); },
    fire: (type) => (listeners[type] || []).forEach((fn) => fn({ type })),
    get offsetWidth() { return this.clientWidth; },
    clientWidth: 0, offsetHeight: 0,
    ...extra,
  };
  return el;
}

function loadSky({ reduced = false, stored = null, apiFetch = null, rest = null } = {}) {
  const frames = [], store = new Map(stored ? [['chronicle.calendar.sky', stored]] : []);
  let now = 0;
  const document = {
    hidden: false,
    createElement: () => ({ width: 0, height: 0, getContext: () => fakeContext() }),
    addEventListener() {}, removeEventListener() {},
  };
  const sandbox = {
    console, Math, JSON, Object, Array, Number, String, Uint8Array, Uint8ClampedArray, Float32Array, Error,
    document,
    ImageData: function (w, h) { this.width = w; this.height = h; this.data = new Uint8ClampedArray(w * h * 4); },
    getComputedStyle: () => ({ borderTopLeftRadius: '24px' }),
    matchMedia: (q) => ({ matches: reduced && /reduce/.test(q) }),
    localStorage: {
      getItem: (k) => (store.has(k) ? store.get(k) : null),
      setItem: (k, v) => store.set(k, String(v)),
      removeItem: (k) => store.delete(k),
    },
    requestAnimationFrame: (fn) => { frames.push(fn); return frames.length; },
    cancelAnimationFrame: (id) => { if (frames[id - 1]) frames[id - 1] = null; },
    performance: { now: () => now },
    devicePixelRatio: 1,
    setTimeout, clearTimeout, setInterval() { return 0; }, clearInterval() {},
    Chronicle: { register() {}, apiFetch: apiFetch || (() => Promise.reject(new Error('no server'))) },
    addEventListener() {}, removeEventListener() {},
  };
  if (rest) sandbox.MotionRest = rest;
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of ['sky_world.js', 'sky_looks.js', 'sky_moon.js', 'sky_events.js', 'sky_2d.js', 'sky_pane.js']) {
    vm.runInContext(readFileSync(join(widgets, f), 'utf8'), sandbox, { filename: f });
  }
  // Runs every frame asked for until none is (or a cap), 16ms apart.
  const drain = (cap = 400) => {
    let n = 0;
    while (n < cap) {
      const fn = frames.shift();
      if (fn === undefined) break;
      if (!fn) continue;
      now += 16; n++;
      fn(now);
    }
    return n;
  };
  const step = () => { const fn = frames.shift(); if (fn) { now += 16; fn(now); return true; } return false; };
  return { SkyPane: sandbox.SkyPane, document, store, drain, step, pending: () => frames.filter(Boolean).length, setNow: (ms) => { now = ms; } };
}

const CAL = {
  name: 'Harptos', current_year: 1491, current_month: 1, current_day: 14, current_hour: 21, current_minute: 40,
  months: [{ name: 'Hammer', days: 30 }], weekdays: [{ name: 'One' }],
  moons: [{ id: 1, name: 'Selûne', cycle_days: 30.4375, phase_offset: 0, color: '#d9e1ec', size: 1 }],
};

function dock(env, opts = {}) {
  const canvas = { width: 0, height: 0, getContext: () => fakeContext() };
  const sky = fakeEl({ querySelector: () => canvas });
  const sw = fakeEl();
  const wrap = fakeEl({ clientWidth: 950, querySelector: () => sky });
  const chip = fakeEl({ querySelector: () => sw });
  const card = fakeEl({ clientWidth: 952, offsetHeight: 900 });
  const follower = fakeEl();
  const said = [];
  const d = new env.SkyPane.Dock({
    card, wrap, chip, seed: 'cal-1', name: CAL.name, campaignId: opts.campaignId, calendarId: opts.calendarId,
    followers: () => [follower],
    travel: opts.travel, before: opts.before, say: (m) => said.push(m),
  });
  d.setDay(opts.cal || CAL, []);
  return { d, card, wrap, chip, sky, follower, said, canvas };
}

const ty = (el) => { const m = /translateY\((-?[\d.]+)px\)/.exec(el.style.transform); return m ? +m[1] : 0; };
const clipTopOf = (el) => { const m = /A[\d.]+ [\d.]+ 0 0 1 [\d.]+ (-?[\d.]+) H/.exec(el.style.clipPath); return m ? +m[1] : null; };

test('the pane is 14% of the card, at least 100px and at most 166px', () => {
  const { SkyPane } = loadSky();
  assert.equal(SkyPane.paneHeight(390), 100);
  assert.equal(SkyPane.paneHeight(950), 133);
  assert.equal(SkyPane.paneHeight(1600), 166);
});

test('opening springs with a little give; closing never passes its mark', () => {
  const { SkyPane } = loadSky();
  const open = { e: 0, v: 0, target: 1, on: true };
  let peak = 0, t = 0;
  while (open.on && t < 3) { SkyPane.spring(open, 1 / 60, true); peak = Math.max(peak, open.e); t += 1 / 60; }
  assert.equal(open.e, 1);
  assert.ok(peak > 1 && peak < 1.05, 'a small overshoot, got ' + peak);
  assert.ok(t < 1, 'opening settles within a second');
  const close = { e: 1, v: 0, target: 0, on: true };
  let low = 1, tc = 0;
  while (close.on && tc < 3) { SkyPane.spring(close, 1 / 60, false); low = Math.min(low, close.e); tc += 1 / 60; }
  assert.equal(close.e, 0);
  assert.ok(low >= 0, 'closing never passes 0, got ' + low);
  assert.ok(tc < t, 'closing settles sooner than opening');
});

test('open by default: the room is there, the chip says so, nothing animates at rest', () => {
  const env = loadSky();
  const { wrap, chip, canvas } = dock(env);
  assert.ok(wrap.classList.contains('open'));
  assert.equal(chip.getAttribute('aria-expanded'), 'true');
  assert.equal(wrap.style.getPropertyValue('--skyH'), '133px');
  assert.equal(canvas.width, 950);
  assert.equal(canvas.height, 133);
  assert.equal(env.pending(), 0, 'no frames asked for at rest');
});

test('folding: room goes at once, the card top stays put every frame, the rest state is clean', () => {
  const env = loadSky();
  const { d, wrap, card, chip, follower, said } = dock(env);
  d.fold(false);
  assert.ok(!wrap.classList.contains('open'), 'the room goes at the first frame');
  assert.equal(chip.getAttribute('aria-expanded'), 'false');
  assert.equal(env.store.get('chronicle.calendar.sky'), 'folded');
  let frames = 0;
  while (card.classList.contains('moving') && env.step()) {
    frames++;
    if (!card.classList.contains('moving')) break;
    const top = clipTopOf(card);
    assert.ok(top !== null, 'the card is clipped while it travels');
    assert.ok(Math.abs(ty(card) + top) < 0.02, 'the visible top edge stays put: ' + ty(card) + ' + ' + top);
    assert.equal(ty(follower), ty(card), 'what follows the card rides with it');
  }
  assert.ok(frames > 3, 'it travelled over several frames');
  env.drain();
  assert.equal(card.style.transform, '');
  assert.equal(card.style.clipPath, '');
  assert.equal(follower.style.transform, '');
  assert.ok(!card.classList.contains('moving'));
  assert.ok(chip.classList.contains('caught'), 'the chip catches the shade');
  assert.equal(env.pending(), 0);
  assert.deepEqual(said, ['Sky folded into its chip']);
});

test('opening again: room comes back at once, the card top stays put, the choice is forgotten', () => {
  const env = loadSky({ stored: 'folded' });
  const { d, wrap, card, chip } = dock(env);
  assert.ok(!wrap.classList.contains('open'), 'a remembered fold is where it starts');
  assert.equal(chip.getAttribute('aria-expanded'), 'false');
  d.fold(true);
  assert.ok(wrap.classList.contains('open'));
  assert.equal(env.store.has('chronicle.calendar.sky'), false);
  let checked = 0;
  while (env.step()) {
    if (!card.classList.contains('moving')) continue;
    assert.ok(Math.abs(ty(card) + clipTopOf(card)) < 0.02);
    checked++;
  }
  assert.ok(checked > 3);
  assert.equal(card.style.transform, '');
  assert.equal(env.pending(), 0);
});

test('reduced motion never moves the card; opening only fades the sky in', () => {
  const env = loadSky({ reduced: true });
  const { d, card, sky } = dock(env);
  d.fold(false);
  assert.ok(!card.classList.contains('moving'));
  assert.equal(card.style.transform, '');
  assert.equal(env.pending(), 0);
  d.fold(true);
  assert.equal(sky.style.opacity, '0');
  env.step();
  assert.ok(+sky.style.opacity > 0 && +sky.style.opacity < 1, 'fading, got ' + sky.style.opacity);
  env.drain();
  assert.equal(sky.style.opacity, '');
  assert.ok(!card.classList.contains('moving'));
});

test('when travel is not allowed, or the tab is hidden, the fold lands at once', () => {
  const env = loadSky();
  const a = dock(env, { travel: () => false });
  a.d.fold(false);
  assert.ok(!a.card.classList.contains('moving'));
  assert.equal(env.pending(), 0);
  const env2 = loadSky();
  const b = dock(env2);
  env2.document.hidden = true;
  b.d.fold(false);
  assert.ok(!b.card.classList.contains('moving'));
  assert.equal(b.card.style.transform, '');
});

test('pressing the chip puts the calendar\'s cards away first', () => {
  const env = loadSky();
  let before = 0;
  const { chip } = dock(env, { before: () => { before++; } });
  chip.fire('click');
  assert.equal(before, 1);
  assert.equal(chip.getAttribute('aria-expanded'), 'false');
});

test('the chip and the sky name the sky now', () => {
  const env = loadSky();
  const { sky, chip } = dock(env);
  assert.match(sky.getAttribute('aria-label'), /^The sky over Harptos now\. .+ · 9:40 PM$/);
  assert.match(chip.querySelector('.sw').style.getPropertyValue('--sw'), /linear-gradient\(#[0-9a-f]{6},#[0-9a-f]{6}\)$/);
});

test('the calendar docks the sky at the top of its card, and the chip leads the header\'s actions', () => {
  const view = readFileSync(join(widgets, 'calendar_view.js'), 'utf8');
  const shell = view.slice(view.indexOf('_buildShell: function'), view.indexOf('say: function'));
  assert.ok(shell.indexOf('id="cal5-skywrap"') > -1 && shell.indexOf('id="cal5-skywrap"') < shell.indexOf('<header class="head">'),
    'the pane comes before the header, flush at the top of the card');
  const acts = shell.slice(shell.indexOf('<div class="h-acts">'));
  assert.ok(acts.indexOf('id="cal5-skybtn"') > -1 && acts.indexOf('id="cal5-skybtn"') < acts.indexOf('id="cal5-moonbtn"'));
  const editor = readFileSync(join(widgets, 'calendar_editor.js'), 'utf8');
  assert.match(editor, /acts\.insertBefore\(btn, sky \? sky\.nextSibling : acts\.firstChild\)/, 'Edit goes after the Sky chip');
});

// The day's own weather reading wins over the calendar's current weather.
const settle = () => new Promise((r) => setTimeout(r, 0));
const reply = (body, ok = true) => Promise.resolve({ ok, status: ok ? 200 : 503, json: () => Promise.resolve(body) });
const WITH_CURRENT = { ...CAL, weather: { preset_id: 'clear', preset_label: 'Clear skies' } };

test("the shown day's weather reading replaces the calendar's current weather", async () => {
  const urls = [];
  const env = loadSky({ apiFetch: (u) => { urls.push(u); return reply([
    { year: 1491, month: 1, day: 13, preset_id: 'snow', preset_label: 'Snow', source: 'manual' },
    { year: 1491, month: 1, day: 14, preset_id: 'thunderstorm', preset_label: 'Thunderstorm', source: 'manual' },
  ]); } });
  const { sky } = dock(env, { campaignId: 'c-1', calendarId: 'cal-1', cal: WITH_CURRENT });
  assert.match(sky.getAttribute('aria-label'), /Clear skies/);
  await settle();
  assert.deepEqual(urls, ['/campaigns/c-1/calendars/cal-1/weather/days?year=1491&month=1']);
  assert.match(sky.getAttribute('aria-label'), /Thunderstorm/);
});

test('with no reading for the day, or no answer, the current weather stays', async () => {
  for (const apiFetch of [() => reply([{ year: 1491, month: 1, day: 13, preset_id: 'snow', preset_label: 'Snow' }]), () => reply({ error: 'calendar_rebuilding' }, false)]) {
    const env = loadSky({ apiFetch });
    const { sky } = dock(env, { campaignId: 'c-1', calendarId: 'cal-1', cal: WITH_CURRENT });
    await settle();
    assert.match(sky.getAttribute('aria-label'), /Clear skies/);
  }
});

test('without campaign and calendar ids the dock asks for nothing', async () => {
  let asked = 0;
  const env = loadSky({ apiFetch: () => { asked++; return reply([]); } });
  dock(env, { cal: WITH_CURRENT });
  await settle();
  assert.equal(asked, 0);
});

// A stand-in for motion_rest.js: the test says when the viewer has stepped away.
function fakeRest() {
  const wakers = [];
  const r = {
    away: false,
    now: () => 10, speed: () => (r.away ? 0 : 1), still: () => r.away,
    onWake: (fn) => wakers.push(fn), offWake: (fn) => { const i = wakers.indexOf(fn); if (i >= 0) wakers.splice(i, 1); },
    wake: () => { r.away = false; wakers.slice().forEach((fn) => fn()); },
  };
  return r;
}

test('a moving sky asks for no frames while the viewer is away, and starts again when they are back', () => {
  const rest = fakeRest();
  const env = loadSky({ rest });
  const { d } = dock(env);
  // This harness has no WebGL, so a painted frame would mark the sky as still; the test holds a moving pace instead.
  d.pace = 2; d.kick();
  assert.ok(env.pending() > 0, 'a moving sky asks for frames');
  rest.away = true;
  d.kick();
  assert.equal(env.pending(), 0, 'at rest the last frame stays up and nothing more is asked for');
  rest.wake();
  assert.ok(env.pending() > 0, 'coming back asks for frames again');
  d.destroy();
  rest.away = true; rest.wake();
  assert.equal(env.pending(), 0, 'a destroyed dock does not wake');
});

// Paints the dock's sky at 60 frames a second for the given seconds, from t0, with the page clock kept in step.
function run(env, d, t0, secs) {
  let t = t0;
  for (let i = 0; i < secs * 60; i++) { t += 1 / 60; env.setNow(t * 1000); d.paint(t, 0, d.H); }
  return t;
}

test('a change of weather rolls in: each dial at its own pace, the layers over a few seconds', () => {
  const env = loadSky();
  const { d } = dock(env, { cal: WITH_CURRENT });
  d.measure();
  let t = run(env, d, 100, 1);
  const g = d.model.glide;
  assert.equal(g.wx.cloud, 0, 'a clear sky starts with no cloud');
  d.setDay({ ...WITH_CURRENT, current_day: 15, weather: { preset_id: 'sandstorm', preset_label: 'Sandstorm' } }, []);
  const G = d.model.glide;
  assert.equal(G, g, 'the new day keeps the sky it is rolling from');
  t = run(env, d, t, 1);
  assert.ok(G.from, 'one second in, the dust is still fading in');
  assert.ok(G.k > .1 && G.k < .3, 'the layers are a sixth of the way: ' + G.k);
  const cloud1 = G.wx.cloud, fog1 = G.wx.fog, wind1 = G.wx.wind;
  assert.ok(cloud1 > 0, 'cloud has started to gather');
  assert.equal(d.pace, 2, 'a change rolling in is painted at the full rate, even by the basic sky');
  t = run(env, d, t, 15);
  assert.equal(G.from, null, 'the fade is done');
  // Each dial has gone most of the way in about three of its paces, the slower ones less far after one second.
  assert.ok(cloud1 < G.wx.cloud * .5, 'cloud is less than half way after the first second, and keeps rolling in');
  assert.ok(wind1 / G.wx.wind > fog1 / G.wx.fog, 'wind (a fast dial) got further in the first second than fog (a slow one)');
});

test('with reduced motion a change of weather shows at once', () => {
  const env = loadSky({ reduced: true });
  const { d } = dock(env, { cal: WITH_CURRENT });
  d.measure();
  let t = run(env, d, 100, .5);
  d.setDay({ ...WITH_CURRENT, current_day: 15, weather: { preset_id: 'sandstorm', preset_label: 'Sandstorm' } }, []);
  t = run(env, d, t, .1);
  const G = d.model.glide;
  assert.equal(G.from, null, 'no fade');
  const before = JSON.stringify(G.wx);
  run(env, d, t, 2);
  assert.equal(JSON.stringify(G.wx), before, 'the dials are already at the new weather');
});
