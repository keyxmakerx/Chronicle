// site_admin_reveal.test.mjs — pins site_admin_reveal.js's plan: the way in
// plays only on an admin page and the way out only outside one, Off plays
// nothing, and without cross-document view transitions only the way in plays
// (the page rising by itself).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'site_admin_reveal.js'), 'utf8');

function load(stored, pathname, motion, hasVT) {
  const mem = new Map(stored ? [['chronicle-admin-vt', stored]] : []);
  const attrs = new Map(motion ? [['data-motion', motion]] : []);
  const classes = new Set();
  const head = [];
  const listeners = {};
  const win = {
    sessionStorage: {
      getItem: (k) => (mem.has(k) ? mem.get(k) : null),
      removeItem: (k) => mem.delete(k),
    },
    addEventListener: (t, fn) => { listeners[t] = fn; },
  };
  if (hasVT) win.CSSViewTransitionRule = function () {};
  const ctx = {
    window: win,
    location: { pathname },
    document: {
      documentElement: {
        getAttribute: (n) => (attrs.has(n) ? attrs.get(n) : null),
        setAttribute: (n, v) => attrs.set(n, v),
        removeAttribute: (n) => attrs.delete(n),
        classList: { add: (c) => classes.add(c), remove: (c) => classes.delete(c) },
      },
      head: { appendChild: (el) => { head.push(el); el.parentNode = { removeChild: () => head.splice(head.indexOf(el), 1) }; } },
      createElement: () => ({ textContent: '' }),
      addEventListener: (t, fn) => { listeners['doc:' + t] = fn; },
      removeEventListener: () => {},
      body: {},
    },
  };
  vm.runInNewContext(src, ctx);
  return { mem, attrs, classes, head, listeners, ctx, plan: win.Chronicle._siteAdminRevealPlan };
}

test('plan matches the mark to the page', () => {
  const { plan } = load(null, '/', null, true);
  const cases = [
    ['in', '/admin', null, true, 'vt'],
    ['in', '/admin/users?x=1', 'calm', true, 'vt'],
    ['in', '/admin', null, false, 'self'],
    ['out', '/campaigns/abc', null, true, 'vt'],
    ['out', '/campaigns/abc', null, false, ''],
    ['in', '/campaigns', null, true, ''],
    ['out', '/admin', null, true, ''],
    ['in', '/administrators', null, true, ''],
    ['in', '/admin', 'off', true, ''],
    ['sideways', '/admin', null, true, ''],
    [null, '/admin', null, true, ''],
  ];
  for (const [dir, p, motion, vt, want] of cases) {
    assert.equal(plan(dir, p, motion, vt), want, `${dir} ${p} ${motion} ${vt}`);
  }
});

test('the way in opts this page in once, then cleans up', async () => {
  const r = load('in', '/admin', null, true);
  assert.equal(r.attrs.get('data-admin-vt'), 'in');
  assert.ok(r.classes.has('admin-vt'));
  assert.equal(r.head.length, 1);
  let finish;
  r.listeners.pagereveal({ viewTransition: { finished: new Promise((res) => { finish = res; }) } });
  finish();
  await new Promise((res) => setTimeout(res, 0));
  assert.equal(r.attrs.has('data-admin-vt'), false);
  assert.equal(r.classes.has('admin-vt'), false);
  assert.equal(r.head.length, 0, 'later navigation is not opted in');
  assert.equal(r.mem.size, 0, 'the mark is used once');
});

test('without view transitions the admin page rises by itself', () => {
  const r = load('in', '/admin', null, false);
  assert.equal(r.attrs.get('data-admin-vt'), 'in');
  assert.equal(r.classes.has('admin-vt'), false);
  assert.equal(r.head.length, 0);
  r.listeners['doc:animationend']({ target: r.ctx.document.body, animationName: 'admin-rise' });
  assert.equal(r.attrs.has('data-admin-vt'), false);
  assert.equal(r.mem.size, 0);
});

test('a stale mark or motion off plays nothing and is cleared', () => {
  for (const [dir, p, motion] of [['in', '/campaigns', null], ['out', '/admin', null], ['in', '/admin', 'off']]) {
    const r = load(dir, p, motion, true);
    assert.equal(r.attrs.has('data-admin-vt'), false, `${dir} ${p}`);
    assert.equal(r.head.length, 0);
    assert.equal(r.mem.size, 0);
  }
});
