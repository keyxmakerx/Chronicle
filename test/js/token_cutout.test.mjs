// token_cutout.test.mjs — the token maker's background remover
// (static/js/token_cutout.js) on made-up pictures: a painted checkerboard and
// a flat backdrop are noticed and cleared, a gap inside the figure that shows
// the checkerboard is cleared but a bright glow is not, a painted scene and an
// already see-through picture are not offered, and the brushes and the
// fringe clean-up work pixel by pixel.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const T = createRequire(import.meta.url)('../../static/js/token_cutout.js');

// A W by H picture; paint(x, y) gives [r, g, b, a].
function picture(W, H, paint) {
  const data = new Uint8ClampedArray(W * H * 4);
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) data.set(paint(x, y), (y * W + x) * 4);
  return { data, width: W, height: H };
}
const checker = (x, y, z = 10) => (((x / z | 0) + (y / z | 0)) & 1 ? [204, 204, 204, 255] : [255, 255, 255, 255]);
const inDisc = (x, y, cx, cy, r) => (x - cx) ** 2 + (y - cy) ** 2 <= r * r;
// A dark figure: a ring, so there is a gap in its middle.
const FIGURE = [40, 30, 28, 255];
function ringPicture(hole) {
  return picture(200, 200, (x, y) => {
    if (inDisc(x, y, 100, 100, 70) && !inDisc(x, y, 100, 100, 30)) return FIGURE;
    if (inDisc(x, y, 100, 100, 30)) return hole(x, y);
    return checker(x, y);
  });
}
const alphaAt = (a, px, x, y) => a[y * px.width + x];

test('a painted checkerboard is noticed, with its two colours and squares', () => {
  const px = ringPicture(checker), bg = T.detect(px);
  assert.equal(bg.kind, 'checker');
  assert.equal(bg.sure, true);
  const cols = bg.colours.map((c) => c.map(Math.round)).sort((p, q) => q[0] - p[0]);
  assert.deepEqual(cols, [[255, 255, 255], [204, 204, 204]]);
  assert.equal(bg.grid.size, 10);
});

test('Medium clears the checkerboard round the figure and in its gap; Light leaves the gap', () => {
  const px = ringPicture(checker), bg = T.detect(px);
  const med = T.mask(px, bg, 'medium'), light = T.mask(px, bg, 'light');
  for (const [x, y] of [[5, 5], [195, 195], [15, 100]]) {
    assert.equal(alphaAt(med, px, x, y), 0, `background at ${x},${y}`);
    assert.equal(alphaAt(light, px, x, y), 0, `background at ${x},${y}`);
  }
  assert.equal(alphaAt(med, px, 100, 100), 0, 'the gap is cleared at Medium');
  assert.equal(alphaAt(light, px, 100, 100), 255, 'Light only clears what touches the edge');
  for (const [x, y] of [[100, 50], [50, 100], [150, 140]]) assert.equal(alphaAt(med, px, x, y), 255, `figure at ${x},${y}`);
});

test('a bright glow inside the figure is not mistaken for the checkerboard', () => {
  // A lantern's glow: white in the middle, fading to grey, nothing like squares.
  const glow = (x, y) => { const v = Math.round(255 - Math.hypot(x - 100, y - 100) * 1.7); return [v, v, v - 4, 255]; };
  const px = ringPicture(glow), bg = T.detect(px);
  assert.equal(bg.kind, 'checker');
  for (const s of ['medium', 'strong']) assert.equal(alphaAt(T.mask(px, bg, s), px, 100, 100), 255, s);
});

test('a flat backdrop is noticed and cleared, with no light fringe left', () => {
  const px = picture(160, 160, (x, y) => {
    if (inDisc(x, y, 80, 80, 50)) return [120, 20, 20, 255];
    // The disc's edge, half backdrop and half figure, as a painted edge is.
    if (inDisc(x, y, 80, 80, 51)) return [188, 138, 138, 255];
    return [255, 255, 255, 255];
  });
  const bg = T.detect(px);
  assert.deepEqual([bg.kind, bg.sure], ['flat', true]);
  const a = T.mask(px, bg, 'medium');
  assert.equal(alphaAt(a, px, 3, 3), 0);
  assert.equal(alphaAt(a, px, 80, 80), 255);
  const edge = 80 * 160 + 131; // on the rim, beside the backdrop
  assert.ok(a[edge] > 0 && a[edge] < 255, `the rim is half cleared (${a[edge]})`);
  const out = { data: new Uint8ClampedArray(px.data.length) };
  T.compose(px, bg, a, null, out, 0, 0, 160, 160);
  const [r, g] = out.data.slice(edge * 4, edge * 4 + 2);
  assert.ok(r > g + 60, `the rim keeps the figure's red, not the white (${r},${g})`);
});

test('a painted scene and a full-bleed picture are not offered', () => {
  // A dark scene: noisy shadows that fade into the figure, no clean edge.
  let seed = 7;
  const rand = () => { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x7fffffff; };
  const scene = picture(200, 140, (x, y) => {
    const v = 14 + rand() * 18 + Math.max(0, 60 - Math.hypot(x - 120, y - 60)) * 1.2;
    return [v, v + 1, v - 3, 255];
  });
  const s = T.detect(scene);
  assert.equal(s.sure, false, `${s.kind} scene`);
  // A gradient sky running off every edge.
  const sky = picture(200, 140, (x, y) => [40 + y, 90 + y / 2, 200 - x / 4, 255]);
  assert.equal(T.detect(sky).sure, false);
  // A figure that fills the picture inside a hairline margin: almost nothing would be cleared.
  const full = picture(200, 200, (x, y) => (x < 1 || y < 1 || x > 198 || y > 198 ? [255, 255, 255, 255] : [90, 60, 40, 255]));
  assert.equal(T.detect(full).sure, false);
});

test('a picture that is already see-through round its edge is left alone', () => {
  const px = picture(100, 100, (x, y) => (inDisc(x, y, 50, 50, 30) ? FIGURE : [255, 255, 255, 0]));
  const bg = T.detect(px);
  assert.deepEqual([bg.kind, bg.sure], ['clear', false]);
});

test('Erase clears and Restore brings the picture back, over the automatic clearing', () => {
  const px = ringPicture(checker), bg = T.detect(px), a = T.mask(px, bg, 'medium');
  const paint = new Uint8Array(px.width * px.height), out = { data: new Uint8ClampedArray(px.data.length) };
  const at = (x, y) => y * px.width + x;
  paint[at(100, 50)] = 1;   // erase a pixel of the figure
  paint[at(5, 5)] = 2;      // restore a pixel of the backdrop
  T.compose(px, bg, a, paint, out, 0, 0, px.width, px.height);
  assert.equal(out.data[at(100, 50) * 4 + 3], 0);
  assert.deepEqual([...out.data.slice(at(5, 5) * 4, at(5, 5) * 4 + 4)], [255, 255, 255, 255]);
  assert.equal(out.data[at(50, 100) * 4 + 3], 255, 'the rest of the figure is untouched');
  // With nothing cleared automatically, only the brushes count.
  T.compose(px, bg, null, paint, out, 0, 0, px.width, px.height);
  assert.equal(out.data[at(195, 195) * 4 + 3], 255);
  assert.equal(out.data[at(100, 50) * 4 + 3], 0);
});

test('the work for a strength is done once per picture', () => {
  const px = ringPicture(checker), bg = T.detect(px);
  assert.equal(T.mask(px, bg, 'strong'), T.mask(px, bg, 'strong'));
  assert.notEqual(T.mask(px, bg, 'light'), T.mask(px, bg, 'strong'));
});
