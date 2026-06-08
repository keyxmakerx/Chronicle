// sky_init_paint.test.mjs — C-CAL-V2-SKY-RENDER-COMPLETION FIX A. The
// production band runs the engine's day pipeline on init but historically
// never the TIME pipeline — so the sun was never placed and the smooth gradient
// never ran on first load. The sky-band-ambient init block now also runs
// renderTimePipeline + applySunState + refeedSky. This boots cal-almanac.js in
// a node vm with a [data-cal-sky] DOM stub (sun + layers) and asserts the sun is
// POSITIONED on init (style.left/top/opacity set) — i.e. the time paint ran.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'cal-almanac.js');

// A minimal element stub: settable style/innerHTML/attributes, null queries.
function el(extra) {
  return Object.assign({
    style: {
      setProperty() {},
      get left() { return this._l; }, set left(v) { this._l = v; },
      get top() { return this._t; }, set top(v) { this._t = v; },
      get opacity() { return this._o; }, set opacity(v) { this._o = v; },
    },
    className: '', innerHTML: '', textContent: 'x · y · z',
    _attrs: {},
    setAttribute(k, v) { this._attrs[k] = v; },
    getAttribute(k) { return this._attrs[k] || null; },
    removeAttribute(k) { delete this._attrs[k]; },
    appendChild() {}, addEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    getBoundingClientRect() { return { left: 0, top: 0, width: 1080, height: 200 }; },
  }, extra || {});
}

test('the sky-band-ambient init places the sun (initial time paint)', () => {
  const sun = el();
  const sky = el({
    querySelector(sel) {
      if (sel === '[data-cal-sky-sun]') return sun;
      // weather/celestial/happening/sub-rest layers the pipelines write into.
      return el();
    },
    querySelectorAll() { return []; }, // no moons
  });
  const seed = el({ getAttribute: () => '{"timeOfDay":0.67,"date":{"year":2026,"month":6,"day":8}}' });
  const sandbox = {
    console,
    navigator: { hardwareConcurrency: 8 }, devicePixelRatio: 1,
    matchMedia: () => ({ matches: false }),
    requestAnimationFrame: () => 0, cancelAnimationFrame: () => {},
    setTimeout, clearTimeout, scrollX: 0, scrollY: 0,
    document: {
      readyState: 'complete',
      getElementById: (id) => (id === 'cal-v2-worldstate' ? seed : null),
      querySelector: (sel) => (sel === '[data-cal-sky]' ? sky : null),
      querySelectorAll: () => [],
      createElement: () => el(),
      addEventListener() {}, removeEventListener() {}, dispatchEvent() { return true; },
    },
  };
  sandbox.window = sandbox; sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'cal-almanac.js' });

  // renderTimePipeline → place(sun, arcPos(t)) sets these on init. Before the
  // fix the sun was never placed (style untouched).
  assert.ok(sun.style.left != null, 'sun left positioned on init');
  assert.ok(sun.style.top != null, 'sun top positioned on init');
  assert.ok(sun.style.opacity != null, 'sun opacity set on init');
});
