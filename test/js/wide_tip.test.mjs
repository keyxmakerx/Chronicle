// wide_tip.test.mjs — pins when the very-wide-screen tip shows. Runs the real
// script in a vm sandbox with no document, so only its pure rule loads.
//
// The rule: only on a 4K-class width at normal zoom (zooming in narrows the
// page and hides it), never once answered, never inside a page beside another.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/wide_tip.js'), 'utf8');

function load() {
  const window = {};
  vm.runInNewContext(src, { window });
  return window.Chronicle.WideTip;
}

const cases = [
  { name: '4K at normal zoom', w: 3840, seen: false, pane: false, want: true },
  { name: '4K zoomed to 200%', w: 1920, seen: false, pane: false, want: false },
  { name: '2560 screen', w: 2560, seen: false, pane: false, want: false },
  { name: 'already answered', w: 3840, seen: true, pane: false, want: false },
  { name: 'inside a page beside another', w: 3840, seen: false, pane: true, want: false },
];

for (const c of cases) {
  test(`wideTip: ${c.name}`, () => {
    assert.equal(load().wideTip(c.w, c.seen, c.pane), c.want);
  });
}
