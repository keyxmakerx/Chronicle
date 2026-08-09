// widget_mount_unmatched_warning.test.mjs — contract for boot.js's
// unmatched-widget diagnostic (warnUnmatchedWidgets / scheduleUnmatchedWidgetScan).
//
// Regression guard for C-SWEEP-R3 "dead widget mounts never loaded". boot.js
// mounts a widget by looking its data-widget name up in the registry that
// Chronicle.register() fills, and mountElement() bails SILENTLY on a miss —
// legitimately, because registration races the scan. The cost of that silence
// is that "this widget has not registered YET" and "this widget's JS file is
// shipped on no page in the product" have the identical observable: an empty
// div, an empty console, and a page that looks finished. Three real widgets
// (aliases, inventory, transaction_log) sat permanently blank on entity pages
// that way — full backend, full REST routes, full widget JS, in no <script src>
// anywhere — and nothing ever said a word.
//
// The fix keeps the silent bail during mounting and adds ONE console.warn per
// unmatched name, emitted only after the load has settled (a setTimeout(…, 0)
// after DOMContentLoaded / htmx:afterSettle), so a widget that registers later
// in document order is counted as present rather than reported as missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'boot.js'), 'utf8');

/** A minimal element carrying only a data-widget attribute. */
function makeMount(name) {
  return {
    attributes: [{ name: 'data-widget', value: name }],
    getAttribute: (a) => (a === 'data-widget' ? name : null),
    querySelectorAll: () => [],
  };
}

/**
 * Boot boot.js in a vm over a document holding the given data-widget mounts,
 * then drive the real DOMContentLoaded listener it registers at load time.
 *
 * @param {string[]} mountNames - data-widget values present in the document.
 * @returns {{fireDOMContentLoaded: Function, fireAfterSettle: Function,
 *            warnings: string[], Chronicle: Object, flush: Function}}
 */
function boot(mountNames) {
  const docHandlers = {};
  const warnings = [];
  const timers = [];

  const mounts = mountNames.map(makeMount);
  const queryAll = (sel) => {
    if (sel === '[data-widget]') return mounts;
    const m = /^\[data-widget="(.+)"\]$/.exec(sel);
    if (m) return mounts.filter((el) => el.getAttribute('data-widget') === m[1]);
    return [];
  };

  const document = {
    querySelectorAll: queryAll,
    querySelector: () => null,
    getElementById: () => null,
    createElement: () => ({
      style: {}, className: '', innerHTML: '', textContent: '',
      setAttribute() {}, getAttribute() { return null; },
      classList: { add() {}, remove() {}, contains() { return false; } },
      appendChild() {}, removeChild() {}, insertBefore() {},
      addEventListener() {}, removeEventListener() {},
      querySelector() { return null; },
    }),
    addEventListener: (ev, fn) => {
      (docHandlers[ev] = docHandlers[ev] || []).push(fn);
    },
    removeEventListener: () => {},
    dispatchEvent: () => {},
    // 'loading': register() must NOT eagerly mount, so the DOMContentLoaded
    // pass below is the one and only mount pass — same as a real page load.
    readyState: 'loading',
    body: { classList: { add() {}, remove() {} } },
    cookie: '',
  };

  const window = { addEventListener: () => {}, dispatchEvent: () => {}, location: { pathname: '/' } };
  const htmx = { config: {} };
  function CustomEvent(type, init) { this.type = type; this.detail = (init || {}).detail || null; }

  const sandbox = {
    Chronicle: {},
    document,
    window,
    htmx,
    CustomEvent,
    console: { warn: (msg) => warnings.push(String(msg)), error: () => {}, log: () => {} },
    // Collect scheduled callbacks so the test controls when "settled" happens.
    setTimeout: (fn) => { timers.push(fn); return timers.length; },
    clearTimeout: () => {},
    Promise,
    WeakMap,
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);

  const fire = (ev, arg) => (docHandlers[ev] || []).forEach((fn) => fn(arg));

  return {
    warnings,
    Chronicle: sandbox.Chronicle,
    fireDOMContentLoaded: () => fire('DOMContentLoaded'),
    fireAfterSettle: (target) => fire('htmx:afterSettle', { detail: { target } }),
    /** Run every callback the code deferred to "after this turn". */
    flush: () => {
      while (timers.length) timers.shift()();
    },
    makeMount,
    queryAll,
  };
}

test('a mount whose widget was never registered is reported once, by name', () => {
  const b = boot(['aliases']);
  b.fireDOMContentLoaded();

  // Nothing is said during the mount pass itself — the bail stays silent,
  // because registration legitimately races the scan.
  assert.deepEqual(b.warnings, [], 'no warning before the load settles');

  b.flush();

  assert.equal(b.warnings.length, 1, 'exactly one warning after settle');
  assert.match(b.warnings[0], /data-widget="aliases"/);
  assert.match(b.warnings[0], /\[Chronicle\]/);
});

test('a registered widget is never reported, and a late registration is not either', () => {
  const b = boot(['aliases']);
  b.Chronicle.register('aliases', { init() {} });
  b.fireDOMContentLoaded();
  b.flush();

  assert.deepEqual(b.warnings, [], 'a registered widget must not be reported');

  // The race the silence exists to protect: a script later in document order
  // registers after the DOMContentLoaded mount pass but before the scan runs.
  const late = boot(['inventory']);
  late.fireDOMContentLoaded();
  late.Chronicle.register('inventory', { init() {} });
  late.flush();

  assert.deepEqual(late.warnings, [], 'a late registration must not be reported');
});

test('twenty mounts of one dead widget produce one line, not twenty', () => {
  const b = boot(Array(20).fill('transaction_log'));
  b.fireDOMContentLoaded();
  b.flush();

  assert.equal(b.warnings.length, 1);
  assert.match(b.warnings[0], /data-widget="transaction_log"/);
});

test('each distinct dead widget gets its own line, and repeats across swaps stay quiet', () => {
  const b = boot(['aliases', 'inventory', 'transaction_log']);
  b.fireDOMContentLoaded();
  b.flush();

  assert.equal(b.warnings.length, 3);
  for (const name of ['aliases', 'inventory', 'transaction_log']) {
    assert.ok(
      b.warnings.some((w) => w.includes(`data-widget="${name}"`)),
      `expected a warning naming ${name}`
    );
  }

  // An htmx swap that re-delivers the same dead mounts must not re-log them.
  b.fireAfterSettle({ querySelectorAll: b.queryAll });
  b.flush();
  assert.equal(b.warnings.length, 3, 'warn-once is per name, not per scan');
});

test('a dead mount arriving in an htmx-swapped fragment is reported', () => {
  const b = boot([]);
  b.fireDOMContentLoaded();
  b.flush();
  assert.deepEqual(b.warnings, []);

  const swapped = b.makeMount('ghost_widget');
  b.fireAfterSettle({ querySelectorAll: (sel) => (sel === '[data-widget]' ? [swapped] : []) });
  b.flush();

  assert.equal(b.warnings.length, 1);
  assert.match(b.warnings[0], /data-widget="ghost_widget"/);
});
