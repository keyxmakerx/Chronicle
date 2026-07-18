// skybox_multi_instance.test.mjs — C-SKYBOX-MULTI-INSTANCE (booked r23).
//
// Repro: when TWO skybox widget instances mount on one page at once (e.g. a
// dashboard block + the calendar sky pane, both showing the same campaign),
// the engine (cal-almanac.js) historically bound its canvas surfaces to only
// the FIRST [data-cal-sky] band it found — `document.querySelector` picks
// the first match, and `band.__calInited` on the first [data-cal-worldstate]
// node made every subsequent init() call for the SAME page a no-op. The
// second band's canvas never got a CalParticleEngine surface, so it never
// painted a frame: it rendered its server-side-rendered gradient once and
// sat there, static, forever.
//
// This boots the REAL cal-almanac.js engine (not a mock) in a node vm with a
// small scoped-query DOM stub that supports TWO independent [data-cal-sky]
// trees on one "page", and asserts both canvases actually get painted by the
// shared rAF loop — the failing-then-passing repro is the deliverable pin
// (see dispatches/chronicle/C-SKYBOX-MULTI-INSTANCE.md).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'cal-almanac.js');

// ---- a tiny real (scoped) DOM: parent/child + attribute-selector queries ---

function selMatches(el, selector) {
  return selector.split(',').some((part) => {
    part = part.trim();
    const groups = part.match(/\[[^\]]+\]/g);
    if (!groups) return false;
    return groups.every((g) => {
      const inner = g.slice(1, -1);
      const eq = inner.indexOf('=');
      if (eq === -1) return Object.prototype.hasOwnProperty.call(el._attrs, inner);
      const name = inner.slice(0, eq);
      const val = inner.slice(eq + 1).replace(/^"(.*)"$/, '$1');
      return el._attrs[name] === val;
    });
  });
}
function queryAllIn(root, sel) {
  const out = [];
  (function walk(node) {
    for (const c of node.children) {
      if (selMatches(c, sel)) out.push(c);
      walk(c);
    }
  })(root);
  return out;
}
function makeStyle() {
  const props = {};
  return {
    setProperty(k, v) { props[k] = v; },
    getPropertyValue(k) { return props[k] || ''; },
  };
}
function makeEl(attrs) {
  const el = {
    _attrs: Object.assign({}, attrs),
    children: [],
    parentNode: null,
    className: '',
    textContent: '',
    innerHTML: '',
    _listeners: {},
    style: makeStyle(),
    getAttribute(n) { return Object.prototype.hasOwnProperty.call(el._attrs, n) ? el._attrs[n] : null; },
    setAttribute(n, v) { el._attrs[n] = String(v); },
    removeAttribute(n) { delete el._attrs[n]; },
    appendChild(c) { c.parentNode = el; el.children.push(c); return c; },
    insertBefore(c, ref) {
      c.parentNode = el;
      const idx = el.children.indexOf(ref);
      if (idx === -1) el.children.push(c); else el.children.splice(idx, 0, c);
      return c;
    },
    removeChild(c) {
      const idx = el.children.indexOf(c);
      if (idx !== -1) el.children.splice(idx, 1);
      c.parentNode = null;
      return c;
    },
    addEventListener(type, fn) { (el._listeners[type] = el._listeners[type] || []).push(fn); },
    removeEventListener(type, fn) {
      const a = el._listeners[type]; if (!a) return;
      const i = a.indexOf(fn); if (i >= 0) a.splice(i, 1);
    },
    querySelector(sel) { return queryAllIn(el, sel)[0] || null; },
    querySelectorAll(sel) { return queryAllIn(el, sel); },
    getBoundingClientRect() { return { left: 0, top: 0, width: 1080, height: 200 }; },
  };
  return el;
}
function makeCanvas(clearCounter) {
  const grad = { addColorStop() {} };
  const ctx = new Proxy({}, {
    get(_, k) {
      if (k === 'createLinearGradient' || k === 'createRadialGradient') return () => grad;
      if (k === 'clearRect') return () => { clearCounter.n++; };
      return () => {};
    },
  });
  const c = makeEl({});
  c.width = 320; c.height = 200;
  c.getContext = () => ctx;
  return c;
}

// One self-contained skybox band: its own hidden worldstate-seed blob +
// its own [data-cal-sky] scaffold — mirrors internal/widgets/skybox/skybox.templ.
function makeBand(seedObj, paintCounter) {
  const seed = makeEl({ 'data-cal-worldstate': JSON.stringify(seedObj) });
  const root = makeEl({ 'data-cal-sky': '', 'data-cal-sky-weather': 'clear' });
  root.appendChild(makeEl({ 'data-cal-sky-weather-layer': '' }));
  root.appendChild(makeEl({ 'data-cal-sky-celestial-layer': '' }));
  const canvas = makeCanvas(paintCounter);
  canvas._attrs['data-cal-sky-canvas'] = '';
  root.appendChild(canvas);
  const arc = makeEl({ 'data-cal-sky-arc': '' });
  const sun = makeEl({ 'data-cal-sky-sun': '', 'data-cal-sun-state': 'default' });
  arc.appendChild(sun);
  root.appendChild(arc);
  const front = makeCanvas({ n: 0 });
  front._attrs['data-cal-sky-canvas-front'] = '';
  root.appendChild(front);
  const overlay = makeEl({ 'data-cal-sky-overlay': '' });
  overlay.appendChild(makeEl({ 'data-cal-sky-date': '' }));
  const sub = makeEl({});
  sub.appendChild(makeEl({ 'data-cal-sky-time-label': '' }));
  const rest = makeEl({ 'data-cal-sky-sub-rest': '' });
  rest.textContent = ' · x · y · z';
  sub.appendChild(rest);
  overlay.appendChild(sub);
  overlay.appendChild(makeEl({ 'data-cal-sky-happening': '' }));
  root.appendChild(overlay);
  return { seed, root, canvas, sun, paintCounter };
}

function boot(bands) {
  const page = makeEl({});
  bands.forEach((b) => { page.appendChild(b.seed); page.appendChild(b.root); });
  const listeners = {};
  const rafQ = [];
  class CustomEvent { constructor(type, o) { this.type = type; this.detail = o && o.detail; } }
  const sandbox = {
    console, CustomEvent,
    navigator: { hardwareConcurrency: 8 }, devicePixelRatio: 1,
    matchMedia: () => ({ matches: false }),
    requestAnimationFrame: (cb) => { rafQ.push(cb); return rafQ.length; },
    cancelAnimationFrame: () => {},
    setTimeout, clearTimeout, scrollX: 0, scrollY: 0,
    document: {
      readyState: 'complete',
      getElementById: () => null,
      querySelector: (sel) => queryAllIn(page, sel)[0] || null,
      querySelectorAll: (sel) => queryAllIn(page, sel),
      createElement: (tag) => (tag === 'canvas' ? makeCanvas({ n: 0 }) : makeEl({})),
      addEventListener: (type, cb) => { (listeners[type] = listeners[type] || []).push(cb); },
      removeEventListener: (type, cb) => {
        const a = listeners[type]; if (!a) return; const i = a.indexOf(cb); if (i >= 0) a.splice(i, 1);
      },
      dispatchEvent: (ev) => { (listeners[ev.type] || []).forEach((cb) => { try { cb(ev); } catch (e) {} }); return true; },
    },
  };
  sandbox.window = sandbox; sandbox.global = sandbox;
  sandbox.__pump = (n) => { for (let i = 0; i < (n || 1) && rafQ.length; i++) { const cb = rafQ.shift(); try { cb(16 * (i + 1)); } catch (e) { sandbox.__pumpThrew = e; } } };
  sandbox.__page = page;
  sandbox.__fireHtmx = (type) => sandbox.document.dispatchEvent(new CustomEvent(type));
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'cal-almanac.js' });
  return sandbox.window;
}

const SEED = { timeOfDay: 0.5, season: 'Spring', date: { year: 1492, month: 4, day: 15 }, weather: { type: 'clear' }, events: [] };

test('two skybox instances mounted at once BOTH get frame ticks (not just the first)', () => {
  const p1 = { n: 0 }, p2 = { n: 0 };
  const bandA = makeBand(SEED, p1);
  const bandB = makeBand(SEED, p2);
  const w = boot([bandA, bandB]);

  assert.equal(w.__pumpThrew, undefined, 'engine boot did not throw');
  for (let i = 0; i < 6; i++) w.__pump(1);

  assert.ok(p1.n > 0, 'band A (first-in-document-order) canvas was painted');
  assert.ok(p2.n > 0, 'band B (second) canvas was ALSO painted — this is the bug: it used to stay static');
});

test('both instances place their sun (independent DOM writes, not just the first)', () => {
  const p1 = { n: 0 }, p2 = { n: 0 };
  const bandA = makeBand(SEED, p1);
  const bandB = makeBand(SEED, p2);
  boot([bandA, bandB]);

  assert.ok(bandA.sun.style.getPropertyValue('--cal-sun-rays') !== '', 'band A sun painted');
  assert.ok(bandB.sun.style.getPropertyValue('--cal-sun-rays') !== '', 'band B sun ALSO painted (not left at its SSR default)');
});

test('destroying one band does not stop the other from ticking', () => {
  const p1 = { n: 0 }, p2 = { n: 0 };
  const bandA = makeBand(SEED, p1);
  const bandB = makeBand(SEED, p2);
  const w = boot([bandA, bandB]);
  for (let i = 0; i < 4; i++) w.__pump(1);
  assert.ok(p1.n > 0 && p2.n > 0, 'both ticking before teardown');

  // Simulate band A's widget/DOM being torn down (unmounted) and an htmx
  // settle firing afterward (the real-world trigger for a re-init pass).
  w.__page.removeChild(bandA.seed);
  w.__page.removeChild(bandA.root);
  assert.doesNotThrow(() => w.__fireHtmx('htmx:afterSettle'), 're-init after a sibling band is removed must not throw');

  const beforeB = p2.n;
  for (let i = 0; i < 6; i++) w.__pump(1);
  assert.ok(p2.n > beforeB, 'band B keeps ticking after band A is torn down — the survivor is unaffected');
});

test('reduced motion freezes BOTH instances (no partial animation)', () => {
  const p1 = { n: 0 }, p2 = { n: 0 };
  const bandA = makeBand(SEED, p1);
  const bandB = makeBand(SEED, p2);
  const page = makeEl({});
  [bandA, bandB].forEach((b) => { page.appendChild(b.seed); page.appendChild(b.root); });
  const listeners = {};
  const sandbox = {
    console,
    navigator: { hardwareConcurrency: 8 }, devicePixelRatio: 1,
    matchMedia: () => ({ matches: true }), // prefers-reduced-motion: reduce
    requestAnimationFrame: () => 0, cancelAnimationFrame: () => {},
    setTimeout, clearTimeout, scrollX: 0, scrollY: 0,
    document: {
      readyState: 'complete',
      getElementById: () => null,
      querySelector: (sel) => queryAllIn(page, sel)[0] || null,
      querySelectorAll: (sel) => queryAllIn(page, sel),
      createElement: (tag) => (tag === 'canvas' ? makeCanvas({ n: 0 }) : makeEl({})),
      addEventListener: (type, cb) => { (listeners[type] = listeners[type] || []).push(cb); },
      removeEventListener() {},
      dispatchEvent: () => true,
    },
  };
  sandbox.window = sandbox; sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'cal-almanac.js' });

  assert.equal(sandbox.CalParticleEngine.reduced(), true, 'engine reports reduced motion');
  // Both surfaces get the SAME (nonzero) static-paint treatment — reduced
  // motion must not single out the first band and leave the second one
  // uninitialised (zero paints) or the two in an asymmetric state.
  assert.ok(p1.n > 0, 'band A got its static paint');
  assert.ok(p2.n > 0, 'band B got its static paint too — reduced motion applies to every instance, not just the first');
  assert.equal(p1.n, p2.n, 'both surfaces treated identically under reduced motion');
});
