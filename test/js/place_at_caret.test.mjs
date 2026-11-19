// place_at_caret.test.mjs — Chronicle.placeAtCaret (static/js/theme.js) keeps
// the editor's "/" menu, Insert Template list and mention list inside the
// window: clamped 8px from the sides, under the caret when a full list fits
// and above it when it does not. The window is the visual viewport, so a
// phone's keyboard counts.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'theme.js'), 'utf8');

function load({ width = 390, height = 844, offsetTop = 0, offsetLeft = 0 } = {}) {
  const html = {
    classList: { add() {}, remove() {}, contains: () => false, toggle() {} },
    hasAttribute: () => false, getAttribute: () => null, setAttribute() {}, removeAttribute() {},
  };
  const sandbox = {
    document: { documentElement: html, querySelectorAll: () => [], addEventListener() {} },
    localStorage: { getItem: () => null, setItem() {} },
    matchMedia: () => ({ matches: false }),
    getComputedStyle: () => ({ maxHeight: '300px' }),
    visualViewport: { width, height, offsetTop, offsetLeft },
    innerWidth: width, innerHeight: height,
    Chronicle: {},
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'theme.js' });
  return sandbox.Chronicle;
}

// A menu whose height is its content or its max-height, whichever is less.
function menu(w, content = 300) {
  const m = { style: {}, offsetWidth: w };
  Object.defineProperty(m, 'offsetHeight', {
    get: () => Math.min(content, parseFloat(m.style.maxHeight) || content),
  });
  return m;
}

test('right edge: a menu at the caret near the right is pulled back 8px from the side', () => {
  const C = load({ width: 390 });
  const m = menu(320);
  C.placeAtCaret(m, { left: 300, top: 100, bottom: 120 });
  assert.equal(m.style.left, '62px'); // 390 - 320 - 8
  assert.equal(m.style.top, '124px');
});

test('left edge: never closer than 8px', () => {
  const C = load({ width: 390 });
  const m = menu(240);
  C.placeAtCaret(m, { left: 2, top: 100, bottom: 120 });
  assert.equal(m.style.left, '8px');
});

test('flips above the caret when there is no room below', () => {
  const C = load({ width: 390, height: 844 });
  const m = menu(260, 300);
  // Caret low on the screen: 80px below it, 700px above it.
  C.placeAtCaret(m, { left: 20, top: 740, bottom: 760 });
  assert.equal(m.style.maxHeight, '300px');
  assert.equal(m.style.top, String(740 - 4 - 300) + 'px');
});

test('the keyboard shrinks the window: the menu goes above a caret that sits over it', () => {
  const C = load({ width: 390, height: 400 }); // keyboard takes the lower half
  const m = menu(260, 300);
  C.placeAtCaret(m, { left: 20, top: 330, bottom: 350 });
  assert.equal(m.style.top, String(330 - 4 - 300) + 'px');
});

test('stays below when a full menu fits there', () => {
  const C = load({ width: 390, height: 844 });
  const m = menu(260, 300);
  C.placeAtCaret(m, { left: 20, top: 100, bottom: 120 });
  assert.equal(m.style.top, '124px');
  assert.equal(m.style.maxHeight, '300px');
});

test('with little room either side the taller side wins and the menu shrinks to it', () => {
  const C = load({ width: 390, height: 300 });
  const m = menu(260, 300);
  C.placeAtCaret(m, { left: 20, top: 200, bottom: 220 });
  // below: 300-8-224 = 68 (under the 96 floor); above: 200-4-8 = 188.
  assert.equal(m.style.maxHeight, '188px');
  assert.equal(m.style.top, String(200 - 4 - 188) + 'px');
});
