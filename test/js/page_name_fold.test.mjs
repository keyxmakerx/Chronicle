// page_name_fold.test.mjs — the Edit name fold's module loads on its own,
// exposes the calls the server-rendered fold makes, and sends only what
// changed (the metadata endpoint is a partial update).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

function load() {
  const window = { Chronicle: {} };
  vm.runInNewContext(readFileSync(join(root, 'static/js/widgets/page_name_fold.js'), 'utf8'),
    { window, matchMedia: () => ({ matches: true }), document: { querySelector: () => null } });
  return window.Chronicle.PageHeader;
}

// The module runs in its own realm; compare plain copies.
const plain = (o) => JSON.parse(JSON.stringify(o));

// A box with just enough DOM for the form reads.
function fakeBox({ saved, form, structure = true }) {
  const attrs = { 'data-structure': String(structure), ...saved };
  const inputs = {
    '[data-f="name"]': { value: form.name },
    '[data-f="label"]': form.label === undefined ? null : { value: form.label },
    'input[name="page-parent"]:checked': form.parent === undefined ? null : { value: form.parent },
  };
  return {
    getAttribute: (k) => (k in attrs ? attrs[k] : null),
    querySelector: (sel) => inputs[sel] || null,
  };
}

test('every call the fold controls make exists', () => {
  const H = load();
  for (const m of ['nameFold', 'nameClose', 'nameDiscard', 'nameInput', 'nameSave', 'parentSearch']) {
    assert.equal(typeof H[m], 'function', m);
  }
});

test('only changed keys are sent, and cleared ones are null', () => {
  const { changes } = load()._fold;
  const saved = { 'data-name': 'Bren', 'data-label': 'Hero', 'data-parent-id': 'p1' };
  const cases = [
    ['nothing changed', { name: 'Bren', label: 'Hero', parent: 'p1' }, {}],
    ['rename only', { name: 'Bran', label: 'Hero', parent: 'p1' }, { name: 'Bran' }],
    ['descriptor cleared', { name: 'Bren', label: '', parent: 'p1' }, { type_label: null }],
    ['parent cleared', { name: 'Bren', label: 'Hero', parent: '' }, { parent_id: null }],
    ['parent moved', { name: 'Bren', label: 'Hero', parent: 'p2' }, { parent_id: 'p2' }],
    ['name is trimmed', { name: '  Bren  ', label: 'Hero', parent: 'p1' }, {}],
  ];
  for (const [label, form, want] of cases) {
    assert.deepEqual(plain(changes(fakeBox({ saved, form }))), want, label);
  }
});

test('a name-only fold never sends the descriptor or parent', () => {
  const { changes } = load()._fold;
  const box = fakeBox({
    saved: { 'data-name': 'Bren', 'data-label': 'Hero', 'data-parent-id': 'p1' },
    form: { name: 'Bran' }, structure: false,
  });
  assert.deepEqual(plain(changes(box)), { name: "Bran" });
});
