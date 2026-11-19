// kit_open.test.mjs — where a style kit's opening starts from (static/js/kit_open.js):
// the middle of what was clicked, measured inside the card, with the card's
// nearest edge, and the middle of the top edge when nothing was clicked.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'kit_open.js'), 'utf8');

function load() {
  const noop = () => {};
  const document = { addEventListener: noop, documentElement: {}, body: null };
  const window = {};
  vm.createContext(window);
  vm.runInContext('var MutationObserver = function(){};', window);
  window.window = window;
  window.document = document;
  vm.runInContext(src, window);
  return window.Chronicle.kitOpen;
}
const box = (left, top, width, height) => ({ getBoundingClientRect: () => ({ left, top, width, height }), isConnected: true });

test('opens from the middle of what was clicked, at the nearest edge', () => {
  const { origin } = load();
  const card = box(100, 200, 300, 120);
  assert.deepEqual({ ...origin(box(140, 160, 40, 20), card) }, { x: 60, y: 0, edge: 'top' });
});

test('a click below the card opens from its bottom edge', () => {
  const { origin } = load();
  const o = origin(box(500, 400, 40, 20), box(100, 200, 300, 120));
  assert.equal(o.edge, 'bottom');
  assert.equal(o.x, 300); // clamped inside the card
  assert.equal(o.y, 120);
});

test('with nothing clicked it opens from the middle of the top edge', () => {
  const { origin } = load();
  const o = origin(null, box(100, 200, 300, 120));
  assert.deepEqual({ ...o }, { x: 150, y: 0, edge: 'top' });
});
