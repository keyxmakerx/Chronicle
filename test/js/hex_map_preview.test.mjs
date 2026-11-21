// hex_map_preview.test.mjs -- the pure parts of the AI review's hex map
// picture (static/js/widgets/hex_map_preview.js): reading the row's data,
// laying the field out, choosing the look, and placing the Peek card; and the
// shared map_hex_look.js the picture and the map page both draw lines with.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, '..', '..');

function load(file) {
  const src = readFileSync(path.join(root, file), 'utf8');
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
const P = load('static/js/widgets/hex_map_preview.js');
const Look = load('internal/plugins/maps/static/js/map_hex_look.js');
const near = (a, b, eps = 1e-6) => assert.ok(Math.abs(a - b) < eps, `${a} !~ ${b}`);

const lands = ['plains', 'forest', 'water'];

test('parse reads a new map: hexes as arrays, land by index, a trailing name', () => {
  const d = P.parse(JSON.stringify({ cols: 6, rows: 5, art: 'detailed', lands, cells: [[0, 0, 2, null, null, null], [1, 1, 0, 5, null, 0, 'Inn'], [2, 2, -1, null, 3, null]] }));
  assert.equal(d.update, false);
  assert.equal(d.before, d.after, 'a new map has one picture');
  assert.equal(d.after['0,0'].terrain, 'water');
  assert.equal(d.after['1,1'].road, 5);
  assert.equal(d.after['1,1'].trail, 0, 'a line with no exits is 0, not none');
  assert.equal(d.after['1,1'].name, 'Inn');
  assert.equal(d.after['2,2'].terrain, '', 'land -1 is none');
  assert.equal(d.after['2,2'].river, 3);
});

test('parse lays an update over what is stored without changing Before', () => {
  const d = P.parse(JSON.stringify({
    cols: 6, rows: 5, art: 'simple', update: true, lands,
    cells: [[0, 0, 0, null, null, null], [1, 0, 1, null, null, null]],
    after: [[0, 0, 2, null, null, null], [3, 3, 0, null, null, null]],
    ring: [[0, 0]],
  }));
  assert.equal(d.update, true);
  assert.equal(d.art, 'simple');
  assert.equal(d.before['0,0'].terrain, 'plains');
  assert.equal(d.after['0,0'].terrain, 'water');
  assert.equal(d.after['1,0'].terrain, 'forest', 'a stored hex the block does not touch stays');
  assert.equal(d.before['3,3'], undefined);
  assert.equal(d.after['3,3'].terrain, 'plains');
  assert.deepEqual(d.ring, [[0, 0]]);
});

test('parse refuses data it cannot draw', () => {
  for (const bad of ['', 'nope', '{}', '{"cols":0,"rows":4,"cells":[]}', '{"cols":4,"rows":4}']) assert.equal(P.parse(bad), null, bad);
});

test('layout fits the field to the width, hex (0,0) at the corner', () => {
  const g = P.layout(22, 14, 880);
  near(g.w, Math.sqrt(3) * g.r);
  near(g.width, 880, 1.01);
  const [x0, y0] = Look.center(g, 0, 0);
  near(x0 - g.w / 2, 0);
  near(y0 - g.r, 0);
  const [x1] = Look.center(g, 21, 1);
  assert.ok(x1 + g.w / 2 <= g.width + 1, 'the last odd-row hex stays inside the canvas');
  const [, y2] = Look.center(g, 0, 13);
  assert.ok(y2 + g.r <= g.height + 1);
});

test('the look: art by the map\'s style, Realistic steps down, tiny hexes go flat', () => {
  assert.equal(P.lookFor('detailed', 20), 'detailed');
  assert.equal(P.lookFor('real', 20), 'detailed');
  assert.equal(P.lookFor('simple', 20), 'simple');
  assert.equal(P.lookFor('detailed', P.FLAT_R - 0.1), 'flat');
  assert.equal(P.lookFor('simple', P.FLAT_R - 0.1), 'flat');
  // A 100 across map in an 880 wide picture is flat; a 30 x 20 one is not.
  assert.equal(P.lookFor('detailed', P.layout(100, 100, 880).r), 'flat');
  assert.equal(P.lookFor('detailed', P.layout(30, 20, 880).r), 'detailed');
});

test('inField drops hexes outside the field', () => {
  const kept = P.inField({ '0,0': { col: 0, row: 0 }, '5,4': { col: 5, row: 4 }, '6,0': { col: 6, row: 0 }, '0,5': { col: 0, row: 5 }, '-1,0': { col: -1, row: 0 } }, 6, 5);
  assert.deepEqual(Object.keys(kept).sort(), ['0,0', '5,4']);
});

const bounds = { left: 600, right: 1360, top: 0, bottom: 900 };
const want = { w: 520, aspect: 0.55, pad: 8, min: 180 };
const place = (anchor, barTop, w = want) => P.peekPlace(anchor, bounds, barTop, w, 6, 12);

test('peek: below the word when there is room', () => {
  const p = place({ left: 700, right: 800, top: 200, bottom: 220 }, null);
  assert.equal(p.above, false);
  near(p.top, 226);
  near(p.w, 520);
  assert.ok(p.top + p.h <= 900 - 12);
});

test('peek: flips above when below has less room, and clears the save bar', () => {
  // The bar's top is 130 below the word: not enough for a 300 tall card.
  const p = place({ left: 700, right: 800, top: 600, bottom: 620 }, 700);
  assert.equal(p.above, true);
  assert.ok(p.top >= 12, 'inside the panel at the top');
  assert.ok(p.top + p.h <= 600 - 6 + 0.01, 'ends above the word');
  const q = place({ left: 700, right: 800, top: 300, bottom: 320 }, 400);
  assert.ok(q.top + q.h <= 300 - 6 + 0.01 || q.top >= 326, 'never over the bar');
  if (!q.above) assert.ok(q.top + q.h <= 400 - 12 + 0.01, 'below the word it stops at the bar');
});

test('peek: stays inside the panel sideways', () => {
  const right = place({ left: 1340, right: 1359, top: 200, bottom: 220 }, null);
  assert.ok(right.left + right.w <= 1360 - 12 + 0.01);
  const left = place({ left: 590, right: 640, top: 200, bottom: 220 }, null);
  assert.ok(left.left >= 600 + 12 - 0.01);
});

test('peek: narrows, keeping the picture\'s proportions, when neither side has room for the full card', () => {
  // 72 below (to the bar) and 152 above: the card fits above at a narrower width.
  const p = place({ left: 700, right: 800, top: 170, bottom: 190 }, 280);
  assert.equal(p.above, true);
  assert.ok(p.w < 520 && p.w >= 180, `w ${p.w}`);
  near(p.h, (p.w - 16) * 0.55 + 16);
  near(p.top + p.h, 170 - 6);
  assert.ok(p.top >= 12);
});

test('the shared look: Simple art has an icon, lines come out as paths, road has no art', () => {
  assert.match(Look.simpleArt('forest', 0, 0, 26), /<path d="M[^"]+" fill="#5f8f4a" fill-opacity="\.4"\/><g transform/);
  assert.equal(Look.simpleArt('road', 0, 0, 26), '');
  const g = P.layout(6, 5, 600);
  const cells = { '1,1': { col: 1, row: 1, road: 1 }, '2,1': { col: 2, row: 1, road: 8 }, '3,3': { col: 3, row: 3, river: 0 } };
  const out = Look.linesMarkup(g, cells, 'detailed', g.r / 26);
  assert.match(out, /stroke="#2f5d86"/, 'a river is drawn');
  assert.match(out, /stroke="#d3b98a"/, 'a road is drawn');
  assert.doesNotMatch(Look.linesMarkup(g, {}, 'detailed', 1), /<path/);
  assert.match(Look.linesMarkup(g, cells, 'simple', 1), /stroke="#3f7fb5"/, 'Simple draws its own river');
});
