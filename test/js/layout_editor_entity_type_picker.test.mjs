// layout_editor_entity_type_picker.test.mjs — pins the `entity_type` config
// field of the layout editor's block config dialog (C-SWEEP-R3).
//
// The regression: the picker GET'd `/campaigns/:id/entity-types`, which is the
// entity-type MANAGEMENT PAGE. middleware.Render hard-sets
// "text/html; charset=utf-8" and never inspects Accept, so both branches of
// EntityTypesPage return HTML; `r.json()` threw a SyntaxError and the empty
// `.catch(function () {})` swallowed it. The `entity_list` block's Entity Type
// dropdown was therefore permanently stuck on its "— Select entity type —"
// placeholder, with nothing on the console to say why.
//
// The naive repoint to the v1 JSON route is not enough either: syncapi's
// ListEntityTypes returns the `{data: [...], total: N}` ENVELOPE, and the old
// `(types || []).forEach` over an object is a silent no-op — still empty. So
// this pins the whole contract: the JSON route, both envelope shapes, and a
// visible failure.
//
// Following worldstate_widget.test.mjs, this EXECUTES the real widget source in
// a vm sandbox with a DOM shim rather than pattern-matching it, so the option
// list asserted below is the one a browser would actually render.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const widgetSrc = readFileSync(join(root, 'static/js/widgets/layout_editor.js'), 'utf8');
const registrySrc = readFileSync(join(root, 'internal/plugins/entities/block_registry_core.go'), 'utf8');

// The config field the server actually ships for the entity_list block, read
// out of the Go registry so renaming either side goes red rather than green.
const ENTITY_TYPE_FIELD = (() => {
  const m = registrySrc.match(/\{Key: "(\w+)", Label: "([^"]+)", Type: "entity_type"\}/);
  assert.ok(m, 'block_registry_core.go must declare a Type:"entity_type" config field');
  return { key: m[1], label: m[2], type: 'entity_type' };
})();

// ── DOM shim ────────────────────────────────────────────────────────
// Only the surface showConfigDialog touches: createElement, appendChild,
// textContent/value/className, addEventListener, remove.

function makeEl(tag) {
  const el = {
    tagName: String(tag).toUpperCase(),
    children: [],
    style: { cssText: '' },
    dataset: {},
    className: '',
    innerHTML: '',
    textContent: '',
    value: '',
    selected: false,
    appendChild(child) { this.children.push(child); return child; },
    removeChild(child) { this.children = this.children.filter((c) => c !== child); },
    remove() {},
    addEventListener() {},
    removeEventListener() {},
    setAttribute(n, v) { this.dataset[n] = v; },
    getAttribute(n) { return n in this.dataset ? this.dataset[n] : null; },
    // Depth-first walk, so a <select> can be found under panel > form > wrapper.
    querySelectorAll(sel) {
      const want = String(sel).toUpperCase();
      const out = [];
      (function walk(node) {
        node.children.forEach((c) => {
          if (c.tagName === want) out.push(c);
          walk(c);
        });
      })(el);
      return out;
    },
  };
  return el;
}

/** Boot the real widget source and return its registered implementation. */
function boot(apiFetch) {
  const document = {
    body: makeEl('div'),
    head: makeEl('head'),
    createElement: makeEl,
    getElementById: () => null,
    addEventListener() {},
    removeEventListener() {},
  };
  const registry = {};
  const sandbox = {
    console: { warn: (...a) => sandbox.__warnings.push(a), error: () => {}, log: () => {} },
    document,
    Promise, setTimeout, clearTimeout, JSON, Math, String, Array, Object, Error, isNaN, parseInt,
    __warnings: [],
    Chronicle: {
      register: (name, impl) => { registry[name] = impl; },
      escapeHtml: (s) => String(s === undefined || s === null ? '' : s),
      apiFetch,
    },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(widgetSrc, sandbox, { filename: 'layout_editor.js' });
  assert.ok(registry['layout-editor'], 'layout_editor.js must register "layout-editor"');
  return { impl: registry['layout-editor'], document, sandbox };
}

/** A fetch stub that records URLs and replies with a raw body string. */
function responder(body, opts) {
  opts = opts || {};
  const urls = [];
  const fetchFn = (url) => {
    urls.push(url);
    return Promise.resolve({
      ok: opts.ok !== false,
      status: opts.status || 200,
      // Real fetch rejects with a SyntaxError when the body is not JSON.
      json: () => new Promise((resolve) => resolve(JSON.parse(body))),
    });
  };
  return { urls, fetchFn };
}

/**
 * Open the block config dialog for an entity_list block against `body`, let
 * the fetch promise chain flush, and return the rendered <select> options.
 */
async function openPicker(body, opts) {
  const { urls, fetchFn } = responder(body, opts);
  const { impl, document } = boot(fetchFn);
  const ed = Object.create(impl);
  ed.campaignId = 'camp-1';
  ed.markDirty = () => {};
  ed.renderCanvas = () => {};

  ed.showConfigDialog(
    { type: 'entity_list', config: (opts && opts.config) || {} },
    { label: 'Entity List', icon: 'fa-list', config_fields: [ENTITY_TYPE_FIELD] },
  );

  // Flush the .then/.catch microtask chain.
  for (let i = 0; i < 10; i++) await Promise.resolve();

  const selects = document.body.children.flatMap((n) => n.querySelectorAll('select'));
  assert.equal(selects.length, 1, 'exactly one <select> should be rendered for the entity_type field');
  return { urls, options: selects[0].children, select: selects[0] };
}

// ── The route ───────────────────────────────────────────────────────

test('the picker asks a JSON route, not the HTML entity-type management page', async () => {
  const { urls } = await openPicker(JSON.stringify({ data: [], total: 0 }));
  assert.equal(urls.length, 1, 'exactly one request');
  assert.equal(urls[0], '/api/v1/campaigns/camp-1/entity-types',
    'GET /campaigns/:id/entity-types renders the management PAGE as text/html ' +
    '(middleware.Render never inspects Accept), so JSON.parse of it always throws');
});

// ── Both list shapes (envelope law) ──────────────────────────────────

test('the envelope shape ListEntityTypes actually sends populates the dropdown', async () => {
  const { options } = await openPicker(JSON.stringify({
    data: [{ id: 3, name: 'Character' }, { id: 7, name: 'Location' }],
    total: 2,
  }));
  assert.deepEqual(options.map((o) => o.textContent),
    ['— Select entity type —', 'Character', 'Location'],
    'syncapi ListEntityTypes wraps the list in {data,total}; iterating that object adds no options');
  assert.deepEqual(options.slice(1).map((o) => o.value), [3, 7]);
});

test('a bare array is accepted too', async () => {
  const { options } = await openPicker(JSON.stringify([{ id: 3, name: 'Character' }]));
  assert.deepEqual(options.map((o) => o.textContent), ['— Select entity type —', 'Character']);
});

test('the block\'s current entity_type_id is pre-selected', async () => {
  const { options } = await openPicker(
    JSON.stringify({ data: [{ id: 3, name: 'Character' }, { id: 7, name: 'Location' }], total: 2 }),
    { config: { [ENTITY_TYPE_FIELD.key]: '7' } },
  );
  assert.deepEqual(options.map((o) => o.selected), [false, false, true]);
});

// ── Visible failure ─────────────────────────────────────────────────

test('an HTML body surfaces the failure instead of an innocent-looking empty picker', async () => {
  // The exact failure mode: EntityTypesPage's 200 text/html response.
  const { options, sandbox } = await openPickerWithSandbox(
    '<!doctype html><html lang="en" class="h-full"><head><title> - Categories</title>',
  );
  assert.deepEqual(options.map((o) => o.textContent), ['Could not load entity types'],
    'the placeholder must say the list failed — an unexplained "— Select entity type —" ' +
    'reads as "this campaign has no categories"');
  assert.equal(sandbox.__warnings.length, 1, 'the swallowed error must reach the console');
});

test('a non-2xx response surfaces the same failure text', async () => {
  const { options } = await openPicker('{}', { ok: false, status: 403 });
  assert.deepEqual(options.map((o) => o.textContent), ['Could not load entity types']);
});

// openPicker variant that also hands back the sandbox, for console assertions.
async function openPickerWithSandbox(body) {
  const { urls, fetchFn } = responder(body);
  const { impl, document, sandbox } = boot(fetchFn);
  const ed = Object.create(impl);
  ed.campaignId = 'camp-1';
  ed.markDirty = () => {};
  ed.renderCanvas = () => {};
  ed.showConfigDialog(
    { type: 'entity_list', config: {} },
    { label: 'Entity List', icon: 'fa-list', config_fields: [ENTITY_TYPE_FIELD] },
  );
  for (let i = 0; i < 10; i++) await Promise.resolve();
  const selects = document.body.children.flatMap((n) => n.querySelectorAll('select'));
  assert.equal(selects.length, 1);
  return { urls, options: selects[0].children, sandbox };
}
