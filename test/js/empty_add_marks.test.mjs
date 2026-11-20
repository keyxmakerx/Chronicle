// empty_add_marks.test.mjs — the tags and relations widgets mark themselves
// empty (data-tp-empty, data-rel-empty) when a page has none and nothing is
// open, which is what lets the Annals page editor show their add control only
// while editing, and word it "Add a tag" / "Add a relation" until there is one.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const load = (file) => readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', file), 'utf8');

// A just-enough element: it keeps children, attributes and the text set on it.
function fakeEl(tag = 'div') {
  const el = {
    tag, children: [], attrs: {}, style: {}, className: '', innerHTML: '', textContent: '',
    appendChild(c) { this.children.push(c); return c; },
    addEventListener() {}, dispatchEvent() {},
    setAttribute(k, v) { this.attrs[k] = String(v); },
    toggleAttribute(k, on) { if (on) this.attrs[k] = ''; else delete this.attrs[k]; },
    querySelector() { return null; },
    focus() {},
  };
  Object.defineProperty(el, 'innerHTML', {
    get() { return this._html || ''; },
    set(v) { this._html = v; if (v === '') this.children = []; },
  });
  return el;
}

function mount(file, name, config, fetched) {
  let widget;
  const sandbox = {
    document: { createElement: (t) => fakeEl(t), documentElement: { hasAttribute: () => false } },
    console, CustomEvent: class {},
  };
  sandbox.window = sandbox;
  sandbox.Chronicle = {
    register: (n, w) => { if (n === name) widget = w; },
    apiFetch: (url) => Promise.resolve({ ok: true, json: () => Promise.resolve(fetched(url)) }),
    notify() {}, escapeHtml: (s) => s,
  };
  vm.runInNewContext(load(file), sandbox);
  const el = fakeEl();
  widget.init(el, config);
  return el;
}

const flat = (el) => [el, ...el.children.flatMap(flat)];
const settle = () => new Promise((r) => setTimeout(r, 20));

test('tags: empty marks the mount and words the button "Add a tag"', async () => {
  const cases = [
    { name: 'no tags', tags: [], empty: true, word: 'Add a tag' },
    { name: 'one tag', tags: [{ id: 1, name: 'Harbour', color: '#123456' }], empty: false, word: ' Tag' },
  ];
  for (const c of cases) {
    const el = mount('tag_picker.js', 'tag-picker', { editable: true, tagsEndpoint: '/t', entityTagsEndpoint: '/e' },
      (url) => (url === '/t' ? c.tags : c.tags));
    await settle();
    assert.equal('data-tp-empty' in el.attrs, c.empty, c.name + ': empty mark');
    const btn = flat(el).find((n) => n.className === 'tp-add-btn');
    assert.ok(btn && btn.innerHTML.endsWith(c.word), c.name + ': button reads ' + (btn && btn.innerHTML));
  }
});

test('relations: empty marks the mount and words the button "Add a relation"', async () => {
  const cases = [
    { name: 'none', rels: [], empty: true, word: 'Add a relation' },
    { name: 'one', rels: [{ id: 1, relationType: 'ally', targetEntityId: 'x', targetEntityName: 'X' }], empty: false, word: 'Add relation' },
  ];
  for (const c of cases) {
    const el = mount('relations.js', 'relations', { editable: true, relationsEndpoint: '/r' }, () => c.rels);
    await settle();
    assert.equal('data-rel-empty' in el.attrs, c.empty, c.name + ': empty mark');
    const btn = flat(el).find((n) => n.className === 'rel-add-btn');
    assert.ok(btn && btn.innerHTML.endsWith(c.word), c.name + ': button reads ' + (btn && btn.innerHTML));
  }
});
