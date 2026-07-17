// availability_out_this_week.test.mjs — C-SCHED-OUT-THIS-WEEK: the one-click
// "Out this week ✈" quick action on the player's own availability view.
//
// Covers: firing writes exactly the 7 dates of the current real week as
// full-day 'unavailable' exceptions; a date that already carries a
// hand-authored exception is left untouched (the key pin); Undo removes
// exactly the rows this action created and nothing else; an all-days-already-
// customized week is a no-op; a mid-week cap failure stops further writes but
// keeps what succeeded and stays undo-able; the day-strip cells repaint and
// stay clickable into the existing per-date editor; the sr-live region
// announces on fire.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot, wait, thisWeekDates } from './availability_harness.mjs';

function owtBtn(doc) { return doc.querySelector('[data-owt-btn]'); }
function owtStatus(doc) { return doc.querySelector('[data-owt-status]'); }
function dayCells(doc) { return doc.querySelectorAll('[data-owt-strip] .owt-day'); }

test('the button is visible at rest and fires exactly the 7 dates of the current week', async () => {
  const { doc, getExceptions } = boot();
  await wait(30);

  const btn = owtBtn(doc);
  assert.ok(btn, 'the Out this week button renders');
  assert.match(btn.textContent, /Out this week/);
  assert.equal(btn.getAttribute('aria-pressed'), 'false');
  assert.equal(dayCells(doc).length, 7, 'the week strip renders all 7 days');

  btn.click();
  await wait(50);

  const dates = thisWeekDates();
  const excs = getExceptions();
  assert.equal(excs.length, 7, 'exactly 7 rows written for an empty week');
  const gotDates = excs.map((e) => e.onDate).sort();
  assert.deepEqual(gotDates, dates.slice().sort());
  excs.forEach((e) => {
    assert.equal(e.startMinute, 0);
    assert.equal(e.endMinute, 1440);
    assert.equal(e.state, 'unavailable');
  });

  assert.match(owtBtn(doc).textContent, /Undo/);
  assert.equal(owtBtn(doc).getAttribute('aria-pressed'), 'true');
  assert.match(owtStatus(doc).textContent, /Marked 7 of 7 days out/);

  const outCells = doc.querySelectorAll('[data-owt-strip] .owt-day[data-owt-state="out"]');
  assert.equal(outCells.length, 7, 'every day chip repaints to the out state');
});

test('a pre-existing hand-authored exception is left untouched — the key pin', async () => {
  const dates = thisWeekDates();
  const handRow = { id: 'hand1', onDate: dates[2], startMinute: 9 * 60, endMinute: 17 * 60, state: 'available' };
  const { doc, getExceptions } = boot({ exceptions: [handRow] });
  await wait(30);

  owtBtn(doc).click();
  await wait(50);

  const excs = getExceptions();
  assert.equal(excs.length, 7, '6 new full-day rows + the 1 pre-existing row');
  const kept = excs.find((e) => e.id === 'hand1');
  assert.ok(kept, 'the hand-authored row survives untouched');
  assert.equal(kept.startMinute, 9 * 60);
  assert.equal(kept.endMinute, 17 * 60);
  assert.equal(kept.state, 'available');

  assert.match(owtStatus(doc).textContent, /Marked 6 of 7 days out/);
  assert.match(owtStatus(doc).textContent, /1 day already had a custom exception and was left alone/);

  const custom = doc.querySelectorAll('[data-owt-strip] .owt-day[data-owt-state="custom"]');
  const out = doc.querySelectorAll('[data-owt-strip] .owt-day[data-owt-state="out"]');
  assert.equal(custom.length, 1, 'the hand-authored day chip reads as custom, not out');
  assert.equal(out.length, 6);
});

test('Undo removes exactly the exceptions this action created, preserving the pre-existing one', async () => {
  const dates = thisWeekDates();
  const handRow = { id: 'hand1', onDate: dates[2], startMinute: 9 * 60, endMinute: 17 * 60, state: 'available' };
  const { doc, getExceptions } = boot({ exceptions: [handRow] });
  await wait(30);

  owtBtn(doc).click();
  await wait(50);
  assert.equal(getExceptions().length, 7);

  owtBtn(doc).click(); // same button, now in the Undo state
  await wait(50);

  const excs = getExceptions();
  assert.equal(excs.length, 1, 'only the pre-existing row remains after undo');
  assert.equal(excs[0].id, 'hand1');

  assert.match(owtBtn(doc).textContent, /Out this week/);
  assert.equal(owtBtn(doc).getAttribute('aria-pressed'), 'false');
  assert.match(owtStatus(doc).textContent, /Undo complete/);

  const custom = doc.querySelectorAll('[data-owt-strip] .owt-day[data-owt-state="custom"]');
  const out = doc.querySelectorAll('[data-owt-strip] .owt-day[data-owt-state="out"]');
  assert.equal(custom.length, 1);
  assert.equal(out.length, 0, 'no day chip still reads as out after undo');
});

test('firing when every day already has a custom exception is a no-op', async () => {
  const dates = thisWeekDates();
  const seeds = dates.map((d, i) => ({ id: 'hand' + i, onDate: d, startMinute: 9 * 60, endMinute: 17 * 60, state: 'available' }));
  const { doc, getExceptions } = boot({ exceptions: seeds });
  await wait(30);

  owtBtn(doc).click();
  await wait(50);

  const excs = getExceptions();
  assert.equal(excs.length, 7, 'nothing added or removed');
  assert.deepEqual(excs.map((e) => e.id).sort(), seeds.map((e) => e.id).sort());

  assert.match(owtBtn(doc).textContent, /Out this week/, 'button stays idle — nothing was created to undo');
  assert.equal(owtBtn(doc).getAttribute('aria-pressed'), 'false');
  assert.match(owtStatus(doc).textContent, /already has a custom exception — nothing changed/);
});

test('a mid-week cap failure stops further writes but keeps what succeeded, and stays undo-able', async () => {
  const { doc, getExceptions } = boot({ failPutAfter: 2 });
  await wait(30);

  owtBtn(doc).click();
  await wait(60);

  const excs = getExceptions();
  assert.equal(excs.length, 2, 'only the first 2 sequential writes succeeded before the simulated cap failure');
  assert.match(owtStatus(doc).textContent, /Marked 2 of 7 days out/);
  assert.match(owtStatus(doc).textContent, /too many availability exceptions/);

  // The partial result is still undo-able — Undo should target exactly those 2.
  assert.match(owtBtn(doc).textContent, /Undo/);
  owtBtn(doc).click();
  await wait(50);

  assert.equal(getExceptions().length, 0, 'undo removes the partial write cleanly');
  assert.match(owtBtn(doc).textContent, /Out this week/);
});

test('clicking a day chip opens the existing per-date editor for that date', async () => {
  const { doc } = boot();
  await wait(30);

  dayCells(doc)[0].click();
  await wait(30);

  const editorCells = doc.querySelectorAll('[data-exc-editor] .avail-cell');
  assert.equal(editorCells.length, 24, 'the single-day 24-hour editor opens for the clicked day');
});

test('the sr-live region announces on fire', async () => {
  const { doc, live } = boot();
  await wait(30);

  owtBtn(doc).click();
  await wait(80); // fire's async chain + announce()'s own 30ms delay

  assert.match(live.textContent, /Marked 7 of 7 days out/);
});

test('a fresh session (reload) shows a prior fire as customized, not Undo — in-memory-only state', async () => {
  // First "session": fire, then read back what the server now holds.
  const first = boot();
  await wait(30);
  owtBtn(first.doc).click();
  await wait(50);
  const savedRows = first.getExceptions();
  assert.equal(savedRows.length, 7);

  // A fresh boot() simulates a reload against the same server data — the new
  // instance has no bookkeeping of what "this action" wrote, by design.
  const second = boot({ exceptions: savedRows });
  await wait(30);

  assert.match(owtBtn(second.doc).textContent, /Out this week/, 'no Undo affordance survives a reload');
  assert.equal(owtBtn(second.doc).getAttribute('aria-pressed'), 'false');
  const custom = second.doc.querySelectorAll('[data-owt-strip] .owt-day[data-owt-state="custom"]');
  const out = second.doc.querySelectorAll('[data-owt-strip] .owt-day[data-owt-state="out"]');
  assert.equal(custom.length, 7, 'the week strip reads the surviving data as ordinary custom exceptions');
  assert.equal(out.length, 0);
});
