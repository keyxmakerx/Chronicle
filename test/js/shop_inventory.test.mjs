// shop_inventory.test.mjs — the shop inventory widget against the wire shapes
// its endpoints actually return: relations carry camelCase `targetEntityId`
// (internal/widgets/relations/model.go), entity pages route by ID, and entity
// search also returns maps/timelines/events that cannot be stocked.
//
// Dependency-free mini-DOM in a vm sandbox, like permissions_inline.test.mjs.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'widgets', 'shop_inventory.js');

function makeNode(tag) {
  const classes = new Set();
  const node = {
    tagName: (tag || 'div').toUpperCase(),
    children: [],
    dataset: {},
    style: {},
    _attrs: {},
    textContent: '',
    get className() { return Array.from(classes).join(' '); },
    set className(v) { classes.clear(); String(v).split(/\s+/).forEach((c) => c && classes.add(c)); },
    classList: { contains: (c) => classes.has(c) },
    setAttribute(k, v) { this._attrs[k] = String(v); },
    appendChild(c) { this.children.push(c); return c; },
    removeChild(c) { const i = this.children.indexOf(c); if (i >= 0) this.children.splice(i, 1); return c; },
    set innerHTML(_v) { this.children = []; },
    get innerHTML() { return ''; },
    querySelector(sel) { return findByClass(this, sel.replace(/^\./, '')); },
    focus() {},
  };
  return node;
}

function findByClass(root, cls) {
  for (const c of root.children || []) {
    if (c.classList && c.classList.contains(cls)) return c;
    const deep = findByClass(c, cls);
    if (deep) return deep;
  }
  return null;
}

function findAllByClass(root, cls, out = []) {
  for (const c of root.children || []) {
    if (c.classList && c.classList.contains(cls)) out.push(c);
    findAllByClass(c, cls, out);
  }
  return out;
}

function json(body) {
  return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) });
}

// boot mounts the widget with apiFetch routed by URL to `routes`.
function boot(routes, editable = true) {
  const notices = [];
  const sandbox = {
    console: { error() {}, log() {}, warn() {} },
    document: { createElement: makeNode },
    setTimeout: (fn) => { fn(); return 0; },
    clearTimeout() {},
    Chronicle: {
      _impls: {},
      register(name, impl) { this._impls[name] = impl; },
      escapeHtml: (s) => String(s == null ? '' : s),
      notify: (msg, type) => notices.push({ msg, type }),
      apiFetch: (url) => {
        for (const [prefix, body] of routes) {
          if (url.startsWith(prefix)) return json(body);
        }
        return Promise.resolve({ ok: false, status: 404, json: () => Promise.resolve({}) });
      },
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox);
  const el = makeNode('div');
  el.dataset = {
    relationsEndpoint: '/campaigns/c1/entities/shop1/relations',
    entitySearchEndpoint: '/campaigns/c1/entities/search',
    quickCreateEndpoint: '/campaigns/c1/entities/quick-create',
    campaignUrl: '/campaigns/c1',
    editable: editable ? 'true' : 'false',
  };
  sandbox.Chronicle._impls.shop_inventory.init(el);
  return { el, notices };
}

const flush = () => new Promise((r) => setImmediate(r));

const sellsRelation = {
  id: 7,
  sourceEntityId: 'shop1',
  targetEntityId: 'item-abc',
  targetEntityName: 'Longsword',
  targetEntitySlug: 'longsword',
  targetEntityType: 'Item',
  relationType: 'sells',
  metadata: { price: 15, quantity: 2, in_stock: true },
};

test('a stocked entity item renders as a linked entity, not a custom item', async () => {
  const { el } = boot([['/campaigns/c1/entities/shop1/relations', [sellsRelation]]], false);
  await flush(); await flush();
  const link = findByClass(el, 'shop-inv-item-name');
  assert.ok(link, 'entity item must render as a link');
  assert.equal(link.textContent, 'Longsword');
  assert.equal(link.href, '/campaigns/c1/entities/item-abc', 'entity pages route by ID, not slug');
  assert.equal(findByClass(el, 'shop-inv-item-name-plain'), null, 'must not fall back to "Unnamed item"');
});

test('only "sells" relations are listed', async () => {
  const other = { ...sellsRelation, id: 8, targetEntityId: 'npc1', relationType: 'owned by' };
  const { el } = boot([['/campaigns/c1/entities/shop1/relations', [sellsRelation, other]]], false);
  await flush(); await flush();
  assert.equal(findAllByClass(el, 'shop-inv-item').length, 1);
});

test('search offers only entity pages that are not already stocked', async () => {
  const { el } = boot([
    ['/campaigns/c1/entities/shop1/relations', [sellsRelation]],
    ['/campaigns/c1/entities/search', { results: [
      { id: 'item-abc', name: 'Longsword', url: '/campaigns/c1/entities/item-abc' },
      { id: 'item-def', name: 'Shield', url: '/campaigns/c1/entities/item-def' },
      { id: 'map1', name: 'Shield Map', url: '/campaigns/c1/maps/map1' },
    ], total: 3 }],
  ]);
  await flush(); await flush();
  findByClass(el, 'shop-inv-add-btn').onclick();
  const input = findByClass(el, 'shop-inv-search');
  input.value = 'Shi';
  input.oninput();
  await flush(); await flush();
  const rows = findAllByClass(el, 'shop-inv-search-item');
  assert.deepEqual(rows.map((r) => r.dataset.entityId), ['item-def']);
});
