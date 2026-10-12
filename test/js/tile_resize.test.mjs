// tile_resize.test.mjs — the chronicle:tile-resize contract widget authors rely
// on (docs/build/widget.md): the `tile` mount on a tile cell fires it when
// data-tile-w/-h change, and when the screen crosses the narrow breakpoint,
// with the columns the tile spans right now.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'tile.js'), 'utf8');

function cell(w, h) {
  const attrs = { 'data-tile-w': String(w), 'data-tile-h': String(h) };
  return {
    events: [],
    getAttribute: (k) => (k in attrs ? attrs[k] : null),
    set(k, v) { attrs[k] = String(v); },
    dispatchEvent(e) { this.events.push(e); return true; },
  };
}

/** Loads the widget against a fake Chronicle, window, observer and media query. */
function load({ narrow = false } = {}) {
  const state = { narrow, widget: null, observers: [], listeners: [] };
  class CustomEvent {
    constructor(type, init) { this.type = type; this.bubbles = init.bubbles; this.detail = init.detail; }
  }
  class MutationObserver {
    constructor(cb) { this.cb = cb; this.opts = null; this.connected = false; state.observers.push(this); }
    observe(_el, opts) { this.opts = opts; this.connected = true; }
    disconnect() { this.connected = false; }
  }
  const window = {
    matchMedia: () => ({
      get matches() { return state.narrow; },
      addEventListener: (_t, fn) => { state.listeners.push(fn); },
      removeEventListener: (_t, fn) => { state.listeners = state.listeners.filter((f) => f !== fn); },
    }),
  };
  const Chronicle = { register: (name, impl) => { state.widget = { name, impl }; } };
  const sandbox = { Chronicle, window, CustomEvent, MutationObserver, WeakMap, parseInt };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return state;
}

test('registers as the "tile" mount the cell carries', () => {
  const s = load();
  assert.equal(s.widget.name, 'tile');
});

test('watches only the two size attributes, with old values', () => {
  const s = load();
  s.widget.impl.init(cell(2, 2));
  const o = s.observers[0];
  assert.deepEqual([...o.opts.attributeFilter], ['data-tile-w', 'data-tile-h']);
  assert.equal(o.opts.attributeOldValue, true);
});

test('a size change announces the new size and the previous one', () => {
  const s = load();
  const el = cell(2, 2);
  s.widget.impl.init(el);
  el.set('data-tile-w', 4);
  s.observers[0].cb([{ attributeName: 'data-tile-w', oldValue: '2' }]);
  assert.equal(el.events.length, 1);
  const e = el.events[0];
  assert.equal(e.type, 'chronicle:tile-resize');
  assert.equal(e.bubbles, true);
  assert.equal(e.detail.w, 4);
  assert.equal(e.detail.h, 2);
  assert.equal(e.detail.columns, 4);
  assert.equal(e.detail.narrow, false);
  assert.equal(e.detail.previous.w, 2);
  assert.equal(e.detail.previous.h, 2);
});

test('width and height changing together fire once with both old values', () => {
  const s = load();
  const el = cell(1, 1);
  s.widget.impl.init(el);
  el.set('data-tile-w', 2);
  el.set('data-tile-h', 2);
  s.observers[0].cb([
    { attributeName: 'data-tile-w', oldValue: '1' },
    { attributeName: 'data-tile-h', oldValue: '1' },
  ]);
  assert.equal(el.events.length, 1);
  assert.equal(el.events[0].detail.previous.w, 1);
  assert.equal(el.events[0].detail.previous.h, 1);
});

test('on the narrow grid a wide tile reports the 2 columns it spans', () => {
  const s = load({ narrow: true });
  const el = cell(4, 1);
  s.widget.impl.init(el);
  el.set('data-tile-h', 2);
  s.observers[0].cb([{ attributeName: 'data-tile-h', oldValue: '1' }]);
  const d = el.events[0].detail;
  assert.equal(d.w, 4);
  assert.equal(d.columns, 2);
  assert.equal(d.narrow, true);
});

test('crossing the breakpoint announces only a tile that changes span', () => {
  const s = load();
  const wide = cell(4, 1);
  const small = cell(1, 1);
  s.widget.impl.init(wide);
  s.widget.impl.init(small);
  s.narrow = true;
  s.listeners.forEach((fn) => fn());
  assert.equal(wide.events.length, 1);
  assert.equal(wide.events[0].detail.columns, 2);
  assert.equal(wide.events[0].detail.previous, null);
  assert.equal(small.events.length, 0);
});

test('a cell without a valid size announces nothing', () => {
  const s = load();
  const el = cell(2, 2);
  s.widget.impl.init(el);
  el.set('data-tile-w', 'wide');
  s.observers[0].cb([{ attributeName: 'data-tile-w', oldValue: '2' }]);
  assert.equal(el.events.length, 0);
});

test('destroy stops watching', () => {
  const s = load();
  const el = cell(2, 2);
  s.widget.impl.init(el);
  s.widget.impl.destroy(el);
  assert.equal(s.observers[0].connected, false);
  assert.equal(s.listeners.length, 0);
});
