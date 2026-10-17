// jot_place.test.mjs — where the Jot notes sheet sits (jot_place.js): saved
// places read back safely, the sheet stays on screen and grabbable, pinning
// takes a bounded width, and stepping aside goes to the far corner. Plus the
// wiring in notes.js that has to keep holding: step-aside listeners are
// taken down with the widget, and the page gets its room back.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/jot_place.js'), 'utf8');
const win = {};
new Function('window', src)(win);
const JP = win.Chronicle.JotPlace;

test('reading a saved place', async (t) => {
  const cases = [
    { name: 'nothing saved', in: null, want: { x: null, y: null, w: 320, h: 480, dock: null, folded: false } },
    { name: 'garbage', in: 'nope', want: { x: null, y: null, w: 320, h: 480, dock: null, folded: false } },
    { name: 'an older size-only save keeps its size', in: { w: 400, h: 300 }, want: { x: null, y: null, w: 400, h: 300, dock: null, folded: false } },
    { name: 'a full place', in: { x: 10, y: 20, w: 300, h: 260, dock: 'l', folded: true }, want: { x: 10, y: 20, w: 300, h: 260, dock: 'l', folded: true } },
    { name: 'half a position is no position', in: { x: 10, w: 300, h: 260 }, want: { x: null, y: null, w: 300, h: 260, dock: null, folded: false } },
    { name: 'an unknown side is not pinned', in: { dock: 'top', folded: 'yes' }, want: { x: null, y: null, w: 320, h: 480, dock: null, folded: false } },
    { name: 'non-numbers are ignored', in: { x: 'a', y: NaN, w: Infinity, h: '9' }, want: { x: null, y: null, w: 320, h: 480, dock: null, folded: false } },
  ];
  for (const c of cases) {
    await t.test(c.name, () => assert.deepEqual(JP.read(c.in), c.want));
  }
});

test('clamping to the screen', async (t) => {
  const cases = [
    { name: 'a never-moved sheet sits in the tab corner', p: JP.fresh(), vw: 1280, vh: 800, want: { x: 1280 - 320 - 24, y: 800 - 480 - 24, w: 320, h: 480 } },
    { name: 'a folded never-moved sheet sits in the corner by its title bar', p: { ...JP.fresh(), folded: true }, vw: 1280, vh: 800, want: { x: 936, y: 800 - 44 - 24, w: 320, h: 480 } },
    { name: 'dragged off the right edge comes back', p: { x: 2000, y: 100, w: 320, h: 400 }, vw: 1280, vh: 800, want: { x: 1280 - 320 - 8, y: 100, w: 320, h: 400 } },
    { name: 'dragged above the top comes back', p: { x: 100, y: -50, w: 320, h: 400 }, vw: 1280, vh: 800, want: { x: 100, y: 8, w: 320, h: 400 } },
    { name: 'never smaller than the minimum', p: { x: 100, y: 100, w: 50, h: 40 }, vw: 1280, vh: 800, want: { x: 100, y: 100, w: 260, h: 220 } },
    { name: 'never larger than the screen', p: { x: 0, y: 0, w: 5000, h: 5000 }, vw: 1000, vh: 700, want: { x: 8, y: 8, w: 984, h: 684 } },
    { name: 'a folded sheet may sit low: only its title bar must show', p: { x: 100, y: 740, w: 320, h: 400, folded: true }, vw: 1280, vh: 800, want: { x: 100, y: 740, w: 320, h: 400 } },
  ];
  for (const c of cases) {
    await t.test(c.name, () => {
      const q = JP.clamp(c.p, c.vw, c.vh);
      assert.deepEqual({ x: q.x, y: q.y, w: q.w, h: q.h }, c.want);
    });
  }
  await t.test('clamping returns a new place', () => {
    const p = { x: 5000, y: 0, w: 320, h: 400, dock: null, folded: false };
    JP.clamp(p, 1280, 800);
    assert.equal(p.x, 5000);
  });
});

test('a pinned sheet takes a bounded width', () => {
  assert.equal(JP.dockWidth({ w: 320 }, 1600), 320);
  assert.equal(JP.dockWidth({ w: 900 }, 1600), 480, 'never wider than the cap');
  assert.equal(JP.dockWidth({ w: 600 }, 800), 400, 'never more than half the screen');
  assert.equal(JP.dockWidth({ w: 100 }, 1600), 260, 'never narrower than the minimum');
});

test('overlap', () => {
  const a = { left: 0, top: 0, right: 100, bottom: 100 };
  assert.equal(JP.overlaps(a, { left: 50, top: 50, right: 150, bottom: 150 }), true);
  assert.equal(JP.overlaps(a, { left: 100, top: 0, right: 200, bottom: 100 }), false, 'touching edges do not overlap');
  assert.equal(JP.overlaps(a, null), false);
});

test('stepping aside for an edit goes folded to the far bottom corner', () => {
  const p = { x: 900, y: 300, w: 320, h: 400, dock: null, folded: false };
  const right = JP.asideFor(p, { left: 800, top: 200, right: 1200, bottom: 500 }, 1280, 800);
  assert.deepEqual({ x: right.x, y: right.y, folded: right.folded }, { x: 8, y: 800 - 44 - 8, folded: true });
  const left = JP.asideFor(p, { left: 100, top: 200, right: 500, bottom: 500 }, 1280, 800);
  assert.equal(left.x, 1280 - 320 - 8);
  assert.equal(p.folded, false, 'the saved place is untouched');
});

test('arrow-key steps', () => {
  assert.deepEqual(JP.step('ArrowLeft', false), [-8, 0]);
  assert.deepEqual(JP.step('ArrowDown', true), [0, 24]);
  assert.equal(JP.step('Enter', false), null);
});

function strip(s) {
  return s.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');
}
const notes = strip(readFileSync(join(root, 'static/js/widgets/notes.js'), 'utf8'));

test('notes.js: step-aside and resize listeners leave with the widget', () => {
  const destroy = notes.slice(notes.indexOf('destroy: function (el)'));
  assert.match(notes, /document\.addEventListener\('focusin', el\._notesFocusIn\)/);
  assert.match(destroy, /document\.removeEventListener\('focusin', el\._notesFocusIn\)/);
  assert.match(notes, /document\.addEventListener\('close', el\._notesDialogClose, true\)/);
  assert.match(destroy, /document\.removeEventListener\('close', el\._notesDialogClose, true\)/);
  assert.match(destroy, /window\.removeEventListener\('resize', el\._notesOnResize\)/);
  assert.match(destroy, /classList\.remove\('jot-dock-l', 'jot-dock-r'\)/, 'the page gets its room back');
});

test('notes.js: a pinned sheet never steps aside, and the saved place survives an aside', () => {
  const i = notes.indexOf('function stepAside(');
  const body = notes.slice(i, notes.indexOf('function comeBack(', i));
  assert.match(body, /place\.dock/, 'a pinned sheet covers nothing, so it stays');
  assert.doesNotMatch(body, /savePlace\(\)/, 'stepping aside is never saved as the place');
  assert.match(notes, /var shown = aside && aside\.place \? aside\.place : place;/);
});

test('notes.js: the place is drawn on open and close', () => {
  const open = notes.slice(notes.indexOf('function openPanel()'), notes.indexOf('function closePanel()'));
  assert.match(open, /applyPlace\(\);/);
  const close = notes.slice(notes.indexOf('function closePanel()'), notes.indexOf("fab.addEventListener('click'"));
  assert.match(close, /aside = null;/);
  assert.match(close, /classList\.remove\('notes-panel-gliding'\)/, 'a later drag must not trail the pointer');
  assert.match(close, /classList\.remove\('jot-dock-l', 'jot-dock-r'\)/, 'closing gives the page its room back');
});

test('notes.js: an outside app\'s frame never shows folded', () => {
  assert.match(notes, /toggle\('notes-panel-collapsed', !embedded && /);
});

test('notes.js: a press on the title bar only moves (or unpins) once the pointer travels', () => {
  const down = notes.slice(notes.indexOf("header.addEventListener('pointerdown'"), notes.indexOf('function startMove('));
  assert.doesNotMatch(down, /place\.dock = null/);
  assert.match(down, /kind: 'press'/);
});
