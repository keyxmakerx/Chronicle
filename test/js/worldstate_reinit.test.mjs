// worldstate_reinit.test.mjs — C-WIDGET-BINDING-QA2 Part A. The worldstate band
// arrives via hx-boost navigation + the P4b binding swap, but its <script>
// can't re-run under htmx.config.allowScriptTags=false. So cal-almanac.js must
// re-initialise the engine against a freshly-swapped #cal-v2-worldstate band on
// htmx:afterSettle — once per band node, tearing down the prior band first.
//
// Runs cal-almanac.js in a node vm with a minimal DOM stub whose
// getElementById('cal-v2-worldstate') returns a controllable band, so we can
// assert the per-band init marking + the afterSettle re-init without a browser.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'cal-almanac.js');

function band() {
  // A band node stub: the engine reads data-cal-worldstate (→ empty seed here)
  // and stamps its own __calInited marker.
  return {
    getAttribute: (n) => (n === 'data-cal-worldstate' ? '{}' : null),
    setAttribute() {}, removeAttribute() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    style: { setProperty() {} }, classList: { add() {}, remove() {} },
    appendChild() {}, addEventListener() {},
  };
}

// boot() loads cal-almanac.js with a swappable band; returns handles to drive
// htmx events + swap the band.
function boot() {
  const state = { current: band() };
  const listeners = {};
  const bus = { threw: null };
  class CustomEvent { constructor(type, o) { this.type = type; this.detail = o && o.detail; } }
  const sandbox = {
    console, CustomEvent,
    navigator: { hardwareConcurrency: 8 }, devicePixelRatio: 1,
    matchMedia: () => ({ matches: false }),
    requestAnimationFrame: () => 0, cancelAnimationFrame: () => {},
    setTimeout, clearTimeout, scrollX: 0, scrollY: 0,
    document: {
      readyState: 'complete',
      getElementById: (id) => (id === 'cal-v2-worldstate' ? state.current : null),
      querySelector: () => null, querySelectorAll: () => [],
      createElement: () => band(),
      addEventListener: (type, cb) => { (listeners[type] = listeners[type] || []).push(cb); },
      removeEventListener: (type, cb) => {
        const a = listeners[type]; if (!a) return; const i = a.indexOf(cb); if (i >= 0) a.splice(i, 1);
      },
      dispatchEvent: (ev) => { (listeners[ev.type] || []).forEach((cb) => { try { cb(ev); } catch (e) { bus.threw = e; } }); return true; },
    },
  };
  sandbox.window = sandbox; sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'cal-almanac.js' });
  const fire = (type) => sandbox.document.dispatchEvent(new CustomEvent(type));
  const swap = () => { state.current = band(); return state.current; };
  return { w: sandbox.window, listeners, bus, fire, swap, getBand: () => state.current };
}

test('boots the engine against the production band (marks it inited)', () => {
  const { w, getBand, bus } = boot();
  assert.equal(w.__calAlmanacInited, true, 'engine inited');
  assert.equal(getBand().__calInited, true, 'the band node is marked inited');
  assert.equal(bus.threw, null, 'no uncaught error during prod init');
});

test('registers htmx re-init listeners', () => {
  const { listeners } = boot();
  assert.ok((listeners['htmx:afterSettle'] || []).length >= 1, 'afterSettle bound');
  assert.ok((listeners['htmx:load'] || []).length >= 1, 'htmx:load bound');
});

test('re-init is idempotent for the same band, and re-inits a swapped band', () => {
  const { fire, swap, getBand, bus } = boot();
  const first = getBand();
  // Same band still present → afterSettle must NOT re-run (band already inited).
  fire('htmx:afterSettle');
  assert.equal(first.__calInited, true, 'same band stays inited');
  assert.equal(bus.threw, null, 'no throw on idempotent re-init');
  // A boosted nav / P4b swap injects a FRESH band node → it must re-init.
  const next = swap();
  assert.ok(!next.__calInited, 'fresh band starts uninited');
  fire('htmx:afterSettle');
  assert.equal(next.__calInited, true, 'swapped-in band is re-initialised');
  assert.equal(bus.threw, null, 'no throw on band-swap re-init');
});
