// page_blocks_helpers.test.mjs: the pure helpers of the Read-aloud, Table and
// Steps widget. The widget file is loaded in a bare vm context with a stubbed
// Chronicle, the way the server ships it, and its exported helpers are driven
// directly; the DOM is not needed.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'pageblocks', 'static', 'js', 'page_blocks.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { H: sandbox.window.PageBlocksHelpers, registered };
}

const plain = (o) => JSON.parse(JSON.stringify(o));

test('the widget registers once under its mount name', () => {
  const { registered } = load();
  assert.deepEqual(Object.keys(registered), ['page_blocks']);
  assert.equal(typeof registered.page_blocks.init, 'function');
  assert.equal(typeof registered.page_blocks.destroy, 'function');
});

test('emptyData and isEmpty agree for every kind', () => {
  const { H } = load();
  for (const kind of ['read_aloud', 'page_table', 'page_steps']) {
    assert.equal(H.isEmpty(kind, H.normalise(kind, H.emptyData(kind))), true, kind);
  }
  assert.equal(H.isEmpty('read_aloud', { text: '  \n ' }), true);
  assert.equal(H.isEmpty('read_aloud', { text: 'Hi' }), false);
  assert.equal(H.isEmpty('page_table', { columns: [{ id: 'c' }], rows: [] }), false);
});

test('normalise survives null and missing lists', () => {
  const { H } = load();
  assert.deepEqual(plain(H.normalise('page_table', null)), { columns: [], rows: [] });
  assert.deepEqual(plain(H.normalise('page_steps', {})), { steps: [] });
  assert.deepEqual(plain(H.normalise('read_aloud', { text: 5 })), { text: '' });
  assert.deepEqual(plain(H.normalise('page_table', { columns: [{ id: 'c' }], rows: [{ id: 'r' }] })),
    { columns: [{ id: 'c', label: '', dmOnly: false }], rows: [{ id: 'r', cells: {} }] });
});

test('move shifts one item and stops at the edges', () => {
  const { H } = load();
  assert.deepEqual(H.move([1, 2, 3], 1, -1), [2, 1, 3]);
  assert.deepEqual(H.move([1, 2, 3], 1, 1), [1, 3, 2]);
  assert.deepEqual(H.move([1, 2, 3], 0, -1), [1, 2, 3]);
  assert.deepEqual(H.move([1, 2, 3], 2, 1), [1, 2, 3]);
  const input = [1, 2, 3];
  H.move(input, 0, 1);
  assert.deepEqual(input, [1, 2, 3], 'move must not change its input');
});

test('table helpers respect the limits and drop a removed column\'s cells', () => {
  const { H } = load();
  let t = { columns: [], rows: [] };
  for (let i = 0; i < 12; i++) t = H.addColumn(t);
  assert.equal(t.columns.length, H.LIM.columns);
  assert.equal(new Set(t.columns.map((c) => c.id)).size, H.LIM.columns, 'column ids are unique');
  for (let i = 0; i < 70; i++) t = H.addRow(t);
  assert.equal(t.rows.length, H.LIM.rows);

  const [a, b] = t.columns;
  t.rows[0].cells[a.id] = 'x';
  t.rows[0].cells[b.id] = 'y';
  const n = H.removeColumn(t, a.id);
  assert.equal(n.columns.length, H.LIM.columns - 1);
  assert.deepEqual(plain(n.rows[0].cells), { [b.id]: 'y' });
  assert.equal(t.columns.length, H.LIM.columns, 'removeColumn must not change its input');
});

test('ids the editor makes are safe tokens the server accepts', () => {
  const { H } = load();
  for (const p of ['c', 'r', 's']) {
    assert.match(H.uid(p), /^[A-Za-z0-9_-]{1,64}$/);
  }
});

test('step helpers tick, clear and cap', () => {
  const { H } = load();
  let d = { steps: [] };
  for (let i = 0; i < 40; i++) d = H.addStep(d);
  assert.equal(d.steps.length, H.LIM.steps);
  const id = d.steps[3].id;
  d = H.setDone(d, id, true);
  assert.equal(H.anyDone(d), true);
  assert.equal(d.steps.filter((s) => s.done).length, 1);
  d = H.clearTicks(d);
  assert.equal(H.anyDone(d), false);
});

// ---- Rendering smoke test over a minimal fake DOM ----

class FakeEl {
  constructor(tag) {
    this.tag = tag; this.children = []; this.attrs = {}; this.handlers = {};
    this._text = ''; this.className = '';
    const self = this;
    this.classList = {
      add: (c) => { self.className = (self.className + ' ' + c).trim(); },
      toggle() {}
    };
  }
  set textContent(v) { this._text = String(v); this.children = []; }
  get textContent() { return this._text + this.children.map((c) => c.textContent).join(''); }
  setAttribute(k, v) { this.attrs[k] = v; }
  appendChild(c) { this.children.push(c); return c; }
  addEventListener(k, fn) { this.handlers[k] = fn; }
  querySelector() { return null; }
  find(pred, out = []) { if (pred(this)) out.push(this); this.children.forEach((c) => c.find(pred, out)); return out; }
}

async function mount(kind, view, status = 200) {
  const registered = {};
  const sandbox = {
    window: {},
    document: { createElement: (t) => new FakeEl(t) },
    Chronicle: {
      register: (n, i) => { registered[n] = i; },
      apiFetch: async () => ({ ok: status < 300, status, text: async () => JSON.stringify(view) })
    },
    setTimeout, console
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const el = new FakeEl('div');
  registered.page_blocks.init(el, { endpoint: '/x', kind, csrfToken: 't' });
  await new Promise((r) => setTimeout(r, 10));
  return el;
}

test('user text is placed as text and a DM-only header carries the DM tag', async () => {
  const evil = '<img src=x onerror=alert(1)>';
  const el = await mount('page_table', {
    kind: 'page_table', version: 1, canEdit: false,
    data: { columns: [{ id: 'a', label: 'Name', dmOnly: false }, { id: 'b', label: 'Secret', dmOnly: true }], rows: [{ id: 'r', cells: { a: evil, b: 'q' } }] }
  });
  assert.equal(el.find((n) => n.tag === 'table').length, 1);
  assert.ok(el.find((n) => n.tag === 'td' && n._text === evil).length === 1, 'cell text kept verbatim as text');
  assert.equal(el.find((n) => (n.className || '').split(' ').includes('pb-dm')).length, 1);
  assert.equal(el.find((n) => n.tag === 'button').length, 0, 'a viewer who cannot edit gets no controls');
});

test('a block with nothing to show draws nothing for a viewer who cannot edit', async () => {
  const el = await mount('read_aloud', { kind: 'read_aloud', version: 0, canEdit: false, data: { text: '' } });
  assert.equal(el.children.length, 0);
});

test('an editor sees a prompt, and steps get tick buttons', async () => {
  const empty = await mount('page_steps', { kind: 'page_steps', version: 0, canEdit: true, data: { steps: [] } });
  assert.match(empty.textContent, /Add the beats of this scene/);

  const el = await mount('page_steps', {
    kind: 'page_steps', version: 2, canEdit: true,
    data: { steps: [{ id: 's1', title: 'Enter', text: '', done: true }, { id: 's2', title: 'Fight', text: '', done: false }] }
  });
  const nums = el.find((n) => n.tag === 'button' && n.className === 'pb-num');
  assert.equal(nums.length, 2);
  assert.equal(nums[0].attrs['aria-pressed'], 'true');
  assert.equal(nums[1].attrs['aria-pressed'], 'false');
  assert.ok(el.find((n) => n.tag === 'button' && n._text === 'Clear ticks').length === 1);
});

test('a failed load leaves the block out rather than throwing', async () => {
  const el = await mount('read_aloud', { message: 'no' }, 404);
  assert.equal(el.children.length, 0);
});
