// where_it_is.test.mjs — pins the "Where it is" block's pure parts: how much
// motion applies, how the enlarged map grows from its card (and fades under
// Calm, and not at all under Off), where the peek sits, the arrows' limits, and
// the Director's note and the facts under the enlarged map.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'maps', 'static', 'js', 'where_it_is.js'), 'utf8');
const sandbox = { module: { exports: {} } };
vm.runInNewContext(src, sandbox);
const W = sandbox.module.exports;

test('motionOf: reduced motion is always off; otherwise the page attribute decides', () => {
  const cases = [
    { attr: null, reduced: false, want: 'full' },
    { attr: 'calm', reduced: false, want: 'calm' },
    { attr: 'off', reduced: false, want: 'off' },
    { attr: 'calm', reduced: true, want: 'off' },
    { attr: null, reduced: true, want: 'off' },
    { attr: 'bogus', reduced: false, want: 'full' }
  ];
  for (const c of cases) assert.equal(W.motionOf(c.attr, c.reduced), c.want, JSON.stringify(c));
});

test('growKeyframes: full grows from the card, calm fades, off plays nothing', () => {
  const from = { left: 100, top: 50, width: 120, height: 84 };
  const to = { left: 0, top: 0, width: 600, height: 300 };
  const full = W.growKeyframes('full', from, to, false);
  assert.equal(full.length, 2);
  assert.match(full[0].transform, /^translate\(100px,50px\) scale\(0\.2,0\.28\)$/);
  assert.equal(full[1].transform, 'none');
  const back = W.growKeyframes('full', from, to, true);
  assert.equal(back[0].transform, 'none');
  assert.match(back[1].transform, /scale\(0\.2,0\.28\)/);
  assert.deepEqual(JSON.parse(JSON.stringify(W.growKeyframes('calm', from, to, false))), [{ opacity: 0 }, { opacity: 1 }]);
  assert.equal(W.growKeyframes('off', from, to, false), null);
  // A sheet with no size cannot be scaled from a card: it fades.
  assert.deepEqual(JSON.parse(JSON.stringify(W.growKeyframes('full', from, { left: 0, top: 0, width: 0, height: 0 }, false))), [{ opacity: 0 }, { opacity: 1 }]);
});

test('peekLeft: centred under the card, kept inside the block', () => {
  const cases = [
    { name: 'centred', args: [200, 100, 0, 800, 250], want: 125 },
    { name: 'clamped left', args: [0, 100, 0, 800, 250], want: 0 },
    { name: 'clamped right', args: [760, 100, 0, 800, 250], want: 550 },
    { name: 'block offset on the page', args: [500, 100, 300, 800, 250], want: 125 }
  ];
  for (const c of cases) assert.equal(W.peekLeft(...c.args), c.want, c.name);
});

test('stepIndex: arrows stop at either end of the ladder', () => {
  const cases = [
    { args: [0, 4, -1], want: -1 },
    { args: [0, 4, 1], want: 1 },
    { args: [3, 4, 1], want: -1 },
    { args: [3, 4, -1], want: 2 },
    { args: [0, 1, 1], want: -1 }
  ];
  for (const c of cases) assert.equal(W.stepIndex(...c.args), c.want, JSON.stringify(c.args));
});

test('escapeShouldClose: Escape only, no modifier', () => {
  assert.equal(W.escapeShouldClose({ key: 'Escape' }), true);
  assert.equal(W.escapeShouldClose({ key: 'Escape', ctrlKey: true }), false);
  assert.equal(W.escapeShouldClose({ key: 'Enter' }), false);
  assert.equal(W.escapeShouldClose(null), false);
});

test('panelRows and dmLine read the card, never the markup', () => {
  const first = { here: true, child: '', next: 'Dockside' };
  assert.deepEqual(JSON.parse(JSON.stringify(W.panelRows(first, 'The Gilded Flagon', true))), [['This place', 'The Gilded Flagon is here'], ['Zooms out to', 'Dockside']]);
  const top = { here: false, child: 'Port Varrow', next: '' };
  assert.deepEqual(JSON.parse(JSON.stringify(W.panelRows(top, 'x', false))), [['Pin on this map', 'Port Varrow'], ['Reach', 'Widest map you can see']]);
  assert.deepEqual(JSON.parse(JSON.stringify(W.panelRows(top, 'x', true))), [['Pin on this map', 'Port Varrow'], ['Reach', 'Widest map in this ladder']]);
  assert.equal(W.dmLine(first), 'Set in Maps: this map sits under the Dockside pin.');
  assert.equal(W.dmLine(top), 'Set in Maps: no pin leads up from this map.');
});

test('a step whose next map is DM-only names it only in the Director\'s note', () => {
  const d = { here: false, child: 'A', next: '', dmNext: 'The Shattered Coast' };
  assert.deepEqual(JSON.parse(JSON.stringify(W.panelRows(d, 'x', true))), [['Pin on this map', 'A'], ['Reach', 'Widest map players can see']]);
  assert.deepEqual(JSON.parse(JSON.stringify(W.panelRows(d, 'x', false))), [['Pin on this map', 'A'], ['Reach', 'Widest map you can see']]);
  assert.equal(W.dmLine(d), 'Set in Maps: this map sits under the The Shattered Coast pin.');
});
