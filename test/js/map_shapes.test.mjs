// map_shapes.test.mjs — pins the Shapes tool's pure parts: snapping to the
// grid, placing a box from a drag or a click, moving and resizing a box
// without leaving the map or shrinking under its minimum, moving an arrow's
// ends, the keyboard nudge, and the request body for a placed shape. The
// script runs as the browser loads it, minus a window, so it exports them.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'maps', 'static', 'js', 'map_shapes.js'), 'utf8');

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
const S = load();
const STEP = { x: 5, y: 5 };
const near = (a, b, eps = 1e-6) => assert.ok(Math.abs(a - b) < eps, `${a} !~ ${b}`);

test('the palette is the six kinds of the signed design, in order', () => {
  assert.deepEqual(S.KINDS.map((k) => k.key), ['room', 'area', 'water', 'wall', 'arrow', 'bubble']);
  assert.deepEqual(S.KINDS.map((k) => k.type), ['room', 'area', 'water', 'wall', 'arrow', 'callout']);
  assert.ok(S.isBoxType('wall') && !S.isBoxType('arrow') && !S.isBoxType('rectangle'));
  // Danger zones come from the AI map sketch: a box, but no palette button.
  assert.ok(S.isBoxType('danger') && !S.KINDS.some((k) => k.type === 'danger'));
  assert.ok(S.isShapeType('callout') && S.isShapeType('arrow') && !S.isShapeType('freehand'));
});

test('a grid cell is the same share of the width on every map', () => {
  assert.deepEqual(S.gridStep(50, 1000, 1000), { x: 5, y: 5 });
  assert.deepEqual(S.gridStep(50, 4000, 4000), { x: 5, y: 5 });
  // On a wide map a cell is square in pixels, so it is a smaller share of the height.
  const wide = S.gridStep(50, 2000, 1000);
  near(wide.x, 5);
  near(wide.y, 10);
  // A missing size (a blank map) falls back to the default grid, never to zero.
  assert.deepEqual(S.gridStep(undefined, 0, 0), { x: 5, y: 5 });
  assert.ok(S.gridStep(1, 1000, 1000).x >= 0.5, 'a tiny grid still snaps to something usable');
});

test('snap rounds to the step, and Alt turns it off', () => {
  assert.equal(S.snap(12.4, 5, true), 10);
  assert.equal(S.snap(12.6, 5, true), 15);
  assert.equal(S.snap(12.4, 5, false), 12.4);
  assert.equal(S.snap(12.4, 0, true), 12.4);
});

test('a wall snaps to half a cell, so it can be thin', () => {
  assert.deepEqual(S.stepFor('wall', STEP), { x: 2.5, y: 2.5 });
  assert.deepEqual(S.stepFor('room', STEP), STEP);
});

test('boxOf sorts the corners and refuses anything that is not two points', () => {
  assert.deepEqual(S.boxOf([{ x: 30, y: 25 }, { x: 10, y: 10 }]), { x1: 10, y1: 10, x2: 30, y2: 25 });
  assert.equal(S.boxOf([{ x: 1, y: 1 }]), null);
  assert.equal(S.boxOf([{ x: 1, y: 1 }, { x: 2, y: NaN }]), null);
  assert.equal(S.boxOf(null), null);
  assert.deepEqual(S.pointsOfBox({ x1: 10, y1: 10, x2: 30, y2: 25 }), [{ x: 10, y: 10 }, { x: 30, y: 25 }]);
});

test('dragging out a box snaps both corners and orders them', () => {
  const b = S.placeBox('room', { x: 31, y: 24 }, { x: 11, y: 9 }, false, STEP, true);
  assert.deepEqual(b, { x1: 10, y1: 10, x2: 30, y2: 25 });
});

test('dragging out a box without snapping keeps the corners where they were', () => {
  const b = S.placeBox('room', { x: 11, y: 9 }, { x: 31.5, y: 24 }, false, STEP, false);
  assert.deepEqual(b, { x1: 11, y1: 9, x2: 31.5, y2: 24 });
});

test('a drag too short to span a cell grows to one cell toward the drag', () => {
  const b = S.placeBox('room', { x: 20, y: 20 }, { x: 21, y: 20.5 }, false, STEP, true);
  assert.deepEqual(b, { x1: 20, y1: 20, x2: 25, y2: 25 });
  const back = S.placeBox('room', { x: 20, y: 20 }, { x: 19, y: 19 }, false, STEP, true);
  assert.deepEqual(back, { x1: 15, y1: 15, x2: 20, y2: 20 });
});

test('a click drops the default size with its top-left at the click', () => {
  assert.deepEqual(S.placeBox('room', { x: 11, y: 9 }, { x: 11, y: 9 }, true, STEP, true), { x1: 10, y1: 10, x2: 25, y2: 20 });
  assert.deepEqual(S.placeBox('area', { x: 10, y: 10 }, { x: 10, y: 10 }, true, STEP, true), { x1: 10, y1: 10, x2: 30, y2: 25 });
  // A wall is long and thin: four cells by half a cell.
  assert.deepEqual(S.placeBox('wall', { x: 10, y: 10 }, { x: 10, y: 10 }, true, STEP, true), { x1: 10, y1: 10, x2: 30, y2: 12.5 });
});

test('a click near the edge slides the shape back onto the map', () => {
  const b = S.placeBox('room', { x: 98, y: 98 }, { x: 98, y: 98 }, true, STEP, true);
  assert.deepEqual(b, { x1: 85, y1: 90, x2: 100, y2: 100 });
});

test('a placed box never breaks the server floor (half a percent)', () => {
  const min = S.MIN_EXTENT;
  for (const type of ['room', 'area', 'water', 'wall']) {
    const b = S.placeBox(type, { x: 50, y: 50 }, { x: 50.01, y: 50.01 }, false, { x: 0.5, y: 0.5 }, false);
    assert.ok(b.x2 - b.x1 >= min - 1e-9 && b.y2 - b.y1 >= min - 1e-9, `${type}: ${JSON.stringify(b)}`);
  }
});

test('moving a box keeps its size, snaps its corner and stays on the map', () => {
  const b = { x1: 10, y1: 10, x2: 30, y2: 25 };
  assert.deepEqual(S.moveBox(b, 12.6, 2, STEP, true), { x1: 25, y1: 10, x2: 45, y2: 25 });
  assert.deepEqual(S.moveBox(b, -50, -50, STEP, true), { x1: 0, y1: 0, x2: 20, y2: 15 });
  assert.deepEqual(S.moveBox(b, 500, 500, STEP, true), { x1: 80, y1: 85, x2: 100, y2: 100 });
  assert.deepEqual(S.moveBox(b, 1.3, 1.3, STEP, false), { x1: 11.3, y1: 11.3, x2: 31.3, y2: 26.3 });
});

test('resizing from a corner moves only that corner', () => {
  const b = { x1: 10, y1: 10, x2: 30, y2: 25 };
  const min = S.minSize('room', STEP);
  assert.deepEqual(S.resizeBox(b, 'se', { x: 41, y: 33 }, STEP, true, min), { x1: 10, y1: 10, x2: 40, y2: 35 });
  assert.deepEqual(S.resizeBox(b, 'nw', { x: 4, y: 6 }, STEP, true, min), { x1: 5, y1: 5, x2: 30, y2: 25 });
});

test('resizing from an edge moves only that edge', () => {
  const b = { x1: 10, y1: 10, x2: 30, y2: 25 };
  const min = S.minSize('room', STEP);
  assert.deepEqual(S.resizeBox(b, 'e', { x: 52, y: 99 }, STEP, true, min), { x1: 10, y1: 10, x2: 50, y2: 25 });
  assert.deepEqual(S.resizeBox(b, 's', { x: 99, y: 41 }, STEP, true, min), { x1: 10, y1: 10, x2: 30, y2: 40 });
});

test('a resize cannot shrink under the minimum, flip over or leave the map', () => {
  const b = { x1: 10, y1: 10, x2: 30, y2: 25 };
  const min = S.minSize('room', STEP);
  assert.deepEqual(S.resizeBox(b, 'se', { x: 0, y: 0 }, STEP, true, min), { x1: 10, y1: 10, x2: 15, y2: 15 });
  assert.deepEqual(S.resizeBox(b, 'nw', { x: 99, y: 99 }, STEP, true, min), { x1: 25, y1: 20, x2: 30, y2: 25 });
  assert.deepEqual(S.resizeBox(b, 'se', { x: 250, y: 250 }, STEP, true, min), { x1: 10, y1: 10, x2: 100, y2: 100 });
  assert.deepEqual(S.resizeBox(b, 'nw', { x: -40, y: -40 }, STEP, true, min), { x1: 0, y1: 0, x2: 30, y2: 25 });
});

test('moving an arrow keeps its length, and the tip and tail move on their own', () => {
  const pts = [{ x: 10, y: 10 }, { x: 30, y: 20 }];
  assert.deepEqual(S.shiftPoints(pts, 12.6, 0, STEP, true), [{ x: 25, y: 10 }, { x: 45, y: 20 }]);
  // Clamped as a whole: neither end leaves the map, and the length survives.
  assert.deepEqual(S.shiftPoints(pts, 500, 0, STEP, true), [{ x: 80, y: 10 }, { x: 100, y: 20 }]);
  assert.deepEqual(S.moveEnd(pts, 1, { x: 52, y: 41 }, STEP, true), [{ x: 10, y: 10 }, { x: 50, y: 40 }]);
  assert.deepEqual(S.moveEnd(pts, 0, { x: -9, y: 3 }, STEP, true), [{ x: 0, y: 5 }, { x: 30, y: 20 }]);
});

test('an arrow end cannot land on the other end', () => {
  const pts = [{ x: 10, y: 10 }, { x: 30, y: 20 }];
  assert.equal(S.moveEnd(pts, 1, { x: 10, y: 10 }, STEP, true), null);
});

test('placing an arrow: drag tail to tip, or click for a short one', () => {
  assert.deepEqual(S.placeArrow({ x: 11, y: 9 }, { x: 41, y: 31 }, false, STEP, true), [{ x: 10, y: 10 }, { x: 40, y: 30 }]);
  assert.deepEqual(S.placeArrow({ x: 11, y: 9 }, { x: 11, y: 9 }, true, STEP, true), [{ x: 10, y: 10 }, { x: 20, y: 10 }]);
  // At the right edge the click-placed arrow points the other way.
  assert.deepEqual(S.placeArrow({ x: 98, y: 50 }, { x: 98, y: 50 }, true, STEP, true), [{ x: 100, y: 50 }, { x: 90, y: 50 }]);
  // A snapped drag that collapses to a point still makes an arrow.
  const a = S.placeArrow({ x: 20, y: 20 }, { x: 21, y: 21 }, false, STEP, true);
  assert.notDeepEqual(a[0], a[1]);
});

test('a bubble points where it was clicked, snapped and on the map', () => {
  assert.deepEqual(S.placePoint({ x: 12.6, y: 99 }, STEP, true), [{ x: 15, y: 100 }]);
  assert.deepEqual(S.placePoint({ x: 12.6, y: 99 }, STEP, false), [{ x: 12.6, y: 99 }]);
});

test('arrow keys move by a step; Shift resizes a box or moves an arrow tip', () => {
  const room = { drawing_type: 'room', points: [{ x: 10, y: 10 }, { x: 30, y: 25 }] };
  assert.deepEqual(S.nudgePoints(room, 'ArrowRight', false, STEP), [{ x: 15, y: 10 }, { x: 35, y: 25 }]);
  assert.deepEqual(S.nudgePoints(room, 'ArrowUp', false, STEP), [{ x: 10, y: 5 }, { x: 30, y: 20 }]);
  assert.deepEqual(S.nudgePoints(room, 'ArrowRight', true, STEP), [{ x: 10, y: 10 }, { x: 35, y: 25 }]);
  assert.deepEqual(S.nudgePoints(room, 'ArrowLeft', true, STEP), [{ x: 10, y: 10 }, { x: 25, y: 25 }]);
  const arrow = { drawing_type: 'arrow', points: [{ x: 10, y: 10 }, { x: 30, y: 20 }] };
  assert.deepEqual(S.nudgePoints(arrow, 'ArrowDown', false, STEP), [{ x: 10, y: 15 }, { x: 30, y: 25 }]);
  assert.deepEqual(S.nudgePoints(arrow, 'ArrowDown', true, STEP), [{ x: 10, y: 10 }, { x: 30, y: 25 }]);
  const bubble = { drawing_type: 'callout', points: [{ x: 10, y: 10 }] };
  assert.deepEqual(S.nudgePoints(bubble, 'ArrowLeft', false, STEP), [{ x: 5, y: 10 }]);
  assert.equal(S.nudgePoints(bubble, 'Enter', false, STEP), null);
});

test('a nudge against the edge or the minimum changes nothing', () => {
  const edge = { drawing_type: 'room', points: [{ x: 0, y: 0 }, { x: 20, y: 15 }] };
  assert.equal(S.nudgePoints(edge, 'ArrowLeft', false, STEP), null);
  const small = { drawing_type: 'room', points: [{ x: 10, y: 10 }, { x: 15, y: 15 }] };
  assert.equal(S.nudgePoints(small, 'ArrowLeft', true, STEP), null, 'Shift cannot shrink a one-cell box');
  const tip = { drawing_type: 'arrow', points: [{ x: 10, y: 10 }, { x: 15, y: 10 }] };
  assert.equal(S.nudgePoints(tip, 'ArrowLeft', true, STEP), null, 'the tip cannot land on the tail');
});

test('handles sit on the box and the edge handles drop out of a small box', () => {
  const b = { x1: 10, y1: 10, x2: 30, y2: 20 };
  assert.deepEqual(S.handlePos(b, 'nw'), { x: 10, y: 10 });
  assert.deepEqual(S.handlePos(b, 'se'), { x: 30, y: 20 });
  assert.deepEqual(S.handlePos(b, 'n'), { x: 20, y: 10 });
  assert.deepEqual(S.handlePos(b, 'e'), { x: 30, y: 15 });
  assert.equal(S.handlesFor(200, 120).length, 8);
  assert.deepEqual(S.handlesFor(30, 30), ['nw', 'ne', 'se', 'sw']);
  // A long thin wall keeps its end handles but not its side ones.
  assert.deepEqual(S.handlesFor(300, 12).sort(), ['n', 'ne', 'nw', 's', 'se', 'sw']);
});

test('the request for a placed shape carries the rail colour only where the viewer uses it', () => {
  const rail = { color: '#2563eb', width: 4 };
  const room = S.bodyFor('room', [{ x: 1, y: 1 }, { x: 9, y: 9 }], rail);
  assert.equal(room.type, 'room');
  assert.equal(room.opts.text_content, 'Room');
  assert.equal(room.opts.stroke_color, S.BOX_STROKE);
  assert.match(room.opts.stroke_color, /^#[0-9a-f]{6}$/i);
  assert.equal(S.bodyFor('wall', [], rail).opts.text_content, undefined, 'a wall has no name');
  const bubble = S.bodyFor('bubble', [{ x: 1, y: 1 }], rail);
  assert.equal(bubble.type, 'callout');
  assert.equal(bubble.opts.text_content, 'Note');
  assert.equal(bubble.opts.stroke_color, '#2563eb');
  const arrow = S.bodyFor('arrow', [{ x: 1, y: 1 }, { x: 5, y: 5 }], rail);
  assert.equal(arrow.type, 'arrow');
  assert.equal(arrow.opts.stroke_width, 4);
  assert.equal(S.bodyFor('nope', [], rail), null);
});

test('labels name the field each shape has', () => {
  assert.equal(S.fieldLabel('room'), 'Name');
  assert.equal(S.fieldLabel('callout'), 'Bubble text');
  assert.equal(S.fieldLabel('wall'), '');
  assert.equal(S.fieldLabel('danger'), 'Name');
  assert.equal(S.typeLabel('danger'), 'Danger');
  assert.equal(S.fieldLabel('arrow'), '');
  assert.equal(S.typeLabel('callout'), 'Speech bubble');
});
