// sidebar_tree_reorder.test.mjs — contracts for sidebar_tree.js entity reorder
// (cordinator#47).
//
// Bug 1 (silent revert): the client must send a 0-based *index* among the final
//   siblings, never a midpoint sort_order. The PUT body carries that index; the
//   reparent/append paths send a real index, never a stray hardcoded 0.
// Bug 2 (nav in edit mode): a capture-phase click handler is added to each tree
//   item on reorg activate and REMOVED on deactivate (no listener leak); a row
//   click in reorg mode is preventDefault'd, while grip/eye/toggle pass through.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sidebar_tree.js'), 'utf8');

/**
 * Load sidebar_tree.js in a vm with just enough DOM/global stubs that the IIFE
 * runs to completion and exports its internal helpers via module.exports. The
 * tree container lookup returns null so initTree() bails immediately — we only
 * want the pure helpers, not a full render. `ok` controls whether the stubbed
 * apiFetch resolves successfully (false exercises the reorder error path).
 */
function load({ fetchCalls = [], notifyCalls = [], ok = true } = {}) {
  const noopEl = {
    style: {}, className: '', innerHTML: '', textContent: '', type: '',
    setAttribute() {}, getAttribute() { return null; }, hasAttribute() { return false; },
    classList: { add() {}, remove() {}, contains() { return false; } },
    appendChild() {}, removeChild() {}, insertBefore() {},
    addEventListener() {}, removeEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
  };

  const document = {
    getElementById() { return null; },
    querySelector() { return null; },
    querySelectorAll() { return []; },
    createElement() { return Object.assign({}, noopEl); },
    addEventListener() {}, removeEventListener() {}, dispatchEvent() {},
    readyState: 'complete',
    body: { classList: { add() {}, remove() {}, contains() { return false; } } },
    head: { appendChild() {} },
  };

  const Chronicle = {
    apiFetch: (url, opts) => {
      fetchCalls.push({ url, opts: opts || {} });
      return Promise.resolve({ ok, text: () => Promise.resolve(ok ? '' : 'boom') });
    },
    notify: (msg, level) => { notifyCalls.push({ msg, level }); },
  };

  const window = { Chronicle, addEventListener() {}, removeEventListener() {} };
  const module = { exports: {} };
  const sandbox = {
    Chronicle, document, window, module,
    console, setTimeout, clearTimeout, Promise,
    MutationObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
    IntersectionObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
    CustomEvent: function (t, i) { this.type = t; this.detail = (i || {}).detail || null; },
    DOMParser: function () { this.parseFromString = () => ({ getElementById: () => null }); },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { mod: module.exports, fetchCalls, notifyCalls };
}

/** Build a fake sibling list sharing one parentNode, for index math. */
function makeSiblings(ids) {
  const nodes = [];
  const parentNode = {
    querySelectorAll(sel) {
      assert.equal(sel, ':scope > .sidebar-tree-node');
      return nodes;
    },
  };
  ids.forEach((id) => nodes.push({
    parentNode,
    getAttribute: (k) => (k === 'data-entity-id' ? id : null),
  }));
  return nodes;
}

test('calculateTargetIndex returns an insertion index, never a midpoint', () => {
  const { mod } = load();
  const sibs = makeSiblings(['a', 'b', 'c', 'd']);

  assert.equal(mod.calculateTargetIndex(sibs[0], 'before'), 0, 'before first → 0');
  assert.equal(mod.calculateTargetIndex(sibs[2], 'before'), 2, 'before index 2 → 2');
  assert.equal(mod.calculateTargetIndex(sibs[0], 'after'), 4, 'append → len');
});

test('calculateTargetIndex corrects the off-by-one when dragging downward', () => {
  const { mod } = load();
  const sibs = makeSiblings(['a', 'b', 'c', 'd']);

  // Drag "a" (index 0) before "c" (index 2): after "a" is removed server-side the
  // target shifts left, so the final desired index is 1, not 2.
  assert.equal(mod.calculateTargetIndex(sibs[2], 'before', 'a'), 1);
  // Dragging upward needs no correction: "d" before "b" stays index 1.
  assert.equal(mod.calculateTargetIndex(sibs[1], 'before', 'd'), 1);
});

test('reorder PUT carries the index as sort_order (real number, no midpoint)', async () => {
  const { mod, fetchCalls } = load();

  mod.reorderEntity('camp-1', 'e-1', 'parent-1', 3, null);
  await new Promise((r) => setTimeout(r, 0));

  const put = fetchCalls.find((c) => c.opts.method === 'PUT');
  assert.ok(put, 'a PUT was issued');
  assert.match(put.url, /\/entities\/e-1\/reorder$/);
  assert.equal(put.opts.body.sort_order, 3, 'index is sent as sort_order');
  assert.equal(put.opts.body.parent_id, 'parent-1');
});

test('folderChildCount returns a real append index, not a hardcoded 0', () => {
  const { mod } = load();
  const childContainer = { querySelectorAll: () => [{}, {}] }; // two existing children
  const folder = {
    querySelector: (sel) =>
      sel === ':scope > .sidebar-tree-children' ? childContainer : null,
  };
  assert.equal(mod.folderChildCount(folder), 2);
  assert.equal(mod.folderChildCount({ querySelector: () => null }), 0); // empty folder → 0
});

test('reorder failure surfaces an error notify (no silent console.error)', async () => {
  const { mod, notifyCalls } = load({ ok: false });

  mod.reorderEntity('camp-1', 'e-9', null, 0, null);
  await new Promise((r) => setTimeout(r, 0));

  assert.ok(notifyCalls.some((n) => n.level === 'error'),
    'a failed reorder raises an error notify instead of failing silently');
});

test('updateDraggable adds a capture click handler on activate and removes it on deactivate', () => {
  const { mod } = load();

  const listeners = [];
  const item = {
    _attrs: {},
    setAttribute(k, v) { this._attrs[k] = v; },
    removeAttribute(k) { delete this._attrs[k]; },
    getAttribute(k) { return this._attrs[k] || null; },
    addEventListener(type, fn, capture) { listeners.push({ type, fn, capture, live: true }); },
    removeEventListener(type, fn, capture) {
      listeners.forEach((l) => { if (l.fn === fn && l.type === type && l.capture === capture) l.live = false; });
    },
    querySelector() { return null; },
  };
  const node = {
    getAttribute: (k) => (k === 'data-entity-id' ? 'e-1' : null),
    hasAttribute: () => false,
    // Report the grip/eye as already present so updateDraggable skips creating
    // them — this test only cares about the capture-phase click listener.
    querySelector: (sel) => {
      if (sel === '.sidebar-tree-item') return item;
      if (sel === '.reorg-drag-handle' || sel === '.reorg-entity-visibility') return { remove() {} };
      return null;
    },
  };
  const container = { querySelectorAll: () => [node] };

  // Activate: a capture-phase click listener is registered and its ref stored.
  mod.updateDraggable(container, true);
  const click = listeners.find((l) => l.type === 'click' && l.capture === true);
  assert.ok(click, 'a capture-phase click handler was added on activate');
  assert.ok(click.live, 'handler is live while reorg is active');
  assert.equal(typeof item._reorgClickInert, 'function', 'handler ref stored for exact removal');

  // A plain row click is suppressed (preventDefault + stopPropagation).
  let prevented = false, stopped = false;
  click.fn({
    target: { closest: () => null },
    preventDefault: () => { prevented = true; },
    stopPropagation: () => { stopped = true; },
  });
  assert.ok(prevented && stopped, 'row click in reorg mode is preventDefault + stopPropagation');

  // A click on the eye toggle passes through (handler bails on controls).
  let prevented2 = false;
  click.fn({
    target: { closest: (sel) => (sel === '.reorg-entity-visibility' ? {} : null) },
    preventDefault: () => { prevented2 = true; },
    stopPropagation: () => {},
  });
  assert.ok(!prevented2, 'clicks on the eye/grip/toggle controls are not suppressed');

  // Deactivate: the exact listener is removed (no leak) and the ref cleared.
  mod.updateDraggable(container, false);
  assert.ok(!click.live, 'capture-phase click handler removed on deactivate');
  assert.equal(item._reorgClickInert, null, 'handler ref cleared on deactivate');
});
