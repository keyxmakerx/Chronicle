// availability_morph.test.mjs — regression guard for C-CAL-BETA-RESCUE #2:
// "clicking a day in month view is broken on desktop".
//
// Root cause: the month<->week morph (AvailabilityApp.setScale) schedules a 340ms
// setTimeout that adds `hidden` to the OUTGOING view. Clicking a month-grid day
// fires setScale('week') while a prior morph's cleanup timer is still pending (the
// current week is cached, so the month grid renders instantly — this window is hit
// routinely). The stale timer then fired and re-hid the week view we had just
// navigated to, leaving BOTH views hidden: a blank calendar.
//
// Fix: setScale cancels any pending cleanup timer (this._morphTimer) and clears
// both views' transient `leave`/`enter` classes before starting a new transition,
// so the incoming view is unambiguously shown and only the outgoing one is hidden.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot, wait } from './availability_harness.mjs';

async function openOverlay(doc) {
  await wait(20);
  doc.querySelector('[data-avail-tab="overlay"]').click();
  await wait(40);
}
function firstInMonthDay(doc) {
  for (const d of doc.querySelectorAll('.ov-mday')) if (!d.classList.contains('out')) return d;
  return null;
}
const visible = (el) => !el.classList.contains('hidden') && !el.classList.contains('leave');

test('clicking a day <340ms after switching to Month lands on the week view (not a blank calendar)', async () => {
  const { doc } = boot();
  await openOverlay(doc);
  const monthView = doc.querySelector('[data-ov-monthview]');
  const weekView = doc.querySelector('[data-ov-weekview]');

  // Switch to Month, then click a day BEFORE the 340ms morph cleanup fires — the
  // exact race that produced the blank calendar in production.
  doc.querySelector('[data-ov-scale="month"]').click();
  await wait(60);
  const day = firstInMonthDay(doc);
  assert.ok(day, 'month grid rendered clickable day cells');
  day.click();

  // Let BOTH morph cleanup timers elapse.
  await wait(420);

  assert.equal(visible(weekView), true, 'week view must be visible after clicking a day');
  assert.equal(monthView.classList.contains('hidden'), true, 'month view must end hidden');
  assert.equal(weekView.classList.contains('hidden'), false, 'week view must NOT be hidden by a stale timer');
  const grid = doc.querySelector('[data-ov-grid]');
  assert.ok(grid && grid.children.length >= 1, 'week grid content is present (calendar not blank)');
});

test('the day-click navigates to the clicked day\'s week', async () => {
  const { doc } = boot();
  await openOverlay(doc);
  doc.querySelector('[data-ov-scale="month"]').click();
  await wait(60);
  const day = firstInMonthDay(doc);
  day.click();
  await wait(420);
  const label = doc.querySelector('[data-week-label]').textContent;
  assert.match(label, /^Week of /, 'week label reflects the navigated week');
});

test('normal (un-raced) month->day still works when the morph is allowed to settle', async () => {
  const { doc } = boot();
  await openOverlay(doc);
  const monthView = doc.querySelector('[data-ov-monthview]');
  const weekView = doc.querySelector('[data-ov-weekview]');
  doc.querySelector('[data-ov-scale="month"]').click();
  await wait(420); // let the week->hidden cleanup fire first
  firstInMonthDay(doc).click();
  await wait(420);
  assert.equal(visible(weekView), true);
  assert.equal(monthView.classList.contains('hidden'), true);
});

test('repeated month<->day round-trips never blank the calendar', async () => {
  const { doc } = boot();
  await openOverlay(doc);
  const weekView = doc.querySelector('[data-ov-weekview]');
  for (let i = 0; i < 3; i++) {
    doc.querySelector('[data-ov-scale="month"]').click();
    await wait(60);
    firstInMonthDay(doc).click();
    await wait(420);
    assert.equal(visible(weekView), true, `round-trip ${i}: week view visible`);
  }
});

test('reduced-motion path also survives the fast day-click', async () => {
  const { doc } = boot({ reduced: true });
  await openOverlay(doc);
  const monthView = doc.querySelector('[data-ov-monthview]');
  const weekView = doc.querySelector('[data-ov-weekview]');
  doc.querySelector('[data-ov-scale="month"]').click();
  await wait(20);
  firstInMonthDay(doc).click();
  await wait(60);
  assert.equal(weekView.classList.contains('hidden'), false, 'week view visible (reduced motion)');
  assert.equal(monthView.classList.contains('hidden'), true, 'month view hidden (reduced motion)');
});
