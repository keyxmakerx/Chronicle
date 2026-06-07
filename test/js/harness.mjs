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
// opts.reduced → matchMedia reports prefers-reduced-motion (so Wave-3 time
// tweens resolve to instant snaps synchronously, testable without a real rAF).
export function boot(opts) {
  opts = opts || {};
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
  // engine mode: a mock 2D context + a manually-pumped rAF, so the particle
  // engine actually runs (it needs a real canvas + getContext) — lets us test
  // the rAF resilience guard + setProfile cap-trim against the live loop.
  const grad = { addColorStop() {} };
  const ctx2d = new Proxy({}, { get: (_, k) => (k === 'createLinearGradient' || k === 'createRadialGradient') ? () => grad : () => {} });
  const canvas = () => Object.assign(node(), { width: 320, height: 200, getContext: () => ctx2d });
  const rafQ = [];
  // Document event bus + CustomEvent shim (cursor-sync DOM-event protocol,
  // Tuner §J1). Backward-compatible: tests that don't use it are unaffected.
  const listeners = {};
  const bus = { events: [] };
  class CustomEvent {
    constructor(type, o) { this.type = type; this.detail = o && o.detail; }
  }
  const sandbox = {
    console,
    CustomEvent,
    navigator: { hardwareConcurrency: 8 },
    devicePixelRatio: 1,
    matchMedia: () => ({ matches: !!opts.reduced }),
    requestAnimationFrame: opts.engine ? ((cb) => { rafQ.push(cb); sandbox.__rafCount = (sandbox.__rafCount || 0) + 1; return rafQ.length; }) : (() => 0),
    cancelAnimationFrame: () => {},
    setTimeout, clearTimeout,
    scrollX: 0, scrollY: 0,
    document: {
      readyState: 'complete',
      getElementById: (id) => (id === 'cal-almanac-data' ? dataEl : null),
      querySelector: (sel) => (opts.engine && typeof sel === 'string' && sel.indexOf('canvas') !== -1) ? canvas() : null,
      querySelectorAll: () => [],
      addEventListener: (type, cb) => { (listeners[type] = listeners[type] || []).push(cb); },
      removeEventListener: () => {},
      dispatchEvent: (ev) => { bus.events.push(ev); (listeners[ev.type] || []).forEach((cb) => { try { cb(ev); } catch (e) { bus.threw = e; } }); return true; },
      createElement: (tag) => (tag === 'canvas' ? canvas() : node()),
    },
  };
  sandbox.__bus = bus;
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  // pump(n) runs up to n queued rAF callbacks (one frame each).
  sandbox.__pump = (n) => { for (let i = 0; i < (n || 1) && rafQ.length; i++) { const cb = rafQ.shift(); try { cb(16 * (i + 1)); } catch (e) { sandbox.__pumpThrew = true; } } };
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'cal-almanac.js' });
  return sandbox.window;
}
