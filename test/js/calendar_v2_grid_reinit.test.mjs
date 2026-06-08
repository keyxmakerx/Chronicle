// calendar_v2_grid_reinit.test.mjs — C-CAL-V2-MONTH-GRID-ALIGN-FIX #3. The V2
// shell + event-grid JS bind day-add / card-edit / popover handlers, but they
// arrive via hx-boost navigation and re-render on view/date nav — and under
// htmx.config.allowScriptTags=false their <script> never re-runs. So they must
// (re-)bind on htmx:afterSettle, once per root node (guarded so a swapped grid
// doesn't double-bind). These tests run each IIFE in a node vm with a minimal
// DOM whose [data-cal-v2-root] is swappable, and assert the per-root marking +
// the afterSettle re-init.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsDir = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js');

function root() {
  // A [data-cal-v2-root] stub with an empty dataset → the grid init returns
  // early (no calendar id), but boot() still stamps the per-root marker, which
  // is what the re-init guard keys off.
  return { dataset: {}, querySelector: () => null, querySelectorAll: () => [], addEventListener() {} };
}

function boot(file, marker) {
  const state = { current: root() };
  const listeners = {};
  const bus = { threw: null };
  class CustomEvent { constructor(type) { this.type = type; } }
  const sandbox = {
    console,
    CustomEvent,
    Chronicle: { apiFetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve({}) }), notify() {} },
    setTimeout, clearTimeout,
    document: {
      readyState: 'complete',
      querySelector: (sel) => (sel === '[data-cal-v2-root]' ? state.current : null),
      querySelectorAll: () => [],
      getElementById: () => null,
      addEventListener: (type, cb) => { (listeners[type] = listeners[type] || []).push(cb); },
      removeEventListener: () => {},
      dispatchEvent: (ev) => { (listeners[ev.type] || []).forEach((cb) => { try { cb(ev); } catch (e) { bus.threw = e; } }); return true; },
      createElement: () => root(),
    },
  };
  sandbox.window = sandbox; sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(join(jsDir, file), 'utf8'), sandbox, { filename: file });
  return {
    listeners, bus,
    fire: (t) => sandbox.document.dispatchEvent(new CustomEvent(t)),
    swap: () => { state.current = root(); return state.current; },
    getRoot: () => state.current,
    marked: (r) => !!r[marker],
  };
}

for (const [file, marker] of [['event_grid.js', '__eventGridInited'], ['calendar_v2_shell.js', '__calV2ShellInited']]) {
  test(`${file}: marks the root inited on boot + registers htmx re-init`, () => {
    const h = boot(file, marker);
    assert.equal(h.marked(h.getRoot()), true, 'root marked inited');
    assert.equal(h.bus.threw, null, 'no throw on init');
    assert.ok((h.listeners['htmx:afterSettle'] || []).length >= 1, 'afterSettle bound');
    assert.ok((h.listeners['htmx:load'] || []).length >= 1, 'htmx:load bound');
  });

  test(`${file}: idempotent for the same root, re-inits a swapped root`, () => {
    const h = boot(file, marker);
    const first = h.getRoot();
    h.fire('htmx:afterSettle'); // same root → no re-bind, no error
    assert.equal(h.marked(first), true);
    assert.equal(h.bus.threw, null);
    const next = h.swap(); // boosted nav / re-render injects a fresh root
    assert.equal(h.marked(next), false, 'fresh root starts unmarked');
    h.fire('htmx:afterSettle');
    assert.equal(h.marked(next), true, 'swapped-in root is re-initialised');
    assert.equal(h.bus.threw, null);
  });
}
