// page_sides.test.mjs — pins where a side column sticks. Runs the real
// script in a vm sandbox with just enough of a page for it to load.
//
// The rule that matters: a side column taller than the view must never stick
// with its bottom out of reach, so it sticks only once its bottom shows.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/page_sides.js'), 'utf8');

function load() {
  const document = {
    readyState: 'complete',
    addEventListener() {},
    querySelectorAll: () => [],
    getElementById: () => null,
  };
  const window = { addEventListener() {}, requestAnimationFrame() {}, innerHeight: 900 };
  vm.runInNewContext(src, { window, document });
  return window.Chronicle.PageSides;
}

const cases = [
  { name: 'short column sticks near the top', colH: 300, viewH: 900, top: 16 },
  { name: 'column that just fits sticks near the top', colH: 868, viewH: 900, top: 16 },
  { name: 'tall column sticks once its bottom shows', colH: 1400, viewH: 900, top: -516 },
];
for (const c of cases) {
  test(c.name, () => {
    assert.equal(load().stickTop(c.colH, c.viewH), c.top);
  });
}
