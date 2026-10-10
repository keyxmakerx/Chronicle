// header_sky.test.mjs — the header's Sky background (#959):
// static/js/widgets/header_sky.js, drawing with SkyPane.Scene from sky_pane.js.
//
// Pins what the signed design promises: it reads the campaign's own calendar
// and that day's events and weather, fills the header, shows only once drawn,
// holds still under reduced motion, stops asking for frames at rest and
// starts again on wake, and leaves the still night showing when it cannot
// draw. The sky scripts run in a vm context with small fakes for the DOM.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const widgets = join(here, '..', '..', 'static', 'js', 'widgets');

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

const CAL = {
  name: 'Harptos', current_year: 1491, current_month: 1, current_day: 14, current_hour: 21, current_minute: 40,
  months: [{ name: 'Hammer', days: 30 }], weekdays: [{ name: 'One' }],
  moons: [{ id: 1, name: 'Selûne', cycle_days: 30.4375, phase_offset: 0, color: '#d9e1ec', size: 1 }],
};

// rest: a fake MotionRest. painted: true gives the page a painted sky (SkyGL)
// whose pace follows the scene, as on a capable device. skyScripts: false
// leaves the sky scripts out, as when they failed to load.
function load({ reduced = false, painted = true, fail = false, skyScripts = true } = {}) {
  const frames = [], fetched = [], wakers = [];
  let still = false;
  const rest = {
    now: () => 1, speed: () => (still ? 0 : 1), still: () => still,
    onWake: (fn) => wakers.push(fn), offWake: (fn) => { const i = wakers.indexOf(fn); if (i >= 0) wakers.splice(i, 1); },
  };
  const json = (body) => Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
  const apiFetch = (url) => {
    fetched.push(url);
    if (fail) return Promise.resolve({ ok: false, status: 500 });
    if (/weather\/days/.test(url)) return json([]);
    if (/events/.test(url)) return json([{ year: 1491, month: 1, day: 14, title: 'Feast' }, { year: 1491, month: 1, day: 2, title: 'Earlier' }]);
    return json(CAL);
  };
  const sandbox = {
    console: { ...console, warn() {}, error() {} }, Math, JSON, Object, Array, Number, String, Uint8Array, Uint8ClampedArray, Float32Array, Error, Promise, Date,
    document: { hidden: false, createElement: () => ({ width: 0, height: 0, getContext: () => fakeContext() }), addEventListener() {}, removeEventListener() {} },
    ImageData: function (w, h) { this.width = w; this.height = h; this.data = new Uint8ClampedArray(w * h * 4); },
    getComputedStyle: () => ({ borderTopLeftRadius: '0px' }),
    matchMedia: (q) => ({ matches: reduced && /reduce/.test(q) }),
    localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
    requestAnimationFrame: (fn) => { frames.push(fn); return frames.length; },
    cancelAnimationFrame: (id) => { if (frames[id - 1]) frames[id - 1] = null; },
    performance: { now: () => 0 },
    devicePixelRatio: 2,
    setTimeout: (fn) => { frames.push(fn); return frames.length; },
    clearTimeout: (id) => { if (frames[id - 1]) frames[id - 1] = null; },
    setInterval() { return 0; }, clearInterval() {},
    Chronicle: { register(name, w) { sandbox.registered = { name, w }; }, apiFetch },
    MotionRest: rest,
    addEventListener() {}, removeEventListener() {},
  };
  if (painted) sandbox.SkyGL = { available: () => true, paint: () => false, onChange() {}, offChange() {} };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  const files = skyScripts ? ['sky_world.js', 'sky_looks.js', 'sky_moon.js', 'sky_events.js', 'sky_2d.js', 'sky_pane.js'] : [];
  for (const f of [...files, 'header_sky.js']) vm.runInContext(readFileSync(join(widgets, f), 'utf8'), sandbox, { filename: f });
  return {
    sandbox, fetched, wakers,
    pending: () => frames.filter(Boolean).length,
    step: () => { const i = frames.findIndex(Boolean); if (i < 0) return false; const fn = frames[i]; frames[i] = null; fn(16); return true; },
    rest: (v) => { still = v; },
  };
}

function header() {
  const classes = new Set();
  const canvas = { width: 0, height: 0, getContext: () => fakeContext() };
  return {
    canvas, classes,
    clientWidth: 900, clientHeight: 56,
    querySelector: () => canvas,
    getAttribute: (k) => ({ 'data-campaign-id': 'camp 1', 'data-calendar-id': 'cal/1' })[k] ?? null,
    classList: { add: (c) => classes.add(c), remove: (c) => classes.delete(c) },
  };
}

const settle = () => new Promise((r) => setTimeout(r, 0));

test('it reads the campaign calendar, that day only, and fills the header', async () => {
  const env = load();
  assert.equal(env.sandbox.registered.name, 'header-sky');
  const el = header();
  const sky = env.sandbox.Chronicle.headerSky.mount(el, { campaignId: 'camp 1', calendarId: 'cal/1' });
  assert.ok(sky);
  await settle(); await settle();
  assert.deepEqual(env.fetched, [
    '/campaigns/camp%201/calendars/cal%2F1',
    '/campaigns/camp%201/calendars/cal%2F1/events?year=1491&month=1',
    '/campaigns/camp%201/calendars/cal%2F1/weather/days?year=1491&month=1',
  ]);
  assert.deepEqual(sky.model.todayEvents.map((e) => e.title), ['Feast']);
  assert.equal(el.canvas.width, 1800, 'drawn at the device pixel ratio');
  assert.equal(el.canvas.height, 112);
  assert.ok(el.classes.has('is-ready'), 'shown once drawn');
  assert.ok(env.pending() > 0, 'a living sky keeps drawing');
});

test('reduced motion: painted once and held still', async () => {
  const env = load({ reduced: true });
  const el = header();
  env.sandbox.Chronicle.headerSky.mount(el, { campaignId: 'c', calendarId: 'k' });
  await settle(); await settle();
  assert.ok(el.classes.has('is-ready'));
  assert.equal(env.pending(), 0, 'no frames asked for');
});

test('the Customize example can hold it still too', async () => {
  const env = load();
  const el = header();
  env.sandbox.Chronicle.headerSky.mount(el, { campaignId: 'c', calendarId: 'k', still: true });
  await settle(); await settle();
  assert.ok(el.classes.has('is-ready'));
  assert.equal(env.pending(), 0);
});

test('at rest it stops asking for frames, and starts again on wake', async () => {
  const env = load();
  env.sandbox.Chronicle.headerSky.mount(header(), { campaignId: 'c', calendarId: 'k' });
  await settle(); await settle();
  env.rest(true);
  let n = 0;
  while (env.step() && n < 50) n++;
  assert.ok(n < 50);
  assert.equal(env.pending(), 0, 'still: nothing pending');
  env.rest(false);
  env.wakers.forEach((fn) => fn());
  assert.ok(env.pending() > 0, 'awake again');
});

test('the basic sky with nothing moving draws once and rests', async () => {
  const env = load({ painted: false });
  const el = header();
  env.sandbox.Chronicle.headerSky.mount(el, { campaignId: 'c', calendarId: 'k' });
  await settle(); await settle();
  assert.ok(el.classes.has('is-ready'));
  assert.equal(env.pending(), 0);
});

test('a failed read leaves the still night showing', async () => {
  const env = load({ fail: true });
  const el = header();
  env.sandbox.Chronicle.headerSky.mount(el, { campaignId: 'c', calendarId: 'k' });
  await settle(); await settle();
  assert.ok(!el.classes.has('is-ready'));
  assert.equal(el.canvas.width, 0, 'nothing drawn');
});

test('without the sky scripts it mounts nothing and throws nothing', () => {
  const env = load({ skyScripts: false });
  assert.equal(env.sandbox.Chronicle.headerSky.mount(header(), { campaignId: 'c', calendarId: 'k' }), null);
});

test('taking it down stops its frames and its wake call', async () => {
  const env = load();
  const sky = env.sandbox.Chronicle.headerSky.mount(header(), { campaignId: 'c', calendarId: 'k' });
  await settle(); await settle();
  sky.destroy();
  assert.equal(env.pending(), 0);
  assert.equal(env.wakers.length, 1, 'only the shared sky loop still listens');
});
