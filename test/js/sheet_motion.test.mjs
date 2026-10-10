// sheet_motion.test.mjs -- the character sheet's motion engine
// (static/js/sheet_motion.js).
//
// Pins the pure rules the engine runs on: the open/close state machine for a
// sheet's one panel, Calm and Off short-circuiting, which effect a changed
// value gets from the style's --change-move (and the fallback for a name it
// does not know), style resolution, and that a page with no [data-sheet] is
// left alone. The script returns before touching the page when there is no
// document, so the helpers load here as the browser loads them.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sheet_motion.js'), 'utf8');

function load() {
  globalThis.window = globalThis;
  globalThis.module = { exports: {} };
  vm.runInThisContext(src);
  const api = globalThis.module.exports;
  delete globalThis.module;
  return api;
}
const S = load();

test('styleFor: the campaign pick wins, then an author value, else modern; unknown names are modern', () => {
  const cases = [
    ['runes', undefined, 'runes'],
    ['runes', 'brass', 'runes'],
    [null, 'brass', 'brass'],
    [undefined, undefined, 'modern'],
    ['made-up', undefined, 'modern'],
    ['made-up', 'also-made-up', 'modern'],
    ['', 'pencil', 'pencil'],
  ];
  for (const [cz, existing, want] of cases) assert.equal(S.styleFor(cz, existing), want, `${cz}/${existing}`);
  assert.equal(S.STYLES.length, 12);
});

test('every style has a font entry, and only styles that need a family produce a link', () => {
  for (const s of S.STYLES) assert.ok(s in S.FONTS, s);
  assert.equal(S.fontHref('modern'), '');
  assert.equal(S.fontHref('parchment'), '');
  assert.equal(S.fontHref('nope'), '');
  const href = S.fontHref('brass');
  assert.match(href, /^https:\/\/fonts\.googleapis\.com\/css2\?family=Lora/);
  assert.match(href, /&display=swap$/);
});

test('modeOf: Off wins, then any Calm signal, else Full', () => {
  const cases = [
    [{}, 'full'],
    [{ motion: 'calm' }, 'calm'],
    [{ viewMotion: 'calm' }, 'calm'],
    [{ czReduce: true }, 'calm'],
    [{ motion: 'off' }, 'off'],
    [{ motion: 'off', viewMotion: 'calm' }, 'off'],
    [{ motion: 'off', czReduce: true }, 'off'],
  ];
  for (const [attrs, want] of cases) assert.equal(S.modeOf(attrs), want, JSON.stringify(attrs));
});

test('durFor: Full follows the token, Calm is a short fade, Off is nothing', () => {
  assert.equal(S.durFor('full', 360), 360);
  assert.equal(S.durFor('full', 230), 230);
  assert.equal(S.durFor('calm', 360), 120);
  assert.equal(S.durFor('off', 360), 0);
});

test('msOf and longestTransition read real CSS time lists', () => {
  assert.equal(S.msOf('360ms'), 360);
  assert.equal(S.msOf('0.36s'), 360);
  assert.equal(S.msOf(''), 0);
  assert.equal(S.msOf('auto'), 0);
  assert.equal(S.longestTransition('0.36s, 0.12s', '0s'), 360);
  assert.equal(S.longestTransition('0.2s, 0.36s', '0s, 0.1s'), 460);
  assert.equal(S.longestTransition('120ms', '0.4s, 0s'), 520);
  assert.equal(S.longestTransition('', ''), 0);
});

test('fxPlan: the style names the effect; unknown names fall back to roll; Calm fades; Off marks', () => {
  const full = (kind, old, now) => S.fxPlan(kind, 'full', old, now, true);
  for (const k of ['rewrite', 'roll', 'glitch', 'carve', 'odometer']) assert.equal(full(k, '4', '5'), 'fx:' + k);
  assert.equal(full('shimmer', '4', '5'), 'fx:roll', 'a name the engine does not know still gets an effect');
  assert.equal(full('mark', '4', '5'), 'mark');
  assert.equal(full('', '4', '5'), 'mark', 'no --change-move means the plain mark');
  assert.equal(full(undefined, '4', '5'), 'mark');
  assert.equal(full('roll', '5', '5'), 'mark', 'unchanged text gets the plain mark');
  assert.equal(full('carve', null, '5'), 'arrive:carve', 'something new arrives in the style\'s way');
  assert.equal(S.fxPlan('carve', 'calm', '4', '5', true), 'crossfade');
  assert.equal(S.fxPlan('carve', 'calm', null, '5', true), 'fade-in');
  assert.equal(S.fxPlan('carve', 'calm', '5', '5', true), 'mark');
  for (const k of ['rewrite', 'carve', 'shimmer']) assert.equal(S.fxPlan(k, 'off', '4', '5', true), 'mark', 'Off never animates: ' + k);
  assert.equal(S.fxPlan('roll', 'full', '4', '5', false), 'mark', 'no Web Animations means the plain mark');
});

test('foldPlan: only Full measures and animates a fold', () => {
  assert.equal(S.foldPlan('full'), 'animate');
  assert.equal(S.foldPlan('calm'), 'instant');
  assert.equal(S.foldPlan('off'), 'instant');
});

test('panelStep: the one-panel open/close state machine', () => {
  const a = { id: 'abilities', trigger: 'btnA' };
  const b = { id: 'kit', trigger: 'btnB' };

  // Opening from nothing.
  let r = S.panelStep(null, { type: 'open', ...a });
  assert.deepEqual(r.effects, ['open']);
  assert.deepEqual(r.state, { id: 'abilities', trigger: 'btnA', dirty: false });

  // The open panel's own button toggles it shut.
  const open = r.state;
  r = S.panelStep(open, { type: 'open', ...a });
  assert.deepEqual(r.effects, ['close']);
  assert.equal(r.state, null);

  // Another part closes the first and opens its own.
  r = S.panelStep(open, { type: 'open', ...b });
  assert.deepEqual(r.effects, ['close', 'open']);
  assert.equal(r.state.id, 'kit');

  // A panel holding an unsaved choice blocks being replaced or closed, until forced.
  const dirty = { ...open, dirty: true };
  r = S.panelStep(dirty, { type: 'open', ...b });
  assert.deepEqual(r.effects, ['warn']);
  assert.equal(r.state, dirty);
  r = S.panelStep(dirty, { type: 'open', ...a });
  assert.deepEqual(r.effects, ['warn'], 'its own button is a close, which is blocked too');
  r = S.panelStep(dirty, { type: 'close', force: false });
  assert.deepEqual(r.effects, ['warn']);
  r = S.panelStep(dirty, { type: 'close', force: true });
  assert.deepEqual(r.effects, ['close']);
  assert.equal(r.state, null);

  // Closing nothing is a no-op.
  r = S.panelStep(null, { type: 'close', force: false });
  assert.deepEqual(r.effects, []);
  assert.equal(r.state, null);
});

// ---------------------------------------------------------------------------
// The page: a fake document the script is run against.
// ---------------------------------------------------------------------------

function fakeEl(attrs = {}) {
  const a = { ...attrs };
  return {
    nodeType: 1,
    getAttribute: (k) => (k in a ? a[k] : null),
    setAttribute: (k, v) => { a[k] = String(v); },
    hasAttribute: (k) => k in a,
    classList: { add() {}, remove() {}, toggle() {}, contains: () => false },
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener() {},
    style: { setProperty() {} },
    attrs: a,
  };
}

// Runs the script as a page would load it. `roots` are the [data-sheet] elements the page has.
function runOnPage({ roots = [], htmlAttrs = {} } = {}) {
  const html = fakeEl(htmlAttrs);
  const docListeners = [];
  const appended = [];
  const observed = [];
  const doc = {
    readyState: 'complete',
    documentElement: html,
    head: { appendChild: (n) => appended.push(n) },
    createElement: (tag) => ({ tag, style: {}, setAttribute() {}, appendChild() {} }),
    querySelector: () => null,
    querySelectorAll: (sel) => (sel === '[data-sheet]' ? roots : []),
    addEventListener: (ev) => docListeners.push(ev),
  };
  const calls = [];
  const win = {
    Chronicle: undefined,
    addEventListener: (ev) => calls.push('window:' + ev),
    getComputedStyle: () => ({ getPropertyValue: () => '', transitionDuration: '0s', transitionDelay: '0s' }),
    MutationObserver: class { constructor() { observed.push('mo'); } observe() { observed.push('observe'); } },
    matchMedia: () => ({ matches: false }),
    devicePixelRatio: 1,
  };
  const g = globalThis;
  const saved = { window: g.window, document: g.document, MutationObserver: g.MutationObserver, getComputedStyle: g.getComputedStyle };
  g.window = win; g.document = doc; g.MutationObserver = win.MutationObserver; g.getComputedStyle = win.getComputedStyle;
  try { vm.runInThisContext(src); } finally {
    g.window = saved.window; g.document = saved.document; g.MutationObserver = saved.MutationObserver; g.getComputedStyle = saved.getComputedStyle;
  }
  return { html, docListeners, appended, observed, calls, win };
}

test('a page with no [data-sheet] is left alone', () => {
  const page = runOnPage({ roots: [] });
  assert.deepEqual(page.appended, [], 'no font link added');
  assert.deepEqual(page.observed, [], 'no observer started');
  assert.deepEqual(page.calls, [], 'no window listeners');
  assert.deepEqual(page.docListeners, ['htmx:afterSettle'], 'only the check for a sheet arriving by a boosted swap');
  assert.deepEqual(page.html.attrs, {}, 'the page element is not touched');
});

test('a [data-sheet] root gets its style from html[data-cz-sheet], else modern', () => {
  const root = fakeEl({ 'data-sheet': '' });
  const chosen = runOnPage({ roots: [root], htmlAttrs: { 'data-cz-sheet': 'brass' } });
  assert.equal(root.attrs['data-sheet-style'], 'brass');
  assert.ok(chosen.appended.length === 1 && /fonts\.googleapis\.com/.test(chosen.appended[0].href), 'brass asks for its fonts');
  assert.ok(chosen.observed.includes('observe'), 'a changed pick is followed');

  const plain = fakeEl({ 'data-sheet': '' });
  runOnPage({ roots: [plain] });
  assert.equal(plain.attrs['data-sheet-style'], 'modern');
});
