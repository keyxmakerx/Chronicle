// daycard_ledger_door.test.mjs — the LEDGER's `+ New event` door
// (calendar-v4 refinement stage 3).
//
// The Ledger's day panel does not ship a second create path. It ships the DAY
// CARD'S control — same `data-dc-new` handle, same delegated listener, same
// editor, same shipped POST route — with one thing the card's own door does not
// need: `data-day`, naming the day it belongs to.
//
// THAT ATTRIBUTE IS THE WHOLE SUITE. The card's door is INSIDE the card and can
// only ever mean the day the card is showing, so the module reads `state.key`
// for it and is always right. The Ledger's door is not: the Ledger is a docked
// column that repaints by CSS, so the chosen day can move by keyboard with no
// card involved, and `closeCard()` deliberately keeps `state.key` when the card
// is dismissed. Reading `state` for this door would therefore create the event
// on a day that merely LOOKS plausible — the quiet wrong-date bug that reads as
// intentional in review and shows up as a player asking why the feast moved.
//
// The gate itself is markup-level and the producer's: with `canEdit` false the
// fixture renders no door at all, exactly as ledger.templ renders none for a
// viewer under the authoring floor.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './daycard_harness.mjs';

// ledgerDoor finds the day panel's door for one day. It asserts there is
// exactly one rather than taking the first: a second door for the same day
// would mean the panel had been duplicated, and every assertion below would
// then be about whichever copy came first in the DOM.
function ledgerDoor(fx, ord) {
  const panels = [].slice.call(fx.ledger.querySelectorAll('.ldp'))
    .filter((p) => p.getAttribute('data-lday') === ord);
  assert.equal(panels.length, 1, `expected one day panel for day ${ord}`);
  const door = panels[0].querySelector('[data-dc-new]');
  assert.ok(door, `the day panel for ${ord} carries no create door`);
  return door;
}

function editorDate(fx) {
  const g = (sel) => fx.editor.querySelector(sel).value;
  return { year: g('[data-de-year]'), month: g('[data-de-month]'), day: g('[data-de-day]') };
}

test('the Ledger door opens the editor on ITS OWN day, with no card ever opened', () => {
  const fx = boot();
  // No day cell has been clicked, so `state` holds nothing at all. A door that
  // read `state.key` would resolve `index[''].days['']` and do nothing —
  // silently, which is the other half of the same defect.
  fx.fire('click', ledgerDoor(fx, '5'));

  assert.equal(fx.editor.popoverOpen, true, 'the editor never opened');
  assert.deepEqual(editorDate(fx), { year: '1523', month: '1', day: '5' });
});

test('the Ledger door does not create on the day the card was last opened on', () => {
  const fx = boot();
  // Open the card on day 3, the way a tap on the cell does…
  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.popoverOpen, true, 'the card never opened');
  // …then dismiss it. `closeCard` clears `state.open` and KEEPS `state.key`,
  // so a stale read does not even look empty — it looks like a valid day.
  fx.fire('click', fx.root);

  // Now use the Ledger's door for a DIFFERENT day.
  fx.fire('click', ledgerDoor(fx, '5'));
  assert.equal(fx.editor.popoverOpen, true, 'the editor never opened');
  assert.deepEqual(editorDate(fx), { year: '1523', month: '1', day: '5' },
    'the editor took the day the CARD was last opened on, not the day the door names');
});

test('the Ledger door writes the shipped create route, dated from the door', async () => {
  const fx = boot();
  fx.fire('click', ledgerDoor(fx, '4'));

  const ed = fx.editor;
  ed.querySelector('[data-de-name]').value = 'Frost fair';
  fx.fire('click', ed.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));

  assert.equal(fx.calls.length, 1, 'exactly one write, through the one shipped route');
  assert.equal(fx.calls[0].method, 'POST');
  assert.equal(fx.calls[0].url, '/campaigns/camp-1/calendars/cal-1/events');
  assert.equal(fx.calls[0].body.day, 4);
  assert.equal(fx.calls[0].body.name, 'Frost fair');
});

test('the card`s own door is unchanged — it still reads the open card`s day', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));

  assert.equal(fx.editor.popoverOpen, true);
  assert.deepEqual(editorDate(fx), { year: '1523', month: '1', day: '3' },
    'the card door carries no data-day and must keep resolving through `state`');
});

test('a viewer below the authoring floor gets no Ledger door at all', () => {
  const fx = boot({ canEdit: false });
  const doors = fx.ledger.querySelectorAll('[data-dc-new]');
  assert.equal(doors.length, 0,
    'permission is ABSENCE: no button, no disabled state, no title explaining one');
  // And the module refuses the whole branch anyway — `canEdit` is read off the
  // card's `data-dc-can-edit`, which the producer did not write.
  assert.equal(fx.editor, null, 'a player`s page carries no editor scaffold either');
});

test('the door does not double as a day cell — clicking it opens no card', () => {
  const fx = boot();
  fx.fire('click', ledgerDoor(fx, '5'));
  assert.equal(fx.card.popoverOpen, false,
    'the create branch returns before cellFrom is ever consulted; a door that also ' +
    'opened the card would put two boxes on screen disagreeing');
});
