// page_contents.test.mjs — pins the "Contents" block's pure rules: which
// headings count, the four-heading minimum, the section being read, where the
// rail's marks and names sit, and above all where the rail goes beside the
// navigation sidebar (open, folded, hidden) and when it gives way to the
// Contents button.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'entities', 'static', 'js', 'page_contents.js'), 'utf8');
const sandbox = { module: { exports: {} } };
vm.runInNewContext(src, sandbox);
const C = sandbox.module.exports;
const plain = (v) => JSON.parse(JSON.stringify(v));

test('motionOf: reduced motion is always off; otherwise the page attribute decides', () => {
  const cases = [
    { attr: null, reduced: false, want: 'full' },
    { attr: 'calm', reduced: false, want: 'calm' },
    { attr: 'off', reduced: false, want: 'off' },
    { attr: 'calm', reduced: true, want: 'off' },
    { attr: 'bogus', reduced: false, want: 'full' }
  ];
  for (const c of cases) assert.equal(C.motionOf(c.attr, c.reduced), c.want, JSON.stringify(c));
});

test('collectHeadings: empty, hidden and spaced headings', () => {
  const cases = [
    { name: 'plain', items: [{ level: 2, text: 'History' }], want: [{ index: 0, level: 2, text: 'History', dm: false }] },
    { name: 'whitespace collapses', items: [{ level: 2, text: '  The \n  Docks ' }], want: [{ index: 0, level: 2, text: 'The Docks', dm: false }] },
    { name: 'empty heading (all Director text, seen by a player) is skipped', items: [{ level: 2, text: '   ' }, { level: 2, text: 'Trade' }], want: [{ index: 1, level: 2, text: 'Trade', dm: false }] },
    { name: 'hidden heading (another tab, player view) is skipped', items: [{ level: 2, text: 'Plot', hidden: true }, { level: 3, text: 'Trade' }], want: [{ index: 1, level: 3, text: 'Trade', dm: false }] },
    { name: 'Director heading keeps its mark', items: [{ level: 2, text: 'Tunnels', dm: true }], want: [{ index: 0, level: 2, text: 'Tunnels', dm: true }] },
    { name: 'null items and missing text', items: [null, { level: 1 }], want: [] }
  ];
  for (const c of cases) assert.deepEqual(plain(C.collectHeadings(c.items)), c.want, c.name);
});

test('shouldShow: fewer than four headings draws nothing', () => {
  const mk = (n) => Array.from({ length: n }, (_, i) => ({ index: i, level: 2, text: 'h' + i, dm: false }));
  const cases = [[0, false], [1, false], [3, false], [4, true], [40, true]];
  for (const [n, want] of cases) assert.equal(C.shouldShow(mk(n)), want, n + ' headings');
  assert.equal(C.MIN_HEADINGS, 4);
});

test('signature: changes with names and the Director mark, not with levels', () => {
  const a = [{ text: 'A', dm: false, level: 2 }, { text: 'B', dm: false, level: 2 }];
  assert.equal(C.signature(a), C.signature([{ text: 'A', dm: false, level: 3 }, { text: 'B', dm: false, level: 1 }]));
  assert.notEqual(C.signature(a), C.signature([{ text: 'A', dm: false }, { text: 'C', dm: false }]));
  assert.notEqual(C.signature(a), C.signature([{ text: 'A', dm: true }, { text: 'B', dm: false }]));
});

test('activeIndex: the last heading at or above the read line, first before any, last at the end', () => {
  const cases = [
    { name: 'none yet: first', tops: [300, 600, 900], atEnd: false, want: 0 },
    { name: 'second is being read', tops: [-400, 20, 500], atEnd: false, want: 1 },
    { name: 'exactly on the line counts', tops: [-400, 60, 500], atEnd: false, want: 1 },
    { name: 'just under the line does not', tops: [-400, 61, 500], atEnd: false, want: 0 },
    { name: 'all above: last', tops: [-900, -600, -100], atEnd: false, want: 2 },
    { name: 'page end wins over a short last section', tops: [-900, -600, 400], atEnd: true, want: 2 },
    { name: 'no headings', tops: [], atEnd: true, want: -1 }
  ];
  for (const c of cases) assert.equal(C.activeIndex(c.tops, c.atEnd), c.want, c.name);
});

test('marksGeometry: proportional to the whole page, never thinner than 3px, last runs to the story end', () => {
  // 1000px of content on a 512px strip (500 usable): scale 0.5.
  const g = plain(C.marksGeometry([100, 400, 700], 900, 1000, 512));
  assert.deepEqual(g, [{ t: 56, h: 148 }, { t: 206, h: 148 }, { t: 356, h: 98 }]);
  const thin = plain(C.marksGeometry([0, 1], 2, 1000, 512));
  assert.equal(thin[0].h, 3);
  assert.deepEqual(plain(C.marksGeometry([], 0, 0, 100)), []);
});

test('markAt: the section a pointer height falls in, a pixel of slack, clamped to the first', () => {
  const geo = [{ t: 6, h: 40 }, { t: 50, h: 40 }, { t: 100, h: 40 }];
  const cases = [[-20, 0], [0, 0], [48, 0], [49, 1], [50, 1], [98, 1], [99, 2], [999, 2]];
  for (const [y, want] of cases) assert.equal(C.markAt(geo, y), want, 'y=' + y);
});

test('alignRows: names sit level with their marks, push down when they would touch, null when they cannot all fit', () => {
  const geo = [{ t: 6, h: 10 }, { t: 12, h: 10 }, { t: 200, h: 10 }];
  assert.deepEqual(plain(C.alignRows(geo, 400, false)), [0, 32, 189]);
  assert.equal(C.alignRows(geo, 400, true), null, 'a phone has no rail');
  assert.equal(C.alignRows(geo, 40, false), null, 'too short for three rows');
  const many = Array.from({ length: 13 }, (_, i) => ({ t: i * 30, h: 10 }));
  assert.equal(C.alignRows(many, 900, false), null, 'more than twelve is a plain list');
  assert.equal(C.alignRows([], 400, false), null);
});

// Sidebar-aware placement. The page area (#main-content) is the sidebar's flex
// sibling, so its left edge is the sidebar's right edge. Widths: open 256,
// folded 48, hidden 0.
const view = { width: 1440, height: 900 };
const area = (left, right) => ({ left, right, top: 56, bottom: 900, width: right - left });
const sb = (w) => ({ left: 0, right: w, width: w });

test('placement: the rail never overlaps the sidebar, open, folded or hidden', () => {
  const cases = [
    { name: 'sidebar open', main: area(256, 1440), sidebar: sb(256), railLeft: 264, panelLeft: 256 },
    { name: 'sidebar folded to icons', main: area(48, 1440), sidebar: sb(48), railLeft: 56, panelLeft: 48 },
    { name: 'sidebar hidden', main: area(0, 1440), sidebar: sb(0), railLeft: 8, panelLeft: 0 },
    { name: 'no sidebar at all', main: area(0, 1440), sidebar: null, railLeft: 8, panelLeft: 0 },
    { name: 'sidebar sliding open over the page', main: area(100, 1440), sidebar: sb(180), railLeft: 188, panelLeft: 180 },
    { name: 'sidebar slid off to the left', main: area(0, 1440), sidebar: { left: -256, right: 0, width: 256 }, railLeft: 8, panelLeft: 0 }
  ];
  for (const c of cases) {
    const p = C.placement(c.main, c.sidebar, view);
    assert.equal(p.mode, 'rail', c.name);
    assert.equal(p.railLeft, c.railLeft, c.name + ' railLeft');
    assert.equal(p.panelLeft, c.panelLeft, c.name + ' panelLeft');
    const sbRight = c.sidebar && c.sidebar.right > 0 ? c.sidebar.right : 0;
    assert.ok(p.railLeft >= sbRight, c.name + ': the rail starts at or right of the sidebar edge');
    assert.ok(p.panelLeft >= sbRight, c.name + ': the panel starts at or right of the sidebar edge');
  }
});

test('placement: the button takes over when the page area cannot hold the gutter and readable text', () => {
  const room = C.MIN_TEXT + C.GUTTER;
  const cases = [
    { name: 'exactly enough', left: 256, w: room, want: 'rail' },
    { name: 'a pixel short', left: 256, w: room - 1, want: 'button' },
    { name: 'tablet with the sidebar open', left: 256, w: 768 - 256, want: 'button' },
    { name: 'tablet with the sidebar folded', left: 48, w: 1024 - 48, want: 'rail' },
    { name: 'desktop, sidebar open', left: 256, w: 1440 - 256, want: 'rail' }
  ];
  for (const c of cases) {
    const p = C.placement(area(c.left, c.left + c.w), sb(c.left), { width: c.left + c.w, height: 900 });
    assert.equal(p.mode, c.want, c.name);
  }
});

test('placement: a phone always gets the button, and the panel stays inside the page area', () => {
  const phone = C.placement(area(0, 390), sb(0), { width: 390, height: 800 });
  assert.equal(phone.mode, 'button');
  assert.equal(phone.panelWidth, 300);
  const tiny = C.placement(area(0, 280), null, { width: 280, height: 800 });
  assert.equal(tiny.panelWidth, 280, 'never wider than the page area');
  // Even a wide window under the phone breakpoint is the button.
  assert.equal(C.placement(area(0, 767), null, { width: 767, height: 800 }).mode, 'button');
  // And the rail is the same rule on either side of the line.
  assert.equal(C.placement(area(0, 1100), null, { width: 768, height: 800 }).mode, 'rail');
});

test('placement: the rail spans only the visible page area', () => {
  const p = C.placement({ left: 0, right: 1440, top: 56, bottom: 1200, width: 1440 }, null, { width: 1440, height: 900 });
  assert.equal(p.top, 56);
  assert.equal(p.height, 900 - 56);
  const scrolledUnderBar = C.placement({ left: 0, right: 1440, top: -40, bottom: 500, width: 1440 }, null, view);
  assert.equal(scrolledUnderBar.top, 0);
  assert.equal(scrolledUnderBar.height, 500);
});

test('matchRows: Find keeps names containing the words, any case; empty keeps all', () => {
  const e = [{ text: 'History' }, { text: 'The Docks' }, { text: 'Old Quarter' }];
  const cases = [['', [true, true, true]], ['  ', [true, true, true]], ['DOCK', [false, true, false]], ['quarter', [false, false, true]], ['zzz', [false, false, false]], [null, [true, true, true]]];
  for (const [q, want] of cases) assert.deepEqual(plain(C.matchRows(e, q)), want, String(q));
});

test('tipText: the name, the Director flag and "n of N"', () => {
  assert.deepEqual(plain(C.tipText({ text: 'Trade', dm: false }, 4, 7)), { name: 'Trade', dm: false, count: '5 of 7' });
  assert.equal(C.tipText({ text: 'Tunnels', dm: true }, 6, 7).dm, true);
});

test('stepKey: arrows clamp, Home and End jump, other keys are not the strip\'s', () => {
  const cases = [
    ['ArrowDown', 0, 5, 1], ['ArrowDown', 4, 5, 4], ['ArrowUp', 0, 5, 0], ['ArrowUp', 3, 5, 2],
    ['Home', 3, 5, 0], ['End', 1, 5, 4], ['Enter', 1, 5, -1], ['a', 1, 5, -1], ['ArrowDown', 0, 0, -1]
  ];
  for (const [key, cur, n, want] of cases) assert.equal(C.stepKey(key, cur, n), want, key + ' ' + cur + '/' + n);
});

test('escapeShouldClose: Escape alone closes', () => {
  assert.equal(C.escapeShouldClose({ key: 'Escape' }), true);
  assert.equal(C.escapeShouldClose({ key: 'Escape', ctrlKey: true }), false);
  assert.equal(C.escapeShouldClose({ key: 'Enter' }), false);
  assert.equal(C.escapeShouldClose(null), false);
});
