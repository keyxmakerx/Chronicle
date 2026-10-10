// map_measure.test.mjs — pins the Measure tool's pure parts: distances from a
// stored scale (two points in percent plus a length), the running totals at
// each turn, the readout on a hex map (built from map_hexes.js's own hexLine,
// distance and tripSummary, so both tools word a trip alike), the double-click
// clean-up and the body "Save scale" sends.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const jsDir = path.join(here, '..', '..', 'internal', 'plugins', 'maps', 'static', 'js');

function load(file) {
  const src = readFileSync(path.join(jsDir, file), 'utf8');
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

const hx = load('map_hexes.js');
const M = load('map_measure.js');
M.useHexMath(hx);

test('unitsPerPixel reads the stored line against the picture size', () => {
  const cases = [
    { name: 'horizontal line', scale: { a: { x: 10, y: 20 }, b: { x: 60, y: 20 }, length: 12.5, unit: 'miles' }, w: 1000, h: 500, want: 12.5 / 500 },
    { name: 'percent on each axis uses that axis', scale: { a: { x: 0, y: 0 }, b: { x: 0, y: 50 }, length: 10, unit: 'km' }, w: 1000, h: 400, want: 10 / 200 },
    { name: 'diagonal', scale: { a: { x: 0, y: 0 }, b: { x: 30, y: 40 }, length: 5, unit: 'feet' }, w: 100, h: 100, want: 5 / 50 },
    { name: 'no scale', scale: null, w: 1000, h: 1000, want: null },
    { name: 'unknown unit', scale: { a: { x: 0, y: 0 }, b: { x: 10, y: 0 }, length: 5, unit: 'cubits' }, w: 100, h: 100, want: null },
    { name: 'zero length', scale: { a: { x: 0, y: 0 }, b: { x: 10, y: 0 }, length: 0, unit: 'km' }, w: 100, h: 100, want: null },
    { name: 'both ends on one spot', scale: { a: { x: 5, y: 5 }, b: { x: 5, y: 5 }, length: 5, unit: 'km' }, w: 100, h: 100, want: null }
  ];
  for (const c of cases) {
    const got = M.unitsPerPixel(c.scale, c.w, c.h);
    if (c.want === null) assert.equal(got, null, c.name);
    else assert.ok(Math.abs(got - c.want) < 1e-12, `${c.name}: ${got} != ${c.want}`);
  }
});

test('legs and running totals along a path', () => {
  const pts = [{ x: 0, y: 0 }, { x: 300, y: 400 }, { x: 300, y: 1000 }];
  const legs = M.legLengths(pts, 0.01);
  assert.deepEqual(legs.map((n) => Math.round(n * 1000) / 1000), [5, 6]);
  assert.deepEqual(M.running(legs).map((n) => Math.round(n * 1000) / 1000), [5, 11]);
  assert.equal(M.sum([1, 2, null, 3]), 6);
  assert.deepEqual(M.legLengths([{ x: 1, y: 1 }], 1), []);
});

test('fmt rounds as people read distances', () => {
  const cases = [[0.04, '0'], [3.26, '3.3'], [9.96, '10'], [12.4, '12'], [99.6, '100'], [1234.5, '1,235'], [Infinity, '']];
  for (const [n, want] of cases) assert.equal(M.fmt(n), want, String(n));
  assert.equal(M.unitChip(12.4, 'miles'), '12 mi');
  assert.equal(M.unitChip(2.25, 'km'), '2.3 km');
  assert.equal(M.unitChip(40, 'leagues'), '40 lea');
});

test('dropRepeats merges the double-click that finishes a path', () => {
  const cases = [
    { name: 'last point clicked twice', in: [{ x: 0, y: 0 }, { x: 50, y: 0 }, { x: 51, y: 1 }], want: 2 },
    { name: 'distinct points kept', in: [{ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 20, y: 0 }], want: 3 },
    { name: 'a run of repeats is one point', in: [{ x: 0, y: 0 }, { x: 1, y: 0 }, { x: 2, y: 0 }, { x: 3, y: 0 }], want: 1 },
    { name: 'empty', in: [], want: 0 }
  ];
  for (const c of cases) assert.equal(M.dropRepeats(c.in, M.SAME_SPOT_PX).length, c.want, c.name);
});

test('hex paths count hexes leg by leg and read like Plan a trip', () => {
  const cases = [
    {
      name: 'two legs, 3 + 2 hexes',
      hexes: [{ col: 0, row: 0 }, { col: 3, row: 0 }, { col: 3, row: 2 }],
      perHex: 6, perDay: 24,
      legs: [3, 2], head: '5 hexes · 30 miles', about: 'About 1.5 days on foot', cells: 6
    },
    {
      name: 'one hex is half a day at least',
      hexes: [{ col: 2, row: 2 }, { col: 3, row: 2 }],
      perHex: 6, perDay: 24,
      legs: [1], head: '1 hex · 6 miles', about: 'About 0.5 days on foot', cells: 2
    },
    {
      name: 'a path that doubles back counts every step but highlights each hex once',
      hexes: [{ col: 0, row: 0 }, { col: 2, row: 0 }, { col: 0, row: 0 }],
      perHex: 10, perDay: 20,
      legs: [2, 2], head: '4 hexes · 40 miles', about: 'About 2 days on foot', cells: 3
    },
    {
      name: 'every point in one hex',
      hexes: [{ col: 4, row: 4 }, { col: 4, row: 4 }],
      perHex: 6, perDay: 24,
      legs: [0], head: '0 hexes · 0 miles', about: 'About 0 days on foot', cells: 1
    }
  ];
  for (const c of cases) {
    assert.deepEqual(M.hexLegs(c.hexes), c.legs, c.name);
    const r = M.hexReadout(c.hexes, c.perHex, c.perDay);
    assert.equal(r.head, c.head, c.name);
    assert.equal(r.about, c.about, c.name);
    const route = M.hexRoute(c.hexes);
    assert.equal(route.length - 1, c.legs.reduce((a, b) => a + b, 0), c.name + ': route length');
    assert.equal(M.hexCells(route).length, c.cells, c.name + ': highlighted hexes');
  }
  // The days match the trip planner's own rule.
  assert.equal(M.hexReadout([{ col: 0, row: 0 }, { col: 7, row: 0 }], 6, 24).days, hx.travelDays(42, 24));
  assert.equal(M.hexChip(1, 6), '1 hex · 6 mi');
  assert.equal(M.hexChip(4, 6), '4 hexes · 24 mi');
});

test('scaleBody is what Save scale sends, or null when the server would refuse it', () => {
  const a = { x: 10.12345, y: 20 }, b = { x: 60, y: 20 };
  assert.deepEqual(M.scaleBody(a, b, '12.5', 'miles'), { a: { x: 10.123, y: 20 }, b: { x: 60, y: 20 }, length: 12.5, unit: 'miles' });
  // A drag that ends a hair off the picture is pulled onto it.
  assert.deepEqual(M.scaleBody({ x: -0.4, y: 0 }, { x: 100.2, y: 50 }, 3, 'km').a, { x: 0, y: 0 });
  assert.deepEqual(M.scaleBody({ x: -0.4, y: 0 }, { x: 100.2, y: 50 }, 3, 'km').b, { x: 100, y: 50 });
  const refused = [
    ['empty length', a, b, '', 'miles'],
    ['zero', a, b, '0', 'miles'],
    ['negative', a, b, '-2', 'miles'],
    ['not a number', a, b, 'ten', 'miles'],
    ['over the cap', a, b, String(M.MAX_LENGTH + 1), 'miles'],
    ['unknown unit', a, b, '5', 'cubits'],
    ['ends on one spot', { x: 5, y: 5 }, { x: 5.01, y: 5 }, '5', 'km']
  ];
  for (const [name, p, q, len, unit] of refused) assert.equal(M.scaleBody(p, q, len, unit), null, name);
  assert.deepEqual(M.UNIT_ORDER, ['miles', 'km', 'feet', 'leagues']);
});
