// harness.mjs — shared node test harness for cal-almanac.js.
//
// The almanac JS is a browser IIFE; we run it in a node `vm` context with a
// minimal DOM stub (every querySelector returns null → render pipelines no-op
// safely) so the world-state pub/sub + the exposed pure helpers
// (__calWorldState, __calEffects, __calHgSim, …) can be exercised headless.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'cal-almanac.js');

// Arrays returned from the vm context carry the sandbox realm's
// Array.prototype, which deepStrictEqual rejects. Copy before comparing.
export const arr = (x) => Array.from(x);

// Minimal mock matching what the seed + lookups read.
export const MOCK = {
  current_year: 1, current_month: 1, current_day: 1, sky_time: 0.5,
  seasons: [{ start: 1, name: 'Spring' }],
  calendar: { hours_per_day: 24 },
  moons: [{ id: 1, phase_offset: 0.1, size: 1 }],
  day_weather: {}, celestial_events: {},
  weather_types: [], weather_effects: [], celestial_effects: [],
};

// Build a fresh, fully-initialised sandbox (the IIFE auto-runs init()).
export function boot() {
  const dataEl = {
    getAttribute: (n) => (n === 'data-cal-almanac-data' ? JSON.stringify(MOCK) : null),
    textContent: '',
  };
  const node = () => ({
    className: '', style: { setProperty() {} },
    setAttribute() {}, getAttribute() { return null; }, removeAttribute() {},
    appendChild() {}, replaceWith() {}, addEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    getBoundingClientRect() { return { left: 0, top: 0, width: 1080, height: 200 }; },
  });
  const sandbox = {
    console,
    navigator: { hardwareConcurrency: 8 },
    devicePixelRatio: 1,
    matchMedia: () => ({ matches: false }),
    requestAnimationFrame: () => 0,
    cancelAnimationFrame: () => {},
    setTimeout, clearTimeout,
    scrollX: 0, scrollY: 0,
    document: {
      readyState: 'complete',
      getElementById: (id) => (id === 'cal-almanac-data' ? dataEl : null),
      querySelector: () => null,
      querySelectorAll: () => [],
      addEventListener: () => {},
      createElement: () => node(),
    },
  };
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'cal-almanac.js' });
  return sandbox.window;
}
