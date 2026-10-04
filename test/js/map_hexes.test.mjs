// map_hexes.test.mjs — pins the hex module's pure parts: the hex maths against
// vectors produced by the approved mockup's own functions (the same vectors the
// Go tests use), path building (one path however many hexes), the request
// batching the server's cap needs, and the partial-update merge that keeps a
// paint stroke from touching a name.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'map_hexes.js'), 'utf8');

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
const H = load();
const near = (a, b, eps = 1e-9) => assert.ok(Math.abs(a - b) < eps, `${a} !~ ${b}`);

test('geometry matches the mockup: gridSize 60 over 1000 x 700', () => {
  const g = H.geometry(60, 1000, 700);
  near(g.r, 30);
  near(g.w, 51.96152422706631);
  assert.equal(g.cols, 21);
  assert.equal(g.rows, 17);
});

test('the field never exceeds the server\'s 400 hexes per axis', () => {
  assert.equal(H.MAX_HEX_AXIS, 400);
  const tall = H.geometry(10, 1000, 100000);
  assert.ok(tall.rows <= 400, `rows ${tall.rows}`);
  assert.equal(tall.rows, 400);
  const wide = H.geometry(10, 100000, 1000);
  assert.ok(wide.cols <= 400, `cols ${wide.cols}`);
  assert.equal(H.hexAt(tall, 0, 1.5 * tall.r * 450), null);
});

test('grid size is relative to the map width', () => {
  const small = H.geometry(60, 1000, 700);
  const big = H.geometry(60, 4000, 2800);
  near(big.r, small.r * 4);
  assert.equal(big.cols, small.cols);
});

test('hex centres, odd rows shifted right', () => {
  const g = H.geometry(60, 1000, 700);
  const vectors = [
    [0, 0, 0, 0], [1, 0, 51.96152422706631, 0], [0, 1, 25.980762113533157, 45],
    [3, 2, 155.88457268119893, 90], [6, 7, 337.749907475931, 315],
  ];
  for (const [c, r, x, y] of vectors) {
    const got = H.center(g, c, r);
    near(got[0], x); near(got[1], y);
  }
});

test('offset to cube', () => {
  const vectors = [
    [0, 0, [0, 0, 0]], [1, 0, [1, -1, 0]], [0, 1, [0, -1, 1]], [1, 1, [1, -2, 1]],
    [5, 4, [3, -7, 4]], [7, 9, [3, -12, 9]], [3, 2, [2, -4, 2]], [0, 5, [-2, -3, 5]], [10, 10, [5, -15, 10]],
  ];
  for (const [c, r, want] of vectors) assert.deepEqual(H.cubeOf(c, r), want, `${c},${r}`);
});

test('distance', () => {
  const v = [
    [[0, 0], [3, 0], 3], [[0, 0], [0, 3], 3], [[2, 2], [5, 6], 5], [[1, 1], [4, 1], 3],
    [[4, 3], [4, 3], 0], [[0, 0], [1, 1], 2], [[0, 1], [0, 0], 1],
  ];
  for (const [a, b, want] of v) {
    assert.equal(H.distance({ col: a[0], row: a[1] }, { col: b[0], row: b[1] }), want);
    assert.equal(H.distance({ col: b[0], row: b[1] }, { col: a[0], row: a[1] }), want);
  }
});

test('point to hex, including outside the field', () => {
  const g = H.geometry(60, 1000, 700);
  const v = [
    [0, 0, 0, 0], [10, 5, 0, 0], [26, 0, 1, 0], [27, 10, 1, 0], [40, 20, 1, 0], [100, 80, 2, 2],
    [250.5, 300.25, 4, 7], [500, 400, 9, 9], [999, 699, 19, 16], [-5, -5, 0, 0], [1020, 10, 20, 0],
    [15, 45, 0, 1], [60, 60, 1, 1], [33.3, 77.7, 1, 2],
  ];
  for (const [x, y, c, r] of v) assert.deepEqual(H.hexAt(g, x, y), { col: c, row: r }, `${x},${y}`);
  for (const [x, y] of [[-200, 10], [10, -200], [5000, 10], [10, 5000]]) assert.equal(H.hexAt(g, x, y), null);
  assert.equal(H.hexAt(H.geometry(0, 1000, 700), 10, 10), null);
});

test('every hex centre resolves to its own hex', () => {
  const g = H.geometry(60, 1000, 700);
  for (let r = 0; r < g.rows; r++) {
    for (let c = 0; c < g.cols; c++) {
      const p = H.center(g, c, r);
      assert.deepEqual(H.hexAt(g, p[0], p[1]), { col: c, row: r });
    }
  }
});

test('hexLine matches the mockup and has no gaps', () => {
  const line = H.hexLine({ col: 0, row: 0 }, { col: 4, row: 3 }).map((h) => `${h.col},${h.row}`);
  assert.deepEqual(line, ['0,0', '1,0', '1,1', '2,1', '3,2', '4,2', '4,3']);
  for (let i = 1; i < line.length; i++) {
    const a = line[i - 1].split(',').map(Number), b = line[i].split(',').map(Number);
    assert.equal(H.distance({ col: a[0], row: a[1] }, { col: b[0], row: b[1] }), 1);
  }
  assert.deepEqual(H.hexLine({ col: 2, row: 2 }, { col: 2, row: 2 }), [{ col: 2, row: 2 }]);
});

test('a terrain kind is one path however many hexes it has', () => {
  const g = H.geometry(60, 1000, 700);
  const cells = [];
  for (let i = 0; i < 50; i++) cells.push({ col: i % 10, row: Math.floor(i / 10) });
  const d = H.cellsPath(g, cells);
  assert.equal((d.match(/M/g) || []).length, 50);
  assert.equal((d.match(/Z/g) || []).length, 50);
  assert.equal(H.cellsPath(g, []), '');
});

test('the line grid has one outline per hex of the field', () => {
  const g = H.geometry(60, 1000, 700);
  assert.equal((H.linesPath(g).match(/M/g) || []).length, g.cols * g.rows);
});

test('roads link neighbouring road hexes once, and a lone road hex is a dot', () => {
  const g = H.geometry(60, 1000, 700);
  // A straight run along row 2, then a branch down to row 3.
  const run = [{ col: 1, row: 2 }, { col: 2, row: 2 }, { col: 3, row: 2 }, { col: 3, row: 3 }];
  const d = H.roadPath(g, run);
  const links = (d.match(/L/g) || []).length;
  // (1,2)-(2,2), (2,2)-(3,2), and (3,2) to (3,3) which is its lower-left? either way each pair once.
  assert.equal(links, 3);
  assert.match(H.roadPath(g, [{ col: 5, row: 5 }]), /h0\.01/);
  assert.equal(H.roadPath(g, []), '');
});

test('requests are split at the server cap', () => {
  const entries = Array.from({ length: 1201 }, (_, i) => ({ col: i, row: 0, terrain: 'forest' }));
  const out = H.batches(entries);
  assert.deepEqual(out.map((b) => b.cells.length), [500, 500, 201]);
  assert.equal(H.batches([]).length, 0);
});

test('a later change to a hex merges into the queued one and keeps unrelated fields', () => {
  const stroke = { col: 1, row: 1, terrain: 'forest' };
  const rename = H.mergeEntry(stroke, { name: 'Port' });
  assert.deepEqual(rename, { col: 1, row: 1, terrain: 'forest', name: 'Port' });
  // The later value wins, including an explicit clear.
  assert.deepEqual(H.mergeEntry(rename, { terrain: null }), { col: 1, row: 1, terrain: null, name: 'Port' });
  // A paint stroke alone never carries name or notes.
  const only = H.mergeEntry({ col: 2, row: 2 }, { terrain: 'hills' });
  assert.equal('name' in only, false);
  assert.equal('notes' in only, false);
});

test('the palette matches the server allowlist', () => {
  assert.deepEqual(
    H.TERRAINS.map((t) => t[0]),
    ['plains', 'forest', 'hills', 'mountain', 'water', 'swamp', 'desert', 'snow', 'town', 'road'],
  );
  // Every filled terrain has an icon; road is a line.
  for (const [id] of H.TERRAINS) assert.equal(id === 'road' ? !H.SIMPLE[id] : !!H.SIMPLE[id], true, id);
});
