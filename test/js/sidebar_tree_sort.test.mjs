// sidebar_tree_sort.test.mjs — contract for the drill panel's folder tree
// shape: folders first, then pages, both alphabetical, and a folder's own
// page count. Nav v3 (issue #817) requires this ordering; the drag-reorder
// sort_order field only tie-breaks two same-named siblings now (see the
// module doc in sidebar_tree.js).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sidebar_tree.js'), 'utf8');

/** Loads sidebar_tree.js in a vm with just enough stubs that the IIFE runs to
 *  completion and exports sortChildren/countPages. document.getElementById
 *  returns null, so initTree() bails at load time — only the pure helpers
 *  are under test here. */
function load() {
  const noop = () => {};
  const document = {
    getElementById() { return null; },
    querySelector() { return null; },
    querySelectorAll() { return []; },
    createElement() {
      return {
        style: {}, className: '', innerHTML: '', textContent: '',
        setAttribute: noop, getAttribute: () => null, hasAttribute: () => false,
        classList: { add: noop, remove: noop, contains: () => false },
        appendChild: noop, removeChild: noop, insertBefore: noop,
        addEventListener: noop, removeEventListener: noop,
        querySelector: () => null, querySelectorAll: () => [],
      };
    },
    addEventListener: noop, removeEventListener: noop, dispatchEvent: noop,
    readyState: 'complete',
    body: { classList: { add: noop, remove: noop, contains: () => false } },
    head: { appendChild: noop },
  };
  const window = { Chronicle: {}, addEventListener: noop, removeEventListener: noop };
  const module = { exports: {} };
  const sandbox = {
    document, window, module, console, setTimeout, clearTimeout, Promise,
    MutationObserver: function () { this.observe = noop; this.disconnect = noop; },
    IntersectionObserver: function () { this.observe = noop; this.disconnect = noop; },
    CustomEvent: function (t, i) { this.type = t; this.detail = (i || {}).detail || null; },
    DOMParser: function () { this.parseFromString = () => ({ getElementById: () => null }); },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return module.exports;
}

/** A fake tree node: el answers data-is-folder/data-entity-name only. */
function node(name, opts = {}) {
  const isFolder = !!opts.folder;
  const attrs = { 'data-entity-name': name };
  if (isFolder) attrs['data-is-folder'] = 'true';
  return {
    id: opts.id || name,
    isNode: isFolder,
    sortOrder: opts.sortOrder || 0,
    children: opts.children || [],
    el: {
      hasAttribute: (k) => k in attrs,
      getAttribute: (k) => (k in attrs ? attrs[k] : null),
    },
  };
}

test('sortChildren puts every folder before every page, regardless of input order', () => {
  const { sortChildren } = load();
  const nodes = [node('Zebra Page'), node('Ants Folder', { folder: true }), node('Bees Page')];
  sortChildren(nodes);
  assert.deepEqual(nodes.map((n) => n.id), ['Ants Folder', 'Bees Page', 'Zebra Page']);
});

test('sortChildren sorts each group alphabetically, case-insensitively', () => {
  const { sortChildren } = load();
  const folders = [node('Zoo'), node('apple', { folder: true }), node('Mango', { folder: true }), node('banana', { folder: true })];
  sortChildren(folders);
  assert.deepEqual(folders.map((n) => n.id), ['apple', 'banana', 'Mango', 'Zoo']);
});

test('sortChildren ignores sort_order between the two groups — a folder never loses its place to a lower-order page', () => {
  const { sortChildren } = load();
  const nodes = [node('A Page', { sortOrder: 0 }), node('B Folder', { folder: true, sortOrder: 99 })];
  sortChildren(nodes);
  assert.deepEqual(nodes.map((n) => n.id), ['B Folder', 'A Page']);
});

test('sortChildren falls back to sort_order only between two same-named siblings', () => {
  const { sortChildren } = load();
  const nodes = [node('Same', { id: 'later', sortOrder: 2 }), node('Same', { id: 'earlier', sortOrder: 1 })];
  sortChildren(nodes);
  assert.deepEqual(nodes.map((n) => n.id), ['earlier', 'later']);
});

test('countPages counts every page in a folder\'s subtree, at any depth, and never the folders themselves', () => {
  const { countPages } = load();
  const leaf1 = node('p1');
  const leaf2 = node('p2');
  const inner = node('inner', { folder: true, children: [leaf1, leaf2] });
  const leaf3 = node('p3');
  const outer = node('outer', { folder: true, children: [inner, leaf3] });
  assert.equal(countPages(outer), 3);
  assert.equal(countPages(inner), 2);
});

test('countPages of an empty folder is 0', () => {
  const { countPages } = load();
  assert.equal(countPages(node('empty', { folder: true, children: [] })), 0);
});

test('countPages of a single page (no folder wrapper) is 1', () => {
  const { countPages } = load();
  assert.equal(countPages(node('lone page')), 1);
});
