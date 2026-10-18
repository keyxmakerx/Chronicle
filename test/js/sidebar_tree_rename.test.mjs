// sidebar_tree_rename.test.mjs — inline folder rename sends one save on Enter
// and none on Escape, even though removing the input fires blur.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sidebar_tree.js'), 'utf8');

function load(fetchCalls) {
  const el = () => ({
    style: {}, className: '', textContent: '', type: '', value: '',
    setAttribute() {}, getAttribute() { return null; },
    classList: { add() {}, remove() {}, contains() { return false; } },
    appendChild() {}, addEventListener() {}, removeEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
  });
  const document = {
    getElementById() { return null; }, querySelector() { return null; }, querySelectorAll() { return []; },
    createElement() { return el(); }, addEventListener() {}, removeEventListener() {}, dispatchEvent() {},
    readyState: 'complete',
    head: { appendChild() {} },
    body: { classList: { add() {}, remove() {}, contains() { return false; } } },
  };
  const Chronicle = {
    apiFetch: (url, opts) => { fetchCalls.push({ url, opts }); return Promise.resolve({ ok: true }); },
    notify() {},
  };
  const module = { exports: {} };
  const sandbox = {
    Chronicle, document, window: { Chronicle, addEventListener() {} }, module, console, Promise,
    setTimeout: () => 0, clearTimeout() {},
    MutationObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
    IntersectionObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
    CustomEvent: function (t) { this.type = t; },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return module.exports;
}

// A name span whose input fires blur when it is removed, as browsers do for a
// focused element.
function renameRow() {
  let input = null;
  const handlers = {};
  const span = {
    get textContent() { return input ? '' : this._text; },
    set textContent(v) {
      this._text = v;
      if (input) { const i = input; input = null; (i._h.blur || []).forEach((fn) => fn()); }
    },
    appendChild(i) {
      input = i;
      i._h = handlers;
      i.addEventListener = (ev, fn) => { (handlers[ev] = handlers[ev] || []).push(fn); };
      i.focus = () => {}; i.select = () => {}; i.scrollIntoView = () => {};
    },
  };
  span._text = 'Old name';
  const link = { querySelector: () => span, setAttribute() {} };
  return {
    link,
    type: (v) => { input.value = v; },
    key: (k) => (handlers.keydown || []).forEach((fn) => fn({ key: k, preventDefault() {} })),
  };
}

test('Escape cancels without saving', async () => {
  const calls = [];
  const mod = load(calls);
  const row = renameRow();
  mod.startInlineRename(row.link, 'n1', 'c1');
  row.type('Typo name');
  row.key('Escape');
  await new Promise((r) => setImmediate(r));
  assert.equal(calls.length, 0);
});

test('Enter saves exactly once', async () => {
  const calls = [];
  const mod = load(calls);
  const row = renameRow();
  mod.startInlineRename(row.link, 'n1', 'c1');
  row.type('New name');
  row.key('Enter');
  await new Promise((r) => setImmediate(r));
  assert.equal(calls.length, 1);
  assert.equal(calls[0].opts.body.name, 'New name');
});
