// tuner_harness.mjs — shared node test harness for cal-timeline-tuner.js.
//
// The tuner JS is a browser IIFE; we run it in a node `vm` context with a
// minimal DOM stub + a working document event bus (so the cursor-sync
// DOM-event protocol can be exercised headless) and a CustomEvent shim.
// Most querySelectors return null → the DOM render pipelines no-op safely,
// while the pure helpers exposed on window.__tuner* are fully testable.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'cal-timeline-tuner.js');

// Mock dataset matching the shape the JS reads (timeline_mock.go). Seeds
// the restraint cases the backdrop tests assert:
//   1492-4-14 → special-moon day (sun+moon backdrop) + eclipse
//   1492-4-12 → plain weather (rain), NOT special
//   1489-3-8  → meteor shower (non-routine celestial), NOT special
//   1492-4-20 → nothing (no backdrop)
export const MOCK = {
  calendar: { name: 'Calendar of Harptos', epoch_name: 'DR', hours_per_day: 24 },
  days_per_month: 30,
  months_per_year: 12,
  current_year: 1492, current_month: 4, current_day: 14,
  eras: [
    { id: 'era-kings', name: 'Age of Kings', start_year: 0, end_year: 1486, color: 'oklch(0.62 0.16 75)', description: 'kings' },
    { id: 'era-reckoning', name: 'Reckoning', start_year: 1487, end_year: 0, color: 'oklch(0.62 0.18 22)', description: 'now' },
  ],
  tiers: [
    { id: 'major', name: 'Major', color: 'oklch(0.65 0.20 22)' },
    { id: 'standard', name: 'Standard', color: 'oklch(0.62 0.18 240)' },
    { id: 'detail', name: 'Detail', color: 'oklch(0.55 0.04 260)' },
  ],
  categories: [
    { id: 'battle', name: 'Battle', color: 'oklch(0.62 0.20 25)', icon: 'sword' },
    { id: 'founding', name: 'Founding', color: 'oklch(0.78 0.16 75)', icon: 'spire' },
  ],
  entities: [
    { id: 'ent-aragorn', name: 'Aragorn', type: 'npc', color: 'oklch(0.66 0.17 145)' },
    { id: 'ent-frodo', name: 'Frodo', type: 'npc', color: 'oklch(0.70 0.16 250)' },
  ],
  events: [
    { id: 'ev-a', name: 'Coronation', description: 'crowned', year: 1492, month: 4, day: 14, tier: 'major', category: 'founding', entities: ['ent-aragorn'] },
    { id: 'ev-b', name: 'Skirmish', description: 'fight', year: 1492, month: 4, day: 12, tier: 'detail', category: 'battle', entities: ['ent-aragorn'] },
    { id: 'ev-c', name: 'Ring Unmade', description: 'end', year: 1492, month: 4, day: 14, tier: 'major', category: 'founding', entities: ['ent-frodo'] },
  ],
  connections: [
    { source: 'ev-b', target: 'ev-a', type: 'caused', label: 'led to', entity_id: 'ent-aragorn' },
    { source: 'ev-a', target: 'ev-c', type: 'co-occurs', label: 'same dawn', entity_id: 'ent-aragorn' },
  ],
  special_moon_days: ['1492-4-14'],
  day_weather: { '1492-4-12': 'rain', '1492-4-13': 'thunderstorm' },
  celestial_events: {
    '1492-4-14': [{ type: 'eclipse-solar', name: 'Crowning Eclipse', start_time: 11, duration: 2 }],
    '1489-3-8': [{ type: 'meteor-shower', name: 'Falling Stars', start_time: -1, duration: 6 }],
  },
  weather_types: [],
};

// Boot a fresh, fully-initialised sandbox. Returns { window, bus } where
// bus.events is the captured list of dispatched custom events.
export function boot(opts) {
  opts = opts || {};
  const data = opts.data || MOCK;
  const dataEl = {
    getAttribute: (n) => (n === 'data-cal-tuner-data' ? JSON.stringify(data) : null),
  };
  // A DOM-ish node: enough surface for the render pipelines to no-op or
  // build small subtrees in registry-render tests.
  function node() {
    const children = [];
    const self = {
      children, className: '', textContent: '', hidden: false, title: '',
      style: { setProperty() {}, removeProperty() {} },
      classList: { add() {}, remove() {}, toggle() {}, contains() { return false; } },
      setAttribute() {}, getAttribute() { return null; }, removeAttribute() {},
      appendChild(c) { children.push(c); return c; },
      addEventListener() {}, removeEventListener() {},
      querySelector() { return null; }, querySelectorAll() { return []; },
      getBoundingClientRect() { return { left: 0, top: 0, width: 1080, height: 560 }; },
      get innerHTML() { return ''; }, set innerHTML(_v) { children.length = 0; },
    };
    return self;
  }

  // Document event bus (cursor-sync protocol).
  const listeners = {};
  const bus = { events: [] };
  const documentStub = {
    readyState: 'complete',
    getElementById: (id) => (id === 'cal-tuner-data' ? dataEl : null),
    querySelector: () => null,
    querySelectorAll: () => [],
    createElement: () => node(),
    createElementNS: () => node(),
    addEventListener: (type, cb) => { (listeners[type] = listeners[type] || []).push(cb); },
    removeEventListener: () => {},
    dispatchEvent: (ev) => {
      bus.events.push(ev);
      (listeners[ev.type] || []).forEach((cb) => { try { cb(ev); } catch (e) { bus.threw = e; } });
      return true;
    },
  };

  class CustomEvent {
    constructor(type, opts2) { this.type = type; this.detail = opts2 && opts2.detail; }
  }

  const sandbox = {
    console,
    CustomEvent,
    navigator: { hardwareConcurrency: 8 },
    devicePixelRatio: 1,
    matchMedia: () => ({ matches: !!opts.reduced }),
    requestAnimationFrame: () => 0,
    cancelAnimationFrame: () => {},
    setTimeout, clearTimeout,
    prompt: () => null,
    document: documentStub,
  };
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'cal-timeline-tuner.js' });
  sandbox.__bus = bus;
  return { window: sandbox.window, bus };
}
