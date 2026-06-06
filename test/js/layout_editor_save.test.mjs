// layout_editor_save.test.mjs — regression for the dead Save button
// (C-CAL layout-save fix). The shared layout editor (campaign dashboard,
// category dashboard, entity page template) silently no-op'd Save when its
// layout was null (fresh/empty surface) — the button fired NO write request.
// This locks that a Save click ALWAYS issues a PUT to the editor's endpoint.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'layout_editor.js'), 'utf8');

// Run the browser IIFE in a vm with minimal DOM + Chronicle stubs; capture the
// registered widget definition + every apiFetch call.
function loadWidget() {
  let widgetDef = null;
  const calls = [];
  const stubEl = () => ({
    style: {}, dataset: {}, className: '',
    setAttribute() {}, appendChild() {}, removeEventListener() {}, addEventListener() {},
    classList: { add() {}, remove() {}, toggle() {} }, querySelector() { return null; },
  });
  const doc = { getElementById: () => null, createElement: stubEl, head: { appendChild() {} }, addEventListener() {} };
  const Chronicle = {
    register: (name, def) => { widgetDef = def; },
    apiFetch: (url, opts) => { calls.push({ url, opts: opts || {} }); return Promise.resolve({ ok: true, json: () => Promise.resolve({}) }); },
    notify: () => {},
  };
  const sandbox = { Chronicle, document: doc, window: {}, console, setTimeout, clearTimeout };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { widgetDef, calls };
}

function newInstance(widgetDef, over) {
  const inst = Object.create(widgetDef);
  inst.endpoint = '/campaigns/c1/dashboard-layout';
  inst.context = 'dashboard';
  inst.layout = null;
  inst.dirty = true;
  inst._findSaveBtn = () => null;     // button lookup is DOM-bound; not under test
  inst._findSaveStatus = () => null;
  return Object.assign(inst, over || {});
}

test('Save issues a PUT even when the layout is null (the dead-button bug)', async () => {
  const { widgetDef, calls } = loadWidget();
  assert.ok(widgetDef && typeof widgetDef.save === 'function', 'layout-editor registered with save()');

  const inst = newInstance(widgetDef, { layout: null });
  widgetDef.save.call(inst);
  await new Promise((r) => setTimeout(r, 0));

  const puts = calls.filter((c) => c.opts.method === 'PUT');
  assert.equal(puts.length, 1, 'exactly one PUT issued for a fresh/empty layout');
  assert.equal(puts[0].url, '/campaigns/c1/dashboard-layout', 'PUT hits the editor endpoint');
});

test('Save PUTs the populated layout (dashboard context = bare body)', async () => {
  const { widgetDef, calls } = loadWidget();
  const layout = { rows: [{ id: 'r1', columns: [{ id: 'c1', width: 12, blocks: [{ id: 'b1', type: 'entity_calendar', config: {} }] }] }] };
  const inst = newInstance(widgetDef, { layout });
  widgetDef.save.call(inst);
  await new Promise((r) => setTimeout(r, 0));

  const puts = calls.filter((c) => c.opts.method === 'PUT');
  assert.equal(puts.length, 1);
  // Dashboard context sends the bare layout (template context wraps in {layout}).
  assert.ok(puts[0].opts.body && Array.isArray(puts[0].opts.body.rows), 'body is the bare layout');
  assert.equal(puts[0].opts.body.rows[0].columns[0].blocks[0].type, 'entity_calendar', 'the block round-trips into the PUT');
});
