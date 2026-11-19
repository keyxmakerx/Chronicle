// entity_type_editor_draft.test.mjs — the field editor in draft mode
// (data-draft): no Save button; edits go into the Customizer's draft through
// Chronicle.customizeCategoryWrite, and put back by hand they leave it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'entity_type_editor.js'), 'utf8');

function el(tag) {
  const e = { tagName: tag, children: [], style: {}, _l: {}, _a: {}, className: '', value: '', textContent: '', title: '' };
  e.appendChild = (c) => { e.children.push(c); return c; };
  e.addEventListener = (t, fn) => { (e._l[t] = e._l[t] || []).push(fn); };
  e.setAttribute = (k, v) => { e._a[k] = v; };
  e.getAttribute = (k) => (k in e._a ? e._a[k] : null);
  e.querySelector = () => null;
  e.querySelectorAll = () => [];
  Object.defineProperty(e, 'innerHTML', { get: () => '', set: (v) => { if (v === '') e.children = []; } });
  return e;
}
function walk(e, fn) { fn(e); e.children.forEach((c) => walk(c, fn)); }
function fire(e, t) { (e._l[t] || []).forEach((fn) => fn.call(e)); }

function boot({ saved, draft }) {
  let widget;
  const writes = [];
  const sandbox = {
    document: { createElement: el, createTextNode: (t) => ({ textContent: t, children: [] }) },
    window: {},
    Chronicle: {
      register: (_n, w) => { widget = w; },
      gmTerm: () => 'GM',
      customizeDraft: () => draft,
      customizeCategoryWrite: (k, patch, opts) => { writes.push([k, patch, opts]); return true; },
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const root = el('div');
  root._a = { 'data-entity-type-id': '7', 'data-fields-only': 'true', 'data-draft': 'true', 'data-fields': JSON.stringify(saved) };
  widget.init(root, {});
  return { root, writes };
}

const saved = [{ key: 'age', label: 'Age', type: 'text', section: 'Basics', options: [] }];

test('draft mode has no Save button', () => {
  const { root } = boot({ saved, draft: {} });
  let buttons = [];
  walk(root, (e) => { if (e.textContent === 'Save changes') buttons.push(e); });
  assert.equal(buttons.length, 0);
});

test('an edit writes the fields into the draft; putting it back clears them', () => {
  const { root, writes } = boot({ saved, draft: {} });
  let label;
  walk(root, (e) => { if (e.tagName === 'input' && e.placeholder === 'Label') label = e; });
  label.value = 'Years';
  fire(label, 'input');
  assert.equal(writes.at(-1)[0], '7');
  assert.equal(writes.at(-1)[1].fields[0].label, 'Years');
  label.value = 'Age';
  fire(label, 'input');
  assert.equal(writes.at(-1)[1].fields, undefined);
});

test('the draft’s fields are what the editor opens with', () => {
  const draft = { categories: { items: { 7: { fields: [{ key: 'rank', label: 'Rank', type: 'text', section: 'Basics', options: [] }] } } } };
  const { root } = boot({ saved, draft });
  const labels = [];
  walk(root, (e) => { if (e.tagName === 'input' && e.placeholder === 'Label') labels.push(e.value); });
  assert.deepEqual(labels, ['Rank']);
});
