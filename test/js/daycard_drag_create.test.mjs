// daycard_drag_create.test.mjs — C-CALV4-EDITOR-R2b stage 4, [DC-11] SIGNED.
//
// THE SEVEN TERMS, ASSERTED RATHER THAN PROMISED. [DC-11] made drag-create
// severable BY CONSTRUCTION and enumerated what that means; this file is where
// each term stops being a sentence in a commit message.
//
// The two that carry the most weight, because they are the two a later hand
// would break without noticing:
//
//   TERM 5 — A DRAG OF ZERO CELLS IS A CLICK. Not "a drag that opens the editor
//   with one day selected": the single-day case must reach the shipped opener
//   and open the CARD, exactly as it did before this stage existed.
//
//   [ER-8] — THE PREVIEW NEVER MARKS A CELL. The obvious implementation adds a
//   class to the cells under the pointer, and those cells are inside
//   .cal-block-host where §1 rule 1 forbids it byte for byte. The span is drawn
//   as a page-level overlay or it is not drawn.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './daycard_harness.mjs';

function dragOver(fx, cells) {
  fx.fire('pointerdown', cells[0], { button: 0 });
  for (const c of cells.slice(1)) fx.fire('pointermove', c);
  fx.fire('pointerup', cells[cells.length - 1]);
}

test('TERM 5: a drag of zero cells IS a click, and the card opens as it always did', () => {
  const fx = boot();
  fx.fire('pointerdown', fx.cells[3], { button: 0 });
  fx.fire('pointerup', fx.cells[3]);
  fx.fire('click', fx.cells[3]);

  assert.equal(fx.card.popoverOpen, true, 'the single-day click stopped opening the card');
  assert.equal(fx.editor.popoverOpen, false, 'a zero-cell drag opened the EDITOR');
  assert.equal(fx.card.querySelector('[data-dc-head]').getAttribute('data-day'), 'harptos-3');
});

test('TERM 5: a press that jitters on one cell is still a click', () => {
  const fx = boot();
  fx.fire('pointerdown', fx.cells[3], { button: 0 });
  // The pointer never reaches a DIFFERENT day, so nothing was dragged.
  fx.fire('pointermove', fx.cells[3]);
  fx.fire('pointerup', fx.cells[3]);
  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.popoverOpen, true);
  assert.equal(fx.editor.popoverOpen, false);
});

test('a real drag opens the EDITOR with the span pre-filled, and eats the click', () => {
  const fx = boot();
  dragOver(fx, [fx.cells[3], fx.cells[4], fx.cells[5]]);
  // The click the browser fires after a real drag must not ALSO open the card.
  fx.fire('click', fx.cells[5]);

  assert.equal(fx.editor.popoverOpen, true, 'the drag did not open the editor');
  assert.equal(fx.card.popoverOpen, false, 'the suppressed click opened the card anyway');

  // TERM 4: the span rides end_year / end_month / end_day — the fields the
  // shipped POST already binds. ZERO NEW API.
  const q = (h) => fx.editor.querySelector('[data-de-' + h + ']').value;
  assert.equal(q('day'), '3', 'the start date is not the cell the drag began on');
  assert.equal(q('endday'), '5', 'the end date is not the cell the drag ended on');
  assert.equal(q('year'), '1523');
  assert.equal(q('endyear'), '1523');
});

test('a BACKWARD drag creates the same span, ordered forward', () => {
  const fx = boot();
  dragOver(fx, [fx.cells[5], fx.cells[4], fx.cells[3]]);
  const q = (h) => fx.editor.querySelector('[data-de-' + h + ']').value;
  // dayRange orders its two ends, so an end date can never precede its start —
  // the same law the `Ends` cycler is built on, reached from the other side.
  assert.equal(q('day'), '3');
  assert.equal(q('endday'), '5');
});

test('[ER-8]: the preview is a PAGE-LEVEL overlay and never a class on a cell', () => {
  const fx = boot();
  const before = fx.blockHost.outerHTML;

  fx.fire('pointerdown', fx.cells[3], { button: 0 });
  fx.fire('pointermove', fx.cells[4]);

  // THE BLOCK IS BYTE-IDENTICAL WHILE THE PREVIEW IS ON SCREEN. This is the
  // whole of the ruling: "just a class, just during a drag" is exactly how this
  // bound would be lost.
  assert.equal(fx.blockHost.outerHTML, before,
    'the drag preview marked the Block’s own cells');

  const layer = fx.root.querySelector('[data-dc-drag]');
  assert.ok(layer, 'no overlay was drawn at all');
  assert.equal(layer.closest('[data-cal-block]'), null,
    'the overlay was mounted INSIDE the Block host');
  assert.ok(layer.querySelectorAll('.dragbox').length >= 1, 'the overlay drew no span');

  fx.fire('pointerup', fx.cells[4]);
  assert.equal(fx.blockHost.outerHTML, before, 'the drag mutated the Block on release');
});

test('[ER-8]: the span is one box per contiguous ROW, never a union over the weeks', () => {
  const fx = boot();
  // The fixture's three cells share a row band by construction (all top:300),
  // so a three-day run is ONE box — and a union over two rows would be visible
  // as a box taller than a cell.
  fx.fire('pointerdown', fx.cells[3], { button: 0 });
  fx.fire('pointermove', fx.cells[5]);
  const boxes = fx.root.querySelector('[data-dc-drag]').querySelectorAll('.dragbox');
  assert.equal(boxes.length, 1);
  assert.equal(boxes[0].style.height, '84px', 'the box is not one row tall');
  fx.fire('pointerup', fx.cells[5]);
});

test('the overlay is cleared on every exit path, and so is the selection lock', () => {
  const fx = boot();
  const sel = () => fx.root.style.getPropertyValue('user-select');

  fx.fire('pointerdown', fx.cells[3], { button: 0 });
  fx.fire('pointermove', fx.cells[4]);
  assert.equal(sel(), 'none', 'TERM 7: text selection was not suppressed during the drag');

  fx.fire('pointerup', fx.cells[4]);
  assert.equal(sel(), '', 'TERM 7: the suppression outlived the drag');
  assert.equal(fx.root.querySelector('[data-dc-drag]').hidden, true);

  // A CANCELLED POINTER — the OS took it, the window blurred — must not leave
  // the page unselectable either.
  fx.fire('pointerdown', fx.cells[3], { button: 0 });
  fx.fire('pointermove', fx.cells[5]);
  assert.equal(sel(), 'none');
  fx.fire('pointercancel', fx.cells[5]);
  assert.equal(sel(), '');
  assert.equal(fx.root.querySelector('[data-dc-drag]').hidden, true);
});

test('TERM 6: a PLAYER never receives the drag listener at all', () => {
  const fx = boot({ canEdit: false });
  fx.fire('pointerdown', fx.cells[3], { button: 0 });
  fx.fire('pointermove', fx.cells[5]);
  fx.fire('pointerup', fx.cells[5]);

  assert.equal(fx.root.querySelector('[data-dc-drag]'), null,
    'a player’s page drew a drag overlay — the gate is the PRODUCER’s and the ' +
    'module must not compute it');
  assert.equal(fx.editor, null, 'the fixture rendered an editor for a player');
  // …and the shipped click still works for them, which is term 5 from the
  // other side: a player reads days, they simply cannot drag one into an event.
  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.popoverOpen, true);
});

test('a secondary button is not a drag', () => {
  const fx = boot();
  fx.fire('pointerdown', fx.cells[3], { button: 2 });
  fx.fire('pointermove', fx.cells[5]);
  assert.equal(fx.root.querySelector('[data-dc-drag]'), null,
    'a right-click started a drag, which would swallow the context menu');
});

test('dayRange is the ordered list’s own slice, intercalary day included', () => {
  const P = boot().pure;
  const plain = (v) => JSON.parse(JSON.stringify(v));
  const dates = [
    { key: 'h-1', ord: '1', day: 1 }, { key: 'h-2', ord: '2', day: 2 },
    { key: 'h-3', ord: '3', day: 3 }, { key: 'h-i1', ord: 'i1', day: 1 },
  ];
  assert.deepEqual(plain(P.dayRange(dates, 'h-1', 'h-3').map((d) => d.key)),
    ['h-1', 'h-2', 'h-3']);
  // ORDERED, whichever end the pointer started from.
  assert.deepEqual(plain(P.dayRange(dates, 'h-3', 'h-1').map((d) => d.key)),
    ['h-1', 'h-2', 'h-3']);
  // THE INTERCALARY DAY IS THE LAST ENTRY, so a run that ends on it is a run
  // like any other — it is not "day 31" and no arithmetic here pretends it is.
  assert.deepEqual(plain(P.dayRange(dates, 'h-2', 'h-i1').map((d) => d.key)),
    ['h-2', 'h-3', 'h-i1']);
  // An unknown key is not a run.
  assert.deepEqual(plain(P.dayRange(dates, 'h-2', 'nope')), []);
});
