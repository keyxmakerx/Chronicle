// calendar_open.test.mjs — pins calendar_open.js's pure helpers: the order
// the calendar's pieces assemble in, the motes' timing and paths, the touch
// peek-then-open rule, and when Escape goes back to the list. The script
// returns before touching the page when there is no document, so it runs
// here as the browser loads it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'calendar_open.js'), 'utf8');

function load() {
  globalThis.window = globalThis;
  globalThis.module = { exports: {} };
  vm.runInThisContext(src);
  const api = globalThis.module.exports;
  delete globalThis.module;
  return api;
}
const O = load();

// A seeded generator, so planned paths are exact.
function seeded(seed) {
  let s = seed >>> 0;
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 4294967296;
  };
}
const rect = (left, top, width, height) => ({ left, top, width, height });

test('moteCount: about one mote per 3000px², never fewer than 2 or more than 12', () => {
  assert.equal(O.moteCount(10, 10), 2);
  assert.equal(O.moteCount(60, 100), 2);
  assert.equal(O.moteCount(150, 100), 5);
  assert.equal(O.moteCount(900, 200), 12);
});

test('pieceDelay: the first piece leaves at once, later pieces later, all within the spread', () => {
  assert.equal(O.pieceDelay(0, 10), 0);
  let prev = -1;
  for (let i = 0; i < 10; i++) {
    const d = O.pieceDelay(i, 10);
    assert.ok(d > prev, `piece ${i} must leave after piece ${i - 1}`);
    assert.ok(d < O.T.SPREAD);
    prev = d;
  }
  assert.equal(O.pieceDelay(0, 0), 0);
});

test('readingOrder: rows top to bottom, then left to right', () => {
  // A header row whose pieces sit a few px apart vertically, then a week.
  const rects = [
    rect(300, 52, 40, 20), // h-acts, right of the header row
    rect(10, 120, 50, 60), // day 1
    rect(20, 48, 90, 30),  // hub, left of the header row
    rect(60, 121, 50, 60), // day 2 (a pixel lower than day 1)
    rect(150, 50, 80, 24), // today pill, middle of the header row
    rect(10, 10, 120, 20)  // breadcrumb, above everything
  ];
  assert.deepEqual(O.readingOrder(rects), [5, 2, 4, 0, 1, 3]);
});

test('readingOrder: a tolerance of 0 splits rows at any difference, and ties keep their order', () => {
  const rects = [rect(0, 0, 10, 10), rect(0, 1, 10, 10), rect(0, 0, 10, 10)];
  assert.deepEqual(O.readingOrder(rects, 0), [0, 2, 1]);
});

test('planMotes: every mote starts in the source and ends well inside its own piece', () => {
  const srcRect = rect(40, 30, 200, 120);
  const pieces = [rect(10, 200, 300, 40), rect(10, 260, 60, 60), rect(80, 260, 60, 60)];
  const P = O.planMotes(srcRect, pieces, seeded(7));
  assert.equal(P.length, pieces.reduce((n, r) => n + O.moteCount(r.width, r.height), 0));
  for (const p of P) {
    const r = pieces[p.ri];
    assert.ok(p.sx >= srcRect.left && p.sx <= srcRect.left + srcRect.width);
    assert.ok(p.sy >= srcRect.top && p.sy <= srcRect.top + srcRect.height);
    assert.ok(p.tx >= r.left + r.width * 0.15 && p.tx <= r.left + r.width * 0.85, 'lands inside the middle of its piece');
    assert.ok(p.ty >= r.top + r.height * 0.2 && p.ty <= r.top + r.height * 0.8);
    assert.ok(p.c === 0 || p.c === 1, 'one of the two glow colours');
  }
  // Each piece's first mote leaves exactly on its piece's turn.
  pieces.forEach((_, ri) => {
    const first = P.find((p) => p.ri === ri);
    assert.equal(first.d, O.pieceDelay(ri, pieces.length));
    for (const p of P.filter((q) => q.ri === ri)) assert.ok(p.d <= first.d + O.T.JITTER);
  });
});

test('planMotes: a month of many big days stays within the mote budget, each day still getting one', () => {
  const pieces = Array.from({ length: 100 }, (_, i) => rect((i % 10) * 100, Math.floor(i / 10) * 100, 100, 100));
  const P = O.planMotes(rect(0, 0, 10, 10), pieces, seeded(3));
  assert.ok(P.length <= O.MOTE_BUDGET, `${P.length} motes`);
  for (let ri = 0; ri < pieces.length; ri++) assert.ok(P.some((p) => p.ri === ri), `day ${ri} gets a mote`);
});

test('moteAt: waits, flies, lands on its target, then fades out and asks for no more frames', () => {
  const [p] = O.planMotes(rect(0, 0, 10, 10), [rect(500, 400, 50, 50)], seeded(11));
  const before = O.moteAt(p, p.d - 0.01);
  assert.equal(before.alpha, 0);
  assert.equal(before.alive, true, 'a waiting mote still needs frames');
  assert.equal(before.landed, false);

  const mid = O.moteAt(p, p.d + O.T.FLY / 2);
  assert.equal(mid.landed, false);
  assert.ok(mid.alpha > 0.9);

  const land = O.moteAt(p, p.d + O.T.FLY);
  assert.equal(land.landed, true);
  assert.ok(Math.abs(land.x - p.tx) < 1e-9 && Math.abs(land.y - p.ty) < 1e-9, 'lands exactly on its target');

  const gone = O.moteAt(p, p.d + O.T.FLY + O.T.FADE + 0.001);
  assert.equal(gone.alive, false);
  assert.equal(gone.alpha, 0);
});

test('timing: everything has landed in under a second and the last mote is gone soon after', () => {
  const lastLand = O.T.SPREAD + O.T.JITTER + O.T.FLY;
  assert.ok(lastLand * O.T.DUR <= 1000, `last landing at ${lastLand * O.T.DUR}ms`);
  const lastFrame = (lastLand + O.T.FADE) * O.T.DUR;
  assert.ok(lastFrame < O.T.TIDY, 'the tidy-up comes after the last mote has faded');
  assert.equal(O.T.PLAIN, 160, 'reduced motion is a 160ms crossfade');
});

test('tapAction: on a touch screen the first tap peeks and the second opens; pointer and keyboard open at once', () => {
  assert.equal(O.tapAction('touch', false), 'peek');
  assert.equal(O.tapAction('touch', true), 'open');
  assert.equal(O.tapAction('mouse', false), 'open');
  assert.equal(O.tapAction('pen', false), 'open');
  assert.equal(O.tapAction('key', false), 'open');

  // Two taps in a row on the same card.
  let peeking = false;
  const first = O.tapAction('touch', peeking);
  if (first === 'peek') peeking = true;
  assert.deepEqual([first, O.tapAction('touch', peeking)], ['peek', 'open']);
});

test('isDark and blendFor: motes add up only on a dark page', () => {
  assert.equal(O.isDark('dark', false), true);
  assert.equal(O.isDark('light', true), false);
  assert.equal(O.isDark(null, true), true);
  assert.equal(O.isDark(null, false), false);
  assert.equal(O.blendFor(true), 'lighter');
  assert.equal(O.blendFor(false), 'source-over');
});

test('isPlainClick: modified and non-primary clicks are left to the browser', () => {
  const base = { defaultPrevented: false, button: 0, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false };
  assert.equal(O.isPlainClick(base), true);
  for (const k of ['metaKey', 'ctrlKey', 'shiftKey', 'altKey']) assert.equal(O.isPlainClick({ ...base, [k]: true }), false, k);
  assert.equal(O.isPlainClick({ ...base, button: 1 }), false);
  assert.equal(O.isPlainClick({ ...base, defaultPrevented: true }), false);
});

test('escapeReturns: only a freshly opened calendar with nothing of its own open goes back', () => {
  const quiet = { open: true, defaultPrevented: false, panelOpen: false, editing: false, drawerOpen: false, typing: false };
  assert.equal(O.escapeReturns(quiet), true);
  for (const k of ['panelOpen', 'editing', 'drawerOpen', 'typing', 'defaultPrevented']) {
    assert.equal(O.escapeReturns({ ...quiet, [k]: true }), false, `${k} takes Escape first`);
  }
  assert.equal(O.escapeReturns({ ...quiet, open: false }), false, 'a calendar reached any other way stays put');
});
