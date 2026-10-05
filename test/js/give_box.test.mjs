// give_box.test.mjs — the Give box's module loads on its own and exposes the
// calls the server-rendered boxes make (each control's inline handler names
// one), and the "How many" stepper never leaves 1..1,000,000.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

function load() {
  const window = { Chronicle: {} };
  vm.runInNewContext(readFileSync(join(root, 'static/js/widgets/give_box.js'), 'utf8'),
    { window, matchMedia: () => ({ matches: true }), document: { querySelector: () => null } });
  return window.Chronicle.GiveBox;
}

test('every call a box control makes exists', () => {
  const G = load();
  // The methods giveBoxCall, giveOpenOnClick and collectionMenuOnClick name.
  for (const m of ['open', 'card', 'tab', 'pick', 'search', 'step', 'cancel', 'discard', 'give', 'tick', 'save']) {
    assert.equal(typeof G[m], 'function', m);
  }
});

test('the stepper stays between 1 and 1,000,000', () => {
  const { clampQty } = load()._internal;
  for (const [v, want] of [['3', 3], ['0', 1], ['-4', 1], ['', 1], ['abc', 1], ['2.7', 2], ['5000000', 1000000]]) {
    assert.equal(clampQty(v), want, String(v));
  }
});
