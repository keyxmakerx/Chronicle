// map_pictures.test.mjs — pins the picture module's pure parts: the limits the
// server also enforces (opacity floor, crop ceiling), the CSS it writes, the
// geometry that keeps a turned picture steady while it is resized, placement
// of a new picture, and stacking order changes.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'map_pictures.js'), 'utf8');

function load() {
  const saved = { window: globalThis.window, document: globalThis.document, module: globalThis.module };
  delete globalThis.window;
  delete globalThis.document;
  globalThis.module = { exports: {} };
  vm.runInThisContext(src);
  const api = globalThis.module.exports;
  globalThis.module = saved.module;
  if (saved.window !== undefined) globalThis.window = saved.window;
  if (saved.document !== undefined) globalThis.document = saved.document;
  return api;
}
const P = load();
const near = (a, b, eps = 1e-6) => assert.ok(Math.abs(a - b) < eps, `${a} !~ ${b}`);

test('opacity is held between 20% and 100%, and unset means opaque', () => {
  assert.equal(P.clampOpacity(0), 1);
  assert.equal(P.clampOpacity(undefined), 1);
  assert.equal(P.clampOpacity(0.05), 0.2);
  assert.equal(P.clampOpacity(0.6), 0.6);
  assert.equal(P.clampOpacity(3), 1);
});

test('turn is held between -45 and 45 degrees', () => {
  assert.equal(P.clampTurn(90), 45);
  assert.equal(P.clampTurn(-90), -45);
  assert.equal(P.clampTurn(12), 12);
  assert.equal(P.clampTurn('x'), 0);
});

test('crop sides stay within 0-45 and a missing side is zero', () => {
  assert.deepEqual(P.normalizeCrop({ t: 60, r: -4, b: 10 }), { t: 45, r: 0, b: 10, l: 0 });
  assert.deepEqual(P.normalizeCrop(null), { t: 0, r: 0, b: 0, l: 0 });
});

test('clip-path insets by the crop, and an empty crop is none', () => {
  assert.equal(P.clipPath({ t: 5, r: 10, b: 0, l: 2.5 }), 'inset(5% 10% 0% 2.5%)');
  assert.equal(P.clipPath(null), 'none');
  assert.equal(P.clipPath({ t: 0, r: 0, b: 0, l: 0 }), 'none');
});

test('corners are ordered whichever way they were dragged', () => {
  assert.deepEqual(P.boxFromPoints([{ x: 40, y: 30 }, { x: 10, y: 10 }]), { minX: 10, maxX: 40, minY: 10, maxY: 30 });
  assert.equal(P.boxFromPoints([{ x: 1, y: 1 }]), null);
  assert.equal(P.boxFromPoints([{ x: 1, y: 1 }, { x: 'a', y: 2 }]), null);
});

test('points round-trip through the centre-based box', () => {
  const box = { minX: 10, maxX: 40, minY: 20, maxY: 30 };
  const g = P.geoFromBox(box, 2000, 1000);
  near(g.w, 600); near(g.h, 100); near(g.cx, 500); near(g.cy, 250);
  const pts = P.pointsFromGeo(g, 2000, 1000);
  assert.deepEqual(pts, [{ x: 10, y: 20 }, { x: 40, y: 30 }]);
});

test('resizing keeps the anchor point fixed on screen, turned or not', () => {
  const g = { cx: 500, cy: 300, w: 400, h: 200 };
  for (const rot of [0, 30, -45]) {
    // anchor: the picture's top-left corner, relative to its centre
    const rx = -g.w / 2, ry = -g.h / 2;
    const before = P.rotate(rx, ry, rot);
    const out = P.scaleGeoAbout(g, rot, rx, ry, 1.5);
    const after = P.rotate(-out.w / 2, -out.h / 2, rot);
    near(out.cx + after.x, g.cx + before.x);
    near(out.cy + after.y, g.cy + before.y);
    near(out.w, 600); near(out.h, 300);
  }
});

test('trimming an edge moves only that edge and stops at 45%', () => {
  const c = P.cropDrag('t', null, 0, 20, 400, 200);
  assert.deepEqual(c, { t: 10, r: 0, b: 0, l: 0 });
  const r = P.cropDrag('r', { t: 0, r: 5, b: 0, l: 0 }, -40, 0, 400, 200);
  assert.equal(r.r, 15);
  const capped = P.cropDrag('l', { t: 0, r: 0, b: 0, l: 40 }, 400, 0, 400, 200);
  assert.equal(capped.l, 45);
  const floored = P.cropDrag('b', null, 0, 500, 400, 200);
  assert.equal(floored.b, 0);
});

test('resizing cannot shrink a picture to nothing or grow it without bound', () => {
  const g = { cx: 0, cy: 0, w: 400, h: 200 };
  const tiny = P.resizeScale(400, 200, -5000, -5000, g, 2000);
  near(tiny * g.w, 2000 * 0.02);
  const huge = P.resizeScale(400, 200, 1e6, 1e6, g, 2000);
  near(huge * g.w, 2000 * 4);
});

test('a new picture keeps its proportions, fits the view and is centred', () => {
  const [a, b] = P.placeNew(1000, 500, 1200, 800, { x: 1000, y: 500 }, 2000, 1000);
  const w = (b.x - a.x) / 100 * 2000;
  const h = (b.y - a.y) / 100 * 1000;
  near(w / h, 2, 1e-2);
  near(w, 1200 * 0.33, 0.5);
  near((a.x + b.x) / 2, 50, 0.01);
  near((a.y + b.y) / 2, 50, 0.01);
  // A tall picture is held to 60% of the view's height.
  const [c, d] = P.placeNew(100, 1000, 1200, 800, { x: 1000, y: 500 }, 2000, 1000);
  const th = (d.y - c.y) / 100 * 1000;
  assert.ok(th <= 800 * 0.6 + 0.5);
});

test('forward and back swap neighbours and report only what changed', () => {
  const items = [{ id: 'a', sort_order: 0 }, { id: 'b', sort_order: 1 }, { id: 'c', sort_order: 2 }];
  assert.deepEqual(P.reorder(items, 'a', 'forward'), [{ id: 'b', sort_order: 0 }, { id: 'a', sort_order: 1 }]);
  assert.deepEqual(P.reorder(items, 'c', 'back'), [{ id: 'c', sort_order: 1 }, { id: 'b', sort_order: 2 }]);
  assert.deepEqual(P.reorder(items, 'c', 'forward'), []);
  assert.deepEqual(P.reorder(items, 'a', 'back'), []);
  assert.deepEqual(P.reorder(items, 'zzz', 'forward'), []);
});

test('pictures that tie on stacking number are renumbered so a move takes effect', () => {
  const tied = [{ id: 'a', sort_order: 0 }, { id: 'b', sort_order: 0 }, { id: 'c', sort_order: 0 }];
  const changes = P.reorder(tied, 'a', 'forward');
  // a was first among ties; after moving forward the order is b, a, c.
  assert.deepEqual(changes, [{ id: 'a', sort_order: 1 }, { id: 'c', sort_order: 2 }]);
});
