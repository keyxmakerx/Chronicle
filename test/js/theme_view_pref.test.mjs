// theme_view_pref.test.mjs — the theme toggle (static/js/theme.js) and the
// My view live-apply hook. Signed in (html[data-view-theme] present) a toggle
// saves to the account and leaves localStorage alone; signed out it keeps
// using localStorage; a failed save puts the old choice back.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'theme.js'), 'utf8');

function load({ attrs = {}, dark = false, prefersDark = false, saveOk = true } = {}) {
  const classes = new Set(dark ? ['dark'] : []);
  const store = {};
  const calls = [];
  const toasts = [];
  const html = {
    classList: {
      add: (c) => classes.add(c), remove: (c) => classes.delete(c),
      contains: (c) => classes.has(c), toggle: (c, on) => (on ? classes.add(c) : classes.delete(c)),
    },
    hasAttribute: (k) => k in attrs,
    getAttribute: (k) => (k in attrs ? attrs[k] : null),
    setAttribute: (k, v) => { attrs[k] = String(v); },
    removeAttribute: (k) => { delete attrs[k]; },
  };
  const sandbox = {
    document: { documentElement: html, addEventListener() {}, querySelectorAll: () => [] },
    localStorage: { getItem: (k) => store[k] ?? null, setItem: (k, v) => { store[k] = v; } },
    matchMedia: () => ({ matches: prefersDark }),
    Chronicle: {
      apiFetch: (url, opts) => { calls.push({ url, opts }); return Promise.resolve({ ok: saveOk }); },
      notify: (m) => toasts.push(m),
    },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'theme.js' });
  return { C: sandbox.Chronicle, attrs, classes, store, calls, toasts };
}

const tick = () => new Promise((r) => setImmediate(r));

test('signed in: toggle saves to the account and does not touch localStorage', async () => {
  const { C, attrs, classes, store, calls } = load({ attrs: { 'data-view-theme': 'device' } });
  C.toggleTheme();
  await tick();
  assert.equal(classes.has('dark'), true);
  assert.equal(attrs['data-view-theme'], 'dark');
  assert.equal(JSON.stringify(calls.map((c) => [c.url, c.opts.method, c.opts.body])), JSON.stringify([['/account/view-prefs', 'PUT', { theme: 'dark' }]]));
  assert.deepEqual(store, {});
});

test('signed out: toggle uses localStorage and makes no request', () => {
  const { C, store, calls } = load();
  C.toggleTheme();
  assert.equal(store['chronicle-theme'], 'dark');
  assert.equal(calls.length, 0);
});

test('a failed save restores the previous choice and says so', async () => {
  const { C, attrs, classes, toasts } = load({ attrs: { 'data-view-theme': 'light' }, saveOk: false });
  C.setTheme('dark');
  await tick();
  assert.equal(attrs['data-view-theme'], 'light');
  assert.equal(classes.has('dark'), false);
  assert.equal(toasts.length, 1);
});

test('applyViewPref: each choice sets or clears only its own attribute', () => {
  for (const [key, value, attr, want] of [
    ['textSize', 'larger', 'data-view-text', 'larger'],
    ['textSize', 'standard', 'data-view-text', undefined],
    ['contrast', 'high', 'data-view-contrast', 'high'],
    ['contrast', 'standard', 'data-view-contrast', undefined],
    ['motion', 'calm', 'data-view-motion', 'calm'],
    ['motion', 'owner', 'data-view-motion', undefined],
  ]) {
    const { C, attrs } = load({ attrs: { 'data-view-theme': 'device', 'data-view-text': 'largest' } });
    C.applyViewPref(key, value);
    assert.equal(attrs[attr], want, `${key}=${value}`);
  }
});

test('applyViewPref motion: nav-rm follows Calmer but never drops the owner switch', () => {
  const a = load({ attrs: { 'data-view-theme': 'device' } });
  a.C.applyViewPref('motion', 'calm');
  assert.equal(a.classes.has('nav-rm'), true);
  a.C.applyViewPref('motion', 'owner');
  assert.equal(a.classes.has('nav-rm'), false);

  const b = load({ attrs: { 'data-view-theme': 'device', 'data-cz-reduce': '1' } });
  b.C.applyViewPref('motion', 'calm');
  b.C.applyViewPref('motion', 'owner');
  assert.equal(b.classes.has('nav-rm'), true);
});

test('applyViewPref theme: device follows the OS, explicit wins', () => {
  const a = load({ attrs: { 'data-view-theme': 'light' }, prefersDark: true });
  a.C.applyViewPref('theme', 'device');
  assert.equal(a.classes.has('dark'), true);
  a.C.applyViewPref('theme', 'light');
  assert.equal(a.classes.has('dark'), false);
});
