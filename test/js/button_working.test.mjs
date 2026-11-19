// button_working.test.mjs — a button whose action runs shows its working
// state and can't be pressed twice (Chronicle.working in static/js/theme.js,
// and the htmx request events that drive it).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'theme.js'), 'utf8');

function btn(label, attrs = {}) {
  const a = { ...attrs };
  return {
    textContent: label,
    matches: (s) => s === '.btn' || s === '.btn[data-working]' && 'data-working' in a,
    hasAttribute: (k) => k in a,
    getAttribute: (k) => (k in a ? a[k] : null),
    setAttribute: (k, v) => { a[k] = String(v); },
    removeAttribute: (k) => { delete a[k]; },
    closest(s) { return s === '.btn[data-working]' ? ('data-working' in a ? this : null) : null; },
  };
}

function load() {
  const handlers = {};
  const attrs = {};
  const html = {
    classList: { add() {}, remove() {}, contains: () => false, toggle() {} },
    hasAttribute: (k) => k in attrs, getAttribute: () => null, setAttribute() {}, removeAttribute() {},
  };
  const sandbox = {
    document: {
      documentElement: html, querySelectorAll: () => [],
      addEventListener: (ev, fn) => { (handlers[ev] = handlers[ev] || []).push(fn); },
    },
    localStorage: { getItem: () => null, setItem() {} },
    matchMedia: () => ({ matches: false }),
    Chronicle: {},
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'theme.js' });
  return { C: sandbox.Chronicle, fire: (ev, e) => (handlers[ev] || []).forEach((fn) => fn(e)) };
}

test('working marks the button busy and swaps in its working label, then restores it', () => {
  const { C } = load();
  const b = btn('Save', { 'data-working-label': 'Saving…' });
  C.working(b, true);
  assert.equal(b.getAttribute('aria-busy'), 'true');
  assert.ok(b.hasAttribute('data-working'));
  assert.equal(b.textContent, 'Saving…');
  C.working(b, false);
  assert.equal(b.hasAttribute('data-working'), false);
  assert.equal(b.textContent, 'Save');
});

test('a button with no working label keeps its words', () => {
  const { C } = load();
  const b = btn('Publish');
  C.working(b, true);
  assert.equal(b.textContent, 'Publish');
  assert.ok(b.hasAttribute('data-working'));
});

test('an htmx request started by a button sets and clears its working state', () => {
  const { C, fire } = load();
  const b = btn('Send');
  fire('htmx:beforeRequest', { detail: { elt: b } });
  assert.ok(b.hasAttribute('data-working'));
  fire('htmx:afterRequest', { detail: { elt: b } });
  assert.equal(b.hasAttribute('data-working'), false);
  assert.ok(C.working);
});

test('a working button swallows a second press', () => {
  const { C, fire } = load();
  const b = btn('Send');
  C.working(b, true);
  let stopped = 0, prevented = 0;
  fire('click', { target: b, preventDefault: () => prevented++, stopPropagation: () => stopped++ });
  assert.deepEqual([prevented, stopped], [1, 1]);
  C.working(b, false);
  fire('click', { target: b, preventDefault: () => prevented++, stopPropagation: () => stopped++ });
  assert.deepEqual([prevented, stopped], [1, 1]);
});
