// sidebar_reorg_contract.test.mjs — contracts for sidebar_reorg.js PUT payload
//
// Regression guard for the clobber bug: sidebar_reorg.js must never write
// legacy fields (entity_type_order / hidden_type_ids) via saveSidebarConfig.
// It must only send {items, hidden_entity_ids, hidden_node_ids} so the
// server-side load-merge-write can preserve fields set by other writers
// (e.g. sidebar_layout_editor.js).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sidebar_reorg.js'), 'utf8');

/**
 * Boot sidebar_reorg.js in a vm with enough stubs to reach saveSidebarConfig.
 * After boot, activateCategoryReorg is triggered by simulating a reorg-toggle
 * click, which fetches (and resolves) the sidebar config. Once the config is
 * loaded, the caller can fire visibility toggle events and inspect PUT calls.
 */
async function boot() {
  const fetchCalls = [];

  // Multi-listener event registry: eventType → [fn, ...]
  const docHandlers = {};
  const winHandlers = {};
  const btnHandlers = {};

  function addHandler(map, event, fn) {
    if (!map[event]) map[event] = [];
    map[event].push(fn);
  }
  function removeHandler(map, event, fn) {
    if (!map[event]) return;
    map[event] = map[event].filter((f) => f !== fn);
  }
  function fire(map, event, arg) {
    (map[event] || []).forEach((fn) => fn(arg));
  }

  const catListAttrs = {};
  const catListEl = {
    getAttribute: (k) => catListAttrs[k] || null,
    setAttribute: (k, v) => { catListAttrs[k] = v; },
    removeAttribute: (k) => { delete catListAttrs[k]; },
    querySelectorAll: () => ({ forEach: () => {} }),
  };

  const reorgBtn = {
    getAttribute: (k) => (k === 'data-campaign-id' ? 'camp-test' : null),
    addEventListener: (ev, fn) => addHandler(btnHandlers, ev, fn),
    removeEventListener: () => {},
    querySelector: () => null,
    classList: { add() {}, remove() {} },
    title: '',
  };

  const document = {
    getElementById: (id) => {
      if (id === 'sidebar-reorg-toggle') return reorgBtn;
      if (id === 'sidebar-cat-list') return catListEl;
      return null;
    },
    querySelector: () => null,
    querySelectorAll: () => ({ forEach: () => {} }),
    createElement: () => ({
      style: {}, className: '', innerHTML: '', type: '',
      setAttribute() {}, getAttribute() { return null; },
      classList: { add() {}, remove() {}, contains() { return false; } },
      appendChild() {}, removeChild() {}, insertBefore() {},
      addEventListener() {}, removeEventListener() {},
      querySelector() { return null; },
    }),
    addEventListener: (ev, fn) => addHandler(docHandlers, ev, fn),
    removeEventListener: (ev, fn) => removeHandler(docHandlers, ev, fn),
    dispatchEvent: () => {},
    readyState: 'complete',
    body: { classList: { add() {}, remove() {} } },
    head: { appendChild() {} },
  };

  const Chronicle = {
    apiFetch: (url, opts) => {
      const call = { url, opts: opts || {} };
      fetchCalls.push(call);
      if (!opts || !opts.method || opts.method === 'GET') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ items: [], hidden_entity_ids: [], hidden_node_ids: [] }),
        });
      }
      return Promise.resolve({ ok: true });
    },
    notify: () => {},
  };

  const window = { addEventListener: (ev, fn) => addHandler(winHandlers, ev, fn) };
  const MutationObserver = function () { this.observe = () => {}; };
  function CustomEvent(type, init) { this.type = type; this.detail = (init || {}).detail || null; }

  const sandbox = { Chronicle, document, window, MutationObserver, CustomEvent, console, setTimeout, clearTimeout, Promise };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  // init() runs immediately (readyState === 'complete').

  // Simulate clicking the reorg toggle to activate category reorg mode.
  // This triggers activateCategoryReorg(), which fetches config (async).
  const fakeClick = { preventDefault() {}, stopPropagation() {} };
  fire(btnHandlers, 'click', fakeClick);

  // Yield to let the config fetch promise resolve and sidebarConfig be populated.
  await new Promise((r) => setTimeout(r, 0));

  return {
    fetchCalls,
    fireDocEvent: (type, detail) => fire(docHandlers, type, { detail }),
  };
}

test('saveSidebarConfig does NOT send entity_type_order or hidden_type_ids', async () => {
  const { fetchCalls, fireDocEvent } = await boot();

  // Trigger a save via entity visibility toggle (guaranteed to call saveSidebarConfig).
  fireDocEvent('chronicle:toggle-entity-visibility', { entityId: 'e-abc' });
  await new Promise((r) => setTimeout(r, 0));

  const puts = fetchCalls.filter((c) => c.opts.method === 'PUT');
  assert.ok(puts.length > 0, 'at least one PUT issued after toggle');

  puts.forEach((put) => {
    const body = put.opts.body || {};
    assert.ok(!('entity_type_order' in body),
      'PUT body must not contain entity_type_order (clobbers unified model)');
    assert.ok(!('hidden_type_ids' in body),
      'PUT body must not contain hidden_type_ids (clobbers unified model)');
    assert.ok(!('custom_sections' in body),
      'PUT body must not contain custom_sections');
    assert.ok(!('custom_links' in body),
      'PUT body must not contain custom_links');
  });
});

test('saveSidebarConfig sends exactly items, hidden_entity_ids, hidden_node_ids', async () => {
  const { fetchCalls, fireDocEvent } = await boot();

  fireDocEvent('chronicle:toggle-entity-visibility', { entityId: 'e-xyz' });
  await new Promise((r) => setTimeout(r, 0));

  const puts = fetchCalls.filter((c) => c.opts.method === 'PUT');
  assert.ok(puts.length > 0, 'at least one PUT issued');

  const body = puts[0].opts.body || {};
  assert.ok('items' in body, 'PUT body must include items');
  assert.ok('hidden_entity_ids' in body, 'PUT body must include hidden_entity_ids');
  assert.ok('hidden_node_ids' in body, 'PUT body must include hidden_node_ids');
});
