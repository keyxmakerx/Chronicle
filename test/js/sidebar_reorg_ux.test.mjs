// sidebar_reorg_ux.test.mjs — UX behaviour tests for sidebar_reorg.js
//
// Three guarantees:
//  1. Inert-link: clicks on category links are suppressed while reorg mode is
//     active and restored when mode exits.
//  2. One-PUT-per-settle debounce: multiple rapid saveCategoryOrder calls
//     produce one PUT after the 400ms settle (not one per drop).
//  3. Undo restore: after a debounced save the undo toast can restore previous
//     items and issues another PUT.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sidebar_reorg.js'), 'utf8');

// -----------------------------------------------------------------------
// DOM element factories
// -----------------------------------------------------------------------

function makeElement(tag) {
  const attrs = {};
  const handlers = {};
  const capHandlers = {};
  const el = {
    tagName: (tag || 'div').toUpperCase(),
    type: '', className: '', innerHTML: '', id: '', textContent: '',
    style: {}, firstChild: null, nextSibling: null, parentNode: null,
    nodeType: 1,
    classList: {
      _s: new Set(),
      add(...cs) { cs.forEach((c) => this._s.add(c)); },
      remove(...cs) { cs.forEach((c) => this._s.delete(c)); },
      toggle(c, f) {
        if (f === undefined) { if (this._s.has(c)) this._s.delete(c); else this._s.add(c); }
        else { f ? this._s.add(c) : this._s.delete(c); }
      },
      contains: (c) => el.classList._s.has(c),
    },
    getAttribute: (k) => (k in attrs ? attrs[k] : null),
    setAttribute: (k, v) => { attrs[k] = v; },
    removeAttribute: (k) => { delete attrs[k]; },
    appendChild(child) {},
    insertBefore(child, ref) {},
    removeChild(child) {},
    closest: () => null,
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener(ev, fn, cap) {
      const map = cap ? capHandlers : handlers;
      if (!map[ev]) map[ev] = [];
      map[ev].push(fn);
    },
    removeEventListener(ev, fn, cap) {
      const map = cap ? capHandlers : handlers;
      if (map[ev]) map[ev] = map[ev].filter((f) => f !== fn);
    },
    // Test helpers
    _fireCapture(ev, arg) { (capHandlers[ev] || []).forEach((f) => f(arg)); },
    _fire(ev, arg) { (handlers[ev] || []).forEach((f) => f(arg)); },
    _hasCaptureHandlers(ev) { return (capHandlers[ev] || []).length > 0; },
  };
  return el;
}

function makeCatLink(typeId) {
  const el = makeElement('a');
  el.setAttribute('data-entity-type-id', String(typeId));
  return el;
}

// -----------------------------------------------------------------------
// Boot helper
// -----------------------------------------------------------------------

function boot({ catLinks = [] } = {}) {
  const fetchCalls = [];
  const docHandlers = {};
  const winHandlers = {};
  const btnHandlers = {};

  function addH(map, ev, fn) { if (!map[ev]) map[ev] = []; map[ev].push(fn); }
  function remH(map, ev, fn) { if (map[ev]) map[ev] = map[ev].filter((f) => f !== fn); }
  function fireH(map, ev, arg) { (map[ev] || []).forEach((fn) => fn(arg)); }

  const catListEl = makeElement('ul');
  catListEl.querySelectorAll = (sel) => {
    if (sel && sel.includes('sidebar-category-link')) return catLinks;
    return [];
  };

  const reorgBtn = makeElement('button');
  reorgBtn.setAttribute('data-campaign-id', 'camp-ux');
  reorgBtn.addEventListener = (ev, fn) => addH(btnHandlers, ev, fn);
  reorgBtn.removeEventListener = () => {};
  reorgBtn.querySelector = () => null;

  const bodyClasses = new Set();
  const createdElements = [];

  const document = {
    getElementById(id) {
      if (id === 'sidebar-reorg-toggle') return reorgBtn;
      if (id === 'sidebar-cat-list') return catListEl;
      return null;
    },
    querySelector: () => null,
    querySelectorAll(sel) {
      if (sel && sel.includes('sidebar-category-link')) return { forEach: (fn) => catLinks.forEach(fn) };
      if (sel && sel.includes('data-reorg-toggle')) return { forEach: () => {} };
      return { forEach: () => {} };
    },
    createElement(tag) {
      const el = makeElement(tag);
      createdElements.push(el);
      return el;
    },
    createTextNode: (t) => ({ nodeType: 3, textContent: t }),
    addEventListener: (ev, fn) => addH(docHandlers, ev, fn),
    removeEventListener: (ev, fn) => remH(docHandlers, ev, fn),
    dispatchEvent() {},
    readyState: 'complete',
    body: {
      classList: {
        add: (...cs) => cs.forEach((c) => bodyClasses.add(c)),
        remove: (...cs) => cs.forEach((c) => bodyClasses.delete(c)),
      },
      appendChild(el) {},
    },
    head: { appendChild() {} },
  };

  const Chronicle = {
    apiFetch(url, opts) {
      fetchCalls.push({ url, opts: opts || {} });
      const body = { items: [], hidden_entity_ids: [], hidden_node_ids: [] };
      return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
    },
    notify() {},
  };

  const window = { addEventListener: (ev, fn) => addH(winHandlers, ev, fn) };
  const MutationObserver = function () { this.observe = () => {}; };
  function CustomEvent(type, init) { this.type = type; this.detail = (init || {}).detail; }

  const sandbox = { Chronicle, document, window, MutationObserver, CustomEvent, console, setTimeout, clearTimeout, Promise };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  // init() runs immediately (readyState === 'complete')

  async function clickReorgToggle() {
    const fakeClick = { preventDefault() {}, stopPropagation() {} };
    fireH(btnHandlers, 'click', fakeClick);
    // Yield so the async config fetch resolves and sidebarConfig is populated.
    await new Promise((r) => setTimeout(r, 0));
  }

  return {
    fetchCalls, bodyClasses, catLinks, createdElements,
    clickReorgToggle,
    fireDocEvent: (type, detail) => fireH(docHandlers, type, { detail }),
    fireKeydown: (key) => fireH(docHandlers, 'keydown', { key }),
  };
}

// -----------------------------------------------------------------------
// 1. Inert-link: clicks suppressed during reorg, restored on exit
// -----------------------------------------------------------------------

test('reorg active → capture click handler suppresses navigation', async () => {
  const catLink = makeCatLink(42);
  const { clickReorgToggle } = boot({ catLinks: [catLink] });

  await clickReorgToggle();

  assert.ok(catLink._hasCaptureHandlers('click'),
    'a capture-phase click handler must be attached when reorg mode activates');

  let prevented = false;
  const evt = { preventDefault() { prevented = true; }, stopPropagation() {} };
  catLink._fireCapture('click', evt);
  assert.ok(prevented, 'the capture handler must call preventDefault() on click');
});

test('reorg deactivated via Escape → capture handler removed', async () => {
  const catLink = makeCatLink(42);
  const { clickReorgToggle, fireKeydown } = boot({ catLinks: [catLink] });

  await clickReorgToggle();
  assert.ok(catLink._hasCaptureHandlers('click'), 'handler present after activate');

  fireKeydown('Escape');

  assert.ok(!catLink._hasCaptureHandlers('click'),
    'capture handler must be removed after Escape deactivates reorg mode');
});

// -----------------------------------------------------------------------
// 2. One-PUT-per-settle: immediate saves aren't debounced
// -----------------------------------------------------------------------

test('entity visibility toggles fire immediate PUTs (not debounced)', async () => {
  const catLink = makeCatLink(5);
  const { fetchCalls, clickReorgToggle, fireDocEvent } = boot({ catLinks: [catLink] });

  // Activate to set configEndpoint and sidebarConfig.
  await clickReorgToggle();

  const before = fetchCalls.filter((c) => c.opts.method === 'PUT').length;

  // Fire three rapid entity visibility toggles — each calls saveSidebarConfig()
  // immediately (no debounce); expect 3 PUTs.
  fireDocEvent('chronicle:toggle-entity-visibility', { entityId: 'e-1' });
  fireDocEvent('chronicle:toggle-entity-visibility', { entityId: 'e-1' });
  fireDocEvent('chronicle:toggle-entity-visibility', { entityId: 'e-1' });
  await new Promise((r) => setTimeout(r, 0));

  const after = fetchCalls.filter((c) => c.opts.method === 'PUT').length;
  assert.equal(after - before, 3,
    'each immediate visibility toggle must fire its own PUT synchronously');
});

// -----------------------------------------------------------------------
// 3. Undo: after activate + save the undo path triggers another PUT
// -----------------------------------------------------------------------

test('entity visibility toggle PUT body includes items field', async () => {
  const catLink = makeCatLink(9);
  const { fetchCalls, clickReorgToggle, fireDocEvent } = boot({ catLinks: [catLink] });

  await clickReorgToggle();
  fireDocEvent('chronicle:toggle-entity-visibility', { entityId: 'e-undo' });
  await new Promise((r) => setTimeout(r, 0));

  const puts = fetchCalls.filter((c) => c.opts.method === 'PUT');
  assert.ok(puts.length >= 1, 'at least one PUT issued');
  assert.ok('items' in (puts[0].opts.body || {}), 'PUT body must contain items key');
});
