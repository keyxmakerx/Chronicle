// calendar_era_blend.test.mjs — pins the era colour field's pure parts
// (look normalising, feel resolution, the device and size rules) and the
// painter's motion contract: it draws once and stops when nothing should
// move (Still feel, reduced motion, colours off), and once at rest after
// the ease-out its loop stops entirely. Also pins calendar_view.js's
// EraMath: which era a day belongs to, a month's era change, and how an
// era's span reads.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const widgets = path.join(here, '..', '..', 'static', 'js', 'widgets');
const blendSrc = readFileSync(path.join(widgets, 'calendar_era_blend.js'), 'utf8');
const viewSrc = readFileSync(path.join(widgets, 'calendar_view.js'), 'utf8');

function fakeCanvas() {
  return {
    width: 0, height: 0, style: {}, className: '', parentNode: null, offsetWidth: 0,
    setAttribute() {},
    getContext() {
      return {
        createImageData: (w, h) => ({ data: new Uint8ClampedArray(w * h * 4) }),
        putImageData() {}, drawImage() {}, clearRect() {}, fillRect() {},
        getImageData: () => ({ data: [250, 248, 244, 255] }),
        set fillStyle(v) {}, get fillStyle() { return '#fff'; }
      };
    }
  };
}

// loadBlend runs the browser script in a sandbox with a hand-cranked
// animation frame clock, so the loop can be stepped deterministically.
function loadBlend({ reduced = false, cores = 8 } = {}) {
  let frameQueue = [];
  let now = 0;
  const listeners = {};
  const sandbox = {
    module: { exports: {} }, console, Math, setTimeout: () => 1, clearTimeout() {},
    navigator: { hardwareConcurrency: cores },
    document: { hidden: false, createElement: fakeCanvas, addEventListener() {} },
    requestAnimationFrame(fn) { frameQueue.push(fn); return frameQueue.length; },
    cancelAnimationFrame() { frameQueue = []; }
  };
  sandbox.window = {
    addEventListener(ev, fn) { (listeners[ev] = listeners[ev] || []).push(fn); },
    matchMedia: () => ({ matches: reduced, addEventListener() {} })
  };
  vm.createContext(sandbox);
  vm.runInContext(blendSrc, sandbox);
  const api = sandbox.module.exports;
  return {
    api,
    // step runs one pending frame `ms` after the last.
    step(ms = 16) {
      const q = frameQueue; frameQueue = [];
      now += ms;
      q.forEach((fn) => fn(now));
      return q.length;
    },
    pending: () => frameQueue.length,
    fire: (ev) => (listeners[ev] || []).forEach((fn) => fn({}))
  };
}

function host() {
  return { insertBefore(el) { el.parentNode = this; }, firstChild: null };
}

const scene = (eras, split = null) => ({
  box: { left: 0, top: 0, width: 280, height: 200 },
  rows: [0, 40, 80, 120, 160].map((top) => ({ top, height: 40 })),
  cols: 7, off: 2, days: 30, split,
  eras
});
const gas = { key: 1, color: '#6e1a2a', color_2: '#d6893a', style: 'gas', feel: null };
const ink = { key: 2, color: '#1d3f6e', color_2: '#5aa6a0', style: 'ink', feel: null };

test('normalizeLook fills and clamps what the server or a hand edit sends', () => {
  const { api } = loadBlend();
  assert.deepEqual({ ...api.normalizeLook(null) }, { colors_on: true, feel: 'subtle', intensity: 1, speed: 1 });
  assert.deepEqual({ ...api.normalizeLook({ colors_on: false, feel: 'wild', intensity: 9, speed: -2 }) },
    { colors_on: false, feel: 'subtle', intensity: 3, speed: 0 });
  assert.equal(api.normalizeLook({ feel: 'custom', intensity: 'x' }).intensity, 1);
});

test('feelOf: an era’s own feel wins, then the calendar’s preset, then its custom numbers', () => {
  const { api } = loadBlend();
  const custom = { feel: 'custom', intensity: 2.2, speed: 0.4 };
  const tests = [
    { look: custom, era: { feel: 'lively' }, want: { intensity: 1.7, speed: 1.9 } },
    { look: custom, era: { feel: null }, want: { intensity: 2.2, speed: 0.4 } },
    { look: { feel: 'still' }, era: {}, want: { intensity: 1, speed: 0 } },
    { look: null, era: null, want: { intensity: 1, speed: 1 } }
  ];
  for (const t of tests) assert.deepEqual({ ...api.feelOf(t.look, t.era) }, t.want);
});

test('moving a slider off a preset reads as Custom, and the words follow the mockup', () => {
  const { api } = loadBlend();
  assert.equal(api.matchPreset(1, 1), 'subtle');
  assert.equal(api.matchPreset(1.7, 1.9), 'lively');
  assert.equal(api.matchPreset(1, 0), 'still');
  assert.equal(api.matchPreset(1.4, 1), 'custom');
  assert.deepEqual([0.5, 1, 1.5, 2.5].map(api.wordI), ['faint', 'subtle', 'clear', 'strong']);
  assert.deepEqual([0, 0.5, 1, 1.5, 2.4].map(api.wordS), ['still', 'slower', 'slow', 'lively', 'quick']);
});

test('a weak device gets the light version: fewer, capped pixels', () => {
  const { api } = loadBlend();
  assert.equal(api.deviceIsLimited({ hardwareConcurrency: 4 }), true);
  assert.equal(api.deviceIsLimited({ deviceMemory: 2, hardwareConcurrency: 16 }), true);
  assert.equal(api.deviceIsLimited({ hardwareConcurrency: 8, deviceMemory: 8 }), false);
  const full = api.canvasSize(1200, 700, false), light = api.canvasSize(1200, 700, true);
  assert.ok(full[0] * full[1] <= 16000 + 200, `full ${full}`);
  assert.ok(light[0] * light[1] <= 2600 + 60, `light ${light}`);
  assert.ok(light[0] < full[0]);
});

test('every style yields an in-range colour, moving or still', () => {
  const { api } = loadBlend();
  const out = [0, 0, 0], a = api.soften('#6e1a2a'), b = api.soften('#d6893a');
  for (const style of ['gas', 'ink']) {
    for (let i = 0; i < 50; i++) {
      api.sample(out, style, a, b, i * 0.37, i * 0.21, i, 2, 3);
      out.forEach((c) => assert.ok(Number.isFinite(c) && c >= -1 && c <= 300, `${style} ${out}`));
    }
  }
  api.sampleStill(out, a, b, 1, 2);
  out.forEach((c) => assert.ok(c >= 0 && c <= 255));
  assert.deepEqual([...api.hexRGB('#abc')], [170, 187, 204]);
  assert.deepEqual([...api.hexRGB('url(x)')], [128, 128, 128]);
});

test('the loop eases to rest and then stops drawing', () => {
  const b = loadBlend();
  const p = b.api.create(host(), { surface: () => '#fff' });
  p.setScene(scene([gas, ink], 18));
  assert.ok(b.pending() > 0, 'a moving look starts the loop');
  for (let i = 0; i < 10; i++) b.step();
  assert.equal(p.state().running, true);
  p.rest();
  let frames = 0;
  while (b.pending() && frames < 1000) { b.step(16); frames++; }
  assert.equal(p.state().running, false, 'stops once at rest');
  assert.equal(p.state().activity, 0);
  // ~2 s of 16 ms frames to come to rest, never an abrupt stop.
  assert.ok(frames > 100 && frames < 160, `took ${frames} frames`);
  p.wake();
  assert.ok(b.pending() > 0, 'input starts it again');
});

test('nothing moves: draw once, no loop', () => {
  const cases = [
    { name: 'Still feel', opts: {}, look: { feel: 'still' } },
    { name: 'reduced motion', opts: { reduced: true }, look: null },
    { name: 'colours off', opts: {}, look: { colors_on: false } }
  ];
  for (const c of cases) {
    const b = loadBlend(c.opts);
    const p = b.api.create(host(), { surface: () => '#fff' });
    p.setLook(c.look);
    p.setScene(scene([gas, ink], 18));
    assert.equal(b.pending(), 0, `${c.name}: no frame queued`);
    assert.equal(p.state().canMove, false, c.name);
  }
});

test('a light device draws at most about 30 times a second', () => {
  const b = loadBlend({ cores: 2 });
  const p = b.api.create(host(), { surface: () => '#fff' });
  p.setScene(scene([gas]));
  assert.equal(p.state().light, true);
  b.step(16); // first frame draws
  const before = p.state().frames;
  for (let i = 0; i < 60; i++) b.step(16); // ~1 s at 60 Hz
  const drawn = p.state().frames - before;
  assert.ok(drawn <= 32 && drawn >= 25, `drew ${drawn} frames in a second`);
});

/* ---------- EraMath (calendar_view.js) ---------- */
function loadEraMath() {
  const sandbox = { module: { exports: {} }, Chronicle: { register() {} }, console };
  vm.createContext(sandbox);
  vm.runInContext(viewSrc, sandbox);
  return sandbox.module.exports.EraMath;
}
// Twelve 30-day months, a 7-day week, today 9 Seedtide 1022.
const cal = (eras) => ({
  months: Array.from({ length: 12 }, (_, i) => ({ name: 'M' + (i + 1), days: 30 })),
  weekdays: Array.from({ length: 7 }, (_, i) => ({ name: 'D' + i })),
  current_year: 1022, current_month: 8, current_day: 9,
  eras
});
const dragons = { id: 1, name: 'Dragons', start_year: 1, start_month: 1, start_day: 1, end_year: 1022, end_month: 6, end_day: 17 };
const humans = { id: 2, name: 'Humanity', start_year: 1022, start_month: 6, start_day: 18 };
const ash = { id: 3, name: 'Ash', start_year: 1040, start_month: 1, start_day: 1, hidden_until_begins: true };

test('EraMath: a month where an era begins splits on that day', () => {
  const E = loadEraMath(), c = cal([humans, ash, dragons]);
  const list = E.sorted(c);
  assert.deepEqual(list.map((r) => r.era.name), ['Dragons', 'Humanity', 'Ash']);
  const m = E.month(c, list, 1022, 6);
  assert.equal(m.first.era.name, 'Dragons');
  assert.equal(m.last.era.name, 'Humanity');
  assert.equal(m.change, 18);
  assert.deepEqual([...m.begins.map((b) => b.day)], [18]);
  const whole = E.month(c, list, 1022, 8);
  assert.equal(whole.change, null);
  assert.equal(whole.last.era.name, 'Humanity');
});

test('EraMath: an open era ends where the next begins; overlap goes to the later start', () => {
  const E = loadEraMath(), c = cal([humans, ash]);
  const list = E.sorted(c);
  assert.deepEqual({ ...list[0].end }, { y: 1039, m: 12, d: 30 });
  const overlap = cal([{ id: 9, name: 'Long', start_year: 1, start_month: 1, start_day: 1, end_year: 2000 }, humans]);
  assert.equal(E.at(E.sorted(overlap), 1022 * 360 + 7 * 30).era.name, 'Humanity');
});

test('EraMath: the span reads against the world date; years run straight through', () => {
  const E = loadEraMath(), c = cal([dragons, humans, ash]), list = E.sorted(c);
  assert.deepEqual({ ...E.when(c, list[0]) }, { range: '1 M1 1 – 17 M6 1022', len: 'Lasted 1,021 years, 5 months' });
  assert.deepEqual({ ...E.when(c, list[1]) }, { range: '18 M6 1022 – today', len: '1 month, 22 days so far' });
  assert.equal(E.when(c, list[2]).len, 'Begins in 17 years, 4 months');
  assert.equal(E.secret(c, list[2]), true);
  assert.equal(E.secret(c, list[1]), false);
});

test('EraMath: ribbon shares have a floor and mark today in the current era', () => {
  const E = loadEraMath(), c = cal([dragons, humans, ash]), list = E.sorted(c);
  const r = E.ribbon(c, list);
  r.forEach((x) => assert.ok(x.flex >= 18));
  assert.equal(r[0].here, null);
  assert.ok(r[1].here > 0 && r[1].here <= 1);
  assert.equal(r[2].here, null);
});

/* ---------- the Era look part's save (calendar_era_look.js) ---------- */
test('Era look save sends only the eras and fields that changed; null clears', () => {
  const src = readFileSync(path.join(widgets, 'calendar_era_look.js'), 'utf8');
  const sandbox = { module: { exports: {} }, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const { changedEras } = sandbox.module.exports;
  const saved = [
    { id: 1, color: '#111111', color_2: '#222222', style: 'gas', feel: null },
    { id: 2, color: '#333333', color_2: null, style: 'ink', feel: 'lively' }
  ];
  const now = [
    { id: 1, color: '#111111', color_2: '#222222', style: 'gas', feel: null },
    { id: 2, color: '#333333', color_2: '#444444', style: 'ink', feel: null }
  ];
  assert.equal(JSON.stringify(changedEras(saved, now)), JSON.stringify([{ id: 2, color_2: '#444444', feel: null }]));
  assert.equal(changedEras(saved, saved).length, 0);
});
