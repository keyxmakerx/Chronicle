// map_focus.test.mjs — pins the pure helpers behind the map focus view: the
// unfold geometry (sideways first, then downward), when Esc may close it, and
// Tab wrap-around. The script runs as the browser loads it, minus a window, so
// it exports its helpers.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'maps', 'static', 'js', 'map_focus.js'), 'utf8');

function load() {
  const saved = { window: globalThis.window, module: globalThis.module };
  delete globalThis.window;
  globalThis.module = { exports: {} };
  vm.runInThisContext(src);
  const api = globalThis.module.exports;
  globalThis.module = saved.module;
  if (saved.window !== undefined) globalThis.window = saved.window;
  return api;
}
const F = load();

test('flipKeyframes starts at the preview rect and ends flat', () => {
  const from = { left: 700, top: 120, width: 320, height: 200 };
  const to = { left: 18, top: 18, width: 1000, height: 700 };
  const kf = F.flipKeyframes(from, to);
  assert.equal(kf.length, 4);
  assert.equal(kf[0].transform, 'translate(682px,102px) scale(0.32,0.2857142857142857)');
  assert.equal(kf[3].transform, 'none');
});

test('flipKeyframes opens sideways first: x is full width at 45% while y is still small', () => {
  const from = { left: 100, top: 100, width: 200, height: 100 };
  const to = { left: 0, top: 0, width: 800, height: 600 };
  const mid = F.flipKeyframes(from, to)[1];
  assert.equal(mid.offset, 0.45);
  assert.match(mid.transform, /scale\(1,0\.1666/);
  // The vertical offset has not started to close yet at the sideways stop.
  assert.match(mid.transform, /translate\(50px,100px\)/);
});

test('flipKeyframes: offsets rise and the last stop settles', () => {
  const kf = F.flipKeyframes({ left: 0, top: 0, width: 10, height: 10 }, { left: 0, top: 0, width: 100, height: 100 });
  const offs = kf.map((k) => k.offset).filter((o) => o !== undefined);
  assert.deepEqual(offs, [...offs].sort((a, b) => a - b));
  assert.equal(kf[kf.length - 1].transform, 'none');
});

test('flipKeyframes refuses a degenerate target', () => {
  assert.equal(F.flipKeyframes({ left: 0, top: 0, width: 1, height: 1 }, { left: 0, top: 0, width: 0, height: 0 }), null);
  assert.equal(F.flipKeyframes(null, { left: 0, top: 0, width: 1, height: 1 }), null);
});

test('the unfold lasts 620ms', () => {
  assert.equal(F.DURATION, 620);
});

test('escapeShouldClose', () => {
  const cases = [
    ['plain Escape closes', { key: 'Escape' }, false, true],
    ['other keys never close', { key: 'Enter' }, false, false],
    ['viewer used the Esc itself', { key: 'Escape', mpConsumed: true }, false, false],
    ['already handled elsewhere', { key: 'Escape', defaultPrevented: true }, false, false],
    ['modifier held', { key: 'Escape', ctrlKey: true }, false, false],
    ['marker editor open owns Esc', { key: 'Escape' }, true, false],
    ['no event', null, false, false],
  ];
  for (const [name, ev, modal, want] of cases) {
    assert.equal(F.escapeShouldClose(ev, modal), want, name);
  }
});

test('nextFocusIndex wraps both ways and handles an empty dialog', () => {
  const cases = [
    [3, 2, false, 0],
    [3, 0, true, 2],
    [3, 1, false, 2],
    [3, 1, true, 0],
    [3, -1, false, 0],
    [3, -1, true, 2],
    [0, -1, false, -1],
  ];
  for (const [n, cur, back, want] of cases) {
    assert.equal(F.nextFocusIndex(n, cur, back), want, `n=${n} cur=${cur} back=${back}`);
  }
});
