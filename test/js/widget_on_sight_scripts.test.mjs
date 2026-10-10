// widget_on_sight_scripts.test.mjs — contract for boot.js's on-sight widget
// loader (ADR-063): a mount whose widget is not registered fetches the
// scripts the layout lists for it, in order, once per page, and register()
// then mounts it. Unlisted widgets keep the old unmatched-widget warning.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'boot.js'), 'utf8');

function makeMount(name) {
  return {
    attributes: [{ name: 'data-widget', value: name }],
    getAttribute: (a) => (a === 'data-widget' ? name : null),
    querySelectorAll: () => [],
  };
}

/**
 * Boot boot.js in a vm over a document holding the given mounts and the
 * given widget script list, recording every script it appends to <head>.
 */
function boot(mountNames, manifest) {
  const docHandlers = {};
  const warnings = [];
  const timers = [];
  const appended = [];
  const mounts = mountNames.map(makeMount);
  const queryAll = (sel) => {
    if (sel === '[data-widget]') return mounts;
    const m = /^\[data-widget="(.+)"\]$/.exec(sel);
    if (m) return mounts.filter((el) => el.getAttribute('data-widget') === m[1]);
    return [];
  };
  const plainEl = () => ({
    style: {}, className: '', innerHTML: '', textContent: '',
    setAttribute() {}, getAttribute() { return null; },
    classList: { add() {}, remove() {}, contains() { return false; } },
    appendChild() {}, removeChild() {}, insertBefore() {},
    addEventListener() {}, removeEventListener() {},
    querySelector() { return null; },
  });
  const manifestEl = manifest === undefined ? null : { textContent: JSON.stringify(manifest) };
  const document = {
    querySelectorAll: queryAll,
    querySelector: () => null,
    getElementById: (id) => (id === 'chronicle-widget-scripts' ? manifestEl : null),
    createElement: plainEl,
    addEventListener: (ev, fn) => { (docHandlers[ev] = docHandlers[ev] || []).push(fn); },
    removeEventListener: () => {},
    dispatchEvent: () => {},
    readyState: 'loading',
    head: { appendChild: (el) => appended.push(el) },
    body: { classList: { add() {}, remove() {} } },
    cookie: '',
  };
  const window = { addEventListener: () => {}, dispatchEvent: () => {}, location: { pathname: '/' } };
  function CustomEvent(type, init) { this.type = type; this.detail = (init || {}).detail || null; }
  const sandbox = {
    Chronicle: {}, document, window, htmx: { config: {} }, CustomEvent,
    console: { warn: (msg) => warnings.push(String(msg)), error: () => {}, log: () => {} },
    setTimeout: (fn) => { timers.push(fn); return timers.length; },
    clearTimeout: () => {},
    Promise, WeakMap,
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const fire = (ev, arg) => (docHandlers[ev] || []).forEach((fn) => fn(arg));
  return {
    warnings, appended, document, Chronicle: sandbox.Chronicle,
    fireDOMContentLoaded: () => fire('DOMContentLoaded'),
    fireAfterSettle: (target) => fire('htmx:afterSettle', { detail: { target } }),
    flush: () => { while (timers.length) timers.shift()(); },
  };
}

const LIST = { 'timeline-viz': ['/static/js/widgets/groups.js?v=1', '/static/plugins/timeline/js/timeline_viz.js?v=1'] };

test('a listed widget fetches its scripts in order, with async off', () => {
  const b = boot(['timeline-viz'], LIST);
  b.fireDOMContentLoaded();
  assert.deepEqual(b.appended.map((s) => s.src), LIST['timeline-viz']);
  assert.ok(b.appended.every((s) => s.async === false), 'async=false keeps the listed order');
});

test('the widget mounts once its script registers it', () => {
  const b = boot(['timeline-viz'], LIST);
  const inits = [];
  b.fireDOMContentLoaded();
  // The last script arriving is what register() looks like from boot.js.
  b.document.readyState = 'complete';
  b.Chronicle.register('timeline-viz', { init: (el) => inits.push(el) });
  b.appended[1].onload();
  b.flush();
  assert.equal(inits.length, 1);
  assert.deepEqual(b.warnings, []);
});

test('scripts are fetched once per page, across mounts and htmx swaps', () => {
  const b = boot(['timeline-viz', 'timeline-viz'], LIST);
  b.fireDOMContentLoaded();
  b.fireAfterSettle({ querySelectorAll: () => [makeMount('timeline-viz')] });
  assert.equal(b.appended.length, 2);
});

test('a widget arriving by boosted navigation fetches its scripts', () => {
  const b = boot([], LIST);
  b.fireDOMContentLoaded();
  assert.equal(b.appended.length, 0, 'nothing fetched on a page without the widget');
  b.fireAfterSettle({ querySelectorAll: (sel) => (sel === '[data-widget]' ? [makeMount('timeline-viz')] : []) });
  assert.equal(b.appended.length, 2);
});

test('a script shared by two widgets is fetched once', () => {
  const b = boot(['a', 'b'], { a: ['/s/shared.js', '/s/a.js'], b: ['/s/shared.js', '/s/b.js'] });
  b.fireDOMContentLoaded();
  assert.deepEqual(b.appended.map((s) => s.src), ['/s/shared.js', '/s/a.js', '/s/b.js']);
});

test('a listed widget is not reported as dead while its scripts load', () => {
  const b = boot(['timeline-viz'], LIST);
  b.fireDOMContentLoaded();
  b.flush();
  assert.deepEqual(b.warnings, []);
});

test('a failed download and a script that never registers are each reported', () => {
  const b = boot(['timeline-viz'], LIST);
  b.fireDOMContentLoaded();
  b.appended[0].onerror();
  assert.match(b.warnings[0], /Could not load \/static\/js\/widgets\/groups\.js/);
  b.appended[1].onload();
  assert.match(b.warnings[1], /did not register data-widget="timeline-viz"/);
});

test('an unlisted widget fetches nothing and keeps the dead-mount warning', () => {
  const b = boot(['ghost'], LIST);
  b.fireDOMContentLoaded();
  b.flush();
  assert.equal(b.appended.length, 0);
  assert.match(b.warnings[0], /No implementation registered for data-widget="ghost"/);
});

test('a page without the list behaves as before', () => {
  const b = boot(['ghost']);
  b.fireDOMContentLoaded();
  b.flush();
  assert.equal(b.appended.length, 0);
  assert.equal(b.warnings.length, 1);
});

test('a list entry named like an Object property is not mistaken for a widget', () => {
  const b = boot(['constructor', 'toString'], {});
  b.fireDOMContentLoaded();
  assert.equal(b.appended.length, 0);
});
