// permissions_inline.test.mjs — C-ENTITY-PERMISSIONS-UX Part 2. The
// permissions widget's inline layout (data-layout="inline"): it builds a
// summary trigger over a collapsible in-flow panel (NOT a body-attached
// slide-in card), expands/collapses in place, and tears down cleanly. The
// default (non-inline) layout still builds the right-edge slide-in card +
// backdrop on <body> (regression guard). Visual feel is the operator's gate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'widgets', 'permissions.js');

// Minimal DOM: a node whose className and classList share one token set, so
// the widget's mix of `el.className = …` and `el.classList.add(…)` stays
// consistent and queryable.
function makeNode(tag) {
  const classes = new Set();
  const node = {
    tagName: (tag || 'div').toUpperCase(),
    children: [],
    _handlers: {},
    _attrs: {},
    style: {},
    offsetWidth: 0,
    get className() { return Array.from(classes).join(' '); },
    set className(v) { classes.clear(); String(v).split(/\s+/).forEach((c) => c && classes.add(c)); },
    classList: {
      add: (c) => classes.add(c),
      remove: (c) => classes.delete(c),
      contains: (c) => classes.has(c),
      toggle: (c, f) => { const on = f === undefined ? !classes.has(c) : f; if (on) classes.add(c); else classes.delete(c); return on; },
    },
    setAttribute(k, v) { this._attrs[k] = String(v); },
    getAttribute(k) { return k in this._attrs ? this._attrs[k] : null; },
    removeAttribute(k) { delete this._attrs[k]; },
    appendChild(c) { this.children.push(c); c._parent = this; return c; },
    removeChild(c) { const i = this.children.indexOf(c); if (i >= 0) this.children.splice(i, 1); return c; },
    addEventListener(ev, fn) { (this._handlers[ev] = this._handlers[ev] || []).push(fn); },
    removeEventListener() {},
    dispatch(ev) { (this._handlers[ev] || []).forEach((fn) => fn({ key: '', preventDefault() {} })); },
    set innerHTML(_v) { this.children = []; },
    get innerHTML() { return ''; },
    querySelector() { return null; },
    querySelectorAll() { return []; },
    get parentNode() { return this._parent || null; },
  };
  return node;
}

// Recursively find the first descendant carrying a class.
function findByClass(root, cls) {
  for (const c of root.children || []) {
    if (c.classList && c.classList.contains(cls)) return c;
    const deep = findByClass(c, cls);
    if (deep) return deep;
  }
  return null;
}

function boot() {
  const head = makeNode('head');
  const body = makeNode('body');
  const styleRegistry = {};
  const document = {
    getElementById: (id) => styleRegistry[id] || null,
    createElement: (tag) => {
      const n = makeNode(tag);
      // The widget tags the injected <style> with an id; register it so the
      // "inject once" guard works across multiple inits.
      const origSet = Object.getOwnPropertyDescriptor(n, 'setAttribute');
      n.setAttribute = function (k, v) { origSet.value.call(n, k, v); };
      Object.defineProperty(n, 'id', { get() { return this._attrs.id; }, set(v) { this._attrs.id = v; styleRegistry[v] = this; } });
      return n;
    },
    head, body,
    addEventListener() {}, removeEventListener() {},
    querySelector() { return null; },
  };
  const apiCalls = [];
  const sandbox = {
    console,
    document,
    AbortController: class { constructor() { this.signal = {}; } abort() {} },
    setTimeout, clearTimeout,
    Chronicle: {
      _impls: {},
      register(name, impl) { this._impls[name] = impl; },
      escapeHtml: (s) => String(s == null ? '' : s),
      apiFetch: (url, opts) => {
        apiCalls.push({ url, opts });
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ visibility: 'default', is_private: false, members: [], groups: [], permissions: [] }) });
      },
    },
  };
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  sandbox.__apiCalls = apiCalls;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'permissions.js' });
  return { sandbox, document, body, apiCalls };
}

test('the permissions widget registers itself', () => {
  const { sandbox } = boot();
  assert.ok(sandbox.Chronicle._impls.permissions, 'registered');
  assert.equal(typeof sandbox.Chronicle._impls.permissions.init, 'function');
  assert.equal(typeof sandbox.Chronicle._impls.permissions.destroy, 'function');
});

test('inline layout builds an in-flow panel, not a body-attached card', () => {
  const { sandbox, body } = boot();
  const impl = sandbox.Chronicle._impls.permissions;
  const el = makeNode('div');
  impl.init(el, { layout: 'inline', editable: true, endpoint: '/x' });

  assert.ok(el.classList.contains('perm-inline'), 'mount tagged perm-inline');
  const trigger = findByClass(el, 'perm-trigger');
  assert.ok(trigger, 'summary trigger rendered in the mount');
  const panel = findByClass(el, 'perm-inline-panel');
  assert.ok(panel, 'collapsible panel rendered in the mount (in-flow)');

  // No slide-in card / backdrop attached to <body> in inline mode.
  assert.equal(findByClass(body, 'perm-card'), null, 'no fixed slide-in card on body');
  assert.equal(findByClass(body, 'perm-backdrop'), null, 'no backdrop on body');
});

test('inline trigger expands and collapses the panel in place', () => {
  const { sandbox } = boot();
  const impl = sandbox.Chronicle._impls.permissions;
  const el = makeNode('div');
  impl.init(el, { layout: 'inline', editable: true, endpoint: '/x' });
  const trigger = findByClass(el, 'perm-trigger');
  const panel = findByClass(el, 'perm-inline-panel');

  assert.equal(panel.classList.contains('perm-open'), false, 'starts collapsed');
  trigger.dispatch('click');
  assert.equal(panel.classList.contains('perm-open'), true, 'expands on click');
  assert.equal(el.classList.contains('perm-inline-open'), true, 'mount marked open');
  assert.equal(trigger.getAttribute('aria-expanded'), 'true');
  trigger.dispatch('click');
  assert.equal(panel.classList.contains('perm-open'), false, 'collapses on second click');
  assert.equal(el.classList.contains('perm-inline-open'), false);
  assert.equal(trigger.getAttribute('aria-expanded'), 'false');
});

test('inline trigger is a disclosure, not a modal dialog', () => {
  const { sandbox } = boot();
  const impl = sandbox.Chronicle._impls.permissions;
  const el = makeNode('div');
  impl.init(el, { layout: 'inline', editable: true, endpoint: '/x' });
  const trigger = findByClass(el, 'perm-trigger');
  assert.equal(trigger.getAttribute('aria-haspopup'), 'true', 'disclosure, not dialog');
});

test('default (non-inline) layout still builds the slide-in card + backdrop on body', () => {
  const { sandbox, body } = boot();
  const impl = sandbox.Chronicle._impls.permissions;
  const el = makeNode('div');
  impl.init(el, { editable: true, endpoint: '/x' }); // no layout → slide-in
  assert.equal(el.classList.contains('perm-inline'), false, 'not inline');
  assert.ok(findByClass(body, 'perm-card'), 'slide-in card on body');
  assert.ok(findByClass(body, 'perm-backdrop'), 'backdrop on body');
});

test('inline still loads via the endpoint (reuses the save/load path)', () => {
  const { sandbox, apiCalls } = boot();
  const impl = sandbox.Chronicle._impls.permissions;
  impl.init(makeNode('div'), { layout: 'inline', editable: true, endpoint: '/perms' });
  assert.equal(apiCalls.length, 1, 'load() called the endpoint');
  assert.equal(apiCalls[0].url, '/perms');
});

test('destroy cleans up inline mount state', () => {
  const { sandbox } = boot();
  const impl = sandbox.Chronicle._impls.permissions;
  const el = makeNode('div');
  impl.init(el, { layout: 'inline', editable: true, endpoint: '/x' });
  assert.ok(el._permState, 'state attached');
  impl.destroy(el);
  assert.equal(el._permState, undefined, 'state cleared on destroy');
});
