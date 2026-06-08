// sky_w1.test.mjs — W1 sky fixes, booted behaviourally in a node vm against a
// [data-cal-sky] DOM stub (same harness shape as sky_init_paint.test.mjs):
//   Q1  weather DIMS the sun but never fully hides it (thunderstorm → ~0.40,
//       floored at 0.28; clear → 1.0).
//   E1  the sun-bloom emitter spec carries spawn:'sun' so it anchors to the
//       sun position instead of scattering "mini suns" at the canvas edges.
//   E2  in production the engine sources the season/weather label from the seed
//       worldState, not the empty DATA stub (no blank-season / "Clear" clobber).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'cal-almanac.js');
const engineSrc = readFileSync(jsPath, 'utf8');

// A minimal element stub: settable style/textContent/attributes, null queries.
function el(extra) {
  return Object.assign({
    style: {
      setProperty() {},
      get left() { return this._l; }, set left(v) { this._l = v; },
      get top() { return this._t; }, set top(v) { this._t = v; },
      get opacity() { return this._o; }, set opacity(v) { this._o = v; },
    },
    className: '', innerHTML: '', textContent: '',
    _attrs: {},
    setAttribute(k, v) { this._attrs[k] = v; },
    getAttribute(k) { return this._attrs[k] || null; },
    removeAttribute(k) { delete this._attrs[k]; },
    appendChild() {}, addEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    getBoundingClientRect() { return { left: 0, top: 0, width: 1080, height: 200 }; },
  }, extra || {});
}

// Boot the engine with a production seed; returns the stable stub elements.
function boot(seedObj) {
  const sun = el();
  const subRest = el({ textContent: '' });
  const sky = el({
    querySelector(sel) {
      if (sel === '[data-cal-sky-sun]') return sun;
      if (sel === '[data-cal-sky-sub-rest]') return subRest;
      return el(); // weather/celestial/happening layers
    },
    querySelectorAll() { return []; }, // no moons
  });
  const seed = el({ getAttribute: () => JSON.stringify(seedObj) });
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
  vm.runInContext(engineSrc, sandbox, { filename: 'cal-almanac.js' });
  return { sun, subRest, sandbox };
}

const baseSeed = (extra) => Object.assign({
  timeOfDay: 0.5, // midday → sun mid-arc, opacity 1 before weather
  date: { year: 2026, month: 6, day: 8 },
}, extra || {});

test('Q1: thunderstorm dims the sun but keeps it visible (never hidden)', () => {
  const { sun } = boot(baseSeed({ weather: { type: 'thunderstorm', intensity: 1 } }));
  const op = parseFloat(sun.style.opacity);
  assert.ok(!Number.isNaN(op), 'sun opacity set on init');
  assert.ok(op >= 0.28, `sun must stay at least faintly visible in a storm (got ${op})`);
  assert.ok(op < 1, `thunderstorm must visibly dim the sun (got ${op})`);
});

test('Q1: clear weather leaves the sun at full brightness', () => {
  const { sun } = boot(baseSeed({ weather: { type: 'clear', intensity: 1 } }));
  assert.equal(parseFloat(sun.style.opacity), 1, 'clear midday sun is full opacity');
});

test('Q1: at night the sun is hidden — the weather floor does NOT lift it', () => {
  // t=0.95 → arcPos opacity 0 (below the horizon). The dim/floor is gated on
  // the sun being up, so even clear weather must leave it fully hidden.
  const { sun } = boot(baseSeed({ timeOfDay: 0.95, weather: { type: 'clear', intensity: 1 } }));
  assert.equal(parseFloat(sun.style.opacity), 0, 'the sun must set — invisible at night');
  // And a stormy night must also stay hidden (floor is daytime-only).
  const storm = boot(baseSeed({ timeOfDay: 0.02, weather: { type: 'thunderstorm', intensity: 1 } }));
  assert.equal(parseFloat(storm.sun.style.opacity), 0, 'no sun on a stormy night either');
});

test('E1: the sun-bloom emitter spec anchors to the sun (spawn:"sun")', () => {
  const { sandbox } = boot(baseSeed({ weather: { type: 'clear', intensity: 1 } }));
  const bloom = sandbox.window.__calCelestialEffects['sun-bloom'];
  assert.equal(bloom.particleSpec.spawn, 'sun',
    'sun-bloom must spawn at the sun position, not the canvas edges');
});

test('E2: production label shows the seed season + weather (no blank/Clear clobber)', () => {
  const { subRest } = boot(baseSeed({
    season: 'Winter',
    weather: { type: 'thunderstorm', intensity: 1 },
  }));
  assert.match(subRest.textContent, /Winter/, 'season must come from the seed, not blank');
  assert.match(subRest.textContent, /Thunderstorm/, 'weather must come from the seed, not "Clear"');
});
