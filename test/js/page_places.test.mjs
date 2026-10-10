// page_places.test.mjs -- contracts for listing one page in more than one
// place in the page tree: the picker's filter (page_places.js) and the tree's
// Alt-drop rules (sidebar_tree.js).
//
// Rules pinned here:
//   - the picker never offers the page itself, its real home, a place it is
//     already listed in, or a result without an id, and never offers a page twice;
//   - an Alt-drop adds a place only onto a page, never onto itself; a drop
//     without Alt is not an add-a-place request (a plain drag still moves);
//   - extra listings take no index in the sibling order the server re-sequences.

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

test('placeRequest: only Alt, only onto a page, never onto itself', () => {
  const { mod } = loadScript('sidebar_tree.js');
  const cases = [
    { name: 'alt onto another page', args: ['dragged', 'target', true], want: { entityId: 'dragged', parentId: 'target' } },
    { name: 'plain drop is a move, not a place', args: ['dragged', 'target', false], want: null },
    { name: 'alt onto itself', args: ['same', 'same', true], want: null },
    { name: 'alt onto a folder (no page id)', args: ['dragged', null, true], want: null },
    { name: 'alt with nothing dragged', args: ['', 'target', true], want: null },
    { name: 'alt with the text null dragged (a folder row)', args: ['null', 'target', true], want: null },
    { name: 'alt with undefined dragged', args: ['undefined', 'target', true], want: null },
  ];
  for (const c of cases) {
    assert.deepEqual(plain(mod.placeRequest(...c.args)), c.want, c.name);
  }
});

test('placeTargetId reads a page row or a listing row, and no folder', () => {
  const { mod } = loadScript('sidebar_tree.js');
  const el = (attrs) => ({ getAttribute: (k) => (k in attrs ? attrs[k] : null) });
  assert.equal(mod.placeTargetId(el({ 'data-entity-id': 'p1' })), 'p1');
  assert.equal(mod.placeTargetId(el({ 'data-place-entity': 'p2' })), 'p2');
  assert.equal(mod.placeTargetId(el({ 'data-node-id': 'folder' })), null);
});

test('listings take no index among the siblings the server orders', () => {
  const { mod } = loadScript('sidebar_tree.js');
  const parentNode = { querySelectorAll: () => nodes };
  const mk = (id, place) => ({
    parentNode,
    hasAttribute: (k) => place && k === 'data-place-key',
    getAttribute: (k) => (k === 'data-entity-id' && !place ? id : null),
  });
  // a, [listing], b, [listing], c : the server only knows a, b, c.
  const nodes = [mk('a'), mk('x', true), mk('b'), mk('y', true), mk('c')];
  assert.equal(mod.calculateTargetIndex(nodes[4], 'before'), 2, 'before c is index 2, not 4');
  assert.equal(mod.calculateTargetIndex(nodes[0], 'append'), 3, 'append counts real siblings only');
  const container = { querySelector: () => ({ querySelectorAll: () => nodes }) };
  assert.equal(mod.folderChildCount(container), 3);
});

test('addPlace posts the parent and refreshes; a refusal is shown, not swallowed', async () => {
  const calls = [];
  const notes = [];
  const apiFetch = (url, opts) => {
    calls.push({ url, opts });
    return Promise.resolve(calls.length === 1
      ? { ok: true }
      : { ok: false, json: () => Promise.resolve({ message: 'a page can’t be listed under itself' }) });
  };
  const { mod } = loadScript('sidebar_tree.js', {
    Chronicle: { apiFetch, notify: (m, l) => notes.push({ m, l }) },
  });
  mod.addPlace('camp-1', 'e-1', 'p-1');
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(calls[0].url, '/campaigns/camp-1/entities/e-1/places');
  assert.equal(calls[0].opts.method, 'POST');
  assert.deepEqual(plain(calls[0].opts.body), { parent_id: 'p-1' });
  assert.equal(notes[0].l, 'success');

  mod.addPlace('camp-1', 'e-1', 'e-1');
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(notes[1].l, 'error');
  assert.match(notes[1].m, /listed under itself/);
});

test('countPages does not count listing rows', () => {
  const { mod } = loadScript('sidebar_tree.js');
  const page = { isNode: false, children: [] };
  const listing = { isNode: false, isPlace: true, children: [] };
  assert.equal(mod.countPages(listing), 0);
  assert.equal(mod.countPages(page), 1);
});
