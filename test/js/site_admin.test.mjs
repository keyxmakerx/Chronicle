// site_admin.test.mjs — pins site_admin.js: "Back to Chronicle" only ever
// points at a same-site page outside Site admin, whatever sessionStorage
// holds, and the entrance plays once after the footer click, not on later
// admin pages.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'site_admin.js'), 'utf8');

let clickHandler = null;
const mem = new Map();
globalThis.window = globalThis;
globalThis.document = { addEventListener: (t, fn) => { if (t === 'click') clickHandler = fn; } };
globalThis.sessionStorage = {
  getItem: (k) => (mem.has(k) ? mem.get(k) : null),
  setItem: (k, v) => mem.set(k, String(v)),
  removeItem: (k) => mem.delete(k),
};
globalThis.location = { pathname: '/campaigns/abc/entities', search: '?page=2' };
vm.runInThisContext(src);
const C = globalThis.Chronicle;

function fakeMenu() {
  const back = { href: '/campaigns', setAttribute(n, v) { this[n] = v; } };
  const classes = new Set();
  const sb = {
    classList: { add: (c) => classes.add(c), remove: (c) => classes.delete(c) },
    addEventListener() {}, removeEventListener() {},
  };
  return {
    back, classes,
    el: { querySelector: () => back, closest: () => sb },
  };
}

function clickEntry(extra = {}) {
  clickHandler({ target: { closest: () => ({}) }, button: 0, ...extra });
}

test('safeReturn keeps same-site pages outside Site admin', () => {
  const ok = ['/campaigns', '/campaigns/abc/entities?page=2', '/', '/account#look'];
  for (const p of ok) assert.equal(C._siteAdminSafeReturn(p), p, p);
});

test('safeReturn refuses other sites, admin pages and junk', () => {
  const bad = [
    null, undefined, '', 'https://evil.example', '//evil.example', '/\\evil.example',
    '\\\\evil.example', 'javascript:alert(1)', '/a\\b', '/x\u0000y', '/x\ty',
    '/admin', '/admin/users', '/admin?x=1', '/' + 'a'.repeat(3000),
  ];
  for (const p of bad) assert.equal(C._siteAdminSafeReturn(p), '/campaigns', String(p));
  assert.equal(C._siteAdminSafeReturn('/administrators'), '/administrators');
});

test('the footer click remembers the page and plays the entrance once', () => {
  mem.clear();
  clickEntry();
  const m = fakeMenu();
  C.siteAdminArrive(m.el);
  assert.equal(m.back.href, '/campaigns/abc/entities?page=2');
  assert.ok(m.classes.has('admin-arrive'));

  const m2 = fakeMenu();
  C.siteAdminArrive(m2.el);
  assert.equal(m2.back.href, '/campaigns/abc/entities?page=2', 'the way back is kept across admin pages');
  assert.ok(!m2.classes.has('admin-arrive'), 'later admin pages arrive without the entrance');
});

test('a click from inside Site admin or into a new tab changes nothing', () => {
  mem.clear();
  clickEntry({ ctrlKey: true });
  assert.equal(mem.size, 0);
  const saved = globalThis.location.pathname;
  globalThis.location.pathname = '/admin/users';
  clickEntry();
  globalThis.location.pathname = saved;
  assert.equal(mem.size, 0);
});

test('a tampered stored path falls back to the campaigns list', () => {
  mem.clear();
  mem.set('chronicle-admin-return', '//evil.example/phish');
  const m = fakeMenu();
  C.siteAdminArrive(m.el);
  assert.equal(m.back.href, '/campaigns');
});
