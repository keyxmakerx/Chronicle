// site_admin.test.mjs — pins site_admin.js: "Back to Chronicle" only ever
// points at a same-site page outside Site admin, whatever sessionStorage
// holds, and only the footer click and "Back to Chronicle" mark a way in or
// out for the next page.

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
const head = [];
globalThis.document = {
  addEventListener: (t, fn) => { if (t === 'click') clickHandler = fn; },
  documentElement: { getAttribute: () => null },
  head: { appendChild: (el) => head.push(el) },
  createElement: () => ({ textContent: '', parentNode: null }),
};
globalThis.CSSViewTransitionRule = function () {};
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
  return { back, el: { querySelector: () => back } };
}

// click fakes a click on an element matching sel.
function click(sel, extra = {}) {
  clickHandler({ target: { closest: (q) => (q === sel ? {} : null) }, button: 0, ...extra });
}
const clickEntry = (extra) => click('[data-site-admin-entry]', extra);

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

test('the footer click remembers the page and marks the way in', () => {
  mem.clear(); head.length = 0;
  clickEntry();
  assert.equal(mem.get('chronicle-admin-vt'), 'in');
  assert.equal(head.length, 1, 'this page opts into the transition');
  assert.match(head[0].textContent, /@view-transition\{navigation:auto\}/);
  const m = fakeMenu();
  C.siteAdminArrive(m.el);
  assert.equal(m.back.href, '/campaigns/abc/entities?page=2');
  const m2 = fakeMenu();
  C.siteAdminArrive(m2.el);
  assert.equal(m2.back.href, '/campaigns/abc/entities?page=2', 'the way back is kept across admin pages');
});

test('"Back to Chronicle" marks the way out, only from inside Site admin', () => {
  mem.clear(); head.length = 0;
  click('[data-admin-back]');
  assert.equal(mem.size, 0, 'outside Site admin it is not the way out');
  const saved = globalThis.location.pathname;
  globalThis.location.pathname = '/admin/users';
  click('[data-admin-back]');
  globalThis.location.pathname = saved;
  assert.equal(mem.get('chronicle-admin-vt'), 'out');
  assert.equal(head.length, 1);
});

test('motion off or no view transitions: the mark is set, no opt-in', () => {
  mem.clear(); head.length = 0;
  globalThis.document.documentElement.getAttribute = (n) => (n === 'data-motion' ? 'off' : null);
  clickEntry();
  globalThis.document.documentElement.getAttribute = () => null;
  const rule = globalThis.CSSViewTransitionRule;
  delete globalThis.CSSViewTransitionRule;
  clickEntry();
  globalThis.CSSViewTransitionRule = rule;
  assert.equal(head.length, 0);
  assert.equal(mem.get('chronicle-admin-vt'), 'in');
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

test('every way out on the page, the top bar\'s too, points at the remembered page', () => {
  mem.clear();
  mem.set('chronicle-admin-return', '/campaigns/abc');
  const m = fakeMenu();
  const leave = { href: '/campaigns', setAttribute(n, v) { this[n] = v; } };
  globalThis.document.querySelectorAll = (q) => (q === '[data-admin-back]' ? [m.back, leave] : []);
  C.siteAdminArrive(m.el);
  delete globalThis.document.querySelectorAll;
  assert.equal(m.back.href, '/campaigns/abc');
  assert.equal(leave.href, '/campaigns/abc');
});
