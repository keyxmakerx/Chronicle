// customize_templates.test.mjs — the Starting text tab writes into the
// Customizer's draft ("templates" section) and keeps only what differs from
// what is saved.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'customize_templates.js'), 'utf8');

const SAVED = [{ id: 5, name: 'Recap', description: 'd', icon: 'fa-scroll', content_json: '{"type":"doc"}', content_html: '<p>x</p>', is_global: false }];

function boot(draft = {}) {
  let widget;
  const sets = [];
  const clicks = [];
  const root = {
    _a: { 'data-campaign-id': 'c1', 'data-href': '/x/text' }, html: '',
    getAttribute(k) { return k in this._a ? this._a[k] : null; },
    addEventListener(t, fn) { if (t === 'click') clicks.push(fn); },
    removeEventListener() {}, contains: () => true,
    querySelector: () => null,
  };
  Object.defineProperty(root, 'innerHTML', { get() { return root.html; }, set(v) { root.html = v; } });
  const sandbox = {
    document: { addEventListener() {}, removeEventListener() {} },
    Chronicle: {
      register: (_n, w) => { widget = w; },
      escapeHtml: (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;'),
      apiFetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve(SAVED) }),
      customizeDraft: () => draft,
      customizeSet: (k, v, o) => { sets.push([k, v, o]); },
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  widget.init(root, {});
  const click = (attr, val) => clicks.forEach((fn) => fn({ target: { closest: () => ({ hasAttribute: (a) => a === attr, getAttribute: (a) => (a === attr ? val : null) }) } }));
  return { root, sets, click };
}
const tick = () => new Promise((r) => setImmediate(r));

test('lists the saved templates and marks a draft deletion', async () => {
  const { root } = boot({ templates: { items: { 5: { delete: true } } } });
  await tick();
  assert.match(root.html, /Recap/);
  assert.match(root.html, /Deleted when you save/);
});

test('delete goes into the draft with a save-bar line; keep puts it back', async () => {
  const { sets, click } = boot();
  await tick();
  click('data-del', '5');
  const v = sets.at(-1)[1];
  assert.equal(JSON.stringify(v.items), JSON.stringify({ 5: { delete: true } }));
  assert.equal(v._changes[0].label, 'Delete starting text Recap');
  assert.equal(v._changes[0].href, '/x/text');
});

test('keeping a template clears the draft section', async () => {
  const { sets, click } = boot({ templates: { items: { 5: { delete: true } } } });
  await tick();
  click('data-keep', '5');
  assert.equal(sets.at(-1)[1], null);
});
