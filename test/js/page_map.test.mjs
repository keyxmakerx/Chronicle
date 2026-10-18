// page_map.test.mjs: the pure helpers of the Map page block. The widget file
// is loaded in a bare vm context with a stubbed Chronicle, the way the server
// ships it, and its exported helpers are driven directly; the DOM is not needed.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'internal', 'plugins', 'pageblocks', 'static', 'js', 'page_map.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { H: sandbox.window.PageMapHelpers, registered };
}

const plain = (o) => JSON.parse(JSON.stringify(o));

test('the widget registers once under its mount name', () => {
  const { registered } = load();
  assert.deepEqual(Object.keys(registered), ['page_map']);
  assert.equal(typeof registered.page_map.init, 'function');
  assert.equal(typeof registered.page_map.destroy, 'function');
});

test('the helpers load without Chronicle, for pages that only read them', () => {
  const sandbox = { window: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  assert.equal(typeof sandbox.window.PageMapHelpers.normalise, 'function');
});

test('snap and clamp land on the grid', () => {
  const { H } = load();
  const cases = [[3.24, 1, 3], [3.6, 1, 4], [3.24, 0.5, 3], [3.3, 0.5, 3.5], [-0.2, 0.5, -0]];
  for (const [v, step, want] of cases) assert.equal(H.snap(v, step), want, `${v} by ${step}`);
  assert.equal(H.clamp(5, 0, 3), 3);
  assert.equal(H.clamp(-1, 0, 3), 0);
});

test('limits keep every kind of thing on the grid', () => {
  const { H } = load();
  const cases = [
    ['box', { t: 'room', x: 2, y: 3, w: 5, h: 4 }, { x0: -2, x1: 33, y0: -3, y1: 19 }],
    ['line', { t: 'path', pts: [[1, 2], [10, 20]] }, { x0: -1, x1: 30, y0: -2, y1: 6 }],
    ['marker', { x: 0.5, y: 25.5, style: 'number' }, { x0: 0, x1: 39, y0: -25, y1: 0 }],
    ['label', { t: 'label', x: 4, y: 5, name: 'x' }, { x0: -4, x1: 34, y0: -4, y1: 21 }]
  ];
  for (const [name, o, want] of cases) assert.deepEqual(plain(H.limits(o)), want, name);
});

test('settle snaps a move and never passes the edge', () => {
  const { H } = load();
  const room = { t: 'room', x: 30, y: 2, w: 8, h: 4 };
  assert.deepEqual(plain(H.settle(room, 1.4, 0.6)), { dx: 1, dy: 1 });
  assert.deepEqual(plain(H.settle(room, 9, -9)), { dx: 2, dy: -2 }, 'clamped to the grid');
  const pin = { x: 3, y: 3, style: 'icon' };
  assert.deepEqual(plain(H.settle(pin, 0.3, 0.2)), { dx: 0.5, dy: 0 }, 'markers land on half cells');
  const wall = { t: 'wall', pts: [[1, 1], [5, 1]] };
  H.moveBy(wall, 2, 3);
  assert.deepEqual(plain(wall.pts), [[3, 4], [7, 4]]);
});

test('numbering skips hidden markers and puts DM-only ones last', () => {
  const { H } = load();
  const pins = [
    { id: 'a', style: 'number', dm: true },
    { id: 'b', style: 'number' },
    { id: 'c', style: 'icon' },
    { id: 'd', style: 'number', hidden: true },
    { id: 'e', style: 'number' }
  ];
  assert.deepEqual(plain(H.pinNumbers(pins, true)), { b: 1, e: 2, a: 3 }, 'the Director sees the players\' numbers, then theirs');
  assert.deepEqual(plain(H.pinNumbers(pins, false)), { b: 1, e: 2 }, 'a player sees no DM-only marker');
  assert.deepEqual(plain(H.visiblePins(pins, false).map((p) => p.id)), ['b', 'c', 'e']);
});

test('colours: the theme, a named token, or a hex outside amber', () => {
  const { H } = load();
  const cases = [
    ['', ''], ['teal', 'var(--map-c-1)'], ['LIME', 'var(--map-c-7)'], ['#3366CC', '#3366cc'],
    ['red', ''], ['#abc', ''], ['url(x)', ''], ['#f59e0b', ''], ['#d97706', ''], ['#fdf6e3', ''], ['#a09080', '#a09080']
  ];
  for (const [c, want] of cases) assert.equal(H.colorVal(c), want, JSON.stringify(c));
  assert.equal(H.amberish('#fbbf24'), true);
  assert.equal(H.amberish('#808080'), false, 'grey is not amber');
  assert.equal(H.amberish('#ffd700'), true, 'gold is inside the band');
  assert.equal(H.amberish('#ff7000'), true, 'hue 26 is inside the band');
  assert.equal(H.amberish('#b39a5c'), false, 'a muted tan is allowed');
  assert.equal(H.inkFor('#ffff99'), 'dark');
  assert.equal(H.inkFor('#1e3a8a'), 'light');
  assert.equal(H.inkFor('sky'), '', 'named colours carry their own ink');
});

test('bbox frames boxes, lines and labels in drawing units', () => {
  const { H } = load();
  assert.deepEqual(plain(H.bbox({ t: 'room', x: 1, y: 2, w: 3, h: 4 })), { x: 10, y: 20, w: 30, h: 40 });
  assert.deepEqual(plain(H.bbox({ t: 'path', pts: [[5, 1], [2, 4]] })), { x: 20, y: 10, w: 30, h: 30 });
  const lb = H.bbox({ t: 'label', x: 2, y: 3, name: 'ab' });
  assert.equal(lb.w, 30, 'a short label keeps a usable frame');
});

test('normalise survives null and drops what cannot be drawn', () => {
  const { H } = load();
  assert.deepEqual(plain(H.normalise(null)), { grid: { w: 40, h: 26 }, background: 'none', shapes: [], pins: [] });
  const d = H.normalise({
    background: 'stripes',
    shapes: [
      { id: 's1', t: 'room', x: 1, y: 1, w: 3, h: 3, color: 'nope' },
      { id: 's2', t: 'path', pts: [[1, 1]] },
      { id: 's3', t: 'blob' },
      { id: 's4', t: 'label', x: 2, y: 2, name: 'Here', color: 'rose' }
    ],
    pins: [{ id: 'p1', x: 99, y: -4, style: 'icon', icon: 'rocket', dm: 1, entityName: 'The Vigil', entityUrl: '/campaigns/c/entities/e' }]
  });
  assert.equal(d.background, 'none');
  assert.deepEqual(plain(d.shapes.map((s) => s.id)), ['s1', 's4']);
  assert.equal(d.shapes[0].color, '');
  assert.equal(d.shapes[1].color, 'rose', 'a label keeps its colour');
  const p = d.pins[0];
  assert.deepEqual([p.x, p.y, p.icon, p.dm, p.entityId], [39.5, 0.5, 'location-dot', true, '']);
  assert.equal(p.entityName, 'The Vigil');
});

test('toWire leaves out what only the server writes, and wordless labels', () => {
  const { H } = load();
  const doc = H.normalise({
    shapes: [{ id: 'l1', t: 'label', x: 1, y: 2, name: '  ' }, { id: 'r', t: 'room', x: 0, y: 0, w: 1, h: 1 }],
    pins: [{ id: 'p', x: 1, y: 1, style: 'number', entityId: 'e1', entityName: 'N', entityUrl: '/u', hidden: true }]
  });
  const w = H.toWire(doc);
  assert.deepEqual(plain(w.shapes.map((s) => s.id)), ['r']);
  assert.equal('entityName' in w.pins[0], false);
  assert.equal('entityUrl' in w.pins[0], false);
  assert.equal('hidden' in w.pins[0], false);
  assert.equal(w.pins[0].entityId, 'e1');
  assert.equal(doc.pins[0].entityName, 'N', 'toWire must not change the document');
});

test('a drag draws a shape that lands on whole cells, and a click draws nothing', () => {
  const { H } = load();
  const box = H.land(H.drawn('room', 4, 4, 9.6, 1.2));
  assert.deepEqual([box.x, box.y, box.w, box.h], [4, 1, 6, 3]);
  assert.equal(H.land(H.drawn('area', 4, 4, 4.2, 4.3)), null);
  const wall = H.land(H.drawn('wall', 2, 2, 7.7, 2.1));
  assert.deepEqual(plain(wall.pts), [[2, 2], [8, 2]]);
  assert.equal(H.land(H.drawn('path', 2, 2, 2.2, 1.8)), null);
  const off = H.land(H.drawn('water', 38, 24, 60, 60));
  assert.deepEqual([off.w, off.h], [2, 2], 'a drag past the edge stops at it');
});

test('addresses: the preview URL and the campaign from the endpoint', () => {
  const { H } = load();
  assert.equal(H.previewURL('c1', 'e 2'), '/campaigns/c1/entities/e%202/preview');
  assert.equal(H.previewURL('', 'e'), '');
  assert.equal(H.campaignOf('/campaigns/abc-1/page-blocks/e/b'), 'abc-1');
  assert.equal(H.campaignOf('/elsewhere'), '');
});

test('highlight splits a name around the typed words', () => {
  const { H } = load();
  assert.deepEqual(plain(H.highlight('The Vigil', 'vig')), ['The ', 'Vig', 'il']);
  assert.deepEqual(plain(H.highlight('The Vigil', 'zz')), ['The Vigil', '', '']);
  assert.equal(H.esc('<b a="1">'), '&lt;b a=&quot;1&quot;&gt;');
});

test('every allowlisted icon has a glyph, and the eight colours have tokens in both themes', () => {
  const { H } = load();
  for (const ic of H.ICONS) assert.ok(H.ICON_GLYPH[ic], ic);
  const css = readFileSync(join(root, 'static', 'css', 'input.css'), 'utf8');
  const light = css.slice(css.indexOf(':root {'), css.indexOf('.dark {'));
  const dark = css.slice(css.indexOf('.dark {'));
  H.COLORS.forEach((name, i) => {
    assert.match(light, new RegExp(`--map-c-${i}:`), `${name} light`);
    assert.match(dark.slice(0, dark.indexOf('}')), new RegExp(`--map-c-${i}:`), `${name} dark`);
  });
});
