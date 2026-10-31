// page_places.test.mjs -- contracts for listing one page in more than one
// place in the page tree: the picker's filter (page_places.js) and how the
// tree counts the listings (sidebar_tree.js).
//
// Rules pinned here:
//   - the picker never offers the page itself, its real home, a place it is
//     already listed in, or a result without an id, and never offers a page twice;
//   - a listing is not counted as a page in its folder's count.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = (rel) => readFileSync(path.join(here, '..', '..', 'static', 'js', rel), 'utf8');

const noopEl = {
  style: {}, className: '', innerHTML: '', textContent: '',
  setAttribute() {}, getAttribute() { return null; }, hasAttribute() { return false; },
  classList: { add() {}, remove() {}, contains() { return false; } },
  appendChild() {}, addEventListener() {}, removeEventListener() {},
  querySelector() { return null; }, querySelectorAll() { return []; },
};

function loadScript(rel, extra = {}) {
  const document = {
    getElementById() { return null; }, querySelector() { return null; }, querySelectorAll() { return []; },
    createElement() { return Object.assign({}, noopEl); },
    addEventListener() {}, removeEventListener() {}, dispatchEvent() {},
    readyState: 'complete', body: { classList: { add() {}, remove() {}, contains() { return false; } } },
    head: { appendChild() {} },
  };
  const Chronicle = Object.assign({ apiFetch: () => Promise.resolve({ ok: true }), notify() {} }, extra.Chronicle || {});
  const window = { Chronicle, addEventListener() {}, removeEventListener() {} };
  const module = { exports: {} };
  const sandbox = {
    Chronicle, document, window, module, console, setTimeout, clearTimeout, Promise,
    matchMedia: () => ({ matches: false }),
    MutationObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
    IntersectionObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
    CustomEvent: function (t, i) { this.type = t; this.detail = (i || {}).detail || null; },
    DOMParser: function () { this.parseFromString = () => ({ getElementById: () => null }); },
  };
  vm.createContext(sandbox);
  vm.runInContext(read(rel), sandbox);
  return { mod: module.exports, window };
}

// Values built inside the vm carry its realm's prototypes; strict deepEqual
// would call them different from identical host values.
function plain(v) {
  return v === undefined ? v : JSON.parse(JSON.stringify(v));
}

test('pickable leaves out the page, its home, listed places, id-less and repeated results', () => {
  const { mod } = loadScript('widgets/page_places.js');
  const results = [
    { id: 'self', name: 'This page' },
    { id: 'home', name: 'Real parent' },
    { id: 'listed', name: 'Already here' },
    { id: 'ok-1', name: 'Port City', type_name: 'Location' },
    { name: 'No id' },
    { id: 'ok-1', name: 'Port City again' },
    { id: 'ok-2' },
  ];
  const got = mod.pickable(results, 'self', 'home', ['listed']);
  assert.deepEqual(plain(got.map((r) => r.id)), ['ok-1', 'ok-2']);
  assert.equal(got[0].type, 'Location');
  assert.equal(got[1].name, 'Untitled', 'a nameless result still gets a label');
});

test('pickable copes with no real parent, no listings and no results', () => {
  const { mod } = loadScript('widgets/page_places.js');
  assert.deepEqual(plain(mod.pickable(undefined, 'self', '', undefined)), []);
  assert.deepEqual(plain(mod.pickable([{ id: 'a', name: 'A' }], 'self', '', []).map((r) => r.id)), ['a']);
});

test('countPages does not count listing rows', () => {
  const { mod } = loadScript('sidebar_tree.js');
  const page = { isNode: false, children: [] };
  const listing = { isNode: false, isPlace: true, children: [] };
  assert.equal(mod.countPages(listing), 0);
  assert.equal(mod.countPages(page), 1);
});
