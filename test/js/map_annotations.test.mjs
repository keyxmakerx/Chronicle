// map_annotations.test.mjs — pins the annotation module's pure parts: the
// arrowhead's size and placement from the line width, the speech bubble's box
// and tail laid out from the measured text, step numbering that continues from
// the highest step on the map, the highlighter's width, and the stroke fitting
// that keeps a long highlighter inside the server's size ceiling.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'maps', 'static', 'js', 'map_annotations.js'), 'utf8');

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
const A = load();
const near = (a, b, eps = 1e-6) => assert.ok(Math.abs(a - b) < eps, `${a} !~ ${b}`);
const dist = (p, q) => Math.hypot(p.x - q.x, p.y - q.y);

test('the arrowhead points along the line and sits on the tip', () => {
  const h = A.arrowHead({ x: 0, y: 0 }, { x: 100, y: 0 }, 4);
  near(h.tip.x, 100); near(h.tip.y, 0);
  // Barbs sit behind the tip, one either side of the line, mirrored.
  assert.ok(h.left.x < 100 && h.right.x < 100);
  near(h.left.x, h.right.x);
  near(h.left.y, -h.right.y);
  assert.ok(Math.abs(h.left.y) > 0);
  // The shaft stops inside the head, short of the tip.
  assert.ok(h.neck.x < 100 && h.neck.x > h.left.x);
  near(h.neck.y, 0);
});

test('a wider line gets a bigger head', () => {
  const sizes = [2, 4, 7, 12].map((w) => A.arrowHead({ x: 0, y: 0 }, { x: 400, y: 0 }, w).length);
  for (let i = 1; i < sizes.length; i++) assert.ok(sizes[i] > sizes[i - 1], `head for width index ${i} is not bigger`);
  // Even a hairline keeps a head big enough to read.
  assert.ok(A.arrowHead({ x: 0, y: 0 }, { x: 400, y: 0 }, 0.5).length >= 10);
});

test('the head follows the direction of the line', () => {
  const h = A.arrowHead({ x: 10, y: 10 }, { x: 10, y: 110 }, 4); // pointing down the screen
  near(h.tip.y, 110);
  assert.ok(h.left.y < 110 && h.right.y < 110);
  near(h.left.y, h.right.y);
  near(h.left.x - 10, -(h.right.x - 10));
});

test('on a short line the head shrinks to fit', () => {
  const h = A.arrowHead({ x: 0, y: 0 }, { x: 12, y: 0 }, 10);
  assert.ok(h.length <= 12 * 0.8 + 1e-9);
  assert.ok(dist(h.tip, h.left) < 12);
});

test('a zero-length arrow is degenerate but safe', () => {
  const h = A.arrowHead({ x: 5, y: 5 }, { x: 5, y: 5 }, 4);
  assert.equal(h.length, 0);
  assert.ok([h.tip, h.left, h.right, h.neck].every((p) => Number.isFinite(p.x) && Number.isFinite(p.y)));
});

test('the bubble sits above its point, and its tail ends on the point', () => {
  const b = A.BUBBLE;
  const lay = A.bubbleLayout(120, 18);
  assert.deepEqual(lay.tip, { x: 0, y: 0 });
  assert.equal(lay.w, 120 + 2 * b.padX);
  assert.equal(lay.h, 18 + 2 * b.padY);
  // The box's bottom is the tail's height above the point.
  assert.equal(lay.y + lay.h, -b.tailH);
  // The box starts a little left of the point, so the tail runs down its left.
  assert.equal(lay.x, -b.tailInset);
  // Text sits inside the padding.
  assert.equal(lay.text.x, lay.x + b.padX);
  assert.equal(lay.text.y, lay.y + b.padY);
  // The single outline runs down to the point and back.
  assert.match(lay.path, /L 0 0 L 0 /);
  assert.match(lay.path, /^M .* Z$/);
});

test('the bubble grows with its text and never collapses below its tail', () => {
  const small = A.bubbleLayout(0, 0);
  const b = A.BUBBLE;
  assert.ok(small.w >= b.tailInset + b.tailBase + b.radius, 'room for the tail and a corner');
  assert.ok(small.h >= 2 * b.radius);
  const wide = A.bubbleLayout(220, 54);
  assert.ok(wide.w > small.w && wide.h > small.h);
  // Wrapped text (taller) moves the top up, keeping the tail on the point.
  const one = A.bubbleLayout(200, 18);
  const three = A.bubbleLayout(200, 54);
  assert.equal(one.y + one.h, three.y + three.h);
  assert.ok(three.y < one.y);
  // Fractional measurements round up so text is never clipped.
  assert.equal(A.bubbleLayout(100.2, 17.1).w, 101 + 2 * b.padX);
});

test('steps count on from the highest step on the map', () => {
  const step = (n) => ({ drawing_type: 'step', text_content: String(n) });
  const cases = [
    { name: 'an empty map starts at 1', list: [], want: 1 },
    { name: 'other drawings do not count', list: [{ drawing_type: 'text', text_content: '7' }, { drawing_type: 'arrow' }], want: 1 },
    { name: 'continues after the highest', list: [step(1), step(2), step(3)], want: 4 },
    { name: 'a deleted middle step is not refilled', list: [step(1), step(3)], want: 4 },
    { name: 'order does not matter', list: [step(5), step(2)], want: 6 },
    { name: 'damaged numbers are ignored', list: [step(2), { drawing_type: 'step', text_content: 'x' }, { drawing_type: 'step', text_content: null }], want: 3 },
    { name: 'stops at the cap', list: [step(999)], want: 999 },
    { name: 'values past the cap are ignored', list: [step(4), { drawing_type: 'step', text_content: '5000' }], want: 5 },
  ];
  for (const c of cases) assert.equal(A.nextStepNumber(c.list), c.want, c.name);
});

test('the step hint names the next numbers', () => {
  assert.equal(A.stepHint(1), 'Click to place step 1, then 2 and on');
  assert.equal(A.stepHint(4), 'Click to place step 4, then 5 and on');
  assert.equal(A.stepHint(999), 'Click to place step 999');
});

test('a highlighter is always broad and scales with the line choice', () => {
  const thin = A.highlightWidth(2);
  const mid = A.highlightWidth(4);
  const thick = A.highlightWidth(7);
  assert.ok(thin >= 12 && thin < mid && mid < thick && thick <= 40);
  assert.equal(A.highlightWidth(undefined), mid);
});

test('a long highlighter stroke is fitted under the size ceiling, ends kept', () => {
  const pts = [];
  for (let i = 0; i < 3000; i++) pts.push({ x: 10 + (i / 3000) * 80 + 0.0001234, y: 50 + Math.sin(i / 40) * 20 });
  const out = A.fitStroke(pts, 9500);
  assert.ok(JSON.stringify(out).length <= 9500, `fitted stroke is ${JSON.stringify(out).length} bytes`);
  assert.ok(out.length >= 2);
  near(out[0].x, Math.round(pts[0].x * 100) / 100);
  near(out[out.length - 1].x, Math.round(pts[pts.length - 1].x * 100) / 100);
  // Points are rounded to hundredths of a percent.
  assert.ok(out.every((p) => Math.round(p.x * 100) === p.x * 100 || Math.abs(Math.round(p.x * 100) - p.x * 100) < 1e-6));
  // A short stroke passes through untouched apart from rounding.
  assert.deepEqual(A.fitStroke([{ x: 1.234, y: 2 }, { x: 5, y: 6.789 }]), [{ x: 1.23, y: 2 }, { x: 5, y: 6.79 }]);
});
